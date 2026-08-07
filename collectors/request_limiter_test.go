// Copyright 2026 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package collectors

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/api/monitoring/v3"
	"google.golang.org/api/option"
)

// noopCounterStore and noopHistogramStore are minimal stand-ins for
// DeltaCounterStore/DeltaHistogramStore. The real implementations live in
// the delta package, which imports collectors, so they can't be used here
// without a circular import; a real implementation isn't needed for this
// test since AggregateDeltas is left disabled.
type noopCounterStore struct{}

func (noopCounterStore) Increment(*monitoring.MetricDescriptor, *ConstMetric) {}
func (noopCounterStore) ListMetrics(string) []*ConstMetric                    { return nil }

type noopHistogramStore struct{}

func (noopHistogramStore) Increment(*monitoring.MetricDescriptor, *HistogramMetric) {}
func (noopHistogramStore) ListMetrics(string) []*HistogramMetric                    { return nil }

// TestMonitoringCollector_RequestLimiterBoundsConcurrency verifies that a
// non-nil requestLimiter caps the number of concurrent TimeSeries.List
// requests a single Collect call can have in flight, regardless of how many
// metric descriptors are being fetched. This guards against the unbounded
// per-descriptor goroutine fan-out (one HTTP request + JSON decode per
// descriptor) that can spike memory enough to OOM the process when a project
// has many metric descriptors.
func TestMonitoringCollector_RequestLimiterBoundsConcurrency(t *testing.T) {
	const (
		numDescriptors = 6
		limit          = 2
	)

	var (
		mu          sync.Mutex
		inFlight    int
		maxInFlight int
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "metricDescriptors"):
			descriptors := make([]*monitoring.MetricDescriptor, 0, numDescriptors)
			for i := 0; i < numDescriptors; i++ {
				descriptors = append(descriptors, &monitoring.MetricDescriptor{
					Type: "custom.googleapis.com/metric_" + string(rune('a'+i)),
				})
			}
			writeJSONResponse(w, &monitoring.ListMetricDescriptorsResponse{MetricDescriptors: descriptors})

		case strings.Contains(r.URL.Path, "timeSeries"):
			mu.Lock()
			inFlight++
			if inFlight > maxInFlight {
				maxInFlight = inFlight
			}
			mu.Unlock()

			// Hold the request open briefly so concurrent requests overlap
			// long enough for the test to observe them.
			time.Sleep(50 * time.Millisecond)

			mu.Lock()
			inFlight--
			mu.Unlock()

			writeJSONResponse(w, &monitoring.ListTimeSeriesResponse{})

		default:
			http.NotFound(w, r)
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	ctx := context.Background()
	service, err := monitoring.NewService(ctx,
		option.WithHTTPClient(server.Client()),
		option.WithEndpoint(server.URL),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("failed to create monitoring service: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	limiter := make(chan struct{}, limit)

	collector, err := NewMonitoringCollector(
		"test-project",
		service,
		MonitoringCollectorOptions{
			MetricTypePrefixes: []string{"custom.googleapis.com"},
			RequestInterval:    5 * time.Minute,
			DescriptorCacheTTL: 0,
		},
		logger,
		noopCounterStore{},
		noopHistogramStore{},
		limiter,
	)
	if err != nil {
		t.Fatalf("failed to create collector: %v", err)
	}

	ch := make(chan prometheus.Metric, 100)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range ch {
		}
	}()

	collector.Collect(ch)
	close(ch)
	<-done

	if maxInFlight > limit {
		t.Fatalf("observed %d concurrent TimeSeries.List requests, want <= %d", maxInFlight, limit)
	}
	if maxInFlight < limit {
		t.Fatalf("expected concurrency to reach the configured limit %d, got max observed %d; test may not be exercising real contention", limit, maxInFlight)
	}
}

func writeJSONResponse(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

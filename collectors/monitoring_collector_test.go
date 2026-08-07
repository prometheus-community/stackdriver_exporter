// Copyright 2023 The Prometheus Authors
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
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/api/monitoring/v3"
	"google.golang.org/api/option"
)

func TestIsGoogleMetric(t *testing.T) {
	good := []string{
		"pubsub.googleapis.com/some/metric",
	}

	bad := []string{
		"my.metric/a/b",
		"my.metrics/pubsub.googleapis.com/a",
	}

	for _, e := range good {
		if !isGoogleMetric(e) {
			t.Errorf("should be a google metric: %s", e)
		}
	}

	for _, e := range bad {
		if isGoogleMetric(e) {
			t.Errorf("should not be a google metric: %s", e)
		}
	}
}

func TestParseMetricExtraFilters(t *testing.T) {
	input := []string{
		"pubsub.googleapis.com/subscription:resource.labels.subscription_id=monitoring.regex.full_match(\"my-subs-prefix.*\")",
		"missing-separator",
		"compute.googleapis.com/instance:metric.labels.instance_name=\"example:vm\"",
	}

	got := ParseMetricExtraFilters(input)
	want := []MetricFilter{
		{
			TargetedMetricPrefix: "pubsub.googleapis.com/subscription",
			FilterQuery:          "resource.labels.subscription_id=monitoring.regex.full_match(\"my-subs-prefix.*\")",
		},
		{
			TargetedMetricPrefix: "compute.googleapis.com/instance",
			FilterQuery:          "metric.labels.instance_name=\"example:vm\"",
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseMetricExtraFilters() = %#v, want %#v", got, want)
	}
}

func TestSplitExtraFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantPrefix string
		wantFilter string
	}{
		{
			name:       "incomplete filter returns empty",
			input:      "This_is__a-MetricName.Example/with/no/filter",
			wantPrefix: "",
			wantFilter: "",
		},
		{
			name:       "basic filter",
			input:      "This_is__a-MetricName.Example/with:filter.name=filter_value",
			wantPrefix: "This_is__a-MetricName.Example/with",
			wantFilter: "filter.name=filter_value",
		},
		{
			name:       "filter value containing the separator",
			input:      `This_is__a-MetricName.Example/with:filter.name="filter:value"`,
			wantPrefix: "This_is__a-MetricName.Example/with",
			wantFilter: `filter.name="filter:value"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotPrefix, gotFilter := splitExtraFilter(tt.input, ":")
			if gotPrefix != tt.wantPrefix || gotFilter != tt.wantFilter {
				t.Fatalf("splitExtraFilter() = (%q, %q), want (%q, %q)", gotPrefix, gotFilter, tt.wantPrefix, tt.wantFilter)
			}
		})
	}
}

func TestProjectResource(t *testing.T) {
	t.Parallel()

	if got := projectResource("fake-project-1"); got != "projects/fake-project-1" {
		t.Fatalf("projectResource() = %q, want %q", got, "projects/fake-project-1")
	}
}

func TestAcquireReleaseRequestLimiterNilIsUnbounded(t *testing.T) {
	t.Parallel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		acquireRequestLimiter(nil)
		releaseRequestLimiter(nil)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("acquire/releaseRequestLimiter blocked on a nil limiter")
	}
}

func TestAcquireRequestLimiterBlocksWhenFull(t *testing.T) {
	t.Parallel()

	sem := make(chan struct{}, 1)
	acquireRequestLimiter(sem)

	acquired := make(chan struct{})
	go func() {
		acquireRequestLimiter(sem)
		close(acquired)
	}()

	select {
	case <-acquired:
		t.Fatal("second acquireRequestLimiter succeeded while limiter was full")
	case <-time.After(50 * time.Millisecond):
	}

	releaseRequestLimiter(sem)

	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("acquireRequestLimiter did not unblock after releaseRequestLimiter")
	}
}

// TestRequestLimiterBoundsConcurrency verifies that a non-nil requestLimiter
// caps the number of concurrent TimeSeries.List requests a single Collect
// call can have in flight, regardless of how many metric descriptors are
// being fetched. This guards against the unbounded per-descriptor goroutine
// fan-out (one HTTP request + JSON decode per descriptor) that can spike
// memory enough to OOM the process when a project has many metric
// descriptors.
func TestRequestLimiterBoundsConcurrency(t *testing.T) {
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

// noopCounterStore and noopHistogramStore are minimal stand-ins for
// DeltaCounterStore/DeltaHistogramStore. The real implementations live in
// the delta package, which imports collectors, so they can't be used here
// without a circular import; a real implementation isn't needed since these
// tests leave AggregateDeltas disabled.
type noopCounterStore struct{}

func (noopCounterStore) Increment(*monitoring.MetricDescriptor, *ConstMetric) {}
func (noopCounterStore) ListMetrics(string) []*ConstMetric                    { return nil }

type noopHistogramStore struct{}

func (noopHistogramStore) Increment(*monitoring.MetricDescriptor, *HistogramMetric) {}
func (noopHistogramStore) ListMetrics(string) []*HistogramMetric                    { return nil }

func writeJSONResponse(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

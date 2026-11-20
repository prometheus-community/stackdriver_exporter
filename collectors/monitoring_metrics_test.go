// Copyright 2020 The Prometheus Authors
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
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/api/monitoring/v3"
)

func TestNormalizeMetricName(t *testing.T) {
	t.Parallel()

	got := normalizeMetricName("This_is__a-MetricName.Example/with:0totals")
	want := "this_is_a_metric_name_example_with_0_totals"
	if got != want {
		t.Fatalf("normalizeMetricName() = %q, want %q", got, want)
	}
}

type collectFunc func(ch chan<- prometheus.Metric)

func (f collectFunc) Describe(chan<- *prometheus.Desc) {}

func (f collectFunc) Collect(ch chan<- prometheus.Metric) { f(ch) }

func TestCustomMetricWithDifferentDescriptionsPerProjectGathers(t *testing.T) {
	t.Parallel()

	const metricType = "custom.googleapis.com/spark/executor/bigquery-metrics-source/bytesRead"
	projects := []struct {
		id          string
		description string
	}{
		{"project-a", "Auto-created custom metric."},
		{"project-b", ""},
	}

	helpStore := NewMetricHelpStore()
	collector := collectFunc(func(ch chan<- prometheus.Metric) {
		for _, p := range projects {
			descriptor := &monitoring.MetricDescriptor{Type: metricType, Description: p.description}
			tsm, err := newTimeSeriesMetrics(descriptor, ch, false, nil, nil, false, helpStore)
			if err != nil {
				t.Errorf("newTimeSeriesMetrics: %v", err)
				return
			}
			ts := &monitoring.TimeSeries{
				Metric:   &monitoring.Metric{Type: metricType},
				Resource: &monitoring.MonitoredResource{Type: "cloud_dataproc_batch"},
			}
			tsm.CollectNewConstMetric(ts, time.Now(), []string{"project_id"}, prometheus.CounterValue, 1, []string{p.id}, "CUMULATIVE")
		}
	})

	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	if len(families) != 1 {
		t.Fatalf("Gather() returned %d families, want 1", len(families))
	}
	if got, want := families[0].GetHelp(), projects[0].description; got != want {
		t.Fatalf("family %s has help %q, want the first description seen, %q", families[0].GetName(), got, want)
	}
	if got := len(families[0].GetMetric()); got != 2 {
		t.Fatalf("family %s has %d metrics, want 2", families[0].GetName(), got)
	}
}

func TestMetricHelpStoreKeepsFirstDescriptionUnderConcurrency(t *testing.T) {
	t.Parallel()

	store := NewMetricHelpStore()
	const callers = 64
	got := make([]string, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got[i] = store.Help("metric_name", fmt.Sprintf("description %d", i))
		}()
	}
	wg.Wait()

	for i, help := range got {
		if help != got[0] {
			t.Fatalf("caller %d got help %q, caller 0 got %q", i, help, got[0])
		}
	}
}

func TestMetricHelpStoreZeroValue(t *testing.T) {
	t.Parallel()

	var store MetricHelpStore
	if got := store.Help("metric_name", "first"); got != "first" {
		t.Fatalf("Help() = %q, want %q", got, "first")
	}
	if got := store.Help("metric_name", "second"); got != "first" {
		t.Fatalf("Help() = %q, want %q", got, "first")
	}
}

func TestMetricHelpStoreDropsUnusedNames(t *testing.T) {
	t.Parallel()

	now := time.Unix(0, 0)
	store := &MetricHelpStore{now: func() time.Time { return now }}
	store.Help("live_metric", "live")
	store.Help("gone_metric", "gone")

	// live_metric keeps being reported, gone_metric does not.
	for range 4 {
		now = now.Add(metricHelpTTL / 2)
		store.Help("live_metric", "other")
	}
	// A new name triggers the sweep.
	store.Help("new_metric", "new")

	if got := store.Help("live_metric", "other"); got != "live" {
		t.Fatalf("Help(live_metric) = %q, want %q", got, "live")
	}
	if got := store.Help("gone_metric", "replaced"); got != "replaced" {
		t.Fatalf("Help(gone_metric) = %q, want %q", got, "replaced")
	}
	if got := len(store.entries); got != 3 {
		t.Fatalf("store holds %d names, want 3", got)
	}
}

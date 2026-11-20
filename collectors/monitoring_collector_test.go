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
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
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

func TestMetricDescriptorsListActiveOnly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		activeOnly bool
		want       []string
	}{
		{
			name:       "set",
			activeOnly: true,
			want:       []string{"true"},
		},
		{
			name:       "unset",
			activeOnly: false,
			want:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			requests := make(chan url.Values, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v3/projects/fake-project-1/metricDescriptors" {
					t.Errorf("unexpected request path %q", r.URL.Path)
				}
				requests <- r.URL.Query()
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte("{}"))
			}))
			t.Cleanup(server.Close)

			service, err := monitoring.NewService(context.Background(),
				option.WithEndpoint(server.URL),
				option.WithHTTPClient(server.Client()),
				option.WithoutAuthentication(),
			)
			if err != nil {
				t.Fatalf("monitoring.NewService() error = %v", err)
			}

			collector, err := NewMonitoringCollector("fake-project-1", service, MonitoringCollectorOptions{
				MetricTypePrefixes: []string{"compute.googleapis.com/instance/cpu"},
				ActiveOnly:         tt.activeOnly,
			}, slog.New(slog.DiscardHandler), nil, nil)
			if err != nil {
				t.Fatalf("NewMonitoringCollector() error = %v", err)
			}

			if err := collector.reportMonitoringMetrics(make(chan prometheus.Metric), time.Now()); err != nil {
				t.Fatalf("reportMonitoringMetrics() error = %v", err)
			}

			got := (<-requests)["activeOnly"]
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("activeOnly query = %v, want %v", got, tt.want)
			}
		})
	}
}

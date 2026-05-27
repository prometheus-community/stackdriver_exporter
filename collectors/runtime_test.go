// Copyright The Prometheus Authors
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
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/prometheus-community/stackdriver_exporter/config"
)

func TestNewRuntimeRequiresValidatedConfig(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{MetricsPrefixes: []string{"compute.googleapis.com/"}}
	_, err := NewRuntime(context.Background(), slog.Default(), cfg, nil, nil)
	if err == nil {
		t.Fatal("expected error for un-Validated config, got nil")
	}
	if !strings.Contains(err.Error(), "validated") {
		t.Fatalf("expected error to mention validation, got %v", err)
	}
}

func TestDeduplicateProjectIDs(t *testing.T) {
	t.Parallel()

	input := []string{"project-b", "project-a", "project-b"}
	want := []string{"project-a", "project-b"}

	got := deduplicateProjectIDs(input)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("deduplicateProjectIDs() = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(input, []string{"project-b", "project-a", "project-b"}) {
		t.Fatalf("deduplicateProjectIDs() mutated input = %#v", input)
	}
}

func TestCollectorCacheTTL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  config.Config
		want time.Duration
	}{
		{
			name: "default fallback",
			cfg:  config.Config{},
			want: 2 * time.Hour,
		},
		{
			name: "aggregate deltas uses deltas ttl",
			cfg: config.Config{
				AggregateDeltas:    true,
				AggregateDeltasTTL: 30 * time.Minute,
			},
			want: 30 * time.Minute,
		},
		{
			name: "descriptor ttl wins when larger",
			cfg: config.Config{
				AggregateDeltas:    true,
				AggregateDeltasTTL: 30 * time.Minute,
				DescriptorCacheTTL: 45 * time.Minute,
			},
			want: 45 * time.Minute,
		},
		{
			name: "descriptor cache alone enables cache ttl",
			cfg: config.Config{
				DescriptorCacheTTL: 15 * time.Minute,
			},
			want: 15 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := collectorCacheTTL(&tt.cfg); got != tt.want {
				t.Fatalf("collectorCacheTTL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCollectorCacheKeyIsOrderIndependent(t *testing.T) {
	t.Parallel()

	a := collectorCacheKey("proj", []string{"compute.googleapis.com/", "pubsub.googleapis.com/"})
	b := collectorCacheKey("proj", []string{"pubsub.googleapis.com/", "compute.googleapis.com/"})
	if a != b {
		t.Fatalf("cache key changed with input order: %q vs %q", a, b)
	}
}

func TestParseMetricTypePrefixes(t *testing.T) {
	t.Parallel()

	input := []string{
		"redis.googleapis.com/stats/memory/usage",
		"loadbalancing.googleapis.com/https/request_count",
		"loadbalancing.googleapis.com",
		"redis.googleapis.com/stats/memory/usage_ratio",
		"redis.googleapis.com/stats/memory/usage_ratio",
	}
	want := []string{
		"loadbalancing.googleapis.com",
		"redis.googleapis.com/stats/memory/usage",
	}

	got := parseMetricTypePrefixes(input)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseMetricTypePrefixes() = %#v, want %#v", got, want)
	}

	wantInput := []string{
		"redis.googleapis.com/stats/memory/usage",
		"loadbalancing.googleapis.com/https/request_count",
		"loadbalancing.googleapis.com",
		"redis.googleapis.com/stats/memory/usage_ratio",
		"redis.googleapis.com/stats/memory/usage_ratio",
	}
	if !reflect.DeepEqual(input, wantInput) {
		t.Fatalf("parseMetricTypePrefixes mutated input = %#v, want %#v", input, wantInput)
	}
}

func TestRuntimeFilterMetricTypePrefixes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		configuredPrefix []string
		prefixFilter     []string
		want             []string
	}{
		{
			name:             "nil filter returns configured prefixes",
			configuredPrefix: []string{"compute.googleapis.com/instance/", "pubsub.googleapis.com/"},
			prefixFilter:     nil,
			want:             []string{"compute.googleapis.com/instance/", "pubsub.googleapis.com/"},
		},
		{
			name:             "filter narrows to matching subprefixes and parse drops shorter overlaps",
			configuredPrefix: []string{"redis.googleapis.com/stats/"},
			prefixFilter: []string{
				"redis.googleapis.com/stats/memory/usage",
				"redis.googleapis.com/stats/memory/usage_ratio",
				"redis.googleapis.com",
			},
			want: []string{"redis.googleapis.com/stats/memory/usage"},
		},
		{
			name:             "filter with no matches returns empty",
			configuredPrefix: []string{"compute.googleapis.com/instance/"},
			prefixFilter:     []string{"pubsub.googleapis.com/topic/foo"},
			want:             []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := &Runtime{cfg: &config.Config{MetricsPrefixes: tt.configuredPrefix}}
			got := r.filterMetricTypePrefixes(tt.prefixFilter)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("filterMetricTypePrefixes() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

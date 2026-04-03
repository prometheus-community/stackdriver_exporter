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

package config

import (
	"reflect"
	"testing"
	"time"

	"github.com/prometheus-community/stackdriver_exporter/collectors"
)

func TestParseDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    time.Duration
		wantErr bool
	}{
		{
			name: "valid",
			raw:  "5m",
			want: 5 * time.Minute,
		},
		{
			name:    "invalid",
			raw:     "nope",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseDuration("metrics_interval", tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseDuration() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Fatalf("ParseDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateRetryStatuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		codes   []int
		wantErr bool
	}{
		{
			name:  "valid",
			codes: []int{429, 503},
		},
		{
			name:    "too low",
			codes:   []int{99},
			wantErr: true,
		},
		{
			name:    "too high",
			codes:   []int{600},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateRetryStatuses(tt.codes)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateRetryStatuses() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCLIFlag_PanicsOnUnknownOption(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Fatal("CLIFlag() did not panic for an unknown option")
		}
	}()

	_ = CLIFlag("DoesNotExist")
}

func TestNormalizeProjectIDs(t *testing.T) {
	t.Parallel()

	input := []string{"project-b", "project-a", "project-b"}
	want := []string{"project-a", "project-b"}

	got := DeduplicateProjectIDs(input)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeProjectIDs() = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(input, []string{"project-b", "project-a", "project-b"}) {
		t.Fatalf("NormalizeProjectIDs() mutated input = %#v", input)
	}
}

func TestRuntimeConfigMonitoringCollectorOptions(t *testing.T) {
	t.Parallel()

	cfg := RuntimeConfig{
		MetricsPrefixes:      []string{"pubsub.googleapis.com/topic/", "compute.googleapis.com/instance"},
		Filters:              []string{"pubsub.googleapis.com/topic:resource.labels.topic_id=has_substring(\"prod\")"},
		MetricsInterval:      5 * time.Minute,
		MetricsOffset:        30 * time.Second,
		MetricsIngest:        true,
		FillMissing:          true,
		DropDelegated:        true,
		AggregateDeltas:      true,
		DescriptorTTL:        10 * time.Minute,
		DescriptorGoogleOnly: true,
	}

	want := collectors.MonitoringCollectorOptions{
		MetricTypePrefixes:        ParseMetricPrefixes(cfg.MetricsPrefixes),
		ExtraFilters:              []collectors.MetricFilter{{TargetedMetricPrefix: "pubsub.googleapis.com/topic", FilterQuery: "resource.labels.topic_id=has_substring(\"prod\")"}},
		RequestInterval:           5 * time.Minute,
		RequestOffset:             30 * time.Second,
		IngestDelay:               true,
		FillMissingLabels:         true,
		DropDelegatedProjects:     true,
		AggregateDeltas:           true,
		DescriptorCacheTTL:        10 * time.Minute,
		DescriptorCacheOnlyGoogle: true,
	}

	got := cfg.MonitoringCollectorOptions()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MonitoringCollectorOptions() = %#v, want %#v", got, want)
	}
}

func TestRuntimeConfigCollectorCacheTTL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  RuntimeConfig
		want time.Duration
	}{
		{
			name: "default fallback",
			cfg:  RuntimeConfig{},
			want: 2 * time.Hour,
		},
		{
			name: "aggregate deltas uses deltas ttl",
			cfg: RuntimeConfig{
				AggregateDeltas: true,
				DeltasTTL:       30 * time.Minute,
			},
			want: 30 * time.Minute,
		},
		{
			name: "descriptor ttl wins when larger",
			cfg: RuntimeConfig{
				AggregateDeltas: true,
				DeltasTTL:       30 * time.Minute,
				DescriptorTTL:   45 * time.Minute,
			},
			want: 45 * time.Minute,
		},
		{
			name: "descriptor cache alone enables cache ttl",
			cfg: RuntimeConfig{
				DeltasTTL:     30 * time.Minute,
				DescriptorTTL: 15 * time.Minute,
			},
			want: 30 * time.Minute,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.cfg.CollectorCacheTTL()
			if got != tt.want {
				t.Fatalf("CollectorCacheTTL() = %v, want %v", got, tt.want)
			}
		})
	}
}

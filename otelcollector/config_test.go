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

package otelcollector

import (
	"reflect"
	"testing"
	"time"

	"github.com/prometheus-community/stackdriver_exporter/config"
)

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid with project IDs",
			cfg: Config{
				ProjectIDs:      []string{"my-project"},
				MetricsPrefixes: []string{"compute.googleapis.com/instance"},
				HTTPTimeout:     "10s",
				MaxBackoff:      "5s",
				BackoffJitter:   "1s",
				MetricsInterval: "5m",
				MetricsOffset:   "0s",
				DeltasTTL:       "30m",
				DescriptorTTL:   "0s",
			},
		},
		{
			name: "valid with projects filter",
			cfg: Config{
				ProjectsFilter:  "parent.type:folder parent.id:123",
				MetricsPrefixes: []string{"pubsub.googleapis.com/subscription"},
				HTTPTimeout:     "10s",
				MaxBackoff:      "5s",
				BackoffJitter:   "1s",
				MetricsInterval: "5m",
				MetricsOffset:   "0s",
				DeltasTTL:       "30m",
				DescriptorTTL:   "0s",
			},
		},
		{
			name: "valid with implicit default project discovery",
			cfg: Config{
				MetricsPrefixes: []string{"compute.googleapis.com/instance"},
				HTTPTimeout:     "10s",
				MaxBackoff:      "5s",
				BackoffJitter:   "1s",
				MetricsInterval: "5m",
				MetricsOffset:   "0s",
				DeltasTTL:       "30m",
				DescriptorTTL:   "0s",
			},
		},
		{
			name: "invalid without metrics prefixes",
			cfg: Config{
				ProjectIDs:    []string{"my-project"},
				HTTPTimeout:   "10s",
				MaxBackoff:    "5s",
				BackoffJitter: "1s",
			},
			wantErr: true,
		},
		{
			name: "invalid timeout",
			cfg: Config{
				ProjectIDs:      []string{"my-project"},
				MetricsPrefixes: []string{"compute.googleapis.com/instance"},
				HTTPTimeout:     "not-a-duration",
				MaxBackoff:      "5s",
				BackoffJitter:   "1s",
				MetricsInterval: "5m",
				MetricsOffset:   "0s",
				DeltasTTL:       "30m",
				DescriptorTTL:   "0s",
			},
			wantErr: true,
		},
		{
			name: "invalid retry status",
			cfg: Config{
				ProjectIDs:      []string{"my-project"},
				MetricsPrefixes: []string{"compute.googleapis.com/instance"},
				HTTPTimeout:     "10s",
				MaxBackoff:      "5s",
				BackoffJitter:   "1s",
				MetricsInterval: "5m",
				MetricsOffset:   "0s",
				DeltasTTL:       "30m",
				DescriptorTTL:   "0s",
				RetryStatuses:   []int{99},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_Durations(t *testing.T) {
	t.Parallel()

	cfg := Config{
		ProjectIDs:      []string{"my-project"},
		MetricsPrefixes: []string{"compute.googleapis.com/instance"},
		HTTPTimeout:     "10s",
		MaxBackoff:      "5s",
		BackoffJitter:   "1s",
		MetricsInterval: "5m",
		MetricsOffset:   "2m",
		DeltasTTL:       "30m",
		DescriptorTTL:   "0s",
	}

	parsed, err := cfg.parsedDurations()
	if err != nil {
		t.Fatalf("parsedDurations() error = %v", err)
	}

	if parsed.HTTPTimeout != 10*time.Second {
		t.Fatalf("HTTPTimeout = %v, want %v", parsed.HTTPTimeout, 10*time.Second)
	}
	if parsed.MaxBackoff != 5*time.Second {
		t.Fatalf("MaxBackoff = %v, want %v", parsed.MaxBackoff, 5*time.Second)
	}
	if parsed.BackoffJitter != 1*time.Second {
		t.Fatalf("BackoffJitter = %v, want %v", parsed.BackoffJitter, 1*time.Second)
	}
	if parsed.MetricsInterval != 5*time.Minute {
		t.Fatalf("MetricsInterval = %v, want %v", parsed.MetricsInterval, 5*time.Minute)
	}
	if parsed.MetricsOffset != 2*time.Minute {
		t.Fatalf("MetricsOffset = %v, want %v", parsed.MetricsOffset, 2*time.Minute)
	}
	if parsed.DeltasTTL != 30*time.Minute {
		t.Fatalf("DeltasTTL = %v, want %v", parsed.DeltasTTL, 30*time.Minute)
	}
	if parsed.DescriptorTTL != 0 {
		t.Fatalf("DescriptorTTL = %v, want %v", parsed.DescriptorTTL, 0*time.Second)
	}
}

func TestDefaultComponentDefaultsMatchOptionDefaults(t *testing.T) {
	t.Parallel()

	defaults := defaultComponentDefaults()

	for _, option := range config.AllOptions {
		value, ok := defaults[option.OTelKey]
		if option.Default != nil {
			if !ok {
				t.Fatalf("defaultComponentDefaults() missing key %q", option.OTelKey)
			}
			if !reflect.DeepEqual(value, option.Default) {
				t.Fatalf("defaultComponentDefaults()[%q] = %#v, want %#v", option.OTelKey, value, option.Default)
			}
			continue
		}

		if ok {
			t.Fatalf("defaultComponentDefaults() unexpectedly includes key %q", option.OTelKey)
		}
	}
}

func TestConfigMapstructureTagsMatchAllOptions(t *testing.T) {
	t.Parallel()

	cfgType := reflect.TypeOf(Config{})
	tags := make(map[string]struct{}, cfgType.NumField())
	for i := 0; i < cfgType.NumField(); i++ {
		tag := cfgType.Field(i).Tag.Get("mapstructure")
		if tag == "" {
			continue
		}
		tags[tag] = struct{}{}
	}

	optionKeys := make(map[string]struct{}, len(config.AllOptions))
	for _, option := range config.AllOptions {
		optionKeys[option.OTelKey] = struct{}{}
		if _, ok := tags[option.OTelKey]; !ok {
			t.Fatalf("Config is missing mapstructure tag %q", option.OTelKey)
		}
	}

	for tag := range tags {
		if _, ok := optionKeys[tag]; !ok {
			t.Fatalf("AllOptions is missing config key %q", tag)
		}
	}
}

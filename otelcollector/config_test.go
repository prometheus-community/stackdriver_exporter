package otelcollector

import (
	"testing"
	"time"
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

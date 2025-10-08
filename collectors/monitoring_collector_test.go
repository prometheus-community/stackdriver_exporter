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
	"log/slog"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/api/monitoring/v3"
)

func TestIsGoogleMetric(t *testing.T) {
	good := []string{
		"pubsub.googleapis.com/some/metric",
		"compute.googleapis.com/instance/cpu/utilization",
		"cloudsql.googleapis.com/database/cpu/utilization",
		"storage.googleapis.com/api/request_count",
		"custom.googleapis.com/my_metric",
	}

	bad := []string{
		"my.metric/a/b",
		"my.metrics/pubsub.googleapis.com/a",
		"mycompany.com/metrics/requests",
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

func TestGoogleDescriptorCache(t *testing.T) {
	ttl := 1 * time.Second
	innerCache := newDescriptorCache(ttl)
	cache := &googleDescriptorCache{inner: innerCache}

	googleMetric := "pubsub.googleapis.com/topic/num_undelivered_messages"
	customMetric := "custom.googleapis.com/my_metric"

	googleDescriptors := []*monitoring.MetricDescriptor{
		{Type: googleMetric, DisplayName: "Google Metric"},
	}
	customDescriptors := []*monitoring.MetricDescriptor{
		{Type: customMetric, DisplayName: "Custom Metric"},
	}

	// Test that Google metrics are cached
	cache.Store(googleMetric, googleDescriptors)
	result := cache.Lookup(googleMetric)
	if result == nil {
		t.Error("Google metric should be cached")
	}
	if len(result) != 1 || result[0].Type != googleMetric {
		t.Error("Cached Google metric should match stored value")
	}

	// Test that custom.googleapis.com metrics are also cached (they are Google metrics)
	cache.Store(customMetric, customDescriptors)
	result = cache.Lookup(customMetric)
	if result == nil {
		t.Error("Custom Google metric should be cached")
	}
	if len(result) != 1 || result[0].Type != customMetric {
		t.Error("Cached custom Google metric should match stored value")
	}

	// Test expiration
	time.Sleep(2 * ttl)
	result = cache.Lookup(googleMetric)
	if result != nil {
		t.Error("Cached Google metric should have expired")
	}
}

func TestNoopDescriptorCache(t *testing.T) {
	cache := &noopDescriptorCache{}

	descriptors := []*monitoring.MetricDescriptor{
		{Type: "test.metric", DisplayName: "Test Metric"},
	}

	// Test that Lookup always returns nil
	result := cache.Lookup("any.prefix")
	if result != nil {
		t.Error("Noop cache should always return nil on lookup")
	}

	// Test that Store does nothing (no panic)
	cache.Store("any.prefix", descriptors)
	result = cache.Lookup("any.prefix")
	if result != nil {
		t.Error("Noop cache should still return nil after store")
	}
}

func TestNewMonitoringCollector(t *testing.T) {
	logger := slog.Default()
	monitoringService := &monitoring.Service{}

	tests := []struct {
		name        string
		projectID   string
		opts        MonitoringCollectorOptions
		expectError bool
	}{
		{
			name:      "basic collector creation",
			projectID: "test-project",
			opts: MonitoringCollectorOptions{
				MetricTypePrefixes: []string{"pubsub.googleapis.com"},
				RequestInterval:    5 * time.Minute,
			},
			expectError: false,
		},
		{
			name:      "collector with descriptor cache",
			projectID: "test-project",
			opts: MonitoringCollectorOptions{
				MetricTypePrefixes:        []string{"pubsub.googleapis.com"},
				RequestInterval:           5 * time.Minute,
				DescriptorCacheTTL:        10 * time.Minute,
				DescriptorCacheOnlyGoogle: true,
			},
			expectError: false,
		},
		{
			name:      "collector with delta aggregation",
			projectID: "test-project",
			opts: MonitoringCollectorOptions{
				MetricTypePrefixes: []string{"pubsub.googleapis.com"},
				RequestInterval:    5 * time.Minute,
				AggregateDeltas:    true,
			},
			expectError: false,
		},
		{
			name:      "collector with all options",
			projectID: "test-project",
			opts: MonitoringCollectorOptions{
				MetricTypePrefixes:        []string{"pubsub.googleapis.com"},
				ExtraFilters:              []MetricFilter{{TargetedMetricPrefix: "pubsub.googleapis.com", FilterQuery: "resource.labels.topic_id=\"test\""}},
				MetricAggregationConfigs:  []MetricAggregationConfig{{TargetedMetricPrefix: "pubsub.googleapis.com", CrossSeriesReducer: "REDUCE_SUM"}},
				RequestInterval:           5 * time.Minute,
				RequestOffset:             1 * time.Minute,
				IngestDelay:               true,
				FillMissingLabels:         true,
				DropDelegatedProjects:     true,
				AggregateDeltas:           true,
				DescriptorCacheTTL:        10 * time.Minute,
				DescriptorCacheOnlyGoogle: false,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector, err := NewMonitoringCollector(tt.projectID, monitoringService, tt.opts, logger, nil, nil)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if err != nil {
				return
			}

			// Verify basic fields
			if collector.projectID != tt.projectID {
				t.Errorf("Expected projectID %s, got %s", tt.projectID, collector.projectID)
			}

			if len(collector.metricsTypePrefixes) != len(tt.opts.MetricTypePrefixes) {
				t.Errorf("Expected %d metric prefixes, got %d", len(tt.opts.MetricTypePrefixes), len(collector.metricsTypePrefixes))
			}

			if collector.metricsInterval != tt.opts.RequestInterval {
				t.Errorf("Expected interval %v, got %v", tt.opts.RequestInterval, collector.metricsInterval)
			}

			if collector.metricsOffset != tt.opts.RequestOffset {
				t.Errorf("Expected offset %v, got %v", tt.opts.RequestOffset, collector.metricsOffset)
			}

			if collector.metricsIngestDelay != tt.opts.IngestDelay {
				t.Errorf("Expected ingest delay %v, got %v", tt.opts.IngestDelay, collector.metricsIngestDelay)
			}

			if collector.collectorFillMissingLabels != tt.opts.FillMissingLabels {
				t.Errorf("Expected fill missing labels %v, got %v", tt.opts.FillMissingLabels, collector.collectorFillMissingLabels)
			}

			if collector.monitoringDropDelegatedProjects != tt.opts.DropDelegatedProjects {
				t.Errorf("Expected drop delegated projects %v, got %v", tt.opts.DropDelegatedProjects, collector.monitoringDropDelegatedProjects)
			}

			if collector.aggregateDeltas != tt.opts.AggregateDeltas {
				t.Errorf("Expected aggregate deltas %v, got %v", tt.opts.AggregateDeltas, collector.aggregateDeltas)
			}

			// Verify descriptor cache type
			if tt.opts.DescriptorCacheTTL == 0 {
				if _, ok := collector.descriptorCache.(*noopDescriptorCache); !ok {
					t.Error("Expected noop descriptor cache when TTL is 0")
				}
			} else if tt.opts.DescriptorCacheOnlyGoogle {
				if _, ok := collector.descriptorCache.(*googleDescriptorCache); !ok {
					t.Error("Expected google descriptor cache when only Google is enabled")
				}
			} else {
				if _, ok := collector.descriptorCache.(*descriptorCache); !ok {
					t.Error("Expected regular descriptor cache when TTL > 0 and not Google-only")
				}
			}

			// Verify metrics are created
			if collector.apiCallsTotalMetric == nil {
				t.Error("Expected apiCallsTotalMetric to be created")
			}
			if collector.scrapesTotalMetric == nil {
				t.Error("Expected scrapesTotalMetric to be created")
			}
			if collector.scrapeErrorsTotalMetric == nil {
				t.Error("Expected scrapeErrorsTotalMetric to be created")
			}
			if collector.lastScrapeErrorMetric == nil {
				t.Error("Expected lastScrapeErrorMetric to be created")
			}
			if collector.lastScrapeTimestampMetric == nil {
				t.Error("Expected lastScrapeTimestampMetric to be created")
			}
			if collector.lastScrapeDurationSecondsMetric == nil {
				t.Error("Expected lastScrapeDurationSecondsMetric to be created")
			}
		})
	}
}

func TestMonitoringCollectorDescribe(t *testing.T) {
	logger := slog.Default()
	monitoringService := &monitoring.Service{}
	opts := MonitoringCollectorOptions{
		MetricTypePrefixes: []string{"pubsub.googleapis.com"},
		RequestInterval:    5 * time.Minute,
	}

	collector, err := NewMonitoringCollector("test-project", monitoringService, opts, logger, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create collector: %v", err)
	}

	// Create a channel to collect descriptions
	ch := make(chan *prometheus.Desc, 10)

	// Call Describe
	collector.Describe(ch)
	close(ch)

	// Count the descriptions
	count := 0
	for range ch {
		count++
	}

	// Should have 6 metrics: api_calls_total, scrapes_total, scrape_errors_total,
	// last_scrape_error, last_scrape_timestamp, last_scrape_duration_seconds
	expectedCount := 6
	if count != expectedCount {
		t.Errorf("Expected %d metric descriptions, got %d", expectedCount, count)
	}
}

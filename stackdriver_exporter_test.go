// Copyright 2024 The Prometheus Authors
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

package main

import (
	"log/slog"
	"reflect"
	"testing"

	"github.com/prometheus-community/stackdriver_exporter/collectors"
)

func TestParseMetricTypePrefixes(t *testing.T) {
	inputPrefixes := []string{
		"redis.googleapis.com/stats/memory/usage",
		"loadbalancing.googleapis.com/https/request_count",
		"loadbalancing.googleapis.com",
		"redis.googleapis.com/stats/memory/usage_ratio",
		"redis.googleapis.com/stats/memory/usage_ratio",
	}
	expectedOutputPrefixes := []string{
		"loadbalancing.googleapis.com",
		"redis.googleapis.com/stats/memory/usage",
	}

	outputPrefixes := parseMetricTypePrefixes(inputPrefixes)

	if !reflect.DeepEqual(outputPrefixes, expectedOutputPrefixes) {
		t.Errorf("Metric type prefix sanitization did not produce expected output. Expected:\n%s\nGot:\n%s", expectedOutputPrefixes, outputPrefixes)
	}
}

func TestFilterMetricTypePrefixes(t *testing.T) {
	metricPrefixes := []string{
		"redis.googleapis.com/stats/",
	}

	h := &handler{
		metricsPrefixes: metricPrefixes,
	}

	inputFilters := map[string]bool{
		"redis.googleapis.com/stats/memory/usage":       true,
		"redis.googleapis.com/stats/memory/usage_ratio": true,
		"redis.googleapis.com":                          true,
	}

	expectedOutputPrefixes := []string{
		"redis.googleapis.com/stats/memory/usage",
	}

	outputPrefixes := h.filterMetricTypePrefixes(inputFilters)

	if !reflect.DeepEqual(outputPrefixes, expectedOutputPrefixes) {
		t.Errorf("filterMetricTypePrefixes did not produce expected output. Expected:\n%s\nGot:\n%s", expectedOutputPrefixes, outputPrefixes)
	}
}

func TestParseMetricsWithAggregations(t *testing.T) {
	logger := slog.Default()

	tests := []struct {
		name     string
		input    []string
		expected []collectors.MetricAggregationConfig
	}{
		{
			name: "valid single aggregation config",
			input: []string{
				"custom.googleapis.com/my_metric:60s:REDUCE_SUM:metric.labels.instance_id,resource.labels.zone:ALIGN_MEAN",
			},
			expected: []collectors.MetricAggregationConfig{
				{
					TargetedMetricPrefix: "custom.googleapis.com/my_metric",
					AlignmentPeriod:      "60s",
					CrossSeriesReducer:   "REDUCE_SUM",
					GroupByFields:        []string{"metric.labels.instance_id", "resource.labels.zone"},
					PerSeriesAligner:     "ALIGN_MEAN",
				},
			},
		},
		{
			name: "valid multiple aggregation configs",
			input: []string{
				"custom.googleapis.com/my_metric:60s:REDUCE_SUM:metric.labels.instance_id,resource.labels.zone:ALIGN_MEAN",
				"pubsub.googleapis.com/subscription:300s:REDUCE_MEAN:resource.labels.subscription_id:ALIGN_RATE",
			},
			expected: []collectors.MetricAggregationConfig{
				{
					TargetedMetricPrefix: "custom.googleapis.com/my_metric",
					AlignmentPeriod:      "60s",
					CrossSeriesReducer:   "REDUCE_SUM",
					GroupByFields:        []string{"metric.labels.instance_id", "resource.labels.zone"},
					PerSeriesAligner:     "ALIGN_MEAN",
				},
				{
					TargetedMetricPrefix: "pubsub.googleapis.com/subscription",
					AlignmentPeriod:      "300s",
					CrossSeriesReducer:   "REDUCE_MEAN",
					GroupByFields:        []string{"resource.labels.subscription_id"},
					PerSeriesAligner:     "ALIGN_RATE",
				},
			},
		},
		{
			name: "valid config with single group by field",
			input: []string{
				"compute.googleapis.com/instance:60s:REDUCE_SUM:resource.labels.instance_name:ALIGN_MEAN",
			},
			expected: []collectors.MetricAggregationConfig{
				{
					TargetedMetricPrefix: "compute.googleapis.com/instance",
					AlignmentPeriod:      "60s",
					CrossSeriesReducer:   "REDUCE_SUM",
					GroupByFields:        []string{"resource.labels.instance_name"},
					PerSeriesAligner:     "ALIGN_MEAN",
				},
			},
		},
		{
			name: "valid config with multiple group by fields",
			input: []string{
				"compute.googleapis.com/instance:60s:REDUCE_SUM:resource.labels.instance_name,resource.labels.zone,metric.labels.instance_id:ALIGN_MEAN",
			},
			expected: []collectors.MetricAggregationConfig{
				{
					TargetedMetricPrefix: "compute.googleapis.com/instance",
					AlignmentPeriod:      "60s",
					CrossSeriesReducer:   "REDUCE_SUM",
					GroupByFields:        []string{"resource.labels.instance_name", "resource.labels.zone", "metric.labels.instance_id"},
					PerSeriesAligner:     "ALIGN_MEAN",
				},
			},
		},
		{
			name:     "empty input",
			input:    []string{},
			expected: []collectors.MetricAggregationConfig{},
		},
		{
			name: "invalid format - too few parts",
			input: []string{
				"custom.googleapis.com/my_metric:60s:REDUCE_SUM",
			},
			expected: []collectors.MetricAggregationConfig{},
		},
		{
			name: "invalid format - too many parts",
			input: []string{
				"custom.googleapis.com/my_metric:60s:REDUCE_SUM:metric.labels.instance_id:ALIGN_MEAN:extra_part",
			},
			expected: []collectors.MetricAggregationConfig{},
		},
		{
			name: "mixed valid and invalid configs",
			input: []string{
				"custom.googleapis.com/my_metric:60s:REDUCE_SUM:metric.labels.instance_id,resource.labels.zone:ALIGN_MEAN",
				"invalid_format",
				"pubsub.googleapis.com/subscription:300s:REDUCE_MEAN:resource.labels.subscription_id:ALIGN_RATE",
			},
			expected: []collectors.MetricAggregationConfig{
				{
					TargetedMetricPrefix: "custom.googleapis.com/my_metric",
					AlignmentPeriod:      "60s",
					CrossSeriesReducer:   "REDUCE_SUM",
					GroupByFields:        []string{"metric.labels.instance_id", "resource.labels.zone"},
					PerSeriesAligner:     "ALIGN_MEAN",
				},
				{
					TargetedMetricPrefix: "pubsub.googleapis.com/subscription",
					AlignmentPeriod:      "300s",
					CrossSeriesReducer:   "REDUCE_MEAN",
					GroupByFields:        []string{"resource.labels.subscription_id"},
					PerSeriesAligner:     "ALIGN_RATE",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseMetricsWithAggregations(logger, tt.input)

			// For empty expected results, check length instead of using reflect.DeepEqual
			if len(tt.expected) == 0 {
				if len(result) != 0 {
					t.Errorf("parseMetricsWithAggregations() returned %d items, want 0", len(result))
				}
			} else {
				if !reflect.DeepEqual(result, tt.expected) {
					t.Errorf("parseMetricsWithAggregations() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

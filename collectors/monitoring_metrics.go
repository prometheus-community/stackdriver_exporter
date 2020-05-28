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
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/api/monitoring/v3"

	"sort"

	"github.com/prometheus-community/stackdriver_exporter/utils"
)

func buildFQName(timeSeries *monitoring.TimeSeries) string {
	// The metric name to report is composed by the 3 parts:
	// 1. namespace is a constant prefix (stackdriver)
	// 2. subsystem is the monitored resource type (ie gce_instance)
	// 3. name is the metric type (ie compute.googleapis.com/instance/cpu/usage_time)
	return prometheus.BuildFQName("stackdriver", utils.NormalizeMetricName(timeSeries.Resource.Type), utils.NormalizeMetricName(timeSeries.Metric.Type))
}

type TimeSeriesMetrics struct {
	metricDescriptor *monitoring.MetricDescriptor
	ch               chan<- prometheus.Metric

	fillMissingLabels bool
	constMetrics      map[string][]ConstMetric
	histogramMetrics  map[string][]HistogramMetric
}

func (t *TimeSeriesMetrics) newMetricDesc(fqName string, labelKeys []string) *prometheus.Desc {
	return prometheus.NewDesc(
		fqName,
		t.metricDescriptor.Description,
		labelKeys,
		prometheus.Labels{},
	)
}

type ConstMetric struct {
	fqName      string
	labelKeys   []string
	valueType   prometheus.ValueType
	value       float64
	labelValues []string
	reportTime  time.Time

	keysHash uint64
}

type HistogramMetric struct {
	fqName      string
	labelKeys   []string
	dist        *monitoring.Distribution
	buckets     map[float64]uint64
	labelValues []string
	reportTime  time.Time

	keysHash uint64
}

func (t *TimeSeriesMetrics) CollectNewConstHistogram(timeSeries *monitoring.TimeSeries, reportTime time.Time, labelKeys []string, dist *monitoring.Distribution, buckets map[float64]uint64, labelValues []string) {
	fqName := buildFQName(timeSeries)

	if t.fillMissingLabels {
		vs, ok := t.histogramMetrics[fqName]
		if !ok {
			vs = make([]HistogramMetric, 0)
		}
		v := HistogramMetric{
			fqName:      fqName,
			labelKeys:   labelKeys,
			dist:        dist,
			buckets:     buckets,
			labelValues: labelValues,
			reportTime:  reportTime,

			keysHash: hashLabelKeys(labelKeys),
		}
		t.histogramMetrics[fqName] = append(vs, v)
		return
	}
	t.ch <- t.newConstHistogram(fqName, reportTime, labelKeys, dist, buckets, labelValues)
}

func (t *TimeSeriesMetrics) newConstHistogram(fqName string, reportTime time.Time, labelKeys []string, dist *monitoring.Distribution, buckets map[float64]uint64, labelValues []string) prometheus.Metric {
	return prometheus.NewMetricWithTimestamp(
		reportTime,
		prometheus.MustNewConstHistogram(
			t.newMetricDesc(fqName, labelKeys),
			uint64(dist.Count),
			dist.Mean*float64(dist.Count), // Stackdriver does not provide the sum, but we can fake it
			buckets,
			labelValues...,
		),
	)
}

func (t *TimeSeriesMetrics) CollectNewConstMetric(timeSeries *monitoring.TimeSeries, reportTime time.Time, labelKeys []string, metricValueType prometheus.ValueType, metricValue float64, labelValues []string) {
	fqName := buildFQName(timeSeries)

	if t.fillMissingLabels {
		vs, ok := t.constMetrics[fqName]
		if !ok {
			vs = make([]ConstMetric, 0)
		}
		v := ConstMetric{
			fqName:      fqName,
			labelKeys:   labelKeys,
			valueType:   metricValueType,
			value:       metricValue,
			labelValues: labelValues,
			reportTime:  reportTime,

			keysHash: hashLabelKeys(labelKeys),
		}
		t.constMetrics[fqName] = append(vs, v)
		return
	}
	t.ch <- t.newConstMetric(fqName, reportTime, labelKeys, metricValueType, metricValue, labelValues)
}

func (t *TimeSeriesMetrics) newConstMetric(fqName string, reportTime time.Time, labelKeys []string, metricValueType prometheus.ValueType, metricValue float64, labelValues []string) prometheus.Metric {
	return prometheus.NewMetricWithTimestamp(
		reportTime,
		prometheus.MustNewConstMetric(
			t.newMetricDesc(fqName, labelKeys),
			metricValueType,
			metricValue,
			labelValues...,
		),
	)
}

func hashLabelKeys(labelKeys []string) uint64 {
	dh := hashNew()
	sortedKeys := make([]string, len(labelKeys))
	copy(sortedKeys, labelKeys)
	sort.Strings(sortedKeys)
	for _, key := range sortedKeys {
		dh = hashAdd(dh, key)
		dh = hashAddByte(dh, separatorByte)
	}
	return dh
}

func (t *TimeSeriesMetrics) Complete() {
	t.completeConstMetrics()
	t.completeHistogramMetrics()
}

func (t *TimeSeriesMetrics) completeConstMetrics() {
	for _, vs := range t.constMetrics {
		if len(vs) > 1 {
			var needFill bool
			for i := 1; i < len(vs); i++ {
				if vs[0].keysHash != vs[i].keysHash {
					needFill = true
				}
			}
			if needFill {
				vs = fillConstMetricsLabels(vs)
			}
		}

		for _, v := range vs {
			t.ch <- t.newConstMetric(v.fqName, v.reportTime, v.labelKeys, v.valueType, v.value, v.labelValues)
		}
	}
}

func (t *TimeSeriesMetrics) completeHistogramMetrics() {
	for _, vs := range t.histogramMetrics {
		if len(vs) > 1 {
			var needFill bool
			for i := 1; i < len(vs); i++ {
				if vs[0].keysHash != vs[i].keysHash {
					needFill = true
				}
			}
			if needFill {
				vs = fillHistogramMetricsLabels(vs)
			}
		}
		for _, v := range vs {
			t.ch <- t.newConstHistogram(v.fqName, v.reportTime, v.labelKeys, v.dist, v.buckets, v.labelValues)
		}
	}
}

func fillConstMetricsLabels(metrics []ConstMetric) []ConstMetric {
	allKeys := make(map[string]struct{})
	for _, metric := range metrics {
		for _, key := range metric.labelKeys {
			allKeys[key] = struct{}{}
		}
	}
	result := make([]ConstMetric, len(metrics))
	for i, metric := range metrics {
		if len(metric.labelKeys) != len(allKeys) {
			metricKeys := make(map[string]struct{})
			for _, key := range metric.labelKeys {
				metricKeys[key] = struct{}{}
			}
			for key := range allKeys {
				if _, ok := metricKeys[key]; !ok {
					metric.labelKeys = append(metric.labelKeys, key)
					metric.labelValues = append(metric.labelValues, "")
				}
			}
		}
		result[i] = metric
	}

	return result
}

func fillHistogramMetricsLabels(metrics []HistogramMetric) []HistogramMetric {
	allKeys := make(map[string]struct{})
	for _, metric := range metrics {
		for _, key := range metric.labelKeys {
			allKeys[key] = struct{}{}
		}
	}
	result := make([]HistogramMetric, len(metrics))
	for i, metric := range metrics {
		if len(metric.labelKeys) != len(allKeys) {
			metricKeys := make(map[string]struct{})
			for _, key := range metric.labelKeys {
				metricKeys[key] = struct{}{}
			}
			for key := range allKeys {
				if _, ok := metricKeys[key]; !ok {
					metric.labelKeys = append(metric.labelKeys, key)
					metric.labelValues = append(metric.labelValues, "")
				}
			}
		}
		result[i] = metric
	}

	return result
}

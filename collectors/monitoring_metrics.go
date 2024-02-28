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

	"github.com/prometheus-community/stackdriver_exporter/hash"
	"github.com/prometheus-community/stackdriver_exporter/utils"
)

func buildFQName(timeSeries *monitoring.TimeSeries) string {
	// The metric name to report is composed by the 3 parts:
	// 1. namespace is a constant prefix (stackdriver)
	// 2. subsystem is the monitored resource type (ie gce_instance)
	// 3. name is the metric type (ie compute.googleapis.com/instance/cpu/usage_time)
	return prometheus.BuildFQName(namespace, utils.NormalizeMetricName(timeSeries.Resource.Type), utils.NormalizeMetricName(timeSeries.Metric.Type))
}

type timeSeriesMetrics struct {
	metricDescriptor *monitoring.MetricDescriptor

	ch chan<- prometheus.Metric

	fillMissingLabels bool
	constMetrics      map[string][]*ConstMetric
	histogramMetrics  map[string][]*HistogramMetric

	counterStore    DeltaCounterStore
	histogramStore  DeltaHistogramStore
	aggregateDeltas bool
}

func newTimeSeriesMetrics(descriptor *monitoring.MetricDescriptor,
	ch chan<- prometheus.Metric,
	fillMissingLabels bool,
	counterStore DeltaCounterStore,
	histogramStore DeltaHistogramStore,
	aggregateDeltas bool) (*timeSeriesMetrics, error) {

	return &timeSeriesMetrics{
		metricDescriptor:  descriptor,
		ch:                ch,
		fillMissingLabels: fillMissingLabels,
		constMetrics:      make(map[string][]*ConstMetric),
		histogramMetrics:  make(map[string][]*HistogramMetric),
		counterStore:      counterStore,
		histogramStore:    histogramStore,
		aggregateDeltas:   aggregateDeltas,
	}, nil
}

func (t *timeSeriesMetrics) newMetricDesc(fqName string, labelKeys []string) *prometheus.Desc {
	return prometheus.NewDesc(
		fqName,
		t.metricDescriptor.Description,
		labelKeys,
		prometheus.Labels{},
	)
}

type ConstMetric struct {
	FqName         string
	LabelKeys      []string
	ValueType      prometheus.ValueType
	Value          float64
	LabelValues    []string
	ReportTime     time.Time
	CollectionTime time.Time

	KeysHash uint64
}

type HistogramMetric struct {
	FqName         string
	LabelKeys      []string
	Sum            float64
	Count          uint64
	Buckets        map[float64]uint64
	LabelValues    []string
	ReportTime     time.Time
	CollectionTime time.Time

	KeysHash uint64
}

func (h *HistogramMetric) MergeHistogram(other *HistogramMetric) {
	// Increment totals based on incoming totals
	h.Sum += other.Sum
	h.Count += other.Count

	// Merge the buckets from existing in to current
	for key, value := range other.Buckets {
		h.Buckets[key] += value
	}
}

func (t *timeSeriesMetrics) CollectNewConstHistogram(timeSeries *monitoring.TimeSeries, reportTime time.Time, labelKeys []string, dist *monitoring.Distribution, buckets map[float64]uint64, labelValues []string, metricKind string) {
	fqName := buildFQName(timeSeries)
	histogramSum := dist.Mean * float64(dist.Count)
	var v HistogramMetric
	if t.fillMissingLabels || (metricKind == "DELTA" && t.aggregateDeltas) {
		v = HistogramMetric{
			FqName:         fqName,
			LabelKeys:      labelKeys,
			Sum:            histogramSum,
			Count:          uint64(dist.Count),
			Buckets:        buckets,
			LabelValues:    labelValues,
			ReportTime:     reportTime,
			CollectionTime: time.Now(),

			KeysHash: hashLabelKeys(labelKeys),
		}
	}

	if metricKind == "DELTA" && t.aggregateDeltas {
		t.histogramStore.Increment(t.metricDescriptor, &v)
		return
	}

	if t.fillMissingLabels {
		vs, ok := t.histogramMetrics[fqName]
		if !ok {
			vs = make([]*HistogramMetric, 0)
		}
		t.histogramMetrics[fqName] = append(vs, &v)
		return
	}

	t.ch <- t.newConstHistogram(fqName, reportTime, labelKeys, histogramSum, uint64(dist.Count), buckets, labelValues)
}

func (t *timeSeriesMetrics) newConstHistogram(fqName string, reportTime time.Time, labelKeys []string, sum float64, count uint64, buckets map[float64]uint64, labelValues []string) prometheus.Metric {
	return prometheus.NewMetricWithTimestamp(
		reportTime,
		prometheus.MustNewConstHistogram(
			t.newMetricDesc(fqName, labelKeys),
			count,
			sum,
			buckets,
			labelValues...,
		),
	)
}

func (t *timeSeriesMetrics) CollectNewConstMetric(timeSeries *monitoring.TimeSeries, reportTime time.Time, labelKeys []string, metricValueType prometheus.ValueType, metricValue float64, labelValues []string, metricKind string) {
	fqName := buildFQName(timeSeries)

	var v ConstMetric
	if t.fillMissingLabels || (metricKind == "DELTA" && t.aggregateDeltas) {
		v = ConstMetric{
			FqName:         fqName,
			LabelKeys:      labelKeys,
			ValueType:      metricValueType,
			Value:          metricValue,
			LabelValues:    labelValues,
			ReportTime:     reportTime,
			CollectionTime: time.Now(),

			KeysHash: hashLabelKeys(labelKeys),
		}
	}

	if metricKind == "DELTA" && t.aggregateDeltas {
		t.counterStore.Increment(t.metricDescriptor, &v)
		return
	}

	if t.fillMissingLabels {
		vs, ok := t.constMetrics[fqName]
		if !ok {
			vs = make([]*ConstMetric, 0)
		}
		t.constMetrics[fqName] = append(vs, &v)
		return
	}

	t.ch <- t.newConstMetric(fqName, reportTime, labelKeys, metricValueType, metricValue, labelValues)
}

func (t *timeSeriesMetrics) newConstMetric(fqName string, reportTime time.Time, labelKeys []string, metricValueType prometheus.ValueType, metricValue float64, labelValues []string) prometheus.Metric {
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
	dh := hash.New()
	sortedKeys := make([]string, len(labelKeys))
	copy(sortedKeys, labelKeys)
	sort.Strings(sortedKeys)
	for _, key := range sortedKeys {
		dh = hash.Add(dh, key)
		dh = hash.AddByte(dh, hash.SeparatorByte)
	}
	return dh
}

func (t *timeSeriesMetrics) Complete(reportingStartTime time.Time) {
	t.completeDeltaConstMetrics(reportingStartTime)
	t.completeDeltaHistogramMetrics(reportingStartTime)
	t.completeConstMetrics(t.constMetrics)
	t.completeHistogramMetrics(t.histogramMetrics)
}

func (t *timeSeriesMetrics) completeConstMetrics(constMetrics map[string][]*ConstMetric) {
	for _, vs := range constMetrics {
		if len(vs) > 1 {
			var needFill bool
			for i := 1; i < len(vs); i++ {
				if vs[0].KeysHash != vs[i].KeysHash {
					needFill = true
				}
			}
			if needFill {
				vs = fillConstMetricsLabels(vs)
			}
		}

		for _, v := range vs {
			t.ch <- t.newConstMetric(v.FqName, v.ReportTime, v.LabelKeys, v.ValueType, v.Value, v.LabelValues)
		}
	}
}

func (t *timeSeriesMetrics) completeHistogramMetrics(histograms map[string][]*HistogramMetric) {
	for _, vs := range histograms {
		if len(vs) > 1 {
			var needFill bool
			for i := 1; i < len(vs); i++ {
				if vs[0].KeysHash != vs[i].KeysHash {
					needFill = true
				}
			}
			if needFill {
				vs = fillHistogramMetricsLabels(vs)
			}
		}
		for _, v := range vs {
			t.ch <- t.newConstHistogram(v.FqName, v.ReportTime, v.LabelKeys, v.Sum, v.Count, v.Buckets, v.LabelValues)
		}
	}
}

func (t *timeSeriesMetrics) completeDeltaConstMetrics(reportingStartTime time.Time) {
	descriptorMetrics := t.counterStore.ListMetrics(t.metricDescriptor.Name)
	now := time.Now().Truncate(time.Minute)

	constMetrics := map[string][]*ConstMetric{}
	for _, collected := range descriptorMetrics {
		// If the metric wasn't collected we should still export it at the next sample time to avoid staleness
		if reportingStartTime.After(collected.CollectionTime) {
			// Ideally we could use monitoring.MetricDescriptorMetadata.SamplePeriod to determine how many
			// samples were missed to adjust this but monitoring.MetricDescriptorMetadata is viewed as optional
			// for a monitoring.MetricDescriptor
			reportingLag := collected.CollectionTime.Sub(collected.ReportTime).Truncate(time.Minute)
			collected.ReportTime = now.Add(-reportingLag)
		}
		if t.fillMissingLabels {
			if _, exists := constMetrics[collected.FqName]; !exists {
				constMetrics[collected.FqName] = []*ConstMetric{}
			}
			constMetrics[collected.FqName] = append(constMetrics[collected.FqName], collected)
		} else {
			t.ch <- t.newConstMetric(
				collected.FqName,
				collected.ReportTime,
				collected.LabelKeys,
				collected.ValueType,
				collected.Value,
				collected.LabelValues,
			)
		}
	}

	if t.fillMissingLabels {
		t.completeConstMetrics(constMetrics)
	}
}

func (t *timeSeriesMetrics) completeDeltaHistogramMetrics(reportingStartTime time.Time) {
	descriptorMetrics := t.histogramStore.ListMetrics(t.metricDescriptor.Name)
	now := time.Now().Truncate(time.Minute)

	histograms := map[string][]*HistogramMetric{}
	for _, collected := range descriptorMetrics {
		// If the histogram wasn't collected we should still export it at the next sample time to avoid staleness
		if reportingStartTime.After(collected.CollectionTime) {
			// Ideally we could use monitoring.MetricDescriptorMetadata.SamplePeriod to determine how many
			// samples were missed to adjust this but monitoring.MetricDescriptorMetadata is viewed as optional
			// for a monitoring.MetricDescriptor
			reportingLag := collected.CollectionTime.Sub(collected.ReportTime).Truncate(time.Minute)
			collected.ReportTime = now.Add(-reportingLag)
		}
		if t.fillMissingLabels {
			if _, exists := histograms[collected.FqName]; !exists {
				histograms[collected.FqName] = []*HistogramMetric{}
			}
			histograms[collected.FqName] = append(histograms[collected.FqName], collected)
		} else {
			t.ch <- t.newConstHistogram(
				collected.FqName,
				collected.ReportTime,
				collected.LabelKeys,
				collected.Sum,
				collected.Count,
				collected.Buckets,
				collected.LabelValues,
			)
		}
	}

	if t.fillMissingLabels {
		t.completeHistogramMetrics(histograms)
	}
}

func fillConstMetricsLabels(metrics []*ConstMetric) []*ConstMetric {
	allKeys := make(map[string]struct{})
	for _, metric := range metrics {
		for _, key := range metric.LabelKeys {
			allKeys[key] = struct{}{}
		}
	}

	for _, metric := range metrics {
		if len(metric.LabelKeys) != len(allKeys) {
			metricKeys := make(map[string]struct{})
			for _, key := range metric.LabelKeys {
				metricKeys[key] = struct{}{}
			}
			for key := range allKeys {
				if _, ok := metricKeys[key]; !ok {
					metric.LabelKeys = append(metric.LabelKeys, key)
					metric.LabelValues = append(metric.LabelValues, "")
				}
			}
		}
	}

	return metrics
}

func fillHistogramMetricsLabels(metrics []*HistogramMetric) []*HistogramMetric {
	allKeys := make(map[string]struct{})
	for _, metric := range metrics {
		for _, key := range metric.LabelKeys {
			allKeys[key] = struct{}{}
		}
	}

	for _, metric := range metrics {
		if len(metric.LabelKeys) != len(allKeys) {
			metricKeys := make(map[string]struct{})
			for _, key := range metric.LabelKeys {
				metricKeys[key] = struct{}{}
			}
			for key := range allKeys {
				if _, ok := metricKeys[key]; !ok {
					metric.LabelKeys = append(metric.LabelKeys, key)
					metric.LabelValues = append(metric.LabelValues, "")
				}
			}
		}
	}

	return metrics
}

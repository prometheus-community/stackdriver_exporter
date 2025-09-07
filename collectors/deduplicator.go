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
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus-community/stackdriver_exporter/hash"
	"github.com/prometheus/client_golang/prometheus"
)

// MetricDeduplicator helps prevent sending duplicate metrics to Prometheus.
// It tracks signatures of metrics that have already been sent.
type MetricDeduplicator struct {
	mu             sync.Mutex // Protects all fields below
	sentSignatures map[uint64]struct{}
	indicesSlice   []int // Reusable slice for sorting indices
	logger         *slog.Logger
	duplicateCount int64 // Count of duplicates detected

	// Prometheus metrics
	duplicatesTotal    prometheus.Counter
	checksTotal        prometheus.Counter
	uniqueMetricsGauge prometheus.Gauge
}

// NewMetricDeduplicator creates a new MetricDeduplicator.
func NewMetricDeduplicator(logger *slog.Logger) *MetricDeduplicator {
	duplicatesTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "stackdriver",
		Subsystem: "deduplicator",
		Name:      "duplicates_total",
		Help:      "Total number of duplicate metrics detected and dropped.",
	})

	checksTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "stackdriver",
		Subsystem: "deduplicator",
		Name:      "checks_total",
		Help:      "Total number of deduplication checks performed.",
	})

	uniqueMetricsGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "stackdriver",
		Subsystem: "deduplicator",
		Name:      "unique_metrics",
		Help:      "Current number of unique metrics being tracked.",
	})

	return &MetricDeduplicator{
		sentSignatures:     make(map[uint64]struct{}),
		indicesSlice:       make([]int, 0),
		logger:             logger.With("component", "deduplicator"),
		duplicatesTotal:    duplicatesTotal,
		checksTotal:        checksTotal,
		uniqueMetricsGauge: uniqueMetricsGauge,
	}
}

// CheckAndMark checks if a metric signature has been seen before.
// If not seen, it marks it as seen and returns false (not a duplicate).
// If seen before, it returns true (duplicate detected).
// This method is thread-safe.
func (d *MetricDeduplicator) CheckAndMark(fqName string, labelKeys, labelValues []string, ts time.Time) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.checksTotal.Inc()

	signature := d.hashLabelsTimestamp(fqName, labelKeys, labelValues, ts)

	if _, exists := d.sentSignatures[signature]; exists {
		d.duplicateCount++
		d.duplicatesTotal.Inc()

		// Log duplicate detection at debug level only
		d.logger.Debug("duplicate metric detected",
			"metric", fqName,
			"timestamp", ts.Format(time.RFC3339Nano),
			"signature", signature,
		)

		return true // Duplicate detected
	}

	d.sentSignatures[signature] = struct{}{} // Mark as seen
	d.uniqueMetricsGauge.Set(float64(len(d.sentSignatures)))

	return false // Not a duplicate
}

// ensureIndicesCapacity ensures the indices slice has enough capacity and returns it with the correct length.
// Must be called with mutex held.
func (d *MetricDeduplicator) ensureIndicesCapacity(length int) []int {
	if cap(d.indicesSlice) < length {
		d.indicesSlice = make([]int, length)
	} else {
		d.indicesSlice = d.indicesSlice[:length]
	}
	return d.indicesSlice
}

// hashLabelsTimestamp calculates a hash based on FQName, sorted labels, and timestamp.
func (d *MetricDeduplicator) hashLabelsTimestamp(fqName string, labelKeys, labelValues []string, ts time.Time) uint64 {
	// Initialize FNV-1a hash with fqName
	h := hash.New()
	h = hash.Add(h, fqName)
	h = hash.AddByte(h, hash.SeparatorByte)

	// Reuse indices slice for stable sorting
	indices := d.ensureIndicesCapacity(len(labelKeys))
	for i := range indices {
		indices[i] = i
	}

	// Sort indices by key using a simple insertion sort
	// This is faster for small slices than sort.Slice
	for i := 0; i < len(indices); i++ {
		for j := i + 1; j < len(indices); j++ {
			if labelKeys[indices[i]] > labelKeys[indices[j]] {
				indices[i], indices[j] = indices[j], indices[i]
			}
		}
	}

	// Add sorted key-value pairs to hash
	for _, idx := range indices {
		// Hash label key
		h = hash.Add(h, labelKeys[idx])
		h = hash.AddByte(h, hash.SeparatorByte)

		// Hash label value if it exists
		if idx < len(labelValues) {
			h = hash.Add(h, labelValues[idx])
		}
		h = hash.AddByte(h, hash.SeparatorByte)
	}

	// Add timestamp using binary operations
	tsNano := ts.UnixNano()
	h = hash.AddUint64(h, uint64(tsNano))

	// Mix in the high bits if they exist (for timestamps far in the future)
	if tsNano > 0xFFFFFFFF {
		h = hash.AddUint64(h, uint64(tsNano>>32))
	}

	return h
}

// GetStats returns deduplication statistics.
// Note: totalChecks is available via the checksTotal Prometheus metric.
func (d *MetricDeduplicator) GetStats() (duplicates int64, uniqueMetrics int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.duplicateCount, len(d.sentSignatures)
}

// Describe implements prometheus.Collector interface.
func (d *MetricDeduplicator) Describe(ch chan<- *prometheus.Desc) {
	d.duplicatesTotal.Describe(ch)
	d.checksTotal.Describe(ch)
	d.uniqueMetricsGauge.Describe(ch)
}

// Collect implements prometheus.Collector interface.
func (d *MetricDeduplicator) Collect(ch chan<- prometheus.Metric) {
	d.duplicatesTotal.Collect(ch)
	d.checksTotal.Collect(ch)
	d.uniqueMetricsGauge.Collect(ch)
}

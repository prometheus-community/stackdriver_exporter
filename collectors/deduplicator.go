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
	"sync"
	"time"

	"github.com/prometheus-community/stackdriver_exporter/hash"
)

// MetricDeduplicator helps prevent sending duplicate metrics to Prometheus.
// It tracks signatures of metrics that have already been sent.
type MetricDeduplicator struct {
	mu             sync.Mutex // Protects all fields below
	sentSignatures map[uint64]struct{}
	indicesSlice   []int // Reusable slice for sorting indices
}

// NewMetricDeduplicator creates a new MetricDeduplicator.
func NewMetricDeduplicator() *MetricDeduplicator {
	return &MetricDeduplicator{
		sentSignatures: make(map[uint64]struct{}),
		indicesSlice:   make([]int, 0),
	}
}

// CheckAndMark checks if a metric signature has been seen before.
// If not seen, it marks it as seen and returns false (not a duplicate).
// If seen before, it returns true (duplicate detected).
// This method is thread-safe.
func (d *MetricDeduplicator) CheckAndMark(fqName string, labelKeys, labelValues []string, ts time.Time) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	signature := d.hashLabelsTimestamp(fqName, labelKeys, labelValues, ts)

	if _, exists := d.sentSignatures[signature]; exists {
		return true // Duplicate detected
	}
	d.sentSignatures[signature] = struct{}{} // Mark as seen
	return false                             // Not a duplicate
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

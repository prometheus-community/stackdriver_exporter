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

package delta

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"google.golang.org/api/monitoring/v3"

	"github.com/prometheus-community/stackdriver_exporter/collectors"
	"github.com/prometheus-community/stackdriver_exporter/hash"
)

type HistogramEntry struct {
	Collected map[uint64]*collectors.HistogramMetric
	mutex     *sync.RWMutex
}

type InMemoryHistogramStore struct {
	store  *sync.Map
	ttl    time.Duration
	logger log.Logger
}

// NewInMemoryHistogramStore returns an implementation of HistogramStore which is persisted in-memory
func NewInMemoryHistogramStore(logger log.Logger, ttl time.Duration) *InMemoryHistogramStore {
	store := &InMemoryHistogramStore{
		store:  &sync.Map{},
		logger: logger,
		ttl:    ttl,
	}

	return store
}

func (s *InMemoryHistogramStore) Increment(metricDescriptor *monitoring.MetricDescriptor, currentValue *collectors.HistogramMetric) {
	if currentValue == nil {
		return
	}

	tmp, _ := s.store.LoadOrStore(metricDescriptor.Name, &HistogramEntry{
		Collected: map[uint64]*collectors.HistogramMetric{},
		mutex:     &sync.RWMutex{},
	})
	entry := tmp.(*HistogramEntry)

	key := toHistogramKey(currentValue)

	entry.mutex.Lock()
	defer entry.mutex.Unlock()
	existing := entry.Collected[key]

	if existing == nil {
		level.Debug(s.logger).Log("msg", "Tracking new histogram", "fqName", currentValue.FqName, "key", key, "incoming_time", currentValue.ReportTime)
		entry.Collected[key] = currentValue
		return
	}

	if existing.ReportTime.Before(currentValue.ReportTime) {
		level.Debug(s.logger).Log("msg", "Incrementing existing histogram", "fqName", currentValue.FqName, "key", key, "last_reported_time", existing.ReportTime, "incoming_time", currentValue.ReportTime)
		currentValue.MergeHistogram(existing)
		// Replace the existing histogram by the new one after merging it.
		entry.Collected[key] = currentValue
		return
	}

	level.Debug(s.logger).Log("msg", "Ignoring old sample for histogram", "fqName", currentValue.FqName, "key", key, "last_reported_time", existing.ReportTime, "incoming_time", currentValue.ReportTime)
}

func toHistogramKey(hist *collectors.HistogramMetric) uint64 {
	labels := make(map[string]string)
	keysCopy := append([]string{}, hist.LabelKeys...)
	for i := range hist.LabelKeys {
		labels[hist.LabelKeys[i]] = hist.LabelValues[i]
	}
	sort.Strings(keysCopy)

	var keyParts []string
	for _, k := range keysCopy {
		keyParts = append(keyParts, fmt.Sprintf("%s:%s", k, labels[k]))
	}
	hashText := fmt.Sprintf("%s|%s", hist.FqName, strings.Join(keyParts, "|"))
	h := hash.New()
	h = hash.Add(h, hashText)

	return h
}

func (s *InMemoryHistogramStore) ListMetrics(metricDescriptorName string) []*collectors.HistogramMetric {
	var output []*collectors.HistogramMetric
	now := time.Now()
	ttlWindowStart := now.Add(-s.ttl)

	tmp, exists := s.store.Load(metricDescriptorName)
	if !exists {
		return output
	}
	entry := tmp.(*HistogramEntry)

	entry.mutex.Lock()
	defer entry.mutex.Unlock()
	for key, collected := range entry.Collected {
		// Scan and remove metrics which are outside the TTL
		if ttlWindowStart.After(collected.CollectionTime) {
			level.Debug(s.logger).Log("msg", "Deleting histogram entry outside of TTL", "key", key, "fqName", collected.FqName)
			delete(entry.Collected, key)
			continue
		}

		copy := *collected
		output = append(output, &copy)
	}

	return output
}

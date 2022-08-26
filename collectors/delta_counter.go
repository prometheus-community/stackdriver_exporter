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
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"google.golang.org/api/monitoring/v3"
)

type CollectedMetric struct {
	metric          *ConstMetric
	lastCollectedAt time.Time
}

type DeltaCounterStore interface {
	Increment(metricDescriptor *monitoring.MetricDescriptor, currentValue *ConstMetric)
	ListMetricsByName(metricDescriptorName string) map[string][]*CollectedMetric
}

type metricEntry = map[uint64]*CollectedMetric

type inMemoryDeltaCounterStore struct {
	store      map[string]metricEntry
	ttl        time.Duration
	storeMutex *sync.RWMutex
	logger     log.Logger
}

func NewInMemoryDeltaCounterStore(logger log.Logger, ttl time.Duration) DeltaCounterStore {
	return inMemoryDeltaCounterStore{
		store:      map[string]metricEntry{},
		storeMutex: &sync.RWMutex{},
		logger:     logger,
		ttl:        ttl,
	}
}

func (s inMemoryDeltaCounterStore) Increment(metricDescriptor *monitoring.MetricDescriptor, currentValue *ConstMetric) {
	if currentValue == nil {
		return
	}

	var entry metricEntry
	s.storeMutex.Lock()
	if _, exists := s.store[metricDescriptor.Name]; !exists {
		s.store[metricDescriptor.Name] = metricEntry{}
	}
	entry = s.store[metricDescriptor.Name]
	s.storeMutex.Unlock()

	key := toCounterKey(currentValue)
	existing := entry[key]

	if existing == nil {
		level.Debug(s.logger).Log("msg", "Tracking new counter", "fqName", currentValue.fqName, "key", key, "current_value", currentValue.value, "incoming_time", currentValue.reportTime)
		entry[key] = &CollectedMetric{currentValue, time.Now()}
		return
	}

	if existing.metric.reportTime.Before(currentValue.reportTime) {
		level.Debug(s.logger).Log("msg", "Incrementing existing counter", "fqName", currentValue.fqName, "key", key, "current_value", existing.metric.value, "adding", currentValue.value, "last_reported_time", entry[key].metric.reportTime, "incoming_time", currentValue.reportTime)
		currentValue.value = currentValue.value + existing.metric.value
		existing.metric = currentValue
		existing.lastCollectedAt = time.Now()
		return
	}

	level.Debug(s.logger).Log("msg", "Ignoring old sample for counter", "fqName", currentValue.fqName, "key", key, "last_reported_time", existing.metric.reportTime, "incoming_time", currentValue.reportTime)
}

func toCounterKey(c *ConstMetric) uint64 {
	labels := make(map[string]string)
	keysCopy := append([]string{}, c.labelKeys...)
	for i := range c.labelKeys {
		labels[c.labelKeys[i]] = c.labelValues[i]
	}
	sort.Strings(keysCopy)

	var keyParts []string
	for _, k := range keysCopy {
		keyParts = append(keyParts, fmt.Sprintf("%s:%s", k, labels[k]))
	}
	hashText := fmt.Sprintf("%s|%s", c.fqName, strings.Join(keyParts, "|"))
	h := hashNew()
	h = hashAdd(h, hashText)

	return h
}

func (s inMemoryDeltaCounterStore) ListMetricsByName(metricDescriptorName string) map[string][]*CollectedMetric {
	output := map[string][]*CollectedMetric{}
	now := time.Now()
	ttlWindowStart := now.Add(-s.ttl)

	s.storeMutex.Lock()
	metric := s.store[metricDescriptorName]
	if metric == nil {
		s.storeMutex.Unlock()
		return output
	}
	s.storeMutex.Unlock()

	for key, collected := range metric {
		//Scan and remove metrics which are outside the TTL
		if ttlWindowStart.After(collected.lastCollectedAt) {
			level.Debug(s.logger).Log("msg", "Deleting counter entry outside of TTL", "key", key, "fqName", collected.metric.fqName)
			delete(metric, key)
			continue
		}

		metrics, exists := output[collected.metric.fqName]
		if !exists {
			metrics = make([]*CollectedMetric, 0)
		}
		metricCopy := *collected.metric
		outputEntry := CollectedMetric{
			metric:          &metricCopy,
			lastCollectedAt: collected.lastCollectedAt,
		}
		output[collected.metric.fqName] = append(metrics, &outputEntry)
	}

	return output
}

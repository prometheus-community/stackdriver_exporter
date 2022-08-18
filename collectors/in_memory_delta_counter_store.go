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

type metricEntry struct {
	collectedMetrics map[uint64]*CollectedMetric
	lastListedAt     time.Time
	description      string
}

type inMemoryDeltaCounterStore struct {
	store      map[string]*metricEntry
	ttl        time.Duration
	storeMutex *sync.RWMutex
	logger     log.Logger
}

func NewInMemoryDeltaCounterStore(logger log.Logger, ttl time.Duration) DeltaCounterStore {
	return inMemoryDeltaCounterStore{
		store:      map[string]*metricEntry{},
		storeMutex: &sync.RWMutex{},
		logger:     logger,
		ttl:        ttl,
	}
}

func (s inMemoryDeltaCounterStore) Increment(metricDescriptor *monitoring.MetricDescriptor, currentValue *ConstMetric) {
	if currentValue == nil {
		return
	}
	key := toKey(currentValue)

	var metric *metricEntry
	s.storeMutex.Lock()
	if _, exists := s.store[metricDescriptor.Name]; !exists {
		s.store[metricDescriptor.Name] = &metricEntry{
			collectedMetrics: map[uint64]*CollectedMetric{},
			lastListedAt:     time.Time{},
		}
	}
	metric = s.store[metricDescriptor.Name]
	s.storeMutex.Unlock()

	if _, exists := metric.collectedMetrics[key]; !exists {
		level.Debug(s.logger).Log("msg", "Tracking new value", "fqName", currentValue.fqName, "key", key, "current_value", currentValue.value, "incoming_time", currentValue.reportTime)
		metric.collectedMetrics[key] = &CollectedMetric{currentValue, time.Now()}
	} else if metric.collectedMetrics[key].metric.reportTime.Before(currentValue.reportTime) {
		level.Debug(s.logger).Log("msg", "Incrementing existing value", "fqName", currentValue.fqName, "key", key, "current_value", metric.collectedMetrics[key].metric.value, "adding", currentValue.value, "last_reported_time", metric.collectedMetrics[key].metric.reportTime, "incoming_time", currentValue.reportTime)
		currentValue.value = currentValue.value + metric.collectedMetrics[key].metric.value
		metric.collectedMetrics[key].metric = currentValue
		metric.collectedMetrics[key].lastCollectedAt = time.Now()
	} else {
		level.Debug(s.logger).Log("msg", "Ignoring repeat sample", "fqName", currentValue.fqName, "key", key, "last_reported_time", metric.collectedMetrics[key].metric.reportTime, "incoming_time", currentValue.reportTime)
	}
}

func toKey(c *ConstMetric) uint64 {
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
	metric.lastListedAt = now
	s.storeMutex.Unlock()

	for key, collectedMetric := range metric.collectedMetrics {
		//Scan and remove metrics which are outside the TTL
		if ttlWindowStart.After(collectedMetric.lastCollectedAt) {
			delete(metric.collectedMetrics, key)
			continue
		}

		metrics, exists := output[collectedMetric.metric.fqName]
		if !exists {
			metrics = make([]*CollectedMetric, 0)
		}
		thing := *collectedMetric.metric
		outputCopy := CollectedMetric{
			metric:          &thing,
			lastCollectedAt: collectedMetric.lastCollectedAt,
		}
		output[collectedMetric.metric.fqName] = append(metrics, &outputCopy)
	}

	return output
}

func (s inMemoryDeltaCounterStore) ListMetricDescriptorsNotCollected(since time.Time) []MetricDescriptor {
	level.Debug(s.logger).Log("msg", "Listing metrics not collected", "since", since)
	var names []MetricDescriptor
	ttlWindowStart := time.Now().Add(-s.ttl)

	s.storeMutex.Lock()
	defer s.storeMutex.Unlock()

	for name, metrics := range s.store {
		//Scan and remove metrics which are outside the TTL
		for key, collectedMetric := range metrics.collectedMetrics {
			if ttlWindowStart.After(collectedMetric.lastCollectedAt) {
				delete(metrics.collectedMetrics, key)
			}
		}

		if len(metrics.collectedMetrics) == 0 {
			delete(s.store, name)
			continue
		}

		if since.After(metrics.lastListedAt) {
			names = append(names, MetricDescriptor{name: name, description: metrics.description})
		}
	}

	return names
}

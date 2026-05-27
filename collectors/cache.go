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
	"sync"
	"time"

	"google.golang.org/api/monitoring/v3"
)

type DescriptorCache interface {
	// Lookup searches the cache for an entry. If the cache has no entry or the entry has expired nil is returned.
	Lookup(prefix string) []*monitoring.MetricDescriptor

	// Store stores an entry in the cache
	Store(prefix string, data []*monitoring.MetricDescriptor)
}

type noopDescriptorCache struct{}

func (d *noopDescriptorCache) Lookup(prefix string) []*monitoring.MetricDescriptor {
	return nil
}

func (d *noopDescriptorCache) Store(prefix string, data []*monitoring.MetricDescriptor) {}

// descriptorCache is a MetricTypePrefix -> MetricDescriptor cache
type descriptorCache struct {
	cache map[string]*descriptorCacheEntry
	lock  sync.Mutex
	ttl   time.Duration
}

type descriptorCacheEntry struct {
	data   []*monitoring.MetricDescriptor
	expiry time.Time
}

func newDescriptorCache(ttl time.Duration) *descriptorCache {
	return &descriptorCache{ttl: ttl, cache: make(map[string]*descriptorCacheEntry)}
}

// Lookup returns a list of MetricDescriptors if the prefix is found, nil if not found or expired
func (d *descriptorCache) Lookup(prefix string) []*monitoring.MetricDescriptor {
	d.lock.Lock()
	defer d.lock.Unlock()

	v, ok := d.cache[prefix]
	if !ok || time.Now().After(v.expiry) {
		return nil
	}

	return v.data
}

// Store overrides a cache entry
func (d *descriptorCache) Store(prefix string, data []*monitoring.MetricDescriptor) {
	entry := descriptorCacheEntry{data: data, expiry: time.Now().Add(d.ttl)}
	d.lock.Lock()
	defer d.lock.Unlock()
	d.cache[prefix] = &entry
}

// collectorCache caches MonitoringCollectors keyed by (project, prefix-filter)
// for the duration of the configured TTL. It exists so HTTP scrape paths that
// rebuild per request can preserve delta-counter state across calls.
type collectorCache struct {
	cache map[string]*collectorCacheEntry
	lock  sync.RWMutex
	ttl   time.Duration
}

type collectorCacheEntry struct {
	collector *MonitoringCollector
	expiry    time.Time
}

func newCollectorCache(ttl time.Duration) *collectorCache {
	c := &collectorCache{
		cache: make(map[string]*collectorCacheEntry),
		ttl:   ttl,
	}

	go c.cleanup()
	return c
}

// Get returns the cached collector for key, refreshing its TTL on hit.
func (c *collectorCache) Get(key string) (*MonitoringCollector, bool) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	entry, ok := c.cache[key]

	if !ok {
		return nil, false
	}

	if time.Now().After(entry.expiry) {
		delete(c.cache, key)
		return nil, false
	}

	entry.expiry = time.Now().Add(c.ttl)
	return entry.collector, true
}

func (c *collectorCache) Store(key string, collector *MonitoringCollector) {
	entry := &collectorCacheEntry{
		collector: collector,
		expiry:    time.Now().Add(c.ttl),
	}

	c.lock.Lock()
	defer c.lock.Unlock()
	c.cache[key] = entry
}

func (c *collectorCache) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		c.removeExpired()
	}
}

func (c *collectorCache) removeExpired() {
	c.lock.Lock()
	defer c.lock.Unlock()

	now := time.Now()
	for key, entry := range c.cache {
		if now.After(entry.expiry) {
			delete(c.cache, key)
		}
	}
}

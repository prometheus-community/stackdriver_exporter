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

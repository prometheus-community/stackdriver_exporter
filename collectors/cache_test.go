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
	"fmt"
	"testing"
	"time"

	"google.golang.org/api/monitoring/v3"
)

func makeDummyMetrics(n int) []*monitoring.MetricDescriptor {
	ret := make([]*monitoring.MetricDescriptor, n)
	for i := 0; i < n; i++ {
		ret[i] = &monitoring.MetricDescriptor{
			DisplayName: fmt.Sprintf("test-%d", i),
		}
	}
	return ret
}

func isEqual(a, b []*monitoring.MetricDescriptor) bool {
	if len(a) != len(b) {
		return false
	}

	for idx, e := range a {
		if e.DisplayName != b[idx].DisplayName {
			return false
		}
	}

	return true
}

func TestDescriptorCache(t *testing.T) {
	ttl := 1 * time.Second
	cache := newDescriptorCache(ttl)
	entries := makeDummyMetrics(10)
	key := "akey"

	if cache.Lookup(key) != nil {
		t.Errorf("Cache should've returned nil on lookup without store")
	}

	cache.Store("more", makeDummyMetrics(10))
	cache.Store("evenmore", makeDummyMetrics(10))

	cache.Store(key, entries)
	newEntries := cache.Lookup(key)

	if newEntries == nil {
		t.Errorf("Cache returned unexpected nil")
	}

	if !isEqual(entries, newEntries) {
		t.Errorf("Cache modified entries")
	}

	time.Sleep(ttl)
	if cache.Lookup(key) != nil {
		t.Error("cache entries should have expired")
	}
}

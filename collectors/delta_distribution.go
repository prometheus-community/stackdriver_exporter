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

	"google.golang.org/api/monitoring/v3"
)

type DistributionCacheEntry struct {
	buckets *[]int64
	mean    float64
	count   int64
	ts      time.Time
}

var (
	distributionCache = make(map[uint64]*DistributionCacheEntry)
	distributionMutex = &sync.RWMutex{}
)

// GetDistributionEntry retrieves the previously stored CacheEntry for a metric.  If
// it doesn't exist, it makes a new empty one and returns that.
func GetDistributionEntry(key uint64) *DistributionCacheEntry {
	distributionMutex.Lock()
	defer distributionMutex.Unlock()

	if entry, ok := distributionCache[key]; ok {
		return entry
	}
	newEntry := DistributionCacheEntry{buckets: &[]int64{}, mean: 0.0, count: 0, ts: time.Time{}}
	distributionCache[key] = &newEntry
	return &newEntry
}

// SetDistributionEntry sets the current cached value for a distribution and returns true
// if it is a new measurement, otherwise it does nothing and returns false.
func SetDistributionEntry(key uint64, dist *monitoring.Distribution, ts time.Time) bool {
	distributionMutex.Lock()
	defer distributionMutex.Unlock()

	if entry, ok := distributionCache[key]; ok {
		if ts.After(entry.ts) {
			var new_buckets []int64

			// Monitoring can return less buckets than currently exists, since
			// it will only go as far as it needs to indicate a unique count.  So
			// we need to use the largest one as our "starter" then add the other.
			if len(*entry.buckets) >= len(dist.BucketCounts) {
				// Copy entry.buckets and then add in buckets
				new_buckets = make([]int64, len(*entry.buckets))
				copy(new_buckets, *entry.buckets)
				for i := range dist.BucketCounts {
					new_buckets[i] += dist.BucketCounts[i]
				}
			} else {
				// Copy counts and then add in entry.counts
				new_buckets = make([]int64, len(dist.BucketCounts))
				copy(new_buckets, dist.BucketCounts)
				for i := range *entry.buckets {
					new_buckets[i] += (*entry.buckets)[i]
				}
			}
			// Calculate a new mean and overall count
			mean := dist.Mean
			mean += entry.mean
			mean /= 2

			var count int64
			for _, v := range new_buckets {
				count += v
			}

			newEntry := DistributionCacheEntry{buckets: &new_buckets, mean: mean, count: count, ts: ts}
			distributionCache[key] = &newEntry
			return true
		}
	}

	return false
}

// CacheEntryToDistribution will take a CacheEntry and a monitoring.Distribution observation and will
// replace the elements we are caching into the distribution.  It returns the distribution
func CacheEntryToDistribution(entry DistributionCacheEntry, dist *monitoring.Distribution) *monitoring.Distribution {
	if len(*entry.buckets) > 0 {
		copy(dist.BucketCounts, *entry.buckets)
		dist.Mean = entry.mean
		dist.Count = entry.count
	}
	return dist
}

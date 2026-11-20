// Copyright The Prometheus Authors
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
	"sync/atomic"
	"time"
)

// metricHelpTTL is how long a help text is kept after its metric name was
// last reported.
const metricHelpTTL = time.Hour

// MetricHelpStore holds the help text served for each metric name. Collectors
// gathered into the same exposition must share one store: projects can
// describe the same metric type differently (custom metrics in particular),
// while an exposition needs a single help text per metric family.
//
// A MetricHelpStore is safe for concurrent use, and its zero value is ready
// to use.
type MetricHelpStore struct {
	mu        sync.RWMutex
	entries   map[string]*metricHelpEntry
	lastSweep time.Time
	now       func() time.Time
}

type metricHelpEntry struct {
	help     string
	lastUsed atomic.Int64
}

// NewMetricHelpStore returns an empty MetricHelpStore.
func NewMetricHelpStore() *MetricHelpStore {
	return &MetricHelpStore{}
}

// Help returns the help text for fqName. The description passed on the first
// call for a name is kept and returned by every later call for that name,
// until the name has not been asked for during metricHelpTTL.
func (s *MetricHelpStore) Help(fqName, description string) string {
	now := time.Now()
	if s.now != nil {
		now = s.now()
	}

	// lastUsed is updated under the read lock, so that sweep, which holds the
	// write lock, never drops an entry a caller is about to return.
	s.mu.RLock()
	entry, ok := s.entries[fqName]
	if ok {
		entry.lastUsed.Store(now.UnixNano())
	}
	s.mu.RUnlock()
	if ok {
		return entry.help
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.entries[fqName]; ok {
		entry.lastUsed.Store(now.UnixNano())
		return entry.help
	}
	s.sweep(now)
	if s.entries == nil {
		s.entries = make(map[string]*metricHelpEntry)
	}
	entry = &metricHelpEntry{help: description}
	entry.lastUsed.Store(now.UnixNano())
	s.entries[fqName] = entry
	return description
}

// sweep drops the entries unused for metricHelpTTL, at most once per
// metricHelpTTL. The caller must hold the write lock.
func (s *MetricHelpStore) sweep(now time.Time) {
	if now.Sub(s.lastSweep) < metricHelpTTL {
		return
	}
	s.lastSweep = now
	deadline := now.Add(-metricHelpTTL).UnixNano()
	for fqName, entry := range s.entries {
		if entry.lastUsed.Load() < deadline {
			delete(s.entries, fqName)
		}
	}
}

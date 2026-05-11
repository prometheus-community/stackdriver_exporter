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
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/monitoring/v3"

	"github.com/prometheus-community/stackdriver_exporter/config"
)

// CounterStoreFactory creates a DeltaCounterStore for a given TTL.
type CounterStoreFactory func(logger *slog.Logger, ttl time.Duration) DeltaCounterStore

// HistogramStoreFactory creates a DeltaHistogramStore for a given TTL.
type HistogramStoreFactory func(logger *slog.Logger, ttl time.Duration) DeltaHistogramStore

// Runtime holds the resolved state produced by NewRuntime.
type Runtime struct {
	cfg                   *config.Config
	projectIDs            []string
	service               *monitoring.Service
	logger                *slog.Logger
	counterStoreFactory   CounterStoreFactory
	histogramStoreFactory HistogramStoreFactory
	cache                 *collectorCache
}

// NewRuntime validates the config, resolves project IDs, and creates the
// monitoring service. counterFactory and histogramFactory are invoked each
// time a new collector is built. The returned Runtime does not cache
// collectors; call WithCache to derive a sibling that does.
func NewRuntime(ctx context.Context, logger *slog.Logger, cfg *config.Config, counterFactory CounterStoreFactory, histogramFactory HistogramStoreFactory) (*Runtime, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	var projectIDs []string

	if cfg.ProjectsFilter != "" {
		ids, err := getProjectIDsFromFilter(ctx, cfg.ProjectsFilter)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve project IDs from projects_filter: %w", err)
		}
		projectIDs = append(projectIDs, ids...)
	}

	projectIDs = append(projectIDs, cfg.ProjectIDs...)

	if len(projectIDs) == 0 {
		id, err := discoverDefaultProjectID(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to discover default GCP project: %w", err)
		}
		projectIDs = append(projectIDs, id)
	}

	projectIDs = deduplicateProjectIDs(projectIDs)

	service, err := createMonitoringService(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &Runtime{
		cfg:                   cfg,
		projectIDs:            projectIDs,
		service:               service,
		logger:                logger,
		counterStoreFactory:   counterFactory,
		histogramStoreFactory: histogramFactory,
	}, nil
}

// WithCache returns a Runtime configured to cache its collectors per
// (project, prefix-filter). Subsequent calls to Collectors or
// CollectorsForPrefixes reuse cached entries until they expire, which lets
// delta-counter state survive repeated rebuilds. The TTL is derived from
// AggregateDeltasTTL and DescriptorCacheTTL.
//
// HTTP scrape paths that rebuild collectors per request (?collect= filtering)
// want this; embedded callers that hold a long-lived registry do not.
//
// Each call allocates a fresh cache (and its cleanup goroutine); call it
// once per consumer and discard the receiver.
func (r *Runtime) WithCache() *Runtime {
	sibling := *r
	sibling.cache = newCollectorCache(collectorCacheTTL(r.cfg))
	return &sibling
}

// Collectors builds one MonitoringCollector per resolved project scoped to all
// configured prefixes.
func (r *Runtime) Collectors() ([]*MonitoringCollector, error) {
	return r.buildCollectors(nil)
}

// CollectorsForPrefixes builds one MonitoringCollector per resolved project
// restricted to the given metric type prefixes. A nil or empty prefixFilter
// is equivalent to Collectors.
func (r *Runtime) CollectorsForPrefixes(prefixFilter []string) ([]*MonitoringCollector, error) {
	return r.buildCollectors(prefixFilter)
}

func (r *Runtime) buildCollectors(prefixFilter []string) ([]*MonitoringCollector, error) {
	result := make([]*MonitoringCollector, 0, len(r.projectIDs))
	for _, projectID := range r.projectIDs {
		c, err := r.collectorFor(projectID, prefixFilter)
		if err != nil {
			return nil, fmt.Errorf("collector for %q: %w", projectID, err)
		}
		result = append(result, c)
	}
	return result, nil
}

func (r *Runtime) collectorFor(projectID string, prefixFilter []string) (*MonitoringCollector, error) {
	if r.cache == nil {
		return r.newCollector(projectID, prefixFilter)
	}
	key := collectorCacheKey(projectID, prefixFilter)
	if c, ok := r.cache.Get(key); ok {
		return c, nil
	}
	c, err := r.newCollector(projectID, prefixFilter)
	if err != nil {
		return nil, err
	}
	r.cache.Store(key, c)
	return c, nil
}

func (r *Runtime) newCollector(projectID string, prefixFilter []string) (*MonitoringCollector, error) {
	filtered := r.filterMetricTypePrefixes(prefixFilter)
	return NewMonitoringCollector(
		projectID,
		r.service,
		monitoringCollectorOptionsForPrefixes(r.cfg, filtered),
		r.logger,
		r.counterStoreFactory(r.logger, r.cfg.AggregateDeltasTTL),
		r.histogramStoreFactory(r.logger, r.cfg.AggregateDeltasTTL),
	)
}

// filterMetricTypePrefixes resolves a request-time prefix filter against the
// configured prefixes. nil/empty means "use everything configured"; otherwise
// only the request prefixes whose configured parent matches are kept.
func (r *Runtime) filterMetricTypePrefixes(prefixFilter []string) []string {
	if len(prefixFilter) == 0 {
		return parseMetricTypePrefixes(r.cfg.MetricsPrefixes)
	}
	var filtered []string
	for _, prefix := range r.cfg.MetricsPrefixes {
		for _, f := range prefixFilter {
			if strings.HasPrefix(f, prefix) {
				filtered = append(filtered, f)
			}
		}
	}
	return parseMetricTypePrefixes(filtered)
}

// collectorCacheKey builds a deterministic cache key for a (project, prefix)
// pair. Prefixes are sorted defensively so callers that pass the same set in
// a different order share a cache entry.
func collectorCacheKey(projectID string, prefixFilter []string) string {
	sorted := slices.Clone(prefixFilter)
	slices.Sort(sorted)
	return fmt.Sprintf("%s-%v", projectID, sorted)
}

func collectorCacheTTL(cfg *config.Config) time.Duration {
	if cfg.AggregateDeltas || cfg.DescriptorCacheTTL > 0 {
		return max(cfg.AggregateDeltasTTL, cfg.DescriptorCacheTTL)
	}
	return 2 * time.Hour
}

func deduplicateProjectIDs(projectIDs []string) []string {
	normalized := slices.Clone(projectIDs)
	slices.Sort(normalized)
	return slices.Compact(normalized)
}

func discoverDefaultProjectID(ctx context.Context) (string, error) {
	credentials, err := google.FindDefaultCredentials(ctx, compute.ComputeScope)
	if err != nil {
		return "", err
	}
	if credentials.ProjectID == "" {
		return "", fmt.Errorf("unable to identify default GCP project")
	}
	return credentials.ProjectID, nil
}

// getProjectIDsFromFilter returns the list of project IDs that match a Google
// Cloud organization-scoped projects filter.
func getProjectIDsFromFilter(ctx context.Context, filter string) ([]string, error) {
	service, err := cloudresourcemanager.NewService(ctx)
	if err != nil {
		return nil, err
	}

	var projectIDs []string
	err = service.Projects.List().Filter(filter).Pages(ctx, func(page *cloudresourcemanager.ListProjectsResponse) error {
		for _, project := range page.Projects {
			projectIDs = append(projectIDs, project.ProjectId)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return projectIDs, nil
}

// parseMetricTypePrefixes sorts prefixes, removes duplicates, and skips prefixes
// already covered by a broader parent prefix.
func parseMetricTypePrefixes(inputPrefixes []string) []string {
	sorted := slices.Clone(inputPrefixes)
	slices.Sort(sorted)
	unique := slices.Compact(sorted)
	out := make([]string, 0, len(unique))

	for _, prefix := range unique {
		if len(out) > 0 && strings.HasPrefix(prefix, out[len(out)-1]) {
			continue
		}
		out = append(out, prefix)
	}
	return out
}

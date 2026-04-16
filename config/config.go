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

package config

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/PuerkitoBio/rehttp"
	"github.com/prometheus-community/stackdriver_exporter/collectors"
	"github.com/prometheus-community/stackdriver_exporter/utils"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/monitoring/v3"
	"google.golang.org/api/option"
)

type Option struct {
	CLIFlag string
	OTelKey string
	Default any
}

const (
	DefaultUniverseDomain       = "googleapis.com"
	DefaultMaxRetries           = 0
	DefaultHTTPTimeout          = "10s"
	DefaultMaxBackoff           = "5s"
	DefaultBackoffJitter        = "1s"
	DefaultMetricsInterval      = "5m"
	DefaultMetricsOffset        = "0s"
	DefaultMetricsIngest        = false
	DefaultFillMissing          = true
	DefaultDropDelegated        = false
	DefaultAggregateDeltas      = false
	DefaultDeltasTTL            = "30m"
	DefaultDescriptorTTL        = "0s"
	DefaultDescriptorGoogleOnly = true
)

// DefaultRetryStatuses must be treated as immutable after declaration.
var DefaultRetryStatuses = []int{http.StatusServiceUnavailable}

var (
	ProjectIDs           = Option{CLIFlag: "google.project-ids", OTelKey: "project_ids"}
	ProjectsFilter       = Option{CLIFlag: "google.projects.filter", OTelKey: "projects_filter"}
	UniverseDomain       = Option{CLIFlag: "google.universe-domain", OTelKey: "universe_domain", Default: DefaultUniverseDomain}
	MaxRetries           = Option{CLIFlag: "stackdriver.max-retries", OTelKey: "max_retries", Default: DefaultMaxRetries}
	HTTPTimeout          = Option{CLIFlag: "stackdriver.http-timeout", OTelKey: "http_timeout", Default: DefaultHTTPTimeout}
	MaxBackoff           = Option{CLIFlag: "stackdriver.max-backoff", OTelKey: "max_backoff", Default: DefaultMaxBackoff}
	BackoffJitter        = Option{CLIFlag: "stackdriver.backoff-jitter", OTelKey: "backoff_jitter", Default: DefaultBackoffJitter}
	RetryStatuses        = Option{CLIFlag: "stackdriver.retry-statuses", OTelKey: "retry_statuses", Default: DefaultRetryStatuses}
	MetricsPrefixes      = Option{CLIFlag: "monitoring.metrics-prefixes", OTelKey: "metrics_prefixes"}
	MetricsInterval      = Option{CLIFlag: "monitoring.metrics-interval", OTelKey: "metrics_interval", Default: DefaultMetricsInterval}
	MetricsOffset        = Option{CLIFlag: "monitoring.metrics-offset", OTelKey: "metrics_offset", Default: DefaultMetricsOffset}
	MetricsIngest        = Option{CLIFlag: "monitoring.metrics-ingest-delay", OTelKey: "metrics_ingest_delay", Default: DefaultMetricsIngest}
	FillMissing          = Option{CLIFlag: "collector.fill-missing-labels", OTelKey: "fill_missing_labels", Default: DefaultFillMissing}
	DropDelegated        = Option{CLIFlag: "monitoring.drop-delegated-projects", OTelKey: "drop_delegated_projects", Default: DefaultDropDelegated}
	Filters              = Option{CLIFlag: "monitoring.filters", OTelKey: "filters"}
	AggregateDeltas      = Option{CLIFlag: "monitoring.aggregate-deltas", OTelKey: "aggregate_deltas", Default: DefaultAggregateDeltas}
	DeltasTTL            = Option{CLIFlag: "monitoring.aggregate-deltas-ttl", OTelKey: "aggregate_deltas_ttl", Default: DefaultDeltasTTL}
	DescriptorTTL        = Option{CLIFlag: "monitoring.descriptor-cache-ttl", OTelKey: "descriptor_cache_ttl", Default: DefaultDescriptorTTL}
	DescriptorGoogleOnly = Option{CLIFlag: "monitoring.descriptor-cache-only-google", OTelKey: "descriptor_cache_only_google", Default: DefaultDescriptorGoogleOnly}

	AllOptions = []Option{
		ProjectIDs,
		ProjectsFilter,
		UniverseDomain,
		MaxRetries,
		HTTPTimeout,
		MaxBackoff,
		BackoffJitter,
		RetryStatuses,
		MetricsPrefixes,
		MetricsInterval,
		MetricsOffset,
		MetricsIngest,
		FillMissing,
		DropDelegated,
		Filters,
		AggregateDeltas,
		DeltasTTL,
		DescriptorTTL,
		DescriptorGoogleOnly,
	}
)

type RuntimeConfig struct {
	ProjectIDs           []string
	ProjectsFilter       string
	UniverseDomain       string
	MaxRetries           int
	HTTPTimeout          time.Duration
	MaxBackoff           time.Duration
	BackoffJitter        time.Duration
	RetryStatuses        []int
	MetricsPrefixes      []string
	MetricsInterval      time.Duration
	MetricsOffset        time.Duration
	MetricsIngest        bool
	FillMissing          bool
	DropDelegated        bool
	Filters              []string
	AggregateDeltas      bool
	DeltasTTL            time.Duration
	DescriptorTTL        time.Duration
	DescriptorGoogleOnly bool
}

func OTelComponentDefaults() map[string]interface{} {
	defaults := make(map[string]interface{}, len(AllOptions))
	for _, option := range AllOptions {
		if option.Default == nil {
			continue
		}
		// Option defaults are shared values and must not be mutated by callers.
		defaults[option.OTelKey] = option.Default
	}
	return defaults
}

func ParseDuration(name, raw string) (time.Duration, error) {
	duration, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid duration %q: %w", name, raw, err)
	}
	return duration, nil
}

func ValidateRetryStatuses(codes []int) error {
	for _, code := range codes {
		if code < http.StatusContinue || code > 599 {
			return fmt.Errorf("retry status %d is not a valid HTTP status code", code)
		}
	}
	return nil
}

func ParseMetricPrefixes(prefixes []string) []string {
	return utils.ParseMetricTypePrefixes(prefixes)
}

func ParseMetricFilters(filters []string) []collectors.MetricFilter {
	return collectors.ParseMetricExtraFilters(filters)
}

func DeduplicateProjectIDs(projectIDs []string) []string {
	normalized := slices.Clone(projectIDs)
	slices.Sort(normalized)
	return slices.Compact(normalized)
}

func (c RuntimeConfig) MonitoringCollectorOptions() collectors.MonitoringCollectorOptions {
	return c.MonitoringCollectorOptionsForPrefixes(ParseMetricPrefixes(c.MetricsPrefixes))
}

func (c RuntimeConfig) MonitoringCollectorOptionsForPrefixes(metricPrefixes []string) collectors.MonitoringCollectorOptions {
	return collectors.MonitoringCollectorOptions{
		MetricTypePrefixes:        metricPrefixes,
		ExtraFilters:              ParseMetricFilters(c.Filters),
		RequestInterval:           c.MetricsInterval,
		RequestOffset:             c.MetricsOffset,
		IngestDelay:               c.MetricsIngest,
		FillMissingLabels:         c.FillMissing,
		DropDelegatedProjects:     c.DropDelegated,
		AggregateDeltas:           c.AggregateDeltas,
		DescriptorCacheTTL:        c.DescriptorTTL,
		DescriptorCacheOnlyGoogle: c.DescriptorGoogleOnly,
	}
}

func (c RuntimeConfig) CollectorCacheTTL() time.Duration {
	if c.AggregateDeltas || c.DescriptorTTL > 0 {
		ttl := c.DeltasTTL
		if c.DescriptorTTL > ttl {
			ttl = c.DescriptorTTL
		}
		return ttl
	}

	return 2 * time.Hour
}

func DiscoverDefaultProjectID(ctx context.Context) (string, error) {
	credentials, err := google.FindDefaultCredentials(ctx, compute.ComputeScope)
	if err != nil {
		return "", err
	}
	if credentials.ProjectID == "" {
		return "", fmt.Errorf("unable to identify default GCP project")
	}
	return credentials.ProjectID, nil
}

func (c RuntimeConfig) CreateMonitoringService(ctx context.Context) (*monitoring.Service, error) {
	googleClient, err := google.DefaultClient(ctx, monitoring.MonitoringReadScope)
	if err != nil {
		return nil, fmt.Errorf("error creating Google client: %w", err)
	}

	googleClient.Timeout = c.HTTPTimeout
	googleClient.Transport = rehttp.NewTransport(
		googleClient.Transport,
		rehttp.RetryAll(
			rehttp.RetryMaxRetries(c.MaxRetries),
			rehttp.RetryStatuses(c.RetryStatuses...),
		),
		rehttp.ExpJitterDelay(c.BackoffJitter, c.MaxBackoff),
	)

	service, err := monitoring.NewService(ctx, option.WithHTTPClient(googleClient), option.WithUniverseDomain(c.UniverseDomain))
	if err != nil {
		return nil, fmt.Errorf("error creating Google Stackdriver Monitoring service: %w", err)
	}
	return service, nil
}

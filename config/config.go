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

type SharedOption struct {
	CLIFlag string
	OTelKey string
	Default []any
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

var DefaultRetryStatuses = []int{http.StatusServiceUnavailable}

var sharedOptions = map[string]SharedOption{
	"ProjectIDs":           {CLIFlag: "google.project-ids", OTelKey: "project_ids"},
	"ProjectsFilter":       {CLIFlag: "google.projects.filter", OTelKey: "projects_filter"},
	"UniverseDomain":       {CLIFlag: "google.universe-domain", OTelKey: "universe_domain", Default: []any{DefaultUniverseDomain}},
	"MaxRetries":           {CLIFlag: "stackdriver.max-retries", OTelKey: "max_retries", Default: []any{DefaultMaxRetries}},
	"HTTPTimeout":          {CLIFlag: "stackdriver.http-timeout", OTelKey: "http_timeout", Default: []any{DefaultHTTPTimeout}},
	"MaxBackoff":           {CLIFlag: "stackdriver.max-backoff", OTelKey: "max_backoff", Default: []any{DefaultMaxBackoff}},
	"BackoffJitter":        {CLIFlag: "stackdriver.backoff-jitter", OTelKey: "backoff_jitter", Default: []any{DefaultBackoffJitter}},
	"RetryStatuses":        {CLIFlag: "stackdriver.retry-statuses", OTelKey: "retry_statuses", Default: anyValues(DefaultRetryStatuses)},
	"MetricsPrefixes":      {CLIFlag: "monitoring.metrics-prefixes", OTelKey: "metrics_prefixes"},
	"MetricsInterval":      {CLIFlag: "monitoring.metrics-interval", OTelKey: "metrics_interval", Default: []any{DefaultMetricsInterval}},
	"MetricsOffset":        {CLIFlag: "monitoring.metrics-offset", OTelKey: "metrics_offset", Default: []any{DefaultMetricsOffset}},
	"MetricsIngest":        {CLIFlag: "monitoring.metrics-ingest-delay", OTelKey: "metrics_ingest_delay", Default: []any{DefaultMetricsIngest}},
	"FillMissing":          {CLIFlag: "collector.fill-missing-labels", OTelKey: "fill_missing_labels", Default: []any{DefaultFillMissing}},
	"DropDelegated":        {CLIFlag: "monitoring.drop-delegated-projects", OTelKey: "drop_delegated_projects", Default: []any{DefaultDropDelegated}},
	"Filters":              {CLIFlag: "monitoring.filters", OTelKey: "filters"},
	"AggregateDeltas":      {CLIFlag: "monitoring.aggregate-deltas", OTelKey: "aggregate_deltas", Default: []any{DefaultAggregateDeltas}},
	"DeltasTTL":            {CLIFlag: "monitoring.aggregate-deltas-ttl", OTelKey: "aggregate_deltas_ttl", Default: []any{DefaultDeltasTTL}},
	"DescriptorTTL":        {CLIFlag: "monitoring.descriptor-cache-ttl", OTelKey: "descriptor_cache_ttl", Default: []any{DefaultDescriptorTTL}},
	"DescriptorGoogleOnly": {CLIFlag: "monitoring.descriptor-cache-only-google", OTelKey: "descriptor_cache_only_google", Default: []any{DefaultDescriptorGoogleOnly}},
}

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

func CLIFlag(fieldName string) string {
	option, ok := sharedOptions[fieldName]
	if !ok {
		panic(fmt.Sprintf("unknown shared option %q", fieldName))
	}
	return option.CLIFlag
}

func anyValues[T any](values []T) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
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

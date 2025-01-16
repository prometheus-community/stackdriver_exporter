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

package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/PuerkitoBio/rehttp"
	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	versioncollector "github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/promslog/flag"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	webflag "github.com/prometheus/exporter-toolkit/web/kingpinflag"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/monitoring/v3"
	"google.golang.org/api/option"

	"github.com/prometheus-community/stackdriver_exporter/collectors"
	"github.com/prometheus-community/stackdriver_exporter/delta"
	"github.com/prometheus-community/stackdriver_exporter/utils"
)

var (
	// General exporter flags

	toolkitFlags = webflag.AddFlags(kingpin.CommandLine, ":9255")

	metricsPath = kingpin.Flag(
		"web.telemetry-path", "Path under which to expose Prometheus metrics.",
	).Default("/metrics").String()

	stackdriverMetricsPath = kingpin.Flag(
		"web.stackdriver-telemetry-path", "Path under which to expose Stackdriver metrics.",
	).Default("/metrics").String()

	projectID = kingpin.Flag(
		"google.project-id", "DEPRECATED - Comma seperated list of Google Project IDs. Use 'google.project-ids' instead.",
	).String()

	projectIDs = kingpin.Flag(
		"google.project-ids", "Repeatable flag of Google Project IDs",
	).Strings()

	projectsFilter = kingpin.Flag(
		"google.projects.filter", "Google projects search filter.",
	).String()

	stackdriverMaxRetries = kingpin.Flag(
		"stackdriver.max-retries", "Max number of retries that should be attempted on 503 errors from stackdriver.",
	).Default("0").Int()

	stackdriverHttpTimeout = kingpin.Flag(
		"stackdriver.http-timeout", "How long should stackdriver_exporter wait for a result from the Stackdriver API.",
	).Default("10s").Duration()

	stackdriverMaxBackoffDuration = kingpin.Flag(
		"stackdriver.max-backoff", "Max time between each request in an exp backoff scenario.",
	).Default("5s").Duration()

	stackdriverBackoffJitterBase = kingpin.Flag(
		"stackdriver.backoff-jitter", "The amount of jitter to introduce in a exp backoff scenario.",
	).Default("1s").Duration()

	stackdriverRetryStatuses = kingpin.Flag(
		"stackdriver.retry-statuses", "The HTTP statuses that should trigger a retry.",
	).Default("503").Ints()

	// Monitoring collector flags
	monitoringMetricsTypePrefixes = kingpin.Flag(
		"monitoring.metrics-type-prefixes", "DEPRECATED - Comma separated Google Stackdriver Monitoring Metric Type prefixes. Use 'monitoring.metrics-prefixes' instead.",
	).String()

	monitoringMetricsPrefixes = kingpin.Flag(
		"monitoring.metrics-prefixes", "Google Stackdriver Monitoring Metric Type prefixes. Repeat this flag to scrape multiple prefixes.",
	).Strings()

	monitoringMetricsInterval = kingpin.Flag(
		"monitoring.metrics-interval", "Interval to request the Google Stackdriver Monitoring Metrics for. Only the most recent data point is used.",
	).Default("5m").Duration()

	monitoringMetricsOffset = kingpin.Flag(
		"monitoring.metrics-offset", "Offset for the Google Stackdriver Monitoring Metrics interval into the past.",
	).Default("0s").Duration()

	monitoringMetricsIngestDelay = kingpin.Flag(
		"monitoring.metrics-ingest-delay", "Offset for the Google Stackdriver Monitoring Metrics interval into the past by the ingest delay from the metric's metadata.",
	).Default("false").Bool()

	collectorFillMissingLabels = kingpin.Flag(
		"collector.fill-missing-labels", "Fill missing metrics labels with empty string to avoid label dimensions inconsistent failure.",
	).Default("true").Bool()

	monitoringDropDelegatedProjects = kingpin.Flag(
		"monitoring.drop-delegated-projects", "Drop metrics from attached projects and fetch `project_id` only.",
	).Default("false").Bool()

	monitoringMetricsExtraFilter = kingpin.Flag(
		"monitoring.filters",
		"Filters. i.e: pubsub.googleapis.com/subscription:resource.labels.subscription_id=monitoring.regex.full_match(\"my-subs-prefix.*\")",
	).Strings()

	monitoringMetricsAggregateDeltas = kingpin.Flag(
		"monitoring.aggregate-deltas", "If enabled will treat all DELTA metrics as an in-memory counter instead of a gauge",
	).Default("false").Bool()

	monitoringMetricsDeltasTTL = kingpin.Flag(
		"monitoring.aggregate-deltas-ttl", "How long should a delta metric continue to be exported after GCP stops producing a metric",
	).Default("30m").Duration()

	monitoringDescriptorCacheTTL = kingpin.Flag(
		"monitoring.descriptor-cache-ttl", "How long should the metric descriptors for a prefixed be cached for",
	).Default("0s").Duration()

	monitoringDescriptorCacheOnlyGoogle = kingpin.Flag(
		"monitoring.descriptor-cache-only-google", "Only cache descriptors for *.googleapis.com metrics",
	).Default("true").Bool()
)

func init() {
	prometheus.MustRegister(versioncollector.NewCollector("stackdriver_exporter"))
}

func getDefaultGCPProject(ctx context.Context) (*string, error) {
	credentials, err := google.FindDefaultCredentials(ctx, compute.ComputeScope)
	if err != nil {
		return nil, err
	}
	if credentials.ProjectID == "" {
		return nil, fmt.Errorf("unable to identify the gcloud project. Got empty string")
	}
	return &credentials.ProjectID, nil
}

func createMonitoringService(ctx context.Context) (*monitoring.Service, error) {
	googleClient, err := google.DefaultClient(ctx, monitoring.MonitoringReadScope)
	if err != nil {
		return nil, fmt.Errorf("Error creating Google client: %v", err)
	}

	googleClient.Timeout = *stackdriverHttpTimeout
	googleClient.Transport = rehttp.NewTransport(
		googleClient.Transport, // need to wrap DefaultClient transport
		rehttp.RetryAll(
			rehttp.RetryMaxRetries(*stackdriverMaxRetries),
			rehttp.RetryStatuses(*stackdriverRetryStatuses...)), // Cloud support suggests retrying on 503 errors
		rehttp.ExpJitterDelay(*stackdriverBackoffJitterBase, *stackdriverMaxBackoffDuration), // Set timeout to <10s as that is prom default timeout
	)

	monitoringService, err := monitoring.NewService(ctx, option.WithHTTPClient(googleClient))
	if err != nil {
		return nil, fmt.Errorf("Error creating Google Stackdriver Monitoring service: %v", err)
	}

	return monitoringService, nil
}

type handler struct {
	handler http.Handler
	logger  *slog.Logger

	projectIDs          []string
	metricsPrefixes     []string
	metricsExtraFilters []collectors.MetricFilter
	additionalGatherer  prometheus.Gatherer
	m                   *monitoring.Service
	collectors          *collectors.CollectorCache
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	collectParams := r.URL.Query()["collect"]
	filters := make(map[string]bool)
	for _, param := range collectParams {
		filters[param] = true
	}

	if len(filters) > 0 {
		h.innerHandler(filters).ServeHTTP(w, r)
		return
	}

	h.handler.ServeHTTP(w, r)
}

func newHandler(projectIDs []string, metricPrefixes []string, metricExtraFilters []collectors.MetricFilter, m *monitoring.Service, logger *slog.Logger, additionalGatherer prometheus.Gatherer) *handler {
	var ttl time.Duration
	// Add collector caching TTL as max of deltas aggregation or descriptor caching
	if *monitoringMetricsAggregateDeltas || *monitoringDescriptorCacheTTL > 0 {
		ttl = *monitoringMetricsDeltasTTL
		if *monitoringDescriptorCacheTTL > ttl {
			ttl = *monitoringDescriptorCacheTTL
		}
	} else {
		ttl = 2 * time.Hour
	}

	logger.Info("Creating collector cache", "ttl", ttl)

	h := &handler{
		logger:              logger,
		projectIDs:          projectIDs,
		metricsPrefixes:     metricPrefixes,
		metricsExtraFilters: metricExtraFilters,
		additionalGatherer:  additionalGatherer,
		m:                   m,
		collectors:          collectors.NewCollectorCache(ttl),
	}

	h.handler = h.innerHandler(nil)
	return h
}

func (h *handler) getCollector(project string, filters map[string]bool) (*collectors.MonitoringCollector, error) {
	filterdPrefixes := h.filterMetricTypePrefixes(filters)
	collectorKey := fmt.Sprintf("%s-%v", project, filterdPrefixes)

	if collector, found := h.collectors.Get(collectorKey); found {
		return collector, nil
	}

	collector, err := collectors.NewMonitoringCollector(project, h.m, collectors.MonitoringCollectorOptions{
		MetricTypePrefixes:        filterdPrefixes,
		ExtraFilters:              h.metricsExtraFilters,
		RequestInterval:           *monitoringMetricsInterval,
		RequestOffset:             *monitoringMetricsOffset,
		IngestDelay:               *monitoringMetricsIngestDelay,
		FillMissingLabels:         *collectorFillMissingLabels,
		DropDelegatedProjects:     *monitoringDropDelegatedProjects,
		AggregateDeltas:           *monitoringMetricsAggregateDeltas,
		DescriptorCacheTTL:        *monitoringDescriptorCacheTTL,
		DescriptorCacheOnlyGoogle: *monitoringDescriptorCacheOnlyGoogle,
	}, h.logger, delta.NewInMemoryCounterStore(h.logger, *monitoringMetricsDeltasTTL), delta.NewInMemoryHistogramStore(h.logger, *monitoringMetricsDeltasTTL))
	if err != nil {
		return nil, err
	}
	h.collectors.Store(collectorKey, collector)
	return collector, nil
}

func (h *handler) innerHandler(filters map[string]bool) http.Handler {
	registry := prometheus.NewRegistry()

	for _, project := range h.projectIDs {
		monitoringCollector, err := h.getCollector(project, filters)
		if err != nil {
			h.logger.Error("error creating monitoring collector", "err", err)
			os.Exit(1)
		}
		registry.MustRegister(monitoringCollector)
	}
	var gatherers prometheus.Gatherer = registry
	if h.additionalGatherer != nil {
		gatherers = prometheus.Gatherers{
			h.additionalGatherer,
			registry,
		}
	}
	opts := promhttp.HandlerOpts{ErrorLog: slog.NewLogLogger(h.logger.Handler(), slog.LevelError)}
	// Delegate http serving to Prometheus client library, which will call collector.Collect.
	return promhttp.HandlerFor(gatherers, opts)
}

// filterMetricTypePrefixes filters the initial list of metric type prefixes, with the ones coming from an individual
// prometheus collect request.
func (h *handler) filterMetricTypePrefixes(filters map[string]bool) []string {
	filteredPrefixes := h.metricsPrefixes
	if len(filters) > 0 {
		filteredPrefixes = nil
		for _, prefix := range h.metricsPrefixes {
			for filter := range filters {
				if strings.HasPrefix(filter, prefix) {
					filteredPrefixes = append(filteredPrefixes, filter)
				}
			}
		}
	}
	return parseMetricTypePrefixes(filteredPrefixes)
}

func main() {
	promslogConfig := &promslog.Config{}
	flag.AddFlags(kingpin.CommandLine, promslogConfig)

	kingpin.Version(version.Print("stackdriver_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	logger := promslog.New(promslogConfig)
	if *projectID != "" {
		logger.Warn("The google.project-id flag is deprecated and will be replaced by google.project-ids.")
	}
	if *monitoringMetricsTypePrefixes != "" {
		logger.Warn("The monitoring.metrics-type-prefixes flag is deprecated and will be replaced by monitoring.metrics-prefix.")
	}
	if *monitoringMetricsTypePrefixes == "" && len(*monitoringMetricsPrefixes) == 0 {
		logger.Error("At least one GCP monitoring prefix is required.")
		os.Exit(1)
	}

	ctx := context.Background()
	var discoveredProjectIDs []string

	if len(*projectIDs) == 0 && *projectID == "" && *projectsFilter == "" {
		logger.Info("Neither projectIDs nor projectsFilter was provided. Trying to discover it")
		var err error
		defaultProject, err := getDefaultGCPProject(ctx)
		if err != nil {
			logger.Error("no explicit projectIDs and error trying to discover default GCloud project", "err", err)
			os.Exit(1)
		}
		discoveredProjectIDs = append(discoveredProjectIDs, *defaultProject)
	}

	monitoringService, err := createMonitoringService(ctx)
	if err != nil {
		logger.Error("failed to create monitoring service", "err", err)
		os.Exit(1)
	}

	if *projectsFilter != "" {
		projectIDsFromFilter, err := utils.GetProjectIDsFromFilter(ctx, *projectsFilter)
		if err != nil {
			logger.Error("failed to get project IDs from filter", "err", err)
			os.Exit(1)
		}
		discoveredProjectIDs = append(discoveredProjectIDs, projectIDsFromFilter...)
	}

	if len(*projectIDs) > 0 {
		discoveredProjectIDs = append(discoveredProjectIDs, *projectIDs...)
	}
	if *projectID != "" {
		discoveredProjectIDs = append(discoveredProjectIDs, strings.Split(*projectID, ",")...)
	}

	var metricsPrefixes []string
	if len(*monitoringMetricsPrefixes) > 0 {
		metricsPrefixes = append(metricsPrefixes, *monitoringMetricsPrefixes...)
	}
	if *monitoringMetricsTypePrefixes != "" {
		metricsPrefixes = append(metricsPrefixes, strings.Split(*monitoringMetricsTypePrefixes, ",")...)
	}

	logger.Info(
		"Starting stackdriver_exporter",
		"version", version.Info(),
		"build_context", version.BuildContext(),
		"metric_prefixes", fmt.Sprintf("%v", metricsPrefixes),
		"extra_filters", strings.Join(*monitoringMetricsExtraFilter, ","),
		"projectIDs", fmt.Sprintf("%v", discoveredProjectIDs),
		"projectsFilter", *projectsFilter,
	)

	parsedMetricsPrefixes := parseMetricTypePrefixes(metricsPrefixes)
	metricExtraFilters := parseMetricExtraFilters()
	// drop duplicate projects
	slices.Sort(discoveredProjectIDs)
	uniqueProjectIds := slices.Compact(discoveredProjectIDs)

	if *metricsPath == *stackdriverMetricsPath {
		handler := newHandler(
			uniqueProjectIds, parsedMetricsPrefixes, metricExtraFilters, monitoringService, logger, prometheus.DefaultGatherer)
		http.Handle(*metricsPath, promhttp.InstrumentMetricHandler(prometheus.DefaultRegisterer, handler))
	} else {
		logger.Info("Serving Stackdriver metrics at separate path", "path", *stackdriverMetricsPath)
		handler := newHandler(
			uniqueProjectIds, parsedMetricsPrefixes, metricExtraFilters, monitoringService, logger, nil)
		http.Handle(*stackdriverMetricsPath, promhttp.InstrumentMetricHandler(prometheus.DefaultRegisterer, handler))
		http.Handle(*metricsPath, promhttp.Handler())
	}

	if *metricsPath != "/" && *metricsPath != "" {
		landingConfig := web.LandingConfig{
			Name:        "Stackdriver Exporter",
			Description: "Prometheus Exporter for Google Stackdriver",
			Version:     version.Info(),
			Links: []web.LandingLinks{
				{
					Address: *metricsPath,
					Text:    "Metrics",
				},
			},
		}
		if *metricsPath != *stackdriverMetricsPath {
			landingConfig.Links = append(landingConfig.Links,
				web.LandingLinks{
					Address: *stackdriverMetricsPath,
					Text:    "Stackdriver Metrics",
				},
			)
		}
		landingPage, err := web.NewLandingPage(landingConfig)
		if err != nil {
			logger.Error("error creating landing page", "err", err)
			os.Exit(1)
		}
		http.Handle("/", landingPage)
	}

	srv := &http.Server{}
	if err := web.ListenAndServe(srv, toolkitFlags, logger); err != nil {
		logger.Error("Error starting server", "err", err)
		os.Exit(1)
	}
}

func parseMetricTypePrefixes(inputPrefixes []string) []string {
	metricTypePrefixes := []string{}

	// Drop duplicate prefixes.
	slices.Sort(inputPrefixes)
	uniquePrefixes := slices.Compact(inputPrefixes)

	// Drop prefixes that start with another existing prefix to avoid error:
	// "collected metric xxx was collected before with the same name and label values".
	for i, prefix := range uniquePrefixes {
		if i != 0 {
			previousIndex := len(metricTypePrefixes) - 1

			// Drop current prefix if it starts with the previous one.
			if strings.HasPrefix(prefix, metricTypePrefixes[previousIndex]) {
				continue
			}
		}
		metricTypePrefixes = append(metricTypePrefixes, prefix)
	}

	return metricTypePrefixes
}

func parseMetricExtraFilters() []collectors.MetricFilter {
	var extraFilters []collectors.MetricFilter
	for _, ef := range *monitoringMetricsExtraFilter {
		targetedMetricPrefix, filterQuery := utils.SplitExtraFilter(ef, ":")
		if targetedMetricPrefix != "" {
			extraFilter := collectors.MetricFilter{
				TargetedMetricPrefix: strings.ToLower(targetedMetricPrefix),
				FilterQuery:          filterQuery,
			}
			extraFilters = append(extraFilters, extraFilter)
		}
	}
	return extraFilters
}

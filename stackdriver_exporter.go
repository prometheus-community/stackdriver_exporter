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
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	versioncollector "github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/promslog/flag"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	webflag "github.com/prometheus/exporter-toolkit/web/kingpinflag"
	"google.golang.org/api/monitoring/v3"

	"github.com/prometheus-community/stackdriver_exporter/collectors"
	"github.com/prometheus-community/stackdriver_exporter/config"
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

	healthCheckPath = kingpin.Flag(
		"web.health-check-path", "Path under which to expose health check.",
	).Default("/health").String()

	projectID = kingpin.Flag(
		"google.project-id", "DEPRECATED - Comma seperated list of Google Project IDs. Use 'google.project-ids' instead.",
	).String()

	projectIDs = kingpin.Flag(
		config.ProjectIDs.CLIFlag, "Repeatable flag of Google Project IDs",
	).Strings()

	projectsFilter = kingpin.Flag(
		config.ProjectsFilter.CLIFlag, "Google projects search filter.",
	).String()

	googleUniverseDomain = kingpin.Flag(
		config.UniverseDomain.CLIFlag, "The Cloud universe to use.",
	).Default(config.DefaultUniverseDomain).String()

	stackdriverMaxRetries = kingpin.Flag(
		config.MaxRetries.CLIFlag, "Max number of retries that should be attempted on 503 errors from stackdriver.",
	).Default(strconv.Itoa(config.DefaultMaxRetries)).Int()

	stackdriverHttpTimeout = kingpin.Flag(
		config.HTTPTimeout.CLIFlag, "How long should stackdriver_exporter wait for a result from the Stackdriver API.",
	).Default(config.DefaultHTTPTimeout).Duration()

	stackdriverMaxBackoffDuration = kingpin.Flag(
		config.MaxBackoff.CLIFlag, "Max time between each request in an exp backoff scenario.",
	).Default(config.DefaultMaxBackoff).Duration()

	stackdriverBackoffJitterBase = kingpin.Flag(
		config.BackoffJitter.CLIFlag, "The amount of jitter to introduce in a exp backoff scenario.",
	).Default(config.DefaultBackoffJitter).Duration()

	stackdriverRetryStatuses = kingpin.Flag(
		config.RetryStatuses.CLIFlag, "The HTTP statuses that should trigger a retry.",
	).Default(defaultRetryStatuses()...).Ints()

	// Monitoring collector flags
	monitoringMetricsTypePrefixes = kingpin.Flag(
		"monitoring.metrics-type-prefixes", "DEPRECATED - Comma separated Google Stackdriver Monitoring Metric Type prefixes. Use 'monitoring.metrics-prefixes' instead.",
	).String()

	monitoringMetricsPrefixes = kingpin.Flag(
		config.MetricsPrefixes.CLIFlag, "Google Stackdriver Monitoring Metric Type prefixes. Repeat this flag to scrape multiple prefixes.",
	).Strings()

	monitoringMetricsInterval = kingpin.Flag(
		config.MetricsInterval.CLIFlag, "Interval to request the Google Stackdriver Monitoring Metrics for. Only the most recent data point is used.",
	).Default(config.DefaultMetricsInterval).Duration()

	monitoringMetricsOffset = kingpin.Flag(
		config.MetricsOffset.CLIFlag, "Offset for the Google Stackdriver Monitoring Metrics interval into the past.",
	).Default(config.DefaultMetricsOffset).Duration()

	monitoringMetricsIngestDelay = kingpin.Flag(
		config.MetricsIngest.CLIFlag, "Offset for the Google Stackdriver Monitoring Metrics interval into the past by the ingest delay from the metric's metadata.",
	).Default(strconv.FormatBool(config.DefaultMetricsIngest)).Bool()

	collectorFillMissingLabels = kingpin.Flag(
		config.FillMissing.CLIFlag, "Fill missing metrics labels with empty string to avoid label dimensions inconsistent failure.",
	).Default(strconv.FormatBool(config.DefaultFillMissing)).Bool()

	monitoringDropDelegatedProjects = kingpin.Flag(
		config.DropDelegated.CLIFlag, "Drop metrics from attached projects and fetch `project_id` only.",
	).Default(strconv.FormatBool(config.DefaultDropDelegated)).Bool()

	monitoringMetricsExtraFilter = kingpin.Flag(
		config.Filters.CLIFlag,
		"Filters. i.e: pubsub.googleapis.com/subscription:resource.labels.subscription_id=monitoring.regex.full_match(\"my-subs-prefix.*\")",
	).Strings()

	monitoringMetricsAggregateDeltas = kingpin.Flag(
		config.AggregateDeltas.CLIFlag, "If enabled will treat all DELTA metrics as an in-memory counter instead of a gauge",
	).Default(strconv.FormatBool(config.DefaultAggregateDeltas)).Bool()

	monitoringMetricsDeltasTTL = kingpin.Flag(
		config.DeltasTTL.CLIFlag, "How long should a delta metric continue to be exported after GCP stops producing a metric",
	).Default(config.DefaultDeltasTTL).Duration()

	monitoringDescriptorCacheTTL = kingpin.Flag(
		config.DescriptorTTL.CLIFlag, "How long should the metric descriptors for a prefixed be cached for",
	).Default(config.DefaultDescriptorTTL).Duration()

	monitoringDescriptorCacheOnlyGoogle = kingpin.Flag(
		config.DescriptorGoogleOnly.CLIFlag, "Only cache descriptors for *.googleapis.com metrics",
	).Default(strconv.FormatBool(config.DefaultDescriptorGoogleOnly)).Bool()
)

func init() {
	prometheus.MustRegister(versioncollector.NewCollector("stackdriver_exporter"))
}

type handler struct {
	handler http.Handler
	logger  *slog.Logger

	projectIDs         []string
	cfg                config.RuntimeConfig
	additionalGatherer prometheus.Gatherer
	m                  *monitoring.Service
	collectors         *collectors.CollectorCache
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

func newHandler(projectIDs []string, cfg config.RuntimeConfig, m *monitoring.Service, logger *slog.Logger, additionalGatherer prometheus.Gatherer) *handler {
	logger.Info("Creating collector cache", "ttl", cfg.CollectorCacheTTL())

	h := &handler{
		logger:             logger,
		projectIDs:         projectIDs,
		cfg:                cfg,
		additionalGatherer: additionalGatherer,
		m:                  m,
		collectors:         collectors.NewCollectorCache(cfg.CollectorCacheTTL()),
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

	collector, err := collectors.NewMonitoringCollector(
		project,
		h.m,
		h.cfg.MonitoringCollectorOptionsForPrefixes(filterdPrefixes),
		h.logger,
		delta.NewInMemoryCounterStore(h.logger, h.cfg.DeltasTTL),
		delta.NewInMemoryHistogramStore(h.logger, h.cfg.DeltasTTL),
	)
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
	filteredPrefixes := h.cfg.MetricsPrefixes
	if len(filters) > 0 {
		filteredPrefixes = nil
		for _, prefix := range h.cfg.MetricsPrefixes {
			for filter := range filters {
				if strings.HasPrefix(filter, prefix) {
					filteredPrefixes = append(filteredPrefixes, filter)
				}
			}
		}
	}
	return config.ParseMetricPrefixes(filteredPrefixes)
}

func main() {
	promslogConfig := &promslog.Config{}
	flag.AddFlags(kingpin.CommandLine, promslogConfig)

	kingpin.Version(version.Print("stackdriver_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	http.HandleFunc(*healthCheckPath, healthCheckHandler)

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
	runtimeCfg := collectorRuntimeConfigFromFlags()
	var discoveredProjectIDs []string

	if len(runtimeCfg.ProjectIDs) == 0 && *projectID == "" && runtimeCfg.ProjectsFilter == "" {
		logger.Info("Neither projectIDs nor projectsFilter was provided. Trying to discover it")
		defaultProject, err := config.DiscoverDefaultProjectID(ctx)
		if err != nil {
			logger.Error("no explicit projectIDs and error trying to discover default GCloud project", "err", err)
			os.Exit(1)
		}
		discoveredProjectIDs = append(discoveredProjectIDs, defaultProject)
	}

	monitoringService, err := runtimeCfg.CreateMonitoringService(ctx)
	if err != nil {
		logger.Error("failed to create monitoring service", "err", err)
		os.Exit(1)
	}

	if runtimeCfg.ProjectsFilter != "" {
		projectIDsFromFilter, err := utils.GetProjectIDsFromFilter(ctx, runtimeCfg.ProjectsFilter)
		if err != nil {
			logger.Error("failed to get project IDs from filter", "err", err)
			os.Exit(1)
		}
		discoveredProjectIDs = append(discoveredProjectIDs, projectIDsFromFilter...)
	}

	if len(runtimeCfg.ProjectIDs) > 0 {
		discoveredProjectIDs = append(discoveredProjectIDs, runtimeCfg.ProjectIDs...)
	}
	if *projectID != "" {
		discoveredProjectIDs = append(discoveredProjectIDs, strings.Split(*projectID, ",")...)
	}

	if *monitoringMetricsTypePrefixes != "" {
		runtimeCfg.MetricsPrefixes = append(runtimeCfg.MetricsPrefixes, strings.Split(*monitoringMetricsTypePrefixes, ",")...)
	}

	logger.Info(
		"Starting stackdriver_exporter",
		"version", version.Info(),
		"build_context", version.BuildContext(),
		"metric_prefixes", fmt.Sprintf("%v", runtimeCfg.MetricsPrefixes),
		"extra_filters", strings.Join(runtimeCfg.Filters, ","),
		"projectIDs", fmt.Sprintf("%v", discoveredProjectIDs),
		"projectsFilter", runtimeCfg.ProjectsFilter,
	)

	uniqueProjectIds := config.DeduplicateProjectIDs(discoveredProjectIDs)

	if *metricsPath == *stackdriverMetricsPath {
		handler := newHandler(uniqueProjectIds, runtimeCfg, monitoringService, logger, prometheus.DefaultGatherer)
		http.Handle(*metricsPath, promhttp.InstrumentMetricHandler(prometheus.DefaultRegisterer, handler))
	} else {
		logger.Info("Serving Stackdriver metrics at separate path", "path", *stackdriverMetricsPath)
		handler := newHandler(uniqueProjectIds, runtimeCfg, monitoringService, logger, nil)
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

func collectorRuntimeConfigFromFlags() config.RuntimeConfig {
	return config.RuntimeConfig{
		ProjectIDs:           slices.Clone(*projectIDs),
		ProjectsFilter:       *projectsFilter,
		UniverseDomain:       *googleUniverseDomain,
		MaxRetries:           *stackdriverMaxRetries,
		HTTPTimeout:          *stackdriverHttpTimeout,
		MaxBackoff:           *stackdriverMaxBackoffDuration,
		BackoffJitter:        *stackdriverBackoffJitterBase,
		RetryStatuses:        slices.Clone(*stackdriverRetryStatuses),
		MetricsPrefixes:      slices.Clone(*monitoringMetricsPrefixes),
		MetricsInterval:      *monitoringMetricsInterval,
		MetricsOffset:        *monitoringMetricsOffset,
		MetricsIngest:        *monitoringMetricsIngestDelay,
		FillMissing:          *collectorFillMissingLabels,
		DropDelegated:        *monitoringDropDelegatedProjects,
		Filters:              slices.Clone(*monitoringMetricsExtraFilter),
		AggregateDeltas:      *monitoringMetricsAggregateDeltas,
		DeltasTTL:            *monitoringMetricsDeltasTTL,
		DescriptorTTL:        *monitoringDescriptorCacheTTL,
		DescriptorGoogleOnly: *monitoringDescriptorCacheOnlyGoogle,
	}
}

func defaultRetryStatuses() []string {
	defaults := make([]string, 0, len(config.DefaultRetryStatuses))
	for _, status := range config.DefaultRetryStatuses {
		defaults = append(defaults, strconv.Itoa(status))
	}
	return defaults
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

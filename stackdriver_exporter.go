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
	stdlog "log"
	"net/http"
	"os"
	"strings"

	"github.com/PuerkitoBio/rehttp"
	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	versioncollector "github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
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

	projectIDs = kingpin.Flag(
		"google.project-id", "Google Project IDs. Repeat this flag for multiple projects.",
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

	monitoringMetricsPrefixes = kingpin.Flag(
		"monitoring.metrics-prefixes", "Google Stackdriver Monitoring Metric Type prefixes. Repeat this flag to scrape multiple prefixes.",
	).Required().Strings()

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

func getDefaultGCPProject(ctx context.Context) (*[]string, error) {
	credentials, err := google.FindDefaultCredentials(ctx, compute.ComputeScope)
	if err != nil {
		return nil, err
	}
	if credentials.ProjectID == "" {
		return nil, fmt.Errorf("unable to identify the gcloud project. Got empty string")
	}
	return &[]string{credentials.ProjectID}, nil
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
	logger  log.Logger

	projectIDs          []string
	metricsPrefixes     []string
	metricsExtraFilters []collectors.MetricFilter
	additionalGatherer  prometheus.Gatherer
	m                   *monitoring.Service
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

func newHandler(projectIDs []string, metricPrefixes []string, metricExtraFilters []collectors.MetricFilter, m *monitoring.Service, logger log.Logger, additionalGatherer prometheus.Gatherer) *handler {
	h := &handler{
		logger:              logger,
		projectIDs:          projectIDs,
		metricsPrefixes:     metricPrefixes,
		metricsExtraFilters: metricExtraFilters,
		additionalGatherer:  additionalGatherer,
		m:                   m,
	}

	h.handler = h.innerHandler(nil)
	return h
}

func (h *handler) innerHandler(filters map[string]bool) http.Handler {
	registry := prometheus.NewRegistry()

	for _, project := range h.projectIDs {
		monitoringCollector, err := collectors.NewMonitoringCollector(project, h.m, collectors.MonitoringCollectorOptions{
			MetricTypePrefixes:        h.filterMetricTypePrefixes(filters),
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
			level.Error(h.logger).Log("err", err)
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
	opts := promhttp.HandlerOpts{ErrorLog: stdlog.New(log.NewStdlibAdapter(level.Error(h.logger)), "", 0)}
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
			if filters[prefix] {
				filteredPrefixes = append(filteredPrefixes, prefix)
			}
		}
	}
	return filteredPrefixes
}

func main() {
	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)

	kingpin.Version(version.Print("stackdriver_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	logger := promlog.New(promlogConfig)

	ctx := context.Background()
	if len(*projectIDs) == 0 && *projectsFilter == "" {
		level.Info(logger).Log("msg", "Neither projectID nor projectsFilter was provided. Trying to discover it")
		var err error
		projectIDs, err = getDefaultGCPProject(ctx)
		if err != nil {
			level.Error(logger).Log("msg", "no explicit projectID and error trying to discover default GCloud project", "err", err)
			os.Exit(1)
		}
	}

	monitoringService, err := createMonitoringService(ctx)
	if err != nil {
		level.Error(logger).Log("msg", "failed to create monitoring service", "err", err)
		os.Exit(1)
	}

	var discoveredProjectIDs []string

	if *projectsFilter != "" {
		discoveredProjectIDs, err = utils.GetProjectIDsFromFilter(ctx, *projectsFilter)
		if err != nil {
			level.Error(logger).Log("msg", "failed to get project IDs from filter", "err", err)
			os.Exit(1)
		}
	}

	if len(*projectIDs) > 0 {
		discoveredProjectIDs = append(discoveredProjectIDs, *projectIDs...)
	}

	level.Info(logger).Log(
		"msg", "Starting stackdriver_exporter",
		"version", version.Info(),
		"build_context", version.BuildContext(),
		"projects", *projectIDs,
		"metric_prefixes", *monitoringMetricsPrefixes,
		"extra_filters", strings.Join(*monitoringMetricsExtraFilter, ","),
		"projectIDs", fmt.Sprintf("%v", discoveredProjectIDs),
		"projectsFilter", *projectsFilter,
	)

	metricExtraFilters := parseMetricExtraFilters()

	if *metricsPath == *stackdriverMetricsPath {
		handler := newHandler(
			discoveredProjectIDs, *monitoringMetricsPrefixes, metricExtraFilters, monitoringService, logger, prometheus.DefaultGatherer)
		http.Handle(*metricsPath, promhttp.InstrumentMetricHandler(prometheus.DefaultRegisterer, handler))
	} else {
		level.Info(logger).Log("msg", "Serving Stackdriver metrics at separate path", "path", *stackdriverMetricsPath)
		handler := newHandler(
			discoveredProjectIDs, *monitoringMetricsPrefixes, metricExtraFilters, monitoringService, logger, nil)
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
			level.Error(logger).Log("err", err)
			os.Exit(1)
		}
		http.Handle("/", landingPage)
	}

	srv := &http.Server{}
	if err := web.ListenAndServe(srv, toolkitFlags, logger); err != nil {
		level.Error(logger).Log("msg", "Error starting server", "err", err)
		os.Exit(1)
	}
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

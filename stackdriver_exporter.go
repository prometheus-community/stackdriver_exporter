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
	"net/http"
	"os"
	"strings"

	"cloud.google.com/go/monitoring/apiv3"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/prometheus-community/stackdriver_exporter/collectors"
	"github.com/prometheus-community/stackdriver_exporter/utils"
)

var (
	// General exporter flags

	listenAddress = kingpin.Flag(
		"web.listen-address", "Address to listen on for web interface and telemetry.",
	).Default(":9255").String()

	metricsPath = kingpin.Flag(
		"web.telemetry-path", "Path under which to expose Prometheus metrics.",
	).Default("/metrics").String()

	stackdriverMetricsPath = kingpin.Flag(
		"web.stackdriver-telemetry-path", "Path under which to expose Go runtime metrics.",
	).Default("/metrics").String()

	projectID = kingpin.Flag(
		"google.project-id", "Comma seperated list of Google Project IDs.",
	).String()

	gcpMetricClientTimeout = kingpin.Flag(
		"stackdriver.client-timeout", "How long should the collector wait for a response from the GCP Monitoring APIs",
	).Default("10s").Duration()

	// Monitoring collector flags

	monitoringMetricsTypePrefixes = kingpin.Flag(
		"monitoring.metrics-type-prefixes", "Comma separated Google Stackdriver Monitoring Metric Type prefixes.",
	).Required().String()

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
		"monitoring.filters", "Filters. i.e: pubsub.googleapis.com/subscription:resource.labels.subscription_id=monitoring.regex.full_match(\"my-subs-prefix.*\")").Strings()

	monitoringMetricsAggregateDeltas = kingpin.Flag(
		"monitoring.aggregate-deltas", "If enabled will treat all DELTA metrics as an in-memory counter instead of a gauge",
	).Default("false").Bool()

	monitoringMetricsDeltasTTL = kingpin.Flag(
		"monitoring.aggregate-deltas-ttl", "How long should a delta metric continue to be exported after GCP stops producing a metric",
	).Default("30m").Duration()
)

func init() {
	prometheus.MustRegister(version.NewCollector("stackdriver_exporter"))
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

type handler struct {
	handler http.Handler
	logger  log.Logger

	projectIDs          []string
	metricsPrefixes     []string
	metricsExtraFilters []collectors.MetricFilter
	additionalGatherer  prometheus.Gatherer
	metricClient        *monitoring.MetricClient
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

func newHandler(projectIDs []string, metricPrefixes []string, metricExtraFilters []collectors.MetricFilter, metricClient *monitoring.MetricClient, logger log.Logger, additionalGatherer prometheus.Gatherer) *handler {
	h := &handler{
		logger:              logger,
		projectIDs:          projectIDs,
		metricsPrefixes:     metricPrefixes,
		metricsExtraFilters: metricExtraFilters,
		additionalGatherer:  additionalGatherer,
		metricClient:        metricClient,
	}

	h.handler = h.innerHandler(nil)
	return h
}

func (h *handler) innerHandler(filters map[string]bool) http.Handler {
	registry := prometheus.NewRegistry()

	for _, project := range h.projectIDs {
		monitoringCollector, err := collectors.NewMonitoringCollector(
			project,
			h.metricClient,
			collectors.MonitoringCollectorOptions{
				MetricTypePrefixes:    h.filterMetricTypePrefixes(filters),
				ExtraFilters:          h.metricsExtraFilters,
				RequestInterval:       *monitoringMetricsInterval,
				RequestOffset:         *monitoringMetricsOffset,
				IngestDelay:           *monitoringMetricsIngestDelay,
				FillMissingLabels:     *collectorFillMissingLabels,
				DropDelegatedProjects: *monitoringDropDelegatedProjects,
				AggregateDeltas:       *monitoringMetricsAggregateDeltas,
				ClientTimeout:         *gcpMetricClientTimeout,
			},
			h.logger,
			collectors.NewInMemoryDeltaCounterStore(h.logger, *monitoringMetricsDeltasTTL),
			collectors.NewInMemoryDeltaDistributionStore(h.logger, *monitoringMetricsDeltasTTL),
		)
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

	// Delegate http serving to Prometheus client library, which will call collector.Collect.
	return promhttp.HandlerFor(gatherers, promhttp.HandlerOpts{})
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
	if *projectID == "" {
		level.Info(logger).Log("msg", "no projectID was provided. Trying to discover it")
		var err error
		projectID, err = getDefaultGCPProject(ctx)
		if err != nil {
			level.Error(logger).Log("msg", "no explicit projectID and error trying to discover default GCloud project", "err", err)
			os.Exit(1)
		}
	}

	level.Info(logger).Log("msg", "Starting stackdriver_exporter", "version", version.Info())
	level.Info(logger).Log("msg", "Build context", "build_context", version.BuildContext())
	level.Info(logger).Log("msg", "Using Google Cloud Project ID", "projectID", *projectID)

	//TODO do we need a connection pool or is a single connection okay?
	//pool := option.WithGRPCConnectionPool(10)

	// Notes from: https://pkg.go.dev/cloud.google.com/go#hdr-Timeouts_and_Cancellation
	// The ctx used by the client should never have a timeout as it can interfere with credential refreshing
	// Transient errors will be retried when correctness allows. Cancellation/timeout should be handled via the context
	// provided to a function which makes an API call
	metricClient, err := monitoring.NewMetricClient(ctx, option.WithScopes("https://www.googleapis.com/auth/monitoring.read"))
	if err != nil {
		level.Error(logger).Log("msg", "failed to create monitoring service", "err", err)
		os.Exit(1)
	}

	projectIDs := strings.Split(*projectID, ",")
	metricsTypePrefixes := strings.Split(*monitoringMetricsTypePrefixes, ",")
	metricExtraFilters := parseMetricExtraFilters()

	if *metricsPath == *stackdriverMetricsPath {
		handler := newHandler(
			projectIDs, metricsTypePrefixes, metricExtraFilters, metricClient, logger, prometheus.DefaultGatherer)
		http.Handle(*metricsPath, promhttp.InstrumentMetricHandler(prometheus.DefaultRegisterer, handler))
	} else {
		level.Info(logger).Log("msg", "Serving Stackdriver metrics at separate path", "path", *stackdriverMetricsPath)
		handler := newHandler(
			projectIDs, metricsTypePrefixes, metricExtraFilters, metricClient, logger, nil)
		http.Handle(*stackdriverMetricsPath, promhttp.InstrumentMetricHandler(prometheus.DefaultRegisterer, handler))
		http.Handle(*metricsPath, promhttp.Handler())
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>Stackdriver Exporter</title></head>
             <body>
             <h1>Stackdriver Exporter</h1>
             <p><a href='` + *metricsPath + `'>Metrics</a></p>
             </body>
             </html>`))
	})

	level.Info(logger).Log("msg", "Listening on", "address", *listenAddress)
	if err := http.ListenAndServe(*listenAddress, nil); err != nil {
		level.Error(logger).Log("err", err)
		os.Exit(1)
	}
}

func parseMetricExtraFilters() []collectors.MetricFilter {
	var extraFilters []collectors.MetricFilter
	for _, ef := range *monitoringMetricsExtraFilter {
		efPrefix, efModifier := utils.GetExtraFilterModifiers(ef, ":")
		if efPrefix != "" {
			extraFilter := collectors.MetricFilter{
				Prefix:   efPrefix,
				Modifier: efModifier,
			}
			extraFilters = append(extraFilters, extraFilter)
		}
	}
	return extraFilters
}

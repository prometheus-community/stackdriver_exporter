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

	"github.com/PuerkitoBio/rehttp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/version"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/monitoring/v3"
	"google.golang.org/api/option"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/prometheus-community/stackdriver_exporter/collectors"
)

var (
	projectID = kingpin.Flag(
		"google.project-id", "Google Project ID ($STACKDRIVER_EXPORTER_GOOGLE_PROJECT_ID).",
	).Envar("STACKDRIVER_EXPORTER_GOOGLE_PROJECT_ID").Required().String()

	monitoringDropDelegatedProjects = kingpin.Flag(
		"monitoring.drop-delegated-projects", "Drop metrics from attached projects and fetch `project_id` only ($STACKDRIVER_EXPORTER_DROP_DELEGATED_PROJECTS).",
	).Envar("STACKDRIVER_EXPORTER_DROP_DELEGATED_PROJECTS").Default("false").Bool()

	monitoringMetricsTypePrefixes = kingpin.Flag(
		"monitoring.metrics-type-prefixes", "Comma separated Google Stackdriver Monitoring Metric Type prefixes ($STACKDRIVER_EXPORTER_MONITORING_METRICS_TYPE_PREFIXES).",
	).Envar("STACKDRIVER_EXPORTER_MONITORING_METRICS_TYPE_PREFIXES").Required().String()

	monitoringMetricsInterval = kingpin.Flag(
		"monitoring.metrics-interval", "Interval to request the Google Stackdriver Monitoring Metrics for. Only the most recent data point is used ($STACKDRIVER_EXPORTER_MONITORING_METRICS_INTERVAL).",
	).Envar("STACKDRIVER_EXPORTER_MONITORING_METRICS_INTERVAL").Default("5m").Duration()

	monitoringMetricsOffset = kingpin.Flag(
		"monitoring.metrics-offset", "Offset for the Google Stackdriver Monitoring Metrics interval into the past ($STACKDRIVER_EXPORTER_MONITORING_METRICS_OFFSET).",
	).Envar("STACKDRIVER_EXPORTER_MONITORING_METRICS_OFFSET").Default("0s").Duration()

	listenAddress = kingpin.Flag(
		"web.listen-address", "Address to listen on for web interface and telemetry ($STACKDRIVER_EXPORTER_WEB_LISTEN_ADDRESS).",
	).Envar("STACKDRIVER_EXPORTER_WEB_LISTEN_ADDRESS").Default(":9255").String()

	metricsPath = kingpin.Flag(
		"web.telemetry-path", "Path under which to expose Prometheus metrics ($STACKDRIVER_EXPORTER_WEB_TELEMETRY_PATH).",
	).Envar("STACKDRIVER_EXPORTER_WEB_TELEMETRY_PATH").Default("/metrics").String()

	collectorFillMissingLabels = kingpin.Flag(
		"collector.fill-missing-labels", "Fill missing metrics labels with empty string to avoid label dimensions inconsistent failure ($STACKDRIVER_EXPORTER_COLLECTOR_FILL_MISSING_LABELS).",
	).Envar("STACKDRIVER_EXPORTER_COLLECTOR_FILL_MISSING_LABELS").Default("true").Bool()

	stackdriverMaxRetries = kingpin.Flag(
		"stackdriver.max-retries", "Max number of retries that should be attempted on 503 errors from stackdriver. ($STACKDRIVER_EXPORTER_MAX_RETRIES)",
	).Envar("STACKDRIVER_EXPORTER_MAX_RETRIES").Default("0").Int()

	stackdriverHttpTimeout = kingpin.Flag(
		"stackdriver.http-timeout", "How long should stackdriver_exporter wait for a result from the Stackdriver API ($STACKDRIVER_EXPORTER_HTTP_TIMEOUT)",
	).Envar("STACKDRIVER_EXPORTER_HTTP_TIMEOUT").Default("10s").Duration()

	stackdriverMaxBackoffDuration = kingpin.Flag(
		"stackdriver.max-backoff", "Max time between each request in an exp backoff scenario ($STACKDRIVER_EXPORTER_MAX_BACKOFF_DURATION)",
	).Envar("STACKDRIVER_EXPORTER_MAX_BACKOFF_DURATION").Default("5s").Duration()

	stackdriverBackoffJitterBase = kingpin.Flag(
		"stackdriver.backoff-jitter", "The amount of jitter to introduce in a exp backoff scenario ($STACKDRIVER_EXPORTER_BACKODFF_JITTER_BASE)",
	).Envar("STACKDRIVER_EXPORTER_BACKODFF_JITTER_BASE").Default("1s").Duration()

	stackdriverRetryStatuses = kingpin.Flag(
		"stackdriver.retry-statuses", "The HTTP statuses that should trigger a retry ($STACKDRIVER_EXPORTER_RETRY_STATUSES)",
	).Envar("STACKDRIVER_EXPORTER_RETRY_STATUSES").Default("503").Ints()
)

func init() {
	prometheus.MustRegister(version.NewCollector("stackdriver_exporter"))
}

func createMonitoringService() (*monitoring.Service, error) {
	ctx := context.Background()

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

func main() {
	log.AddFlags(kingpin.CommandLine)
	kingpin.Version(version.Print("stackdriver_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	if *projectID == "" {
		log.Error("Flag `google.project-id` is required")
		os.Exit(1)
	}

	if *monitoringMetricsTypePrefixes == "" {
		log.Error("Flag `monitoring.metrics-type-prefixes` is required")
		os.Exit(1)
	}
	metricsTypePrefixes := strings.Split(*monitoringMetricsTypePrefixes, ",")

	log.Infoln("Starting stackdriver_exporter", version.Info())
	log.Infoln("Build context", version.BuildContext())

	monitoringService, err := createMonitoringService()
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}

	monitoringCollector, err := collectors.NewMonitoringCollector(*projectID, metricsTypePrefixes, *monitoringMetricsInterval, *monitoringMetricsOffset, monitoringService, *collectorFillMissingLabels, *monitoringDropDelegatedProjects)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
	prometheus.MustRegister(monitoringCollector)

	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>Stackdriver Exporter</title></head>
             <body>
             <h1>Stackdriver Exporter</h1>
             <p><a href='` + *metricsPath + `'>Metrics</a></p>
             </body>
             </html>`))
	})

	log.Infoln("Listening on", *listenAddress)
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}

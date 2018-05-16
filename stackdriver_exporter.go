package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/version"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/monitoring/v3"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/frodenas/stackdriver_exporter/collectors"
)

var (
	projectID = kingpin.Flag(
		"google.project-id", "Google Project ID ($STACKDRIVER_EXPORTER_GOOGLE_PROJECT_ID).",
	).Envar("STACKDRIVER_EXPORTER_GOOGLE_PROJECT_ID").Required().String()

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

	monitoringService, err := monitoring.New(googleClient)
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

	monitoringCollector, err := collectors.NewMonitoringCollector(*projectID, metricsTypePrefixes, *monitoringMetricsInterval, *monitoringMetricsOffset, monitoringService, *collectorFillMissingLabels)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
	prometheus.MustRegister(monitoringCollector)

	http.Handle(*metricsPath, prometheus.Handler())
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

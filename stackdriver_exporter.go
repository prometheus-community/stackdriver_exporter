package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/version"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/monitoring/v3"

	"github.com/frodenas/stackdriver_exporter/collectors"
)

var (
	projectID = flag.String(
		"google.project-id", "",
		"Google Project ID ($STACKDRIVER_EXPORTER_GOOGLE_PROJECT_ID).",
	)

	monitoringMetricsTypePrefixes = flag.String(
		"monitoring.metrics-type-prefixes", "",
		"Comma separated Google Stackdriver Monitoring Metric Type prefixes ($STACKDRIVER_EXPORTER_MONITORING_METRICS_TYPE_PREFIXES).",
	)

	monitoringMetricsInterval = flag.Duration(
		"monitoring.metrics-interval", 5*time.Minute,
		"Interval to request the Google Stackdriver Monitoring Metrics for. Only the most recent data point is used ($STACKDRIVER_EXPORTER_MONITORING_METRICS_INTERVAL).",
	)

	listenAddress = flag.String(
		"web.listen-address", ":9255",
		"Address to listen on for web interface and telemetry ($STACKDRIVER_EXPORTER_WEB_LISTEN_ADDRESS).",
	)

	metricsPath = flag.String(
		"web.telemetry-path", "/metrics",
		"Path under which to expose Prometheus metrics ($STACKDRIVER_EXPORTER_WEB_TELEMETRY_PATH)).",
	)

	showVersion = flag.Bool(
		"version", false,
		"Print version information.",
	)
)

func init() {
	prometheus.MustRegister(version.NewCollector("stackdriver_exporter"))
}

func overrideFlagsWithEnvVars() {
	overrideWithEnvVar("STACKDRIVER_EXPORTER_GOOGLE_PROJECT_ID", projectID)
	overrideWithEnvVar("STACKDRIVER_EXPORTER_MONITORING_METRICS_TYPE_PREFIXES", monitoringMetricsTypePrefixes)
	overrideWithEnvDuration("STACKDRIVER_EXPORTER_MONITORING_METRICS_INTERVAL", monitoringMetricsInterval)
	overrideWithEnvVar("STACKDRIVER_EXPORTER_WEB_LISTEN_ADDRESS", listenAddress)
	overrideWithEnvVar("STACKDRIVER_EXPORTER_WEB_TELEMETRY_PATH", metricsPath)
}

func overrideWithEnvVar(name string, value *string) {
	envValue := os.Getenv(name)
	if envValue != "" {
		*value = envValue
	}
}

func overrideWithEnvDuration(name string, value *time.Duration) {
	envValue := os.Getenv(name)
	if envValue != "" {
		var err error
		*value, err = time.ParseDuration(envValue)
		if err != nil {
			log.Fatalf("Invalid `%s`: %s", name, err)
		}
	}
}

func createMonitoringService() (*monitoring.Service, error) {
	ctx := context.Background()

	googleClient, err := google.DefaultClient(ctx, monitoring.MonitoringReadScope)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Error creating Google client: %v", err))
	}

	monitoringService, err := monitoring.New(googleClient)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Error creating Google Stackdriver Monitoring service: %v", err))
	}

	return monitoringService, nil
}

func main() {
	flag.Parse()
	overrideFlagsWithEnvVars()

	if *showVersion {
		fmt.Fprintln(os.Stdout, version.Print("stackdriver_exporter"))
		os.Exit(0)
	}

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

	monitoringCollector, err := collectors.NewMonitoringCollector(*projectID, metricsTypePrefixes, *monitoringMetricsInterval, monitoringService)
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

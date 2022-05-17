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

import "gopkg.in/alecthomas/kingpin.v2"

var (
	// General exporter flags
	listenAddress = kingpin.Flag(
		"web.listen-address", "Address to listen on for web interface and telemetry.",
	).Default(":9255").String()

	metricsPath = kingpin.Flag(
		"web.telemetry-path", "Path under which to expose Prometheus metrics.",
	).Default("/metrics").String()

	projectID = kingpin.Flag(
		"google.project-id", "Comma seperated list of Google Project IDs.",
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
)

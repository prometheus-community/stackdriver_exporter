# Google Stackdriver Prometheus Exporter
[![Build Status](https://circleci.com/gh/prometheus-community/stackdriver_exporter.svg?style=svg)](https://circleci.com/gh/prometheus-community/stackdriver_exporter)
[![golangci-lint](https://github.com/prometheus-community/stackdriver_exporter/actions/workflows/golangci-lint.yml/badge.svg)](https://github.com/prometheus-community/stackdriver_exporter/actions/workflows/golangci-lint.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/prometheus-community/stackdriver_exporter)](https://goreportcard.com/report/github.com/prometheus-community/stackdriver_exporter)
[![GoDoc](https://pkg.go.dev/badge/github.com/prometheus-community/stackdriver_exporter?status.svg)](https://pkg.go.dev/github.com/prometheus-community/stackdriver_exporter?tab=doc)
[![Release](https://img.shields.io/github/v/release/prometheus-community/stackdriver_exporter)](https://github.com/prometheus-community/stackdriver_exporter/releases)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/prometheus-community/stackdriver_exporter)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

A [Prometheus][prometheus] exporter for [Google Stackdriver Monitoring][stackdriver] metrics. It acts as a proxy that requests Stackdriver API for the metric's time-series everytime prometheus scrapes it.

## Installation

### Binaries

Download the already existing [binaries][binaries] for your platform:

```console
$ ./stackdriver_exporter <flags>
```

### From source

Using the standard `go install` (you must have [Go][golang] already installed in your local machine):

```console
$ go install github.com/prometheus-community/stackdriver_exporter
$ stackdriver_exporter <flags>
```

### Docker

To run the stackdriver exporter as a Docker container, run:

```console
$ docker run -p 9255:9255 prometheuscommunity/stackdriver-exporter <flags>
```

#### Kubernetes

You can find a helm chart in the prometheus-community charts repository at <https://github.com/prometheus-community/helm-charts/tree/main/charts/prometheus-stackdriver-exporter>

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm install [RELEASE_NAME] prometheus-community/prometheus-stackdriver-exporter
```

### Cloud Foundry

The exporter can be deployed to an already existing [Cloud Foundry][cloudfoundry] environment:

```console
$ git clone https://github.com/prometheus-community/stackdriver_exporter.git
$ cd stackdriver_exporter
```

Modify the included [application manifest file][manifest] to include the desired properties. Then you can push the exporter to your Cloud Foundry environment:

```console
$ cf push
```

### BOSH

This exporter can be deployed using the [Prometheus BOSH Release][prometheus-boshrelease].

## Usage

### Credentials and Permissions

The Google Stackdriver Exporter uses the Google Golang Client Library, which offers a variety of ways to provide credentials. Please refer to the [Google Application Default Credentials][application-default-credentials] documentation to see how the credentials can be provided.

If you are using IAM roles, the `roles/monitoring.viewer` IAM role contains the required permissions. See the [Access Control Guide][access-control] for more information.

If you are still using the legacy [Access scopes][access-scopes], the `https://www.googleapis.com/auth/monitoring.read` scope is required.

### Flags

| Flag                                | Required | Default                   | Description                                                                                                                                                                                       |
| ----------------------------------- | -------- |---------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `google.project-ids`                 | No       | GCloud SDK auto-discovery | Repeatable flag of Google Project IDs                                                                                                                                                        |
| `google.projects.filter`            | No       |                           | GCloud projects filter expression. See more [here](https://cloud.google.com/sdk/gcloud/reference/projects/list).                                                                                                                                                        |
| `monitoring.metrics-ingest-delay`   | No       |                           | Offsets metric collection by a delay appropriate for each metric type, e.g. because bigquery metrics are slow to appear                                                                           |
| `monitoring.drop-delegated-projects` | No       | No                        | Drop metrics from attached projects and fetch `project_id` only.                                                                                                                                  |
| `monitoring.metrics-prefixes`  | Yes      |                           | Repeatable flag of Google Stackdriver Monitoring Metric Type prefixes (see [example][metrics-prefix-example] and [available metrics][metrics-list])                                                  |
| `monitoring.metrics-interval`       | No       | `5m`                      | Metric's timestamp interval to request from the Google Stackdriver Monitoring Metrics API. Only the most recent data point is used                                                                |
| `monitoring.metrics-offset`         | No       | `0s`                      | Offset (into the past) for the metric's timestamp interval to request from the Google Stackdriver Monitoring Metrics API, to handle latency in published metrics                                  |
| `monitoring.filters`                | No       |                           | Additonal filters to be sent on the Monitoring API call. Add multiple filters by providing this parameter multiple times. See [monitoring.filters](#using-filters) for more info. |
| `monitoring.aggregate-deltas`       | No       |                           | If enabled will treat all DELTA metrics as an in-memory counter instead of a gauge. Be sure to read [what to know about aggregating DELTA metrics](#what-to-know-about-aggregating-delta-metrics) |
| `monitoring.aggregate-deltas-ttl`   | No       | `30m`                     | How long should a delta metric continue to be exported and stored after GCP stops producing it. Read [slow moving metrics](#slow-moving-metrics) to understand the problem this attempts to solve |
| `monitoring.descriptor-cache-ttl`   | No       | `0s`                      | How long should the metric descriptors for a prefixed be cached for                                                                                                                               |
| `stackdriver.max-retries`           | No       | `0`                       | Max number of retries that should be attempted on 503 errors from stackdriver.                                                                                                                    |
| `stackdriver.http-timeout`          | No       | `10s`                     |  How long should stackdriver_exporter wait for a result from the Stackdriver API.                                                                                                                 |
| `stackdriver.max-backoff=`          | No       |                           | Max time between each request in an exp backoff scenario.                                                                                                                                         |
| `stackdriver.backoff-jitter`        | No       | `1s`                       | The amount of jitter to introduce in a exp backoff scenario.                                                                                                                                      |
| `stackdriver.retry-statuses`        | No       | `503`                     |  The HTTP statuses that should trigger a retry.                                                                                                                                                   |
| `web.config.file`                   | No       |                           | [EXPERIMENTAL] Path to configuration file that can enable TLS or authentication.                                                                                                                  |
| `web.listen-address`                | No       | `:9255`                   | Address to listen on for web interface and telemetry Repeatable for multiple addresses.                                                                                                           |
| `web.systemd-socket`                | No       |                           | Use systemd socket activation listeners instead of port listeners (Linux only).                                                                                                                   |
| `web.stackdriver-telemetry-path`    | No       | `/metrics`                | Path under which to expose Stackdriver metrics.                                                                                                                                                   |
| `web.telemetry-path`                | No       | `/metrics`                | Path under which to expose Prometheus metrics                                                                                                                                                     |

### TLS and basic authentication

The Stackdriver Exporter supports TLS and basic authentication.

To use TLS and/or basic authentication, you need to pass a configuration file
using the `--web.config.file` parameter. The format of the file is described
[in the exporter-toolkit repository](https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md).

### Metrics

The exporter returns the following metrics:

| Metric | Description | Labels |
| ------ | ----------- | ------ |
| `stackdriver_monitoring_api_calls_total` | Total number of Google Stackdriver Monitoring API calls made | `project_id` |
| `stackdriver_monitoring_scrapes_total` | Total number of Google Stackdriver Monitoring metrics scrapes | `project_id` |
| `stackdriver_monitoring_scrape_errors_total` | Total number of Google Stackdriver Monitoring metrics scrape errors | `project_id` |
| `stackdriver_monitoring_last_scrape_error` | Whether the last metrics scrape from Google Stackdriver Monitoring resulted in an error (`1` for error, `0` for success) | `project_id` |
| `stackdriver_monitoring_last_scrape_timestamp` | Number of seconds since 1970 since last metrics scrape from Google Stackdriver Monitoring | `project_id` |
| `stackdriver_monitoring_last_scrape_duration_seconds` | Duration of the last metrics scrape from Google Stackdriver Monitoring | `project_id` |

Metrics gathered from Google Stackdriver Monitoring are converted to Prometheus metrics:
* Metric's names are normalized according to the Prometheus [specification][metrics-name] using the following pattern:
  1. `namespace` is a constant prefix (`stackdriver`)
  2. `subsystem` is the normalized monitored resource type (ie `gce_instance`)
  3. `name` is the normalized metric type (ie `compute_googleapis_com_instance_cpu_usage_time`)
* Labels attached to each metric are an aggregation of:
  1. the `unit` in which the metric value is reported
  3. the metric type labels (see [Metrics List][metrics-list])
  4. the monitored resource labels (see [Monitored Resource Types][monitored-resources])
* For each timeseries, only the most recent data point is exported.
* Stackdriver `GAUGE` metric kinds are reported as Prometheus `Gauge` metrics
* Stackdriver `CUMULATIVE` metric kinds are reported as Prometheus `Counter` metrics.
* Stackdriver `DELTA` metric kinds are reported as Prometheus `Gauge` metrics or an accumulating `Counter` if `monitoring.aggregate-deltas` is set
* Only `BOOL`, `INT64`, `DOUBLE` and `DISTRIBUTION` metric types are supported, other types (`STRING` and `MONEY`) are discarded.
* `DISTRIBUTION` metric type is reported as a Prometheus `Histogram`, except the `_sum` time series is not supported.

### Example

If we want to get all `CPU` (`compute.googleapis.com/instance/cpu`) and `Disk` (`compute.googleapis.com/instance/disk`) metrics for all [Google Compute Engine][google-compute] instances, we can run the exporter with the following options:

```
stackdriver_exporter \
  --google.project-ids=my-test-project \
  --monitoring.metrics-prefixes "compute.googleapis.com/instance/cpu"
  --monitoring.metrics-prefixes "compute.googleapis.com/instance/disk"
```

### Using filters

The structure for a filter is `<targeted_metric_prefix>:<filter_query>`

The `targeted_metric_prefix` is used to ensure the filter is only applied to the metric_prefix(es) where it makes sense. 
It does not explicitly have to match a value from `metric_prefixes` but the `targeted_metric_prefix` must be at least a prefix to one or more `metric_prefixes`
 
Example: \
 metrics_prefixes = pubsub.googleapis.com/snapshot, pubsub.googleapis.com/subscription/num_undelivered_messages \
 targeted_metric_prefix options would be \ 
   pubsub.googleapis.com (apply to all defined prefixes) \
   pubsub.googleapis.com/snapshot (apply to only snapshot metrics) \
   pubsub.googleapis.com/subscription (apply to only subscription metrics) \
   pubsub.googleapis.com/subscription/num_undelivered_messages (apply to only the specific subscription metric) \

The `filter_query` will be applied to a final metrics API query when querying for metric data. You can read more about the metric API filter options in GCPs documentation https://cloud.google.com/monitoring/api/v3/filters

The final query sent to the metrics API already includes filters for project and metric type. Each applicable `filter_query` will be appended to the query with an AND

Full example
```
stackdriver_exporter \
 --google.project-ids=my-test-project \
 --monitoring.metrics-prefixes='pubsub.googleapis.com/subscription' \
 --monitoring.metrics-prefixes='compute.googleapis.com/instance/cpu' \
 --monitoring.filters='pubsub.googleapis.com/subscription:resource.labels.subscription_id=monitoring.regex.full_match("us-west4.*my-team-subs.*")' \
 --monitoring.filters='compute.googleapis.com/instance/cpu:resource.labels.instance=monitoring.regex.full_match("us-west4.*my-team-subs.*")'
```

Using projects filter:

```
stackdriver_exporter \
  --google.projects.filter='labels.monitoring="true"'
```

### Filtering enabled collectors

The `stackdriver_exporter` collects all metrics type prefixes by default.

For advanced uses, the collection can be filtered by using a repeatable URL param called `collect`. In the Prometheus configuration you can use you can use this syntax under the [scrape config](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#<scrape_config>).


```yaml
params:
  collect:
  - compute.googleapis.com/instance/cpu
  - compute.googleapis.com/instance/disk
```

### What to know about Aggregating DELTA Metrics

Treating DELTA Metrics as a gauge produces data which is wildly inaccurate/not very useful (see https://github.com/prometheus-community/stackdriver_exporter/issues/116). However, aggregating the DELTA metrics overtime is not a perfect solution and is intended to produce data which mirrors GCP's data as close as possible. 

The biggest challenge to producing a correct result is that a counter for prometheus does not start at 0, it starts at the first value which is exported. This can cause inconsistencies when the exporter first starts and for slow moving metrics which are described below.

#### Start-up Delay

When the exporter first starts it has no persisted counter information and the stores will be empty. When the first sample is received for a series it is intended to be a change from a previous value according to GCP, a delta. But the prometheus counter is not initialized to 0 so it does not export this as a change from 0, it exports that the counter started at the sample value. Since the series exported are dynamic it's not possible to export an [initial 0 value](https://prometheus.io/docs/practices/instrumentation/#avoid-missing-metrics) in order to account for this issue. The end result is that it can take a few cycles for aggregated metrics to start showing rates exactly as GCP. 

As an example consider a prometheus query, `sum by(backend_target_name) (rate(stackdriver_https_lb_rule_loadbalancing_googleapis_com_https_request_bytes_count[1m]))` which is aggregating 5 series. All 5 series will need to have two samples from GCP in order for the query to produce the same result as GCP.

#### Slow Moving Metrics

A slow moving metric would be a metric which is not constantly changing with every sample from GCP. GCP does not consistently report slow moving metrics DELTA metrics. If this occurs for too long (default 5m) prometheus will mark the series as [stale](https://prometheus.io/docs/prometheus/latest/querying/basics/#staleness). The end result is that the next reported sample will be treated as the start of a new series and not an increment from the previous value. Here's an example of this in action, ![](https://user-images.githubusercontent.com/4571540/184961445-ed40237b-108e-4177-9d06-aafe61f92430.png)

There are two features which attempt to combat this issue, 

1. `monitoring.aggregate-deltas-ttl` which controls how long a metric is persisted in the data store after its no longer being reported by GCP
1. Metrics which were not collected during a scrape are still exported at their current counter value

The configuration when using `monitoring.aggregate-deltas` gives a 30 minute buffer to slower moving metrics and `monitoring.aggregate-deltas-ttl` can be adjusted to tune memory requirements vs correctness. Storing the data for longer results in a higher memory cost.

The feature which continues to export metrics which are not collected can cause `the sample has been rejected because another sample with the same timestamp, but a different value, has already been ingested` if your [scrape config](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#scrape_config) for the exporter has `honor_timestamps` enabled (this is the default value). This is caused by the fact that it's not possible to know the different between GCP having late arriving data and GCP not exporting a value. The underlying counter is still incremented when this happens so the next reported sample will show a higher rate than expected.

## Contributing

Refer to the [contributing guidelines][contributing].

## License

Apache License 2.0, see [LICENSE][license].

[access-control]: https://cloud.google.com/monitoring/access-control
[access-scopes]: https://cloud.google.com/compute/docs/access/service-accounts#accesscopesiam
[application-default-credentials]: https://developers.google.com/identity/protocols/application-default-credentials
[binaries]: https://github.com/prometheus-community/stackdriver_exporter/releases
[cloudfoundry]: https://www.cloudfoundry.org/
[contributing]: https://github.com/prometheus-community/stackdriver_exporter/blob/master/CONTRIBUTING.md
[google-compute]: https://cloud.google.com/compute/
[golang]: https://golang.org/
[license]: https://github.com/prometheus-community/stackdriver_exporter/blob/master/LICENSE
[manifest]: https://github.com/prometheus-community/stackdriver_exporter/blob/master/manifest.yml
[metrics-prefix-example]: https://github.com/prometheus-community/stackdriver_exporter#example
[metrics-list]: https://cloud.google.com/monitoring/api/metrics
[metrics-name]: https://prometheus.io/docs/concepts/data_model/#metric-names-and-labels
[monitored-resources]: https://cloud.google.com/monitoring/api/resources
[prometheus]: https://prometheus.io/
[prometheus-boshrelease]: https://github.com/cloudfoundry-community/prometheus-boshrelease
[stackdriver]: https://cloud.google.com/monitoring/

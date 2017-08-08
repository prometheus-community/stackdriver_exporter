# Google Stackdriver Prometheus Exporter [![Build Status](https://travis-ci.org/frodenas/stackdriver_exporter.png)](https://travis-ci.org/frodenas/stackdriver_exporter)

A [Prometheus][prometheus] exporter for [Google Stackdriver Monitoring][stackdriver] metrics. It acts as a proxy that requests Stackdriver API everytime prometheus scrapes it.

## Installation

### Binaries

Download the already existing [binaries][binaries] for your platform:

```bash
$ ./stackdriver_exporter <flags>
```

### From source

Using the standard `go install` (you must have [Go][golang] already installed in your local machine):

```bash
$ go install github.com/frodenas/stackdriver_exporter
$ stackdriver_exporter <flags>
```

### Docker

To run the stackdriver exporter as a Docker container, run:

```bash
$ docker run -p 9255:9255 frodenas/stackdriver-exporter <flags>
```

### Cloud Foundry

The exporter can be deployed to an already existing [Cloud Foundry][cloudfoundry] environment:

```bash
$ git clone https://github.com/frodenas/stackdriver_exporter.git
$ cd stackdriver_exporter
```

Modify the included [application manifest file][manifest] to include the desired properties. Then you can push the exporter to your Cloud Foundry environment:

```bash
$ cf push
```

### BOSH

This exporter can be deployed using the [Prometheus BOSH Release][prometheus-boshrelease].

## Usage

### Credentials and Permissions

The Google Stackdriver Exporter uses the Google Golang Client Library, which offers a variety of ways to provide credentials. Please refer to the [Google Application Default Credentials][application-default-credentials] documentation to see how the credentials can be provided.

If you are using IAM roles, the `monitoring.metricDescriptors.list` and `monitoring.timeSeries.list` IAM permissions are required. The `roles/monitoring.viewer` IAM role contains those permissions. See the [Access Control Guide][access-control] for more information.

If you are still using the legacy [Access scopes][access-scopes], the `https://www.googleapis.com/auth/monitoring.read` scope is required.

### Flags

| Flag / Environment Variable | Required | Default | Description |
| --------------------------- | -------- | ------- | ----------- |
| `google.project-id`<br />`STACKDRIVER_EXPORTER_GOOGLE_PROJECT_ID` | Yes | | Google Project ID |
| `monitoring.metrics-type-prefixes`<br />`STACKDRIVER_EXPORTER_MONITORING_METRICS_TYPE_PREFIXES` | Yes | | Comma separated Google Stackdriver Monitoring Metric Type prefixes (see [example][metrics-prefix-example] and [available metrics][metrics-list]) |
| `monitoring.metrics-interval`<br />`STACKDRIVER_EXPORTER_MONITORING_METRICS_INTERVAL` | No | `1m` | Interval to request the Google Stackdriver Monitoring Metrics for. Only the most recent data point is used |
| `web.listen-address`<br />`STACKDRIVER_EXPORTER_WEB_LISTEN_ADDRESS` | No | `:9255` | Address to listen on for web interface and telemetry |
| `web.telemetry-path`<br />`STACKDRIVER_EXPORTER_WEB_TELEMETRY_PATH` | No | `/metrics` | Path under which to expose Prometheus metrics |

__NOTE:__ monitoring.metrics-interval is not the scape interval; it specifies the metric's timestamp interval to send in the request to the Stackdriver API.

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
* Metrics names are normalized using Prometheus [specification][metrics-name].
* For each timeseries, only the most recent data point is exported.
* Stackdriver `GAUGE` metric kinds are reported as Prometheus `Gauge` metrics; Stackdriver `DELTA` and `CUMULATIVE` metric kinds are reported as Prometheus `Counter` metrics.
* Only `BOOL`, `INT64` and `DOUBLE` metric types are supported, other types (`STRING`, `DISTRIBUTION` and `MONEY`) are discarded.

### Example

If we want to get all `CPU` (`compute.googleapis.com/instance/cpu`) and `Disk` (`compute.googleapis.com/instance/disk`) metrics for all [Google Compute Engine][google-compute] instances, we can run the exporter with the following options:

```
stackdriver_exporter \
  -google.project-id my-test-project \
  -monitoring.metrics-type-prefixes "compute.googleapis.com/instance/cpu,compute.googleapis.com/instance/disk"
```

## Contributing

Refer to the [contributing guidelines][contributing].

## License

Apache License 2.0, see [LICENSE][license].

[access-control]: https://cloud.google.com/monitoring/access-control
[access-scopes]: https://cloud.google.com/compute/docs/access/service-accounts#accesscopesiam
[application-default-credentials]: https://developers.google.com/identity/protocols/application-default-credentials
[binaries]: https://github.com/frodenas/stackdriver_exporter/releases
[cloudfoundry]: https://www.cloudfoundry.org/
[contributing]: https://github.com/frodenas/stackdriver_exporter/blob/master/CONTRIBUTING.md
[google-compute]: https://cloud.google.com/compute/
[golang]: https://golang.org/
[license]: https://github.com/frodenas/stackdriver_exporter/blob/master/LICENSE
[manifest]: https://github.com/frodenas/stackdriver_exporter/blob/master/manifest.yml
[metrics-prefix-example]: https://github.com/frodenas/stackdriver_exporter#example
[metrics-list]: https://cloud.google.com/monitoring/api/metrics
[metrics-name]: https://prometheus.io/docs/concepts/data_model/#metric-names-and-labels
[prometheus]: https://prometheus.io/
[prometheus-boshrelease]: https://github.com/cloudfoundry-community/prometheus-boshrelease
[stackdriver]: https://cloud.google.com/monitoring/

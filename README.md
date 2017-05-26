# Google Stackdriver Prometheus Exporter [![Build Status](https://travis-ci.org/frodenas/stackdriver_exporter.png)](https://travis-ci.org/frodenas/stackdriver_exporter)

A [Prometheus][prometheus] exporter for [Google Stackdriver Monitoring][stackdriver] metrics.

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
| `monitoring.metrics-prefix`<br />`STACKDRIVER_EXPORTER_MONITORING_METRICS_PREFIX` | Yes | | Google Stackdriver Monitoring Metrics Type prefix (see [available metrics][metrics-list]) |
| `monitoring.metrics-interval`<br />`STACKDRIVER_EXPORTER_MONITORING_METRICS_INTERVAL` | No | `1m` | Interval to request the Google Stackdriver Monitoring Metrics for. Only the most recent data point is used |
| `web.listen-address`<br />`STACKDRIVER_EXPORTER_WEB_LISTEN_ADDRESS` | No | `:9255` | Address to listen on for web interface and telemetry |
| `web.telemetry-path`<br />`STACKDRIVER_EXPORTER_WEB_TELEMETRY_PATH` | No | `/metrics` | Path under which to expose Prometheus metrics |

### Metrics

The exporter returns the following metrics:

| Metric | Description | Labels |
| ------ | ----------- | ------ |
| `stackdriver_monitoring_scrapes_total` | Total number of Google Stackdriver Monitoring metrics scrapes | `project_id` |
| `stackdriver_monitoring_scrape_errors_total` | Total number of Google Stackdriver Monitoring metrics scrape errors | `project_id` |
| `stackdriver_monitoring_last_scrape_error` | Whether the last metrics scrape from Google Stackdriver Monitoring resulted in an error (`1` for error, `0` for success) | `project_id` |
| `stackdriver_monitoring_last_scrape_timestamp` | Number of seconds since 1970 since last metrics scrape from Google Stackdriver Monitoring | `project_id` |
| `stackdriver_monitoring_last_scrape_duration_seconds` | Duration of the last metrics scrape from Google Stackdriver Monitoring | `project_id` |

Metrics gathered from Google Stackdriver Monitoring are converted to Prometheus metrics:
* Metrics names are normalized using Prometheus [specification][metrics-name].
* Only `BOOL`, `INT64` and `DOUBLE` metric types are supported, other types are discarded.
* For each timeseries, Only the most recent data point is exported.

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
[golang]: https://golang.org/
[license]: https://github.com/frodenas/stackdriver_exporter/blob/master/LICENSE
[manifest]: https://github.com/frodenas/stackdriver_exporter/blob/master/manifest.yml
[metrics-list]: https://cloud.google.com/monitoring/api/metrics
[metrics-name]: https://prometheus.io/docs/concepts/data_model/#metric-names-and-labels
[prometheus]: https://prometheus.io/
[prometheus-boshrelease]: https://github.com/cloudfoundry-community/prometheus-boshrelease
[stackdriver]: https://cloud.google.com/monitoring/

## master / unreleased

## 0.18.0 / 2025-01-16

- [FEATURE] Support more specific prefixes in ?collect parameter #387
- [FEATURE] Enabling monitoring metrics, aggregate deltas, and descriptor cache with ?collect #389

## 0.17.0 / 2024-11-04

Deprecation notice: The comma delimited flags `google.project-id` and `monitoring.metrics-type-prefixes` are being replaced by repeatable flags `google.project-ids` and `monitoring.metrics-prefixes`. The comma delimited flags will be supported for at least one more release. 

- [CHANGE] Migrate logging to promslog #378
- [ENHANCEMENT] Sanitize metric type prefixes to prevent duplicate metrics #319
- [ENHANCEMENT] Add project ID to all logs from the collector #362
- [FEATURE] Add support for specifying comma-delimited string flags as repeatable flags #355 

## 0.16.0 / 2024-07-15

* [FEATURE] Add ErrorLogger for promhttp #277
* [ENHANCEMENT] Add more info about filters to docs and rename struct fields #198

## 0.15.1 / 2024-05-15

* [BUGFIX] Fix histogram merge #324

## 0.15.0 / 2024-03-07

* [FEATURE] Add projects query #243
* [ENHANCEMENT] Refactor delta logic for library usage #190

## 0.14.1 / 2023-05-26

* [BUGFIX] Fix default listening port #229

## 0.14.0 / 2023-05-26

* [FEATURE] cache descriptors to reduce API calls #218

## 0.13.0 / 2023-01-25

* [FEATURE] Add `monitoring.aggregate-deltas` and `monitoring.aggregate-deltas-ttl` flags which allow aggregating DELTA
  metrics as counters instead of a gauge #168
* [FEATURE] Add `web.stackdriver-telemetry-path` flag. When configured the stackdriver metrics go to this endpoint and
  `web.telemetry-path` contain just the runtime metrics. #173
* [ENHANCEMENT] Make Stackdriver main collector more library-friendly #157
* [BUGFIX] Fixes suspected duplicate label panic for some GCP metric #153
* [BUGFIX] Metrics-ingest-delay bugfix #151
* [BUGFIX] Fix data race on metricDescriptorsFunction start and end times #158

## 0.12.0 / 2022-02-08

Breaking Changes:

The exporter nolonger supports configuration via ENV vars. This was a non-standard feature that is not part of the Prometheus ecossystem. All configuration is now handled by the existing command line arguments.

* [CHANGE] Cleanup non-standard ENV var setup #142
* [FEATURE] Add support to include ingest delay when pull metrics #129
* [FEATURE] Add monitoring.filters flag #133
* [ENHANCEMENT] Setup exporter metrics only once when we can #124

## 0.11.0 / 2020-09-02

* [CHANGE] Do not treat failure to collect metrics as fatal #102
* [FEATURE] Add support for multiple google project IDs #105

## 0.10.0 / 2020-06-28

* [FEATURE] Autodiscover Google Poject ID #62

## 0.9.1 / 2020-06-02

* [BUGFIX] Fix report time missing for histogram metrics #94

## 0.9.0 / 2020-05-26

* [CHANGE] Add stackdriver timestamp to metrics #84
* [CHANGE] Fix collect param name #91

## 0.8.0 / 2020-05-13

* [CHANGE] Treat failure to collect metric as fatal #83
* [CHANGE] Switch logging to promlog #88
* [FEATURE] Add metrics prefix collect URL param #87

## 0.7.0 / 2020-05-01

* [CHANGE] Remove deprecated `monitoring.New()` use. #76
* [ENHANCEMENT] Server-side selection of project's metrics #53
* [BUGFIX] Ensure metrics are fetched once for each metric descriptor #50
  
## 0.6.0 / 2018-12-02

Google Stackdriver Prometheus Exporter v0.6.0:

* Added a `collector.fill-missing-labels` flag to fill missing metrics labels with empty strings in order to avoid label dimensions inconsistent failure (PR https://github.com/frodenas/stackdriver_exporter/pull/23)
* Added `stackdriver.max-retries`, `stackdriver.http-timeout`, `stackdriver.max-backoff`, `stackdriver.backoff-jitter`, and`stackdriver.retry-statuses` flags to allow exponential backoff and retries on stackdriver api (PR https://github.com/frodenas/stackdriver_exporter/pull/35)
* Added a `monitoring.drop-delegated-projects` flag which allows one to disable metrics collection from delegated projects (PR https://github.com/frodenas/stackdriver_exporter/pull/40)
* Fix segmentation fault on missing credentials (PR https://github.com/frodenas/stackdriver_exporter/pull/42)

## master / unreleased

* [CHANGE] Remove deprecated `monitoring.New()` use. #76
  
## 0.6.0 / 2018-12-02

Google Stackdriver Prometheus Exporter v0.6.0:

* Added a `collector.fill-missing-labels` flag to fill missing metrics labels with empty strings in order to avoid label dimensions inconsistent failure (PR https://github.com/frodenas/stackdriver_exporter/pull/23)
* Added `stackdriver.max-retries`, `stackdriver.http-timeout`, `stackdriver.max-backoff`, `stackdriver.backoff-jitter`, and`stackdriver.retry-statuses` flags to allow exponential backoff and retries on stackdriver api (PR https://github.com/frodenas/stackdriver_exporter/pull/35)
* Added a `monitoring.drop-delegated-projects` flag which allows one to disable metrics collection from delegated projects (PR https://github.com/frodenas/stackdriver_exporter/pull/40)
* Fix segmentation fault on missing credentials (PR https://github.com/frodenas/stackdriver_exporter/pull/42)

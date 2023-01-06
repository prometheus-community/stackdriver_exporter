## master / unreleased

## 0.14.0 / 2023-01-06

de981bbbac674f43580ed7fcf13c024ae883c234 Merge pull request #179 from prometheus-community/dependabot/go_modules/github.com/onsi/gomega-1.24.1
0956e148bd8aacdf128f643c9cd0951bd1eae163 Bump github.com/onsi/gomega from 1.19.0 to 1.24.1
70b8b7a9a712ba1b040794edf183531685644fc1 Merge pull request #178 from prometheus-community/dependabot/go_modules/google.golang.org/api-0.103.0
5074cb93495eccde52ddae0eefb39dd3ec1061ea Bump google.golang.org/api from 0.75.0 to 0.103.0
2afe4f4a00d59870d242dbff938130a0a64e510d Update build (#177)
358d272f0353c71662f259aaafec1620b215b0ed Refactor metric naming (#174)
42321dad6875b339062479544cec18b47150fa42 Track delta metrics between scrapes (#168)
5dae76f47f53d274f0bb03d393a86b462b11bbf3 Update common Prometheus files (#176)
33894dca17f7583e332ffc536f526b3b2f05e5bb Make it possible to support runtime metrics on a seperate path (#173)
f98df85cec92119f8363b840eea9a5a52392c7d3 Fix README call with --google.project-id (#160)
b06e49c4f4f18286854a35e9a1452c85c0e84c50 Update common Prometheus files (#163)
08fdfbbcb6ec31ada48ed1a688485921bc2a5cd4 Update common Prometheus files (#161)
d8cd28da178cf730935f067b9f36ce2869f4e5c2 Add trailing backtick for monitoring.metrics-type-prefixes (#159)
b114dabc5167736e5ab6a3f7f65945d73bc3c0cb Make Stackdriver main collector more library-friendly (#157)
ed76fbf70a39e26e0bf71c7cac128aded2371bee request time object data race fixed (#158)
fe27164bd42b3012b9cae65b8a95cc2b24a9aed6 Update common Prometheus files (#156)
8c2d1fbdf0e8f98b28bfeebd0a55440310fd4b20 Metrics-ingest-delay bugfix (#151)
6cbfff7584971a0544c08e9171e49c4d168f8890 Fix the example filter arg (#144)
df5aae7e4cfa549b6640f7b1379ff63f1ac9343b Fixes suspected duplicate label panic for some GCP metric (#153)
bf280ee31a4a7794543f378b6c53999ecdb9b5a0 Update build (#154)
f566765fd4d8ee4aef252228fe5f419ff619e8fc Update common Prometheus files (#155)
4e339aaa282a3b1ea06556127604e170642027ae Update common Prometheus files (#147)
96baa4cfb05edc97cc99a2fba96f6210baeb31fd Update common Prometheus files (#146)

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

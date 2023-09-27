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

package collectors

import (
	"github.com/prometheus/client_golang/prometheus"
)

type InternalMonitoring struct {
	apiCallsTotalMetric             prometheus.Counter
	scrapesTotalMetric              prometheus.Counter
	scrapeErrorsTotalMetric         prometheus.Counter
	lastScrapeErrorMetric           prometheus.Gauge
	lastScrapeTimestampMetric       prometheus.Gauge
	lastScrapeDurationSecondsMetric prometheus.Gauge
	/* Additional internal metrics */
	//numMetricDescriptors prometheus.Gauge
}

func NewInternalMonitoring(shardName string) *InternalMonitoring {
	const (
		namespace = "stackdriver"
		subsystem = "monitoring"
	)
	return &InternalMonitoring{
		apiCallsTotalMetric: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "api_calls_total",
				Help:        "Total number of Google Stackdriver Monitoring API calls made.",
				ConstLabels: prometheus.Labels{"shard": shardName},
			},
		),
		scrapesTotalMetric: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "scrapes_total",
				Help:        "Total number of Google Stackdriver Monitoring metrics scrapes.",
				ConstLabels: prometheus.Labels{"shard": shardName},
			},
		),
		scrapeErrorsTotalMetric: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "scrape_errors_total",
				Help:        "Total number of Google Stackdriver Monitoring metrics scrape errors.",
				ConstLabels: prometheus.Labels{"shard": shardName},
			},
		),
		lastScrapeErrorMetric: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "last_scrape_error",
				Help:        "Whether the last metrics scrape from Google Stackdriver Monitoring resulted in an error (1 for error, 0 for success).",
				ConstLabels: prometheus.Labels{"shard": shardName},
			},
		),
		lastScrapeTimestampMetric: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "last_scrape_timestamp",
				Help:        "Number of seconds since 1970 since last metrics scrape from Google Stackdriver Monitoring.",
				ConstLabels: prometheus.Labels{"shard": shardName},
			},
		),
		lastScrapeDurationSecondsMetric: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "last_scrape_duration_seconds",
				Help:        "Duration of the last metrics scrape from Google Stackdriver Monitoring.",
				ConstLabels: prometheus.Labels{"shard": shardName},
			},
		),
	}
}

func (m *InternalMonitoring) Register() {
	prometheus.MustRegister(m.apiCallsTotalMetric)
	prometheus.MustRegister(m.scrapesTotalMetric)
	prometheus.MustRegister(m.scrapeErrorsTotalMetric)
	prometheus.MustRegister(m.lastScrapeErrorMetric)
	prometheus.MustRegister(m.lastScrapeTimestampMetric)
	prometheus.MustRegister(m.lastScrapeDurationSecondsMetric)
}

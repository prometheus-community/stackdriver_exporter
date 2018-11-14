package collectors

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"golang.org/x/net/context"
	"google.golang.org/api/monitoring/v3"

	"github.com/frodenas/stackdriver_exporter/utils"
)

type MonitoringCollector struct {
	projectID                       string
	metricsTypePrefixes             []string
	metricsInterval                 time.Duration
	metricsOffset                   time.Duration
	monitoringService               *monitoring.Service
	apiCallsTotalMetric             prometheus.Counter
	scrapesTotalMetric              prometheus.Counter
	scrapeErrorsTotalMetric         prometheus.Counter
	lastScrapeErrorMetric           prometheus.Gauge
	lastScrapeTimestampMetric       prometheus.Gauge
	lastScrapeDurationSecondsMetric prometheus.Gauge
	collectorFillMissingLabels      bool
	monitoringDropDelegatedProjects bool
}

func NewMonitoringCollector(
	projectID string,
	metricsTypePrefixes []string,
	metricsInterval time.Duration,
	metricsOffset time.Duration,
	monitoringService *monitoring.Service,
	collectorFillMissingLabels bool,
	monitoringDropDelegatedProjects bool,
) (*MonitoringCollector, error) {
	apiCallsTotalMetric := prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace:   "stackdriver",
			Subsystem:   "monitoring",
			Name:        "api_calls_total",
			Help:        "Total number of Google Stackdriver Monitoring API calls made.",
			ConstLabels: prometheus.Labels{"project_id": projectID},
		},
	)

	scrapesTotalMetric := prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace:   "stackdriver",
			Subsystem:   "monitoring",
			Name:        "scrapes_total",
			Help:        "Total number of Google Stackdriver Monitoring metrics scrapes.",
			ConstLabels: prometheus.Labels{"project_id": projectID},
		},
	)

	scrapeErrorsTotalMetric := prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace:   "stackdriver",
			Subsystem:   "monitoring",
			Name:        "scrape_errors_total",
			Help:        "Total number of Google Stackdriver Monitoring metrics scrape errors.",
			ConstLabels: prometheus.Labels{"project_id": projectID},
		},
	)

	lastScrapeErrorMetric := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace:   "stackdriver",
			Subsystem:   "monitoring",
			Name:        "last_scrape_error",
			Help:        "Whether the last metrics scrape from Google Stackdriver Monitoring resulted in an error (1 for error, 0 for success).",
			ConstLabels: prometheus.Labels{"project_id": projectID},
		},
	)

	lastScrapeTimestampMetric := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace:   "stackdriver",
			Subsystem:   "monitoring",
			Name:        "last_scrape_timestamp",
			Help:        "Number of seconds since 1970 since last metrics scrape from Google Stackdriver Monitoring.",
			ConstLabels: prometheus.Labels{"project_id": projectID},
		},
	)

	lastScrapeDurationSecondsMetric := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace:   "stackdriver",
			Subsystem:   "monitoring",
			Name:        "last_scrape_duration_seconds",
			Help:        "Duration of the last metrics scrape from Google Stackdriver Monitoring.",
			ConstLabels: prometheus.Labels{"project_id": projectID},
		},
	)

	monitoringCollector := &MonitoringCollector{
		projectID:                       projectID,
		metricsTypePrefixes:             metricsTypePrefixes,
		metricsInterval:                 metricsInterval,
		metricsOffset:                   metricsOffset,
		monitoringService:               monitoringService,
		apiCallsTotalMetric:             apiCallsTotalMetric,
		scrapesTotalMetric:              scrapesTotalMetric,
		scrapeErrorsTotalMetric:         scrapeErrorsTotalMetric,
		lastScrapeErrorMetric:           lastScrapeErrorMetric,
		lastScrapeTimestampMetric:       lastScrapeTimestampMetric,
		lastScrapeDurationSecondsMetric: lastScrapeDurationSecondsMetric,
		collectorFillMissingLabels:      collectorFillMissingLabels,
		monitoringDropDelegatedProjects: monitoringDropDelegatedProjects,
	}

	return monitoringCollector, nil
}

func (c *MonitoringCollector) Describe(ch chan<- *prometheus.Desc) {
	c.apiCallsTotalMetric.Describe(ch)
	c.scrapesTotalMetric.Describe(ch)
	c.scrapeErrorsTotalMetric.Describe(ch)
	c.lastScrapeErrorMetric.Describe(ch)
	c.lastScrapeTimestampMetric.Describe(ch)
	c.lastScrapeDurationSecondsMetric.Describe(ch)
}

func (c *MonitoringCollector) Collect(ch chan<- prometheus.Metric) {
	var begun = time.Now()

	errorMetric := float64(0)
	if err := c.reportMonitoringMetrics(ch); err != nil {
		errorMetric = float64(1)
		c.scrapeErrorsTotalMetric.Inc()
		log.Errorf("Error while getting Google Stackdriver Monitoring metrics: %s", err)
	}
	c.scrapeErrorsTotalMetric.Collect(ch)

	c.apiCallsTotalMetric.Collect(ch)

	c.scrapesTotalMetric.Inc()
	c.scrapesTotalMetric.Collect(ch)

	c.lastScrapeErrorMetric.Set(errorMetric)
	c.lastScrapeErrorMetric.Collect(ch)

	c.lastScrapeTimestampMetric.Set(float64(time.Now().Unix()))
	c.lastScrapeTimestampMetric.Collect(ch)

	c.lastScrapeDurationSecondsMetric.Set(time.Since(begun).Seconds())
	c.lastScrapeDurationSecondsMetric.Collect(ch)
}

func (c *MonitoringCollector) reportMonitoringMetrics(ch chan<- prometheus.Metric) error {
	metricDescriptorsFunction := func(page *monitoring.ListMetricDescriptorsResponse) error {
		var wg = &sync.WaitGroup{}

		c.apiCallsTotalMetric.Inc()

		errChannel := make(chan error, len(page.MetricDescriptors))

		endTime := time.Now().UTC().Add(c.metricsOffset * -1)
		startTime := endTime.Add(c.metricsInterval * -1)

		for _, metricDescriptor := range page.MetricDescriptors {
			wg.Add(1)
			go func(metricDescriptor *monitoring.MetricDescriptor, ch chan<- prometheus.Metric) {
				defer wg.Done()
				log.Debugf("Retrieving Google Stackdriver Monitoring metrics for descriptor `%s`...", metricDescriptor.Type)
				timeSeriesListCall := c.monitoringService.Projects.TimeSeries.List(utils.ProjectResource(c.projectID)).
					Filter(fmt.Sprintf("metric.type=\"%s\"", metricDescriptor.Type)).
					IntervalStartTime(startTime.Format(time.RFC3339Nano)).
					IntervalEndTime(endTime.Format(time.RFC3339Nano))

				for {
					c.apiCallsTotalMetric.Inc()
					page, err := timeSeriesListCall.Do()
					if err != nil {
						log.Errorf("Error retrieving Time Series metrics for descriptor `%s`: %v", metricDescriptor.Type, err)
						errChannel <- err
						break
					}
					if page == nil {
						break
					}
					if err := c.reportTimeSeriesMetrics(page, metricDescriptor, ch); err != nil {
						log.Errorf("Error reporting Time Series metrics for descriptor `%s`: %v", metricDescriptor.Type, err)
						errChannel <- err
						break
					}
					if page.NextPageToken == "" {
						break
					}
					timeSeriesListCall.PageToken(page.NextPageToken)
				}
			}(metricDescriptor, ch)
		}

		wg.Wait()
		close(errChannel)

		return <-errChannel
	}

	var wg = &sync.WaitGroup{}

	errChannel := make(chan error, len(c.metricsTypePrefixes))

	for _, metricsTypePrefix := range c.metricsTypePrefixes {
		wg.Add(1)
		go func(metricsTypePrefix string) {
			defer wg.Done()
			log.Debugf("Listing Google Stackdriver Monitoring metric descriptors starting with `%s`...", metricsTypePrefix)
			ctx := context.Background()
			if err := c.monitoringService.Projects.MetricDescriptors.List(utils.ProjectResource(c.projectID)).
				Filter(fmt.Sprintf("metric.type = starts_with(\"%s\")", metricsTypePrefix)).
				Pages(ctx, metricDescriptorsFunction); err != nil {
				errChannel <- err
			}
		}(metricsTypePrefix)
	}

	wg.Wait()
	close(errChannel)

	return <-errChannel
}

func (c *MonitoringCollector) reportTimeSeriesMetrics(
	page *monitoring.ListTimeSeriesResponse,
	metricDescriptor *monitoring.MetricDescriptor,
	ch chan<- prometheus.Metric,
) error {
	var metricValue float64
	var metricValueType prometheus.ValueType
	var newestTSPoint *monitoring.Point

	timeSeriesMetrics := &TimeSeriesMetrics{
		metricDescriptor:  metricDescriptor,
		ch:                ch,
		fillMissingLabels: c.collectorFillMissingLabels,
		constMetrics:      make(map[string][]ConstMetric),
		histogramMetrics:  make(map[string][]HistogramMetric),
	}
	for _, timeSeries := range page.TimeSeries {
		newestEndTime := time.Unix(0, 0)
		for _, point := range timeSeries.Points {
			endTime, err := time.Parse(time.RFC3339Nano, point.Interval.EndTime)
			if err != nil {
				return fmt.Errorf("Error parsing TimeSeries Point interval end time `%s`: %s", point.Interval.EndTime, err)
			}
			if endTime.After(newestEndTime) {
				newestEndTime = endTime
				newestTSPoint = point
			}
		}
		labelKeys := []string{"unit"}
		labelValues := []string{metricDescriptor.Unit}

		// Add the metric labels
		// @see https://cloud.google.com/monitoring/api/metrics
		for key, value := range timeSeries.Metric.Labels {
			labelKeys = append(labelKeys, key)
			labelValues = append(labelValues, value)
		}

		// Add the monitored resource labels
		// @see https://cloud.google.com/monitoring/api/resources
		for key, value := range timeSeries.Resource.Labels {
			labelKeys = append(labelKeys, key)
			labelValues = append(labelValues, value)
		}

		if c.monitoringDropDelegatedProjects {
			dropDelegatedProject := false

			for idx, val := range labelKeys {
				if val == "project_id" && labelValues[idx] != c.projectID {
					dropDelegatedProject = true
					break
				}
			}

			if dropDelegatedProject {
				continue
			}
		}

		switch timeSeries.MetricKind {
		case "GAUGE":
			metricValueType = prometheus.GaugeValue
		case "DELTA":
			metricValueType = prometheus.GaugeValue
		case "CUMULATIVE":
			metricValueType = prometheus.CounterValue
		default:
			continue
		}

		switch timeSeries.ValueType {
		case "BOOL":
			metricValue = 0
			if *newestTSPoint.Value.BoolValue {
				metricValue = 1
			}
		case "INT64":
			metricValue = float64(*newestTSPoint.Value.Int64Value)
		case "DOUBLE":
			metricValue = *newestTSPoint.Value.DoubleValue
		case "DISTRIBUTION":
			dist := newestTSPoint.Value.DistributionValue
			buckets, err := c.generateHistogramBuckets(dist)
			if err == nil {
				timeSeriesMetrics.CollectNewConstHistogram(timeSeries, labelKeys, dist, buckets, labelValues)
			} else {
				log.Debugf("Discarding resource %s metric %s: %s", timeSeries.Resource.Type, timeSeries.Metric.Type, err)
			}
			continue
		default:
			log.Debugf("Discarding `%s` metric: %+v", timeSeries.ValueType, timeSeries)
			continue
		}

		timeSeriesMetrics.CollectNewConstMetric(timeSeries, labelKeys, metricValueType, metricValue, labelValues)
	}
	timeSeriesMetrics.Complete()
	return nil
}

func (c *MonitoringCollector) generateHistogramBuckets(
	dist *monitoring.Distribution,
) (map[float64]uint64, error) {
	opts := dist.BucketOptions
	var bucketKeys []float64
	switch {
	case opts.ExplicitBuckets != nil:
		// @see https://cloud.google.com/monitoring/api/ref_v3/rest/v3/TypedValue#explicit
		bucketKeys = make([]float64, len(opts.ExplicitBuckets.Bounds)+1)
		for i, b := range opts.ExplicitBuckets.Bounds {
			bucketKeys[i] = b
		}
	case opts.LinearBuckets != nil:
		// @see https://cloud.google.com/monitoring/api/ref_v3/rest/v3/TypedValue#linear
		// NumFiniteBuckets is inclusive so bucket count is num+2
		num := int(opts.LinearBuckets.NumFiniteBuckets)
		bucketKeys = make([]float64, num+2)
		for i := 0; i <= num; i++ {
			bucketKeys[i] = opts.LinearBuckets.Offset + (float64(i) * opts.LinearBuckets.Width)
		}
	case opts.ExponentialBuckets != nil:
		// @see https://cloud.google.com/monitoring/api/ref_v3/rest/v3/TypedValue#exponential
		// NumFiniteBuckets is inclusive so bucket count is num+2
		num := int(opts.ExponentialBuckets.NumFiniteBuckets)
		bucketKeys = make([]float64, num+2)
		for i := 0; i <= num; i++ {
			bucketKeys[i] = opts.ExponentialBuckets.Scale * math.Pow(opts.ExponentialBuckets.GrowthFactor, float64(i))
		}
	default:
		return nil, errors.New("Unknown distribution buckets")
	}
	// The last bucket is always infinity
	// @see https://cloud.google.com/monitoring/api/ref_v3/rest/v3/TypedValue#bucketoptions
	bucketKeys[len(bucketKeys)-1] = math.Inf(1)

	// Prometheus expects each bucket to have a lower bound of 0, but Google
	// sends a bucket with a lower bound of the previous bucket's upper bound, so
	// we need to store the last bucket and add it to the next bucket to make it
	// 0-bound.
	// Any remaining keys without data have a value of 0
	buckets := map[float64]uint64{}
	var last uint64
	for i, b := range bucketKeys {
		if len(dist.BucketCounts) > i {
			buckets[b] = uint64(dist.BucketCounts[i]) + last
			last = buckets[b]
		} else {
			buckets[b] = last
		}
	}
	return buckets, nil
}

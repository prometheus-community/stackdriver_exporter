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

	cache *CollectionCache
}

type CollectionCache struct {
	// This map holds the read-only result of a collection run
	// It will be served from the promethus scrape endpoint until the next
	// collection is complete.
	cachedTimeSeries map[string]*TimeSeriesMetrics

	// This map holds the (potentially incomplete) metrics that have been collected.
	// Once completed it will replace the `cachedTimeSeries` and will start being served.
	activeTimeSeries map[string]*TimeSeriesMetrics

	// Indicates whether there is a collection currently running, and populating `activeTimeSeries`
	// at the moment.
	collectionActive bool

	// Guards `activeTimeSeries` and `collectionActive`
	mu sync.Mutex
}

// Update the cache state to indicate that a collection has started
func (c *CollectionCache) markCollectionStarted() {
	log.Debugf("markCollectionStarted")
	c.mu.Lock()
	c.collectionActive = true
	c.mu.Unlock()
}

// Update the cache state to indicate that a collection has completed
func (c *CollectionCache) markCollectionCompleted() {
	log.Debugf("markCollectionCompleted")
	c.mu.Lock()
	defer c.mu.Unlock()
	collected := c.activeTimeSeries
	c.cachedTimeSeries = collected
	c.activeTimeSeries = make(map[string]*TimeSeriesMetrics)
	c.collectionActive = false
}

// Check if there is a collection running int he background
func (c *CollectionCache) isCollectionActive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.collectionActive
}

// During a collection, this func should be used to save the collected data
func (c *CollectionCache) putMetric(metricType string, timeSeries *TimeSeriesMetrics) {
	c.mu.Lock()
	c.activeTimeSeries[metricType] = timeSeries
	c.mu.Unlock()
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
		cache: &CollectionCache{
			cachedTimeSeries: make(map[string]*TimeSeriesMetrics),
			activeTimeSeries: make(map[string]*TimeSeriesMetrics),
			collectionActive: false,
		},
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

	for _, timeSeries := range c.cache.cachedTimeSeries {
		timeSeries.Complete(ch)
	}

	c.scrapeErrorsTotalMetric.Collect(ch)
	c.apiCallsTotalMetric.Collect(ch)
	c.scrapesTotalMetric.Collect(ch)
	c.lastScrapeErrorMetric.Collect(ch)
	c.lastScrapeTimestampMetric.Collect(ch)
	c.lastScrapeDurationSecondsMetric.Collect(ch)

	if !c.cache.isCollectionActive() {
		go func() {
			start := time.Now()
			errorMetric := float64(0)

			c.cache.markCollectionStarted()
			if err := c.updateMetricsCache(); err != nil {
				errorMetric = float64(1)
				c.scrapeErrorsTotalMetric.Inc()
				log.Errorf("Error while getting Google Stackdriver Monitoring metrics: %s", err)
			}

			c.scrapesTotalMetric.Inc()
			c.lastScrapeErrorMetric.Set(errorMetric)
			c.lastScrapeTimestampMetric.Set(float64(time.Now().Unix()))
			c.lastScrapeDurationSecondsMetric.Set(time.Since(start).Seconds())

			c.cache.markCollectionCompleted()
		}()
	}
}

func (c *MonitoringCollector) updateMetricsCache() error {
	metricDescriptorsFunction := func(page *monitoring.ListMetricDescriptorsResponse) error {
		var wg = &sync.WaitGroup{}

		c.apiCallsTotalMetric.Inc()

		// It has been noticed that the same metric descriptor can be obtained from different GCP
		// projects. When that happens, metrics are fetched twice and it provokes the error:
		//     "collected metric xxx was collected before with the same name and label values"
		//
		// Metric descriptor project is irrelevant when it comes to fetch metrics, as they will be
		// fetched from all the delegated projects filtering by metric type. Considering that, we
		// can filter descriptors to keep just one per type.
		//
		// The following makes sure metric descriptors are unique to avoid fetching more than once
		uniqueDescriptors := make(map[string]*monitoring.MetricDescriptor)
		for _, descriptor := range page.MetricDescriptors {
			uniqueDescriptors[descriptor.Type] = descriptor
		}

		errChannel := make(chan error, len(uniqueDescriptors))

		endTime := time.Now().UTC().Add(c.metricsOffset * -1)
		startTime := endTime.Add(c.metricsInterval * -1)

		for _, metricDescriptor := range uniqueDescriptors {
			wg.Add(1)
			go func(metricDescriptor *monitoring.MetricDescriptor) {
				defer wg.Done()
				log.Debugf("Retrieving Google Stackdriver Monitoring metrics for descriptor `%s`...", metricDescriptor.Type)
				filter := fmt.Sprintf("metric.type=\"%s\"", metricDescriptor.Type)
				if c.monitoringDropDelegatedProjects {
					filter = fmt.Sprintf(
						"project=\"%s\" AND metric.type=\"%s\"",
						c.projectID,
						metricDescriptor.Type)
				}
				timeSeriesListCall := c.monitoringService.Projects.TimeSeries.List(utils.ProjectResource(c.projectID)).
					Filter(filter).
					PageSize(100000).
					IntervalStartTime(startTime.Format(time.RFC3339Nano)).
					IntervalEndTime(endTime.Format(time.RFC3339Nano))

				pageNumber := 0

				start := time.Now()
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
					if err := c.updateMetricsCacheForMetric(page, metricDescriptor); err != nil {
						log.Errorf("Error reporting Time Series metrics for descriptor `%s`: %v", metricDescriptor.Type, err)
						errChannel <- err
						break
					}
					if page.NextPageToken == "" {
						break
					}
					pageNumber++
					timeSeriesListCall.PageToken(page.NextPageToken)
				}

				elapsed := time.Since(start)
				log.Debugf("Took %s to retrieve %v pages for metric descriptor %s", elapsed, pageNumber+1, metricDescriptor.Type)

			}(metricDescriptor)
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
			filter := fmt.Sprintf("metric.type = starts_with(\"%s\")", metricsTypePrefix)
			if c.monitoringDropDelegatedProjects {
				filter = fmt.Sprintf(
					"project = \"%s\" AND metric.type = starts_with(\"%s\")",
					c.projectID,
					metricsTypePrefix)
			}
			if err := c.monitoringService.Projects.MetricDescriptors.List(utils.ProjectResource(c.projectID)).
				Filter(filter).
				Pages(ctx, metricDescriptorsFunction); err != nil {
				errChannel <- err
			}
		}(metricsTypePrefix)
	}

	wg.Wait()
	close(errChannel)

	return <-errChannel
}

func (c *MonitoringCollector) updateMetricsCacheForMetric(
	page *monitoring.ListTimeSeriesResponse,
	metricDescriptor *monitoring.MetricDescriptor) error {
	var metricValue float64
	var metricValueType prometheus.ValueType
	var newestTSPoint *monitoring.Point

	timeSeriesMetrics := &TimeSeriesMetrics{
		metricDescriptor:  metricDescriptor,
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
	c.cache.putMetric(metricDescriptor.Type, timeSeriesMetrics)
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

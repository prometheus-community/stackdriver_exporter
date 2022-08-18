package collectors

import (
	"time"

	"google.golang.org/api/monitoring/v3"
)

type CollectedMetric struct {
	metric          *ConstMetric
	lastCollectedAt time.Time
}

type MetricDescriptor struct {
	name        string
	description string
}

type DeltaCounterStore interface {
	Increment(metricDescriptor *monitoring.MetricDescriptor, currentValue *ConstMetric)
	ListMetricsByName(metricDescriptorName string) map[string][]*CollectedMetric
	ListMetricDescriptorsNotCollected(since time.Time) []MetricDescriptor
}

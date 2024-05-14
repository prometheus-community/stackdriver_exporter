// Copyright 2023 The Prometheus Authors
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

package delta_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/promlog"
	"google.golang.org/api/monitoring/v3"

	"github.com/prometheus-community/stackdriver_exporter/collectors"
	"github.com/prometheus-community/stackdriver_exporter/delta"
)

var _ = Describe("HistogramStore", func() {
	var store *delta.InMemoryHistogramStore
	var histogram *collectors.HistogramMetric
	descriptor := &monitoring.MetricDescriptor{Name: "This is a metric"}
	bucketKey := 1.00000000000000000001
	bucketValue := uint64(1000)

	BeforeEach(func() {
		store = delta.NewInMemoryHistogramStore(promlog.New(&promlog.Config{}), time.Minute)
		histogram = &collectors.HistogramMetric{
			FqName:         "histogram_name",
			LabelKeys:      []string{"labelKey"},
			Sum:            10,
			Count:          100,
			Buckets:        map[float64]uint64{bucketKey: bucketValue},
			LabelValues:    []string{"labelValue"},
			ReportTime:     time.Now().Truncate(time.Second),
			CollectionTime: time.Now().Truncate(time.Second),
			KeysHash:       8765,
		}
	})

	It("can return tracked histograms", func() {
		store.Increment(descriptor, histogram)
		metrics := store.ListMetrics(descriptor.Name)

		Expect(len(metrics)).To(Equal(1))
		Expect(metrics[0]).To(Equal(histogram))
	})

	It("can merge histograms", func() {
		store.Increment(descriptor, histogram)

		// Shallow copy and change report time so they will merge
		nextValue := &collectors.HistogramMetric{
			FqName:         "histogram_name",
			LabelKeys:      []string{"labelKey"},
			Sum:            10,
			Count:          100,
			Buckets:        map[float64]uint64{bucketKey: bucketValue},
			LabelValues:    []string{"labelValue"},
			ReportTime:     time.Now().Truncate(time.Second).Add(time.Second),
			CollectionTime: time.Now().Truncate(time.Second),
			KeysHash:       8765,
		}

		store.Increment(descriptor, nextValue)

		metrics := store.ListMetrics(descriptor.Name)

		Expect(len(metrics)).To(Equal(1))
		histogram := metrics[0]
		Expect(histogram.Count).To(Equal(uint64(200)))
		Expect(histogram.Sum).To(Equal(20.0))
		Expect(len(histogram.Buckets)).To(Equal(1))
		Expect(histogram.Buckets[bucketKey]).To(Equal(bucketValue * 2))
	})

	It("will remove histograms outside of TTL", func() {
		histogram.CollectionTime = histogram.CollectionTime.Add(-time.Hour)

		store.Increment(descriptor, histogram)

		metrics := store.ListMetrics(descriptor.Name)
		Expect(len(metrics)).To(Equal(0))
	})
})

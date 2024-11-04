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
	"github.com/prometheus/common/promslog"
	"google.golang.org/api/monitoring/v3"

	"github.com/prometheus-community/stackdriver_exporter/collectors"
	"github.com/prometheus-community/stackdriver_exporter/delta"
)

var _ = Describe("Counter", func() {
	var store *delta.InMemoryCounterStore
	var metric *collectors.ConstMetric
	descriptor := &monitoring.MetricDescriptor{Name: "This is a metric"}

	BeforeEach(func() {
		store = delta.NewInMemoryCounterStore(promslog.New(&promslog.Config{}), time.Minute)
		metric = &collectors.ConstMetric{
			FqName:         "counter_name",
			LabelKeys:      []string{"labelKey"},
			ValueType:      1,
			Value:          10,
			LabelValues:    []string{"labelValue"},
			ReportTime:     time.Now().Truncate(time.Second),
			CollectionTime: time.Now().Truncate(time.Second),
			KeysHash:       4321,
		}
	})

	It("can return tracked counters", func() {
		store.Increment(descriptor, metric)
		metrics := store.ListMetrics(descriptor.Name)

		Expect(len(metrics)).To(Equal(1))
		Expect(metrics[0]).To(Equal(metric))
	})

	It("can increment counters multiple times", func() {
		store.Increment(descriptor, metric)

		metric2 := &collectors.ConstMetric{
			FqName:         "counter_name",
			LabelKeys:      []string{"labelKey"},
			ValueType:      1,
			Value:          20,
			LabelValues:    []string{"labelValue"},
			ReportTime:     time.Now().Truncate(time.Second).Add(time.Second),
			CollectionTime: time.Now().Truncate(time.Second),
			KeysHash:       4321,
		}

		store.Increment(descriptor, metric2)

		metrics := store.ListMetrics(descriptor.Name)
		Expect(len(metrics)).To(Equal(1))
		Expect(metrics[0].Value).To(Equal(float64(30)))
	})

	It("will remove counters outside of TTL", func() {
		metric.CollectionTime = metric.CollectionTime.Add(-time.Hour)

		store.Increment(descriptor, metric)

		metrics := store.ListMetrics(descriptor.Name)
		Expect(len(metrics)).To(Equal(0))
	})
})

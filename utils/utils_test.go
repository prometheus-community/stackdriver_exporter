package utils_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/frodenas/stackdriver_exporter/utils"
)

var _ = Describe("NormalizeMetricName", func() {
	It("returns a normalized metric name", func() {
		Expect(NormalizeMetricName("This_is__a-MetricName.Example/with:0totals")).To(Equal("this_is_a_metric_name_example_with_0_totals"))
	})
})

var _ = Describe("ProjectResource", func() {
	It("returns a project resource", func() {
		Expect(ProjectResource("fake-project-1")).To(Equal("projects/fake-project-1"))
	})
})

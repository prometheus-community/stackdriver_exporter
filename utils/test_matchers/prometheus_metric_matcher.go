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

package test_matchers

import (
	"fmt"
	"reflect"

	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func PrometheusMetric(expected prometheus.Metric) types.GomegaMatcher {
	expectedMetric := &dto.Metric{}
	expected.Write(expectedMetric)

	return &PrometheusMetricMatcher{
		Expected: expectedMetric,
	}
}

type PrometheusMetricMatcher struct {
	Expected *dto.Metric
}

func (matcher *PrometheusMetricMatcher) Match(actual interface{}) (success bool, err error) {
	metric, ok := actual.(prometheus.Metric)
	if !ok {
		return false, fmt.Errorf("PrometheusMetric matcher expects a prometheus.Metric")
	}

	actualMetric := &dto.Metric{}
	metric.Write(actualMetric)

	return reflect.DeepEqual(actualMetric.String(), matcher.Expected.String()), nil
}

func (matcher *PrometheusMetricMatcher) FailureMessage(actual interface{}) (message string) {
	metric, ok := actual.(prometheus.Metric)
	if ok {
		actualMetric := &dto.Metric{}
		metric.Write(actualMetric)
		return format.Message(actualMetric.String(), "to equal", matcher.Expected.String())
	}

	return format.Message(actual, "to equal", matcher.Expected)
}

func (matcher *PrometheusMetricMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "not to equal", matcher.Expected)
}

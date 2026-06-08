// Copyright The Prometheus Authors
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
	"errors"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

type dupCollector struct {
	desc *prometheus.Desc
}

func newDupCollector() dupCollector {
	return dupCollector{
		desc: prometheus.NewDesc("test_metric", "help", nil, prometheus.Labels{"project_id": "p"}),
	}
}

func (c dupCollector) Describe(chan<- *prometheus.Desc) {}

func (c dupCollector) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, 1)
}

func registryWithDuplicateSeries(t *testing.T) *prometheus.Registry {
	t.Helper()

	registry := prometheus.NewRegistry()
	if err := registry.Register(newDupCollector()); err != nil {
		t.Fatalf("register first collector: %v", err)
	}
	if err := registry.Register(newDupCollector()); err != nil {
		t.Fatalf("register second collector: %v", err)
	}
	return registry
}

func duplicateSeriesError() error {
	return errors.New(`collected metric "test_metric" { label:{name:"project_id" value:"p"} gauge:{value:1}} ` + duplicateSeriesErrorMessage)
}

func TestIgnoreDuplicatesGatherer(t *testing.T) {
	t.Parallel()

	families, err := IgnoreDuplicatesGatherer(registryWithDuplicateSeries(t)).Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v, want nil", err)
	}
	if len(families) != 1 {
		t.Fatalf("Gather() families = %d, want 1", len(families))
	}
	if got := len(families[0].Metric); got != 1 {
		t.Fatalf("Gather() metrics = %d, want duplicate collapsed to 1", got)
	}
}

func TestSuppressDuplicateSeriesErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		err         error
		wantPresent string
		wantAbsent  string
	}{
		{"nil", nil, "", ""},
		{"single duplicate", duplicateSeriesError(), "", ""},
		{"all duplicates", prometheus.MultiError{duplicateSeriesError(), duplicateSeriesError()}, "", ""},
		{"real error", errors.New("boom"), "boom", ""},
		{"help mismatch", errors.New(`collected metric "x" has help "a" but should have "b"`), "has help", ""},
		{"mixed", prometheus.MultiError{duplicateSeriesError(), errors.New("boom")}, "boom", duplicateSeriesErrorMessage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := suppressDuplicateSeriesErrors(tt.err)
			if tt.wantPresent == "" {
				if got != nil {
					t.Fatalf("suppressDuplicateSeriesErrors() = %v, want nil", got)
				}
				return
			}
			if got == nil || !strings.Contains(got.Error(), tt.wantPresent) {
				t.Fatalf("suppressDuplicateSeriesErrors() = %v, want %q kept", got, tt.wantPresent)
			}
			if tt.wantAbsent != "" && strings.Contains(got.Error(), tt.wantAbsent) {
				t.Fatalf("suppressDuplicateSeriesErrors() = %v, want %q dropped", got, tt.wantAbsent)
			}
		})
	}
}

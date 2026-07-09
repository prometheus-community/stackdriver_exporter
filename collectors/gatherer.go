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
	"fmt"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

const duplicateSeriesErrorMessage = "was collected before with the same name and label values"

// GathererFor builds a gatherer from the given collectors. when IgnoreDuplicates
// is set it also drops duplicate series that Cloud Monitoring returns.
func (r *Runtime) GathererFor(cs []*MonitoringCollector) (prometheus.Gatherer, error) {
	registry := prometheus.NewRegistry()
	for _, c := range cs {
		if err := registry.Register(c); err != nil {
			return nil, fmt.Errorf("register collector: %w", err)
		}
	}
	if r.cfg.IgnoreDuplicates {
		return IgnoreDuplicatesGatherer(registry), nil
	}
	return registry, nil
}

// IgnoreDuplicatesGatherer drops the error that shows up when Cloud Monitoring
// returns the same series twice. the series that are left still get gathered,
// and anything that differs by timestamp stays separate.
func IgnoreDuplicatesGatherer(gatherer prometheus.Gatherer) prometheus.Gatherer {
	return prometheus.GathererFunc(func() ([]*dto.MetricFamily, error) {
		families, err := gatherer.Gather()
		return families, suppressDuplicateSeriesErrors(err)
	})
}

func suppressDuplicateSeriesErrors(err error) error {
	if err == nil {
		return nil
	}

	var multiErr prometheus.MultiError
	if !errors.As(err, &multiErr) {
		if isDuplicateSeriesError(err) {
			return nil
		}
		return err
	}

	remaining := prometheus.MultiError{}
	for _, e := range multiErr {
		if isDuplicateSeriesError(e) {
			continue
		}
		remaining = append(remaining, e)
	}
	return remaining.MaybeUnwrap()
}

func isDuplicateSeriesError(err error) bool {
	return strings.Contains(err.Error(), duplicateSeriesErrorMessage)
}

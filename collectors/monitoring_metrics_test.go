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

import "testing"

func TestNormalizeMetricName(t *testing.T) {
	t.Parallel()

	got := normalizeMetricName("This_is__a-MetricName.Example/with:0totals")
	want := "this_is_a_metric_name_example_with_0_totals"
	if got != want {
		t.Fatalf("normalizeMetricName() = %q, want %q", got, want)
	}
}

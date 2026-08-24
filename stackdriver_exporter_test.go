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

package main

import (
	"testing"
	"time"

	"github.com/alecthomas/kingpin/v2"

	"github.com/prometheus-community/stackdriver_exporter/config"
)

func TestCollectorConfigFromFlagsIncludesProjectsRefreshInterval(t *testing.T) {
	if _, err := kingpin.CommandLine.Parse([]string{
		"--google.projects.filter-refresh-interval=2m",
	}); err != nil {
		t.Fatalf("kingpin.CommandLine.Parse() error = %v", err)
	}

	cfg := collectorConfigFromFlags()
	want := 2 * time.Minute
	if cfg.ProjectsRefreshInterval != want {
		t.Fatalf("ProjectsRefreshInterval = %v, want %v", cfg.ProjectsRefreshInterval, want)
	}
}

func TestCollectorConfigFromFlagsDefaultsProjectsRefreshIntervalToDisabled(t *testing.T) {
	if _, err := kingpin.CommandLine.Parse([]string{}); err != nil {
		t.Fatalf("kingpin.CommandLine.Parse() error = %v", err)
	}

	cfg := collectorConfigFromFlags()
	if cfg.ProjectsRefreshInterval != config.DefaultProjectsRefreshInterval {
		t.Fatalf("ProjectsRefreshInterval = %v, want default %v", cfg.ProjectsRefreshInterval, config.DefaultProjectsRefreshInterval)
	}
}

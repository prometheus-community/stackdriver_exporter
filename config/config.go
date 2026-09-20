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

// Package config is the pure value-type definition of the stackdriver_exporter
// configuration. It has no dependencies on the collectors package or any GCP
// client libraries.
package config

import (
	"fmt"
	"net/http"
	"time"
)

const (
	DefaultUniverseDomain       = "googleapis.com"
	DefaultMaxRetries           = 0
	DefaultHTTPTimeout          = 10 * time.Second
	DefaultMaxBackoff           = 5 * time.Second
	DefaultBackoffJitter        = 1 * time.Second
	DefaultMetricsInterval      = 5 * time.Minute
	DefaultMetricsOffset        = 0 * time.Second
	DefaultMetricsIngest        = false
	DefaultFillMissing          = true
	DefaultDropDelegated        = false
	DefaultAggregateDeltas      = false
	DefaultDeltasTTL            = 30 * time.Minute
	DefaultDescriptorTTL        = 0 * time.Second
	DefaultDescriptorGoogleOnly = true
)

// DefaultRetryStatuses must be treated as immutable after declaration.
var DefaultRetryStatuses = []int{http.StatusServiceUnavailable}

type Config struct {
	ProjectIDs                []string
	ProjectsFilter            string
	UniverseDomain            string
	MaxRetries                int
	HTTPTimeout               time.Duration
	MaxBackoff                time.Duration
	BackoffJitter             time.Duration
	RetryStatuses             []int
	MetricsPrefixes           []string
	MetricsInterval           time.Duration
	MetricsOffset             time.Duration
	MetricsIngestDelay        bool
	FillMissingLabels         bool
	DropDelegatedProjects     bool
	Filters                   []string
	AggregateDeltas           bool
	AggregateDeltasTTL        time.Duration
	DescriptorCacheTTL        time.Duration
	DescriptorCacheOnlyGoogle bool

	// validated is set by Validate on success.
	validated bool
}

// NewConfigWithDefaults returns a Config populated with package defaults. Fields
// without a default (project IDs, metrics prefixes, filters) are left zero and
// must be set by the caller before Validate succeeds.
func NewConfigWithDefaults() *Config {
	return &Config{
		UniverseDomain:            DefaultUniverseDomain,
		MaxRetries:                DefaultMaxRetries,
		HTTPTimeout:               DefaultHTTPTimeout,
		MaxBackoff:                DefaultMaxBackoff,
		BackoffJitter:             DefaultBackoffJitter,
		RetryStatuses:             append([]int(nil), DefaultRetryStatuses...),
		MetricsInterval:           DefaultMetricsInterval,
		MetricsOffset:             DefaultMetricsOffset,
		MetricsIngestDelay:        DefaultMetricsIngest,
		FillMissingLabels:         DefaultFillMissing,
		DropDelegatedProjects:     DefaultDropDelegated,
		AggregateDeltas:           DefaultAggregateDeltas,
		AggregateDeltasTTL:        DefaultDeltasTTL,
		DescriptorCacheTTL:        DefaultDescriptorTTL,
		DescriptorCacheOnlyGoogle: DefaultDescriptorGoogleOnly,
	}
}

// Validate reports configuration errors that prevent the exporter from starting
// and marks the Config as validated.
func (c *Config) Validate() error {
	if len(c.MetricsPrefixes) == 0 {
		return fmt.Errorf("metrics_prefixes must have at least one entry")
	}
	c.validated = true
	return nil
}

// Validated reports whether Validate has been called successfully on c.
func (c *Config) Validated() bool {
	return c.validated
}

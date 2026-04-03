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

package otelcollector

import (
	"fmt"
	"slices"
	"time"

	"github.com/prometheus-community/stackdriver_exporter/config"
	prombridge "github.com/prometheus/opentelemetry-collector-bridge"
)

// Config maps stackdriver_exporter runtime settings into exporter_config.
type Config struct {
	ProjectIDs           []string `mapstructure:"project_ids"`
	ProjectsFilter       string   `mapstructure:"projects_filter"`
	UniverseDomain       string   `mapstructure:"universe_domain"`
	MaxRetries           int      `mapstructure:"max_retries"`
	HTTPTimeout          string   `mapstructure:"http_timeout"`
	MaxBackoff           string   `mapstructure:"max_backoff"`
	BackoffJitter        string   `mapstructure:"backoff_jitter"`
	RetryStatuses        []int    `mapstructure:"retry_statuses"`
	MetricsPrefixes      []string `mapstructure:"metrics_prefixes"`
	MetricsInterval      string   `mapstructure:"metrics_interval"`
	MetricsOffset        string   `mapstructure:"metrics_offset"`
	MetricsIngest        bool     `mapstructure:"metrics_ingest_delay"`
	FillMissing          bool     `mapstructure:"fill_missing_labels"`
	DropDelegated        bool     `mapstructure:"drop_delegated_projects"`
	Filters              []string `mapstructure:"filters"`
	AggregateDeltas      bool     `mapstructure:"aggregate_deltas"`
	DeltasTTL            string   `mapstructure:"aggregate_deltas_ttl"`
	DescriptorTTL        string   `mapstructure:"descriptor_cache_ttl"`
	DescriptorGoogleOnly bool     `mapstructure:"descriptor_cache_only_google"`
}

var _ prombridge.Config = (*Config)(nil)

func defaultConfig() *Config {
	return &Config{
		UniverseDomain:       config.DefaultUniverseDomain,
		MaxRetries:           config.DefaultMaxRetries,
		HTTPTimeout:          config.DefaultHTTPTimeout,
		MaxBackoff:           config.DefaultMaxBackoff,
		BackoffJitter:        config.DefaultBackoffJitter,
		RetryStatuses:        slices.Clone(config.DefaultRetryStatuses),
		MetricsInterval:      config.DefaultMetricsInterval,
		MetricsOffset:        config.DefaultMetricsOffset,
		MetricsIngest:        config.DefaultMetricsIngest,
		FillMissing:          config.DefaultFillMissing,
		DropDelegated:        config.DefaultDropDelegated,
		AggregateDeltas:      config.DefaultAggregateDeltas,
		DeltasTTL:            config.DefaultDeltasTTL,
		DescriptorTTL:        config.DefaultDescriptorTTL,
		DescriptorGoogleOnly: config.DefaultDescriptorGoogleOnly,
	}
}

func defaultComponentDefaults() map[string]interface{} {
	cfg := defaultConfig()
	return map[string]interface{}{
		"max_retries":                  cfg.MaxRetries,
		"http_timeout":                 cfg.HTTPTimeout,
		"max_backoff":                  cfg.MaxBackoff,
		"backoff_jitter":               cfg.BackoffJitter,
		"retry_statuses":               cfg.RetryStatuses,
		"universe_domain":              cfg.UniverseDomain,
		"metrics_interval":             cfg.MetricsInterval,
		"metrics_offset":               cfg.MetricsOffset,
		"metrics_ingest_delay":         cfg.MetricsIngest,
		"fill_missing_labels":          cfg.FillMissing,
		"drop_delegated_projects":      cfg.DropDelegated,
		"aggregate_deltas":             cfg.AggregateDeltas,
		"aggregate_deltas_ttl":         cfg.DeltasTTL,
		"descriptor_cache_ttl":         cfg.DescriptorTTL,
		"descriptor_cache_only_google": cfg.DescriptorGoogleOnly,
	}
}

func (c *Config) Validate() error {
	if len(c.MetricsPrefixes) == 0 {
		return fmt.Errorf("metrics_prefixes must have at least one entry")
	}

	_, err := c.parsedDurations()
	if err != nil {
		return err
	}

	if err := config.ValidateRetryStatuses(c.RetryStatuses); err != nil {
		return err
	}

	return nil
}

type parsedConfig struct {
	HTTPTimeout     time.Duration
	MaxBackoff      time.Duration
	BackoffJitter   time.Duration
	MetricsInterval time.Duration
	MetricsOffset   time.Duration
	DeltasTTL       time.Duration
	DescriptorTTL   time.Duration
}

func (c *Config) parsedDurations() (parsedConfig, error) {
	httpTimeout, err := config.ParseDuration("http_timeout", c.HTTPTimeout)
	if err != nil {
		return parsedConfig{}, err
	}
	maxBackoff, err := config.ParseDuration("max_backoff", c.MaxBackoff)
	if err != nil {
		return parsedConfig{}, err
	}
	backoffJitter, err := config.ParseDuration("backoff_jitter", c.BackoffJitter)
	if err != nil {
		return parsedConfig{}, err
	}
	metricsInterval, err := config.ParseDuration("metrics_interval", c.MetricsInterval)
	if err != nil {
		return parsedConfig{}, err
	}
	metricsOffset, err := config.ParseDuration("metrics_offset", c.MetricsOffset)
	if err != nil {
		return parsedConfig{}, err
	}
	deltasTTL, err := config.ParseDuration("aggregate_deltas_ttl", c.DeltasTTL)
	if err != nil {
		return parsedConfig{}, err
	}
	descriptorTTL, err := config.ParseDuration("descriptor_cache_ttl", c.DescriptorTTL)
	if err != nil {
		return parsedConfig{}, err
	}

	return parsedConfig{
		HTTPTimeout:     httpTimeout,
		MaxBackoff:      maxBackoff,
		BackoffJitter:   backoffJitter,
		MetricsInterval: metricsInterval,
		MetricsOffset:   metricsOffset,
		DeltasTTL:       deltasTTL,
		DescriptorTTL:   descriptorTTL,
	}, nil
}

func (c *Config) runtimeConfig() (config.RuntimeConfig, error) {
	parsed, err := c.parsedDurations()
	if err != nil {
		return config.RuntimeConfig{}, err
	}

	return config.RuntimeConfig{
		ProjectIDs:           slices.Clone(c.ProjectIDs),
		ProjectsFilter:       c.ProjectsFilter,
		UniverseDomain:       c.UniverseDomain,
		MaxRetries:           c.MaxRetries,
		HTTPTimeout:          parsed.HTTPTimeout,
		MaxBackoff:           parsed.MaxBackoff,
		BackoffJitter:        parsed.BackoffJitter,
		RetryStatuses:        slices.Clone(c.RetryStatuses),
		MetricsPrefixes:      slices.Clone(c.MetricsPrefixes),
		MetricsInterval:      parsed.MetricsInterval,
		MetricsOffset:        parsed.MetricsOffset,
		MetricsIngest:        c.MetricsIngest,
		FillMissing:          c.FillMissing,
		DropDelegated:        c.DropDelegated,
		Filters:              slices.Clone(c.Filters),
		AggregateDeltas:      c.AggregateDeltas,
		DeltasTTL:            parsed.DeltasTTL,
		DescriptorTTL:        parsed.DescriptorTTL,
		DescriptorGoogleOnly: c.DescriptorGoogleOnly,
	}, nil
}

type configUnmarshaler struct{}

func (configUnmarshaler) GetConfigStruct() prombridge.Config {
	return defaultConfig()
}

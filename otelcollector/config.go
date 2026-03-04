package otelcollector

import (
	"fmt"
	"net/http"
	"time"

	prombridge "github.com/ArthurSens/prometheus-collector-bridge"
)

// Config maps stackdriver_exporter runtime settings into exporter_config.
type Config struct {
	ProjectIDs      []string `mapstructure:"project_ids"`
	ProjectsFilter  string   `mapstructure:"projects_filter"`
	UniverseDomain  string   `mapstructure:"universe_domain"`
	MaxRetries      int      `mapstructure:"max_retries"`
	HTTPTimeout     string   `mapstructure:"http_timeout"`
	MaxBackoff      string   `mapstructure:"max_backoff"`
	BackoffJitter   string   `mapstructure:"backoff_jitter"`
	RetryStatuses   []int    `mapstructure:"retry_statuses"`
	MetricsPrefixes []string `mapstructure:"metrics_prefixes"`
	MetricsInterval string   `mapstructure:"metrics_interval"`
	MetricsOffset   string   `mapstructure:"metrics_offset"`
	MetricsIngest   bool     `mapstructure:"metrics_ingest_delay"`
	FillMissing     bool     `mapstructure:"fill_missing_labels"`
	DropDelegated   bool     `mapstructure:"drop_delegated_projects"`
	Filters         []string `mapstructure:"filters"`
	AggregateDeltas bool     `mapstructure:"aggregate_deltas"`
	DeltasTTL       string   `mapstructure:"aggregate_deltas_ttl"`
	DescriptorTTL   string   `mapstructure:"descriptor_cache_ttl"`
	DescriptorGoogleOnly bool `mapstructure:"descriptor_cache_only_google"`
}

var _ prombridge.Config = (*Config)(nil)

func defaultConfig() *Config {
	return &Config{
		UniverseDomain:      "googleapis.com",
		MaxRetries:          0,
		HTTPTimeout:         "10s",
		MaxBackoff:          "5s",
		BackoffJitter:       "1s",
		RetryStatuses:       []int{503},
		MetricsInterval:     "5m",
		MetricsOffset:       "0s",
		MetricsIngest:       false,
		FillMissing:         true,
		DropDelegated:       false,
		AggregateDeltas:     false,
		DeltasTTL:           "30m",
		DescriptorTTL:       "0s",
		DescriptorGoogleOnly: true,
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

	for _, code := range c.RetryStatuses {
		if code < http.StatusContinue || code > 599 {
			return fmt.Errorf("retry status %d is not a valid HTTP status code", code)
		}
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
	parse := func(name, raw string) (time.Duration, error) {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return 0, fmt.Errorf("%s: invalid duration %q: %w", name, raw, err)
		}
		return d, nil
	}

	httpTimeout, err := parse("http_timeout", c.HTTPTimeout)
	if err != nil {
		return parsedConfig{}, err
	}
	maxBackoff, err := parse("max_backoff", c.MaxBackoff)
	if err != nil {
		return parsedConfig{}, err
	}
	backoffJitter, err := parse("backoff_jitter", c.BackoffJitter)
	if err != nil {
		return parsedConfig{}, err
	}
	metricsInterval, err := parse("metrics_interval", c.MetricsInterval)
	if err != nil {
		return parsedConfig{}, err
	}
	metricsOffset, err := parse("metrics_offset", c.MetricsOffset)
	if err != nil {
		return parsedConfig{}, err
	}
	deltasTTL, err := parse("aggregate_deltas_ttl", c.DeltasTTL)
	if err != nil {
		return parsedConfig{}, err
	}
	descriptorTTL, err := parse("descriptor_cache_ttl", c.DescriptorTTL)
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

type configUnmarshaler struct{}

func (configUnmarshaler) GetConfigStruct() prombridge.Config {
	return defaultConfig()
}


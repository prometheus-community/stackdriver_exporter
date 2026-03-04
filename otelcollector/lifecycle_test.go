package otelcollector

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/prometheus-community/stackdriver_exporter/collectors"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/api/monitoring/v3"
)

func TestLifecycleManager_Start(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		ProjectIDs:      []string{"project-a", "project-b"},
		MetricsPrefixes: []string{"compute.googleapis.com/instance"},
		HTTPTimeout:     "10s",
		MaxBackoff:      "5s",
		BackoffJitter:   "1s",
		MetricsInterval: "5m",
		MetricsOffset:   "0s",
		DeltasTTL:       "30m",
		DescriptorTTL:   "0s",
	}

	var createdProjects []string
	mgr := newLifecycleManager(slog.Default())
	mgr.monitoringServiceFactory = func(context.Context, parsedConfig, *Config) (*monitoring.Service, error) {
		return &monitoring.Service{}, nil
	}
	mgr.collectorFactory = func(projectID string, _ *monitoring.Service, _ collectors.MonitoringCollectorOptions, _ time.Duration, _ *slog.Logger) (prometheus.Collector, error) {
		createdProjects = append(createdProjects, projectID)
		return prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_metric_" + strings.ReplaceAll(projectID, "-", "_"),
			Help: "test",
		}), nil
	}

	reg, err := mgr.Start(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if reg == nil {
		t.Fatal("Start() returned nil registry")
	}
	if len(createdProjects) != 2 {
		t.Fatalf("collectorFactory called %d times, want 2", len(createdProjects))
	}

	// Ensure the created registry can gather metrics.
	if _, err := reg.Gather(); err != nil {
		t.Fatalf("registry.Gather() error = %v", err)
	}
}

func TestLifecycleManager_Start_UsesDefaultProjectDiscovery(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		MetricsPrefixes: []string{"compute.googleapis.com/instance"},
		HTTPTimeout:     "10s",
		MaxBackoff:      "5s",
		BackoffJitter:   "1s",
		MetricsInterval: "5m",
		MetricsOffset:   "0s",
		DeltasTTL:       "30m",
		DescriptorTTL:   "0s",
	}

	mgr := newLifecycleManager(slog.Default())
	mgr.defaultProjectDiscoverer = func(context.Context) (string, error) {
		return "auto-project", nil
	}
	mgr.monitoringServiceFactory = func(context.Context, parsedConfig, *Config) (*monitoring.Service, error) {
		return &monitoring.Service{}, nil
	}
	mgr.collectorFactory = func(projectID string, _ *monitoring.Service, _ collectors.MonitoringCollectorOptions, _ time.Duration, _ *slog.Logger) (prometheus.Collector, error) {
		if projectID != "auto-project" {
			t.Fatalf("projectID = %q, want %q", projectID, "auto-project")
		}
		return prometheus.NewGauge(prometheus.GaugeOpts{Name: "auto_project_metric", Help: "test"}), nil
	}

	if _, err := mgr.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
}

func TestLifecycleManager_Start_ReturnsErrorFromCollectorFactory(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		ProjectIDs:      []string{"project-a"},
		MetricsPrefixes: []string{"compute.googleapis.com/instance"},
		HTTPTimeout:     "10s",
		MaxBackoff:      "5s",
		BackoffJitter:   "1s",
		MetricsInterval: "5m",
		MetricsOffset:   "0s",
		DeltasTTL:       "30m",
		DescriptorTTL:   "0s",
	}

	mgr := newLifecycleManager(slog.Default())
	mgr.monitoringServiceFactory = func(context.Context, parsedConfig, *Config) (*monitoring.Service, error) {
		return &monitoring.Service{}, nil
	}
	mgr.collectorFactory = func(string, *monitoring.Service, collectors.MonitoringCollectorOptions, time.Duration, *slog.Logger) (prometheus.Collector, error) {
		return nil, errors.New("boom")
	}

	if _, err := mgr.Start(context.Background(), cfg); err == nil {
		t.Fatal("Start() expected error, got nil")
	}
}

func TestLifecycleManager_Shutdown(t *testing.T) {
	t.Parallel()

	mgr := newLifecycleManager(slog.Default())
	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

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
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/prometheus-community/stackdriver_exporter/collectors"
	"github.com/prometheus-community/stackdriver_exporter/config"
	"github.com/prometheus-community/stackdriver_exporter/delta"
	"github.com/prometheus-community/stackdriver_exporter/utils"
	"github.com/prometheus/client_golang/prometheus"
	prombridge "github.com/prometheus/opentelemetry-collector-bridge"
	"go.opentelemetry.io/collector/receiver"
	"go.uber.org/zap/exp/zapslog"
	"google.golang.org/api/monitoring/v3"
)

type collectorFactoryFunc func(projectID string, service *monitoring.Service, opts collectors.MonitoringCollectorOptions, deltasTTL time.Duration, logger *slog.Logger) (prometheus.Collector, error)

type lifecycleManager struct {
	monitoringServiceFactory func(ctx context.Context, cfg config.RuntimeConfig) (*monitoring.Service, error)
	collectorFactory         collectorFactoryFunc
	filterProjectDiscoverer  func(ctx context.Context, filter string) ([]string, error)
	defaultProjectDiscoverer func(ctx context.Context) (string, error)
	loggerForSettings        func(set receiver.Settings) *slog.Logger
}

func newLifecycleManager() *lifecycleManager {
	return &lifecycleManager{
		monitoringServiceFactory: func(ctx context.Context, cfg config.RuntimeConfig) (*monitoring.Service, error) {
			return cfg.CreateMonitoringService(ctx)
		},
		collectorFactory: func(projectID string, service *monitoring.Service, opts collectors.MonitoringCollectorOptions, deltasTTL time.Duration, logger *slog.Logger) (prometheus.Collector, error) {
			return collectors.NewMonitoringCollector(
				projectID,
				service,
				opts,
				logger,
				delta.NewInMemoryCounterStore(logger, deltasTTL),
				delta.NewInMemoryHistogramStore(logger, deltasTTL),
			)
		},
		filterProjectDiscoverer:  utils.GetProjectIDsFromFilter,
		defaultProjectDiscoverer: config.DiscoverDefaultProjectID,
		loggerForSettings:        collectorSlogLogger,
	}
}

func (m *lifecycleManager) Start(ctx context.Context, set receiver.Settings, exporterConfig prombridge.Config) (*prometheus.Registry, error) {
	cfg, ok := exporterConfig.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid exporter config type: %T", exporterConfig)
	}

	runtimeCfg, err := cfg.runtimeConfig()
	if err != nil {
		return nil, err
	}

	projectIDs, err := m.resolveProjectIDs(ctx, cfg)
	if err != nil {
		return nil, err
	}

	monitoringService, err := m.monitoringServiceFactory(ctx, runtimeCfg)
	if err != nil {
		return nil, err
	}

	registry := prometheus.NewRegistry()
	logger := m.loggerForSettings(set)

	for _, projectID := range projectIDs {
		collector, err := m.collectorFactory(projectID, monitoringService, runtimeCfg.MonitoringCollectorOptions(), runtimeCfg.DeltasTTL, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create collector for project %q: %w", projectID, err)
		}
		if err := registry.Register(collector); err != nil {
			return nil, fmt.Errorf("failed to register collector for project %q: %w", projectID, err)
		}
	}

	return registry, nil
}

func (m *lifecycleManager) Shutdown(context.Context) error {
	return nil
}

func collectorSlogLogger(set receiver.Settings) *slog.Logger {
	if set.Logger == nil {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	return slog.New(zapslog.NewHandler(set.Logger.Core()))
}

func (m *lifecycleManager) resolveProjectIDs(ctx context.Context, cfg *Config) ([]string, error) {
	var projectIDs []string

	if cfg.ProjectsFilter != "" {
		ids, err := m.filterProjectDiscoverer(ctx, cfg.ProjectsFilter)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve project IDs from projects_filter: %w", err)
		}
		projectIDs = append(projectIDs, ids...)
	}

	projectIDs = append(projectIDs, cfg.ProjectIDs...)

	if len(projectIDs) == 0 {
		projectID, err := m.defaultProjectDiscoverer(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to discover default GCP project: %w", err)
		}
		projectIDs = append(projectIDs, projectID)
	}

	return config.DeduplicateProjectIDs(projectIDs), nil
}

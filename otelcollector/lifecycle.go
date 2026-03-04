package otelcollector

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	prombridge "github.com/ArthurSens/prometheus-collector-bridge"
	"github.com/PuerkitoBio/rehttp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus-community/stackdriver_exporter/collectors"
	"github.com/prometheus-community/stackdriver_exporter/delta"
	"github.com/prometheus-community/stackdriver_exporter/utils"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/monitoring/v3"
	"google.golang.org/api/option"
)

type collectorFactoryFunc func(projectID string, service *monitoring.Service, opts collectors.MonitoringCollectorOptions, deltasTTL time.Duration, logger *slog.Logger) (prometheus.Collector, error)

type lifecycleManager struct {
	logger *slog.Logger

	monitoringServiceFactory func(ctx context.Context, parsed parsedConfig, cfg *Config) (*monitoring.Service, error)
	collectorFactory         collectorFactoryFunc
	filterProjectDiscoverer  func(ctx context.Context, filter string) ([]string, error)
	defaultProjectDiscoverer func(ctx context.Context) (string, error)
}

func newLifecycleManager(logger *slog.Logger) *lifecycleManager {
	return &lifecycleManager{
		logger: logger,
		monitoringServiceFactory: createMonitoringService,
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
		defaultProjectDiscoverer: discoverDefaultProjectID,
	}
}

func (m *lifecycleManager) Start(ctx context.Context, exporterConfig prombridge.Config) (*prometheus.Registry, error) {
	cfg, ok := exporterConfig.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid exporter config type: %T", exporterConfig)
	}

	parsed, err := cfg.parsedDurations()
	if err != nil {
		return nil, err
	}

	projectIDs, err := m.resolveProjectIDs(ctx, cfg)
	if err != nil {
		return nil, err
	}

	monitoringService, err := m.monitoringServiceFactory(ctx, parsed, cfg)
	if err != nil {
		return nil, err
	}

	registry := prometheus.NewRegistry()
	metricPrefixes := parseMetricTypePrefixes(cfg.MetricsPrefixes)
	extraFilters := parseMetricExtraFilters(cfg.Filters)

	for _, projectID := range projectIDs {
		opts := collectors.MonitoringCollectorOptions{
			MetricTypePrefixes:        metricPrefixes,
			ExtraFilters:              extraFilters,
			RequestInterval:           parsed.MetricsInterval,
			RequestOffset:             parsed.MetricsOffset,
			IngestDelay:               cfg.MetricsIngest,
			FillMissingLabels:         cfg.FillMissing,
			DropDelegatedProjects:     cfg.DropDelegated,
			AggregateDeltas:           cfg.AggregateDeltas,
			DescriptorCacheTTL:        parsed.DescriptorTTL,
			DescriptorCacheOnlyGoogle: cfg.DescriptorGoogleOnly,
		}

		collector, err := m.collectorFactory(projectID, monitoringService, opts, parsed.DeltasTTL, m.logger)
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

	slices.Sort(projectIDs)
	return slices.Compact(projectIDs), nil
}

func discoverDefaultProjectID(ctx context.Context) (string, error) {
	credentials, err := google.FindDefaultCredentials(ctx, compute.ComputeScope)
	if err != nil {
		return "", err
	}
	if credentials.ProjectID == "" {
		return "", fmt.Errorf("unable to identify default GCP project")
	}
	return credentials.ProjectID, nil
}

func createMonitoringService(ctx context.Context, parsed parsedConfig, cfg *Config) (*monitoring.Service, error) {
	googleClient, err := google.DefaultClient(ctx, monitoring.MonitoringReadScope)
	if err != nil {
		return nil, fmt.Errorf("error creating Google client: %w", err)
	}

	googleClient.Timeout = parsed.HTTPTimeout
	googleClient.Transport = rehttp.NewTransport(
		googleClient.Transport,
		rehttp.RetryAll(
			rehttp.RetryMaxRetries(cfg.MaxRetries),
			rehttp.RetryStatuses(cfg.RetryStatuses...),
		),
		rehttp.ExpJitterDelay(parsed.BackoffJitter, parsed.MaxBackoff),
	)

	service, err := monitoring.NewService(ctx, option.WithHTTPClient(googleClient), option.WithUniverseDomain(cfg.UniverseDomain))
	if err != nil {
		return nil, fmt.Errorf("error creating Google Stackdriver Monitoring service: %w", err)
	}
	return service, nil
}

func parseMetricTypePrefixes(input []string) []string {
	in := append([]string(nil), input...)
	slices.Sort(in)
	unique := slices.Compact(in)
	out := make([]string, 0, len(unique))

	for i, prefix := range unique {
		if i > 0 && len(out) > 0 {
			prev := out[len(out)-1]
			if strings.HasPrefix(prefix, prev) {
				continue
			}
		}
		out = append(out, prefix)
	}
	return out
}

func parseMetricExtraFilters(raw []string) []collectors.MetricFilter {
	out := make([]collectors.MetricFilter, 0, len(raw))
	for _, entry := range raw {
		prefix, filter := utils.SplitExtraFilter(entry, ":")
		if prefix == "" {
			continue
		}
		out = append(out, collectors.MetricFilter{
			TargetedMetricPrefix: strings.ToLower(prefix),
			FilterQuery:          filter,
		})
	}
	return out
}


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
	"context"
	"fmt"

	"github.com/PuerkitoBio/rehttp"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/monitoring/v3"
	"google.golang.org/api/option"

	"github.com/prometheus-community/stackdriver_exporter/config"
)

func monitoringCollectorOptionsForPrefixes(cfg *config.Config, metricPrefixes []string) MonitoringCollectorOptions {
	return MonitoringCollectorOptions{
		MetricTypePrefixes:        metricPrefixes,
		ExtraFilters:              ParseMetricExtraFilters(cfg.Filters),
		RequestInterval:           cfg.MetricsInterval,
		RequestOffset:             cfg.MetricsOffset,
		IngestDelay:               cfg.MetricsIngestDelay,
		FillMissingLabels:         cfg.FillMissingLabels,
		DropDelegatedProjects:     cfg.DropDelegatedProjects,
		AggregateDeltas:           cfg.AggregateDeltas,
		DescriptorCacheTTL:        cfg.DescriptorCacheTTL,
		DescriptorCacheOnlyGoogle: cfg.DescriptorCacheOnlyGoogle,
	}
}

func createMonitoringService(ctx context.Context, cfg *config.Config) (*monitoring.Service, error) {
	googleClient, err := google.DefaultClient(ctx, monitoring.MonitoringReadScope)
	if err != nil {
		return nil, fmt.Errorf("error creating Google client: %w", err)
	}

	googleClient.Timeout = cfg.HTTPTimeout
	googleClient.Transport = rehttp.NewTransport(
		googleClient.Transport,
		rehttp.RetryAll(
			rehttp.RetryMaxRetries(cfg.MaxRetries),
			rehttp.RetryStatuses(cfg.RetryStatuses...),
		),
		rehttp.ExpJitterDelay(cfg.BackoffJitter, cfg.MaxBackoff),
	)

	service, err := monitoring.NewService(ctx, option.WithHTTPClient(googleClient), option.WithUniverseDomain(cfg.UniverseDomain))
	if err != nil {
		return nil, fmt.Errorf("error creating Google Stackdriver Monitoring service: %w", err)
	}
	return service, nil
}

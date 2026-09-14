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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/option"

	"github.com/prometheus-community/stackdriver_exporter/config"
)

func TestNewRuntimeRequiresValidatedConfig(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{MetricsPrefixes: []string{"compute.googleapis.com/"}}
	_, err := NewRuntime(context.Background(), slog.Default(), cfg, nil, nil)
	if err == nil {
		t.Fatal("expected error for un-Validated config, got nil")
	}
	if !strings.Contains(err.Error(), "validated") {
		t.Fatalf("expected error to mention validation, got %v", err)
	}
}

func TestDeduplicateProjectIDs(t *testing.T) {
	t.Parallel()

	input := []string{"project-b", "project-a", "project-b"}
	want := []string{"project-a", "project-b"}

	got := deduplicateProjectIDs(input)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("deduplicateProjectIDs() = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(input, []string{"project-b", "project-a", "project-b"}) {
		t.Fatalf("deduplicateProjectIDs() mutated input = %#v", input)
	}
}

func TestCollectorCacheTTL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  config.Config
		want time.Duration
	}{
		{
			name: "default fallback",
			cfg:  config.Config{},
			want: 2 * time.Hour,
		},
		{
			name: "aggregate deltas uses deltas ttl",
			cfg: config.Config{
				AggregateDeltas:    true,
				AggregateDeltasTTL: 30 * time.Minute,
			},
			want: 30 * time.Minute,
		},
		{
			name: "descriptor ttl wins when larger",
			cfg: config.Config{
				AggregateDeltas:    true,
				AggregateDeltasTTL: 30 * time.Minute,
				DescriptorCacheTTL: 45 * time.Minute,
			},
			want: 45 * time.Minute,
		},
		{
			name: "descriptor cache alone enables cache ttl",
			cfg: config.Config{
				DescriptorCacheTTL: 15 * time.Minute,
			},
			want: 15 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := collectorCacheTTL(&tt.cfg); got != tt.want {
				t.Fatalf("collectorCacheTTL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCollectorCacheKeyIsOrderIndependent(t *testing.T) {
	t.Parallel()

	a := collectorCacheKey("proj", []string{"compute.googleapis.com/", "pubsub.googleapis.com/"})
	b := collectorCacheKey("proj", []string{"pubsub.googleapis.com/", "compute.googleapis.com/"})
	if a != b {
		t.Fatalf("cache key changed with input order: %q vs %q", a, b)
	}
}

func TestParseMetricTypePrefixes(t *testing.T) {
	t.Parallel()

	input := []string{
		"redis.googleapis.com/stats/memory/usage",
		"loadbalancing.googleapis.com/https/request_count",
		"loadbalancing.googleapis.com",
		"redis.googleapis.com/stats/memory/usage_ratio",
		"redis.googleapis.com/stats/memory/usage_ratio",
	}
	want := []string{
		"loadbalancing.googleapis.com",
		"redis.googleapis.com/stats/memory/usage",
	}

	got := parseMetricTypePrefixes(input)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseMetricTypePrefixes() = %#v, want %#v", got, want)
	}

	wantInput := []string{
		"redis.googleapis.com/stats/memory/usage",
		"loadbalancing.googleapis.com/https/request_count",
		"loadbalancing.googleapis.com",
		"redis.googleapis.com/stats/memory/usage_ratio",
		"redis.googleapis.com/stats/memory/usage_ratio",
	}
	if !reflect.DeepEqual(input, wantInput) {
		t.Fatalf("parseMetricTypePrefixes mutated input = %#v, want %#v", input, wantInput)
	}
}

func TestRuntimeFilterMetricTypePrefixes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		configuredPrefix []string
		prefixFilter     []string
		want             []string
	}{
		{
			name:             "nil filter returns configured prefixes",
			configuredPrefix: []string{"compute.googleapis.com/instance/", "pubsub.googleapis.com/"},
			prefixFilter:     nil,
			want:             []string{"compute.googleapis.com/instance/", "pubsub.googleapis.com/"},
		},
		{
			name:             "filter narrows to matching subprefixes and parse drops shorter overlaps",
			configuredPrefix: []string{"redis.googleapis.com/stats/"},
			prefixFilter: []string{
				"redis.googleapis.com/stats/memory/usage",
				"redis.googleapis.com/stats/memory/usage_ratio",
				"redis.googleapis.com",
			},
			want: []string{"redis.googleapis.com/stats/memory/usage"},
		},
		{
			name:             "filter with no matches returns empty",
			configuredPrefix: []string{"compute.googleapis.com/instance/"},
			prefixFilter:     []string{"pubsub.googleapis.com/topic/foo"},
			want:             []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := &Runtime{cfg: &config.Config{MetricsPrefixes: tt.configuredPrefix}}
			got := r.filterMetricTypePrefixes(tt.prefixFilter)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("filterMetricTypePrefixes() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

// newTestRuntime builds a minimal Runtime for unit testing the project
// discovery refresh path, bypassing NewRuntime's ADC/GCP service setup.
func newTestRuntime(t *testing.T, cfg *config.Config, initialIDs []string, discover func(ctx context.Context, filter string) ([]string, error)) *Runtime {
	t.Helper()
	ptr := &atomic.Pointer[[]string]{}
	ptr.Store(&initialIDs)
	return &Runtime{
		cfg:                cfg,
		projectIDs:         ptr,
		logger:             slog.Default(),
		discoverProjectIDs: discover,
	}
}

// fakeCloudResourceManagerServer serves a canned ListProjectsResponse (or an
// error status) so listProjectIDs can be tested against a real HTTP client
// without depending on Application Default Credentials.
func fakeCloudResourceManagerServer(t *testing.T, statusCode int, resp *cloudresourcemanager.ListProjectsResponse) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if resp != nil {
			_ = json.NewEncoder(w).Encode(resp)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func fakeCloudResourceManagerService(t *testing.T, server *httptest.Server) *cloudresourcemanager.Service {
	t.Helper()
	service, err := cloudresourcemanager.NewService(context.Background(),
		option.WithEndpoint(server.URL),
		option.WithHTTPClient(server.Client()),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("cloudresourcemanager.NewService() error = %v", err)
	}
	return service
}

func TestListProjectIDsReturnsMatchingProjects(t *testing.T) {
	t.Parallel()

	server := fakeCloudResourceManagerServer(t, http.StatusOK, &cloudresourcemanager.ListProjectsResponse{
		Projects: []*cloudresourcemanager.Project{
			{ProjectId: "project-a"},
			{ProjectId: "project-b"},
		},
	})
	service := fakeCloudResourceManagerService(t, server)

	got, err := listProjectIDs(context.Background(), service, "parent.id:12345")
	if err != nil {
		t.Fatalf("listProjectIDs() error = %v", err)
	}
	want := []string{"project-a", "project-b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("listProjectIDs() = %#v, want %#v", got, want)
	}
}

func TestListProjectIDsPropagatesAPIError(t *testing.T) {
	t.Parallel()

	server := fakeCloudResourceManagerServer(t, http.StatusInternalServerError, nil)
	service := fakeCloudResourceManagerService(t, server)

	_, err := listProjectIDs(context.Background(), service, "parent.id:12345")
	if err == nil {
		t.Fatal("listProjectIDs() expected error for 500 response, got nil")
	}
}

func TestRefreshProjectIDsUpdatesListOnSuccess(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{ProjectsFilter: "parent.id:12345"}
	r := newTestRuntime(t, cfg, []string{"old-project"}, func(_ context.Context, _ string) ([]string, error) {
		return []string{"new-project-b", "new-project-a"}, nil
	})

	r.refreshProjectIDs(context.Background())

	got := *r.projectIDs.Load()
	want := []string{"new-project-a", "new-project-b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projectIDs after refresh = %#v, want %#v", got, want)
	}
}

func TestRefreshProjectIDsMergesStaticProjectIDs(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{ProjectsFilter: "parent.id:12345", ProjectIDs: []string{"static-project", "new-project-a"}}
	r := newTestRuntime(t, cfg, []string{"old-project"}, func(_ context.Context, _ string) ([]string, error) {
		return []string{"new-project-a"}, nil
	})

	r.refreshProjectIDs(context.Background())

	got := *r.projectIDs.Load()
	want := []string{"new-project-a", "static-project"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projectIDs after refresh = %#v, want %#v", got, want)
	}
}

func TestRefreshProjectIDsKeepsPreviousListOnAPIError(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{ProjectsFilter: "parent.id:12345"}
	r := newTestRuntime(t, cfg, []string{"old-project"}, func(_ context.Context, _ string) ([]string, error) {
		return nil, errors.New("transient GCP error")
	})

	r.refreshProjectIDs(context.Background())

	got := *r.projectIDs.Load()
	want := []string{"old-project"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projectIDs after failed refresh = %#v, want unchanged %#v", got, want)
	}
}

func TestRefreshProjectIDsKeepsPreviousListWhenFilterReturnsZero(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{ProjectsFilter: "parent.id:12345"}
	r := newTestRuntime(t, cfg, []string{"old-project"}, func(_ context.Context, _ string) ([]string, error) {
		return nil, nil
	})

	r.refreshProjectIDs(context.Background())

	got := *r.projectIDs.Load()
	want := []string{"old-project"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projectIDs after empty-result refresh = %#v, want unchanged %#v", got, want)
	}
}

func TestStartProjectDiscoveryRefreshRunsOnlyWhenFilterAndIntervalSet(t *testing.T) {
	tests := []struct {
		name            string
		filter          string
		interval        time.Duration
		expectTriggered bool
	}{
		{name: "neither filter nor interval set", filter: "", interval: 0, expectTriggered: false},
		{name: "filter set, interval zero", filter: "parent.id:1", interval: 0, expectTriggered: false},
		{name: "interval set, filter empty", filter: "", interval: 10 * time.Millisecond, expectTriggered: false},
		{name: "filter and interval both set", filter: "parent.id:1", interval: 10 * time.Millisecond, expectTriggered: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls int32
			cfg := &config.Config{ProjectsFilter: tt.filter, ProjectsRefreshInterval: tt.interval}
			r := newTestRuntime(t, cfg, []string{"p"}, func(_ context.Context, _ string) ([]string, error) {
				atomic.AddInt32(&calls, 1)
				return []string{"p2"}, nil
			})

			ctx, cancel := context.WithCancel(context.Background())
			r.StartProjectDiscoveryRefresh(ctx)

			time.Sleep(50 * time.Millisecond)
			cancel()
			time.Sleep(20 * time.Millisecond)

			triggered := atomic.LoadInt32(&calls) > 0
			if triggered != tt.expectTriggered {
				t.Fatalf("refresh triggered = %v, want %v (calls=%d)", triggered, tt.expectTriggered, calls)
			}
		})
	}
}

func TestBuildCollectorsSafeUnderConcurrentRefresh(t *testing.T) {
	cfg := &config.Config{MetricsPrefixes: []string{"compute.googleapis.com/"}}
	r := newTestRuntime(t, cfg, []string{"project-a"}, nil)
	r.counterStoreFactory = func(_ *slog.Logger, _ time.Duration) DeltaCounterStore { return nil }
	r.histogramStoreFactory = func(_ *slog.Logger, _ time.Duration) DeltaHistogramStore { return nil }

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			ids := []string{fmt.Sprintf("project-%d", i)}
			r.projectIDs.Store(&ids)
		}
	}()

	for i := 0; i < 200; i++ {
		if _, err := r.buildCollectors(nil); err != nil {
			t.Fatalf("buildCollectors() error = %v", err)
		}
	}
	<-done
}

// TestWithCacheSharesProjectIDsPointer guards against a regression where
// WithCache's shallow struct copy (sibling := *r) would give the cached
// sibling a disconnected snapshot of the project list instead of sharing
// live state with the original Runtime that a background refresh updates.
func TestWithCacheSharesProjectIDsPointer(t *testing.T) {
	cfg := &config.Config{MetricsPrefixes: []string{"compute.googleapis.com/"}}
	r := newTestRuntime(t, cfg, []string{"project-a"}, nil)
	r.counterStoreFactory = func(_ *slog.Logger, _ time.Duration) DeltaCounterStore { return nil }
	r.histogramStoreFactory = func(_ *slog.Logger, _ time.Duration) DeltaHistogramStore { return nil }

	sibling := r.WithCache()

	updated := []string{"project-a", "project-b"}
	r.projectIDs.Store(&updated)

	cs, err := sibling.Collectors()
	if err != nil {
		t.Fatalf("sibling.Collectors() error = %v", err)
	}
	if len(cs) != 2 {
		t.Fatalf("sibling.Collectors() returned %d collectors after original's projectIDs updated, want 2", len(cs))
	}
}

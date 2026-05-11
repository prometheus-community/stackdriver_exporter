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

package config

import "testing"

func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name:    "missing metrics prefixes",
			cfg:     Config{},
			wantErr: true,
		},
		{
			name:    "valid with single prefix",
			cfg:     Config{MetricsPrefixes: []string{"compute.googleapis.com/"}},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewConfigWithDefaults(t *testing.T) {
	t.Parallel()

	c := NewConfigWithDefaults()
	if c.UniverseDomain != DefaultUniverseDomain {
		t.Errorf("UniverseDomain = %q, want %q", c.UniverseDomain, DefaultUniverseDomain)
	}
	if c.HTTPTimeout != DefaultHTTPTimeout {
		t.Errorf("HTTPTimeout = %v, want %v", c.HTTPTimeout, DefaultHTTPTimeout)
	}
	if len(c.RetryStatuses) != len(DefaultRetryStatuses) || c.RetryStatuses[0] != DefaultRetryStatuses[0] {
		t.Errorf("RetryStatuses = %v, want %v", c.RetryStatuses, DefaultRetryStatuses)
	}
	c.RetryStatuses[0] = 999
	if DefaultRetryStatuses[0] == 999 {
		t.Fatal("NewConfigWithDefaults did not copy RetryStatuses; default mutated")
	}
}

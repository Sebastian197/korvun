// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package config

import "testing"

// TestObservabilitySettings covers the deliberate asymmetry with Storage
// (ADR-0020 §4): an ABSENT observability block means the admin server is ON with
// safe loopback defaults (observability is safe and always useful), whereas an
// absent storage block means OFF. The operator disables the server explicitly.
func TestObservabilitySettings(t *testing.T) {
	enabledTrue := true
	enabledFalse := false

	tests := []struct {
		name        string
		cfg         *ObservabilityConfig
		wantEnabled bool
		wantAddr    string
	}{
		{
			name:        "absent block: on with default loopback addr",
			cfg:         nil,
			wantEnabled: true,
			wantAddr:    DefaultObservabilityAddr,
		},
		{
			name:        "present, enabled unset: defaults to on",
			cfg:         &ObservabilityConfig{},
			wantEnabled: true,
			wantAddr:    DefaultObservabilityAddr,
		},
		{
			name:        "explicitly disabled",
			cfg:         &ObservabilityConfig{Enabled: &enabledFalse},
			wantEnabled: false,
			wantAddr:    DefaultObservabilityAddr,
		},
		{
			name:        "custom addr overrides default",
			cfg:         &ObservabilityConfig{Enabled: &enabledTrue, Addr: "0.0.0.0:9999"},
			wantEnabled: true,
			wantAddr:    "0.0.0.0:9999",
		},
		{
			name:        "enabled unset, custom addr",
			cfg:         &ObservabilityConfig{Addr: "127.0.0.1:3000"},
			wantEnabled: true,
			wantAddr:    "127.0.0.1:3000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{Observability: tt.cfg}
			gotEnabled, gotAddr := c.ObservabilitySettings()
			if gotEnabled != tt.wantEnabled {
				t.Errorf("enabled = %v, want %v", gotEnabled, tt.wantEnabled)
			}
			if gotAddr != tt.wantAddr {
				t.Errorf("addr = %q, want %q", gotAddr, tt.wantAddr)
			}
		})
	}
}

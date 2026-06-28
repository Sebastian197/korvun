// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"path/filepath"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
)

// TestExampleConfigsLoad asserts every shipped example config under configs/
// parses and validates (Stage 15 / ADR-0025 §3): the edge and cloud profiles, and
// the pre-existing example/local configs, must always load, so a schema drift can
// never silently break the install story. Secrets are NOT resolved here (that is
// app.Build's job); this guards the config shape only.
func TestExampleConfigsLoad(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("..", "..", "configs", "*.json"))
	if err != nil {
		t.Fatalf("glob configs: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no configs/*.json found; expected at least the example/local/edge/cloud profiles")
	}

	wantPresent := map[string]bool{"edge.json": false, "cloud.json": false}
	for _, path := range matches {
		base := filepath.Base(path)
		t.Run(base, func(t *testing.T) {
			if _, err := config.Load(path); err != nil {
				t.Errorf("config.Load(%s) failed: %v", base, err)
			}
		})
		if _, ok := wantPresent[base]; ok {
			wantPresent[base] = true
		}
	}
	for name, seen := range wantPresent {
		if !seen {
			t.Errorf("expected configs/%s to exist (ADR-0025 §3 profile)", name)
		}
	}
}

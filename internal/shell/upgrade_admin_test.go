// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Red for the admin half of config upgrading (v0.9.1, app-audit 2026-08-23
// finding A4): without an admin block the core mounts NEITHER /api/config
// NOR /builder (app.go "no token => mutation not mounted"), yet the desktop
// embeds the Builder unconditionally — an admin-less config leaves the tab a
// 404 trap. EnsureAdminBlock adds exactly the missing block (token_env
// KORVUN_ADMIN_TOKEN, the shell-managed per-cycle bearer — loopback bind is
// unchanged: the shell's ephemeral override owns WHERE it binds), preserves
// everything else verbatim (unknown fields included), and touches nothing
// when the block is already present. New installs never need it: the
// first-run template ships the block (pinned in firstrun_test.go).
package shell

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
)

// preAdminConfig mirrors the audited real-world shape: a valid config with
// no admin block and an unknown field the upgrade must pass through.
const preAdminConfig = `{
  "channels": [],
  "brains": [
    {
      "name": "default",
      "sensitivity": "private",
      "policy": {"kind": "priority", "order": ["ollama"]},
      "dispatch": "fanout",
      "models": [
        {
          "provider": "ollama",
          "model_id": "llama3.2:1b",
          "locality": "local"
        }
      ]
    }
  ],
  "routes": [],
  "x_future": {"keep": true}
}`

func TestEnsureAdminBlock_addsWhenMissing(t *testing.T) {
	path := writeFixtureConfig(t, preAdminConfig)

	changed, err := EnsureAdminBlock(path)
	if err != nil || !changed {
		t.Fatalf("EnsureAdminBlock = (%v, %v), want (true, nil)", changed, err)
	}

	m := readTopLevel(t, path)
	if _, ok := m["admin"]; !ok {
		t.Fatal("admin block not provisioned")
	}
	if !strings.Contains(string(m["x_future"]), "keep") {
		t.Fatalf("unknown field dropped by the upgrade: x_future = %q", m["x_future"])
	}

	// The upgraded file must load AND validate, with the shell-managed
	// bearer variable named — the value stays per-cycle, never persisted.
	// #nosec G304 -- fixture path is supplied by the test itself, not user input.
	data, _ := os.ReadFile(path)
	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("upgraded config does not parse as config.Config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("upgraded config does not validate: %v", err)
	}
	if cfg.Admin == nil || cfg.Admin.TokenEnv != "KORVUN_ADMIN_TOKEN" {
		t.Fatalf("upgraded config admin = %+v, want token_env KORVUN_ADMIN_TOKEN", cfg.Admin)
	}
}

func TestEnsureAdminBlock_presentBlockUntouched(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "custom token_env survives verbatim",
			body: strings.Replace(preAdminConfig, `"x_future": {"keep": true}`,
				`"x_future": {"keep": true},
  "admin": {"token_env": "MY_OWN_ADMIN_VAR"}`, 1),
		},
		{
			name: "template-named block is not rewritten",
			body: strings.Replace(preAdminConfig, `"x_future": {"keep": true}`,
				`"x_future": {"keep": true},
  "admin": {"token_env": "KORVUN_ADMIN_TOKEN"}`, 1),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeFixtureConfig(t, tt.body)

			changed, err := EnsureAdminBlock(path)
			if err != nil || changed {
				t.Fatalf("EnsureAdminBlock = (%v, %v), want (false, nil)", changed, err)
			}

			// Not even an identical rewrite: bytes stay exactly as written.
			// #nosec G304 -- fixture path is supplied by the test itself.
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read config after no-op upgrade: %v", err)
			}
			if string(after) != tt.body {
				t.Fatalf("no-op upgrade rewrote the file:\n got: %s\nwant: %s", after, tt.body)
			}
		})
	}
}

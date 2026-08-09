// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// parseConfig unmarshals and validates a config JSON, the Load path without
// the file.
func parseConfig(t *testing.T, raw string) (*Config, error) {
	t.Helper()
	var cfg Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// The SP5 agent-block growth (ADR-0041, spec FR-CFG-1): tri-state grants,
// declared attrs overrides, per-tool cages, skills path + body budget — all
// ADDITIVE and structurally validated here; semantic resolution (names
// against the safe toolset, cages against the filesystem) stays in
// internal/app, the established split.

// governedBrainJSON returns a config JSON with the given agent block body.
func governedBrainJSON(agentBlock string) string {
	return `{
  "channels": [{"type": "telegram", "mode": "polling", "token_env": "T"}],
  "brains": [{
    "name": "agent",
    "sensitivity": "private",
    "policy": {"kind": "priority"},
    "models": [{"provider": "ollama", "model_id": "llama3.2", "locality": "local"}],
    "agent": ` + agentBlock + `
  }],
  "routes": [{"channel": "telegram", "brain": "agent"}]
}`
}

func TestValidate_governedAgentBlock_full(t *testing.T) {
	t.Parallel()
	cfg, err := parseConfig(t, governedBrainJSON(`{
    "tools": ["calc", "read_file", "http_fetch", "webhook_call"],
    "max_iterations": 4,
    "governance": [
      {"tool": "calc", "mode": "allow"},
      {"tool": "read_file", "mode": "allow", "channels": ["console"]},
      {"tool": "http_fetch", "mode": "shadow"},
      {"tool": "webhook_call", "mode": "deny"}
    ],
    "tool_attrs": {"http_fetch": {"sensitive": true}},
    "read_file": {"root": "/tmp/jail", "max_bytes": 4096},
    "http_fetch": {"allow_hosts": ["example.com"], "max_bytes": 8192, "max_redirects": 2},
    "webhook_call": {"allow_hosts": ["127.0.0.1:5678"], "timeout_seconds": 5},
    "skills_dir": "./skills",
    "skills_body_budget": 4096
  }`))
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	a := cfg.Brains[0].Agent
	if len(a.Governance) != 4 {
		t.Fatalf("Governance = %+v, want 4 grants", a.Governance)
	}
	if a.Governance[1].Tool != "read_file" || a.Governance[1].Mode != "allow" || a.Governance[1].Channels[0] != "console" {
		t.Fatalf("grant round-trip mangled: %+v", a.Governance[1])
	}
	if s := a.ToolAttrs["http_fetch"].Sensitive; s == nil || !*s {
		t.Fatalf("attrs override round-trip mangled: %+v", a.ToolAttrs)
	}
	if a.ReadFile == nil || a.ReadFile.Root != "/tmp/jail" || a.ReadFile.MaxBytes != 4096 {
		t.Fatalf("read_file cage round-trip mangled: %+v", a.ReadFile)
	}
	if a.HTTPFetch == nil || len(a.HTTPFetch.AllowHosts) != 1 || a.HTTPFetch.MaxRedirects != 2 {
		t.Fatalf("http_fetch cage round-trip mangled: %+v", a.HTTPFetch)
	}
	if a.WebhookCall == nil || a.WebhookCall.TimeoutSeconds != 5 {
		t.Fatalf("webhook_call cage round-trip mangled: %+v", a.WebhookCall)
	}
	if a.SkillsDir != "./skills" || a.SkillsBodyBudget != 4096 {
		t.Fatalf("skills round-trip mangled: dir %q budget %d", a.SkillsDir, a.SkillsBodyBudget)
	}
}

// AS-4 tripwire: yesterday's agent block — no governance, no cages, no
// skills — still validates untouched.
func TestValidate_agentBlockWithoutGovernanceStandsAsIs(t *testing.T) {
	t.Parallel()
	cfg, err := parseConfig(t, governedBrainJSON(
		`{"tools": ["time", "echo", "calc"], "max_iterations": 5}`))
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	a := cfg.Brains[0].Agent
	if a.Governance != nil || a.ToolAttrs != nil || a.ReadFile != nil || a.SkillsDir != "" {
		t.Fatalf("absent blocks must stay absent: %+v", a)
	}
}

func TestValidate_governanceRejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		block string
		want  string // fragment the error must carry (a CLEAR path)
	}{
		{
			"unknown mode",
			`{"tools": ["calc"], "governance": [{"tool": "calc", "mode": "maybe"}]}`,
			"governance[0].mode",
		},
		{
			"missing mode",
			`{"tools": ["calc"], "governance": [{"tool": "calc"}]}`,
			"governance[0].mode",
		},
		{
			"missing tool name",
			`{"tools": ["calc"], "governance": [{"mode": "allow"}]}`,
			"governance[0].tool",
		},
		{
			"duplicate grant",
			`{"tools": ["calc"], "governance": [{"tool": "calc", "mode": "allow"}, {"tool": "calc", "mode": "deny"}]}`,
			"duplicate",
		},
		{
			"blank channel entry",
			`{"tools": ["calc"], "governance": [{"tool": "calc", "mode": "allow", "channels": [""]}]}`,
			"channels",
		},
		{
			"read_file cage without a root",
			`{"tools": ["read_file"], "read_file": {"max_bytes": 10}}`,
			"read_file.root",
		},
		{
			"http_fetch cage without hosts",
			`{"tools": ["http_fetch"], "http_fetch": {"max_bytes": 10}}`,
			"http_fetch.allow_hosts",
		},
		{
			"webhook_call cage without hosts",
			`{"tools": ["webhook_call"], "webhook_call": {}}`,
			"webhook_call.allow_hosts",
		},
		{
			"negative body budget",
			`{"tools": ["calc"], "skills_body_budget": -1}`,
			"skills_body_budget",
		},
		{
			"negative read_file cap",
			`{"tools": ["read_file"], "read_file": {"root": "/x", "max_bytes": -1}}`,
			"read_file.max_bytes",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseConfig(t, governedBrainJSON(tc.block))
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("err = %v, want errors.Is(_, ErrInvalidConfig)", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not carry the clear path %q", err.Error(), tc.want)
			}
		})
	}
}

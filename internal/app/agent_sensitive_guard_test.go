// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
)

// Estreno E-11 (security specialist critical + adversarial H3): without a
// governance block, the sensitive×locality rule did not exist — the gate
// (policy.SelectTools) is only mounted with grants, so an ungoverned agent
// brain with a Sensitive tool on a Cloud model shipped jail content to the
// cloud provider with no check anywhere, while /tools asserted the
// protection. The shield, by contrast, IS armed ungoverned. The asymmetry
// closes with a fail-loud boot guard.

func sensitiveTrue() *bool { b := true; return &b }

func agentBrainCfg(locality string, governed bool) config.BrainConfig {
	bc := config.BrainConfig{
		Name:        "agent",
		Sensitivity: "public",
		Policy:      config.PolicyConfig{Kind: "priority"},
		Models:      []config.ModelConfig{{Provider: "ollama", ModelID: "m", Locality: locality}},
		Agent: &config.AgentConfig{
			Tools:     []string{"calc"},
			ToolAttrs: map[string]config.ToolAttrsConfig{"calc": {Sensitive: sensitiveTrue()}},
		},
	}
	if governed {
		bc.Agent.Governance = []config.ToolGrantConfig{{Tool: "calc", Mode: "allow"}}
	}
	return bc
}

func TestBuildBrain_sensitiveToolOnCloudUngoverned_failsLoud(t *testing.T) {
	t.Parallel()
	_, err := testBuilder().buildBrain(agentBrainCfg("cloud", false))
	if !errors.Is(err, ErrSensitiveToolUngoverned) {
		t.Fatalf("err = %v, want ErrSensitiveToolUngoverned", err)
	}
	if !strings.Contains(err.Error(), "calc") {
		t.Errorf("error %q does not name the sensitive tool", err.Error())
	}
}

func TestBuildBrain_sensitiveToolOnCloudGoverned_builds(t *testing.T) {
	t.Parallel()
	if _, err := testBuilder().buildBrain(agentBrainCfg("cloud", true)); err != nil {
		t.Fatalf("governed cloud brain must build (the gate rules per message): %v", err)
	}
}

func TestBuildBrain_sensitiveToolOnLocalUngoverned_builds(t *testing.T) {
	t.Parallel()
	if _, err := testBuilder().buildBrain(agentBrainCfg("local", false)); err != nil {
		t.Fatalf("local ungoverned brain must build (the rule is about cloud egress): %v", err)
	}
}

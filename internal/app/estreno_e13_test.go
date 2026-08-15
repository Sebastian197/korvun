// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/policy"
	"github.com/Sebastian197/korvun/internal/tool"
)

// Estreno E-13 (testing specialist critical): the shield's ARMING at wiring
// time was never asserted — every shield test set PrivateOnly directly, so a
// regression of the `attrs.Network && sens == Private` predicate would
// silently disarm the shield in production with all hermetic tests green.
// These tests build the tool THROUGH the app wiring and observe the shield
// class at the dial.

func shieldWiringCfg(sensitivity string) config.BrainConfig {
	return config.BrainConfig{
		Name: "b", Sensitivity: sensitivity,
		Policy: config.PolicyConfig{Kind: "priority"},
		Models: []config.ModelConfig{{Provider: "ollama", ModelID: "m", Locality: "local"}},
		Agent: &config.AgentConfig{
			Tools: []string{"http_fetch"},
			HTTPFetch: &config.HTTPFetchToolConfig{
				// TEST-NET-3: public, guaranteed non-private, never routable.
				AllowHosts: []string{"203.0.113.7"},
			},
		},
	}
}

func TestAgentTool_privateBrainArmsTheShield(t *testing.T) {
	t.Parallel()
	bc := shieldWiringCfg("private")
	b := testBuilder()
	attrs, err := effectiveToolAttrs(bc.Agent)
	if err != nil {
		t.Fatalf("attrs: %v", err)
	}
	tl, err := b.agentTool(bc, "http_fetch", attrs, mustSensitivity(t, bc.Sensitivity))
	if err != nil {
		t.Fatalf("agentTool: %v", err)
	}
	// The allow-list PERMITS the public IP; only an armed shield refuses it
	// at the dial. err class is the wiring's observable truth.
	_, execErr := tl.Execute(context.Background(), "http://203.0.113.7/x")
	if !errors.Is(execErr, tool.ErrShieldViolation) {
		t.Fatalf("Execute err = %v, want ErrShieldViolation — the wiring did not arm the shield on a Private brain", execErr)
	}
}

func TestAgentTool_publicBrainDoesNotArmTheShield(t *testing.T) {
	t.Parallel()
	bc := shieldWiringCfg("public")
	b := testBuilder()
	attrs, err := effectiveToolAttrs(bc.Agent)
	if err != nil {
		t.Fatalf("attrs: %v", err)
	}
	tl, err := b.agentTool(bc, "http_fetch", attrs, mustSensitivity(t, bc.Sensitivity))
	if err != nil {
		t.Fatalf("agentTool: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, execErr := tl.Execute(ctx, "http://203.0.113.7/x")
	if errors.Is(execErr, tool.ErrShieldViolation) {
		t.Fatal("public brain dialed through the SHIELD — the wiring armed it where it must not")
	}
}

func TestAgentTool_networkFalseOverrideDisarmsTheShield(t *testing.T) {
	t.Parallel()
	bc := shieldWiringCfg("private")
	f := false
	bc.Agent.ToolAttrs = map[string]config.ToolAttrsConfig{"http_fetch": {Network: &f}}
	b := testBuilder()
	attrs, err := effectiveToolAttrs(bc.Agent)
	if err != nil {
		t.Fatalf("attrs: %v", err)
	}
	tl, err := b.agentTool(bc, "http_fetch", attrs, mustSensitivity(t, bc.Sensitivity))
	if err != nil {
		t.Fatalf("agentTool: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, execErr := tl.Execute(ctx, "http://203.0.113.7/x")
	if errors.Is(execErr, tool.ErrShieldViolation) {
		t.Fatal("network:false override did not disarm the shield")
	}
}

func mustSensitivity(t *testing.T, s string) policy.Sensitivity {
	t.Helper()
	sens, err := parseSensitivity(s)
	if err != nil {
		t.Fatalf("parseSensitivity(%q): %v", s, err)
	}
	return sens
}

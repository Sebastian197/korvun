// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"errors"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
)

// Estreno E-12 (red-team RT-4 + RT-6): the agent ceiling must budget the
// model that SURVIVES the privacy selector (buildAgentBrain runs
// selected[0], not Models[0]), and Preflight must derive the router ceiling
// so a too-low override fails cheaply BEFORE the cutover tears the serving
// app down.

func TestCeilingForBrain_agentBudgetsTheSelectorSurvivor(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	// Private brain listing a fast cloud model FIRST: the selector drops it
	// and the agent runs the LOCAL model (120s default per-attempt). A
	// ceiling derived from Models[0] (15s) would guillotine the local model
	// mid-first-attempt on every cold call.
	mixed := config.BrainConfig{
		Name: "agent", Sensitivity: "private",
		Models: []config.ModelConfig{
			{Provider: "groq", ModelID: "fast", Locality: "cloud", RequestTimeout: "15s"},
			{Provider: "ollama", ModelID: "big", Locality: "local"},
		},
		Agent: &config.AgentConfig{Tools: []string{"calc"}},
	}
	localOnly := config.BrainConfig{
		Name: "agent", Sensitivity: "private",
		Models: []config.ModelConfig{
			{Provider: "ollama", ModelID: "big", Locality: "local"},
		},
		Agent: &config.AgentConfig{Tools: []string{"calc"}},
	}
	got := ceilingForBrain(cfg, mixed)
	want := ceilingForBrain(cfg, localOnly)
	if got != want {
		t.Fatalf("ceiling(mixed private) = %v, want %v (the surviving LOCAL model's budget, not Models[0]'s)", got, want)
	}
}

func TestPreflight_tooLowCeilingOverride_failsBeforeCutover(t *testing.T) {
	cfg := cfgWith(ollamaBrain())
	cfg.BrainHandlerTimeout = "1s" // far below any derived ceiling

	err := Preflight(cfg, withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if !errors.Is(err, ErrCeilingOverrideTooLow) {
		t.Fatalf("Preflight err = %v, want ErrCeilingOverrideTooLow — this class of construction error must fail while the old app still serves", err)
	}
}

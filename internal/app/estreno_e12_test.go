// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"errors"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/brain"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/tool"
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

// Re-review follow-up (F4): the agent ceiling must budget TOOL time — the
// native lane runs up to brain.MaxNativeCallsPerTurn tools per iteration and
// http_fetch now owns a 30s bound, so a legitimate multi-tool exchange was
// guillotined by a ceiling that only counted model attempts.
func TestCeilingForBrain_agentBudgetsToolTime(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	base := config.BrainConfig{
		Name: "agent", Sensitivity: "public",
		Models: []config.ModelConfig{{Provider: "ollama", ModelID: "m", Locality: "local"}},
	}
	noTools := base
	noTools.Agent = &config.AgentConfig{}
	withCalc := base
	withCalc.Agent = &config.AgentConfig{Tools: []string{"calc"}}
	withFetch := base
	withFetch.Agent = &config.AgentConfig{Tools: []string{"http_fetch"},
		HTTPFetch: &config.HTTPFetchToolConfig{AllowHosts: []string{"x"}}}

	iters := time.Duration(brain.DefaultAgentMaxIterations)
	calls := time.Duration(brain.MaxNativeCallsPerTurn)

	if got, want := ceilingForBrain(cfg, withFetch)-ceilingForBrain(cfg, noTools), iters*calls*tool.DefaultFetchTimeout; got != want {
		t.Fatalf("fetch tool budget in ceiling = %v, want %v (maxIter × per-turn cap × the tool's own bound)", got, want)
	}
	if got, want := ceilingForBrain(cfg, withCalc)-ceilingForBrain(cfg, noTools), iters*calls*time.Second; got != want {
		t.Fatalf("local tool budget in ceiling = %v, want %v (1s per local tool call)", got, want)
	}
}

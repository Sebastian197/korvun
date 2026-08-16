// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// SP-B red suite (minimal-memory spec 2026-08-16, P2 + FR-PRIV-1 at boot):
// memory_note without a covering governance grant fails loud (the E-11
// molde — D1 is never vacuously ungoverned), and memory.scope "brain"
// requires the SELECTED model to be Local (the localityOf precedent, not
// the raw catalog). AS-B4 (guard half) + AS-B14 (boot half).
package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/conversation"
)

// storedBuilder is testBuilder with a live store — the memory closures hang
// off b.store (config validation requires storage in production; direct
// buildBrain tests provide the store by hand).
func storedBuilder() *builder {
	b := testBuilder()
	b.store = conversation.NewMemStore()
	return b
}

// memAgentBrainCfg is an agent brain listing memory_note with its memory
// block; governance and scope vary per test.
func memAgentBrainCfg(locality, scope string, governed bool) config.BrainConfig {
	bc := config.BrainConfig{
		Name:        "agent",
		Sensitivity: "public",
		Policy:      config.PolicyConfig{Kind: "priority"},
		Models:      []config.ModelConfig{{Provider: "ollama", ModelID: "m", Locality: locality}},
		Agent: &config.AgentConfig{
			Tools:  []string{"memory_note"},
			Memory: &config.MemoryConfig{Scope: scope},
		},
	}
	if governed {
		bc.Agent.Governance = []config.ToolGrantConfig{{Tool: "memory_note", Mode: "allow"}}
	}
	return bc
}

// P2: no governance block at all — fail loud naming the tool.
func TestBuildBrain_memoryNoteUngoverned_failsLoud(t *testing.T) {
	t.Parallel()
	_, err := testBuilder().buildBrain(memAgentBrainCfg("local", "conversation", false))
	if !errors.Is(err, ErrMemoryToolUngoverned) {
		t.Fatalf("err = %v, want ErrMemoryToolUngoverned", err)
	}
	if !strings.Contains(err.Error(), "memory_note") {
		t.Errorf("error %q does not name memory_note", err.Error())
	}
}

// P2: a governance block that does NOT cover memory_note is the same hole.
func TestBuildBrain_memoryNoteGrantNotCovering_failsLoud(t *testing.T) {
	t.Parallel()
	bc := memAgentBrainCfg("local", "conversation", false)
	bc.Agent.Tools = append(bc.Agent.Tools, "time")
	bc.Agent.Governance = []config.ToolGrantConfig{{Tool: "time", Mode: "allow"}}
	_, err := testBuilder().buildBrain(bc)
	if !errors.Is(err, ErrMemoryToolUngoverned) {
		t.Fatalf("err = %v, want ErrMemoryToolUngoverned (the grant list does not cover memory_note)", err)
	}
}

// FR-PRIV-1: scope "brain" on a cloud-SELECTED model is a boot error naming
// the field; on an all-local brain it builds (AS-B4's guard halves).
func TestBuildBrain_memoryScopeBrain_localityGuard(t *testing.T) {
	t.Parallel()
	_, err := testBuilder().buildBrain(memAgentBrainCfg("cloud", "brain", true))
	if !errors.Is(err, ErrMemoryScopeCloud) {
		t.Fatalf("cloud-selected err = %v, want ErrMemoryScopeCloud", err)
	}
	if !strings.Contains(err.Error(), "memory.scope") {
		t.Errorf("error %q does not name memory.scope", err.Error())
	}
	if _, err := storedBuilder().buildBrain(memAgentBrainCfg("local", "brain", true)); err != nil {
		t.Fatalf("all-local brain-scope must build, got %v", err)
	}
}

// Conversation scope stays valid on any locality (the same posture live
// history already has) — a governed cloud brain with conversation scope
// builds.
func TestBuildBrain_memoryConversationScopeOnCloud_builds(t *testing.T) {
	t.Parallel()
	if _, err := storedBuilder().buildBrain(memAgentBrainCfg("cloud", "conversation", true)); err != nil {
		t.Fatalf("conversation-scope cloud brain must build, got %v", err)
	}
}

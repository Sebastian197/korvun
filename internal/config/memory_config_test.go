// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// SP-B red suite (minimal-memory spec 2026-08-16, FR-CFG-1 + AS-B14): the
// brains[i].agent.memory block — presence-detected, coherent defaults,
// range validation, the tool↔block pairing (the read_file-cage precedent),
// the storage requirement (the session precedent), and the P4 coherence
// bound — everything fail-loud naming the field path.
package config

import (
	"errors"
	"strings"
	"testing"
)

// memoryBrainBase is validBase() with an agent brain carrying memory_note,
// a covering governance grant, and a memory block.
func memoryBrainBase(mem *MemoryConfig) *Config {
	c := validBase()
	c.Brains[0].Agent = &AgentConfig{
		Tools:      []string{"memory_note"},
		Governance: []ToolGrantConfig{{Tool: "memory_note", Mode: "allow"}},
		Memory:     mem,
	}
	return c
}

func TestMemoryConfig_DefaultsResolve(t *testing.T) {
	got := (&MemoryConfig{}).Settings()
	want := MemorySettings{BrainGlobal: false, MaxNotes: 10, MaxNoteRunes: 200, BudgetRunes: 2000}
	if got != want {
		t.Fatalf("Settings() = %+v, want the spec defaults %+v", got, want)
	}
}

func TestMemoryConfig_ScopeBrainResolves(t *testing.T) {
	got := (&MemoryConfig{Scope: "brain"}).Settings()
	if !got.BrainGlobal {
		t.Fatalf("Settings(scope=brain).BrainGlobal = false, want true")
	}
	if got2 := (&MemoryConfig{Scope: "conversation"}).Settings(); got2.BrainGlobal {
		t.Fatalf("Settings(scope=conversation).BrainGlobal = true, want false")
	}
}

func TestMemoryConfig_ValidShapesPass(t *testing.T) {
	cases := []*MemoryConfig{
		{},
		{Scope: "conversation"},
		{Scope: "brain"},
		{MaxNotes: 1, MaxNoteRunes: 1, BudgetRunes: 1},
		{MaxNotes: 100, MaxNoteRunes: 2000, BudgetRunes: 200000},
	}
	for i, mem := range cases {
		if err := memoryBrainBase(mem).Validate(); err != nil {
			t.Fatalf("case %d: Validate rejected a valid memory block %+v: %v", i, mem, err)
		}
	}
}

// AS-B14 (the config half, table): every invalid shape fails loud naming
// its field path.
func TestMemoryConfig_InvalidShapesFailClearly(t *testing.T) {
	cases := []struct {
		name     string
		mut      func(*Config)
		wantFrag string
	}{
		{"memory_note listed without the block", func(c *Config) {
			c.Brains[0].Agent.Memory = nil
		}, "agent.memory"},
		{"block without the tool", func(c *Config) {
			c.Brains[0].Agent.Tools = []string{"time"}
			c.Brains[0].Agent.Governance = nil
		}, "agent.memory"},
		{"invalid scope", func(c *Config) {
			c.Brains[0].Agent.Memory = &MemoryConfig{Scope: "global"}
		}, "agent.memory.scope"},
		{"max_notes over 100", func(c *Config) {
			c.Brains[0].Agent.Memory = &MemoryConfig{MaxNotes: 101}
		}, "agent.memory.max_notes"},
		{"max_notes negative", func(c *Config) {
			c.Brains[0].Agent.Memory = &MemoryConfig{MaxNotes: -1}
		}, "agent.memory.max_notes"},
		{"max_note_runes over 2000", func(c *Config) {
			c.Brains[0].Agent.Memory = &MemoryConfig{MaxNoteRunes: 2001}
		}, "agent.memory.max_note_runes"},
		{"max_note_runes negative", func(c *Config) {
			c.Brains[0].Agent.Memory = &MemoryConfig{MaxNoteRunes: -1}
		}, "agent.memory.max_note_runes"},
		{"budget below the coherence bound", func(c *Config) {
			c.Brains[0].Agent.Memory = &MemoryConfig{MaxNotes: 10, MaxNoteRunes: 200, BudgetRunes: 100}
		}, "agent.memory.budget_runes"},
		{"memory without storage", func(c *Config) {
			c.Storage = nil
		}, "storage"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := memoryBrainBase(&MemoryConfig{})
			tc.mut(c)
			err := c.Validate()
			if err == nil {
				t.Fatalf("Validate accepted the invalid shape (%s)", tc.name)
			}
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("error = %v, want ErrInvalidConfig", err)
			}
			if !strings.Contains(err.Error(), tc.wantFrag) {
				t.Fatalf("error %q does not name %q", err, tc.wantFrag)
			}
		})
	}
}

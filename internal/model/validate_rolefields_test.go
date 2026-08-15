// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"errors"
	"testing"
)

// Estreno E-10 (Codex C-5): the tool-calling fields obey per-role
// invariants. Before this, the empty-content exception keyed on ToolCalls
// alone, so System/User turns could carry ToolCalls, RoleTool could carry
// its own ToolCalls, and ToolName rode on any role — ambiguous shapes the
// native adapter would serialize to the provider while old adapters ignored
// them (role confusion, provider-dependent failures).

func validateOne(m Message) error {
	return ValidateRequest(&Request{Model: "m", Messages: []Message{m}})
}

func TestValidateRequest_roleFieldMatrix(t *testing.T) {
	valid := []Message{
		{Role: RoleSystem, Content: "s"},
		{Role: RoleUser, Content: "u"},
		{Role: RoleAssistant, Content: "a"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{{Name: "t"}}},               // words-free tool turn
		{Role: RoleAssistant, Content: "a", ToolCalls: []ToolCall{{Name: "t"}}}, // both is fine
		{Role: RoleTool, ToolName: "t", Content: "result"},
	}
	for i, m := range valid {
		if err := validateOne(m); err != nil {
			t.Errorf("valid[%d] (%+v): unexpected error %v", i, m, err)
		}
	}

	invalid := []struct {
		name string
		m    Message
	}{
		{"system with tool calls", Message{Role: RoleSystem, Content: "s", ToolCalls: []ToolCall{{Name: "t"}}}},
		{"user with tool calls", Message{Role: RoleUser, Content: "u", ToolCalls: []ToolCall{{Name: "t"}}}},
		{"system with tool name", Message{Role: RoleSystem, Content: "s", ToolName: "t"}},
		{"user with tool name", Message{Role: RoleUser, Content: "u", ToolName: "t"}},
		{"assistant with tool name", Message{Role: RoleAssistant, Content: "a", ToolName: "t"}},
		{"tool turn with its own tool calls", Message{Role: RoleTool, ToolName: "t", Content: "r", ToolCalls: []ToolCall{{Name: "x"}}}},
		{"tool turn without content", Message{Role: RoleTool, ToolName: "t"}},
		{"empty system with tool calls", Message{Role: RoleSystem, ToolCalls: []ToolCall{{Name: "t"}}}},
	}
	for _, tc := range invalid {
		if err := validateOne(tc.m); err == nil {
			t.Errorf("%s: accepted, want a refusal", tc.name)
		} else if !errors.Is(err, ErrInvalidRole) && !errors.Is(err, ErrEmptyContent) {
			t.Errorf("%s: err = %v, want the validation sentinel grammar", tc.name, err)
		}
	}
}

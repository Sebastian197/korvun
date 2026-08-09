// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"context"
	"testing"
)

// The native tool-calling lane's types (ADR-0042 §1/§2): ToolCallingModel as
// a SIBLING of Model (never a widening), RoleTool + additive Message fields
// for the verified Ollama cycle, all zero-values invisible to the old lane.

// fakeToolCaller proves the compile contract: a ToolCallingModel IS a Model.
type fakeToolCaller struct{}

func (fakeToolCaller) Generate(context.Context, *Request) (*Response, error) { return &Response{}, nil }
func (fakeToolCaller) Name() string                                          { return "fake" }
func (fakeToolCaller) GenerateWithTools(context.Context, *Request, []ToolSpec) (*Response, error) {
	return &Response{}, nil
}

func TestToolCallingModel_isASiblingOfModel(t *testing.T) {
	t.Parallel()
	var tcm ToolCallingModel = fakeToolCaller{}
	var m Model = tcm // a ToolCallingModel is always a Model
	if m.Name() != "fake" {
		t.Fatal("embedding broken")
	}
	// The capability is discoverable by assertion — the lane-picking move.
	if _, ok := m.(ToolCallingModel); !ok {
		t.Fatal("capability not discoverable via type assertion")
	}
}

func TestRoleTool_String(t *testing.T) {
	t.Parallel()
	if got := RoleTool.String(); got != "tool" {
		t.Fatalf("RoleTool.String() = %q, want %q (the verified Ollama wire role)", got, "tool")
	}
}

// Zero-values of the additive Message fields are invisible: an old-lane
// message constructed exactly as before carries nil/empty new fields.
func TestMessage_additiveFieldsZeroByDefault(t *testing.T) {
	t.Parallel()
	m := Message{Role: RoleUser, Content: "hola"}
	if m.ToolCalls != nil || m.ToolName != "" {
		t.Fatalf("additive fields not zero by default: %+v", m)
	}
}

func TestToolCall_shape(t *testing.T) {
	t.Parallel()
	tc := ToolCall{Name: "read_file", Arguments: map[string]any{"args": "nota.txt"}}
	if tc.Name != "read_file" || tc.Arguments["args"] != "nota.txt" {
		t.Fatalf("ToolCall shape wrong: %+v", tc)
	}
}

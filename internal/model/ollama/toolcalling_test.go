// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package ollama

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Sebastian197/korvun/internal/model"
)

// The native lane on the Ollama adapter (ADR-0042 §3), pinned to the
// source-verified /api/chat contract: `tools` with the uniform
// {"args": string} schema on the request, `tool_calls` parsed from the
// response, RoleTool/ToolName/ToolCalls turns serialized per the wire.

// nativeServer captures the raw request body and answers with the given
// response JSON.
func nativeServer(t *testing.T, respond string) (*Adapter, *[]byte) {
	t.Helper()
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(respond))
	}))
	t.Cleanup(srv.Close)
	return New(WithBaseURL(srv.URL)), &captured
}

func specs() []model.ToolSpec {
	return []model.ToolSpec{
		{Name: "read_file", Description: "reads a file. args = the path."},
		{Name: "calc", Description: "arithmetic. args = the expression."},
	}
}

func TestAdapter_satisfiesToolCallingModel(t *testing.T) {
	t.Parallel()
	a, _ := nativeServer(t, `{"message":{"role":"assistant","content":"x"},"done":true}`)
	var _ model.ToolCallingModel = a
}

func TestGenerateWithTools_requestCarriesUniformToolSchema(t *testing.T) {
	t.Parallel()
	a, captured := nativeServer(t, `{"message":{"role":"assistant","content":"done"},"done":true}`)

	req := &model.Request{Model: "llama3.2", Messages: []model.Message{{Role: model.RoleUser, Content: "hola"}}}
	if _, err := a.GenerateWithTools(context.Background(), req, specs()); err != nil {
		t.Fatalf("GenerateWithTools: %v", err)
	}

	var wire struct {
		Tools []struct {
			Type     string `json:"type"`
			Function struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Parameters  struct {
					Type       string   `json:"type"`
					Required   []string `json:"required"`
					Properties map[string]struct {
						Type        string `json:"type"`
						Description string `json:"description"`
					} `json:"properties"`
				} `json:"parameters"`
			} `json:"function"`
		} `json:"tools"`
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(*captured, &wire); err != nil {
		t.Fatalf("unmarshal wire: %v", err)
	}
	if wire.Stream {
		t.Fatal("native lane must be non-streaming")
	}
	if len(wire.Tools) != 2 {
		t.Fatalf("tools = %d, want 2", len(wire.Tools))
	}
	rf := wire.Tools[0]
	if rf.Type != "function" || rf.Function.Name != "read_file" || rf.Function.Description == "" {
		t.Fatalf("tool wire shape wrong: %+v", rf)
	}
	if rf.Function.Parameters.Type != "object" ||
		len(rf.Function.Parameters.Required) != 1 || rf.Function.Parameters.Required[0] != "args" ||
		rf.Function.Parameters.Properties["args"].Type != "string" {
		t.Fatalf("uniform args schema wrong: %+v", rf.Function.Parameters)
	}
}

func TestGenerateWithTools_parsesToolCalls(t *testing.T) {
	t.Parallel()
	a, _ := nativeServer(t, `{
		"message": {"role": "assistant", "content": "",
			"tool_calls": [{"function": {"name": "read_file", "arguments": {"args": "nota.txt"}}}]},
		"done": true}`)

	req := &model.Request{Model: "llama3.2", Messages: []model.Message{{Role: model.RoleUser, Content: "lee"}}}
	resp, err := a.GenerateWithTools(context.Background(), req, specs())
	if err != nil {
		t.Fatalf("GenerateWithTools: %v", err)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %+v, want 1", resp.Message.ToolCalls)
	}
	tc := resp.Message.ToolCalls[0]
	if tc.Name != "read_file" || tc.Arguments["args"] != "nota.txt" {
		t.Fatalf("tool call mangled: %+v", tc)
	}
}

// Empty content WITH tool_calls is a valid native reply (the model wants a
// tool, not words) — never the empty-content provider error the old lane
// raises.
func TestGenerateWithTools_emptyContentWithCallsIsValid(t *testing.T) {
	t.Parallel()
	a, _ := nativeServer(t, `{
		"message": {"role": "assistant", "content": "",
			"tool_calls": [{"function": {"name": "calc", "arguments": {"args": "2+2"}}}]},
		"done": true}`)

	req := &model.Request{Model: "llama3.2", Messages: []model.Message{{Role: model.RoleUser, Content: "suma"}}}
	if _, err := a.GenerateWithTools(context.Background(), req, specs()); err != nil {
		t.Fatalf("empty content with tool_calls must be valid: %v", err)
	}
}

// Empty content WITHOUT tool_calls stays the provider error it always was.
func TestGenerateWithTools_emptyContentWithoutCallsIsError(t *testing.T) {
	t.Parallel()
	a, _ := nativeServer(t, `{"message":{"role":"assistant","content":""},"done":true}`)

	req := &model.Request{Model: "llama3.2", Messages: []model.Message{{Role: model.RoleUser, Content: "x"}}}
	if _, err := a.GenerateWithTools(context.Background(), req, specs()); err == nil {
		t.Fatal("empty content with no tool_calls must be a provider error")
	}
}

func TestGenerateWithTools_serializesToolCycleTurns(t *testing.T) {
	t.Parallel()
	a, captured := nativeServer(t, `{"message":{"role":"assistant","content":"la clave es ALMENDRA"},"done":true}`)

	req := &model.Request{Model: "llama3.2", Messages: []model.Message{
		{Role: model.RoleUser, Content: "lee la nota"},
		{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{Name: "read_file", Arguments: map[string]any{"args": "nota.txt"}}}},
		{Role: model.RoleTool, ToolName: "read_file", Content: "La palabra clave es ALMENDRA."},
	}}
	if _, err := a.GenerateWithTools(context.Background(), req, specs()); err != nil {
		t.Fatalf("GenerateWithTools: %v", err)
	}

	var wire struct {
		Messages []struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			ToolName  string `json:"tool_name,omitempty"`
			ToolCalls []struct {
				Function struct {
					Name      string         `json:"name"`
					Arguments map[string]any `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls,omitempty"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(*captured, &wire); err != nil {
		t.Fatalf("unmarshal wire: %v", err)
	}
	if len(wire.Messages) != 3 {
		t.Fatalf("messages = %d, want 3", len(wire.Messages))
	}
	asst := wire.Messages[1]
	if asst.Role != "assistant" || len(asst.ToolCalls) != 1 || asst.ToolCalls[0].Function.Name != "read_file" {
		t.Fatalf("assistant tool_calls turn wrong: %+v", asst)
	}
	toolTurn := wire.Messages[2]
	if toolTurn.Role != "tool" || toolTurn.ToolName != "read_file" || toolTurn.Content != "La palabra clave es ALMENDRA." {
		t.Fatalf("tool result turn wrong: %+v", toolTurn)
	}
}

// The OLD lane is byte-identical: a plain Generate request carries no tools
// key and no tool fields on messages (the additive DTO growth is invisible).
func TestGenerate_oldLaneWireUnchanged(t *testing.T) {
	t.Parallel()
	a, captured := nativeServer(t, `{"message":{"role":"assistant","content":"hola"},"done":true}`)

	req := &model.Request{Model: "llama3.2", Messages: []model.Message{{Role: model.RoleUser, Content: "hola"}}}
	if _, err := a.Generate(context.Background(), req); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(*captured, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, has := raw["tools"]; has {
		t.Fatal("old-lane request grew a tools key")
	}
	msgs := raw["messages"].([]any)
	first := msgs[0].(map[string]any)
	if _, has := first["tool_calls"]; has {
		t.Fatal("old-lane message grew tool_calls")
	}
	if _, has := first["tool_name"]; has {
		t.Fatal("old-lane message grew tool_name")
	}
}

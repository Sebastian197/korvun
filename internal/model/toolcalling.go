// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package model

import "context"

// This file is the native tool-calling lane's type surface (ADR-0042 §1/§2):
// a SIBLING capability interface — the ADR-0009 §2 StreamingModel precedent —
// plus the additive DTO growth the verified Ollama cycle needs. Model itself
// is never widened; every new field is zero by default and invisible to the
// prompt-protocol lane and to every existing adapter.

// RoleTool labels a tool-result turn in the native lane (the verified wire
// role "tool"). It never appears in prompt-protocol conversations.
const RoleTool Role = RoleAssistant + 1

// ToolSpec advertises one tool to a native tool-calling model (ADR-0042 §1).
// The v1 parameter schema is UNIFORM — a single string argument "args",
// documented by Description (each built-in already explains its format
// there) — so the tool.Tool seam's Execute(ctx, args string) contract stays
// untouched. Richer per-tool schemas are an additive future extension.
type ToolSpec struct {
	// Name is the tool's protocol name.
	Name string
	// Description is the capability line advertised to the model.
	Description string
}

// ToolCall is one native tool request returned by the model (ADR-0042 §1).
type ToolCall struct {
	// Name is the requested tool's name.
	Name string
	// Arguments is the raw JSON object the provider returned. Under the
	// uniform v1 schema the payload is Arguments["args"] (a string); the
	// lane re-serializes anything else verbatim for the tool to parse.
	Arguments map[string]any
}

// ToolCallingModel is the native-lane capability interface (ADR-0042 §1,
// ADR-0021 §3.4): providers with structured tool calling ALSO satisfy it.
// The AgentBrain discovers it by type assertion and prefers it; models
// without it keep the prompt-protocol lane, unchanged.
type ToolCallingModel interface {
	Model
	// GenerateWithTools is Generate plus a tool catalog: the reply may carry
	// Message.ToolCalls (the model wants tools) or plain Content (the final
	// answer). Implementations honor the same statelessness and concurrency
	// contract Generate carries.
	GenerateWithTools(ctx context.Context, req *Request, tools []ToolSpec) (*Response, error)
}

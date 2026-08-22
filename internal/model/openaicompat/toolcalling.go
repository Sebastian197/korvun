// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package openaicompat

// This file is the adapter's native tool-calling lane (ADR-0044 SP-B,
// FR-GWB-2/3): tools on the request mapped from model.ToolSpec, history
// replay with id threading, and tool_calls parsing with STRING-JSON
// arguments normalized to the seam's map type. Both lanes ride the ONE
// shared chat engine (openaicompat.go), so they cannot drift in
// semantics — HTTP construction, redirect refusal, caps, EOF-demanding
// decode, the FR-GW-4 matrix, and the H8 belt are a single code path.
//
// NO capability auto-detection: the operator chooses tools-capable
// models (documented in CONFIGURATION.md); a server 400 flows through
// the matrix as an honest permanent error and is NEVER translated to
// ErrToolsUnsupported — that sentinel stays with the ollama-verified
// refusal (ADR-0042 RT-3).

import (
	"context"

	"github.com/Sebastian197/korvun/internal/model"
)

// Compile-time assertion: the adapter satisfies the sibling capability
// (model.ToolCallingModel), so the production chain retry → WithModelID
// propagates it with zero new wiring code (ADR-0042 §4).
var _ model.ToolCallingModel = (*Adapter)(nil)

// GenerateWithTools implements model.ToolCallingModel per FR-GWB-2/3:
// Generate plus the tools catalog, on the shared engine.
func (a *Adapter) GenerateWithTools(ctx context.Context, req *model.Request, tools []model.ToolSpec) (*model.Response, error) {
	return a.chat(ctx, req, tools)
}

// wireTool / wireFunction / wireParameters / wireProperty advertise one
// tool in the OpenAI shape (FR-GWB-2), rendered from model.ToolSpec with
// the ollama mold's schema discipline: the tool's declared structured
// fields when present (the ParamTool surface), else the uniform v1
// {"args": string} schema (ADR-0042 §1).
type wireTool struct {
	Type     string       `json:"type"`
	Function wireFunction `json:"function"`
}

type wireFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  wireParameters `json:"parameters"`
}

type wireParameters struct {
	Type       string                  `json:"type"`
	Required   []string                `json:"required,omitempty"`
	Properties map[string]wireProperty `json:"properties"`
}

type wireProperty struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// wireToolCall is one call on the wire — request replay AND response
// parse (FR-GWB-2/3). Arguments is a JSON STRING on this wire, unlike
// ollama's object.
type wireToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function wireCalledFunction `json:"function"`
}

type wireCalledFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// toWireTools renders each spec's schema (the ollama toWireTools mold,
// ollama/toolcalling.go:161-185).
func toWireTools(tools []model.ToolSpec) []wireTool {
	out := make([]wireTool, len(tools))
	for i, ts := range tools {
		params := wireParameters{Type: "object"}
		if len(ts.Params) > 0 {
			params.Properties = make(map[string]wireProperty, len(ts.Params))
			for _, p := range ts.Params {
				params.Properties[p.Name] = wireProperty{Type: "string", Description: p.Description}
				if p.Required {
					params.Required = append(params.Required, p.Name)
				}
			}
		} else {
			params.Required = []string{"args"}
			params.Properties = map[string]wireProperty{
				"args": {Type: "string", Description: ts.Description},
			}
		}
		out[i] = wireTool{
			Type:     "function",
			Function: wireFunction{Name: ts.Name, Description: ts.Description, Parameters: params},
		}
	}
	return out
}

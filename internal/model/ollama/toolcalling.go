// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Sebastian197/korvun/internal/model"
)

// This file is the adapter's native tool-calling lane (ADR-0042 §3), pinned
// to the source-verified /api/chat contract (Context7, 2026-08-09): `tools`
// with JSON-Schema functions on the request; `message.tool_calls` with
// OBJECT arguments on the response; results returned as role:"tool" turns
// with tool_name. The old Generate wire is untouched — separate wire structs,
// zero new fields on the old ones.

// Compile-time assertion: the adapter satisfies the sibling capability.
var _ model.ToolCallingModel = (*Adapter)(nil)

// nativeChatRequest is the tools-bearing /api/chat request.
type nativeChatRequest struct {
	Model    string              `json:"model"`
	Messages []nativeChatMessage `json:"messages"`
	Tools    []wireTool          `json:"tools"`
	Stream   bool                `json:"stream"`
}

// nativeChatMessage is the native-lane conversation turn: the old
// role/content plus the verified cycle fields.
type nativeChatMessage struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	ToolName  string         `json:"tool_name,omitempty"`
	ToolCalls []wireToolCall `json:"tool_calls,omitempty"`
}

// wireTool / wireToolCall mirror the verified wire shapes.
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

type wireToolCall struct {
	Function wireCalledFunction `json:"function"`
}

type wireCalledFunction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// nativeChatResponse is the native-lane response subset.
type nativeChatResponse struct {
	Model   string            `json:"model"`
	Message nativeChatMessage `json:"message"`
	Done    bool              `json:"done"`
}

// GenerateWithTools implements model.ToolCallingModel: Generate plus the
// verified tools cycle. Error mapping is identical to Generate's (the same
// sentinels, the same mapHTTPError), with ONE contract difference: an empty
// assistant content WITH tool_calls is a valid reply — the model wants a
// tool, not words.
func (a *Adapter) GenerateWithTools(ctx context.Context, req *model.Request, tools []model.ToolSpec) (*model.Response, error) {
	if err := model.ValidateRequest(req); err != nil {
		return nil, err
	}

	if a.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.timeout)
		defer cancel()
	}

	payload := nativeChatRequest{
		Model:    req.Model,
		Messages: toNativeChatMessages(req.Messages),
		Tools:    toWireTools(tools),
		Stream:   false,
	}
	body, err := json.Marshal(&payload)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal request: %w", model.ErrProviderResponse, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+chatPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %w", model.ErrProviderResponse, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", model.ErrProviderUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := mapHTTPError(resp)
		// The provider's capability refusal, verified raw (2026-08-15,
		// ollama 0.30.8, gemma3:270m, 3/3): HTTP 400 with
		// {"error":"...does not support tools"}. It gets its OWN sentinel so
		// the agent can degrade to the prompt-protocol lane (RT-3) instead
		// of treating it as a generic bad response.
		if resp.StatusCode == http.StatusBadRequest && strings.Contains(err.Error(), "does not support tools") {
			return nil, fmt.Errorf("%w: %w", model.ErrToolsUnsupported, err)
		}
		return nil, err
	}

	var apiResp nativeChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("%w: decode response: %w", model.ErrProviderResponse, err)
	}
	if apiResp.Message.Content == "" && len(apiResp.Message.ToolCalls) == 0 {
		return nil, fmt.Errorf("%w: response had neither assistant content nor tool calls",
			model.ErrProviderResponse)
	}

	msg := model.Message{Role: model.RoleAssistant, Content: apiResp.Message.Content}
	for _, tc := range apiResp.Message.ToolCalls {
		msg.ToolCalls = append(msg.ToolCalls, model.ToolCall{
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return &model.Response{
		Message:   msg,
		Provider:  ProviderName,
		ModelName: apiResp.Model,
	}, nil
}

// toWireTools renders each spec's schema: the tool's declared structured
// fields when present (the ParamTool surface), else the uniform v1
// {"args": string} schema (ADR-0042 §1).
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

// toNativeChatMessages maps the canonical turns onto the native wire,
// carrying the cycle fields the old lane never sends.
func toNativeChatMessages(in []model.Message) []nativeChatMessage {
	out := make([]nativeChatMessage, len(in))
	for i, m := range in {
		msg := nativeChatMessage{
			Role:     m.Role.String(),
			Content:  m.Content,
			ToolName: m.ToolName,
		}
		for _, tc := range m.ToolCalls {
			msg.ToolCalls = append(msg.ToolCalls, wireToolCall{
				Function: wireCalledFunction{Name: tc.Name, Arguments: tc.Arguments},
			})
		}
		out[i] = msg
	}
	return out
}

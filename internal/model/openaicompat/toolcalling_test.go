// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package openaicompat

// RED suite for SP-B of the universal model gateway (FR-GWB-1/2/3/4 +
// AS-B-1/2/3/6). The SP-A suite is untouched; the empty-content amendment
// is NEW contract pinned here (no approved SP-A test pinned the amended
// form — the SP-A malformed row carries neither refusal nor tool_calls).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Sebastian197/korvun/internal/model"
)

// toolsReq is a request with history for the replay scenarios.
func toolsReq(msgs ...model.Message) *model.Request {
	if len(msgs) == 0 {
		msgs = []model.Message{{Role: model.RoleUser, Content: "what is 2+2?"}}
	}
	return &model.Request{Model: "test-model", Messages: msgs}
}

// calcSpec is the uniform-v1 tool catalog used across the SP-B rows.
var calcSpec = []model.ToolSpec{{Name: "calc", Description: "evaluate an arithmetic expression"}}

// toolCallsBody is a valid calls-bearing 200: EMPTY content, tool_calls
// with id + STRING-JSON arguments, finish_reason "tool_calls".
func toolCallsBody() string {
	return `{"model":"served-model","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"calc","arguments":"{\"args\":\"2+2\"}"}}]}}]}`
}

// TestGenerateWithTools_requestCarriesTools pins AS-B-1: the outbound
// request carries the tools array in the OpenAI shape mapped from
// model.ToolSpec (uniform v1 args schema), beside {model, messages,
// stream:false}.
func TestGenerateWithTools_requestCarriesTools(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(okBody("four")))
	}))
	t.Cleanup(srv.Close)

	a := newAdapter(t, srv.URL)
	if _, err := a.GenerateWithTools(context.Background(), toolsReq(), calcSpec); err != nil {
		t.Fatalf("GenerateWithTools: %v", err)
	}

	tools, _ := gotBody["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("body tools len = %d, want 1", len(tools))
	}
	tw, _ := tools[0].(map[string]any)
	if tw["type"] != "function" {
		t.Errorf("tools[0].type = %v, want function", tw["type"])
	}
	fn, _ := tw["function"].(map[string]any)
	if fn["name"] != "calc" || fn["description"] != "evaluate an arithmetic expression" {
		t.Errorf("tools[0].function = %v, want calc + description", fn)
	}
	params, _ := fn["parameters"].(map[string]any)
	if params["type"] != "object" {
		t.Errorf("parameters.type = %v, want object (uniform v1 schema)", params["type"])
	}
	if v, present := gotBody["stream"]; !present || v != false {
		t.Errorf("body stream = %v (present=%t), want explicit false", v, present)
	}
}

// TestGenerateWithTools_parsesToolCalls pins AS-B-2 and the amendment:
// EMPTY content + non-empty tool_calls with finish_reason "tool_calls" is
// a VALID calls-bearing reply; ids ride ToolCall.ID and the STRING-JSON
// arguments normalize to the seam's map type.
func TestGenerateWithTools_parsesToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(toolCallsBody()))
	}))
	t.Cleanup(srv.Close)

	a := newAdapter(t, srv.URL)
	resp, err := a.GenerateWithTools(context.Background(), toolsReq(), calcSpec)
	if err != nil {
		t.Fatalf("GenerateWithTools on calls-bearing 200: %v (the amendment: valid, not malformed)", err)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(resp.Message.ToolCalls))
	}
	call := resp.Message.ToolCalls[0]
	if call.ID != "call_1" {
		t.Errorf("ToolCall.ID = %q, want call_1 (FR-GWB-1/3)", call.ID)
	}
	if call.Name != "calc" {
		t.Errorf("ToolCall.Name = %q, want calc", call.Name)
	}
	if got := call.Arguments["args"]; got != "2+2" {
		t.Errorf("Arguments[args] = %v, want 2+2 (STRING-JSON normalized to the seam map)", got)
	}
}

// TestGenerateWithTools_badArgumentsString pins FR-GWB-3's malformed
// bound: an arguments string that is not a JSON object maps to
// ErrProviderResponse.
func TestGenerateWithTools_badArgumentsString(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"model":"m","choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"calc","arguments":"not json"}}]}}]}`))
	}))
	t.Cleanup(srv.Close)

	a := newAdapter(t, srv.URL)
	if _, err := a.GenerateWithTools(context.Background(), toolsReq(), calcSpec); !errors.Is(err, model.ErrProviderResponse) {
		t.Errorf("err = %v, want ErrProviderResponse (unparseable arguments string)", err)
	}
}

// TestGenerateWithTools_historyReplay pins AS-B-3: assistant turns WITH
// ToolCalls re-serialize to the wire (id + type + function with STRING
// arguments) and RoleTool turns go out as {role:"tool", tool_call_id,
// content}.
func TestGenerateWithTools_historyReplay(t *testing.T) {
	var gotBody struct {
		Messages []map[string]any `json:"messages"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(okBody("the answer is 4")))
	}))
	t.Cleanup(srv.Close)

	history := []model.Message{
		{Role: model.RoleUser, Content: "what is 2+2?"},
		{Role: model.RoleAssistant, Content: "", ToolCalls: []model.ToolCall{{
			ID: "call_1", Name: "calc", Arguments: map[string]any{"args": "2+2"},
		}}},
		{Role: model.RoleTool, ToolName: "calc", ToolCallID: "call_1", Content: "4"},
	}
	a := newAdapter(t, srv.URL)
	if _, err := a.GenerateWithTools(context.Background(), toolsReq(history...), calcSpec); err != nil {
		t.Fatalf("GenerateWithTools with history: %v", err)
	}
	if len(gotBody.Messages) != 3 {
		t.Fatalf("wire messages len = %d, want 3", len(gotBody.Messages))
	}

	asst := gotBody.Messages[1]
	calls, _ := asst["tool_calls"].([]any)
	if len(calls) != 1 {
		t.Fatalf("assistant turn tool_calls len = %d, want 1 (history replay)", len(calls))
	}
	cw, _ := calls[0].(map[string]any)
	if cw["id"] != "call_1" || cw["type"] != "function" {
		t.Errorf("replayed call = %v, want id=call_1 type=function", cw)
	}
	fn, _ := cw["function"].(map[string]any)
	argsStr, isString := fn["arguments"].(string)
	if !isString {
		t.Fatalf("replayed arguments = %T, want a JSON STRING on this wire", fn["arguments"])
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(argsStr), &decoded); err != nil || decoded["args"] != "2+2" {
		t.Errorf("replayed arguments string = %q, want JSON object with args=2+2", argsStr)
	}

	toolTurn := gotBody.Messages[2]
	if toolTurn["role"] != "tool" {
		t.Errorf("turn 3 role = %v, want tool", toolTurn["role"])
	}
	if toolTurn["tool_call_id"] != "call_1" {
		t.Errorf("turn 3 tool_call_id = %v, want call_1 (FR-GWB-1 threading on the wire)", toolTurn["tool_call_id"])
	}
	if toolTurn["content"] != "4" {
		t.Errorf("turn 3 content = %v, want 4", toolTurn["content"])
	}
}

// TestGenerateWithTools_400IsHonestPermanent pins FR-GWB-2's no-detection
// rule: a server 400 (e.g. a model without tools support) flows through
// the matrix as an HONEST permanent ErrProviderResponse — never a magic
// capability probe, and NOT ErrToolsUnsupported (that sentinel stays with
// the ollama-verified refusal).
func TestGenerateWithTools_400IsHonestPermanent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"tools is not supported for this model","type":"invalid_request_error","code":""}}`))
	}))
	t.Cleanup(srv.Close)

	a := newAdapter(t, srv.URL)
	_, err := a.GenerateWithTools(context.Background(), toolsReq(), calcSpec)
	if !errors.Is(err, model.ErrProviderResponse) {
		t.Errorf("err = %v, want permanent ErrProviderResponse", err)
	}
	if errors.Is(err, model.ErrToolsUnsupported) {
		t.Errorf("err = %v: must NOT be ErrToolsUnsupported — SP-B does no capability detection", err)
	}
}

// TestGenerate_quotaKimiCode is the AS-B-6 matrix row (FR-GWB-4, the AS-5
// demo finding, double-cited in the spec): a 429 with
// type=exceeded_current_quota_error is quota exhaustion — permanent,
// never a RateLimitError. Born RED: today the code falls to the safe
// default.
func TestGenerate_quotaKimiCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"error":{"message":"account suspended due to insufficient balance","type":%q,"code":""}}`, "exceeded_current_quota_error")))
	}))
	t.Cleanup(srv.Close)

	_, err := newAdapter(t, srv.URL).Generate(context.Background(), chatReq())
	if !errors.Is(err, model.ErrProviderResponse) {
		t.Errorf("err = %v, want permanent ErrProviderResponse (Moonshot quota code)", err)
	}
	var rle *model.RateLimitError
	if errors.As(err, &rle) {
		t.Errorf("quota exhaustion must not carry a RateLimitError: %v", err)
	}
}

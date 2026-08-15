// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/bus"
	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/tool"
)

// Estreno E-4/E-5 (adversarial H2/H7, red-team RT-2/RT-11): native-lane
// hardening — an empty tool result must not become an empty RoleTool turn
// (ValidateRequest would refuse the whole next request), an empty registry
// must not teach the TOOL: grammar, a rescued text call must not replay its
// raw JSON into the context, and one response cannot execute unbounded calls.

// emptyResultTool returns "" successfully — http_fetch on a 200 with an
// empty body, read_file on a 0-byte file.
type emptyResultTool struct{ spyTool }

func (e *emptyResultTool) Name() string { return "empty" }
func (e *emptyResultTool) Execute(ctx context.Context, args string) (string, error) {
	_, _ = e.spyTool.Execute(ctx, args)
	return "", nil
}

func TestNativeLane_emptyToolResultYieldsNonEmptyToolTurn(t *testing.T) {
	t.Parallel()
	et := &emptyResultTool{}
	m := &nativeScriptedModel{name: "n", replies: []model.Message{
		toolCallReply("empty", "x"), finalReply("done"),
	}}
	a := NewAgentBrain(m, tool.Registry{"empty": et}, WithAgentLogger(quietLogger()))

	out, err := a.Handle(context.Background(), inboundText("console", "c", "go"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("no reply")
	}
	var toolTurn *model.Message
	for i := range m.lastReq.Messages {
		if m.lastReq.Messages[i].Role == model.RoleTool {
			toolTurn = &m.lastReq.Messages[i]
		}
	}
	if toolTurn == nil {
		t.Fatal("no RoleTool turn appended")
	}
	if toolTurn.Content == "" {
		t.Fatal("empty RoleTool turn — the next real-adapter call would fail ValidateRequest (ErrEmptyContent) and the user would get the canned fallback")
	}
}

func TestSystemPrompt_emptyRegistrySkipsToolProtocol(t *testing.T) {
	t.Parallel()
	got := buildSystemPrompt(tool.Registry{}, "be helpful")
	if strings.Contains(got, "TOOL:") || strings.Contains(got, "Available tools") {
		t.Fatalf("empty registry still teaches the tool grammar — an invitation to hallucinate calls:\n%s", got)
	}
	if !strings.Contains(got, "be helpful") {
		t.Fatalf("operator prompt missing: %q", got)
	}
}

func TestNativeLane_rescuedCallBlanksItsRawJSONInContext(t *testing.T) {
	t.Parallel()
	spy := &spyTool{}
	blob := `{"name":"spy","args":"launch"}`
	m := &nativeScriptedModel{name: "n", replies: []model.Message{
		finalReply(blob), finalReply("done"),
	}}
	a := NewAgentBrain(m, spyRegistry(spy), WithAgentLogger(quietLogger()))

	if _, err := a.Handle(context.Background(), inboundText("console", "c", "go")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if spy.count() != 1 {
		t.Fatalf("rescued call executed %d times, want 1", spy.count())
	}
	for _, msg := range m.lastReq.Messages {
		if msg.Role == model.RoleAssistant && strings.Contains(msg.Content, `"name"`) {
			t.Fatalf("the raw printed JSON was replayed into the context as the model's own words: %q — the exact failure mode the clean-context fix targeted", msg.Content)
		}
	}
}

func TestNativeLane_perTurnToolCallCap(t *testing.T) {
	t.Parallel()
	spy := &spyTool{}
	pub := &spyPublisher{}
	calls := make([]model.ToolCall, maxNativeCallsPerTurn+2)
	for i := range calls {
		calls[i] = model.ToolCall{Name: "spy", Arguments: map[string]any{"a": fmt.Sprintf("%d", i)}}
	}
	m := &nativeScriptedModel{name: "n", replies: []model.Message{
		{Role: model.RoleAssistant, ToolCalls: calls}, finalReply("done"),
	}}
	a := NewAgentBrain(m, spyRegistry(spy), WithAgentLogger(quietLogger()),
		WithAgentToolAudit(pub, "agent-1"))

	if _, err := a.Handle(context.Background(), inboundText("console", "c", "go")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got := spy.count(); got != maxNativeCallsPerTurn {
		t.Fatalf("executed %d calls in one turn, want the cap (%d)", got, maxNativeCallsPerTurn)
	}
	// The HISTORY is bounded too (re-review follow-up): the assistant turn
	// keeps exactly the cap, and the discard is ONE aggregated note.
	var assistantCalls, toolTurns, budgetNotes int
	for _, msg := range m.lastReq.Messages {
		if msg.Role == model.RoleAssistant && len(msg.ToolCalls) > 0 {
			assistantCalls = len(msg.ToolCalls)
		}
		if msg.Role == model.RoleTool {
			toolTurns++
			if strings.Contains(msg.Content, "budget") {
				budgetNotes++
			}
		}
	}
	if assistantCalls != maxNativeCallsPerTurn {
		t.Fatalf("assistant turn in history carries %d calls, want the cap (%d)", assistantCalls, maxNativeCallsPerTurn)
	}
	if toolTurns != maxNativeCallsPerTurn+1 || budgetNotes != 1 {
		t.Fatalf("tool turns = %d (budget notes %d), want cap+1 with ONE aggregated note", toolTurns, budgetNotes)
	}
	// The discard audits once, with FINITE labels.
	var overflowEvents int
	for _, ev := range pub.snapshot() {
		if ev.Type == bus.ToolDenied && ev.Rule == "per_turn_budget" && ev.Tool == "overflow" {
			overflowEvents++
		}
	}
	if overflowEvents != 1 {
		t.Fatalf("per_turn_budget audits = %d, want exactly 1", overflowEvents)
	}
}

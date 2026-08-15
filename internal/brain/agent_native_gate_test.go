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
	"github.com/Sebastian197/korvun/internal/policy"
	"github.com/Sebastian197/korvun/internal/tool"
)

// strictParamTool is a ParamTool whose ArgsFromCall enforces its required
// field — the fixture the gate-vs-field-error contract needs (paramSpyTool
// deliberately never errors).
type strictParamTool struct{ paramSpyTool }

func (p *strictParamTool) ArgsFromCall(fields map[string]any) (string, error) {
	u, _ := fields["url"].(string)
	if u == "" {
		return "", fmt.Errorf("the url field is required")
	}
	m, _ := fields["message"].(string)
	return u + " | " + m, nil
}

// Estreno E-3 (adversarial H1 + red-team RT-10): governance must rule BEFORE
// argument parsing, and every tool-call attempt must be visible on the audit
// surfaces — including a ParamTool field error and a hallucinated tool name.

// A SHADOWED ParamTool called with a MISSING required field must still
// produce the shadow observation and the tool_shadowed audit — the field
// error must not smuggle the call past the gate's observation.
func TestNativeLane_shadowedParamToolBadArgsStillShadows(t *testing.T) {
	t.Parallel()
	spy := &strictParamTool{}
	pub := &spyPublisher{}
	met := &auditMetrics{}
	g := governanceFor(
		[]policy.ToolGrant{{Name: spy.Name(), Mode: policy.ToolShadow}},
		nil, policy.Public, policy.Local)
	m := &nativeScriptedModel{name: "n", replies: []model.Message{
		{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{
			Name:      spy.Name(),
			Arguments: map[string]any{"message": "hola"}, // url MISSING
		}}},
		finalReply("done"),
	}}
	a := NewAgentBrain(m, tool.Registry{spy.Name(): spy},
		WithAgentLogger(quietLogger()), WithAgentMetrics(met),
		WithAgentToolAudit(pub, "agent-1"), WithAgentGovernance(g))

	if _, err := a.Handle(context.Background(), inboundText("console", "c", "go")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if n := len(spy.executed()); n != 0 {
		t.Fatalf("shadowed tool executed %d times, want 0", n)
	}
	var toolTurn string
	for _, msg := range m.lastReq.Messages {
		if msg.Role == model.RoleTool {
			toolTurn = msg.Content
		}
	}
	if !strings.Contains(toolTurn, "was NOT executed") {
		t.Fatalf("observation %q is not the shadow simulation — the field error bypassed the gate", toolTurn)
	}
	events := pub.snapshot()
	if len(events) != 1 || events[0].Type != bus.ToolShadowed {
		t.Fatalf("audit = %+v, want exactly one tool_shadowed", events)
	}
}

// An ALLOWED ParamTool whose args fail the field contract must audit the
// attempt (tool_used outcome=error) — the known ParamTool audit gap.
func TestNativeLane_allowedParamToolBadArgsAudits(t *testing.T) {
	t.Parallel()
	spy := &strictParamTool{}
	pub := &spyPublisher{}
	met := &auditMetrics{}
	g := governanceFor(
		[]policy.ToolGrant{{Name: spy.Name(), Mode: policy.ToolAllow}},
		nil, policy.Public, policy.Local)
	m := &nativeScriptedModel{name: "n", replies: []model.Message{
		{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{
			Name:      spy.Name(),
			Arguments: map[string]any{"message": "hola"}, // url MISSING
		}}},
		finalReply("done"),
	}}
	a := NewAgentBrain(m, tool.Registry{spy.Name(): spy},
		WithAgentLogger(quietLogger()), WithAgentMetrics(met),
		WithAgentToolAudit(pub, "agent-1"), WithAgentGovernance(g))

	if _, err := a.Handle(context.Background(), inboundText("console", "c", "go")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if n := len(spy.executed()); n != 0 {
		t.Fatalf("tool with bad args executed %d times, want 0", n)
	}
	events := pub.snapshot()
	if len(events) != 1 || events[0].Type != bus.ToolUsed || events[0].Outcome != "error" {
		t.Fatalf("audit = %+v, want exactly one tool_used outcome=error", events)
	}
}

// A hallucinated call to a tool that exists in NO registry must audit as a
// denial (rule unknown_tool) instead of vanishing from all three surfaces.
func TestNativeLane_unknownToolAuditsDenied(t *testing.T) {
	t.Parallel()
	pub := &spyPublisher{}
	met := &auditMetrics{}
	g := governanceFor(
		[]policy.ToolGrant{{Name: "spy", Mode: policy.ToolAllow}},
		nil, policy.Public, policy.Local)
	m := &nativeScriptedModel{name: "n", replies: []model.Message{
		toolCallReply("ghost", "boo"), finalReply("done"),
	}}
	a := NewAgentBrain(m, spyRegistry(&spyTool{}),
		WithAgentLogger(quietLogger()), WithAgentMetrics(met),
		WithAgentToolAudit(pub, "agent-1"), WithAgentGovernance(g))

	if _, err := a.Handle(context.Background(), inboundText("console", "c", "go")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	events := pub.snapshot()
	if len(events) != 1 || events[0].Type != bus.ToolDenied || events[0].Rule != "unknown_tool" {
		t.Fatalf("audit = %+v, want exactly one tool_denied rule=unknown_tool", events)
	}
	// The MODEL controls the hallucinated name: shared surfaces (metrics
	// labels, bus, /tools, SSE) must see a FINITE category, never the raw
	// name — unbounded label cardinality and prompt content in a
	// metadata-only surface (re-review follow-up).
	if events[0].Tool != "unknown" {
		t.Fatalf("event Tool = %q, want the finite category %q", events[0].Tool, "unknown")
	}
	if got := met.toolUseSnapshot(); len(got) != 1 || got[0].tool != "unknown" {
		t.Fatalf("metric tool label = %+v, want [unknown]", got)
	}
}

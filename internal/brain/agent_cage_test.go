// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"fmt"
	"testing"

	"github.com/Sebastian197/korvun/internal/bus"
	"github.com/Sebastian197/korvun/internal/tool"
)

// Cage-violation classification (ADR-0041 §4/§5, mandate SP3): a tool error
// wrapping tool.ErrCageViolation audits as tool_denied with rule "cage" (and
// tool.ErrShieldViolation as rule "private_network_shield") — NOT as a
// tool_used with outcome error. The model still receives the tool's honest
// error observation (allow-list named, AS-6); only the audit classification
// changes.

// cagedFailTool always fails with the given sentinel wrapped.
type cagedFailTool struct{ sentinel error }

func (c cagedFailTool) Name() string        { return "caged" }
func (c cagedFailTool) Description() string { return "always violates its cage. args ignored." }
func (c cagedFailTool) Execute(context.Context, string) (string, error) {
	return "", fmt.Errorf("caged: host not in allow-list: %w", c.sentinel)
}

func runCagedTool(t *testing.T, sentinel error) (*spyPublisher, *auditMetrics, *scriptedModel) {
	t.Helper()
	pub := &spyPublisher{}
	met := &auditMetrics{}
	m := &scriptedModel{name: "m", replies: []string{"TOOL: caged(x)", "done"}}
	a := NewAgentBrain(m, tool.Registry{"caged": cagedFailTool{sentinel: sentinel}},
		WithAgentLogger(quietLogger()), WithAgentMetrics(met), WithAgentToolAudit(pub, "agent-1"))
	if _, err := a.Handle(context.Background(), inboundText("telegram", "c", "go")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	return pub, met, m
}

func TestAgentCage_violationAuditsAsDeniedWithCageRule(t *testing.T) {
	t.Parallel()
	pub, met, m := runCagedTool(t, tool.ErrCageViolation)

	events := pub.snapshot()
	if len(events) != 1 {
		t.Fatalf("published %d events, want 1: %+v", len(events), events)
	}
	ev := events[0]
	if ev.Type != bus.ToolDenied || ev.Outcome != "denied" || ev.Rule != "cage" {
		t.Fatalf("cage violation misclassified: %+v (want ToolDenied/denied/cage)", ev)
	}
	if uses := met.toolUseSnapshot(); len(uses) != 1 || uses[0] != (toolUseRecord{tool: "caged", outcome: "denied"}) {
		t.Fatalf("metric observations = %+v, want one (caged, denied)", uses)
	}
	// The model still sees the tool's honest failure observation.
	if !requestHasObservationContaining(m.lastReq, "not in allow-list") {
		t.Fatalf("model did not receive the tool's error observation: %+v", m.lastReq)
	}
}

func TestAgentCage_shieldViolationAuditsWithShieldRule(t *testing.T) {
	t.Parallel()
	pub, met, _ := runCagedTool(t, tool.ErrShieldViolation)

	events := pub.snapshot()
	if len(events) != 1 {
		t.Fatalf("published %d events, want 1: %+v", len(events), events)
	}
	ev := events[0]
	if ev.Type != bus.ToolDenied || ev.Outcome != "denied" || ev.Rule != "private_network_shield" {
		t.Fatalf("shield violation misclassified: %+v (want ToolDenied/denied/private_network_shield)", ev)
	}
	if uses := met.toolUseSnapshot(); len(uses) != 1 || uses[0] != (toolUseRecord{tool: "caged", outcome: "denied"}) {
		t.Fatalf("metric observations = %+v, want one (caged, denied)", uses)
	}
}

// An ORDINARY tool error (no sentinel) keeps auditing as tool_used/error —
// the SP2 contract unchanged.
func TestAgentCage_ordinaryErrorStaysToolUsed(t *testing.T) {
	t.Parallel()
	pub, _, _ := runCagedTool(t, errOrdinary)

	events := pub.snapshot()
	if len(events) != 1 || events[0].Type != bus.ToolUsed || events[0].Outcome != "error" {
		t.Fatalf("ordinary error misclassified: %+v (want ToolUsed/error)", events)
	}
}

// errOrdinary is a plain error with no cage/shield sentinel behind it.
var errOrdinary = fmt.Errorf("plain failure")

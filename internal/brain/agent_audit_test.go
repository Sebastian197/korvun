// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/bus"
	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/policy"
	"github.com/Sebastian197/korvun/internal/tool"
)

// The audit half of the gate (ADR-0041 §5, spec FR-AUD-1/2/4, AS-3): every
// tool execution, denial, and shadow rehearsal publishes exactly one
// metadata-only bus event and records one metric observation. The bounded
// args prefix lives in slog ONLY — the Event has no args field by
// construction, so these tests assert the metadata is complete, not that
// args are absent (the type system already guarantees that).

// spyPublisher records published events; mutex-guarded so the AS-3 concurrent
// shape can share one instance.
type spyPublisher struct {
	mu     sync.Mutex
	events []bus.Event
}

func (p *spyPublisher) Publish(_ context.Context, ev bus.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, ev)
}

func (p *spyPublisher) snapshot() []bus.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]bus.Event, len(p.events))
	copy(out, p.events)
	return out
}

// toolUseRecord is one ObserveToolUse observation.
type toolUseRecord struct {
	tool    string
	outcome string
}

// auditMetrics extends the package's recordingMetrics shape with the tool
// observation, mutex-guarded for the concurrent test.
type auditMetrics struct {
	recordingMetrics
	mu       sync.Mutex
	toolUses []toolUseRecord
}

func (m *auditMetrics) ObserveToolUse(toolName, outcome string, _ time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolUses = append(m.toolUses, toolUseRecord{tool: toolName, outcome: outcome})
}

func (m *auditMetrics) toolUseSnapshot() []toolUseRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]toolUseRecord, len(m.toolUses))
	copy(out, m.toolUses)
	return out
}

// failingTool always errors, to drive the "error" outcome.
type failingTool struct{}

func (failingTool) Name() string        { return "boom" }
func (failingTool) Description() string { return "always fails. args ignored." }
func (failingTool) Execute(context.Context, string) (string, error) {
	return "", errors.New("boom: deliberate failure")
}

// onceToolModel answers "TOOL: <name>(x)" until the request carries an
// OBSERVATION, then answers "done". Unlike scriptedModel it is stateless per
// request, so under CONCURRENT Handles every Handle performs EXACTLY ONE tool
// round-trip regardless of interleaving — the property AS-3 counts on.
type onceToolModel struct{ toolName string }

func (m *onceToolModel) Generate(_ context.Context, req *model.Request) (*model.Response, error) {
	reply := "TOOL: " + m.toolName + "(x)"
	for _, msg := range req.Messages {
		if strings.HasPrefix(msg.Content, observationPrefix) {
			reply = "done"
			break
		}
	}
	return &model.Response{
		Message:  model.Message{Role: model.RoleAssistant, Content: reply},
		Provider: "once",
	}, nil
}

func (m *onceToolModel) Name() string { return "once" }

// auditedBrain builds an AgentBrain with the audit seam mounted.
func auditedBrain(m model.Model, reg tool.Registry, pub *spyPublisher, met *auditMetrics, extra ...AgentOption) *AgentBrain {
	opts := append([]AgentOption{
		WithAgentLogger(quietLogger()),
		WithAgentMetrics(met),
		WithAgentToolAudit(pub, "agent-1"),
	}, extra...)
	return NewAgentBrain(m, reg, opts...)
}

// A successful execution publishes exactly one tool_used with full metadata
// and records one ("spy", "ok") observation.
func TestAgentAudit_usedEvent(t *testing.T) {
	t.Parallel()
	spy := &spyTool{}
	pub := &spyPublisher{}
	met := &auditMetrics{}
	a := auditedBrain(&onceToolModel{toolName: "spy"}, spyRegistry(spy), pub, met)

	env := inboundText("telegram", "c", "go")
	if _, err := a.Handle(context.Background(), env); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	events := pub.snapshot()
	if len(events) != 1 {
		t.Fatalf("published %d events, want exactly 1: %+v", len(events), events)
	}
	ev := events[0]
	if ev.Type != bus.ToolUsed {
		t.Fatalf("event type = %v, want ToolUsed", ev.Type)
	}
	if ev.Tool != "spy" || ev.Outcome != "ok" || ev.Rule != "" {
		t.Fatalf("tool metadata wrong: %+v", ev)
	}
	if ev.Brain != "agent-1" || ev.Channel != "telegram" {
		t.Fatalf("routing metadata wrong: %+v", ev)
	}
	if ev.Envelope == nil || ev.Envelope.ID != env.ID {
		t.Fatalf("event does not reference the inbound envelope: %+v", ev)
	}
	if ev.Latency < 0 {
		t.Fatalf("negative latency: %v", ev.Latency)
	}
	uses := met.toolUseSnapshot()
	if len(uses) != 1 || uses[0] != (toolUseRecord{tool: "spy", outcome: "ok"}) {
		t.Fatalf("metric observations = %+v, want exactly one (spy, ok)", uses)
	}
}

// A tool that returns an error is still a USE — outcome "error" — and the
// model keeps receiving the failure observation (ADR-0021 §2 unchanged).
func TestAgentAudit_usedErrorOutcome(t *testing.T) {
	t.Parallel()
	pub := &spyPublisher{}
	met := &auditMetrics{}
	a := auditedBrain(&onceToolModel{toolName: "boom"}, tool.Registry{"boom": failingTool{}}, pub, met)

	if _, err := a.Handle(context.Background(), inboundText("telegram", "c", "go")); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	events := pub.snapshot()
	if len(events) != 1 || events[0].Type != bus.ToolUsed || events[0].Outcome != "error" {
		t.Fatalf("events = %+v, want one ToolUsed with outcome error", events)
	}
	uses := met.toolUseSnapshot()
	if len(uses) != 1 || uses[0] != (toolUseRecord{tool: "boom", outcome: "error"}) {
		t.Fatalf("metric observations = %+v, want exactly one (boom, error)", uses)
	}
}

// A gate denial publishes tool_denied carrying the denying RULE (the rule goes
// to the audit surfaces, never to the model — coherent with ADR-0041 §2).
func TestAgentAudit_deniedEventCarriesRule(t *testing.T) {
	t.Parallel()
	spy := &spyTool{}
	pub := &spyPublisher{}
	met := &auditMetrics{}
	g := governanceFor(
		[]policy.ToolGrant{{Name: "spy", Mode: policy.ToolAllow, Channels: []string{"telegram"}}},
		nil, policy.Public, policy.Local)
	a := auditedBrain(&onceToolModel{toolName: "spy"}, spyRegistry(spy), pub, met, WithAgentGovernance(g))

	if _, err := a.Handle(context.Background(), inboundText("discord", "c", "go")); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	events := pub.snapshot()
	if len(events) != 1 {
		t.Fatalf("published %d events, want exactly 1: %+v", len(events), events)
	}
	ev := events[0]
	if ev.Type != bus.ToolDenied || ev.Outcome != "denied" || ev.Rule != string(policy.ToolRuleChannel) {
		t.Fatalf("denied event wrong: %+v", ev)
	}
	if got := spy.count(); got != 0 {
		t.Fatalf("denied tool executed %d times, want 0", got)
	}
	uses := met.toolUseSnapshot()
	if len(uses) != 1 || uses[0] != (toolUseRecord{tool: "spy", outcome: "denied"}) {
		t.Fatalf("metric observations = %+v, want exactly one (spy, denied)", uses)
	}
}

// A shadow rehearsal publishes tool_shadowed (FR-AUD-4) — never executed.
func TestAgentAudit_shadowedEvent(t *testing.T) {
	t.Parallel()
	spy := &spyTool{}
	pub := &spyPublisher{}
	met := &auditMetrics{}
	g := governanceFor(
		[]policy.ToolGrant{{Name: "spy", Mode: policy.ToolShadow}},
		nil, policy.Public, policy.Local)
	a := auditedBrain(&onceToolModel{toolName: "spy"}, spyRegistry(spy), pub, met, WithAgentGovernance(g))

	if _, err := a.Handle(context.Background(), inboundText("telegram", "c", "go")); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	events := pub.snapshot()
	if len(events) != 1 || events[0].Type != bus.ToolShadowed || events[0].Outcome != "shadowed" {
		t.Fatalf("events = %+v, want one ToolShadowed", events)
	}
	if got := spy.count(); got != 0 {
		t.Fatalf("shadowed tool executed %d times, want 0", got)
	}
	uses := met.toolUseSnapshot()
	if len(uses) != 1 || uses[0] != (toolUseRecord{tool: "spy", outcome: "shadowed"}) {
		t.Fatalf("metric observations = %+v, want exactly one (spy, shadowed)", uses)
	}
}

// No publisher mounted → no panic, no events, behavior unchanged (the audit
// seam is optional exactly like metrics.Nop).
func TestAgentAudit_noPublisherIsSafe(t *testing.T) {
	t.Parallel()
	spy := &spyTool{}
	a := NewAgentBrain(&onceToolModel{toolName: "spy"}, spyRegistry(spy), WithAgentLogger(quietLogger()))

	if _, err := a.Handle(context.Background(), inboundText("telegram", "c", "go")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got := spy.count(); got != 1 {
		t.Fatalf("tool executed %d times, want 1", got)
	}
}

// AS-3 in its mandatory shape: CONCURRENT Handles over ONE shared AgentBrain
// under -race, asserting EXACTLY one tool_used per use with its metadata.
func TestAgentAudit_concurrentExactlyOneEventPerUse(t *testing.T) {
	t.Parallel()
	const workers = 8
	spy := &spyTool{}
	pub := &spyPublisher{}
	met := &auditMetrics{}
	a := auditedBrain(&onceToolModel{toolName: "spy"}, spyRegistry(spy), pub, met)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := a.Handle(context.Background(), inboundText("telegram", "c", "go")); err != nil {
				t.Errorf("Handle: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := spy.count(); got != workers {
		t.Fatalf("tool executed %d times, want %d", got, workers)
	}
	events := pub.snapshot()
	if len(events) != workers {
		t.Fatalf("published %d events, want exactly %d (one per use)", len(events), workers)
	}
	for i, ev := range events {
		if ev.Type != bus.ToolUsed || ev.Tool != "spy" || ev.Outcome != "ok" ||
			ev.Brain != "agent-1" || ev.Channel != "telegram" {
			t.Fatalf("event %d metadata wrong: %+v", i, ev)
		}
	}
	if uses := met.toolUseSnapshot(); len(uses) != workers {
		t.Fatalf("metric observations = %d, want %d", len(uses), workers)
	}
}

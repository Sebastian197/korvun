// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/Sebastian197/korvun/internal/bus"
	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/policy"
	"github.com/Sebastian197/korvun/internal/tool"
)

// The native tool-calling lane in the AgentBrain (ADR-0042 §5): lane picked
// by capability assertion; the gate, shadow, cages, shield, and audit are
// IDENTICAL by construction (every native tool_call routes through the same
// runTool). These tests pin exactly that.

// nativeScriptedModel is a ToolCallingModel returning a fixed sequence of
// native replies, capturing every request and the advertised specs.
type nativeScriptedModel struct {
	name    string
	replies []model.Message

	mu        sync.Mutex
	calls     int
	lastReq   *model.Request
	lastSpecs []model.ToolSpec
}

func (m *nativeScriptedModel) Generate(_ context.Context, req *model.Request) (*model.Response, error) {
	return &model.Response{Message: model.Message{Role: model.RoleAssistant, Content: "old-lane"}, Provider: m.name}, nil
}
func (m *nativeScriptedModel) Name() string { return m.name }
func (m *nativeScriptedModel) GenerateWithTools(_ context.Context, req *model.Request, specs []model.ToolSpec) (*model.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastReq = req
	m.lastSpecs = specs
	i := m.calls
	m.calls++
	if i >= len(m.replies) {
		i = len(m.replies) - 1
	}
	return &model.Response{Message: m.replies[i], Provider: m.name}, nil
}

// toolCallReply builds an assistant reply requesting one tool.
func toolCallReply(name, args string) model.Message {
	return model.Message{Role: model.RoleAssistant,
		ToolCalls: []model.ToolCall{{Name: name, Arguments: map[string]any{"args": args}}}}
}

func finalReply(text string) model.Message {
	return model.Message{Role: model.RoleAssistant, Content: text}
}

func specNames(specs []model.ToolSpec) []string {
	out := make([]string, len(specs))
	for i, s := range specs {
		out[i] = s.Name
	}
	return out
}

// The lane is picked by capability: a native model never sees the textual
// grammar, and receives the advertised registry as structured specs.
func TestNativeLane_pickedAndGrammarFree(t *testing.T) {
	t.Parallel()
	spy := &spyTool{}
	m := &nativeScriptedModel{name: "n", replies: []model.Message{finalReply("done")}}
	a := NewAgentBrain(m, spyRegistry(spy), WithAgentLogger(quietLogger()))

	out, err := a.Handle(context.Background(), inboundText("console", "c", "hola"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(out) != 1 || out[0].Parts[0].Content != "done" {
		t.Fatalf("got %+v, want the native final answer", out)
	}
	for _, msg := range m.lastReq.Messages {
		if strings.Contains(msg.Content, "You can use tools") {
			t.Fatalf("native lane leaked the textual grammar:\n%s", msg.Content)
		}
	}
	if got := specNames(m.lastSpecs); len(got) != 1 || got[0] != "spy" {
		t.Fatalf("specs = %v, want [spy]", got)
	}
}

// Governance at advertisement: only allow ∪ shadow tools become specs; a
// channel-denied tool is never announced (AS-1 native).
func TestNativeLane_governanceFiltersSpecs(t *testing.T) {
	t.Parallel()
	spy := &spyTool{}
	reg := tool.Registry{"spy": spy, "calc": tool.Calc(), "echo": tool.Echo()}
	g := governanceFor(
		[]policy.ToolGrant{
			{Name: "spy", Mode: policy.ToolAllow, Channels: []string{"console"}},
			{Name: "calc", Mode: policy.ToolShadow},
			{Name: "echo", Mode: policy.ToolDeny},
		},
		nil, policy.Public, policy.Local)
	m := &nativeScriptedModel{name: "n", replies: []model.Message{finalReply("done")}}
	a := NewAgentBrain(m, reg, WithAgentLogger(quietLogger()), WithAgentGovernance(g))

	if _, err := a.Handle(context.Background(), inboundText("discord", "c", "hola")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	got := specNames(m.lastSpecs)
	if len(got) != 1 || got[0] != "calc" {
		t.Fatalf("specs on discord = %v, want [calc] (spy channel-denied, echo deny-granted, shadow announced)", got)
	}
}

// A permitted call executes through the SAME runTool: the result returns as
// a RoleTool turn and audits tool_used (AS-3 native).
func TestNativeLane_allowedCallExecutesAndAudits(t *testing.T) {
	t.Parallel()
	spy := &spyTool{}
	pub := &spyPublisher{}
	met := &auditMetrics{}
	m := &nativeScriptedModel{name: "n", replies: []model.Message{
		toolCallReply("spy", "x"), finalReply("done"),
	}}
	a := NewAgentBrain(m, spyRegistry(spy),
		WithAgentLogger(quietLogger()), WithAgentMetrics(met), WithAgentToolAudit(pub, "agent-1"))

	out, err := a.Handle(context.Background(), inboundText("console", "c", "usa spy"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(out) != 1 || out[0].Parts[0].Content != "done" {
		t.Fatalf("got %+v, want the final answer", out)
	}
	if spy.count() != 1 {
		t.Fatalf("spy executed %d times, want 1", spy.count())
	}
	// The second request carries the cycle turns: assistant tool_calls, then
	// the RoleTool result.
	var sawToolTurn bool
	for _, msg := range m.lastReq.Messages {
		if msg.Role == model.RoleTool {
			sawToolTurn = true
			if msg.ToolName != "spy" || msg.Content != "spied:x" {
				t.Fatalf("tool turn wrong: %+v", msg)
			}
		}
	}
	if !sawToolTurn {
		t.Fatalf("no RoleTool turn reached the model: %+v", m.lastReq.Messages)
	}
	events := pub.snapshot()
	if len(events) != 1 || events[0].Type != bus.ToolUsed || events[0].Outcome != "ok" {
		t.Fatalf("audit = %+v, want one tool_used ok", events)
	}
}

// Shadow on the native lane: announced in specs, NEVER executed, the
// simulation observation returns as the tool turn, tool_shadowed audits
// (AS-8 native).
func TestNativeLane_shadowAnnouncedNeverExecuted(t *testing.T) {
	t.Parallel()
	spy := &spyTool{}
	pub := &spyPublisher{}
	met := &auditMetrics{}
	g := governanceFor(
		[]policy.ToolGrant{{Name: "spy", Mode: policy.ToolShadow}},
		nil, policy.Public, policy.Local)
	m := &nativeScriptedModel{name: "n", replies: []model.Message{
		toolCallReply("spy", "launch"), finalReply("done"),
	}}
	a := NewAgentBrain(m, spyRegistry(spy),
		WithAgentLogger(quietLogger()), WithAgentMetrics(met),
		WithAgentToolAudit(pub, "agent-1"), WithAgentGovernance(g))

	if _, err := a.Handle(context.Background(), inboundText("console", "c", "go")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got := specNames(m.lastSpecs); len(got) != 1 || got[0] != "spy" {
		t.Fatalf("shadow tool must be announced in specs: %v", got)
	}
	if spy.count() != 0 {
		t.Fatalf("shadowed tool executed %d times, want 0", spy.count())
	}
	var toolTurn string
	for _, msg := range m.lastReq.Messages {
		if msg.Role == model.RoleTool {
			toolTurn = msg.Content
		}
	}
	if !strings.Contains(toolTurn, "was NOT executed") {
		t.Fatalf("simulation observation missing from the tool turn: %q", toolTurn)
	}
	events := pub.snapshot()
	if len(events) != 1 || events[0].Type != bus.ToolShadowed {
		t.Fatalf("audit = %+v, want one tool_shadowed", events)
	}
}

// A hallucinated call to a non-advertised (denied) tool is refused at
// execution and audited — the second gate point holds on the native lane.
func TestNativeLane_deniedCallRefusedAndAudited(t *testing.T) {
	t.Parallel()
	spy := &spyTool{}
	pub := &spyPublisher{}
	met := &auditMetrics{}
	g := governanceFor(
		[]policy.ToolGrant{{Name: "spy", Mode: policy.ToolAllow, Channels: []string{"telegram"}}},
		nil, policy.Public, policy.Local)
	m := &nativeScriptedModel{name: "n", replies: []model.Message{
		toolCallReply("spy", "x"), finalReply("done"),
	}}
	a := NewAgentBrain(m, spyRegistry(spy),
		WithAgentLogger(quietLogger()), WithAgentMetrics(met),
		WithAgentToolAudit(pub, "agent-1"), WithAgentGovernance(g))

	if _, err := a.Handle(context.Background(), inboundText("console", "c", "go")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if spy.count() != 0 {
		t.Fatalf("denied tool executed %d times, want 0", spy.count())
	}
	events := pub.snapshot()
	if len(events) != 1 || events[0].Type != bus.ToolDenied || events[0].Rule != string(policy.ToolRuleChannel) {
		t.Fatalf("audit = %+v, want one tool_denied rule channel", events)
	}
}

// A cage violation on the native lane audits as a denial with its rule.
func TestNativeLane_cageViolationAuditsAsDenied(t *testing.T) {
	t.Parallel()
	pub := &spyPublisher{}
	met := &auditMetrics{}
	m := &nativeScriptedModel{name: "n", replies: []model.Message{
		toolCallReply("caged", "x"), finalReply("done"),
	}}
	a := NewAgentBrain(m, tool.Registry{"caged": cagedFailTool{sentinel: tool.ErrShieldViolation}},
		WithAgentLogger(quietLogger()), WithAgentMetrics(met), WithAgentToolAudit(pub, "agent-1"))

	if _, err := a.Handle(context.Background(), inboundText("console", "c", "go")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	events := pub.snapshot()
	if len(events) != 1 || events[0].Type != bus.ToolDenied || events[0].Rule != "private_network_shield" {
		t.Fatalf("audit = %+v, want tool_denied private_network_shield", events)
	}
}

// The iteration cap bounds the native loop exactly like the old lane.
func TestNativeLane_iterationCap(t *testing.T) {
	t.Parallel()
	spy := &spyTool{}
	m := &nativeScriptedModel{name: "n", replies: []model.Message{
		toolCallReply("spy", "again"), // repeated forever
	}}
	a := NewAgentBrain(m, spyRegistry(spy),
		WithAgentLogger(quietLogger()), WithAgentMaxIterations(3))

	out, err := a.Handle(context.Background(), inboundText("console", "c", "go"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(out) != 1 || out[0].Parts[0].Content == "" {
		t.Fatalf("cap must degrade to the fallback reply, got %+v", out)
	}
	if m.calls != 3 {
		t.Fatalf("model called %d times, want exactly the cap (3)", m.calls)
	}
}

// A model WITHOUT the capability keeps today's prompt-protocol lane.
func TestNativeLane_fallbackToPromptProtocol(t *testing.T) {
	t.Parallel()
	spy := &spyTool{}
	m := &scriptedModel{name: "m", replies: []string{"TOOL: spy(x)", "done"}}
	a := NewAgentBrain(m, spyRegistry(spy), WithAgentLogger(quietLogger()))

	if _, err := a.Handle(context.Background(), inboundText("console", "c", "go")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if spy.count() != 1 {
		t.Fatalf("prompt-protocol lane broken: spy executed %d times, want 1", spy.count())
	}
	if !strings.Contains(systemPromptOf(t, m), "You can use tools") {
		t.Fatal("prompt-protocol lane lost its grammar")
	}
}

// Concurrent Handles over ONE native AgentBrain under -race, exactly one
// audit event per use (the ADR-0021 §5 mandatory shape on the new lane).
func TestNativeLane_concurrentHandles(t *testing.T) {
	t.Parallel()
	const workers = 8
	spy := &spyTool{}
	pub := &spyPublisher{}
	met := &auditMetrics{}
	m := &nativeScriptedModel{name: "n", replies: []model.Message{finalReply("done")}}
	a := NewAgentBrain(m, spyRegistry(spy),
		WithAgentLogger(quietLogger()), WithAgentMetrics(met), WithAgentToolAudit(pub, "agent-1"))

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := a.Handle(context.Background(), inboundText("console", "c", "go")); err != nil {
				t.Errorf("Handle: %v", err)
			}
		}()
	}
	wg.Wait()
}

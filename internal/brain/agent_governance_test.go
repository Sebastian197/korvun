// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/Sebastian197/korvun/internal/policy"
	"github.com/Sebastian197/korvun/internal/tool"
)

// The two-point gate under test (ADR-0041 §2, spec FR-GOV-3/5, AS-1/AS-2/
// AS-4/AS-8 at unit level): advertisement filters the system-prompt catalog,
// execution consults the same decisions; shadow is announced but NEVER
// executed; misconfigured governance fails CLOSED (deny-all).

// spyTool records executions so tests can assert a gated tool NEVER ran.
// Mutex-guarded per the Tool concurrency contract (ADR-0021 §4).
type spyTool struct {
	mu    sync.Mutex
	calls int
}

func (s *spyTool) Name() string        { return "spy" }
func (s *spyTool) Description() string { return "records executions. args = anything." }
func (s *spyTool) Execute(_ context.Context, args string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return "spied:" + args, nil
}

func (s *spyTool) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// spyRegistry returns a registry holding only the spy tool.
func spyRegistry(s *spyTool) tool.Registry { return tool.Registry{"spy": s} }

// governanceFor builds the option under test with sane defaults.
func governanceFor(grants []policy.ToolGrant, attrs map[string]policy.ToolAttrs, sens policy.Sensitivity, loc policy.Locality) *AgentGovernance {
	return &AgentGovernance{Grants: grants, Attrs: attrs, Sensitivity: sens, Locality: loc}
}

// systemPromptOf extracts the seed system message the model saw.
func systemPromptOf(t *testing.T, m *scriptedModel) string {
	t.Helper()
	if m.lastReq == nil || len(m.lastReq.Messages) == 0 {
		t.Fatalf("model saw no request")
	}
	return m.lastReq.Messages[0].Content
}

// AS-4 (unit): nil governance is byte-for-byte today's behavior — same
// system prompt, every registered tool executable.
func TestAgentGovernance_nil_isTodayByteForByte(t *testing.T) {
	t.Parallel()
	spy := &spyTool{}
	m := &scriptedModel{name: "m", replies: []string{"TOOL: spy(x)", "done"}}
	a := NewAgentBrain(m, spyRegistry(spy), WithAgentLogger(quietLogger()))

	out, err := a.Handle(context.Background(), inboundText("telegram", "c", "go"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(out) != 1 || out[0].Parts[0].Content != "done" {
		t.Fatalf("got %+v, want the final answer", out)
	}
	if got := spy.count(); got != 1 {
		t.Fatalf("spy executed %d times, want 1", got)
	}
	want := buildSystemPrompt(spyRegistry(spy), "")
	if got := systemPromptOf(t, m); got != want {
		t.Fatalf("system prompt drifted without governance:\ngot:  %q\nwant: %q", got, want)
	}
}

// AS-1 (unit): a channel-restricted grant on the wrong channel — not
// advertised, not executed, denial observation fed back, loop survives.
func TestAgentGovernance_channelDeny(t *testing.T) {
	t.Parallel()
	spy := &spyTool{}
	m := &scriptedModel{name: "m", replies: []string{"TOOL: spy(x)", "done"}}
	g := governanceFor(
		[]policy.ToolGrant{{Name: "spy", Mode: policy.ToolAllow, Channels: []string{"telegram"}}},
		nil, policy.Public, policy.Local)
	a := NewAgentBrain(m, spyRegistry(spy), WithAgentLogger(quietLogger()), WithAgentGovernance(g))

	out, err := a.Handle(context.Background(), inboundText("discord", "c", "go"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(out) != 1 || out[0].Parts[0].Content != "done" {
		t.Fatalf("got %+v, want the final answer after the denial observation", out)
	}
	if got := spy.count(); got != 0 {
		t.Fatalf("denied tool executed %d times, want 0", got)
	}
	if prompt := systemPromptOf(t, m); strings.Contains(prompt, "- spy:") {
		t.Fatalf("denied tool advertised in the system prompt:\n%s", prompt)
	}
	if !requestHasObservationContaining(m.lastReq, "not permitted") {
		t.Fatalf("model did not receive the denial observation: %+v", m.lastReq)
	}
}

// The same grant on the RIGHT channel advertises and executes.
func TestAgentGovernance_channelAllow(t *testing.T) {
	t.Parallel()
	spy := &spyTool{}
	m := &scriptedModel{name: "m", replies: []string{"TOOL: spy(x)", "done"}}
	g := governanceFor(
		[]policy.ToolGrant{{Name: "spy", Mode: policy.ToolAllow, Channels: []string{"telegram"}}},
		nil, policy.Public, policy.Local)
	a := NewAgentBrain(m, spyRegistry(spy), WithAgentLogger(quietLogger()), WithAgentGovernance(g))

	if _, err := a.Handle(context.Background(), inboundText("telegram", "c", "go")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got := spy.count(); got != 1 {
		t.Fatalf("allowed tool executed %d times, want 1", got)
	}
	if prompt := systemPromptOf(t, m); !strings.Contains(prompt, "- spy:") {
		t.Fatalf("allowed tool missing from the system prompt:\n%s", prompt)
	}
	if !requestHasObservationContaining(m.lastReq, "spied:x") {
		t.Fatalf("model did not receive the real observation: %+v", m.lastReq)
	}
}

// AS-2 (unit): a sensitive tool with a Cloud model is neither advertised nor
// executed; the same grant with a Local model works.
func TestAgentGovernance_sensitiveLocality(t *testing.T) {
	t.Parallel()

	run := func(t *testing.T, loc policy.Locality) (*spyTool, *scriptedModel) {
		t.Helper()
		spy := &spyTool{}
		m := &scriptedModel{name: "m", replies: []string{"TOOL: spy(x)", "done"}}
		g := governanceFor(
			[]policy.ToolGrant{{Name: "spy", Mode: policy.ToolAllow}},
			map[string]policy.ToolAttrs{"spy": {Sensitive: true}},
			policy.Public, loc)
		a := NewAgentBrain(m, spyRegistry(spy), WithAgentLogger(quietLogger()), WithAgentGovernance(g))
		if _, err := a.Handle(context.Background(), inboundText("telegram", "c", "go")); err != nil {
			t.Fatalf("Handle: %v", err)
		}
		return spy, m
	}

	t.Run("cloud model excludes the sensitive tool", func(t *testing.T) {
		t.Parallel()
		spy, m := run(t, policy.Cloud)
		if got := spy.count(); got != 0 {
			t.Fatalf("sensitive tool executed %d times on a cloud model, want 0", got)
		}
		if prompt := systemPromptOf(t, m); strings.Contains(prompt, "- spy:") {
			t.Fatalf("sensitive tool advertised to a cloud model:\n%s", prompt)
		}
	})
	t.Run("local model keeps the sensitive tool", func(t *testing.T) {
		t.Parallel()
		spy, m := run(t, policy.Local)
		if got := spy.count(); got != 1 {
			t.Fatalf("sensitive tool executed %d times on a local model, want 1", got)
		}
		if prompt := systemPromptOf(t, m); !strings.Contains(prompt, "- spy:") {
			t.Fatalf("sensitive tool missing from a local model's prompt:\n%s", prompt)
		}
	})
}

// AS-8 (unit): a shadow grant IS advertised, is NEVER executed, and feeds the
// exact simulation observation back; the loop reaches the final answer.
func TestAgentGovernance_shadowAnnouncedNeverExecuted(t *testing.T) {
	t.Parallel()
	spy := &spyTool{}
	m := &scriptedModel{name: "m", replies: []string{"TOOL: spy(launch it)", "done"}}
	g := governanceFor(
		[]policy.ToolGrant{{Name: "spy", Mode: policy.ToolShadow}},
		nil, policy.Public, policy.Local)
	a := NewAgentBrain(m, spyRegistry(spy), WithAgentLogger(quietLogger()), WithAgentGovernance(g))

	out, err := a.Handle(context.Background(), inboundText("telegram", "c", "go"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(out) != 1 || out[0].Parts[0].Content != "done" {
		t.Fatalf("got %+v, want the final answer after the simulation observation", out)
	}
	if got := spy.count(); got != 0 {
		t.Fatalf("shadowed tool executed %d times, want 0 — shadow must never execute", got)
	}
	if prompt := systemPromptOf(t, m); !strings.Contains(prompt, "- spy:") {
		t.Fatalf("shadowed tool must be advertised (rehearsal observes the model's judgment):\n%s", prompt)
	}
	if !requestHasObservationContaining(m.lastReq, "was NOT executed") {
		t.Fatalf("model did not receive the simulation observation: %+v", m.lastReq)
	}
}

// Misconfigured governance (duplicate grant) fails CLOSED: nothing advertised,
// nothing executed, the reply still degrades gracefully (D-6).
func TestAgentGovernance_misconfigFailsClosed(t *testing.T) {
	t.Parallel()
	spy := &spyTool{}
	m := &scriptedModel{name: "m", replies: []string{"TOOL: spy(x)", "done"}}
	g := governanceFor(
		[]policy.ToolGrant{
			{Name: "spy", Mode: policy.ToolAllow},
			{Name: "spy", Mode: policy.ToolAllow},
		},
		nil, policy.Public, policy.Local)
	a := NewAgentBrain(m, spyRegistry(spy), WithAgentLogger(quietLogger()), WithAgentGovernance(g))

	out, err := a.Handle(context.Background(), inboundText("telegram", "c", "go"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(out) != 1 || out[0].Parts[0].Content != "done" {
		t.Fatalf("got %+v, want a degraded-but-alive reply", out)
	}
	if got := spy.count(); got != 0 {
		t.Fatalf("tool executed %d times under fail-closed, want 0", got)
	}
	if prompt := systemPromptOf(t, m); strings.Contains(prompt, "- spy:") {
		t.Fatalf("fail-closed still advertised a tool:\n%s", prompt)
	}
	if !requestHasObservationContaining(m.lastReq, "not permitted") {
		t.Fatalf("model did not receive the denial observation under fail-closed: %+v", m.lastReq)
	}
}

// A tool absent from the REGISTRY keeps today's honest "not found"
// observation even under governance — nonexistence beats governance.
func TestAgentGovernance_unknownToolKeepsNotFound(t *testing.T) {
	t.Parallel()
	spy := &spyTool{}
	m := &scriptedModel{name: "m", replies: []string{"TOOL: ghost(x)", "done"}}
	g := governanceFor(
		[]policy.ToolGrant{{Name: "spy", Mode: policy.ToolAllow}},
		nil, policy.Public, policy.Local)
	a := NewAgentBrain(m, spyRegistry(spy), WithAgentLogger(quietLogger()), WithAgentGovernance(g))

	if _, err := a.Handle(context.Background(), inboundText("telegram", "c", "go")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !requestHasObservationContaining(m.lastReq, "not found") {
		t.Fatalf("unknown tool must keep the not-found observation: %+v", m.lastReq)
	}
}

// The mandatory concurrent shape (ADR-0021 §5) extended to the gate: N
// goroutines Handle over ONE governed AgentBrain; run under -race.
func TestAgentGovernance_concurrentHandle(t *testing.T) {
	t.Parallel()
	const workers = 8
	spy := &spyTool{}
	m := &scriptedModel{name: "m", replies: []string{"TOOL: spy(x)", "done"}}
	g := governanceFor(
		[]policy.ToolGrant{{Name: "spy", Mode: policy.ToolAllow}},
		nil, policy.Public, policy.Local)
	a := NewAgentBrain(m, spyRegistry(spy), WithAgentLogger(quietLogger()), WithAgentGovernance(g))

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// The scripted model is shared: replies interleave across workers,
			// so we only assert absence of races and loop termination here.
			_, _ = a.Handle(context.Background(), inboundText("telegram", "c", "go"))
		}()
	}
	wg.Wait()
}

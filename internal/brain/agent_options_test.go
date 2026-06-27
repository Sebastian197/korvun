// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/tool"
)

func agentFixedClock() time.Time { return time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC) }

// TestAgentBrain_options exercises every construction option through behavior: a
// custom fallback on the iteration cap, the operator system prompt landing in the
// seed system message, the injected clock stamping persisted turns, and the
// metrics funnels firing (ADR-0021 §2, §3.1, §6).
func TestAgentBrain_options_systemPromptAndMetrics(t *testing.T) {
	t.Parallel()
	rec := &recordingMetrics{}
	store := conversation.NewMemStore()
	m := &recordingModel{name: "m", response: "done"} // answers immediately, records messages

	a := NewAgentBrain(m, builtinRegistry(),
		WithAgentLogger(quietLogger()),
		WithAgentSystemPrompt("OPERATOR-RULES"),
		WithAgentMetrics(rec),
		WithAgentClock(agentFixedClock),
		WithAgentPerModelTimeout(time.Second),
		WithAgentPerToolTimeout(time.Second),
		WithAgentConversationStore(store, 5))

	out, err := a.Handle(context.Background(), inboundConv("telegram", "c1", "hi"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(out) != 1 || out[0].Parts[0].Content != "done" {
		t.Fatalf("got %+v, want the direct answer", out)
	}
	// The seed system message must carry BOTH the protocol grammar and the
	// operator rules (buildSystemPrompt appends the operator prompt).
	if len(m.gotMessages) == 0 || m.gotMessages[0].Role != model.RoleSystem {
		t.Fatalf("first message is not a system prompt: %+v", m.gotMessages)
	}
	sys := m.gotMessages[0].Content
	if !strings.Contains(sys, "TOOL:") || !strings.Contains(sys, "OPERATOR-RULES") {
		t.Fatalf("system prompt missing protocol or operator rules:\n%s", sys)
	}
	// Metrics funnels fired: one message, one model duration, one persisted group.
	if len(rec.messages) != 1 || len(rec.durations) != 1 || len(rec.turns) != 1 {
		t.Fatalf("metrics = msgs:%d durations:%d turns:%d, want 1/1/1",
			len(rec.messages), len(rec.durations), len(rec.turns))
	}
	// The injected clock stamped the persisted pair.
	turns, _ := store.LoadRecent(context.Background(), conversation.Key("telegram::c1"), 5)
	if len(turns) != 2 || !turns[0].Timestamp.Equal(agentFixedClock()) {
		t.Fatalf("persisted turns %+v, want 2 stamped with the fixed clock", turns)
	}
}

func TestAgentBrain_options_customFallback(t *testing.T) {
	t.Parallel()
	m := &scriptedModel{name: "m", replies: []string{"TOOL: echo(loop)"}} // never answers
	a := NewAgentBrain(m, builtinRegistry(),
		WithAgentLogger(quietLogger()),
		WithAgentFallback("nothing useful"),
		WithAgentMaxIterations(2))

	out, err := a.Handle(context.Background(), inboundText("telegram", "c", "go"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if out[0].Parts[0].Content != "nothing useful" {
		t.Fatalf("got %q, want the custom fallback", out[0].Parts[0].Content)
	}
}

// TestAgentBrain_ctxCancelled proves a context already done before the loop yields
// the fallback without calling the model (the runLoop ctx.Err guard, ADR-0021 §2).
func TestAgentBrain_ctxCancelled(t *testing.T) {
	t.Parallel()
	m := &scriptedModel{name: "m", replies: []string{"unused"}}
	a := NewAgentBrain(m, builtinRegistry(), WithAgentLogger(quietLogger()))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, err := a.Handle(ctx, inboundText("telegram", "c", "go"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if out[0].Parts[0].Content != defaultFallback {
		t.Fatalf("got %q, want fallback on cancelled ctx", out[0].Parts[0].Content)
	}
	if m.calls != 0 {
		t.Fatalf("model called %d times, want 0 (ctx done before first step)", m.calls)
	}
}

// TestAgentBrain_noConversationID proves a store-configured agent answers
// statelessly (no key) when the envelope carries no conversation id, persisting
// nothing (loadHistory no-key branch + persistPair empty-key no-op).
func TestAgentBrain_noConversationID(t *testing.T) {
	t.Parallel()
	store := conversation.NewMemStore()
	m := &scriptedModel{name: "m", replies: []string{"direct"}}
	a := NewAgentBrain(m, builtinRegistry(),
		WithAgentLogger(quietLogger()), WithAgentConversationStore(store, 5))

	// inboundText sets only telegram.chat_id, NOT conversation.id.
	out, err := a.Handle(context.Background(), inboundText("telegram", "c", "hi"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if out[0].Parts[0].Content != "direct" {
		t.Fatalf("got %q, want the stateless answer", out[0].Parts[0].Content)
	}
}

// errLoadStore errors on LoadRecent but delegates writes, proving the agent
// answers without memory when history load fails (loadHistory error branch).
type errLoadStore struct{ inner conversation.Store }

func (s errLoadStore) LoadRecent(context.Context, conversation.Key, int) ([]conversation.Turn, error) {
	return nil, errors.New("load boom")
}

func (s errLoadStore) Append(ctx context.Context, k conversation.Key, t conversation.Turn) (conversation.Turn, error) {
	return s.inner.Append(ctx, k, t)
}

func (s errLoadStore) AppendTurns(ctx context.Context, k conversation.Key, ts ...conversation.Turn) ([]conversation.Turn, error) {
	return s.inner.AppendTurns(ctx, k, ts...)
}

func TestAgentBrain_loadHistoryError(t *testing.T) {
	t.Parallel()
	store := errLoadStore{inner: conversation.NewMemStore()}
	m := &scriptedModel{name: "m", replies: []string{"answer"}}
	a := NewAgentBrain(m, builtinRegistry(),
		WithAgentLogger(quietLogger()), WithAgentConversationStore(store, 5))

	out, err := a.Handle(context.Background(), inboundConv("telegram", "c2", "hi"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if out[0].Parts[0].Content != "answer" {
		t.Fatalf("got %q, want the answer despite a failed history load", out[0].Parts[0].Content)
	}
	// The final pair still persisted (write side is healthy).
	turns, _ := store.inner.LoadRecent(context.Background(), conversation.Key("telegram::c2"), 5)
	if len(turns) != 2 {
		t.Fatalf("persisted %d turns, want 2", len(turns))
	}
}

// blockingTool blocks until ctx is done, then returns its error — proving the
// per-tool timeout bounds a hung tool (runTool perTool branch, ADR-0021 §2).
type blockingTool struct{}

func (blockingTool) Name() string        { return "block" }
func (blockingTool) Description() string { return "blocks until ctx is done." }
func (blockingTool) Execute(ctx context.Context, _ string) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func TestAgentBrain_perToolTimeout(t *testing.T) {
	t.Parallel()
	reg := tool.Registry{"block": blockingTool{}}
	m := &scriptedModel{name: "m", replies: []string{"TOOL: block()", "recovered"}}
	a := NewAgentBrain(m, reg,
		WithAgentLogger(quietLogger()),
		WithAgentPerToolTimeout(20*time.Millisecond))

	out, err := a.Handle(context.Background(), inboundText("telegram", "c", "go"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if out[0].Parts[0].Content != "recovered" {
		t.Fatalf("got %q, want the loop to recover after the per-tool timeout", out[0].Parts[0].Content)
	}
	if !requestHasObservationContaining(m.lastReq, "failed") {
		t.Fatalf("timed-out tool not surfaced as a failed OBSERVATION: %+v", m.lastReq)
	}
}

// TestAgentBrain_persistPair_empty covers the empty-text guards directly: a
// no-op for an empty pair, and a single-turn append when only one side has text.
func TestAgentBrain_persistPair_empty(t *testing.T) {
	t.Parallel()
	store := conversation.NewMemStore()
	a := NewAgentBrain(nil, nil,
		WithAgentLogger(quietLogger()), WithAgentConversationStore(store, 5))
	const key = conversation.Key("telegram::pp")

	a.persistPair(context.Background(), key, "", "") // both empty → no-op
	if turns, _ := store.LoadRecent(context.Background(), key, 5); len(turns) != 0 {
		t.Fatalf("empty pair persisted %d turns, want 0", len(turns))
	}
	a.persistPair(context.Background(), key, "only user", "") // assistant empty → one turn
	turns, _ := store.LoadRecent(context.Background(), key, 5)
	if len(turns) != 1 || turns[0].Content != "only user" {
		t.Fatalf("got %+v, want a single user turn", turns)
	}
}

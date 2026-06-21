// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"testing"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/fanout"
	"github.com/Sebastian197/korvun/internal/policy"
)

// inboundConv builds an inbound text Envelope carrying the canonical
// conversation id (so conversation.KeyFromEnvelope succeeds).
func inboundConv(channel, convID, text string) *envelope.Envelope {
	e := envelope.New(channel, envelope.Inbound, envelope.Participant{ID: "user1", Name: "User"})
	e.AddText(text)
	e.Meta[conversation.MetaConversationID] = convID
	return e
}

func fixedDecision(content string) fakePolicy {
	return fakePolicy{dec: &policy.Decision{Response: &model.Response{
		Message: model.Message{Role: model.RoleAssistant, Content: content},
	}}}
}

// TestOrchestrator_Handle_loadsHistoryIntoRequest: with a store, prior turns are
// loaded and placed before the current user message, in order.
func TestOrchestrator_Handle_loadsHistoryIntoRequest(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := conversation.NewMemStore()
	in := inboundConv("telegram", "c1", "current q")
	key, _ := conversation.KeyFromEnvelope(in)
	if _, err := store.Append(ctx, key, conversation.Turn{Role: conversation.RoleUser, Content: "first q"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(ctx, key, conversation.Turn{Role: conversation.RoleAssistant, Content: "first a"}); err != nil {
		t.Fatal(err)
	}

	rec := &recordingModel{name: "a-provider", response: "answer"}
	o := NewOrchestrator(fanout.New(), []model.Model{WithModelID(rec, "a")},
		fixedDecision("answer"), WithLogger(quietLogger()), WithConversationStore(store, 10))

	if _, err := o.Handle(ctx, in); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	want := []model.Message{
		{Role: model.RoleUser, Content: "first q"},
		{Role: model.RoleAssistant, Content: "first a"},
		{Role: model.RoleUser, Content: "current q"},
	}
	if len(rec.gotMessages) != len(want) {
		t.Fatalf("provider saw %d messages, want %d: %+v", len(rec.gotMessages), len(want), rec.gotMessages)
	}
	for i, w := range want {
		if rec.gotMessages[i].Role != w.Role || rec.gotMessages[i].Content != w.Content {
			t.Errorf("message[%d] = %+v, want %+v", i, rec.gotMessages[i], w)
		}
	}
}

// TestOrchestrator_Handle_appendsTurnsAfterReply: after a successful reply, the
// user turn and the assistant turn are persisted, in that order.
func TestOrchestrator_Handle_appendsTurnsAfterReply(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := conversation.NewMemStore()
	in := inboundConv("telegram", "c2", "hello")
	key, _ := conversation.KeyFromEnvelope(in)

	rec := &recordingModel{name: "a-provider", response: "hi there"}
	o := NewOrchestrator(fanout.New(), []model.Model{WithModelID(rec, "a")},
		fixedDecision("hi there"), WithLogger(quietLogger()), WithConversationStore(store, 10))

	if _, err := o.Handle(ctx, in); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	turns, err := store.LoadRecent(ctx, key, 10)
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	want := []conversation.Turn{
		{Role: conversation.RoleUser, Content: "hello", Seq: 0},
		{Role: conversation.RoleAssistant, Content: "hi there", Seq: 1},
	}
	if len(turns) != 2 {
		t.Fatalf("stored %d turns, want 2: %+v", len(turns), turns)
	}
	for i, w := range want {
		if turns[i].Role != w.Role || turns[i].Content != w.Content || turns[i].Seq != w.Seq {
			t.Errorf("turn[%d] = {%s %q seq=%d}, want {%s %q seq=%d}",
				i, turns[i].Role, turns[i].Content, turns[i].Seq, w.Role, w.Content, w.Seq)
		}
	}
}

// TestOrchestrator_Handle_noStore_behavesStateless: with no store injected, the
// Orchestrator behaves exactly as Stage 11 — no memory accumulates across calls.
func TestOrchestrator_Handle_noStore_behavesStateless(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rec := &recordingModel{name: "a-provider", response: "ok"}
	o := NewOrchestrator(fanout.New(), []model.Model{WithModelID(rec, "a")},
		fixedDecision("ok"), WithLogger(quietLogger()))

	for _, text := range []string{"one", "two"} {
		if _, err := o.Handle(ctx, inboundConv("telegram", "c3", text)); err != nil {
			t.Fatalf("Handle: %v", err)
		}
		if len(rec.gotMessages) != 1 {
			t.Fatalf("without a store the provider must see only the current user message, saw %d: %+v",
				len(rec.gotMessages), rec.gotMessages)
		}
		if rec.gotMessages[0].Content != text {
			t.Errorf("provider saw %q, want %q", rec.gotMessages[0].Content, text)
		}
	}
}

// TestOrchestrator_Handle_storeButNoConversationID_degradesStateless: a store is
// configured but the envelope carries no conversation id. The Brain still
// answers (no dropped reply) and persists nothing.
func TestOrchestrator_Handle_storeButNoConversationID_degradesStateless(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := conversation.NewMemStore()
	rec := &recordingModel{name: "a-provider", response: "ok"}
	o := NewOrchestrator(fanout.New(), []model.Model{WithModelID(rec, "a")},
		fixedDecision("ok"), WithLogger(quietLogger()), WithConversationStore(store, 10))

	// inboundText sets only telegram.chat_id, NOT conversation.id.
	out, err := o.Handle(ctx, inboundText("telegram", "chat-x", "hi"))
	if err != nil {
		t.Fatalf("Handle must still answer, got err: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("want one reply, got %d", len(out))
	}
	if len(rec.gotMessages) != 1 {
		t.Errorf("request should be stateless (no history), provider saw %d messages", len(rec.gotMessages))
	}
}

// TestOrchestrator_Handle_fallback_doesNotAppend: on the no-answer path the
// (canned) fallback and the user turn are NOT persisted — only real answers are.
func TestOrchestrator_Handle_fallback_doesNotAppend(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := conversation.NewMemStore()
	in := inboundConv("telegram", "c4", "vote?")
	key, _ := conversation.KeyFromEnvelope(in)

	a := &recordingModel{name: "a", err: model.ErrProviderUnavailable}
	o := NewOrchestrator(fanout.New(), []model.Model{WithModelID(a, "id-a")},
		policy.PriorityReducer{}, WithFallback("none"), WithLogger(quietLogger()),
		WithConversationStore(store, 10))

	out, err := o.Handle(ctx, in)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(out) != 1 || out[0].Parts[0].Content != "none" {
		t.Fatalf("want fallback reply, got %+v", out)
	}
	turns, _ := store.LoadRecent(ctx, key, 10)
	if len(turns) != 0 {
		t.Errorf("fallback path must not persist turns, stored %d: %+v", len(turns), turns)
	}
}

// TestOrchestrator_Handle_historyN_isBrainParameter: the Brain decides how many
// turns to load; only the last n reach the request.
func TestOrchestrator_Handle_historyN_isBrainParameter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := conversation.NewMemStore()
	in := inboundConv("telegram", "c5", "current")
	key, _ := conversation.KeyFromEnvelope(in)
	for _, c := range []string{"t0", "t1", "t2", "t3", "t4"} {
		if _, err := store.Append(ctx, key, conversation.Turn{Role: conversation.RoleUser, Content: c}); err != nil {
			t.Fatal(err)
		}
	}

	rec := &recordingModel{name: "a-provider", response: "ok"}
	o := NewOrchestrator(fanout.New(), []model.Model{WithModelID(rec, "a")},
		fixedDecision("ok"), WithLogger(quietLogger()), WithConversationStore(store, 2))

	if _, err := o.Handle(ctx, in); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	// last 2 history turns (t3, t4) + the current message = 3
	if len(rec.gotMessages) != 3 {
		t.Fatalf("provider saw %d messages, want 3 (n=2 history + current): %+v", len(rec.gotMessages), rec.gotMessages)
	}
	if rec.gotMessages[0].Content != "t3" || rec.gotMessages[1].Content != "t4" || rec.gotMessages[2].Content != "current" {
		t.Errorf("got %q,%q,%q want t3,t4,current",
			rec.gotMessages[0].Content, rec.gotMessages[1].Content, rec.gotMessages[2].Content)
	}
}

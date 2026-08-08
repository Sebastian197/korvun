// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// FR-ATTACH red (operator-console spec): the persistence points announce
// non-text parts honestly — the operator never faces a mute void.
package brain

import (
	"context"
	"testing"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/fanout"
)

func inboundWithImage(caption string) *envelope.Envelope {
	e := envelope.New("telegram", envelope.Inbound, envelope.Participant{ID: "u1"})
	e.Parts = append(e.Parts, envelope.Part{Type: envelope.Image, Source: "photo-id"})
	if caption != "" {
		e.AddText(caption)
	}
	e.Meta[conversation.MetaConversationID] = "c1"
	return e
}

func TestHandle_MediaOnlyPersistsMarkerTurnWithoutModelCall(t *testing.T) {
	store := conversation.NewMemStore()
	// nil coordinator ON PURPOSE: a media-only inbound must never reach the
	// fan-out — a call would panic and fail this test loudly.
	o := NewOrchestrator(nil, nil, nil,
		WithConversationStore(store, 10), WithLogger(quietLogger()))

	out, err := o.Handle(context.Background(), inboundWithImage(""))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("media-only produced a reply: %+v", out)
	}
	turns, _ := store.LoadRecent(context.Background(), "telegram::c1", 10)
	if len(turns) != 1 || turns[0].Role != conversation.RoleUser || turns[0].Content != "[image]" {
		t.Fatalf("persisted = %+v, want one user turn \"[image]\"", turns)
	}
}

func TestHandle_ImageWithCaptionPersistsMarkerAndText(t *testing.T) {
	store := conversation.NewMemStore()
	res := &fanout.Result{Outcomes: []fanout.Outcome{{
		Provider: "ollama",
		Response: &model.Response{Message: model.Message{Role: model.RoleAssistant, Content: "qué buena foto"}},
	}}}
	o := NewOrchestrator(fixedCoord{res: res}, nil, fixedDecision("qué buena foto"),
		WithConversationStore(store, 10), WithLogger(quietLogger()))

	if _, err := o.Handle(context.Background(), inboundWithImage("mira esto")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	turns, _ := store.LoadRecent(context.Background(), "telegram::c1", 10)
	if len(turns) != 2 {
		t.Fatalf("turns = %+v, want the user+assistant pair", turns)
	}
	if turns[0].Content != "[image] mira esto" {
		t.Fatalf("user turn = %q, want \"[image] mira esto\"", turns[0].Content)
	}
}

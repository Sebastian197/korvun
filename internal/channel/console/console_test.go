// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// FR-CONS-1 red: Send is I/O-free — a plain brain reply persists NOTHING
// here (the brain already did), while a router-marked ack lands as a
// SYSTEM turn in the conversation's active session.
package console

import (
	"context"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/router"
)

func outboundTo(convID, text string, meta map[string]string) *envelope.Envelope {
	e := envelope.New(ChannelName, envelope.Outbound, envelope.Participant{ID: "korvun"}).AddText(text)
	e.Meta[conversation.MetaConversationID] = convID
	for k, v := range meta {
		e.Meta[k] = v
	}
	return e
}

func TestSend_PlainReplyIsIOFreeAndPersistsNothing(t *testing.T) {
	store := conversation.NewMemStore()
	c := New(store)
	if err := c.Send(context.Background(), outboundTo("c1", "respuesta del brain", nil)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	turns, _ := store.LoadRecent(context.Background(), "console::c1", 10)
	if len(turns) != 0 {
		t.Fatalf("plain reply persisted %+v — the brain already owns that pair", turns)
	}
}

func TestSend_MarkedAckPersistsAsSystemTurn(t *testing.T) {
	store := conversation.NewMemStore()
	now := time.Unix(500, 0).UTC()
	c := New(store, WithClock(func() time.Time { return now }))
	ack := outboundTo("c1", router.SessionResetAck, map[string]string{
		envelope.MetaAck: envelope.AckSessionReset,
	})
	if err := c.Send(context.Background(), ack); err != nil {
		t.Fatalf("Send(ack): %v", err)
	}
	turns, _ := store.LoadRecent(context.Background(), "console::c1", 10)
	if len(turns) != 1 || turns[0].Role != conversation.RoleSystem ||
		turns[0].Content != router.SessionResetAck || !turns[0].Timestamp.Equal(now) {
		t.Fatalf("ack persisted as %+v, want one SYSTEM turn with the ack copy", turns)
	}
}

func TestSend_AckWithoutConversationIDIsHonestError(t *testing.T) {
	store := conversation.NewMemStore()
	c := New(store)
	e := envelope.New(ChannelName, envelope.Outbound, envelope.Participant{ID: "korvun"}).AddText("x")
	e.Meta[envelope.MetaAck] = envelope.AckSessionReset
	if err := c.Send(context.Background(), e); err == nil {
		t.Fatal("ack without conversation id: want error")
	}
}

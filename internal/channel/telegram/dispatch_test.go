// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import (
	"context"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/router"
	"github.com/go-telegram/bot/models"
)

func TestDispatchUpdate_emitsEnvelope(t *testing.T) {
	a := newTestAdapter(t)
	u := newTextUpdate(42, 1, "hola mundo")
	a.dispatchUpdate(context.Background(), u)

	select {
	case env := <-a.inbound:
		if env.Channel != ChannelName {
			t.Errorf("Channel = %q, want %q", env.Channel, ChannelName)
		}
		if env.Direction != envelope.Inbound {
			t.Errorf("Direction = %v, want Inbound", env.Direction)
		}
		if env.Parts[0].Content != "hola mundo" {
			t.Errorf("Content = %q", env.Parts[0].Content)
		}
		if got := env.Meta[router.MetaConversationID]; got != "42" {
			t.Errorf("Meta[%q] = %q, want %q", router.MetaConversationID, got, "42")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("no envelope on inbound channel after dispatchUpdate")
	}
}

func TestDispatchUpdate_silentSkipOnNoMessage(t *testing.T) {
	a := newTestAdapter(t)
	a.dispatchUpdate(context.Background(), nil)
	select {
	case env := <-a.inbound:
		t.Fatalf("unexpected envelope on inbound: %+v", env)
	case <-time.After(50 * time.Millisecond):
	}
	if a.DroppedCount() != 0 {
		t.Errorf("DroppedCount = %d, want 0 for no-message skip", a.DroppedCount())
	}
}

func TestDispatchUpdate_silentSkipOnUnsupportedContent(t *testing.T) {
	a := newTestAdapter(t)
	u := &models.Update{Message: &models.Message{}}
	a.dispatchUpdate(context.Background(), u)
	select {
	case env := <-a.inbound:
		t.Fatalf("unexpected envelope on inbound: %+v", env)
	case <-time.After(50 * time.Millisecond):
	}
	if a.DroppedCount() != 0 {
		t.Errorf("DroppedCount = %d, want 0 for unsupported-content skip", a.DroppedCount())
	}
}

func TestDispatchUpdate_dropsOnSaturation(t *testing.T) {
	a, err := New(
		WithToken("test-token"),
		WithMode(ModePolling),
		WithInboundCapacity(1),
		WithEnqueueTimeout(20*time.Millisecond),
		withInjectedBotForTests(stubBotClient{}),
	)
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	if cap(a.inbound) != 1 {
		t.Fatalf("inbound cap = %d, want 1", cap(a.inbound))
	}
	// Fill the buffer so the next enqueue blocks.
	a.dispatchUpdate(context.Background(), newTextUpdate(1, 11, "first"))
	if l := len(a.inbound); l != 1 {
		t.Fatalf("inbound len after seed = %d, want 1", l)
	}

	// Now the buffer is full; dispatch a second update and confirm
	// we drop after the configured enqueue timeout.
	start := time.Now()
	a.dispatchUpdate(context.Background(), newTextUpdate(1, 12, "second"))
	elapsed := time.Since(start)
	if elapsed < 15*time.Millisecond {
		t.Errorf("dispatch returned in %v, want at least the enqueueTimeout (20ms)", elapsed)
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("dispatch took %v, way over enqueueTimeout", elapsed)
	}
	if got := a.DroppedCount(); got != 1 {
		t.Errorf("DroppedCount = %d, want 1 after one saturated dispatch", got)
	}
}

func TestDispatchUpdate_cancelDoesNotCountAsDrop(t *testing.T) {
	a, err := New(
		WithToken("test-token"),
		WithMode(ModePolling),
		WithInboundCapacity(1),
		WithEnqueueTimeout(500*time.Millisecond),
		withInjectedBotForTests(stubBotClient{}),
	)
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	// Fill buffer.
	a.dispatchUpdate(context.Background(), newTextUpdate(2, 21, "first"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	a.dispatchUpdate(ctx, newTextUpdate(2, 22, "second"))
	if got := a.DroppedCount(); got != 0 {
		t.Errorf("DroppedCount = %d, want 0 when ctx already cancelled", got)
	}
}

// newTextUpdate builds a minimal Update carrying a Message with the
// given chat_id, message_id and text. Sender is a fixed test user.
func newTextUpdate(chatID int64, messageID int, text string) *models.Update {
	return &models.Update{
		Message: &models.Message{
			ID:   messageID,
			Date: int(time.Now().Unix()),
			From: &models.User{ID: 1001, FirstName: "Test", Username: "tester"},
			Chat: models.Chat{ID: chatID, Type: models.ChatTypePrivate},
			Text: text,
		},
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import (
	"errors"
	"testing"

	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/go-telegram/bot/models"
)

// ----------------------------------------------------------------------------
// Phase 2E.4 — inbound callback_query. A callback is a separate Update kind
// (sibling of Message); the adapter must produce a canonical Envelope that
// carries the callback data as a Callback Part and the channel-specific
// callback ID in Meta so a later outbound CallbackAck can address it.
// ----------------------------------------------------------------------------

func TestInboundFromUpdate_CallbackQuery_FromFixture(t *testing.T) {
	u := loadUpdateFixture(t, "callback_query.json")
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if env.Direction != envelope.Inbound {
		t.Errorf("Direction = %v, want Inbound", env.Direction)
	}
	if env.Channel != ChannelName {
		t.Errorf("Channel = %q, want %q", env.Channel, ChannelName)
	}
	if env.Sender.ID != "555" {
		t.Errorf("Sender.ID = %q, want %q", env.Sender.ID, "555")
	}
	if env.Sender.Name != "alice" {
		t.Errorf("Sender.Name = %q, want %q", env.Sender.Name, "alice")
	}
	if len(env.Parts) != 1 {
		t.Fatalf("Parts len = %d, want 1", len(env.Parts))
	}
	if env.Parts[0].Type != envelope.Callback {
		t.Errorf("Parts[0].Type = %v, want Callback", env.Parts[0].Type)
	}
	if env.Parts[0].Content != "button_yes" {
		t.Errorf("Parts[0].Content = %q, want %q", env.Parts[0].Content, "button_yes")
	}
	if env.Meta[MetaCallbackQueryID] != "CQ_unique_id_42" {
		t.Errorf("Meta[%s] = %q, want %q",
			MetaCallbackQueryID, env.Meta[MetaCallbackQueryID], "CQ_unique_id_42")
	}
	// When the original message is accessible, chat_id / message_id are
	// preserved so a downstream consumer can address it (e.g. edit it).
	if env.Meta[MetaChatID] != "1000" {
		t.Errorf("Meta[%s] = %q, want %q", MetaChatID, env.Meta[MetaChatID], "1000")
	}
	if env.Meta[MetaMessageID] != "401" {
		t.Errorf("Meta[%s] = %q, want %q", MetaMessageID, env.Meta[MetaMessageID], "401")
	}
}

func TestInboundFromUpdate_CallbackQuery_PassesValidate(t *testing.T) {
	u := loadUpdateFixture(t, "callback_query.json")
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if err := env.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestInboundFromUpdate_CallbackQuery_NilFrom_Unsupported(t *testing.T) {
	u := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "x",
			Data: "y",
		},
	}
	_, err := InboundFromUpdate(u)
	if !errors.Is(err, ErrUnsupportedContent) {
		t.Errorf("err = %v, want ErrUnsupportedContent", err)
	}
}

func TestInboundFromUpdate_CallbackQuery_EmptyData_Unsupported(t *testing.T) {
	// A callback query with no Data string carries no payload the
	// adapter can translate into a Callback Part. Mirrors how an empty
	// message returns ErrUnsupportedContent.
	u := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "x",
			From: models.User{ID: 9, Username: "alice"},
			Data: "",
		},
	}
	_, err := InboundFromUpdate(u)
	if !errors.Is(err, ErrUnsupportedContent) {
		t.Errorf("err = %v, want ErrUnsupportedContent", err)
	}
}

func TestInboundFromUpdate_CallbackQuery_EmptyID_NoMessage(t *testing.T) {
	// A callback query without an ID cannot be acknowledged; reject so
	// the orchestrator never receives a Callback envelope it cannot ack.
	u := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "",
			From: models.User{ID: 9, Username: "alice"},
			Data: "x",
		},
	}
	_, err := InboundFromUpdate(u)
	if !errors.Is(err, ErrNoMessage) {
		t.Errorf("err = %v, want ErrNoMessage", err)
	}
}

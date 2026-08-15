// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import (
	"testing"

	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/go-telegram/bot/models"
)

// Audit finding R-1: every inbound Envelope must carry the provider-native
// delivery id (the Telegram UPDATE id — not the message id: an edit arrives
// as a NEW update and must not deduplicate against the original) so the
// router's dedup window can recognize an at-least-once re-delivery.

func TestInboundFromUpdate_StampsProviderEventID(t *testing.T) {
	u := &models.Update{
		ID: 123456789,
		Message: &models.Message{
			ID:   7,
			From: &models.User{ID: 555, FirstName: "Ana"},
			Chat: models.Chat{ID: 999},
			Text: "hola",
		},
	}
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if got := env.Meta[envelope.MetaProviderEventID]; got != "123456789" {
		t.Errorf("Meta[%q] = %q, want %q (the UPDATE id)",
			envelope.MetaProviderEventID, got, "123456789")
	}
}

func TestInboundFromUpdate_EditedMessageStampsItsOwnUpdateID(t *testing.T) {
	u := &models.Update{
		ID: 123456790, // a NEW update id: an edit is a new event
		EditedMessage: &models.Message{
			ID:       7, // the SAME message id as the original
			From:     &models.User{ID: 555, FirstName: "Ana"},
			Chat:     models.Chat{ID: 999},
			Text:     "hola (editado)",
			EditDate: 1_700_000_000,
		},
	}
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if got := env.Meta[envelope.MetaProviderEventID]; got != "123456790" {
		t.Errorf("Meta[%q] = %q, want %q (edits carry their OWN update id)",
			envelope.MetaProviderEventID, got, "123456790")
	}
}

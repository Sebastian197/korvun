// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import (
	"strconv"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/go-telegram/bot/models"
)

// ----------------------------------------------------------------------------
// Phase 2E.6 — inbound edited_message.
//
// An Update.EditedMessage carries the same *Message struct shape as a fresh
// Message; the discriminator is Message.EditDate (Unix timestamp, non-zero
// iff the user has edited the message). The adapter:
//
//   - Reuses the existing inboundFromMessage helper end-to-end.
//   - Switches Envelope.Timestamp from Date to EditDate when EditDate > 0
//     (the moment of the event for an edit is when the user edited, not
//     when they originally sent).
//   - Sets Meta[telegram.edited_at] to the stringified EditDate so
//     consumers can detect the edit without comparing Timestamps.
//   - Preserves telegram.message_id verbatim (Telegram does not change
//     message_id on edit), so the orchestrator can correlate the edit
//     with the original send by ID lookup.
// ----------------------------------------------------------------------------

func TestInboundFromUpdate_EditedMessage_FromFixture(t *testing.T) {
	u := loadUpdateFixture(t, "edited_message.json")
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
	if len(env.Parts) != 1 || env.Parts[0].Type != envelope.Text {
		t.Fatalf("Parts = %+v, want one Text part", env.Parts)
	}
	if env.Parts[0].Content != "edited text" {
		t.Errorf("Parts[0].Content = %q, want %q", env.Parts[0].Content, "edited text")
	}
	// Same message_id as the (hypothetical) original send.
	if env.Meta[MetaMessageID] != "701" {
		t.Errorf("Meta[%s] = %q, want %q", MetaMessageID, env.Meta[MetaMessageID], "701")
	}
	// Edited_at marker present and matches the fixture's edit_date.
	if env.Meta[MetaEditedAt] != "1786000800" {
		t.Errorf("Meta[%s] = %q, want %q",
			MetaEditedAt, env.Meta[MetaEditedAt], "1786000800")
	}
	// Envelope.Timestamp is the EditDate moment, not the original Date.
	wantTS := time.Unix(1786000800, 0).UTC()
	if !env.Timestamp.Equal(wantTS) {
		t.Errorf("Timestamp = %v, want %v (EditDate, not Date)", env.Timestamp, wantTS)
	}
}

func TestInboundFromUpdate_EditedMessage_PassesValidate(t *testing.T) {
	u := loadUpdateFixture(t, "edited_message.json")
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if err := env.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestInboundFromUpdate_RegularMessage_NoEditedAt(t *testing.T) {
	// Regression: a fresh (non-edited) Message must NOT set MetaEditedAt
	// and must use Date (not EditDate) for Envelope.Timestamp.
	u := loadUpdateFixture(t, "text_message.json")
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if _, ok := env.Meta[MetaEditedAt]; ok {
		t.Errorf("Meta[%s] set on non-edited message: %q",
			MetaEditedAt, env.Meta[MetaEditedAt])
	}
}

func TestInboundFromUpdate_EditedMessage_PreservesDateInMessage(t *testing.T) {
	// When EditDate is set, Envelope.Timestamp uses EditDate. The
	// original Date is not lost from the inbound payload — Telegram
	// always delivers both — but Korvun's canonical Timestamp is
	// "moment of this event", which for an edit is when the edit
	// happened.
	u := &models.Update{
		EditedMessage: &models.Message{
			ID:       42,
			Date:     1000,
			EditDate: 2000,
			From:     &models.User{ID: 555, Username: "alice"},
			Chat:     models.Chat{ID: 1000, Type: "private"},
			Text:     "v2",
		},
	}
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if env.Timestamp.Unix() != 2000 {
		t.Errorf("Timestamp.Unix() = %d, want 2000 (EditDate)", env.Timestamp.Unix())
	}
	if env.Meta[MetaEditedAt] != strconv.Itoa(2000) {
		t.Errorf("Meta[%s] = %q, want %q",
			MetaEditedAt, env.Meta[MetaEditedAt], "2000")
	}
}

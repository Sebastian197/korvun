// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/go-telegram/bot/models"
)

// loadUpdateFixture decodes a Telegram Update payload from a JSON file under
// testdata/. It returns a pointer to the decoded models.Update so the test
// can exercise the adapter exactly as it would from a real webhook handler.
func loadUpdateFixture(t *testing.T, name string) *models.Update {
	t.Helper()
	// #nosec G304 -- fixture name is supplied by the test itself, not user input.
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %q: %v", name, err)
	}
	var u models.Update
	if err := json.Unmarshal(data, &u); err != nil {
		t.Fatalf("unmarshal fixture %q: %v", name, err)
	}
	return &u
}

// ---------- Inbound -------------------------------------------------------

func TestInboundFromUpdate_TextFixture(t *testing.T) {
	u := loadUpdateFixture(t, "text_message.json")

	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if env == nil {
		t.Fatal("InboundFromUpdate returned nil envelope")
	}
	if env.Channel != ChannelName {
		t.Errorf("Channel = %q, want %q", env.Channel, ChannelName)
	}
	if env.Direction != envelope.Inbound {
		t.Errorf("Direction = %v, want Inbound", env.Direction)
	}
	if env.Sender.ID != "555" {
		t.Errorf("Sender.ID = %q, want %q", env.Sender.ID, "555")
	}
	if env.Sender.Name != "alice" {
		t.Errorf("Sender.Name = %q, want %q (username preferred)", env.Sender.Name, "alice")
	}
	if len(env.Parts) != 1 {
		t.Fatalf("Parts len = %d, want 1", len(env.Parts))
	}
	if env.Parts[0].Type != envelope.Text {
		t.Errorf("Parts[0].Type = %v, want Text", env.Parts[0].Type)
	}
	if env.Parts[0].Content != "hola Korvun" {
		t.Errorf("Parts[0].Content = %q, want %q", env.Parts[0].Content, "hola Korvun")
	}
	if got, want := env.Meta[MetaChatID], "1000"; got != want {
		t.Errorf("Meta[%q] = %q, want %q", MetaChatID, got, want)
	}
	if got, want := env.Meta[MetaMessageID], "42"; got != want {
		t.Errorf("Meta[%q] = %q, want %q", MetaMessageID, got, want)
	}
	if got, want := env.Meta[MetaChatType], "private"; got != want {
		t.Errorf("Meta[%q] = %q, want %q", MetaChatType, got, want)
	}
	wantTS := time.Unix(1786000000, 0).UTC()
	if !env.Timestamp.Equal(wantTS) {
		t.Errorf("Timestamp = %v, want %v", env.Timestamp, wantTS)
	}
}

func TestInboundFromUpdate_NilUpdate(t *testing.T) {
	_, err := InboundFromUpdate(nil)
	if !errors.Is(err, ErrNoMessage) {
		t.Errorf("err = %v, want ErrNoMessage", err)
	}
}

func TestInboundFromUpdate_UpdateWithoutMessage(t *testing.T) {
	_, err := InboundFromUpdate(&models.Update{ID: 1})
	if !errors.Is(err, ErrNoMessage) {
		t.Errorf("err = %v, want ErrNoMessage", err)
	}
}

func TestInboundFromUpdate_EmptyText(t *testing.T) {
	u := &models.Update{
		Message: &models.Message{
			ID:   1,
			Date: 1000,
			From: &models.User{ID: 99, Username: "bob"},
			Chat: models.Chat{ID: 5, Type: "private"},
			Text: "",
		},
	}
	_, err := InboundFromUpdate(u)
	if !errors.Is(err, ErrUnsupportedContent) {
		t.Errorf("err = %v, want ErrUnsupportedContent", err)
	}
}

func TestInboundFromUpdate_NoFrom(t *testing.T) {
	u := &models.Update{
		Message: &models.Message{
			ID:   1,
			Date: 1000,
			Chat: models.Chat{ID: 5, Type: "channel"},
			Text: "anonymous channel post",
		},
	}
	_, err := InboundFromUpdate(u)
	if !errors.Is(err, ErrUnsupportedContent) {
		t.Errorf("err = %v, want ErrUnsupportedContent", err)
	}
}

func TestInboundFromUpdate_SenderNamePreference(t *testing.T) {
	tests := []struct {
		name     string
		user     models.User
		wantName string
	}{
		{
			name:     "username preferred",
			user:     models.User{ID: 1, Username: "alice", FirstName: "Alice", LastName: "Wonderland"},
			wantName: "alice",
		},
		{
			name:     "first + last when no username",
			user:     models.User{ID: 2, FirstName: "Alice", LastName: "Wonderland"},
			wantName: "Alice Wonderland",
		},
		{
			name:     "first only when no last name",
			user:     models.User{ID: 3, FirstName: "Alice"},
			wantName: "Alice",
		},
		{
			name:     "empty when nothing set",
			user:     models.User{ID: 4},
			wantName: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &models.Update{
				Message: &models.Message{
					ID:   1,
					Date: 1000,
					From: &tt.user,
					Chat: models.Chat{ID: 5, Type: "private"},
					Text: "x",
				},
			}
			env, err := InboundFromUpdate(u)
			if err != nil {
				t.Fatalf("InboundFromUpdate: %v", err)
			}
			if env.Sender.Name != tt.wantName {
				t.Errorf("Sender.Name = %q, want %q", env.Sender.Name, tt.wantName)
			}
		})
	}
}

func TestInboundFromUpdate_SenderIDIsAlwaysStringifiedTelegramUserID(t *testing.T) {
	u := &models.Update{
		Message: &models.Message{
			ID:   1,
			Date: 1000,
			From: &models.User{ID: 9876543210, Username: "alice"},
			Chat: models.Chat{ID: 5, Type: "private"},
			Text: "x",
		},
	}
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if env.Sender.ID != "9876543210" {
		t.Errorf("Sender.ID = %q, want %q (stable across username changes)", env.Sender.ID, "9876543210")
	}
}

func TestInboundFromUpdate_ResultPassesEnvelopeValidate(t *testing.T) {
	u := loadUpdateFixture(t, "text_message.json")
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if err := env.Validate(); err != nil {
		t.Errorf("Validate on adapter output: %v", err)
	}
}

// ---------- Outbound ------------------------------------------------------

func TestOutboundToSendMessage_TextEnvelope(t *testing.T) {
	env := envelope.New(ChannelName, envelope.Outbound, envelope.Participant{ID: "bot"})
	env.AddText("respuesta")
	env.Meta[MetaChatID] = "1000"

	params, err := OutboundToSendMessage(env)
	if err != nil {
		t.Fatalf("OutboundToSendMessage: %v", err)
	}
	if params == nil {
		t.Fatal("params is nil")
	}
	if params.Text != "respuesta" {
		t.Errorf("Text = %q, want %q", params.Text, "respuesta")
	}
	gotChatID, ok := params.ChatID.(int64)
	if !ok {
		t.Fatalf("ChatID type = %T, want int64", params.ChatID)
	}
	if gotChatID != 1000 {
		t.Errorf("ChatID = %d, want 1000", gotChatID)
	}
}

func TestOutboundToSendMessage_NilEnvelope(t *testing.T) {
	_, err := OutboundToSendMessage(nil)
	if !errors.Is(err, ErrNilEnvelope) {
		t.Errorf("err = %v, want ErrNilEnvelope", err)
	}
}

func TestOutboundToSendMessage_InboundDirection(t *testing.T) {
	env := envelope.New(ChannelName, envelope.Inbound, envelope.Participant{ID: "user-1"})
	env.AddText("nope")
	env.Meta[MetaChatID] = "1000"

	_, err := OutboundToSendMessage(env)
	if !errors.Is(err, ErrNotOutbound) {
		t.Errorf("err = %v, want ErrNotOutbound", err)
	}
}

func TestOutboundToSendMessage_WrongChannel(t *testing.T) {
	env := envelope.New("webhook", envelope.Outbound, envelope.Participant{ID: "bot"})
	env.AddText("nope")
	env.Meta[MetaChatID] = "1000"

	_, err := OutboundToSendMessage(env)
	if !errors.Is(err, ErrWrongChannel) {
		t.Errorf("err = %v, want ErrWrongChannel", err)
	}
}

func TestOutboundToSendMessage_MissingChatID(t *testing.T) {
	env := envelope.New(ChannelName, envelope.Outbound, envelope.Participant{ID: "bot"})
	env.AddText("respuesta")

	_, err := OutboundToSendMessage(env)
	if !errors.Is(err, ErrMissingChatID) {
		t.Errorf("err = %v, want ErrMissingChatID", err)
	}
}

func TestOutboundToSendMessage_EmptyChatID(t *testing.T) {
	env := envelope.New(ChannelName, envelope.Outbound, envelope.Participant{ID: "bot"})
	env.AddText("respuesta")
	env.Meta[MetaChatID] = ""

	_, err := OutboundToSendMessage(env)
	if !errors.Is(err, ErrMissingChatID) {
		t.Errorf("err = %v, want ErrMissingChatID", err)
	}
}

func TestOutboundToSendMessage_InvalidChatID(t *testing.T) {
	env := envelope.New(ChannelName, envelope.Outbound, envelope.Participant{ID: "bot"})
	env.AddText("respuesta")
	env.Meta[MetaChatID] = "not-an-int"

	_, err := OutboundToSendMessage(env)
	if !errors.Is(err, ErrInvalidChatID) {
		t.Errorf("err = %v, want ErrInvalidChatID", err)
	}
}

func TestOutboundToSendMessage_NoTextPart(t *testing.T) {
	env := envelope.New(ChannelName, envelope.Outbound, envelope.Participant{ID: "bot"})
	env.AddMedia(envelope.Image, "https://example.com/img.png", "image/png")
	env.Meta[MetaChatID] = "1000"

	_, err := OutboundToSendMessage(env)
	if !errors.Is(err, ErrNoTextPart) {
		t.Errorf("err = %v, want ErrNoTextPart", err)
	}
}

func TestOutboundToSendMessage_FirstTextPartUsed(t *testing.T) {
	env := envelope.New(ChannelName, envelope.Outbound, envelope.Participant{ID: "bot"})
	env.AddMedia(envelope.Image, "https://example.com/img.png", "image/png")
	env.AddText("primero")
	env.AddText("segundo")
	env.Meta[MetaChatID] = "1000"

	params, err := OutboundToSendMessage(env)
	if err != nil {
		t.Fatalf("OutboundToSendMessage: %v", err)
	}
	if params.Text != "primero" {
		t.Errorf("Text = %q, want %q", params.Text, "primero")
	}
}

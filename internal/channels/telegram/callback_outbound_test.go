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
// Phase 2E.4 — outbound CallbackAck.
//
// A CallbackAck Envelope translates to *bot.AnswerCallbackQueryParams via a
// new OutboundKindAnswerCallback. The minimal contract (ADR-0005) is:
//   - CallbackQueryID is read from Meta[MetaCallbackQueryID]; absence
//     yields ErrMissingCallbackQueryID.
//   - Text is the CallbackAck Part's Content; empty Content = silent ack.
//   - No ChatID is needed (the ack is addressed by the callback ID, not by
//     chat), so the standard MetaChatID precondition does NOT apply to acks.
// ----------------------------------------------------------------------------

// ackEnv builds an outbound CallbackAck envelope with the given toast and
// the callback_query_id Meta entry set. Tests that want to remove either
// piece tweak the result.
func ackEnv(toast string) *envelope.Envelope {
	e := envelope.New(ChannelName, envelope.Outbound, envelope.Participant{ID: "bot"})
	e.Meta[MetaCallbackQueryID] = "CQ_xyz_42"
	e.AddCallbackAck(toast)
	return e
}

func TestOutboundParams_CallbackAck_Silent(t *testing.T) {
	e := ackEnv("")
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindAnswerCallback {
		t.Errorf("Kind = %v, want OutboundKindAnswerCallback", out.Kind)
	}
	if out.AnswerCallback == nil {
		t.Fatal("AnswerCallback is nil")
	}
	if out.AnswerCallback.CallbackQueryID != "CQ_xyz_42" {
		t.Errorf("CallbackQueryID = %q, want %q",
			out.AnswerCallback.CallbackQueryID, "CQ_xyz_42")
	}
	if out.AnswerCallback.Text != "" {
		t.Errorf("Text = %q, want empty (silent ack)", out.AnswerCallback.Text)
	}
	// Sibling tagged-union fields must remain nil.
	if out.Message != nil || out.Photo != nil || out.Document != nil ||
		out.Voice != nil || out.Audio != nil || out.Video != nil || out.Location != nil {
		t.Error("only AnswerCallback should be populated")
	}
}

func TestOutboundParams_CallbackAck_WithToast(t *testing.T) {
	e := ackEnv("Saved!")
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.AnswerCallback.Text != "Saved!" {
		t.Errorf("Text = %q, want %q", out.AnswerCallback.Text, "Saved!")
	}
}

func TestOutboundParams_CallbackAck_MissingMetaID(t *testing.T) {
	e := ackEnv("")
	delete(e.Meta, MetaCallbackQueryID)
	_, err := OutboundParams(e)
	if !errors.Is(err, ErrMissingCallbackQueryID) {
		t.Errorf("err = %v, want ErrMissingCallbackQueryID", err)
	}
}

func TestOutboundParams_CallbackAck_EmptyMetaID(t *testing.T) {
	e := ackEnv("")
	e.Meta[MetaCallbackQueryID] = ""
	_, err := OutboundParams(e)
	if !errors.Is(err, ErrMissingCallbackQueryID) {
		t.Errorf("err = %v, want ErrMissingCallbackQueryID", err)
	}
}

func TestOutboundParams_CallbackAck_NoChatIDRequired(t *testing.T) {
	// An ack envelope does NOT need MetaChatID — the ack is addressed
	// by the callback query ID, not by chat. The standard outbound
	// chat-ID precondition must not fire here.
	e := envelope.New(ChannelName, envelope.Outbound, envelope.Participant{ID: "bot"})
	e.Meta[MetaCallbackQueryID] = "CQ_no_chat"
	e.AddCallbackAck("")
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindAnswerCallback {
		t.Errorf("Kind = %v, want OutboundKindAnswerCallback", out.Kind)
	}
}

func TestOutboundKind_AnswerCallbackString(t *testing.T) {
	if got := OutboundKindAnswerCallback.String(); got != "answer_callback" {
		t.Errorf("OutboundKindAnswerCallback.String() = %q, want %q", got, "answer_callback")
	}
}

// ----------------------------------------------------------------------------
// Phase 2E.4 — outbound Keyboard markup.
//
// When Envelope.Keyboard != nil, the translated InlineKeyboardMarkup is
// attached as the ReplyMarkup of whatever Send*Params is otherwise
// produced (Message, Photo, Document, Voice, Audio, Video, Location).
// CallbackAck envelopes never carry a keyboard and are excluded here.
// ----------------------------------------------------------------------------

// kbEnv builds the same kind of outbound envelope as mkOutboundEnv but
// also attaches a 2-button single-row inline keyboard.
func kbEnv(parts ...envelope.Part) *envelope.Envelope {
	e := mkOutboundEnv(parts...)
	e.WithKeyboard(
		[]envelope.Button{
			envelope.CallbackButton("Yes", "yes"),
			envelope.URLButton("Help", "https://example.com"),
		},
	)
	return e
}

func assertInlineKeyboard(t *testing.T, m models.ReplyMarkup) {
	t.Helper()
	if m == nil {
		t.Fatal("ReplyMarkup is nil, want InlineKeyboardMarkup")
	}
	mk, ok := m.(models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("ReplyMarkup is %T, want models.InlineKeyboardMarkup", m)
	}
	if len(mk.InlineKeyboard) != 1 {
		t.Fatalf("InlineKeyboard rows = %d, want 1", len(mk.InlineKeyboard))
	}
	row := mk.InlineKeyboard[0]
	if len(row) != 2 {
		t.Fatalf("row buttons = %d, want 2", len(row))
	}
	if row[0].Text != "Yes" || row[0].CallbackData != "yes" || row[0].URL != "" {
		t.Errorf("row[0] = %+v, want Yes/yes/empty", row[0])
	}
	if row[1].Text != "Help" || row[1].URL != "https://example.com" || row[1].CallbackData != "" {
		t.Errorf("row[1] = %+v, want Help/empty/https://example.com", row[1])
	}
}

func TestOutboundParams_Keyboard_AttachedToTextMessage(t *testing.T) {
	e := kbEnv(textPart("Choose:"))
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindMessage {
		t.Errorf("Kind = %v, want OutboundKindMessage", out.Kind)
	}
	assertInlineKeyboard(t, out.Message.ReplyMarkup)
}

func TestOutboundParams_Keyboard_AttachedToPhoto(t *testing.T) {
	e := kbEnv(imagePart("photo_fid"), textPart("caption"))
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindPhoto {
		t.Errorf("Kind = %v, want OutboundKindPhoto", out.Kind)
	}
	assertInlineKeyboard(t, out.Photo.ReplyMarkup)
}

func TestOutboundParams_Keyboard_AttachedToDocument(t *testing.T) {
	e := kbEnv(filePart("doc_fid"))
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindDocument {
		t.Errorf("Kind = %v, want OutboundKindDocument", out.Kind)
	}
	assertInlineKeyboard(t, out.Document.ReplyMarkup)
}

func TestOutboundParams_Keyboard_AttachedToVoice(t *testing.T) {
	e := kbEnv(audioPart("voice_fid"))
	e.Meta[MetaAudioKind] = AudioKindVoice
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindVoice {
		t.Errorf("Kind = %v, want OutboundKindVoice", out.Kind)
	}
	assertInlineKeyboard(t, out.Voice.ReplyMarkup)
}

func TestOutboundParams_Keyboard_AttachedToAudio(t *testing.T) {
	e := kbEnv(audioPart("audio_fid"))
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindAudio {
		t.Errorf("Kind = %v, want OutboundKindAudio", out.Kind)
	}
	assertInlineKeyboard(t, out.Audio.ReplyMarkup)
}

func TestOutboundParams_Keyboard_AttachedToVideo(t *testing.T) {
	e := kbEnv(videoPart("vid_fid"))
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindVideo {
		t.Errorf("Kind = %v, want OutboundKindVideo", out.Kind)
	}
	assertInlineKeyboard(t, out.Video.ReplyMarkup)
}

func TestOutboundParams_Keyboard_AttachedToLocation(t *testing.T) {
	e := kbEnv(locationPart(41.40338, 2.17403))
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindLocation {
		t.Errorf("Kind = %v, want OutboundKindLocation", out.Kind)
	}
	assertInlineKeyboard(t, out.Location.ReplyMarkup)
}

func TestOutboundParams_NoKeyboard_ReplyMarkupIsNil(t *testing.T) {
	// Regression: an envelope without a keyboard must still produce a
	// Send*Params with ReplyMarkup left untouched (nil interface).
	e := mkOutboundEnv(textPart("plain"))
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Message.ReplyMarkup != nil {
		t.Errorf("ReplyMarkup = %+v, want nil when Envelope.Keyboard is nil", out.Message.ReplyMarkup)
	}
}

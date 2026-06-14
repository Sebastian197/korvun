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
// Phase 2E.6 — outbound edit/delete operations.
//
// An Envelope with Operation.Kind in {OpEditText, OpEditCaption, OpDelete}
// translates to its native Telegram primitive via OutboundParams. Target
// identification is the same across the three:
//   - telegram.chat_id    (required, parsed as int64)
//   - telegram.message_id (required, parsed as int)
// Missing either yields a sentinel error.
//
// ReplyMarkup attachment from Envelope.Keyboard applies to OpEditText and
// OpEditCaption (the Telegram primitives both accept it). OpDelete has no
// ReplyMarkup; Validate rejects an OpDelete envelope that carries a
// Keyboard, so the dispatch never needs to deal with that case.
// ----------------------------------------------------------------------------

// mkOpEnv builds an outbound envelope with both chat_id and message_id Meta
// entries set; tests that need to remove either piece tweak the result.
// The Operation is not set; tests apply Set* builders.
func mkOpEnv() *envelope.Envelope {
	e := envelope.New(ChannelName, envelope.Outbound, envelope.Participant{ID: "bot"})
	e.Meta[MetaChatID] = "1000"
	e.Meta[MetaMessageID] = "42"
	return e
}

// ---------- OpEditText ----------------------------------------------------

func TestOutboundParams_EditText_HappyPath(t *testing.T) {
	e := mkOpEnv()
	e.SetEditText("new body")
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindEditText {
		t.Errorf("Kind = %v, want OutboundKindEditText", out.Kind)
	}
	if out.EditText == nil {
		t.Fatal("EditText is nil")
	}
	assertChatID(t, out.EditText.ChatID, 1000)
	if out.EditText.MessageID != 42 {
		t.Errorf("MessageID = %d, want 42", out.EditText.MessageID)
	}
	if out.EditText.Text != "new body" {
		t.Errorf("Text = %q, want %q", out.EditText.Text, "new body")
	}
	if out.EditText.ReplyMarkup != nil {
		t.Errorf("ReplyMarkup = %+v, want nil (no keyboard)", out.EditText.ReplyMarkup)
	}
	// Sibling tagged-union fields must remain nil.
	if out.Message != nil || out.Photo != nil || out.Document != nil ||
		out.Voice != nil || out.Audio != nil || out.Video != nil ||
		out.Location != nil || out.AnswerCallback != nil ||
		out.EditCaption != nil || out.Delete != nil {
		t.Error("only EditText should be populated")
	}
}

func TestOutboundParams_EditText_WithKeyboard(t *testing.T) {
	e := mkOpEnv()
	e.SetEditText("new body").WithKeyboard(
		[]envelope.Button{envelope.CallbackButton("Yes", "yes")},
	)
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	assertInlineKeyboardSingle(t, out.EditText.ReplyMarkup, "Yes", "yes", "")
}

func TestOutboundParams_EditText_MissingMessageID(t *testing.T) {
	e := mkOpEnv()
	delete(e.Meta, MetaMessageID)
	e.SetEditText("new body")
	_, err := OutboundParams(e)
	if !errors.Is(err, ErrMissingTargetMessageID) {
		t.Errorf("err = %v, want ErrMissingTargetMessageID", err)
	}
}

func TestOutboundParams_EditText_EmptyMessageID(t *testing.T) {
	e := mkOpEnv()
	e.Meta[MetaMessageID] = ""
	e.SetEditText("new body")
	_, err := OutboundParams(e)
	if !errors.Is(err, ErrMissingTargetMessageID) {
		t.Errorf("err = %v, want ErrMissingTargetMessageID", err)
	}
}

func TestOutboundParams_EditText_InvalidMessageID(t *testing.T) {
	e := mkOpEnv()
	e.Meta[MetaMessageID] = "not-an-int"
	e.SetEditText("new body")
	_, err := OutboundParams(e)
	if !errors.Is(err, ErrMissingTargetMessageID) {
		t.Errorf("err = %v, want ErrMissingTargetMessageID (invalid int)", err)
	}
}

func TestOutboundParams_EditText_MissingChatID(t *testing.T) {
	e := envelope.New(ChannelName, envelope.Outbound, envelope.Participant{ID: "bot"})
	e.Meta[MetaMessageID] = "42"
	e.SetEditText("new body")
	_, err := OutboundParams(e)
	if !errors.Is(err, ErrMissingChatID) {
		t.Errorf("err = %v, want ErrMissingChatID", err)
	}
}

// ---------- OpEditCaption -------------------------------------------------

func TestOutboundParams_EditCaption_HappyPath(t *testing.T) {
	e := mkOpEnv()
	e.SetEditCaption("new caption")
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindEditCaption {
		t.Errorf("Kind = %v, want OutboundKindEditCaption", out.Kind)
	}
	if out.EditCaption == nil {
		t.Fatal("EditCaption is nil")
	}
	assertChatID(t, out.EditCaption.ChatID, 1000)
	if out.EditCaption.MessageID != 42 {
		t.Errorf("MessageID = %d, want 42", out.EditCaption.MessageID)
	}
	if out.EditCaption.Caption != "new caption" {
		t.Errorf("Caption = %q, want %q", out.EditCaption.Caption, "new caption")
	}
}

func TestOutboundParams_EditCaption_EmptyClears(t *testing.T) {
	// An empty caption is a legitimate intent: clear the existing
	// caption on the target message. EditMessageCaptionParams.Caption
	// is the empty string and the call goes out.
	e := mkOpEnv()
	e.SetEditCaption("")
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindEditCaption {
		t.Errorf("Kind = %v, want OutboundKindEditCaption", out.Kind)
	}
	if out.EditCaption.Caption != "" {
		t.Errorf("Caption = %q, want empty (clear)", out.EditCaption.Caption)
	}
}

func TestOutboundParams_EditCaption_WithKeyboard(t *testing.T) {
	e := mkOpEnv()
	e.SetEditCaption("new caption").WithKeyboard(
		[]envelope.Button{envelope.URLButton("Docs", "https://example.com")},
	)
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	assertInlineKeyboardSingle(t, out.EditCaption.ReplyMarkup, "Docs", "", "https://example.com")
}

func TestOutboundParams_EditCaption_MissingMessageID(t *testing.T) {
	e := mkOpEnv()
	delete(e.Meta, MetaMessageID)
	e.SetEditCaption("c")
	_, err := OutboundParams(e)
	if !errors.Is(err, ErrMissingTargetMessageID) {
		t.Errorf("err = %v, want ErrMissingTargetMessageID", err)
	}
}

// ---------- OpDelete ------------------------------------------------------

func TestOutboundParams_Delete_HappyPath(t *testing.T) {
	e := mkOpEnv()
	e.SetDelete()
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindDelete {
		t.Errorf("Kind = %v, want OutboundKindDelete", out.Kind)
	}
	if out.Delete == nil {
		t.Fatal("Delete is nil")
	}
	assertChatID(t, out.Delete.ChatID, 1000)
	if out.Delete.MessageID != 42 {
		t.Errorf("MessageID = %d, want 42", out.Delete.MessageID)
	}
}

func TestOutboundParams_Delete_MissingMessageID(t *testing.T) {
	e := mkOpEnv()
	delete(e.Meta, MetaMessageID)
	e.SetDelete()
	_, err := OutboundParams(e)
	if !errors.Is(err, ErrMissingTargetMessageID) {
		t.Errorf("err = %v, want ErrMissingTargetMessageID", err)
	}
}

func TestOutboundParams_Delete_MissingChatID(t *testing.T) {
	e := envelope.New(ChannelName, envelope.Outbound, envelope.Participant{ID: "bot"})
	e.Meta[MetaMessageID] = "42"
	e.SetDelete()
	_, err := OutboundParams(e)
	if !errors.Is(err, ErrMissingChatID) {
		t.Errorf("err = %v, want ErrMissingChatID", err)
	}
}

// ---------- OutboundKind.String -------------------------------------------

func TestOutboundKind_OperationStrings(t *testing.T) {
	cases := map[OutboundKind]string{
		OutboundKindEditText:    "edit_text",
		OutboundKindEditCaption: "edit_caption",
		OutboundKindDelete:      "delete",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("OutboundKind(%d).String() = %q, want %q", int(k), got, want)
		}
	}
}

// assertInlineKeyboardSingle asserts that the given ReplyMarkup is an
// InlineKeyboardMarkup with a single row of a single button matching
// the wanted Text / CallbackData / URL. Empty wantData / wantURL
// strings assert the corresponding field is empty on the
// materialised button.
func assertInlineKeyboardSingle(t *testing.T, m models.ReplyMarkup, wantText, wantData, wantURL string) {
	t.Helper()
	if m == nil {
		t.Fatal("ReplyMarkup is nil, want InlineKeyboardMarkup")
	}
	mk, ok := m.(models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("ReplyMarkup is %T, want models.InlineKeyboardMarkup", m)
	}
	if len(mk.InlineKeyboard) != 1 || len(mk.InlineKeyboard[0]) != 1 {
		t.Fatalf("InlineKeyboard shape = %v, want one row of one button", mk.InlineKeyboard)
	}
	b := mk.InlineKeyboard[0][0]
	if b.Text != wantText {
		t.Errorf("Text = %q, want %q", b.Text, wantText)
	}
	if b.CallbackData != wantData {
		t.Errorf("CallbackData = %q, want %q", b.CallbackData, wantData)
	}
	if b.URL != wantURL {
		t.Errorf("URL = %q, want %q", b.URL, wantURL)
	}
}

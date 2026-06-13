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
// Phase 2E.2 — outbound media. Envelope -> Telegram Send*Params dispatch.
// Symmetric to Phase 2E.1: when the outbound Envelope carries [Media, Text],
// the Text part becomes the Caption of the Send*Params for that media, not
// a separate message.
// ----------------------------------------------------------------------------

// ---------- test builders ---------------------------------------------------

// mkOutboundEnv builds a Telegram outbound envelope with chat_id set and the
// given parts appended.
func mkOutboundEnv(parts ...envelope.Part) *envelope.Envelope {
	e := envelope.New(ChannelName, envelope.Outbound, envelope.Participant{ID: "bot"})
	e.Meta[MetaChatID] = "1000"
	e.Parts = append(e.Parts, parts...)
	return e
}

func imagePart(source string) envelope.Part {
	return envelope.Part{Type: envelope.Image, Source: source}
}

func audioPart(source string) envelope.Part {
	return envelope.Part{Type: envelope.Audio, Source: source}
}

func videoPart(source string) envelope.Part {
	return envelope.Part{Type: envelope.Video, Source: source}
}

func filePart(source string) envelope.Part {
	return envelope.Part{Type: envelope.File, Source: source}
}

func textPart(content string) envelope.Part {
	return envelope.Part{Type: envelope.Text, Content: content}
}

// ---------- type assertion helpers -----------------------------------------

func assertChatID(t *testing.T, got any, want int64) {
	t.Helper()
	cid, ok := got.(int64)
	if !ok {
		t.Fatalf("ChatID is %T, want int64", got)
	}
	if cid != want {
		t.Errorf("ChatID = %d, want %d", cid, want)
	}
}

func assertInputFileString(t *testing.T, got models.InputFile, want string) {
	t.Helper()
	s, ok := got.(*models.InputFileString)
	if !ok {
		t.Fatalf("InputFile is %T, want *InputFileString", got)
	}
	if s.Data != want {
		t.Errorf("InputFileString.Data = %q, want %q", s.Data, want)
	}
}

// ---------- Happy path per media type --------------------------------------

func TestOutboundParams_Photo_NoCaption(t *testing.T) {
	e := mkOutboundEnv(imagePart("photo_fid"))
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindPhoto {
		t.Errorf("Kind = %v, want OutboundKindPhoto", out.Kind)
	}
	if out.Photo == nil {
		t.Fatal("Photo is nil")
	}
	assertChatID(t, out.Photo.ChatID, 1000)
	assertInputFileString(t, out.Photo.Photo, "photo_fid")
	if out.Photo.Caption != "" {
		t.Errorf("Caption = %q, want empty", out.Photo.Caption)
	}
	// Sibling fields must remain nil so the Outbound is a clean tagged union.
	if out.Message != nil || out.Document != nil || out.Voice != nil || out.Audio != nil || out.Video != nil {
		t.Error("only Photo should be populated")
	}
}

func TestOutboundParams_Photo_WithCaption(t *testing.T) {
	e := mkOutboundEnv(imagePart("photo_fid"), textPart("descripción"))
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindPhoto {
		t.Errorf("Kind = %v, want Photo", out.Kind)
	}
	if out.Photo.Caption != "descripción" {
		t.Errorf("Caption = %q, want %q", out.Photo.Caption, "descripción")
	}
}

func TestOutboundParams_Document_NoCaption(t *testing.T) {
	e := mkOutboundEnv(filePart("doc_fid"))
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindDocument {
		t.Errorf("Kind = %v, want Document", out.Kind)
	}
	assertChatID(t, out.Document.ChatID, 1000)
	assertInputFileString(t, out.Document.Document, "doc_fid")
}

func TestOutboundParams_Document_WithCaption(t *testing.T) {
	e := mkOutboundEnv(filePart("doc_fid"), textPart("informe Q3"))
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Document.Caption != "informe Q3" {
		t.Errorf("Caption = %q, want %q", out.Document.Caption, "informe Q3")
	}
}

func TestOutboundParams_Audio_DefaultUsesSendAudio(t *testing.T) {
	e := mkOutboundEnv(audioPart("audio_fid"))
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindAudio {
		t.Errorf("Kind = %v, want Audio (default for envelope.Audio with no MetaAudioKind)", out.Kind)
	}
	assertInputFileString(t, out.Audio.Audio, "audio_fid")
}

func TestOutboundParams_Audio_VoiceHintUsesSendVoice(t *testing.T) {
	e := mkOutboundEnv(audioPart("voice_fid"))
	e.Meta[MetaAudioKind] = AudioKindVoice
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindVoice {
		t.Errorf("Kind = %v, want Voice (MetaAudioKind = voice)", out.Kind)
	}
	if out.Voice == nil {
		t.Fatal("Voice is nil")
	}
	assertInputFileString(t, out.Voice.Voice, "voice_fid")
}

func TestOutboundParams_Audio_VoiceHintWithCaption(t *testing.T) {
	e := mkOutboundEnv(audioPart("voice_fid"), textPart("mensaje de voz"))
	e.Meta[MetaAudioKind] = AudioKindVoice
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Voice.Caption != "mensaje de voz" {
		t.Errorf("Voice.Caption = %q, want %q", out.Voice.Caption, "mensaje de voz")
	}
}

func TestOutboundParams_Audio_NonVoiceHintFallsBackToAudio(t *testing.T) {
	// Any value other than AudioKindVoice should be treated as audio.
	e := mkOutboundEnv(audioPart("audio_fid"))
	e.Meta[MetaAudioKind] = "unknown_value"
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindAudio {
		t.Errorf("Kind = %v, want Audio (only %q triggers Voice)", out.Kind, AudioKindVoice)
	}
}

func TestOutboundParams_Video_NoCaption(t *testing.T) {
	e := mkOutboundEnv(videoPart("vid_fid"))
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindVideo {
		t.Errorf("Kind = %v, want Video", out.Kind)
	}
	assertInputFileString(t, out.Video.Video, "vid_fid")
}

func TestOutboundParams_Video_WithCaption(t *testing.T) {
	e := mkOutboundEnv(videoPart("vid_fid"), textPart("clip"))
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Video.Caption != "clip" {
		t.Errorf("Caption = %q, want %q", out.Video.Caption, "clip")
	}
}

// ---------- Text only (delegates to SendMessage) --------------------------

func TestOutboundParams_TextOnly_DelegatesToSendMessage(t *testing.T) {
	e := mkOutboundEnv(textPart("hola"))
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindMessage {
		t.Errorf("Kind = %v, want Message", out.Kind)
	}
	if out.Message == nil {
		t.Fatal("Message is nil")
	}
	if out.Message.Text != "hola" {
		t.Errorf("Text = %q", out.Message.Text)
	}
	assertChatID(t, out.Message.ChatID, 1000)
}

func TestOutboundParams_TextOnly_FirstNonEmptyTextUsed(t *testing.T) {
	e := mkOutboundEnv(textPart(""), textPart("primero"), textPart("segundo"))
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Message.Text != "primero" {
		t.Errorf("Text = %q, want %q", out.Message.Text, "primero")
	}
}

// ---------- Caption: first non-empty text -----------------------------------

func TestOutboundParams_Photo_FirstNonEmptyTextUsedAsCaption(t *testing.T) {
	e := mkOutboundEnv(imagePart("p"), textPart(""), textPart("primera caption"), textPart("ignorada"))
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Photo.Caption != "primera caption" {
		t.Errorf("Caption = %q, want %q", out.Photo.Caption, "primera caption")
	}
}

// ---------- Errors --------------------------------------------------------

func TestOutboundParams_NilEnvelope(t *testing.T) {
	_, err := OutboundParams(nil)
	if !errors.Is(err, ErrNilEnvelope) {
		t.Errorf("err = %v, want ErrNilEnvelope", err)
	}
}

func TestOutboundParams_NotOutbound(t *testing.T) {
	e := envelope.New(ChannelName, envelope.Inbound, envelope.Participant{ID: "u"})
	e.AddText("x")
	e.Meta[MetaChatID] = "1000"
	_, err := OutboundParams(e)
	if !errors.Is(err, ErrNotOutbound) {
		t.Errorf("err = %v, want ErrNotOutbound", err)
	}
}

func TestOutboundParams_WrongChannel(t *testing.T) {
	e := envelope.New("webhook", envelope.Outbound, envelope.Participant{ID: "bot"})
	e.AddText("x")
	e.Meta[MetaChatID] = "1000"
	_, err := OutboundParams(e)
	if !errors.Is(err, ErrWrongChannel) {
		t.Errorf("err = %v, want ErrWrongChannel", err)
	}
}

func TestOutboundParams_MissingChatID(t *testing.T) {
	e := envelope.New(ChannelName, envelope.Outbound, envelope.Participant{ID: "bot"})
	e.AddText("x")
	_, err := OutboundParams(e)
	if !errors.Is(err, ErrMissingChatID) {
		t.Errorf("err = %v, want ErrMissingChatID", err)
	}
}

func TestOutboundParams_EmptyChatID(t *testing.T) {
	e := mkOutboundEnv(textPart("x"))
	e.Meta[MetaChatID] = ""
	_, err := OutboundParams(e)
	if !errors.Is(err, ErrMissingChatID) {
		t.Errorf("err = %v, want ErrMissingChatID", err)
	}
}

func TestOutboundParams_InvalidChatID(t *testing.T) {
	e := mkOutboundEnv(textPart("x"))
	e.Meta[MetaChatID] = "not-an-int"
	_, err := OutboundParams(e)
	if !errors.Is(err, ErrInvalidChatID) {
		t.Errorf("err = %v, want ErrInvalidChatID", err)
	}
}

func TestOutboundParams_NoPartsAtAll(t *testing.T) {
	e := envelope.New(ChannelName, envelope.Outbound, envelope.Participant{ID: "bot"})
	e.Meta[MetaChatID] = "1000"
	_, err := OutboundParams(e)
	if !errors.Is(err, ErrNoPartsToSend) {
		t.Errorf("err = %v, want ErrNoPartsToSend", err)
	}
}

func TestOutboundParams_TextOnlyEmptyContent(t *testing.T) {
	// A single text part with empty content is still nothing to send.
	e := mkOutboundEnv(textPart(""))
	_, err := OutboundParams(e)
	if !errors.Is(err, ErrNoPartsToSend) {
		t.Errorf("err = %v, want ErrNoPartsToSend", err)
	}
}

func TestOutboundParams_TooManyMediaParts_SameType(t *testing.T) {
	e := mkOutboundEnv(imagePart("p1"), imagePart("p2"))
	_, err := OutboundParams(e)
	if !errors.Is(err, ErrTooManyMediaParts) {
		t.Errorf("err = %v, want ErrTooManyMediaParts", err)
	}
}

func TestOutboundParams_TooManyMediaParts_MixedTypes(t *testing.T) {
	e := mkOutboundEnv(imagePart("p1"), videoPart("v1"))
	_, err := OutboundParams(e)
	if !errors.Is(err, ErrTooManyMediaParts) {
		t.Errorf("err = %v, want ErrTooManyMediaParts", err)
	}
}

func TestOutboundParams_MissingMediaSource(t *testing.T) {
	e := mkOutboundEnv(envelope.Part{Type: envelope.Image, Source: ""})
	_, err := OutboundParams(e)
	if !errors.Is(err, ErrMissingMediaSource) {
		t.Errorf("err = %v, want ErrMissingMediaSource", err)
	}
}

// ---------- OutboundKind.String --------------------------------------------

func TestOutboundKind_String(t *testing.T) {
	cases := map[OutboundKind]string{
		OutboundKindMessage:  "message",
		OutboundKindPhoto:    "photo",
		OutboundKindDocument: "document",
		OutboundKindVoice:    "voice",
		OutboundKindAudio:    "audio",
		OutboundKindVideo:    "video",
		OutboundKind(99):     "unknown(99)",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("OutboundKind(%d).String() = %q, want %q", int(k), got, want)
		}
	}
}
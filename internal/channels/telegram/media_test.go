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
// Phase 2E.1 — inbound media.
// One fixture per Telegram media type; verifies the conversion to the
// canonical Envelope. No network: the test feeds *models.Update directly.
// ----------------------------------------------------------------------------

// ---------- Photo ----------------------------------------------------------

func TestInboundFromUpdate_Photo_PicksLargestSize(t *testing.T) {
	u := loadUpdateFixture(t, "photo_message.json")
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if len(env.Parts) != 1 {
		t.Fatalf("Parts len = %d, want 1", len(env.Parts))
	}
	got := env.Parts[0]
	if got.Type != envelope.Image {
		t.Errorf("Type = %v, want Image", got.Type)
	}
	// PhotoSizes in the fixture have file_size {1500, 18000, 95000}; the
	// largest by FileSize is the 800x800 entry with file_id PhotoLarge_FID.
	if got.Source != "PhotoLarge_FID" {
		t.Errorf("Source = %q, want %q (largest PhotoSize selected)", got.Source, "PhotoLarge_FID")
	}
	if got.MIMEType != "" {
		t.Errorf("MIMEType = %q, want empty (PhotoSize has no mime_type)", got.MIMEType)
	}
}

func TestInboundFromUpdate_Photo_SinglePhotoSize(t *testing.T) {
	// Edge case: only one PhotoSize entry. Must still pick it.
	u := &models.Update{
		Message: &models.Message{
			ID:   1,
			Date: 1000,
			From: &models.User{ID: 9, Username: "alice"},
			Chat: models.Chat{ID: 5, Type: "private"},
			Photo: []models.PhotoSize{
				{FileID: "OnlyOne", FileUniqueID: "u", Width: 100, Height: 100, FileSize: 5000},
			},
		},
	}
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if len(env.Parts) != 1 || env.Parts[0].Source != "OnlyOne" {
		t.Errorf("Parts = %+v, want one Image with Source=OnlyOne", env.Parts)
	}
}

// ---------- Voice ----------------------------------------------------------

func TestInboundFromUpdate_Voice(t *testing.T) {
	u := loadUpdateFixture(t, "voice_message.json")
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if len(env.Parts) != 1 {
		t.Fatalf("Parts len = %d, want 1", len(env.Parts))
	}
	got := env.Parts[0]
	if got.Type != envelope.Audio {
		t.Errorf("Type = %v, want Audio", got.Type)
	}
	if got.Source != "VoiceNote_FID" {
		t.Errorf("Source = %q, want VoiceNote_FID", got.Source)
	}
	if got.MIMEType != "audio/ogg" {
		t.Errorf("MIMEType = %q, want audio/ogg", got.MIMEType)
	}
}

// ---------- Audio ----------------------------------------------------------

func TestInboundFromUpdate_Audio(t *testing.T) {
	u := loadUpdateFixture(t, "audio_message.json")
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if len(env.Parts) != 1 {
		t.Fatalf("Parts len = %d, want 1", len(env.Parts))
	}
	got := env.Parts[0]
	if got.Type != envelope.Audio {
		t.Errorf("Type = %v, want Audio", got.Type)
	}
	if got.Source != "AudioTrack_FID" {
		t.Errorf("Source = %q, want AudioTrack_FID", got.Source)
	}
	if got.MIMEType != "audio/mpeg" {
		t.Errorf("MIMEType = %q, want audio/mpeg", got.MIMEType)
	}
}

// ---------- Video ----------------------------------------------------------

func TestInboundFromUpdate_Video(t *testing.T) {
	u := loadUpdateFixture(t, "video_message.json")
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if len(env.Parts) != 1 {
		t.Fatalf("Parts len = %d, want 1", len(env.Parts))
	}
	got := env.Parts[0]
	if got.Type != envelope.Video {
		t.Errorf("Type = %v, want Video", got.Type)
	}
	if got.Source != "Video_FID" {
		t.Errorf("Source = %q, want Video_FID", got.Source)
	}
	if got.MIMEType != "video/mp4" {
		t.Errorf("MIMEType = %q, want video/mp4", got.MIMEType)
	}
}

// ---------- Document ------------------------------------------------------

func TestInboundFromUpdate_Document(t *testing.T) {
	u := loadUpdateFixture(t, "document_message.json")
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if len(env.Parts) != 1 {
		t.Fatalf("Parts len = %d, want 1", len(env.Parts))
	}
	got := env.Parts[0]
	if got.Type != envelope.File {
		t.Errorf("Type = %v, want File", got.Type)
	}
	if got.Source != "Document_FID" {
		t.Errorf("Source = %q, want Document_FID", got.Source)
	}
	if got.MIMEType != "application/pdf" {
		t.Errorf("MIMEType = %q, want application/pdf", got.MIMEType)
	}
}

// ---------- Caption -------------------------------------------------------

func TestInboundFromUpdate_PhotoWithCaption_OrderIsMediaThenText(t *testing.T) {
	u := loadUpdateFixture(t, "photo_with_caption.json")
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if len(env.Parts) != 2 {
		t.Fatalf("Parts len = %d, want 2 (media + caption text)", len(env.Parts))
	}
	if env.Parts[0].Type != envelope.Image {
		t.Errorf("Parts[0].Type = %v, want Image (media first)", env.Parts[0].Type)
	}
	if env.Parts[0].Source != "CaptionedPhoto_FID" {
		t.Errorf("Parts[0].Source = %q, want CaptionedPhoto_FID", env.Parts[0].Source)
	}
	if env.Parts[1].Type != envelope.Text {
		t.Errorf("Parts[1].Type = %v, want Text (caption second)", env.Parts[1].Type)
	}
	if env.Parts[1].Content != "una foto con descripción" {
		t.Errorf("Parts[1].Content = %q, want %q", env.Parts[1].Content, "una foto con descripción")
	}
}

func TestInboundFromUpdate_DocumentWithCaption(t *testing.T) {
	u := loadUpdateFixture(t, "document_with_caption.json")
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if len(env.Parts) != 2 {
		t.Fatalf("Parts len = %d, want 2", len(env.Parts))
	}
	if env.Parts[0].Type != envelope.File {
		t.Errorf("Parts[0].Type = %v, want File", env.Parts[0].Type)
	}
	if env.Parts[1].Type != envelope.Text {
		t.Errorf("Parts[1].Type = %v, want Text", env.Parts[1].Type)
	}
	if env.Parts[1].Content != "informe del Q3" {
		t.Errorf("Parts[1].Content = %q, want %q", env.Parts[1].Content, "informe del Q3")
	}
}

// ---------- Media without caption / no text ------------------------------

func TestInboundFromUpdate_MediaWithoutText_NoCaption(t *testing.T) {
	// voice_message.json has voice but neither text nor caption.
	// The resulting Envelope must carry exactly one media Part.
	u := loadUpdateFixture(t, "voice_message.json")
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if len(env.Parts) != 1 {
		t.Errorf("Parts len = %d, want exactly 1 (no caption/text)", len(env.Parts))
	}
}

// ---------- Validate output ----------------------------------------------

func TestInboundFromUpdate_MediaEnvelopePassesValidate(t *testing.T) {
	fixtures := []string{
		"photo_message.json",
		"voice_message.json",
		"audio_message.json",
		"video_message.json",
		"document_message.json",
		"photo_with_caption.json",
		"document_with_caption.json",
	}
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			u := loadUpdateFixture(t, name)
			env, err := InboundFromUpdate(u)
			if err != nil {
				t.Fatalf("InboundFromUpdate: %v", err)
			}
			if err := env.Validate(); err != nil {
				t.Errorf("Validate: %v", err)
			}
		})
	}
}

// ---------- Empty message regression -------------------------------------

func TestInboundFromUpdate_EmptyMessage_NoMediaNoText_StillUnsupported(t *testing.T) {
	// A message with no text, no caption, and no media is the only
	// case 2E.1 still treats as unsupported content.
	u := &models.Update{
		Message: &models.Message{
			ID:   1,
			Date: 1000,
			From: &models.User{ID: 9, Username: "alice"},
			Chat: models.Chat{ID: 5, Type: "private"},
		},
	}
	_, err := InboundFromUpdate(u)
	if !errors.Is(err, ErrUnsupportedContent) {
		t.Errorf("err = %v, want ErrUnsupportedContent", err)
	}
}
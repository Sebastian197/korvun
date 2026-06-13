// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package envelope

import (
	"testing"
	"time"
)

func TestNew_creates_valid_envelope(t *testing.T) {
	env := New("telegram", Inbound, Participant{ID: "user-1", Name: "Alice"})

	if env.ID == "" {
		t.Error("New() should generate a non-empty ID")
	}
	if env.Channel != "telegram" {
		t.Errorf("Channel = %q, want %q", env.Channel, "telegram")
	}
	if env.Direction != Inbound {
		t.Errorf("Direction = %v, want %v", env.Direction, Inbound)
	}
	if env.Sender.ID != "user-1" {
		t.Errorf("Sender.ID = %q, want %q", env.Sender.ID, "user-1")
	}
	if env.Timestamp.IsZero() {
		t.Error("New() should set Timestamp")
	}
	if env.Meta == nil {
		t.Error("New() must initialize Meta map")
	}
	if len(env.Parts) != 0 {
		t.Errorf("New() should start with 0 parts, got %d", len(env.Parts))
	}
}

func TestNew_timestamp_is_recent(t *testing.T) {
	before := time.Now()
	env := New("webhook", Outbound, Participant{ID: "bot"})
	after := time.Now()

	if env.Timestamp.Before(before) || env.Timestamp.After(after) {
		t.Errorf("Timestamp %v not between %v and %v", env.Timestamp, before, after)
	}
}

func TestBuilder_AddText(t *testing.T) {
	env := New("telegram", Inbound, Participant{ID: "user-1"})
	env.AddText("hello").AddText("world")

	if len(env.Parts) != 2 {
		t.Fatalf("Parts len = %d, want 2", len(env.Parts))
	}

	tests := []struct {
		idx     int
		content string
	}{
		{0, "hello"},
		{1, "world"},
	}
	for _, tt := range tests {
		p := env.Parts[tt.idx]
		if p.Type != Text {
			t.Errorf("Parts[%d].Type = %v, want Text", tt.idx, p.Type)
		}
		if p.Content != tt.content {
			t.Errorf("Parts[%d].Content = %q, want %q", tt.idx, p.Content, tt.content)
		}
	}
}

func TestBuilder_AddMedia(t *testing.T) {
	env := New("webhook", Inbound, Participant{ID: "user-1"})
	env.AddMedia(Image, "https://example.com/photo.jpg", "image/jpeg")

	if len(env.Parts) != 1 {
		t.Fatalf("Parts len = %d, want 1", len(env.Parts))
	}

	p := env.Parts[0]
	if p.Type != Image {
		t.Errorf("Type = %v, want Image", p.Type)
	}
	if p.Source != "https://example.com/photo.jpg" {
		t.Errorf("Source = %q, want url", p.Source)
	}
	if p.MIMEType != "image/jpeg" {
		t.Errorf("MIMEType = %q, want %q", p.MIMEType, "image/jpeg")
	}
}

func TestBuilder_chaining(t *testing.T) {
	env := New("telegram", Inbound, Participant{ID: "user-1"})
	result := env.
		AddText("check this out").
		AddMedia(Image, "https://example.com/img.png", "image/png").
		AddMedia(Audio, "https://example.com/clip.mp3", "audio/mpeg")

	if result != env {
		t.Error("Builder methods should return the same *Envelope for chaining")
	}
	if len(env.Parts) != 3 {
		t.Fatalf("Parts len = %d, want 3", len(env.Parts))
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package envelope

import (
	"testing"
	"time"
)

func TestDirection_String(t *testing.T) {
	tests := []struct {
		name string
		d    Direction
		want string
	}{
		{"inbound", Inbound, "inbound"},
		{"outbound", Outbound, "outbound"},
		{"unknown", Direction(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.String(); got != tt.want {
				t.Errorf("Direction.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPartType_String(t *testing.T) {
	tests := []struct {
		name string
		pt   PartType
		want string
	}{
		{"text", Text, "text"},
		{"image", Image, "image"},
		{"audio", Audio, "audio"},
		{"video", Video, "video"},
		{"file", File, "file"},
		{"unknown", PartType(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pt.String(); got != tt.want {
				t.Errorf("PartType.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParticipant_fields(t *testing.T) {
	p := Participant{
		ID:   "user-123",
		Name: "Alice",
	}
	if p.ID != "user-123" {
		t.Errorf("Participant.ID = %q, want %q", p.ID, "user-123")
	}
	if p.Name != "Alice" {
		t.Errorf("Participant.Name = %q, want %q", p.Name, "Alice")
	}
}

func TestPart_text(t *testing.T) {
	p := Part{
		Type:    Text,
		Content: "hello world",
	}
	if p.Type != Text {
		t.Errorf("Part.Type = %v, want %v", p.Type, Text)
	}
	if p.Content != "hello world" {
		t.Errorf("Part.Content = %q, want %q", p.Content, "hello world")
	}
}

func TestPart_media(t *testing.T) {
	p := Part{
		Type:     Image,
		Source:   "https://example.com/img.png",
		MIMEType: "image/png",
	}
	if p.Type != Image {
		t.Errorf("Part.Type = %v, want %v", p.Type, Image)
	}
	if p.Source != "https://example.com/img.png" {
		t.Errorf("Part.Source = %q, want url", p.Source)
	}
	if p.MIMEType != "image/png" {
		t.Errorf("Part.MIMEType = %q, want %q", p.MIMEType, "image/png")
	}
}

func TestEnvelope_defaults(t *testing.T) {
	env := Envelope{
		ID:        "test-id",
		Channel:   "telegram",
		Direction: Inbound,
		Sender: Participant{
			ID:   "user-1",
			Name: "Bob",
		},
		Parts: []Part{
			{Type: Text, Content: "hi"},
		},
		Timestamp: time.Now(),
	}

	if env.ID != "test-id" {
		t.Errorf("Envelope.ID = %q, want %q", env.ID, "test-id")
	}
	if env.Channel != "telegram" {
		t.Errorf("Envelope.Channel = %q, want %q", env.Channel, "telegram")
	}
	if env.Direction != Inbound {
		t.Errorf("Envelope.Direction = %v, want %v", env.Direction, Inbound)
	}
	if len(env.Parts) != 1 {
		t.Fatalf("Envelope.Parts len = %d, want 1", len(env.Parts))
	}
}

func TestEnvelope_meta_explicit(t *testing.T) {
	env := Envelope{
		ID:        "test-id",
		Channel:   "webhook",
		Direction: Outbound,
		Sender:    Participant{ID: "bot"},
		Parts:     []Part{{Type: Text, Content: "reply"}},
		Timestamp: time.Now(),
		Meta:      map[string]string{"key": "value"},
	}

	if env.Meta["key"] != "value" {
		t.Errorf("Envelope.Meta[key] = %q, want %q", env.Meta["key"], "value")
	}
}

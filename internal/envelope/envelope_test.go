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
		{"location", Location, "location"},
		{"callback", Callback, "callback"},
		{"reaction", Reaction, "reaction"},
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

func TestOperationKind_String(t *testing.T) {
	tests := []struct {
		name string
		k    OperationKind
		want string
	}{
		{"edit_text", OpEditText, "edit_text"},
		{"edit_caption", OpEditCaption, "edit_caption"},
		{"delete", OpDelete, "delete"},
		{"callback_ack", OpCallbackAck, "callback_ack"},
		{"set_reaction", OpSetReaction, "set_reaction"},
		{"unknown", OperationKind(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.k.String(); got != tt.want {
				t.Errorf("OperationKind.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestPartType_WireFormatPositions anchors the integer values of every
// PartType constant. The previous CallbackAck slot (7) was retired by
// ADR-0006 and is intentionally NOT reused; the next PartType added
// (Reaction in ADR-0007) jumps to 8 so a hypothetical old marshalled
// envelope carrying type=7 never silently deserialises into a new
// content kind.
func TestPartType_WireFormatPositions(t *testing.T) {
	cases := []struct {
		name string
		pt   PartType
		want int
	}{
		{"Text", Text, 0},
		{"Image", Image, 1},
		{"Audio", Audio, 2},
		{"Video", Video, 3},
		{"File", File, 4},
		{"Location", Location, 5},
		{"Callback", Callback, 6},
		// slot 7 retired (was CallbackAck before ADR-0006 migration)
		{"Reaction", Reaction, 8},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if int(tt.pt) != tt.want {
				t.Errorf("PartType %s = %d, want %d", tt.name, int(tt.pt), tt.want)
			}
		})
	}
}

func TestOperationKind_WireFormatPositions(t *testing.T) {
	cases := []struct {
		name string
		k    OperationKind
		want int
	}{
		{"OpEditText", OpEditText, 0},
		{"OpEditCaption", OpEditCaption, 1},
		{"OpDelete", OpDelete, 2},
		{"OpCallbackAck", OpCallbackAck, 3},
		{"OpSetReaction", OpSetReaction, 4},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if int(tt.k) != tt.want {
				t.Errorf("OperationKind %s = %d, want %d", tt.name, int(tt.k), tt.want)
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

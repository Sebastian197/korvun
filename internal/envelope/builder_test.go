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

func TestBuilder_AddLocation(t *testing.T) {
	tests := []struct {
		name     string
		lat, lon float64
	}{
		{"barcelona", 41.40338, 2.17403},
		{"sydney negative lon", -33.8688, 151.2093},
		{"null island origin", 0, 0},
		{"antipode all negative", -41.40338, -2.17403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := New("telegram", Inbound, Participant{ID: "user-1"})
			result := env.AddLocation(tt.lat, tt.lon)
			if result != env {
				t.Error("AddLocation should return the same *Envelope for chaining")
			}
			if len(env.Parts) != 1 {
				t.Fatalf("Parts len = %d, want 1", len(env.Parts))
			}
			p := env.Parts[0]
			if p.Type != Location {
				t.Errorf("Parts[0].Type = %v, want Location", p.Type)
			}
			if p.Source != "" {
				t.Errorf("Parts[0].Source = %q, want empty for Location", p.Source)
			}
			if p.MIMEType != "" {
				t.Errorf("Parts[0].MIMEType = %q, want empty for Location", p.MIMEType)
			}
			lat, lon, ok := p.Location()
			if !ok {
				t.Fatalf("Part.Location() ok = false, want true; content=%q", p.Content)
			}
			if lat != tt.lat || lon != tt.lon {
				t.Errorf("Part.Location() = (%v, %v), want (%v, %v)", lat, lon, tt.lat, tt.lon)
			}
		})
	}
}

func TestPart_Location_accessor_negative(t *testing.T) {
	tests := []struct {
		name string
		part Part
	}{
		{
			name: "non-location type returns ok=false",
			part: Part{Type: Text, Content: `{"lat":1,"lon":2}`},
		},
		{
			name: "location with empty content",
			part: Part{Type: Location, Content: ""},
		},
		{
			name: "location with non-JSON content",
			part: Part{Type: Location, Content: "41.40338,2.17403"},
		},
		{
			name: "location with JSON missing lat",
			part: Part{Type: Location, Content: `{"lon":2.17403}`},
		},
		{
			name: "location with JSON missing lon",
			part: Part{Type: Location, Content: `{"lat":41.40338}`},
		},
		{
			name: "location with JSON wrong type for lat",
			part: Part{Type: Location, Content: `{"lat":"x","lon":2}`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, ok := tt.part.Location(); ok {
				t.Errorf("Part.Location() ok = true, want false")
			}
		})
	}
}

func TestPart_Location_tolerates_unknown_keys(t *testing.T) {
	p := Part{
		Type:    Location,
		Content: `{"lat":41.40338,"lon":2.17403,"accuracy":12.5,"live_period":600}`,
	}
	lat, lon, ok := p.Location()
	if !ok {
		t.Fatalf("Part.Location() with unknown extra keys ok = false, want true")
	}
	if lat != 41.40338 || lon != 2.17403 {
		t.Errorf("Part.Location() = (%v, %v), want (41.40338, 2.17403)", lat, lon)
	}
}

func TestBuilder_AddCallback(t *testing.T) {
	env := New("telegram", Inbound, Participant{ID: "user-1"})
	result := env.AddCallback("button_yes")
	if result != env {
		t.Error("AddCallback should return the same *Envelope for chaining")
	}
	if len(env.Parts) != 1 {
		t.Fatalf("Parts len = %d, want 1", len(env.Parts))
	}
	p := env.Parts[0]
	if p.Type != Callback {
		t.Errorf("Type = %v, want Callback", p.Type)
	}
	if p.Content != "button_yes" {
		t.Errorf("Content = %q, want %q", p.Content, "button_yes")
	}
	if p.Source != "" || p.MIMEType != "" {
		t.Errorf("Source/MIMEType must be empty on Callback (got %q / %q)", p.Source, p.MIMEType)
	}
}

func TestBuilder_AddCallbackAck(t *testing.T) {
	tests := []struct {
		name  string
		toast string
	}{
		{"silent ack (empty toast)", ""},
		{"ack with toast", "Saved!"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := New("telegram", Outbound, Participant{ID: "bot"})
			result := env.AddCallbackAck(tt.toast)
			if result != env {
				t.Error("AddCallbackAck should return the same *Envelope for chaining")
			}
			if len(env.Parts) != 1 {
				t.Fatalf("Parts len = %d, want 1", len(env.Parts))
			}
			p := env.Parts[0]
			if p.Type != CallbackAck {
				t.Errorf("Type = %v, want CallbackAck", p.Type)
			}
			if p.Content != tt.toast {
				t.Errorf("Content = %q, want %q", p.Content, tt.toast)
			}
			if p.Source != "" || p.MIMEType != "" {
				t.Errorf("Source/MIMEType must be empty on CallbackAck")
			}
		})
	}
}

func TestBuilder_WithKeyboard_SingleRow(t *testing.T) {
	env := New("telegram", Outbound, Participant{ID: "bot"})
	result := env.AddText("Choose:").WithKeyboard(
		[]Button{CallbackButton("Yes", "yes"), CallbackButton("No", "no")},
	)
	if result != env {
		t.Error("WithKeyboard should return the same *Envelope for chaining")
	}
	if env.Keyboard == nil {
		t.Fatal("Keyboard is nil after WithKeyboard")
	}
	if len(env.Keyboard.Rows) != 1 {
		t.Fatalf("Rows len = %d, want 1", len(env.Keyboard.Rows))
	}
	if len(env.Keyboard.Rows[0]) != 2 {
		t.Fatalf("Rows[0] len = %d, want 2", len(env.Keyboard.Rows[0]))
	}
	if env.Keyboard.Rows[0][0].Text != "Yes" || env.Keyboard.Rows[0][0].CallbackData != "yes" {
		t.Errorf("Rows[0][0] = %+v, want Yes/yes", env.Keyboard.Rows[0][0])
	}
	if env.Keyboard.Rows[0][1].Text != "No" || env.Keyboard.Rows[0][1].CallbackData != "no" {
		t.Errorf("Rows[0][1] = %+v, want No/no", env.Keyboard.Rows[0][1])
	}
}

func TestBuilder_WithKeyboard_MultipleRows_MixedKinds(t *testing.T) {
	env := New("telegram", Outbound, Participant{ID: "bot"})
	env.AddText("Pick:").WithKeyboard(
		[]Button{CallbackButton("Buy", "buy")},
		[]Button{URLButton("Docs", "https://example.com/docs")},
	)
	if len(env.Keyboard.Rows) != 2 {
		t.Fatalf("Rows len = %d, want 2", len(env.Keyboard.Rows))
	}
	if env.Keyboard.Rows[0][0].CallbackData != "buy" {
		t.Errorf("Rows[0][0].CallbackData = %q, want %q", env.Keyboard.Rows[0][0].CallbackData, "buy")
	}
	if env.Keyboard.Rows[0][0].URL != "" {
		t.Errorf("callback button should not set URL, got %q", env.Keyboard.Rows[0][0].URL)
	}
	if env.Keyboard.Rows[1][0].URL != "https://example.com/docs" {
		t.Errorf("Rows[1][0].URL = %q", env.Keyboard.Rows[1][0].URL)
	}
	if env.Keyboard.Rows[1][0].CallbackData != "" {
		t.Errorf("URL button should not set CallbackData, got %q", env.Keyboard.Rows[1][0].CallbackData)
	}
}

func TestButton_Constructors(t *testing.T) {
	cb := CallbackButton("Yes", "yes")
	if cb.Text != "Yes" || cb.CallbackData != "yes" || cb.URL != "" {
		t.Errorf("CallbackButton = %+v, want Text=Yes CallbackData=yes URL=empty", cb)
	}
	url := URLButton("Open", "https://example.com")
	if url.Text != "Open" || url.URL != "https://example.com" || url.CallbackData != "" {
		t.Errorf("URLButton = %+v, want Text=Open URL=https://example.com CallbackData=empty", url)
	}
}

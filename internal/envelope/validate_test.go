// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package envelope

import (
	"encoding/json"
	"testing"
	"time"
)

func TestValidate_valid_envelope(t *testing.T) {
	env := New("telegram", Inbound, Participant{ID: "user-1"})
	env.AddText("hello")

	if err := env.Validate(); err != nil {
		t.Errorf("Validate() returned error for valid envelope: %v", err)
	}
}

func TestValidate_valid_callback_envelopes(t *testing.T) {
	tests := []struct {
		name string
		env  *Envelope
	}{
		{
			name: "inbound callback with data",
			env: func() *Envelope {
				e := New("telegram", Inbound, Participant{ID: "user-1"})
				return e.AddCallback("button_yes")
			}(),
		},
		{
			name: "outbound silent ack (empty Content)",
			env: func() *Envelope {
				e := New("telegram", Outbound, Participant{ID: "bot"})
				return e.AddCallbackAck("")
			}(),
		},
		{
			name: "outbound ack with toast",
			env: func() *Envelope {
				e := New("telegram", Outbound, Participant{ID: "bot"})
				return e.AddCallbackAck("Saved!")
			}(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.env.Validate(); err != nil {
				t.Errorf("Validate() returned error: %v", err)
			}
		})
	}
}

func TestValidate_valid_keyboard_envelopes(t *testing.T) {
	tests := []struct {
		name string
		env  *Envelope
	}{
		{
			name: "text + single-row keyboard with callback buttons",
			env: func() *Envelope {
				e := New("telegram", Outbound, Participant{ID: "bot"})
				return e.AddText("Choose:").WithKeyboard(
					[]Button{CallbackButton("Yes", "yes"), CallbackButton("No", "no")},
				)
			}(),
		},
		{
			name: "text + multi-row keyboard with mixed kinds",
			env: func() *Envelope {
				e := New("telegram", Outbound, Participant{ID: "bot"})
				return e.AddText("Pick:").WithKeyboard(
					[]Button{CallbackButton("Buy", "buy")},
					[]Button{URLButton("Docs", "https://example.com")},
				)
			}(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.env.Validate(); err != nil {
				t.Errorf("Validate() returned error: %v", err)
			}
		})
	}
}

func TestValidate_valid_location_envelopes(t *testing.T) {
	tests := []struct {
		name string
		env  *Envelope
	}{
		{
			name: "AddLocation builder",
			env: func() *Envelope {
				e := New("telegram", Inbound, Participant{ID: "user-1"})
				return e.AddLocation(41.40338, 2.17403)
			}(),
		},
		{
			name: "Null Island (0,0) is valid",
			env: func() *Envelope {
				e := New("telegram", Inbound, Participant{ID: "user-1"})
				return e.AddLocation(0, 0)
			}(),
		},
		{
			name: "unknown extra keys are tolerated (forward compat)",
			env: func() *Envelope {
				e := New("telegram", Inbound, Participant{ID: "user-1"})
				e.Parts = append(e.Parts, Part{
					Type:    Location,
					Content: `{"lat":41.40338,"lon":2.17403,"accuracy":12.5,"live_period":600}`,
				})
				return e
			}(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.env.Validate(); err != nil {
				t.Errorf("Validate() returned error for valid location envelope: %v", err)
			}
		})
	}
}

func TestValidate_errors(t *testing.T) {
	base := func() *Envelope {
		env := New("telegram", Inbound, Participant{ID: "user-1"})
		env.AddText("hello")
		return env
	}

	tests := []struct {
		name    string
		modify  func(e *Envelope)
		wantErr string
	}{
		{
			name:    "empty ID",
			modify:  func(e *Envelope) { e.ID = "" },
			wantErr: "empty ID",
		},
		{
			name:    "empty channel",
			modify:  func(e *Envelope) { e.Channel = "" },
			wantErr: "empty channel",
		},
		{
			name:    "invalid direction",
			modify:  func(e *Envelope) { e.Direction = Direction(99) },
			wantErr: "invalid direction",
		},
		{
			name:    "empty sender ID",
			modify:  func(e *Envelope) { e.Sender.ID = "" },
			wantErr: "empty sender ID",
		},
		{
			name:    "no parts",
			modify:  func(e *Envelope) { e.Parts = nil },
			wantErr: "no parts",
		},
		{
			name:    "empty parts slice",
			modify:  func(e *Envelope) { e.Parts = []Part{} },
			wantErr: "no parts",
		},
		{
			name: "text part with empty content",
			modify: func(e *Envelope) {
				e.Parts = []Part{{Type: Text, Content: ""}}
			},
			wantErr: "empty content",
		},
		{
			name: "media part without source",
			modify: func(e *Envelope) {
				e.Parts = []Part{{Type: Image, Source: ""}}
			},
			wantErr: "empty source",
		},
		{
			name:    "zero timestamp",
			modify:  func(e *Envelope) { e.Timestamp = time.Time{} },
			wantErr: "zero timestamp",
		},
		{
			name: "location part with empty content",
			modify: func(e *Envelope) {
				e.Parts = []Part{{Type: Location, Content: ""}}
			},
			wantErr: "empty content",
		},
		{
			name: "location part with non-JSON content",
			modify: func(e *Envelope) {
				e.Parts = []Part{{Type: Location, Content: "41.40338,2.17403"}}
			},
			wantErr: "invalid location",
		},
		{
			name: "location part missing lat",
			modify: func(e *Envelope) {
				e.Parts = []Part{{Type: Location, Content: `{"lon":2.17403}`}}
			},
			wantErr: "invalid location",
		},
		{
			name: "location part missing lon",
			modify: func(e *Envelope) {
				e.Parts = []Part{{Type: Location, Content: `{"lat":41.40338}`}}
			},
			wantErr: "invalid location",
		},
		{
			name: "location part with wrong lat type",
			modify: func(e *Envelope) {
				e.Parts = []Part{{Type: Location, Content: `{"lat":"x","lon":2}`}}
			},
			wantErr: "invalid location",
		},
		{
			name: "location part with non-empty source",
			modify: func(e *Envelope) {
				e.Parts = []Part{{
					Type:    Location,
					Content: `{"lat":0,"lon":0}`,
					Source:  "https://example.com/map",
				}}
			},
			wantErr: "location must not set source",
		},
		{
			name: "location part with non-empty mime type",
			modify: func(e *Envelope) {
				e.Parts = []Part{{
					Type:     Location,
					Content:  `{"lat":0,"lon":0}`,
					MIMEType: "application/geo+json",
				}}
			},
			wantErr: "location must not set mime",
		},
		{
			name: "callback part with empty content",
			modify: func(e *Envelope) {
				e.Parts = []Part{{Type: Callback, Content: ""}}
			},
			wantErr: "empty content",
		},
		{
			name: "callback part with non-empty source",
			modify: func(e *Envelope) {
				e.Parts = []Part{{Type: Callback, Content: "data", Source: "x"}}
			},
			wantErr: "callback must not set source",
		},
		{
			name: "callback part with non-empty mime type",
			modify: func(e *Envelope) {
				e.Parts = []Part{{Type: Callback, Content: "data", MIMEType: "text/plain"}}
			},
			wantErr: "callback must not set mime",
		},
		{
			name: "callback part mixed with text part",
			modify: func(e *Envelope) {
				e.Parts = []Part{
					{Type: Callback, Content: "yes"},
					{Type: Text, Content: "hola"},
				}
			},
			wantErr: "must be the only part",
		},
		{
			name: "two callback parts in the same envelope",
			modify: func(e *Envelope) {
				e.Parts = []Part{
					{Type: Callback, Content: "yes"},
					{Type: Callback, Content: "no"},
				}
			},
			wantErr: "must be the only part",
		},
		{
			name: "callback_ack part with non-empty source",
			modify: func(e *Envelope) {
				e.Parts = []Part{{Type: CallbackAck, Source: "x"}}
			},
			wantErr: "callback_ack must not set source",
		},
		{
			name: "callback_ack part with non-empty mime type",
			modify: func(e *Envelope) {
				e.Parts = []Part{{Type: CallbackAck, MIMEType: "text/plain"}}
			},
			wantErr: "callback_ack must not set mime",
		},
		{
			name: "callback_ack part mixed with text part",
			modify: func(e *Envelope) {
				e.Parts = []Part{
					{Type: CallbackAck, Content: "Saved!"},
					{Type: Text, Content: "extra"},
				}
			},
			wantErr: "must be the only part",
		},
		{
			name: "keyboard with no rows",
			modify: func(e *Envelope) {
				e.Keyboard = &Keyboard{Rows: nil}
			},
			wantErr: "keyboard has no rows",
		},
		{
			name: "keyboard with empty Rows slice",
			modify: func(e *Envelope) {
				e.Keyboard = &Keyboard{Rows: [][]Button{}}
			},
			wantErr: "keyboard has no rows",
		},
		{
			name: "keyboard row with no buttons",
			modify: func(e *Envelope) {
				e.Keyboard = &Keyboard{Rows: [][]Button{
					{CallbackButton("a", "a")},
					{},
				}}
			},
			wantErr: "row 1 has no buttons",
		},
		{
			name: "keyboard button with empty text",
			modify: func(e *Envelope) {
				e.Keyboard = &Keyboard{Rows: [][]Button{
					{Button{Text: "", CallbackData: "x"}},
				}}
			},
			wantErr: "button text is empty",
		},
		{
			name: "keyboard button with both CallbackData and URL",
			modify: func(e *Envelope) {
				e.Keyboard = &Keyboard{Rows: [][]Button{
					{Button{Text: "ambiguous", CallbackData: "x", URL: "https://example.com"}},
				}}
			},
			wantErr: "button must set exactly one of callback_data or url",
		},
		{
			name: "keyboard button with neither action",
			modify: func(e *Envelope) {
				e.Keyboard = &Keyboard{Rows: [][]Button{
					{Button{Text: "no-action"}},
				}}
			},
			wantErr: "button must set exactly one of callback_data or url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := base()
			tt.modify(env)
			err := env.Validate()
			if err == nil {
				t.Fatal("Validate() should return error")
			}
			if !containsSubstring(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestJSON_roundtrip(t *testing.T) {
	original := New("telegram", Inbound, Participant{ID: "user-1", Name: "Alice"})
	original.AddText("hello world")
	original.AddMedia(Image, "https://example.com/img.png", "image/png")
	original.Meta["conversation_id"] = "conv-42"

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}

	var decoded Envelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Channel != original.Channel {
		t.Errorf("Channel = %q, want %q", decoded.Channel, original.Channel)
	}
	if decoded.Direction != original.Direction {
		t.Errorf("Direction = %v, want %v", decoded.Direction, original.Direction)
	}
	if decoded.Sender.ID != original.Sender.ID {
		t.Errorf("Sender.ID = %q, want %q", decoded.Sender.ID, original.Sender.ID)
	}
	if decoded.Sender.Name != original.Sender.Name {
		t.Errorf("Sender.Name = %q, want %q", decoded.Sender.Name, original.Sender.Name)
	}
	if !decoded.Timestamp.Equal(original.Timestamp) {
		t.Errorf("Timestamp = %v, want %v", decoded.Timestamp, original.Timestamp)
	}
	if len(decoded.Parts) != len(original.Parts) {
		t.Fatalf("Parts len = %d, want %d", len(decoded.Parts), len(original.Parts))
	}
	for i, p := range decoded.Parts {
		op := original.Parts[i]
		if p.Type != op.Type {
			t.Errorf("Parts[%d].Type = %v, want %v", i, p.Type, op.Type)
		}
		if p.Content != op.Content {
			t.Errorf("Parts[%d].Content = %q, want %q", i, p.Content, op.Content)
		}
		if p.Source != op.Source {
			t.Errorf("Parts[%d].Source = %q, want %q", i, p.Source, op.Source)
		}
		if p.MIMEType != op.MIMEType {
			t.Errorf("Parts[%d].MIMEType = %q, want %q", i, p.MIMEType, op.MIMEType)
		}
	}
	if decoded.Meta["conversation_id"] != "conv-42" {
		t.Errorf("Meta[conversation_id] = %q, want %q", decoded.Meta["conversation_id"], "conv-42")
	}
}

func TestJSON_roundtrip_preserves_all_part_types(t *testing.T) {
	original := New("webhook", Outbound, Participant{ID: "bot"})
	original.AddText("text content")
	original.AddMedia(Image, "img.png", "image/png")
	original.AddMedia(Audio, "clip.mp3", "audio/mpeg")
	original.AddMedia(Video, "vid.mp4", "video/mp4")
	original.AddMedia(File, "doc.pdf", "application/pdf")
	original.AddLocation(41.40338, 2.17403)

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}

	var decoded Envelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}

	wantTypes := []PartType{Text, Image, Audio, Video, File, Location}
	if len(decoded.Parts) != len(wantTypes) {
		t.Fatalf("Parts len = %d, want %d", len(decoded.Parts), len(wantTypes))
	}
	for i, wt := range wantTypes {
		if decoded.Parts[i].Type != wt {
			t.Errorf("Parts[%d].Type = %v, want %v", i, decoded.Parts[i].Type, wt)
		}
	}
	lat, lon, ok := decoded.Parts[5].Location()
	if !ok {
		t.Fatalf("decoded location part not parseable: %q", decoded.Parts[5].Content)
	}
	if lat != 41.40338 || lon != 2.17403 {
		t.Errorf("decoded location = (%v, %v), want (41.40338, 2.17403)", lat, lon)
	}
}

func TestJSON_roundtrip_keyboard_field(t *testing.T) {
	original := New("telegram", Outbound, Participant{ID: "bot"})
	original.AddText("Choose:").WithKeyboard(
		[]Button{CallbackButton("Yes", "yes"), URLButton("Help", "https://example.com")},
		[]Button{CallbackButton("Cancel", "cancel")},
	)

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded Envelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.Keyboard == nil {
		t.Fatal("decoded.Keyboard is nil")
	}
	if len(decoded.Keyboard.Rows) != 2 {
		t.Fatalf("Rows len = %d, want 2", len(decoded.Keyboard.Rows))
	}
	if decoded.Keyboard.Rows[0][0].CallbackData != "yes" {
		t.Errorf("Rows[0][0].CallbackData = %q, want %q",
			decoded.Keyboard.Rows[0][0].CallbackData, "yes")
	}
	if decoded.Keyboard.Rows[0][1].URL != "https://example.com" {
		t.Errorf("Rows[0][1].URL = %q", decoded.Keyboard.Rows[0][1].URL)
	}
	if decoded.Keyboard.Rows[1][0].CallbackData != "cancel" {
		t.Errorf("Rows[1][0].CallbackData = %q, want %q",
			decoded.Keyboard.Rows[1][0].CallbackData, "cancel")
	}
}

func TestJSON_roundtrip_keyboard_absent_when_nil(t *testing.T) {
	// Envelopes without a keyboard must marshal to JSON that contains
	// no "keyboard" key, so the wire format of every 2.1-2E.3 envelope
	// stays byte-stable.
	original := New("telegram", Outbound, Participant{ID: "bot"})
	original.AddText("plain")

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if containsSubstring(string(data), `"keyboard"`) {
		t.Errorf("marshalled JSON contains keyboard key when Keyboard is nil: %s", data)
	}
}

// containsSubstring checks if s contains substr.
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

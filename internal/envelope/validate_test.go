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

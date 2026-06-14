// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import (
	"testing"

	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/go-telegram/bot/models"
)

// ----------------------------------------------------------------------------
// Phase 2E.3 — inbound location.
// One fixture covers a Telegram location_message with the full set of
// Bot API fields (latitude, longitude + companions). The Envelope must
// only carry the canonical {lat, lon} pair per ADR-0004; companion
// fields are intentionally NOT modelled until a future amending ADR.
// ----------------------------------------------------------------------------

func TestInboundFromUpdate_Location_FromFixture(t *testing.T) {
	u := loadUpdateFixture(t, "location_message.json")
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if len(env.Parts) != 1 {
		t.Fatalf("Parts len = %d, want 1", len(env.Parts))
	}
	got := env.Parts[0]
	if got.Type != envelope.Location {
		t.Errorf("Type = %v, want Location", got.Type)
	}
	if got.Source != "" {
		t.Errorf("Source = %q, want empty for Location", got.Source)
	}
	if got.MIMEType != "" {
		t.Errorf("MIMEType = %q, want empty for Location", got.MIMEType)
	}
	lat, lon, ok := got.Location()
	if !ok {
		t.Fatalf("Part.Location() ok = false, content = %q", got.Content)
	}
	if lat != 41.40338 || lon != 2.17403 {
		t.Errorf("Part.Location() = (%v, %v), want (41.40338, 2.17403)", lat, lon)
	}
}

func TestInboundFromUpdate_Location_OnlyCoordinates_NoCompanions(t *testing.T) {
	// Telegram lets a plain location be sent without the optional
	// companion fields. The Envelope must look identical to the
	// fully-populated case minus the irrelevant fields.
	u := &models.Update{
		Message: &models.Message{
			ID:   42,
			Date: 1786000400,
			From: &models.User{ID: 7, Username: "bob"},
			Chat: models.Chat{ID: 1000, Type: "private"},
			Location: &models.Location{
				Latitude:  -33.8688,
				Longitude: 151.2093,
			},
		},
	}
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if len(env.Parts) != 1 {
		t.Fatalf("Parts len = %d, want 1", len(env.Parts))
	}
	lat, lon, ok := env.Parts[0].Location()
	if !ok {
		t.Fatalf("Part.Location() ok = false, content = %q", env.Parts[0].Content)
	}
	if lat != -33.8688 || lon != 151.2093 {
		t.Errorf("Part.Location() = (%v, %v), want (-33.8688, 151.2093)", lat, lon)
	}
}

func TestInboundFromUpdate_Location_NullIsland_IsValid(t *testing.T) {
	// (0, 0) is a legitimate coordinate; the inbound mapping must
	// preserve it as a Location part, not drop it as zero-valued.
	u := &models.Update{
		Message: &models.Message{
			ID:   43,
			Date: 1786000500,
			From: &models.User{ID: 7, Username: "bob"},
			Chat: models.Chat{ID: 1000, Type: "private"},
			Location: &models.Location{
				Latitude:  0,
				Longitude: 0,
			},
		},
	}
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if len(env.Parts) != 1 || env.Parts[0].Type != envelope.Location {
		t.Fatalf("Parts = %+v, want one Location part", env.Parts)
	}
	lat, lon, ok := env.Parts[0].Location()
	if !ok || lat != 0 || lon != 0 {
		t.Errorf("Part.Location() = (%v, %v, %v), want (0, 0, true)", lat, lon, ok)
	}
}

func TestInboundFromUpdate_LocationEnvelope_PassesValidate(t *testing.T) {
	u := loadUpdateFixture(t, "location_message.json")
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if err := env.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

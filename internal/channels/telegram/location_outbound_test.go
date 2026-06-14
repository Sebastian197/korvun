// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import (
	"errors"
	"testing"

	"github.com/Sebastian197/korvun/internal/envelope"
)

// ----------------------------------------------------------------------------
// Phase 2E.3 — outbound location. Envelope -> *bot.SendLocationParams.
// SendLocationParams has no Caption field on the Telegram side, so a text
// part accompanying a Location is silently dropped (documented invariant);
// rejecting would punish callers that legitimately carry a text label
// alongside a location for other consumers.
// ----------------------------------------------------------------------------

func locationPart(lat, lon float64) envelope.Part {
	// Build a Location part via the canonical builder so the wire form
	// is always the one fixed by ADR-0004.
	tmp := envelope.New(ChannelName, envelope.Outbound, envelope.Participant{ID: "bot"})
	tmp.AddLocation(lat, lon)
	return tmp.Parts[0]
}

func TestOutboundParams_Location_HappyPath(t *testing.T) {
	e := mkOutboundEnv(locationPart(41.40338, 2.17403))
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindLocation {
		t.Errorf("Kind = %v, want OutboundKindLocation", out.Kind)
	}
	if out.Location == nil {
		t.Fatal("Location is nil")
	}
	assertChatID(t, out.Location.ChatID, 1000)
	if out.Location.Latitude != 41.40338 {
		t.Errorf("Latitude = %v, want 41.40338", out.Location.Latitude)
	}
	if out.Location.Longitude != 2.17403 {
		t.Errorf("Longitude = %v, want 2.17403", out.Location.Longitude)
	}
	// Sibling fields must remain nil so the Outbound is a clean tagged union.
	if out.Message != nil || out.Photo != nil || out.Document != nil ||
		out.Voice != nil || out.Audio != nil || out.Video != nil {
		t.Error("only Location should be populated")
	}
}

func TestOutboundParams_Location_NullIsland_IsValid(t *testing.T) {
	e := mkOutboundEnv(locationPart(0, 0))
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindLocation {
		t.Errorf("Kind = %v, want OutboundKindLocation", out.Kind)
	}
	if out.Location.Latitude != 0 || out.Location.Longitude != 0 {
		t.Errorf("(lat, lon) = (%v, %v), want (0, 0)", out.Location.Latitude, out.Location.Longitude)
	}
}

func TestOutboundParams_Location_NegativeCoordinates_PreservedExactly(t *testing.T) {
	e := mkOutboundEnv(locationPart(-33.8688, -151.2093))
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Location.Latitude != -33.8688 || out.Location.Longitude != -151.2093 {
		t.Errorf("(lat, lon) = (%v, %v), want (-33.8688, -151.2093)",
			out.Location.Latitude, out.Location.Longitude)
	}
}

func TestOutboundParams_Location_AccompanyingTextIsDropped(t *testing.T) {
	// SendLocationParams has no Caption field; the adapter silently drops
	// the text part rather than rejecting the envelope. This locks in the
	// behavior so a future change cannot regress it accidentally.
	e := mkOutboundEnv(locationPart(41.40338, 2.17403), textPart("etiqueta ignorada"))
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindLocation {
		t.Errorf("Kind = %v, want OutboundKindLocation", out.Kind)
	}
	if out.Location == nil {
		t.Fatal("Location is nil")
	}
	if out.Message != nil {
		t.Error("text part must not be promoted to a separate SendMessage")
	}
}

func TestOutboundParams_Location_WithMedia_TooMany(t *testing.T) {
	e := mkOutboundEnv(locationPart(0, 0), imagePart("photo_fid"))
	_, err := OutboundParams(e)
	if !errors.Is(err, ErrTooManyMediaParts) {
		t.Errorf("err = %v, want ErrTooManyMediaParts", err)
	}
}

func TestOutboundParams_Location_TwoLocations_TooMany(t *testing.T) {
	e := mkOutboundEnv(locationPart(0, 0), locationPart(1, 1))
	_, err := OutboundParams(e)
	if !errors.Is(err, ErrTooManyMediaParts) {
		t.Errorf("err = %v, want ErrTooManyMediaParts", err)
	}
}

func TestOutboundParams_Location_InvalidContent(t *testing.T) {
	// Hand-crafted broken Location part (bypassing the canonical builder)
	// to verify the outbound rejects it instead of constructing a
	// SendLocationParams with zeroed coordinates silently.
	e := mkOutboundEnv(envelope.Part{Type: envelope.Location, Content: "not-json"})
	_, err := OutboundParams(e)
	if !errors.Is(err, ErrInvalidLocation) {
		t.Errorf("err = %v, want ErrInvalidLocation", err)
	}
}

func TestOutboundKind_LocationString(t *testing.T) {
	if got := OutboundKindLocation.String(); got != "location" {
		t.Errorf("OutboundKindLocation.String() = %q, want %q", got, "location")
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package envelope defines the canonical message type used throughout Korvun.
// Every channel adapter converts its native format into an Envelope, and the
// rest of the system speaks only this language.
package envelope

import "time"

// Direction indicates whether a message is coming in from a channel or going
// out to one.
type Direction int

const (
	// Inbound represents a message received from an external channel.
	Inbound Direction = iota
	// Outbound represents a message sent to an external channel.
	Outbound
)

// String returns the human-readable name of the direction.
func (d Direction) String() string {
	switch d {
	case Inbound:
		return "inbound"
	case Outbound:
		return "outbound"
	default:
		return "unknown"
	}
}

// PartType classifies the content type of a message part.
type PartType int

const (
	// Text represents a plain-text message part.
	Text PartType = iota
	// Image represents an image attachment.
	Image
	// Audio represents an audio attachment.
	Audio
	// Video represents a video attachment.
	Video
	// File represents a generic file attachment.
	File
	// Location represents a geographic coordinate pair. The latitude and
	// longitude ride inside Content as a JSON object {"lat":..,"lon":..};
	// see ADR-0004. Callers should use Envelope.AddLocation to build a
	// Location part and Part.Location to read the coordinates back.
	Location
)

// String returns the human-readable name of the part type.
func (pt PartType) String() string {
	switch pt {
	case Text:
		return "text"
	case Image:
		return "image"
	case Audio:
		return "audio"
	case Video:
		return "video"
	case File:
		return "file"
	case Location:
		return "location"
	default:
		return "unknown"
	}
}

// Participant identifies a sender or recipient in a conversation.
type Participant struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// Part is a single content unit within an Envelope. An Envelope may contain
// multiple parts (e.g., text + image).
type Part struct {
	Type     PartType `json:"type"`
	Content  string   `json:"content,omitempty"`
	Source   string   `json:"source,omitempty"`
	MIMEType string   `json:"mime_type,omitempty"`
}

// Envelope is the canonical, channel-agnostic message representation.
// All components in Korvun communicate exclusively through Envelopes.
type Envelope struct {
	ID        string            `json:"id"`
	Channel   string            `json:"channel"`
	Direction Direction         `json:"direction"`
	Sender    Participant       `json:"sender"`
	Parts     []Part            `json:"parts"`
	Timestamp time.Time         `json:"timestamp"`
	Meta      map[string]string `json:"meta,omitempty"`
}

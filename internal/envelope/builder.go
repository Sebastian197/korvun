// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package envelope

import "time"

// New creates an Envelope with a generated ID, current timestamp, and an
// initialized Meta map. Parts start empty; use AddText / AddMedia to populate.
func New(channel string, dir Direction, sender Participant) *Envelope {
	return &Envelope{
		ID:        NewID(),
		Channel:   channel,
		Direction: dir,
		Sender:    sender,
		Parts:     []Part{},
		Timestamp: time.Now(),
		Meta:      make(map[string]string),
	}
}

// AddText appends a text part to the envelope. Returns the envelope for
// method chaining.
func (e *Envelope) AddText(content string) *Envelope {
	e.Parts = append(e.Parts, Part{
		Type:    Text,
		Content: content,
	})
	return e
}

// AddMedia appends a media part (image, audio, video, file) to the envelope.
// Returns the envelope for method chaining.
func (e *Envelope) AddMedia(pt PartType, source, mimeType string) *Envelope {
	e.Parts = append(e.Parts, Part{
		Type:     pt,
		Source:   source,
		MIMEType: mimeType,
	})
	return e
}

// AddLocation appends a Location part to the envelope, encoding the
// coordinate pair in the canonical wire form fixed by ADR-0004. Returns
// the envelope for method chaining.
func (e *Envelope) AddLocation(lat, lon float64) *Envelope {
	e.Parts = append(e.Parts, Part{
		Type:    Location,
		Content: marshalLocation(lat, lon),
	})
	return e
}

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

// AddCallback appends a Callback part to the envelope. The data string
// is the channel's callback payload (e.g. the callback_data of the
// inline-keyboard button the user tapped). See ADR-0005.
func (e *Envelope) AddCallback(data string) *Envelope {
	e.Parts = append(e.Parts, Part{
		Type:    Callback,
		Content: data,
	})
	return e
}

// AddCallbackAck appends a CallbackAck part to the envelope. The toast
// string is shown to the user as a small notification; empty toast
// produces a silent ack that only clears the channel's retry. The
// channel-specific identifier of the callback being acknowledged must
// already be present in Meta (e.g. "telegram.callback_query_id"). See
// ADR-0005.
func (e *Envelope) AddCallbackAck(toast string) *Envelope {
	e.Parts = append(e.Parts, Part{
		Type:    CallbackAck,
		Content: toast,
	})
	return e
}

// WithKeyboard attaches an interactive overlay to the envelope. Each
// variadic argument is one row of buttons; the rows are stacked
// top-to-bottom in the resulting UI. Validate enforces structural
// rules on the keyboard contents. Returns the envelope for method
// chaining.
func (e *Envelope) WithKeyboard(rows ...[]Button) *Envelope {
	e.Keyboard = &Keyboard{Rows: rows}
	return e
}

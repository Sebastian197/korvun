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

// WithKeyboard attaches an interactive overlay to the envelope. Each
// variadic argument is one row of buttons; the rows are stacked
// top-to-bottom in the resulting UI. Validate enforces structural
// rules on the keyboard contents. Returns the envelope for method
// chaining.
func (e *Envelope) WithKeyboard(rows ...[]Button) *Envelope {
	e.Keyboard = &Keyboard{Rows: rows}
	return e
}

// SetEditText marks the envelope as an OpEditText operation and stores
// the new body text as a single Text Part. The channel-specific target
// identifier (e.g. telegram.chat_id + telegram.message_id) must
// already be present in Meta when OutboundParams is called. Replaces
// any pre-existing Parts and Operation. See ADR-0006.
func (e *Envelope) SetEditText(text string) *Envelope {
	e.Operation = &Operation{Kind: OpEditText}
	e.Parts = []Part{{Type: Text, Content: text}}
	return e
}

// SetEditCaption marks the envelope as an OpEditCaption operation and
// stores the new caption as a single Text Part. Empty caption clears
// the existing caption on the target message. Replaces any
// pre-existing Parts and Operation. See ADR-0006.
func (e *Envelope) SetEditCaption(caption string) *Envelope {
	e.Operation = &Operation{Kind: OpEditCaption}
	e.Parts = []Part{{Type: Text, Content: caption}}
	return e
}

// SetDelete marks the envelope as an OpDelete operation. Parts is set
// to empty (delete has no body); any pre-existing Parts and Operation
// are replaced. The Keyboard, if any, must be cleared by the caller —
// Validate rejects an OpDelete envelope that carries a Keyboard. See
// ADR-0006.
func (e *Envelope) SetDelete() *Envelope {
	e.Operation = &Operation{Kind: OpDelete}
	e.Parts = []Part{}
	return e
}

// SetCallbackAck marks the envelope as an OpCallbackAck operation.
// Empty toast produces a silent ack (empty Parts); a non-empty toast
// is stored as a single Text Part. The channel-specific callback
// identifier must already be present in Meta (e.g.
// telegram.callback_query_id). Replaces any pre-existing Parts and
// Operation. See ADR-0006.
func (e *Envelope) SetCallbackAck(toast string) *Envelope {
	e.Operation = &Operation{Kind: OpCallbackAck}
	if toast == "" {
		e.Parts = []Part{}
	} else {
		e.Parts = []Part{{Type: Text, Content: toast}}
	}
	return e
}

// SetReactions marks the envelope as an OpSetReaction operation that
// sets the bot's reactions on the target message to the given emojis,
// one per variadic argument. Passing no arguments clears all of the
// bot's existing reactions on the target. Replaces any pre-existing
// Parts and Operation. See ADR-0007.
//
// Each emoji is stored as its own Text Part (so multi-emoji reactions
// round-trip cleanly through the canonical envelope shape). The
// channel-specific target identifier (e.g. telegram.chat_id +
// telegram.message_id) must already be present in Meta when
// OutboundParams is called.
func (e *Envelope) SetReactions(emojis ...string) *Envelope {
	e.Operation = &Operation{Kind: OpSetReaction}
	parts := make([]Part, len(emojis))
	for i, em := range emojis {
		parts[i] = Part{Type: Text, Content: em}
	}
	e.Parts = parts
	return e
}

// AddReaction appends a single Reaction part to the envelope. Unlike
// the other Part builders, AddReaction is intended for inbound use:
// when an adapter materialises a user-initiated reaction event, it
// calls AddReaction once per emoji the user holds on the target
// message. See ADR-0007.
func (e *Envelope) AddReaction(emoji string) *Envelope {
	e.Parts = append(e.Parts, Part{
		Type:    Reaction,
		Content: emoji,
	})
	return e
}

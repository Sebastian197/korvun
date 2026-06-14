// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package envelope

import (
	"errors"
	"fmt"
)

// Validate checks that the Envelope contains all required fields and that
// each part is well-formed. Returns the first validation error found.
func (e *Envelope) Validate() error {
	if e.ID == "" {
		return errors.New("empty ID")
	}
	if e.Channel == "" {
		return errors.New("empty channel")
	}
	if e.Direction != Inbound && e.Direction != Outbound {
		return fmt.Errorf("invalid direction: %d", e.Direction)
	}
	if e.Sender.ID == "" {
		return errors.New("empty sender ID")
	}
	if e.Timestamp.IsZero() {
		return errors.New("zero timestamp")
	}
	if e.Operation != nil {
		if err := validateOperation(e); err != nil {
			return err
		}
	} else {
		if len(e.Parts) == 0 {
			return errors.New("no parts")
		}
		if err := validateExclusivePartTypes(e.Parts); err != nil {
			return err
		}
		for i, p := range e.Parts {
			if err := validatePart(i, p); err != nil {
				return err
			}
		}
	}
	if e.Keyboard != nil {
		if err := validateKeyboard(e.Keyboard); err != nil {
			return err
		}
	}
	return nil
}

// validateExclusivePartTypes enforces two distinct exclusivity rules:
//
//   - Callback parts are *singletons*: when a Callback Part appears, it
//     must be the only Part in the envelope. A tap event has no media
//     or text body beyond its data string.
//   - Reaction parts are *uniform-typed*: when a Reaction Part appears,
//     every other Part must also be a Reaction Part. A reaction event
//     may carry multiple emojis (Premium accounts), so the rule is "all
//     same kind" rather than "exactly one".
//
// The Callback check runs first so a Reaction + Callback mix is
// rejected for the same Callback-singleton reason it would be rejected
// without a Reaction present. CallbackAck used to be in this list but
// ADR-0006 moved it out of Parts entirely (its exclusivity is enforced
// at the Operation level now).
func validateExclusivePartTypes(parts []Part) error {
	if len(parts) <= 1 {
		return nil
	}
	for _, p := range parts {
		if p.Type == Callback {
			return fmt.Errorf("%s part must be the only part in the envelope", p.Type)
		}
	}
	// Reaction-uniformity rule. Walk once: any non-Reaction Part
	// alongside a Reaction Part is a coexistence violation.
	hasReaction := false
	hasNonReaction := false
	for _, p := range parts {
		if p.Type == Reaction {
			hasReaction = true
		} else {
			hasNonReaction = true
		}
	}
	if hasReaction && hasNonReaction {
		return errors.New("reaction parts must not coexist with other part types")
	}
	return nil
}

// validateOperation enforces the per-OperationKind contract documented
// in ADR-0006 §1. Each kind specifies a strict Parts shape and a
// Keyboard policy; this function rejects any envelope that deviates.
func validateOperation(e *Envelope) error {
	op := e.Operation
	switch op.Kind {
	case OpEditText:
		if len(e.Parts) != 1 {
			return fmt.Errorf("OpEditText requires exactly 1 part, got %d", len(e.Parts))
		}
		p := e.Parts[0]
		if p.Type != Text {
			return fmt.Errorf("OpEditText part must be Text, got %s", p.Type)
		}
		if p.Content == "" {
			return errors.New("OpEditText part must have non-empty Content")
		}
		if p.Source != "" || p.MIMEType != "" {
			return errors.New("OpEditText part must not set Source/MIMEType")
		}
	case OpEditCaption:
		if len(e.Parts) != 1 {
			return fmt.Errorf("OpEditCaption requires exactly 1 part, got %d", len(e.Parts))
		}
		p := e.Parts[0]
		if p.Type != Text {
			return fmt.Errorf("OpEditCaption part must be Text, got %s", p.Type)
		}
		if p.Source != "" || p.MIMEType != "" {
			return errors.New("OpEditCaption part must not set Source/MIMEType")
		}
	case OpDelete:
		if len(e.Parts) != 0 {
			return fmt.Errorf("OpDelete requires empty parts, got %d", len(e.Parts))
		}
		if e.Keyboard != nil {
			return errors.New("OpDelete must not carry a Keyboard")
		}
	case OpCallbackAck:
		if len(e.Parts) > 1 {
			return fmt.Errorf("OpCallbackAck requires 0 or 1 parts, got %d", len(e.Parts))
		}
		if len(e.Parts) == 1 {
			p := e.Parts[0]
			if p.Type != Text {
				return fmt.Errorf("OpCallbackAck part must be Text, got %s", p.Type)
			}
			if p.Content == "" {
				return errors.New("OpCallbackAck part must have non-empty Content (use empty Parts for silent ack)")
			}
			if p.Source != "" || p.MIMEType != "" {
				return errors.New("OpCallbackAck part must not set Source/MIMEType")
			}
		}
		if e.Keyboard != nil {
			return errors.New("OpCallbackAck must not carry a Keyboard")
		}
	case OpSetReaction:
		// Parts may be empty (clear all the bot's reactions on the
		// target) or 1+ Text Parts (each Content is one emoji).
		for i, p := range e.Parts {
			if p.Type != Text {
				return fmt.Errorf("OpSetReaction part %d must be Text, got %s", i, p.Type)
			}
			if p.Content == "" {
				return fmt.Errorf("OpSetReaction part %d must have non-empty Content", i)
			}
			if p.Source != "" || p.MIMEType != "" {
				return fmt.Errorf("OpSetReaction part %d must not set Source/MIMEType", i)
			}
		}
		if e.Keyboard != nil {
			return errors.New("OpSetReaction must not carry a Keyboard")
		}
	default:
		return fmt.Errorf("unknown OperationKind: %d", op.Kind)
	}
	return nil
}

func validatePart(idx int, p Part) error {
	switch p.Type {
	case Text:
		if p.Content == "" {
			return fmt.Errorf("part %d: empty content for text part", idx)
		}
	case Image, Audio, Video, File:
		if p.Source == "" {
			return fmt.Errorf("part %d: empty source for %s part", idx, p.Type)
		}
	case Location:
		if p.Content == "" {
			return fmt.Errorf("part %d: empty content for location part", idx)
		}
		if p.Source != "" {
			return fmt.Errorf("part %d: location must not set source", idx)
		}
		if p.MIMEType != "" {
			return fmt.Errorf("part %d: location must not set mime type", idx)
		}
		if _, _, ok := p.Location(); !ok {
			return fmt.Errorf("part %d: invalid location content: %q", idx, p.Content)
		}
	case Callback:
		if p.Content == "" {
			return fmt.Errorf("part %d: empty content for callback part", idx)
		}
		if p.Source != "" {
			return fmt.Errorf("part %d: callback must not set source", idx)
		}
		if p.MIMEType != "" {
			return fmt.Errorf("part %d: callback must not set mime type", idx)
		}
	case Reaction:
		if p.Content == "" {
			return fmt.Errorf("part %d: empty content for reaction part", idx)
		}
		if p.Source != "" {
			return fmt.Errorf("part %d: reaction must not set source", idx)
		}
		if p.MIMEType != "" {
			return fmt.Errorf("part %d: reaction must not set mime type", idx)
		}
	default:
		return fmt.Errorf("part %d: unknown part type %d", idx, p.Type)
	}
	return nil
}

// validateKeyboard checks structural invariants of an attached
// Keyboard: at least one row, every row non-empty, every button with
// non-empty Text and exactly one of CallbackData / URL set.
func validateKeyboard(k *Keyboard) error {
	if len(k.Rows) == 0 {
		return errors.New("keyboard has no rows")
	}
	for ri, row := range k.Rows {
		if len(row) == 0 {
			return fmt.Errorf("keyboard row %d has no buttons", ri)
		}
		for bi, b := range row {
			if b.Text == "" {
				return fmt.Errorf("keyboard row %d button %d: button text is empty", ri, bi)
			}
			hasCallback := b.CallbackData != ""
			hasURL := b.URL != ""
			if hasCallback == hasURL {
				return fmt.Errorf("keyboard row %d button %d: button must set exactly one of callback_data or url", ri, bi)
			}
		}
	}
	return nil
}

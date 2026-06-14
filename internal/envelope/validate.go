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
	if len(e.Parts) == 0 {
		return errors.New("no parts")
	}
	if e.Timestamp.IsZero() {
		return errors.New("zero timestamp")
	}
	if err := validateExclusivePartTypes(e.Parts); err != nil {
		return err
	}
	for i, p := range e.Parts {
		if err := validatePart(i, p); err != nil {
			return err
		}
	}
	if e.Keyboard != nil {
		if err := validateKeyboard(e.Keyboard); err != nil {
			return err
		}
	}
	return nil
}

// validateExclusivePartTypes enforces that Callback and CallbackAck
// parts must be the only Part in the envelope when they appear. A tap
// event has no media or text body beyond its data string, and an ack
// is a single instruction to the channel — both refuse to share the
// Parts slice with anything else.
func validateExclusivePartTypes(parts []Part) error {
	if len(parts) <= 1 {
		return nil
	}
	for _, p := range parts {
		if p.Type == Callback || p.Type == CallbackAck {
			return fmt.Errorf("%s part must be the only part in the envelope", p.Type)
		}
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
	case CallbackAck:
		// Content is optional: empty Content means a silent ack.
		if p.Source != "" {
			return fmt.Errorf("part %d: callback_ack must not set source", idx)
		}
		if p.MIMEType != "" {
			return fmt.Errorf("part %d: callback_ack must not set mime type", idx)
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

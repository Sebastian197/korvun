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
	for i, p := range e.Parts {
		if err := validatePart(i, p); err != nil {
			return err
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
	default:
		return fmt.Errorf("part %d: unknown part type %d", idx, p.Type)
	}
	return nil
}

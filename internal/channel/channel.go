// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package channel defines the interface that every messaging adapter must
// implement, along with a registry for managing active channels.
package channel

import (
	"context"

	"github.com/Sebastian197/korvun/internal/envelope"
)

// Channel is the contract for a messaging adapter. Each adapter converts
// between its native protocol and the canonical Envelope type.
type Channel interface {
	// Name returns the unique identifier of this channel (e.g. "telegram").
	Name() string

	// Manifest describes the capabilities supported by this channel.
	Manifest() Manifest

	// Send delivers an outbound Envelope through the channel.
	Send(ctx context.Context, env *envelope.Envelope) error

	// Receive returns a read-only channel that emits inbound Envelopes.
	// The caller must respect context cancellation.
	Receive(ctx context.Context) (<-chan *envelope.Envelope, error)
}

// Manifest declares which content types a channel supports.
type Manifest struct {
	Text    bool `json:"text"`
	Image   bool `json:"image"`
	Audio   bool `json:"audio"`
	Video   bool `json:"video"`
	Buttons bool `json:"buttons"`
}

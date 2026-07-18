// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package discord

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/Sebastian197/korvun/internal/channel"
	"github.com/Sebastian197/korvun/internal/envelope"
)

// Adapter is the Discord channel adapter; it implements channel.Channel. In
// sub-phase 1 it is the skeleton: New validates the config and the env-only token
// (ADR-0010), and the Gateway (Receive) and REST (Send) paths are explicit stubs.
// The inbound channel and the resolved token value are added in the sub-phases that
// first need them (SP3), keeping the secret out of a struct field until then.
type Adapter struct {
	cfg     *config
	dropped atomic.Uint64
}

// New constructs a Discord adapter. It validates the structural options (a
// token_env name is present and the mode is gateway), then resolves the bot token
// SOLELY from the environment variable named by WithTokenEnv (ADR-0010): an unset
// variable is a loud error naming the variable — never its value. The value is read
// from the environment again at connect time (SP3), so it is never stored on the
// adapter where it could be logged.
func New(opts ...Option) (*Adapter, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.tokenEnv == "" {
		return nil, ErrMissingTokenEnv
	}
	if cfg.mode != ModeGateway {
		return nil, ErrInvalidMode
	}
	if os.Getenv(cfg.tokenEnv) == "" {
		return nil, fmt.Errorf("%w: %q (discord bot token)", ErrMissingToken, cfg.tokenEnv)
	}

	return &Adapter{cfg: cfg}, nil
}

// Name returns the channel identifier ("discord").
func (a *Adapter) Name() string { return ChannelName }

// Manifest reports the content kinds this adapter supports. v1 is text-only —
// attachments/media are out of v1 scope (ADR-0033 §8).
func (a *Adapter) Manifest() channel.Manifest {
	return channel.Manifest{Text: true}
}

// Mode reports the configured transport mode (always gateway in v1).
func (a *Adapter) Mode() Mode { return a.cfg.mode }

// DroppedCount returns the cumulative number of inbound Envelopes dropped at the
// channel edge. Zero until the Gateway receive path lands (SP2/SP3).
func (a *Adapter) DroppedCount() uint64 { return a.dropped.Load() }

// Send delivers an outbound Envelope via the Discord REST API. Sub-phase 1 is an
// explicit stub — the REST createMessage path (with 429/Retry-After handling) lands
// in sub-phase 5; it is never a silent no-op.
func (a *Adapter) Send(_ context.Context, _ *envelope.Envelope) error {
	return ErrSendNotImplemented
}

// Receive would return the inbound Envelope channel fed by the Gateway. Sub-phase 1
// has no Gateway yet: connectGateway is the SP3/SP4 seam and today returns an
// explicit error, so Receive fails honestly rather than handing back a dead channel.
func (a *Adapter) Receive(ctx context.Context) (<-chan *envelope.Envelope, error) {
	_, err := a.connectGateway(ctx)
	return nil, err
}

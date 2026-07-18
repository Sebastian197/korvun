// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package discord

import (
	"log/slog"
	"math/rand/v2"
)

// ChannelName is the unique identifier of the Discord channel (the value of
// Name(), and the config `type`).
const ChannelName = "discord"

// defaultGatewayURL is the documented Discord Gateway WebSocket endpoint (API v10,
// JSON encoding). SP3 dials it directly; resolving it dynamically via GET
// /gateway/bot is a later refinement. Tests override it with a fake server URL.
const defaultGatewayURL = "wss://gateway.discord.gg/?v=10&encoding=json"

// defaultInboundCapacity is the buffered depth of the inbound Envelope channel. When
// full, further inbound messages are dropped at the edge and counted (backpressure).
const defaultInboundCapacity = 256

// Mode is the Discord transport mode. Only ModeGateway exists: the Gateway
// WebSocket is the sole way to receive messages (ADR-0033); REST send is not a
// separate mode.
type Mode string

// ModeGateway is the Gateway WebSocket receive mode.
const ModeGateway Mode = "gateway"

// config carries the resolved options after every Option has run. It holds only the
// env-var NAME, never the token VALUE (ADR-0010).
type config struct {
	tokenEnv        string
	mode            Mode
	gatewayURL      string
	inboundCapacity int
	logger          *slog.Logger
	// jitterFrac returns the fraction of the heartbeat interval to wait before the
	// FIRST heartbeat (Discord's recommended startup jitter). It returns a value in
	// [0,1); tests inject a deterministic 0.
	jitterFrac func() float64
}

func defaultConfig() *config {
	return &config{
		mode:            ModeGateway,
		gatewayURL:      defaultGatewayURL,
		inboundCapacity: defaultInboundCapacity,
		logger:          slog.Default(),
		jitterFrac:      randJitterFrac,
	}
}

// randJitterFrac returns a random fraction in [0,1) for the heartbeat startup jitter
// (Discord's recommended spread). The value is timing jitter, never a secret, so a
// non-cryptographic RNG is appropriate.
func randJitterFrac() float64 {
	return rand.Float64() // #nosec G404 -- heartbeat startup jitter, not security-sensitive
}

// Option configures the Adapter at construction time.
type Option func(*config)

// WithTokenEnv sets the NAME of the environment variable that holds the bot token
// (never the token value — ADR-0010). Required.
func WithTokenEnv(name string) Option { return func(c *config) { c.tokenEnv = name } }

// WithMode sets the transport mode. Only ModeGateway is supported.
func WithMode(m Mode) Option { return func(c *config) { c.mode = m } }

// WithLogger sets the structured logger the adapter uses for edge drops and
// lifecycle events. Defaults to slog.Default().
func WithLogger(l *slog.Logger) Option {
	return func(c *config) {
		if l != nil {
			c.logger = l
		}
	}
}

// withGatewayURLForTests overrides the Gateway URL so tests can dial a fake server
// instead of the real Discord endpoint. Unexported: not part of the public API.
func withGatewayURLForTests(u string) Option { return func(c *config) { c.gatewayURL = u } }

// withInboundCapacityForTests overrides the inbound buffer depth so tests can force
// backpressure deterministically. Unexported: not part of the public API.
func withInboundCapacityForTests(n int) Option {
	return func(c *config) {
		if n > 0 {
			c.inboundCapacity = n
		}
	}
}

// withJitterFracForTests pins the heartbeat startup jitter (normally random) so
// tests get deterministic timing. Unexported: not part of the public API.
func withJitterFracForTests(f func() float64) Option {
	return func(c *config) {
		if f != nil {
			c.jitterFrac = f
		}
	}
}

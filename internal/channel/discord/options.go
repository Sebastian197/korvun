// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package discord

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"time"
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

// gwClock abstracts the reconnect backoff sleep so tests never wait in wall-clock
// time. Sleep MUST honor ctx: a cancel during the wait returns promptly with the ctx
// error (mirrors the retry decorator's Clock seam, ADR-0031).
type gwClock interface {
	Sleep(ctx context.Context, d time.Duration) error
}

// realGWClock is the default time-backed clock.
type realGWClock struct{}

func (realGWClock) Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// config carries the resolved options after every Option has run. It holds only the
// env-var NAME, never the token VALUE (ADR-0010).
type config struct {
	tokenEnv        string
	mode            Mode
	gatewayURL      string
	inboundCapacity int
	logger          *slog.Logger
	clock           gwClock
	// rnd returns a fraction in [0,1); it feeds BOTH the heartbeat startup jitter
	// and the full-jitter reconnect backoff. Tests inject a deterministic value.
	rnd func() float64
}

func defaultConfig() *config {
	return &config{
		mode:            ModeGateway,
		gatewayURL:      defaultGatewayURL,
		inboundCapacity: defaultInboundCapacity,
		logger:          slog.Default(),
		clock:           realGWClock{},
		rnd:             randFrac,
	}
}

// randFrac returns a random fraction in [0,1) for jitter (heartbeat startup + backoff).
// The value is timing jitter, never a secret, so a non-cryptographic RNG is fine.
func randFrac() float64 {
	return rand.Float64() // #nosec G404 -- timing jitter, not security-sensitive
}

// Option configures the Adapter at construction time.
type Option func(*config)

// WithTokenEnv sets the NAME of the environment variable that holds the bot token
// (never the token value — ADR-0010). Required.
func WithTokenEnv(name string) Option { return func(c *config) { c.tokenEnv = name } }

// WithMode sets the transport mode. Only ModeGateway is supported.
func WithMode(m Mode) Option { return func(c *config) { c.mode = m } }

// WithLogger sets the structured logger the adapter uses for edge drops and
// lifecycle/reconnect events. Defaults to slog.Default().
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

// withRandForTests pins the jitter source (heartbeat startup + backoff) so tests get
// deterministic timing. Unexported: not part of the public API.
func withRandForTests(f func() float64) Option {
	return func(c *config) {
		if f != nil {
			c.rnd = f
		}
	}
}

// withClockForTests injects the backoff clock so tests never sleep in wall-clock
// time. Unexported: not part of the public API.
func withClockForTests(c gwClock) Option {
	return func(cfg *config) {
		if c != nil {
			cfg.clock = c
		}
	}
}

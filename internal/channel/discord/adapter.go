// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package discord

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/Sebastian197/korvun/internal/channel"
	"github.com/Sebastian197/korvun/internal/envelope"
)

// Adapter is the Discord channel adapter; it implements channel.Channel. New
// validates the config and the env-only token name (ADR-0010). Receive runs the
// Gateway state machine (SP3); Send (REST) is the SP5 stub. The bot token value is
// never stored on the Adapter — it is read from the environment at connect time.
type Adapter struct {
	cfg     *config
	dropped atomic.Uint64

	// inbound carries mapped Envelopes from the Gateway read loop to the router. It
	// is created per Receive and closed once every gateway goroutine has returned.
	inbound chan *envelope.Envelope

	// termMu guards termErr, the terminal cause of the last Gateway session (zombie,
	// op7/op9, a read failure, or nil for a clean ctx-cancel shutdown). It is set by
	// run before it closes inbound, so a reader that observes inbound closed can read
	// it safely.
	termMu  sync.Mutex
	termErr error

	// ready records the session id + resume URL captured from the READY event. SP3
	// only stores them; SP4 uses them to resume a dropped connection.
	ready atomic.Pointer[readySession]

	// started guards against a second Receive racing (and double-closing) the inbound
	// channel of a session that is already running.
	started atomic.Bool
}

// readySession is the reconnect-relevant subset of a READY event (ADR-0033 §3).
type readySession struct {
	id        string
	resumeURL string
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
// channel edge — mapper drops (self/bot/webhook/malformed/…) plus backpressure
// drops (inbound buffer saturated).
func (a *Adapter) DroppedCount() uint64 { return a.dropped.Load() }

// setTermErr records the terminal cause of the Gateway session. Called by run
// before it closes the inbound channel.
func (a *Adapter) setTermErr(err error) {
	a.termMu.Lock()
	a.termErr = err
	a.termMu.Unlock()
}

// terminalErr returns the terminal cause of the last Gateway session (nil for a
// clean shutdown). Used by tests; SP4 reads it to drive resume/reconnect.
func (a *Adapter) terminalErr() error {
	a.termMu.Lock()
	defer a.termMu.Unlock()
	return a.termErr
}

// readyInfo returns the session id + resume URL captured from the READY event, or
// nil before READY. Recorded for SP4's resume path.
func (a *Adapter) readyInfo() *readySession { return a.ready.Load() }

// Send delivers an outbound Envelope via the Discord REST API. Sub-phase 1 is an
// explicit stub — the REST createMessage path (with 429/Retry-After handling) lands
// in sub-phase 5; it is never a silent no-op.
func (a *Adapter) Send(_ context.Context, _ *envelope.Envelope) error {
	return ErrSendNotImplemented
}

// Receive opens the Gateway WebSocket and returns the inbound Envelope channel the
// router consumes. It dials synchronously so a connection failure is an honest error
// to the caller; the handshake (Hello → Identify → Ready) and the read/heartbeat
// loops then run in background goroutines bound to ctx. Cancelling ctx closes the
// WebSocket and joins every gateway goroutine, after which the returned channel is
// closed — a clean end-of-stream the router can drain. Call Receive once per Adapter
// (SP4 adds resume/reconnect on top of this base lifecycle).
func (a *Adapter) Receive(ctx context.Context) (<-chan *envelope.Envelope, error) {
	if !a.started.CompareAndSwap(false, true) {
		return nil, ErrAlreadyReceiving
	}
	conn, err := a.dial(ctx)
	if err != nil {
		a.started.Store(false) // dial failed: no session started, allow a retry
		return nil, err
	}
	a.inbound = make(chan *envelope.Envelope, a.cfg.inboundCapacity)
	go a.run(ctx, conn)
	return a.inbound, nil
}

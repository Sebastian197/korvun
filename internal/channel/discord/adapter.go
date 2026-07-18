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

// Adapter is the Discord channel adapter; it implements the app's Channel contract
// (channel.Channel + Start/Stop). New validates the config and the env-only token name
// (ADR-0010) and creates the inbound channel; Start launches the Gateway reconnect
// SUPERVISOR (SP4), Stop tears it down (parity with the Telegram adapter's lifecycle,
// ADR-0008). Send (REST, SP5) is the outbound path. The bot token value is never
// stored on the Adapter — it is read from the environment at each connect.
type Adapter struct {
	cfg        *config
	dropped    atomic.Uint64
	reconnects atomic.Uint64

	// inbound carries mapped Envelopes to the router. It is OWNED BY THE SUPERVISOR
	// (not by any single session): it survives every reconnect and is closed exactly
	// once, when the supervisor stops (Stop, or a fatal cause). The router therefore
	// never sees an end-of-stream because of a reconnect.
	inbound chan *envelope.Envelope

	// termMu guards termErr, the terminal cause the supervisor stopped on (a fatal
	// close code / missing token, or nil for a clean Stop). It is set before inbound is
	// closed, so a reader that observes inbound closed can read it.
	termMu  sync.Mutex
	termErr error

	// ready records the session id + resume URL from the most recent READY event.
	ready atomic.Pointer[readySession]

	// Lifecycle, guarded by mu (parity with the Telegram adapter): started admits a
	// single Start; loopCancel stops the supervisor; supDone closes once the supervisor
	// goroutine has returned (after it closes inbound). stopOnce makes Stop idempotent.
	// The mutex publishes loopCancel/supDone before started is observable, so a
	// concurrent Stop never sees started==true with a nil cancel/channel.
	mu         sync.Mutex
	started    bool
	loopCancel context.CancelFunc
	supDone    chan struct{}
	stopOnce   sync.Once
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

	return &Adapter{
		cfg:     cfg,
		inbound: make(chan *envelope.Envelope, cfg.inboundCapacity),
	}, nil
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

// ReconnectCount returns the cumulative number of Gateway reconnect attempts (dial
// failures plus resume/re-identify after a dropped session). Exposed like
// DroppedCount for observability; SP6 decides whether it becomes a Prometheus series.
func (a *Adapter) ReconnectCount() uint64 { return a.reconnects.Load() }

// setTermErr records the terminal cause the supervisor stopped on. Called before the
// inbound channel is closed.
func (a *Adapter) setTermErr(err error) {
	a.termMu.Lock()
	a.termErr = err
	a.termMu.Unlock()
}

// terminalErr returns the terminal cause the supervisor stopped on (nil for a clean
// ctx-cancel shutdown, a fatal cause otherwise). Used by tests.
func (a *Adapter) terminalErr() error {
	a.termMu.Lock()
	defer a.termMu.Unlock()
	return a.termErr
}

// readyInfo returns the session id + resume URL from the most recent READY event, or
// nil before the first READY. Used by tests.
func (a *Adapter) readyInfo() *readySession { return a.ready.Load() }

// Receive returns the inbound Envelope channel the router pump consumes. The channel
// exists from construction (parity with the Telegram adapter); the supervisor that
// feeds it is started by Start, not here. The supplied ctx is not bound — Start/Stop
// own the lifecycle. The same channel is returned on every call.
func (a *Adapter) Receive(_ context.Context) (<-chan *envelope.Envelope, error) {
	return a.inbound, nil
}

// Start launches the Gateway reconnect supervisor in the background. It does NOT fail
// on a connectivity error: the channel is availability, so the supervisor keeps
// (re)connecting with exponential backoff for as long as it runs — dialing,
// identifying, resuming after a drop, re-identifying when a resume is rejected —
// logging each attempt. The supervisor runs under its own context (not the Start ctx),
// cancelled by Stop; the inbound channel is closed exactly once, when the supervisor
// stops (Stop or a fatal, non-recoverable cause), never on a reconnect. A second Start
// returns ErrAlreadyStarted.
func (a *Adapter) Start(_ context.Context) error {
	a.mu.Lock()
	if a.started {
		a.mu.Unlock()
		return ErrAlreadyStarted
	}
	a.started = true
	loopCtx, cancel := context.WithCancel(context.Background())
	a.loopCancel = cancel
	a.supDone = make(chan struct{})
	done := a.supDone
	a.mu.Unlock()

	go func() {
		defer close(done)
		a.supervise(loopCtx)
	}()
	return nil
}

// Stop tears the supervisor down: it cancels the supervisor's context, then waits
// (bounded by ctx) for the supervisor goroutine to return, which closes the inbound
// channel so the router's pump drains and exits. Stop is idempotent and a no-op if
// Start was never called.
func (a *Adapter) Stop(ctx context.Context) error {
	a.mu.Lock()
	started := a.started
	cancel := a.loopCancel
	done := a.supDone
	a.mu.Unlock()
	if !started {
		return nil // never started; nothing to tear down
	}
	var err error
	a.stopOnce.Do(func() {
		cancel()
		select {
		case <-done:
		case <-ctx.Done():
			err = ctx.Err()
		}
	})
	return err
}

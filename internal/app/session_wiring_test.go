// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// SP3 red: the app assembly wires session dispatch (WithSessionStore +
// WithSessionPolicy from config.session) end to end — proven by behavior: a
// built app whose config enables sessions answers a bare "/new" trigger with
// the fixed acknowledgement, through the real store opened at boot.
package app

import (
	"context"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/router"
)

// sessionRecChannel is the shared app fakeChannel plus outbound recording.
type sessionRecChannel struct {
	*fakeChannel
	mu   sync.Mutex
	sent []*envelope.Envelope
}

func (r *sessionRecChannel) Send(_ context.Context, env *envelope.Envelope) error {
	r.mu.Lock()
	r.sent = append(r.sent, env)
	r.mu.Unlock()
	return nil
}

func (r *sessionRecChannel) Sent() []*envelope.Envelope {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*envelope.Envelope(nil), r.sent...)
}

func TestBuild_SessionDispatchWiredFromConfig(t *testing.T) {
	cfg := cfgWith(ollamaBrain())
	cfg.Storage = &config.StorageConfig{Path: filepath.Join(t.TempDir(), "korvun.db")}
	cfg.Session = &config.SessionConfig{} // present block -> default triggers
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	ch := &sessionRecChannel{fakeChannel: newFakeChannel("telegram")}
	app, err := Build(cfg,
		WithLogger(slog.New(slog.DiscardHandler)),
		withChannelFactory(okFactory(ch)))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })

	// A bare default trigger must come back as the fixed ack through the
	// channel — the proof that store AND policy reached the router.
	e := envelope.New("telegram", envelope.Inbound, envelope.Participant{ID: "u1"}).AddText("/new")
	e.Meta["conversation.id"] = "c"
	if err := app.router.DispatchInbound(context.Background(), e); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	// The EXACT fixed ack — a model-generated reply (a live local Ollama
	// answering the literal "/new") must NOT satisfy this test: only the
	// router's session path produces this copy with zero model calls.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, sent := range ch.Sent() {
			if sent.Direction == envelope.Outbound && len(sent.Parts) == 1 &&
				sent.Parts[0].Content == router.SessionResetAck {
				return // the ack arrived — sessions are wired
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no acknowledgement left the channel: session config did not reach the router")
}

func TestBuild_ConsoleChannelAutoRoutesToFirstBrain(t *testing.T) {
	// FR-CONS-2: a console channel with no explicit route must not boot
	// inert — DispatchInbound to it finds a brain (no ErrNoRoute).
	cfg := cfgWith(ollamaBrain())
	cfg.Storage = &config.StorageConfig{Path: filepath.Join(t.TempDir(), "korvun.db")}
	cfg.Session = &config.SessionConfig{}
	cfg.Channels = append(cfg.Channels, config.ChannelConfig{Type: "console"})
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	app, err := Build(cfg,
		WithLogger(slog.New(slog.DiscardHandler)),
		// Per-type fakes: one factory call per configured channel, each
		// registering under its OWN name (a single shared fake would
		// register "telegram" twice and never "console").
		withChannelFactory(func(_ *builder, cc config.ChannelConfig) (Channel, error) {
			return &sessionRecChannel{fakeChannel: newFakeChannel(cc.Type)}, nil
		}))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })

	e := envelope.New("console", envelope.Inbound, envelope.Participant{ID: "op"}).AddText("hola")
	e.Meta["conversation.id"] = "chat-1"
	if err := app.router.DispatchInbound(context.Background(), e); err != nil {
		t.Fatalf("DispatchInbound to console: %v (auto-route missing?)", err)
	}
}

func TestPreflight_AcceptsConsoleChannel(t *testing.T) {
	// The 2026-08-08 harness repro: Preflight constructs channels WITHOUT a
	// store by design (openStore happens inside the cutover), so the console
	// factory must prove constructibility with a nil store — otherwise EVERY
	// reload of a console-bearing config fails preflight and rolls back.
	// Console-only channel list: this test isolates the console factory's
	// nil-store path (a telegram entry would demand its secret and a real
	// getMe — out of scope here).
	cfg := cfgWith(ollamaBrain())
	cfg.Channels = []config.ChannelConfig{{Type: "console"}}
	cfg.Routes = []config.RouteConfig{{Channel: "console", Brain: "default"}}
	cfg.Storage = &config.StorageConfig{Path: filepath.Join(t.TempDir(), "korvun.db")}
	cfg.Session = &config.SessionConfig{}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := Preflight(cfg, WithLogger(slog.New(slog.DiscardHandler))); err != nil {
		t.Fatalf("Preflight with console channel: %v", err)
	}
}

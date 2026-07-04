// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/Sebastian197/korvun/internal/channel"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/policy"
)

// TestPreflight_doesNotOpenStore is the LOAD-BEARING F1 test (ADR-0027 §b, step 5):
// Preflight validates effect-free — a configured storage path must NOT be created
// or opened by Preflight. openStore stays INSIDE the cutover, after the old app's
// Shutdown closes the old store, so the store is never open twice.
func TestPreflight_doesNotOpenStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "korvun.db")
	cfg := cfgWith(ollamaBrain())
	cfg.Storage = &config.StorageConfig{Path: dbPath}

	if err := Preflight(cfg, withChannelFactory(okFactory(newFakeChannel("telegram")))); err != nil {
		t.Fatalf("Preflight on a valid config: %v", err)
	}
	if _, statErr := os.Stat(dbPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("Preflight opened the store: %q exists (want not-created); stat err = %v", dbPath, statErr)
	}
}

// TestPreflight_badToken_failsBeforeAnyEffect proves the throwaway getMe validation
// (channel construction via telegram.New -> bot.New) surfaces a bad token, and that
// it fails WITHOUT opening the store — the old app is never touched (ADR-0027 §5:
// failing is cheap and safe).
func TestPreflight_badToken_failsBeforeAnyEffect(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "korvun.db")
	cfg := cfgWith(ollamaBrain())
	cfg.Storage = &config.StorageConfig{Path: dbPath}

	boom := errors.New("telegram: bot.New: getMe: 401 Unauthorized")
	failFactory := func(*builder, config.ChannelConfig) (Channel, error) { return nil, boom }

	err := Preflight(cfg, withChannelFactory(failFactory))
	if !errors.Is(err, boom) {
		t.Fatalf("Preflight err = %v, want the wrapped getMe/bad-token failure", err)
	}
	if _, statErr := os.Stat(dbPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("Preflight opened the store on a failing check: %q exists", dbPath)
	}
}

// TestPreflight_getMe_runsExactlyOnce pins option B: the Preflight constructs the
// channel (the getMe surrogate) exactly once. The cutover's wire does the SECOND,
// strictly-sequential getMe (ADR-0027 §6); here we assert the preflight half is one.
func TestPreflight_getMe_runsExactlyOnce(t *testing.T) {
	var calls int32
	fake := newFakeChannel("telegram")
	counting := func(*builder, config.ChannelConfig) (Channel, error) {
		atomic.AddInt32(&calls, 1)
		return fake, nil
	}
	if err := Preflight(cfgWith(ollamaBrain()), withChannelFactory(counting)); err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("channel construction (getMe surrogate) ran %d times, want exactly 1", n)
	}
}

// TestPreflight_missingGroqSecret proves secret resolution is part of the effect-free
// pass: an unset api_key_env fails with ErrMissingSecret before any resource opens.
func TestPreflight_missingGroqSecret(t *testing.T) {
	b := config.BrainConfig{
		Name:        "default",
		Sensitivity: "public",
		Policy:      config.PolicyConfig{Kind: "priority"},
		Models: []config.ModelConfig{
			{Provider: "groq", ModelID: "llama-3.3-70b-versatile", Locality: "cloud", APIKeyEnv: "KORVUN_TEST_GROQ_UNSET_PF"},
		},
	}
	err := Preflight(cfgWith(b), withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if !errors.Is(err, ErrMissingSecret) {
		t.Fatalf("err = %v, want ErrMissingSecret", err)
	}
}

// TestPreflight_privacySelector_noEligibleModels proves the privacy selector runs in
// Preflight: a Private brain with only cloud models has nothing eligible, so Preflight
// fails loudly (ADR-0015) — caught before the old app is ever touched.
func TestPreflight_privacySelector_noEligibleModels(t *testing.T) {
	t.Setenv("KORVUN_TEST_GROQ_KEY_PF", "test-key-value")
	b := config.BrainConfig{
		Name:        "private-brain",
		Sensitivity: "private",
		Policy:      config.PolicyConfig{Kind: "priority"},
		Models: []config.ModelConfig{
			{Provider: "groq", ModelID: "llama-3.3-70b-versatile", Locality: "cloud", APIKeyEnv: "KORVUN_TEST_GROQ_KEY_PF"},
		},
	}
	err := Preflight(cfgWith(b), withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if !errors.Is(err, policy.ErrNoEligibleModels) {
		t.Fatalf("err = %v, want policy.ErrNoEligibleModels", err)
	}
}

// TestPreflight_repeatable_noSideEffects proves Preflight is safe to call twice (two
// reload attempts) and starts NO workers / serving: it constructs-and-discards, never
// Starts a channel. Run under -race (make quality) this also covers the leak/race
// dimension with the stateful fake channel.
func TestPreflight_repeatable_noSideEffects(t *testing.T) {
	fake := newFakeChannel("telegram")
	cfg := cfgWith(ollamaBrain())
	for i := 0; i < 2; i++ {
		if err := Preflight(cfg, withChannelFactory(okFactory(fake))); err != nil {
			t.Fatalf("Preflight call %d: %v", i, err)
		}
	}
	if fake.isStarted() {
		t.Error("Preflight Started a channel; it must construct-and-discard, never start workers/serving")
	}
	if fake.isStopped() {
		t.Error("Preflight Stopped a channel; it should neither start nor stop anything")
	}
}

// registerSpyChannel trips a flag if the router ever registers or starts it.
// router.RegisterChannel calls ch.Receive (router.go:131) BEFORE launching the
// inbound pump + outbound worker (router.go:162-163), and Start begins polling —
// so a correct Preflight, which only CONSTRUCTS and discards a channel, leaves
// both flags false. atomic so a leaked pump goroutine from a regression can't race
// the read.
type registerSpyChannel struct {
	name       string
	registered atomic.Bool // set by Receive — the router.RegisterChannel path
	started    atomic.Bool // set by Start
}

func (c *registerSpyChannel) Name() string               { return c.name }
func (c *registerSpyChannel) Manifest() channel.Manifest { return channel.Manifest{Text: true} }
func (c *registerSpyChannel) Send(context.Context, *envelope.Envelope) error {
	return nil
}
func (c *registerSpyChannel) Receive(context.Context) (<-chan *envelope.Envelope, error) {
	c.registered.Store(true)
	return make(chan *envelope.Envelope), nil
}
func (c *registerSpyChannel) Start(context.Context) error { c.started.Store(true); return nil }
func (c *registerSpyChannel) Stop(context.Context) error  { return nil }

// TestPreflight_neverRegistersOnRouter is the load-bearing regression guard for the
// effect-free "no workers" property (the /review P2 that the prior isStarted-only
// assertion did not catch). It proves the property at its ROOT: Preflight must never
// register a channel on a router. router.RegisterChannel calls ch.Receive
// (router.go:131) and then starts the inbound pump + worker (router.go:162-163), so
// a spy whose Receive/Start trips a flag stays untripped after a correct Preflight.
// If a regression reintroduced wire()/RegisterChannel into Preflight, RegisterChannel
// would call Receive, this flag would flip, and the test would FAIL — the exact
// pump-starting hole the review identified. Deterministic (a flag, no goroutine
// counting), stdlib only.
func TestPreflight_neverRegistersOnRouter(t *testing.T) {
	spy := &registerSpyChannel{name: "telegram"}
	factory := func(*builder, config.ChannelConfig) (Channel, error) { return spy, nil }

	if err := Preflight(cfgWith(ollamaBrain()), withChannelFactory(factory)); err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if spy.registered.Load() {
		t.Error("Preflight registered the channel on a router (ch.Receive was called): it must only construct and discard. A wire()/RegisterChannel regression would start the router pumps (router.go:162-163).")
	}
	if spy.started.Load() {
		t.Error("Preflight Started the channel: it must never Start.")
	}
}

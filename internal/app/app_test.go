// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/channel"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/policy"
	"github.com/Sebastian197/korvun/internal/router"
)

// fakeChannel implements app.Channel for boot/lifecycle tests, with no real
// transport. Stop closes inbound so the router's pump drains and exits.
type fakeChannel struct {
	name    string
	inbound chan *envelope.Envelope

	mu      sync.Mutex
	started bool
	stopped bool
}

func newFakeChannel(name string) *fakeChannel {
	return &fakeChannel{name: name, inbound: make(chan *envelope.Envelope)}
}

func (f *fakeChannel) Name() string               { return f.name }
func (f *fakeChannel) Manifest() channel.Manifest { return channel.Manifest{Text: true} }
func (f *fakeChannel) Send(context.Context, *envelope.Envelope) error {
	return nil
}
func (f *fakeChannel) Receive(context.Context) (<-chan *envelope.Envelope, error) {
	return f.inbound, nil
}
func (f *fakeChannel) Start(context.Context) error {
	f.mu.Lock()
	f.started = true
	f.mu.Unlock()
	return nil
}
func (f *fakeChannel) Stop(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.stopped {
		f.stopped = true
		close(f.inbound)
	}
	return nil
}
func (f *fakeChannel) isStarted() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.started }
func (f *fakeChannel) isStopped() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.stopped }

// okFactory returns a channel factory that always yields the given fake.
func okFactory(ch Channel) func(*builder, config.ChannelConfig) (Channel, error) {
	return func(*builder, config.ChannelConfig) (Channel, error) { return ch, nil }
}

// telegramChannel is a valid single-telegram channel config.
func telegramChannel() config.ChannelConfig {
	return config.ChannelConfig{Type: "telegram", Mode: "polling", TokenEnv: "KORVUN_TEST_TOKEN"}
}

// cfgWith builds a one-channel, one-brain, one-route config around the given
// brain.
func cfgWith(b config.BrainConfig) *config.Config {
	return &config.Config{
		Channels: []config.ChannelConfig{telegramChannel()},
		Brains:   []config.BrainConfig{b},
		Routes:   []config.RouteConfig{{Channel: "telegram", Brain: b.Name}},
	}
}

func ollamaBrain() config.BrainConfig {
	return config.BrainConfig{
		Name:        "default",
		Sensitivity: "public",
		Policy:      config.PolicyConfig{Kind: "priority", Order: []string{"ollama"}},
		Models: []config.ModelConfig{
			{Provider: "ollama", ModelID: "llama3.2", Locality: "local"},
		},
	}
}

// TestBuild_success_ollamaDownIsNotFatal proves the happy path AND the golden
// rule: Ollama never connects at construction, so Build succeeds even with no
// Ollama running — a downed local provider is not a boot error (ADR-0017 §5).
func TestBuild_success_ollamaDownIsNotFatal(t *testing.T) {
	fake := newFakeChannel("telegram")
	app, err := Build(cfgWith(ollamaBrain()), withChannelFactory(okFactory(fake)))
	if err != nil {
		t.Fatalf("Build (Ollama down should still boot): %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = app.Shutdown(ctx)
	})
	if len(app.channels) != 1 {
		t.Fatalf("app has %d channels, want 1", len(app.channels))
	}
}

func TestBuild_telegramTokenEnvMissing(t *testing.T) {
	// Real default factory; the named env var is unset → fatal boot error that
	// names the var, WITHOUT any network call (fails before telegram.New).
	cfg := cfgWith(ollamaBrain())
	cfg.Channels[0].TokenEnv = "KORVUN_TEST_TOKEN_DEFINITELY_UNSET"
	_, err := Build(cfg) // no factory injection → defaultChannelFactory
	if !errors.Is(err, ErrMissingSecret) {
		t.Fatalf("err = %v, want ErrMissingSecret", err)
	}
	if !strings.Contains(err.Error(), "KORVUN_TEST_TOKEN_DEFINITELY_UNSET") {
		t.Errorf("error %q does not name the missing env var", err.Error())
	}
}

// TestBuild_channelConstructionFails simulates the production invalid-token
// path: telegram.New (via bot.New's getMe) returns an error. Build must surface
// it as a fatal boot error rather than booting a silently-deaf binary.
func TestBuild_channelConstructionFails(t *testing.T) {
	boom := errors.New("telegram: bot.New: getMe: 401 Unauthorized")
	failFactory := func(*builder, config.ChannelConfig) (Channel, error) { return nil, boom }
	_, err := Build(cfgWith(ollamaBrain()), withChannelFactory(failFactory))
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want wrapped getMe failure", err)
	}
	if !strings.Contains(err.Error(), "build channel") {
		t.Errorf("error %q should name the channel build step", err.Error())
	}
}

func TestBuild_missingGroqKey(t *testing.T) {
	b := config.BrainConfig{
		Name:        "default",
		Sensitivity: "public",
		Policy:      config.PolicyConfig{Kind: "priority"},
		Models: []config.ModelConfig{
			{Provider: "groq", ModelID: "llama-3.3-70b-versatile", Locality: "cloud", APIKeyEnv: "KORVUN_TEST_GROQ_UNSET"},
		},
	}
	_, err := Build(cfgWith(b), withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if !errors.Is(err, ErrMissingSecret) {
		t.Fatalf("err = %v, want ErrMissingSecret", err)
	}
	if !strings.Contains(err.Error(), "KORVUN_TEST_GROQ_UNSET") {
		t.Errorf("error %q does not name the missing API key env var", err.Error())
	}
}

// TestBuild_unknownProvider exercises the app-layer guard for a Config built
// without config.Load (which would have rejected it first).
func TestBuild_unknownProvider(t *testing.T) {
	b := config.BrainConfig{
		Name:        "default",
		Sensitivity: "public",
		Policy:      config.PolicyConfig{Kind: "priority"},
		Models: []config.ModelConfig{
			{Provider: "openai", ModelID: "gpt", Locality: "cloud", APIKeyEnv: "X"},
		},
	}
	_, err := Build(cfgWith(b), withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if !errors.Is(err, ErrUnknownProvider) {
		t.Fatalf("err = %v, want ErrUnknownProvider", err)
	}
	if !strings.Contains(err.Error(), "openai") {
		t.Errorf("error %q does not name the unknown provider", err.Error())
	}
}

// TestBuild_privateBrainCloudOnly proves the privacy selector is wired: a
// Private brain whose only model is cloud has nothing eligible, so Build fails
// loudly at boot (ADR-0015).
func TestBuild_privateBrainCloudOnly(t *testing.T) {
	t.Setenv("KORVUN_TEST_GROQ_KEY", "test-key-value")
	b := config.BrainConfig{
		Name:        "private-brain",
		Sensitivity: "private",
		Policy:      config.PolicyConfig{Kind: "priority"},
		Models: []config.ModelConfig{
			{Provider: "groq", ModelID: "llama-3.3-70b-versatile", Locality: "cloud", APIKeyEnv: "KORVUN_TEST_GROQ_KEY"},
		},
	}
	_, err := Build(cfgWith(b), withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if !errors.Is(err, policy.ErrNoEligibleModels) {
		t.Fatalf("err = %v, want policy.ErrNoEligibleModels", err)
	}
}

// TestBuild_groqAndBaseURL covers the Groq construction path (key present, no
// network at construction) and the optional base_url branch for both providers,
// plus WithLogger. A public brain keeps both providers through the selector.
func TestBuild_groqAndBaseURL(t *testing.T) {
	t.Setenv("KORVUN_TEST_GROQ_KEY", "test-key-value")
	b := config.BrainConfig{
		Name:        "default",
		Sensitivity: "public",
		Policy:      config.PolicyConfig{Kind: "priority", Order: []string{"ollama", "groq"}},
		Models: []config.ModelConfig{
			{Provider: "ollama", ModelID: "llama3.2", Locality: "local", BaseURL: "http://localhost:11434"},
			{Provider: "groq", ModelID: "llama-3.3-70b-versatile", Locality: "cloud", BaseURL: "https://api.groq.com/openai/v1", APIKeyEnv: "KORVUN_TEST_GROQ_KEY"},
		},
	}
	app, err := Build(cfgWith(b),
		WithLogger(slog.New(slog.DiscardHandler)),
		withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build groq+base_url: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = app.Shutdown(ctx)
	})
}

// TestBuild_appLayerGuards covers the app-layer semantic guards for values
// config.Load would normally reject first (raw Config bypasses Load).
func TestBuild_appLayerGuards(t *testing.T) {
	t.Run("unknown policy kind", func(t *testing.T) {
		b := config.BrainConfig{
			Name: "d", Sensitivity: "public",
			Policy: config.PolicyConfig{Kind: "vote"},
			Models: []config.ModelConfig{{Provider: "ollama", ModelID: "m", Locality: "local"}},
		}
		_, err := Build(cfgWith(b), withChannelFactory(okFactory(newFakeChannel("telegram"))))
		if !errors.Is(err, ErrUnknownPolicy) {
			t.Fatalf("err = %v, want ErrUnknownPolicy", err)
		}
	})
	t.Run("unknown sensitivity", func(t *testing.T) {
		b := config.BrainConfig{
			Name: "d", Sensitivity: "secret",
			Policy: config.PolicyConfig{Kind: "priority"},
			Models: []config.ModelConfig{{Provider: "ollama", ModelID: "m", Locality: "local"}},
		}
		_, err := Build(cfgWith(b), withChannelFactory(okFactory(newFakeChannel("telegram"))))
		if !errors.Is(err, policy.ErrUnknownSensitivity) {
			t.Fatalf("err = %v, want policy.ErrUnknownSensitivity", err)
		}
	})
}

// TestBuild_sequentialDispatchAndConsensus exercises the non-default coordinator
// (sequential fail-over, ADR-0017 §3) and the consensus policy branch.
func TestBuild_sequentialDispatchAndConsensus(t *testing.T) {
	b := config.BrainConfig{
		Name:        "default",
		Sensitivity: "public",
		Dispatch:    "sequential",
		Policy:      config.PolicyConfig{Kind: "consensus", Order: []string{"ollama"}},
		Models: []config.ModelConfig{
			{Provider: "ollama", ModelID: "llama3.2", Locality: "local"},
		},
	}
	app, err := Build(cfgWith(b), withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build sequential+consensus: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = app.Shutdown(ctx)
	})
}

// TestBuild_unknownLocality covers the app-layer locality guard (config.Load
// would reject it first; this is the guard for a raw Config).
func TestBuild_unknownLocality(t *testing.T) {
	b := config.BrainConfig{
		Name:        "default",
		Sensitivity: "public",
		Policy:      config.PolicyConfig{Kind: "priority"},
		Models: []config.ModelConfig{
			{Provider: "ollama", ModelID: "m", Locality: "edge"},
		},
	}
	_, err := Build(cfgWith(b), withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if !errors.Is(err, ErrUnknownLocality) {
		t.Fatalf("err = %v, want ErrUnknownLocality", err)
	}
}

// TestBuild_unknownChannelType covers defaultChannelFactory's guard for a
// channel type this build cannot construct (raw Config, no injection).
func TestBuild_unknownChannelType(t *testing.T) {
	cfg := cfgWith(ollamaBrain())
	cfg.Channels[0].Type = "discord"
	cfg.Routes[0].Channel = "discord"
	_, err := Build(cfg) // default factory → unknown type
	if !errors.Is(err, ErrUnknownChannelType) {
		t.Fatalf("err = %v, want ErrUnknownChannelType", err)
	}
}

// TestRun_startFailureRollsBack confirms that if a later channel fails to
// Start, the channels already started are stopped before Run returns.
func TestRun_startFailureRollsBack(t *testing.T) {
	ok := newFakeChannel("ok")
	bad := &startErrChannel{name: "bad"}
	a := &App{
		router:   router.New(),
		channels: []Channel{ok, bad},
		logger:   slog.Default(),
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = a.router.Shutdown(ctx)
	})

	err := a.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "start channel") {
		t.Fatalf("Run err = %v, want a start-channel failure", err)
	}
	if !ok.isStopped() {
		t.Error("the already-started channel was not rolled back (stopped) on a later Start failure")
	}
}

// TestShutdown_joinsChannelStopError confirms a channel Stop failure is
// surfaced (joined) rather than swallowed, and does not stop the router teardown.
func TestShutdown_joinsChannelStopError(t *testing.T) {
	a := &App{
		router:   router.New(),
		channels: []Channel{&stopErrChannel{name: "telegram"}},
		logger:   slog.Default(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := a.Shutdown(ctx)
	if err == nil || !strings.Contains(err.Error(), "stop channel") {
		t.Fatalf("Shutdown err = %v, want a joined stop-channel failure", err)
	}
}

// stopErrChannel is a Channel whose Stop always fails.
type stopErrChannel struct{ name string }

func (c *stopErrChannel) Name() string               { return c.name }
func (c *stopErrChannel) Manifest() channel.Manifest { return channel.Manifest{Text: true} }
func (c *stopErrChannel) Send(context.Context, *envelope.Envelope) error {
	return nil
}
func (c *stopErrChannel) Receive(context.Context) (<-chan *envelope.Envelope, error) {
	return make(chan *envelope.Envelope), nil
}
func (c *stopErrChannel) Start(context.Context) error { return nil }
func (c *stopErrChannel) Stop(context.Context) error  { return errors.New("stop boom") }

// startErrChannel is a Channel whose Start always fails.
type startErrChannel struct {
	name    string
	mu      sync.Mutex
	stopped bool
}

func (c *startErrChannel) Name() string               { return c.name }
func (c *startErrChannel) Manifest() channel.Manifest { return channel.Manifest{Text: true} }
func (c *startErrChannel) Send(context.Context, *envelope.Envelope) error {
	return nil
}
func (c *startErrChannel) Receive(context.Context) (<-chan *envelope.Envelope, error) {
	return make(chan *envelope.Envelope), nil
}
func (c *startErrChannel) Start(context.Context) error { return errors.New("start boom") }
func (c *startErrChannel) Stop(context.Context) error {
	c.mu.Lock()
	c.stopped = true
	c.mu.Unlock()
	return nil
}

// TestRunShutdown_lifecycle confirms Run starts the channel and Shutdown stops
// it in ADR-0008 order without hanging.
func TestRunShutdown_lifecycle(t *testing.T) {
	fake := newFakeChannel("telegram")
	app, err := Build(cfgWith(ollamaBrain()), withChannelFactory(okFactory(fake)))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- app.Run(ctx) }()

	deadline := time.Now().Add(time.Second)
	for !fake.isStarted() && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if !fake.isStarted() {
		t.Fatal("Run did not start the channel")
	}

	cancel() // signal shutdown
	select {
	case <-runDone:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}

	shutCtx, shutCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutCancel()
	if err := app.Shutdown(shutCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if !fake.isStopped() {
		t.Error("Shutdown did not stop the channel")
	}
}

// --- Stage 9 ADR-B: durable conversation store wiring (ADR-0019 §6) ----------

// TestBuild_noStorage_stateless confirms the default: with no storage block, no
// store is opened and the app owns no closer — exact Stage 11 / ADR-0018 behavior.
func TestBuild_noStorage_stateless(t *testing.T) {
	app, err := Build(cfgWith(ollamaBrain()), withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = app.Shutdown(ctx)
	})
	if app.store != nil {
		t.Fatalf("app.store = %v, want nil (no storage configured)", app.store)
	}
}

// TestBuild_storage_opensSharedStoreAndOwnsCloser confirms a configured store is
// opened once, the app owns its closer, the DB file is created, and Shutdown
// closes it cleanly. Two brains share the one store (opened before the brain loop).
func TestBuild_storage_opensSharedStoreAndOwnsCloser(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "korvun.db")
	cfg := &config.Config{
		Channels: []config.ChannelConfig{telegramChannel()},
		Brains: []config.BrainConfig{
			{Name: "a", Sensitivity: "public", Policy: config.PolicyConfig{Kind: "priority"},
				Models: []config.ModelConfig{{Provider: "ollama", ModelID: "m", Locality: "local"}}},
			{Name: "b", Sensitivity: "public", Policy: config.PolicyConfig{Kind: "priority"},
				Models: []config.ModelConfig{{Provider: "ollama", ModelID: "m", Locality: "local"}}},
		},
		Routes:  []config.RouteConfig{{Channel: "telegram", Brain: "a"}},
		Storage: &config.StorageConfig{Path: dbPath},
	}
	app, err := Build(cfg, withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build with storage: %v", err)
	}
	if app.store == nil {
		t.Fatal("app.store is nil, want the opened store's closer")
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("DB file was not created at %q: %v", dbPath, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := app.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown closing the store: %v", err)
	}
}

// recordingCloser counts Close calls for the shutdown-ordering tests.
type recordingCloser struct{ closed int }

func (c *recordingCloser) Close() error { c.closed++; return nil }

// TestShutdown_closesStoreAfterCleanDrain asserts the store is closed exactly
// once when the router drains cleanly (routerErr == nil) — the gated Close path
// (ADR-0019 §6). An idle router.New() drains immediately, so Shutdown reaches the
// store Close.
func TestShutdown_closesStoreAfterCleanDrain(t *testing.T) {
	rc := &recordingCloser{}
	a := &App{router: router.New(), logger: slog.Default(), store: rc}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := a.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if rc.closed != 1 {
		t.Fatalf("store Close called %d times, want 1 (clean drain must close it)", rc.closed)
	}
}

// TestBuild_storage_openFailureIsFatal confirms a configured-but-unopenable store
// fails Build loudly (the boot-fatal path, ADR-0019 §5): a path whose parent is a
// regular file cannot be created.
func TestBuild_storage_openFailureIsFatal(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "afile")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	cfg := cfgWith(ollamaBrain())
	cfg.Storage = &config.StorageConfig{Path: filepath.Join(blocker, "korvun.db")}
	_, err := Build(cfg, withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err == nil {
		t.Fatal("Build with an unopenable store returned nil error, want a fatal boot error")
	}
	if !strings.Contains(err.Error(), "conversation store") {
		t.Errorf("error %q should name the conversation store open step", err.Error())
	}
}

// TestBuild_storage_emptyPathUsesDefault confirms an empty Path resolves to the
// OS config dir default. HOME (darwin) and XDG_CONFIG_HOME (linux) are redirected
// to a temp dir so the test never writes to the real user config dir.
func TestBuild_storage_emptyPathUsesDefault(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)            // darwin: <HOME>/Library/Application Support
	t.Setenv("XDG_CONFIG_HOME", tmp) // linux: <XDG_CONFIG_HOME>
	cfg := cfgWith(ollamaBrain())
	cfg.Storage = &config.StorageConfig{} // present, empty path → default
	app, err := Build(cfg, withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build with default storage path: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = app.Shutdown(ctx)
	})
	if app.store == nil {
		t.Fatal("app.store is nil, want the default-path store")
	}
}

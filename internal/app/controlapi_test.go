// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/policy"
)

const (
	testGroqKeyEnv = "KORVUN_TEST_GROQ_KEY"
	testGroqSecret = "sk-supersecret-DO-NOT-LEAK"
)

// localCloudPrivateBrain is a Private brain wired with one Local (ollama) and one
// Cloud (groq) model. The privacy selector must drop the cloud model at boot, so
// the resolved summary shows ONLY the local one.
func localCloudPrivateBrain() config.BrainConfig {
	return config.BrainConfig{
		Name:        "secure",
		Sensitivity: "private",
		Policy:      config.PolicyConfig{Kind: "priority", Order: []string{"ollama"}},
		Dispatch:    "sequential",
		Models: []config.ModelConfig{
			{Provider: "ollama", ModelID: "llama3.2", Locality: "local"},
			{Provider: "groq", ModelID: "llama-3.3-70b", Locality: "cloud", APIKeyEnv: testGroqKeyEnv},
		},
	}
}

// fakeDroppingChannel is a fakeChannel that also exposes a live, mutable drop
// count, so app-level tests can prove ChannelSummaries reads it at request time.
type fakeDroppingChannel struct {
	*fakeChannel
	dropped uint64
}

func (f *fakeDroppingChannel) DroppedCount() uint64 { return atomic.LoadUint64(&f.dropped) }

// TestControlAPI_brainSummary_resolvedPostSelector is the headline behavior: the
// /api/brains view reflects the selector's EFFECT, not the raw config — a Private
// brain's cloud model is absent because it was never dispatchable (ADR-0022 §2).
func TestControlAPI_brainSummary_resolvedPostSelector(t *testing.T) {
	t.Setenv(testGroqKeyEnv, testGroqSecret)

	app, err := Build(cfgWith(localCloudPrivateBrain()),
		WithLogger(slog.New(slog.DiscardHandler)),
		withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })

	got := app.BrainSummaries()
	if len(got) != 1 {
		t.Fatalf("got %d brains, want 1", len(got))
	}
	b := got[0]
	if b.Name != "secure" || b.Sensitivity != "private" || b.Policy != "priority" || b.Dispatch != "sequential" {
		t.Errorf("brain attrs = %+v, want name=secure sensitivity=private policy=priority dispatch=sequential", b)
	}
	if len(b.Models) != 1 {
		t.Fatalf("resolved models = %+v, want exactly 1 (cloud dropped by selector)", b.Models)
	}
	if b.Models[0].Provider != "ollama" || b.Models[0].ModelID != "llama3.2" {
		t.Errorf("survivor = %+v, want {ollama llama3.2}", b.Models[0])
	}
	for _, m := range b.Models {
		if m.Provider == "groq" {
			t.Error("cloud model groq leaked into a Private brain's resolved summary")
		}
	}
}

// TestBrainSummary_matchesSelector is the anti-drift guard: the providers in the
// summary must equal those policy.SelectModels actually keeps, for both
// sensitivities — so the summary rule can never silently diverge from the real
// selector (ADR-0022 §3, brainSummary godoc).
func TestBrainSummary_matchesSelector(t *testing.T) {
	t.Setenv(testGroqKeyEnv, testGroqSecret)
	b := testBuilder()

	for _, sens := range []string{"public", "private"} {
		bc := localCloudPrivateBrain()
		bc.Sensitivity = sens

		catalog, err := b.buildCatalog(bc)
		if err != nil {
			t.Fatalf("buildCatalog(%s): %v", sens, err)
		}
		parsed, err := parseSensitivity(sens)
		if err != nil {
			t.Fatalf("parseSensitivity(%s): %v", sens, err)
		}
		selected, err := policy.SelectModels(catalog, parsed)
		if err != nil {
			t.Fatalf("SelectModels(%s): %v", sens, err)
		}
		summary, err := brainSummary(bc)
		if err != nil {
			t.Fatalf("brainSummary(%s): %v", sens, err)
		}

		if len(summary.Models) != len(selected) {
			t.Fatalf("%s: summary has %d models, selector kept %d", sens, len(summary.Models), len(selected))
		}
		for i, m := range selected {
			if summary.Models[i].Provider != m.Name() {
				t.Errorf("%s: model[%d] provider = %q, selector = %q", sens, i, summary.Models[i].Provider, m.Name())
			}
		}
	}
}

// TestControlAPI_channelSummaries_liveDropCount proves the static facts are
// captured and the drop count is read LIVE at request time.
func TestControlAPI_channelSummaries_liveDropCount(t *testing.T) {
	fake := &fakeDroppingChannel{fakeChannel: newFakeChannel("telegram")}
	app, err := Build(cfgWith(ollamaBrain()),
		WithLogger(slog.New(slog.DiscardHandler)),
		withChannelFactory(okFactory(fake)))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })

	atomic.StoreUint64(&fake.dropped, 5)
	got := app.ChannelSummaries()
	if len(got) != 1 {
		t.Fatalf("got %d channels, want 1", len(got))
	}
	c := got[0]
	if c.Type != "telegram" || c.Mode != "polling" || c.Name != "telegram" {
		t.Errorf("channel facts = %+v, want type=telegram mode=polling name=telegram", c)
	}
	if c.Dropped == nil || *c.Dropped != 5 {
		t.Fatalf("dropped = %v, want 5", c.Dropped)
	}

	// Live, not snapshot: a later read sees the new count.
	atomic.StoreUint64(&fake.dropped, 9)
	if c2 := app.ChannelSummaries(); c2[0].Dropped == nil || *c2[0].Dropped != 9 {
		t.Errorf("dropped after change = %v, want 9 (read must be live)", c2[0].Dropped)
	}
}

// TestControlAPI_channelSummaries_noCounterOmitsDropped asserts a channel with no
// DroppedCount accessor omits the field entirely (omitempty).
func TestControlAPI_channelSummaries_noCounterOmitsDropped(t *testing.T) {
	app, err := Build(cfgWith(ollamaBrain()),
		WithLogger(slog.New(slog.DiscardHandler)),
		withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })

	got := app.ChannelSummaries()
	if len(got) != 1 || got[0].Dropped != nil {
		t.Errorf("dropped = %v, want nil for a channel with no counter", got[0].Dropped)
	}
}

// TestControlAPI_endToEnd_coexistsAndLeaksNoSecrets runs the real app: /api/brains
// and /api/channels answer 200 with JSON ALONGSIDE the untouched /healthz and
// /metrics, and neither /api response contains a secret value or an env-var NAME
// (ADR-0022 §4, §5).
func TestControlAPI_endToEnd_coexistsAndLeaksNoSecrets(t *testing.T) {
	t.Setenv(testGroqKeyEnv, testGroqSecret)

	app, err := Build(cfgWith(localCloudPrivateBrain()),
		WithLogger(slog.New(slog.DiscardHandler)),
		withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- app.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		<-runDone
		sc, c := context.WithTimeout(context.Background(), time.Second)
		defer c()
		_ = app.Shutdown(sc)
	})

	base := func() string { return "http://" + app.adminServer.Addr() }
	if !waitFor(t, func() bool {
		if app.adminServer.Addr() == "" {
			return false
		}
		code, _ := tryGet(base() + "/healthz")
		return code == 200
	}) {
		t.Fatal("admin server never became healthy")
	}

	// All four routes live on the same server.
	for _, path := range []string{"/healthz", "/metrics", "/api/brains", "/api/channels"} {
		if code, _ := tryGet(base() + path); code != 200 {
			t.Errorf("GET %s = %d, want 200", path, code)
		}
	}

	// The /api responses are valid JSON and leak no secret value nor env-var name.
	for _, path := range []string{"/api/brains", "/api/channels"} {
		code, body := tryGet(base() + path)
		if code != 200 {
			t.Fatalf("GET %s = %d", path, code)
		}
		var js any
		if err := json.Unmarshal([]byte(body), &js); err != nil {
			t.Errorf("GET %s: body is not valid JSON: %v", path, err)
		}
		for _, forbidden := range []string{testGroqSecret, testGroqKeyEnv, "KORVUN_TEST_TOKEN", "api_key", "token_env"} {
			if strings.Contains(body, forbidden) {
				t.Errorf("GET %s leaked %q in body: %s", path, forbidden, body)
			}
		}
	}
}

// TestControlAPI_notServedWhenObservabilityDisabled documents the conscious
// coupling (ADR-0022 §5): no admin server => no /api. The Reader still functions
// (the snapshot exists); it is simply not exposed.
func TestControlAPI_notServedWhenObservabilityDisabled(t *testing.T) {
	disabled := false
	cfg := cfgWith(ollamaBrain())
	cfg.Observability = &config.ObservabilityConfig{Enabled: &disabled}

	app, err := Build(cfg,
		WithLogger(slog.New(slog.DiscardHandler)),
		withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })

	if app.adminServer != nil {
		t.Fatal("admin server is non-nil with observability disabled; /api would be exposed")
	}
	// The Reader still works — only the exposure is gated.
	if len(app.BrainSummaries()) != 1 {
		t.Error("BrainSummaries should still function even when /api is not served")
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

// RED suite for AS-4 + the wiring half of AS-1 of the universal model
// gateway spec (docs/superpowers/specs/2026-08-22-universal-model-gateway.md,
// FINAL): buildModel's "openai-compatible" case, the D2 named-must-resolve
// rule, and the privacy filter proven from the REAL assembly
// (config → Build → inbound message) with a spy server.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/router"
)

// writeCompatOK writes a valid non-streaming compat /chat/completions 200.
func writeCompatOK(w http.ResponseWriter, content string) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"model": "compat-model",
		"choices": []map[string]any{
			{"index": 0, "finish_reason": "stop", "message": map[string]string{"role": "assistant", "content": content}},
		},
	})
}

// TestBuildModel_openaicompat_success pins the FR-GW-6 wiring: the
// provider constructs, and the adapter's Name() is the provider constant
// (FR-GW-2 — the identity the policy Order and attribution ride on).
func TestBuildModel_openaicompat_success(t *testing.T) {
	t.Parallel()
	m, err := testBuilder().buildModel(config.ModelConfig{
		Provider: "openai-compatible",
		ModelID:  "some-model",
		Locality: "local",
		BaseURL:  "http://127.0.0.1:9/v1",
	})
	if err != nil {
		t.Fatalf("buildModel(openai-compatible): %v", err)
	}
	if m.Name() != "openai-compatible" {
		t.Errorf("Name() = %q, want %q", m.Name(), "openai-compatible")
	}
}

// TestBuildModel_openaicompat_missingSecret pins D2 (AS-1): api_key_env
// NAMED but unresolvable at boot fails with ErrMissingSecret naming the
// VARIABLE — never any key material (there is none to name).
func TestBuildModel_openaicompat_missingSecret(t *testing.T) {
	t.Setenv("KORVUN_TEST_COMPAT_KEY", "")
	_, err := testBuilder().buildModel(config.ModelConfig{
		Provider:  "openai-compatible",
		ModelID:   "some-model",
		Locality:  "cloud",
		BaseURL:   "https://api.example.com/v1",
		APIKeyEnv: "KORVUN_TEST_COMPAT_KEY",
	})
	if !errors.Is(err, ErrMissingSecret) {
		t.Fatalf("err = %v, want ErrMissingSecret", err)
	}
	if !strings.Contains(err.Error(), "KORVUN_TEST_COMPAT_KEY") {
		t.Errorf("error %q does not name the variable", err.Error())
	}
}

// runCompatAssembly boots the app with the given config and a capturing
// channel, injects one inbound message, and returns the reply envelope
// (or fails the test on timeout).
func runCompatAssembly(t *testing.T, cfg *config.Config) *envelope.Envelope {
	t.Helper()
	ch := newCapturingChannel("telegram")
	a, err := Build(cfg, withChannelFactory(okFactory(ch)))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- a.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		<-runDone
		shutdownApp(t, a)
	})

	deadline := time.Now().Add(time.Second)
	for !ch.isStarted() && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if !ch.isStarted() {
		t.Fatal("Run did not start the channel")
	}

	in := envelope.New("telegram", envelope.Inbound, envelope.Participant{ID: "u-1"})
	in.AddText("hello")
	in.Meta[router.MetaConversationID] = "c-1"
	ch.inbound <- in

	select {
	case e := <-ch.sent:
		return e
	case <-time.After(8 * time.Second):
		t.Fatal("no reply sent within 8s")
		return nil
	}
}

// compatAssemblyCfg builds a one-brain config around the given models.
func compatAssemblyCfg(sensitivity string, models []config.ModelConfig, order []string) *config.Config {
	return &config.Config{
		Observability: &config.ObservabilityConfig{Enabled: boolPtr(false)},
		Channels:      []config.ChannelConfig{telegramChannel()},
		Brains: []config.BrainConfig{{
			Name:        "default",
			Sensitivity: sensitivity,
			Dispatch:    "fanout",
			Policy:      config.PolicyConfig{Kind: "priority", Order: order},
			Models:      models,
		}},
		Routes: []config.RouteConfig{{Channel: "telegram", Brain: "default"}},
	}
}

// TestAssembly_privateBrainNeverContactsCloudCompat is AS-4's negative
// direction from the REAL assembly (H12): a Private brain wired with a
// cloud compat entry pointing at a SPY, plus a local sibling so Build
// succeeds, handles a message WITHOUT the spy ever seeing a byte.
func TestAssembly_privateBrainNeverContactsCloudCompat(t *testing.T) {
	var spyHits int32
	spy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&spyHits, 1)
		writeCompatOK(w, "leaked")
	}))
	t.Cleanup(spy.Close)

	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeOllamaOK(w, "answered locally")
	}))
	t.Cleanup(local.Close)

	t.Setenv("KORVUN_TEST_COMPAT_SPY_KEY", "spy-key")
	cfg := compatAssemblyCfg("private", []config.ModelConfig{
		{Provider: "ollama", ModelID: "llama3.2", Locality: "local", BaseURL: local.URL},
		{Provider: "openai-compatible", ModelID: "cloud-model", Locality: "cloud",
			BaseURL: spy.URL + "/v1", APIKeyEnv: "KORVUN_TEST_COMPAT_SPY_KEY"},
	}, []string{"ollama", "openai-compatible"})

	e := runCompatAssembly(t, cfg)
	if len(e.Parts) == 0 || !strings.Contains(e.Parts[0].Content, "answered locally") {
		t.Errorf("reply = %+v, want the local sibling's answer", e.Parts)
	}
	if n := atomic.LoadInt32(&spyHits); n != 0 {
		t.Errorf("spy hits = %d, want 0 — a Private brain must never contact a cloud compat entry", n)
	}
}

// TestAssembly_privateBrainSelectsLocalCompat is AS-4's positive
// direction: a compat entry DECLARED local is eligible in a Private brain
// (the LM Studio case) and actually serves the reply.
func TestAssembly_privateBrainSelectsLocalCompat(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		writeCompatOK(w, "local compat answer")
	}))
	t.Cleanup(srv.Close)

	cfg := compatAssemblyCfg("private", []config.ModelConfig{
		{Provider: "openai-compatible", ModelID: "lmstudio-model", Locality: "local", BaseURL: srv.URL + "/v1"},
	}, []string{"openai-compatible"})

	e := runCompatAssembly(t, cfg)
	if len(e.Parts) == 0 || !strings.Contains(e.Parts[0].Content, "local compat answer") {
		t.Errorf("reply = %+v, want the compat server's answer", e.Parts)
	}
	if n := atomic.LoadInt32(&hits); n < 1 {
		t.Errorf("compat server hits = %d, want >= 1 (the local compat entry must be selected)", n)
	}
}

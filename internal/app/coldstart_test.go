// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"encoding/json"
	"io"
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

// This file encodes ADR-0031 sub-phase 5: Chano's cold-start case, F6
// verified on hardware and turned into permanent tests against the REAL retry
// decorator (sub-phase 4). The decorator already exists, so these are invariant
// guards (they are expected to be born GREEN, like the sub-phase-1 SV2 guard) —
// the honest "red" is any test that bites because of a decorator bug.
//
// All three drive Handle end-to-end through an httptest server (no real network,
// no real Ollama) and assert the exact server hit count with atomics.

// coldStartCfg is a one-fanout-brain config over a single Ollama model with a
// tunable per-model request_timeout and max_retries, observability off.
func coldStartCfg(baseURL, requestTimeout string, maxRetries int) *config.Config {
	return &config.Config{
		Observability: &config.ObservabilityConfig{Enabled: boolPtr(false)},
		Channels:      []config.ChannelConfig{telegramChannel()},
		Brains: []config.BrainConfig{{
			Name:        "default",
			Sensitivity: "public",
			Dispatch:    "fanout",
			Policy:      config.PolicyConfig{Kind: "priority", Order: []string{"ollama"}},
			Models: []config.ModelConfig{
				{Provider: "ollama", ModelID: "llama3.2", Locality: "local", BaseURL: baseURL, RequestTimeout: requestTimeout, MaxRetries: maxRetries},
			},
		}},
		Routes: []config.RouteConfig{{Channel: "telegram", Brain: "default"}},
	}
}

// runColdStart boots the app with the given config + capturing channel, injects
// one inbound message, and returns the reply envelope the router sent. It
// centralizes the boot/run/inject/await dance the three cold-start tests share.
func runColdStart(t *testing.T, cfg *config.Config) *envelope.Envelope {
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

// writeOllamaOK writes a valid non-streaming Ollama /api/chat 200 body.
func writeOllamaOK(w http.ResponseWriter, content string) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"model":   "llama3.2",
		"message": map[string]string{"role": "assistant", "content": content},
		"done":    true,
	})
}

// TestColdStart_generousTimeoutLetsLoadComplete is the good face of F6: a
// generous per-attempt timeout lets a slow cold load COMPLETE. The server takes
// ~300ms on its first (only) request — well under the 2s window — and answers.
// The model's answer reaches the channel and the server is hit EXACTLY ONCE (no
// retry was needed). Pins "a generous timeout lets the load complete".
func TestColdStart_generousTimeoutLetsLoadComplete(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		time.Sleep(300 * time.Millisecond) // simulate the cold model load (server-side, not client)
		writeOllamaOK(w, "loaded and answering")
	}))
	t.Cleanup(srv.Close)

	e := runColdStart(t, coldStartCfg(srv.URL, "2s", 1))
	if len(e.Parts) == 0 || !strings.Contains(e.Parts[0].Content, "loaded and answering") {
		t.Errorf("reply = %+v, want the model answer (generous timeout should let the load complete)", e.Parts)
	}
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Errorf("server hits = %d, want exactly 1 (a slow success needs no retry)", n)
	}
}

// TestColdStart_shortTimeoutIsNotRescuedByRetry is the pure F6 face: a short
// per-attempt timeout expires while the model is still loading; the decorator
// does NOT retry (deadline-expiry is non-retryable), so the server is hit
// EXACTLY ONCE — never re-triggering and re-aborting the cold load. The handler
// blocks until the client's ctx is cancelled (Korvun cutting the connection —
// Ollama's "aborting load"). Pins "a short timeout is not rescued by retry; it
// is not even attempted".
func TestColdStart_shortTimeoutIsNotRescuedByRetry(t *testing.T) {
	var hits int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		select {
		case <-r.Context().Done(): // the per-attempt deadline fired; the client disconnected
		case <-release: // teardown safety valve
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })

	e := runColdStart(t, coldStartCfg(srv.URL, "100ms", 2))
	if len(e.Parts) == 0 { // the all-failed fallback reply
		t.Fatalf("reply = %+v, want the all-failed fallback envelope", e.Parts)
	}
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Errorf("server hits = %d, want exactly 1 (deadline-expiry is NOT retried — F6; retrying would re-abort the load)", n)
	}
}

// TestColdStart_transientRefusalIsRetried is the contrast that separates F6 (a
// slow load) from FR-R4 (a fast refusal): a 503 on the first hit (the service is
// still coming up / not yet listening) IS retried, unlike a deadline. The second
// hit answers 200 and the model's reply reaches the channel with EXACTLY 2 hits.
// Pins "a fast transient refusal is retried, a deadline is not". (Overlaps
// TestBuild_retryRecoversTransient503 from sub-phase 4 — see the report's
// recommendation on duplication.)
func TestColdStart_transientRefusalIsRetried(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "starting up")
			return
		}
		writeOllamaOK(w, "up now")
	}))
	t.Cleanup(srv.Close)

	e := runColdStart(t, coldStartCfg(srv.URL, "2s", 1))
	if len(e.Parts) == 0 || !strings.Contains(e.Parts[0].Content, "up now") {
		t.Errorf("reply = %+v, want the model answer (a fast 503 should be retried)", e.Parts)
	}
	if n := atomic.LoadInt32(&hits); n != 2 {
		t.Errorf("server hits = %d, want exactly 2 (fast refusal retried once, then success)", n)
	}
}

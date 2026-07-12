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

// This file is the app-level RED for ADR-0031 sub-phase 4 wiring: with the retry
// decorator wired per instance in buildCatalog, a transient 503 followed by a
// 200 is recovered end-to-end (the model's answer reaches the channel, not the
// all-failed fallback). It is a BEHAVIORAL red: today no decorator is wired, so
// the first 503 collapses to the fallback and this test fails; green wires the
// decorator and the retried 200 wins.

// capturingChannel records the envelope the router sends, so a test can assert
// on the reply content (not just its timing).
type capturingChannel struct {
	*fakeChannel
	sent chan *envelope.Envelope
}

func newCapturingChannel(name string) *capturingChannel {
	return &capturingChannel{fakeChannel: newFakeChannel(name), sent: make(chan *envelope.Envelope, 4)}
}

func (c *capturingChannel) Send(_ context.Context, e *envelope.Envelope) error {
	select {
	case c.sent <- e:
	default:
	}
	return nil
}

// retryOllamaCfg is a one-fanout-brain config over a single Ollama model with a
// per-model max_retries and a generous per-attempt window, observability off.
func retryOllamaCfg(baseURL string, maxRetries int) *config.Config {
	return &config.Config{
		Observability: &config.ObservabilityConfig{Enabled: boolPtr(false)},
		Channels:      []config.ChannelConfig{telegramChannel()},
		Brains: []config.BrainConfig{{
			Name:        "default",
			Sensitivity: "public",
			Dispatch:    "fanout",
			Policy:      config.PolicyConfig{Kind: "priority", Order: []string{"ollama"}},
			Models: []config.ModelConfig{
				{Provider: "ollama", ModelID: "llama3.2", Locality: "local", BaseURL: baseURL, RequestTimeout: "2s", MaxRetries: maxRetries},
			},
		}},
		Routes: []config.RouteConfig{{Channel: "telegram", Brain: "default"}},
	}
}

// seqOllamaCfg is a one-sequential-brain config over a single Ollama model with
// a per-model max_retries. Sequential forces retry OFF by construction (SV2), so
// max_retries must NOT multiply the attempts regardless of its value.
func seqOllamaCfg(baseURL string, maxRetries int) *config.Config {
	return &config.Config{
		Observability: &config.ObservabilityConfig{Enabled: boolPtr(false)},
		Channels:      []config.ChannelConfig{telegramChannel()},
		Brains: []config.BrainConfig{{
			Name:        "default",
			Sensitivity: "public",
			Dispatch:    "sequential",
			Policy:      config.PolicyConfig{Kind: "priority", Order: []string{"ollama"}},
			Models: []config.ModelConfig{
				{Provider: "ollama", ModelID: "llama3.2", Locality: "local", BaseURL: baseURL, RequestTimeout: "2s", MaxRetries: maxRetries},
			},
		}},
		Routes: []config.RouteConfig{{Channel: "telegram", Brain: "default"}},
	}
}

// TestBuild_sequentialForcesRetryOff is the wiring-level SV2 guard: a sequential
// brain's model with max_retries=2 that always 503s is still hit EXACTLY ONCE —
// the wiring forces retry off for sequential (effectiveMaxRetries), so the
// decorator does not multiply the attempt. This BITES the BP-a experiment:
// removing the sequential guard in effectiveMaxRetries makes the server hit 3x.
// (The mechanism-level guard TestRun_failingModelConsumesExactlyOneAttempt uses
// undecorated models and does not exercise the wiring, so this app-level test is
// what proves the wiring's retry-off decision.)
func TestBuild_sequentialForcesRetryOff(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "down")
	}))
	t.Cleanup(srv.Close)

	ch := newCapturingChannel("telegram")
	a, err := Build(seqOllamaCfg(srv.URL, 2), withChannelFactory(okFactory(ch)))
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
	case <-ch.sent: // the all-failed fallback reply
	case <-time.After(8 * time.Second):
		t.Fatal("no reply sent within 8s")
	}
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Errorf("server hits = %d, want exactly 1 (sequential forces retry off — SV2)", n)
	}
}

// TestBuild_retryRecoversTransient503 is the end-to-end behavioral red: an
// Ollama endpoint returns 503 once then a valid 200; with max_retries=1 the
// decorated model retries and the user receives the model's answer, and the
// server is hit exactly twice. RED today (no decorator wired → fallback wins).
func TestBuild_retryRecoversTransient503(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "loading")
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "llama3.2",
			"message": map[string]string{"role": "assistant", "content": "hi there"},
			"done":    true,
		})
	}))
	t.Cleanup(srv.Close)

	ch := newCapturingChannel("telegram")
	a, err := Build(retryOllamaCfg(srv.URL, 1), withChannelFactory(okFactory(ch)))
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
		if len(e.Parts) == 0 || !strings.Contains(e.Parts[0].Content, "hi there") {
			t.Errorf("reply = %+v, want the model answer %q (retry did not recover the 503)", e.Parts, "hi there")
		}
	case <-time.After(8 * time.Second):
		t.Fatal("no reply sent within 8s")
	}
	if n := atomic.LoadInt32(&hits); n != 2 {
		t.Errorf("server hits = %d, want 2 (one 503 + one retried 200)", n)
	}
}

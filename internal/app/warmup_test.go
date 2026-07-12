// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/config"
)

// This file is the app-level RED for ADR-0031 sub-phase 6 (boot warmup). The
// configs are built via JSON + config.Load (NOT a ModelConfig struct literal) so
// the file compiles TODAY even though ModelConfig.Warmup does not exist yet — the
// unknown "warmup" key is ignored by json.Unmarshal. That lets the no-warmup
// guard (AS-4) be born GREEN while the new-mechanics tests are born RED by
// behaviour (no warmup runs yet), the honest sub-phase-5 style split.

// loadCfg writes js to a temp config file and loads it (Validate included).
func loadCfg(t *testing.T, js string) *config.Config {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(js), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg
}

// oneLocalModelCfg builds a one-fanout-brain config over a single local Ollama
// model, optionally marked warmup, with a tunable request_timeout/max_retries.
func oneLocalModelCfg(t *testing.T, baseURL, requestTimeout string, maxRetries int, warmup bool) *config.Config {
	w := ""
	if warmup {
		w = `,"warmup":true`
	}
	js := fmt.Sprintf(`{
	  "observability":{"enabled":false},
	  "channels":[{"type":"telegram","mode":"polling","token_env":"KORVUN_TEST_TOKEN"}],
	  "brains":[{
	    "name":"default","sensitivity":"public","dispatch":"fanout",
	    "policy":{"kind":"priority","order":["ollama"]},
	    "models":[{"provider":"ollama","model_id":"llama3.2","locality":"local","base_url":%q,"request_timeout":%q,"max_retries":%d%s}]
	  }],
	  "routes":[{"channel":"telegram","brain":"default"}]
	}`, baseURL, requestTimeout, maxRetries, w)
	return loadCfg(t, js)
}

// twoIdenticalWarmupModelsCfg builds a fanout brain with TWO models pointing at
// the SAME base_url + model_id, both warmup:true — the dedup case (AS-7). (The
// spec frames it as "two brains"; two identical models in one brain exercises the
// same (provider,baseURL,modelID) dedup key without multi-brain routing.)
func twoIdenticalWarmupModelsCfg(t *testing.T, baseURL string) *config.Config {
	js := fmt.Sprintf(`{
	  "observability":{"enabled":false},
	  "channels":[{"type":"telegram","mode":"polling","token_env":"KORVUN_TEST_TOKEN"}],
	  "brains":[{
	    "name":"default","sensitivity":"public","dispatch":"fanout",
	    "policy":{"kind":"priority","order":["ollama"]},
	    "models":[
	      {"provider":"ollama","model_id":"llama3.2","locality":"local","base_url":%q,"request_timeout":"2s","warmup":true},
	      {"provider":"ollama","model_id":"llama3.2","locality":"local","base_url":%q,"request_timeout":"2s","warmup":true}
	    ]
	  }],
	  "routes":[{"channel":"telegram","brain":"default"}]
	}`, baseURL, baseURL)
	return loadCfg(t, js)
}

// runApp boots the app with a capturing logger and a fake channel, runs it in the
// background, waits for the channel to start (so any post-Start warmup has been
// launched), and returns the log records + a cleanup-registered shutdown.
func runApp(t *testing.T, cfg *config.Config) (*App, *[]slog.Record, *sync.Mutex) {
	t.Helper()
	logger, recs, mu := newCapturingLogger()
	ch := newFakeChannel("telegram")
	a, err := Build(cfg, WithLogger(logger), withChannelFactory(okFactory(ch)))
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
	return a, recs, mu
}

// waitHits polls an atomic hit counter until it reaches want or the deadline.
func waitHits(t *testing.T, hits *int32, want int32, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(hits) >= want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("server hits = %d after %v, want >= %d", atomic.LoadInt32(hits), within, want)
}

// hasLog reports whether a record with the given message was captured.
func hasLog(recs *[]slog.Record, mu *sync.Mutex, msg string) bool {
	mu.Lock()
	defer mu.Unlock()
	for _, r := range *recs {
		if r.Message == msg {
			return true
		}
	}
	return false
}

func writeOllamaChatOK(w http.ResponseWriter) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"model":   "llama3.2",
		"message": map[string]string{"role": "assistant", "content": "warm"},
		"done":    true,
	})
}

// AS-1: a warmup:true local model is warmed at boot without any user message —
// the backend gets exactly one hit and an INFO "model warm" is logged.
func TestWarmup_happyPathWarmsWithoutMessage(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		time.Sleep(200 * time.Millisecond) // simulate the cold load (server-side)
		writeOllamaChatOK(w)
	}))
	t.Cleanup(srv.Close)

	_, recs, mu := runApp(t, oneLocalModelCfg(t, srv.URL, "2s", 1, true))
	waitHits(t, &hits, 1, 3*time.Second) // no user message injected — warmup produced this
	// let the warmup complete and log "model warm"
	deadline := time.Now().Add(2 * time.Second)
	for !hasLog(recs, mu, "model warm") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Errorf("server hits = %d, want exactly 1 (warmup, no retry needed)", n)
	}
	if !hasLog(recs, mu, "model warm") {
		t.Errorf(`no "model warm" INFO logged after a successful warmup`)
	}
}

// AS-4 (no-regression guard, born GREEN): with no model marked warmup, no warmup
// request is sent and no warmup log line appears — behaviour identical to today.
func TestWarmup_toggleOffSendsNothing(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		writeOllamaChatOK(w)
	}))
	t.Cleanup(srv.Close)

	_, recs, mu := runApp(t, oneLocalModelCfg(t, srv.URL, "2s", 1, false))
	// Give any (erroneous) warmup a window to fire, then assert none did.
	time.Sleep(150 * time.Millisecond)
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Errorf("server hits = %d, want 0 (no warmup model → no warmup request)", n)
	}
	if hasLog(recs, mu, "warming up model") || hasLog(recs, mu, "model warm") {
		t.Errorf("a warmup log line appeared with no warmup model configured")
	}
}

// AS-2: a warmup that always fails (500) is best-effort — boot succeeds, a WARN
// names the provider/model, and the channels start regardless.
func TestWarmup_failureIsBestEffort(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "boom")
	}))
	t.Cleanup(srv.Close)

	// runApp asserts the channel started (boot OK). A 500 is ErrProviderResponse
	// (non-retryable), so the warmup hits once and gives up.
	_, recs, mu := runApp(t, oneLocalModelCfg(t, srv.URL, "2s", 1, true))
	waitHits(t, &hits, 1, 3*time.Second)
	deadline := time.Now().Add(2 * time.Second)
	for !hasLog(recs, mu, "warmup failed") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !hasLog(recs, mu, "warmup failed") {
		t.Errorf(`no "warmup failed" WARN logged after a failing warmup (best-effort must log)`)
	}
}

// AS-3 (F6): a warmup whose per-attempt deadline expires is NOT retried — the
// backend is hit exactly once, boot is unaffected.
func TestWarmup_deadlineIsNotRetried(t *testing.T) {
	var hits int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })

	_, _, _ = runApp(t, oneLocalModelCfg(t, srv.URL, "100ms", 2, true))
	waitHits(t, &hits, 1, 3*time.Second)
	// Give any (erroneous) retry a window; the deadline-expiry must NOT retry.
	time.Sleep(400 * time.Millisecond)
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Errorf("server hits = %d, want exactly 1 (warmup deadline is NOT retried — F6)", n)
	}
}

// AS-8 (benign-503 guard, co-pilot requirement): a fast 503 during warmup IS
// retried (FR-R4), so the warmup completes with exactly 2 hits — pinning that
// warmup inherits the decorator's transient-retry, not only F6's no-retry.
func TestWarmup_transient503IsRetried(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "starting")
			return
		}
		writeOllamaChatOK(w)
	}))
	t.Cleanup(srv.Close)

	_, _, _ = runApp(t, oneLocalModelCfg(t, srv.URL, "2s", 1, true))
	waitHits(t, &hits, 2, 3*time.Second)
	// let it settle; must not exceed 2 (one retry, then success).
	time.Sleep(150 * time.Millisecond)
	if n := atomic.LoadInt32(&hits); n != 2 {
		t.Errorf("server hits = %d, want exactly 2 (fast 503 retried once, then success)", n)
	}
}

// AS-7 (dedup): two identical warmup models warm the backend exactly once.
func TestWarmup_dedupSameBackend(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		writeOllamaChatOK(w)
	}))
	t.Cleanup(srv.Close)

	_, _, _ = runApp(t, twoIdenticalWarmupModelsCfg(t, srv.URL))
	waitHits(t, &hits, 1, 3*time.Second)
	time.Sleep(150 * time.Millisecond)
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Errorf("server hits = %d, want exactly 1 (dedup by provider/baseURL/modelID)", n)
	}
}

// AS-6 (Shutdown-during-warmup race, co-pilot requirement): Shutdown fired while
// a warmup is still in flight cancels it and returns within its deadline, leaving
// no goroutine dangling. Run under -race (make quality).
func TestWarmup_shutdownCancelsInFlight(t *testing.T) {
	var hits int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })

	logger, _, _ := newCapturingLogger()
	ch := newFakeChannel("telegram")
	// A long request_timeout so the warmup is genuinely in flight at Shutdown.
	a, err := Build(oneLocalModelCfg(t, srv.URL, "30s", 0, true), WithLogger(logger), withChannelFactory(okFactory(ch)))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Warmup runs from Run's launch; drive it here since we called Start directly.
	// (In green the warmup is launched by Run/Start; this test drives Start then
	// waits for the in-flight warmup hit.)
	baseline := runtime.NumGoroutine()
	waitHits(t, &hits, 1, 3*time.Second) // warmup is now in flight (server parked on ctx.Done)

	done := make(chan error, 1)
	go func() {
		sctx, scancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer scancel()
		done <- a.Shutdown(sctx)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Shutdown = %v, want nil (warmup must not block Shutdown)", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("Shutdown did not return within 4s — an in-flight warmup blocked it")
	}
	// No goroutine left dangling (generous margin; poll for the warmup goroutine to unwind).
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > baseline+4 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if leaked := runtime.NumGoroutine() - baseline; leaked > 4 {
		t.Errorf("goroutines grew by %d after Shutdown, want ~0 (warmup goroutine leaked?)", leaked)
	}
}

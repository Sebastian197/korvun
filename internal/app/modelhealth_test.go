// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// N6 (bug-bash 2026-08-23): a model that does not answer at boot — the real
// "invalid model name" case — failed only as a WARN in the log; the UI had no
// health surface and the user discovered the breakage mid-chat. The warmup
// outcome must be OBSERVABLE: BrainSummaries carries a per-model health state
// ("unknown" | "warming" | "ready" | "unreachable") + a secret-free detail for
// the unreachable case, so /api/brains can feed the builder badge and the
// chat notice.

// waitModelHealth polls BrainSummaries until brain 0 / model 0 reaches want or
// the deadline expires, returning the last observed health.
func waitModelHealth(t *testing.T, a *App, want string, within time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(within)
	last := ""
	for time.Now().Before(deadline) {
		bs := a.BrainSummaries()
		if len(bs) > 0 && len(bs[0].Models) > 0 {
			last = bs[0].Models[0].Health
			if last == want {
				return last
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	return last
}

func TestModelHealth_warmupFailure_reportsUnreachable(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid model name"}`)
	}))
	t.Cleanup(srv.Close)

	a, _, _ := runApp(t, oneLocalModelCfg(t, srv.URL, "2s", 1, true))
	if got := waitModelHealth(t, a, "unreachable", 3*time.Second); got != "unreachable" {
		t.Fatalf("model health = %q, want unreachable", got)
	}
	detail := a.BrainSummaries()[0].Models[0].HealthDetail
	if !strings.Contains(detail, "invalid model name") {
		t.Errorf("health detail = %q, want the provider's error surfaced", detail)
	}
}

func TestModelHealth_warmupSuccess_reportsReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeOllamaChatOK(w)
	}))
	t.Cleanup(srv.Close)

	a, _, _ := runApp(t, oneLocalModelCfg(t, srv.URL, "2s", 1, true))
	if got := waitModelHealth(t, a, "ready", 3*time.Second); got != "ready" {
		t.Fatalf("model health = %q, want ready", got)
	}
	if detail := a.BrainSummaries()[0].Models[0].HealthDetail; detail != "" {
		t.Errorf("health detail = %q, want empty on a ready model", detail)
	}
}

func TestModelHealth_noWarmup_reportsUnknown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeOllamaChatOK(w)
	}))
	t.Cleanup(srv.Close)

	a, _, _ := runApp(t, oneLocalModelCfg(t, srv.URL, "2s", 1, false))
	// No probe ever runs: the health must be an honest "unknown", never an
	// invented "ready".
	if got := a.BrainSummaries()[0].Models[0].Health; got != "unknown" {
		t.Fatalf("model health = %q, want unknown (never probed)", got)
	}
}

func TestModelHealth_serializesForTheControlAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeOllamaChatOK(w)
	}))
	t.Cleanup(srv.Close)

	a, _, _ := runApp(t, oneLocalModelCfg(t, srv.URL, "2s", 1, true))
	if got := waitModelHealth(t, a, "ready", 3*time.Second); got != "ready" {
		t.Fatalf("model health = %q, want ready", got)
	}
	b, err := json.Marshal(a.BrainSummaries())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"health":"ready"`) {
		t.Errorf("summaries JSON lacks the health field: %s", b)
	}
	if strings.Contains(string(b), "health_detail") {
		t.Errorf("empty health_detail must be omitted from the JSON: %s", b)
	}
}

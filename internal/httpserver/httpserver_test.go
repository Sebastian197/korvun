// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package httpserver

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"
)

func quietLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

func getBody(t *testing.T, url string) (int, string) {
	t.Helper()
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get(url) //nolint:noctx // short-lived test request
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// TestHealthz_returns200 asserts the built-in liveness endpoint answers 200
// while the server is serving (ADR-0020 §4).
func TestHealthz_returns200(t *testing.T) {
	s := New("127.0.0.1:0", quietLogger())
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Shutdown(context.Background()) }()

	code, body := getBody(t, "http://"+s.Addr()+"/healthz")
	if code != http.StatusOK {
		t.Errorf("/healthz status = %d, want 200", code)
	}
	if body != "ok" {
		t.Errorf("/healthz body = %q, want %q", body, "ok")
	}
}

// TestHandle_servesRegisteredHandler asserts a handler registered before Start
// is served (the seam Stage 13 mounts on, ADR-0020 §4).
func TestHandle_servesRegisteredHandler(t *testing.T) {
	s := New("127.0.0.1:0", quietLogger())
	s.Handle("/metrics", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "metrics-payload")
	}))
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Shutdown(context.Background()) }()

	code, body := getBody(t, "http://"+s.Addr()+"/metrics")
	if code != http.StatusOK || body != "metrics-payload" {
		t.Errorf("/metrics = (%d, %q), want (200, %q)", code, body, "metrics-payload")
	}
}

// TestStart_reportsBindError asserts a bind failure (address already in use) is
// returned synchronously from Start, not swallowed in the serve goroutine — so
// the binary can fail loudly at boot (ADR-0017 §5 golden rule).
func TestStart_reportsBindError(t *testing.T) {
	first := New("127.0.0.1:0", quietLogger())
	if err := first.Start(context.Background()); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	defer func() { _ = first.Shutdown(context.Background()) }()

	second := New(first.Addr(), quietLogger())
	if err := second.Start(context.Background()); err == nil {
		_ = second.Shutdown(context.Background())
		t.Fatalf("second Start on a used address returned nil, want a bind error")
	}
}

// TestShutdown_stopsServing asserts that after Shutdown the server no longer
// answers, so App.Shutdown can rely on it releasing the port.
func TestShutdown_stopsServing(t *testing.T) {
	s := New("127.0.0.1:0", quietLogger())
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	addr := s.Addr()
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	client := &http.Client{Timeout: 200 * time.Millisecond}
	if resp, err := client.Get("http://" + addr + "/healthz"); err == nil {
		_ = resp.Body.Close()
		t.Errorf("server still answering after Shutdown")
	}
}

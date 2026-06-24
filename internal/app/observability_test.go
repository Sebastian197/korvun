// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/httpserver"
	"github.com/Sebastian197/korvun/internal/metrics"
	"github.com/Sebastian197/korvun/internal/router"
)

// TestBuild_observabilityOnByDefault asserts a config with no observability
// block boots WITH the admin server (the absent-is-on asymmetry, ADR-0020 §4),
// and that the domain metrics backend is the real Prometheus impl, not Nop.
func TestBuild_observabilityOnByDefault(t *testing.T) {
	app, err := Build(cfgWith(ollamaBrain()),
		WithLogger(slog.New(slog.DiscardHandler)),
		withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = app.Shutdown(ctx)
	})

	if app.adminServer == nil {
		t.Error("adminServer is nil, want a running admin server on by default")
	}
	if _, isNop := app.metrics.(metrics.Nop); isNop {
		t.Error("metrics backend is Nop, want the Prometheus impl when observability is on")
	}
}

// TestBuild_observabilityDisabled asserts an explicit enabled=false yields no
// admin server and a Nop metrics backend (the domain records nothing).
func TestBuild_observabilityDisabled(t *testing.T) {
	disabled := false
	cfg := cfgWith(ollamaBrain())
	cfg.Observability = &config.ObservabilityConfig{Enabled: &disabled}

	app, err := Build(cfg,
		WithLogger(slog.New(slog.DiscardHandler)),
		withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = app.Shutdown(ctx)
	})

	if app.adminServer != nil {
		t.Error("adminServer is non-nil, want nil when observability is disabled")
	}
	if _, isNop := app.metrics.(metrics.Nop); !isNop {
		t.Errorf("metrics backend = %T, want metrics.Nop when disabled", app.metrics)
	}
}

// TestRunShutdown_adminServerLifecycle asserts the admin server is up and
// answering /healthz while the app runs, and stops answering after Shutdown —
// the Start-first / Shutdown-last lifecycle (ADR-0020 §4).
func TestRunShutdown_adminServerLifecycle(t *testing.T) {
	srv := httpserver.New("127.0.0.1:0", slog.New(slog.DiscardHandler))
	a := &App{
		router:      router.New(),
		channels:    []Channel{newFakeChannel("telegram")},
		logger:      slog.New(slog.DiscardHandler),
		adminServer: srv,
		metrics:     metrics.Nop{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- a.Run(ctx) }()

	// Wait for the admin server to come up (Run starts it before channels).
	healthzURL := func() string { return "http://" + srv.Addr() + "/healthz" }
	if !waitFor(t, func() bool {
		if srv.Addr() == "" {
			return false
		}
		code, _ := tryGet(healthzURL())
		return code == http.StatusOK
	}) {
		t.Fatal("admin server never became healthy during Run")
	}

	cancel()
	if err := <-runDone; err != nil {
		t.Fatalf("Run returned %v, want nil on ctx cancel", err)
	}

	shutdownCtx, sc := context.WithTimeout(context.Background(), time.Second)
	defer sc()
	if err := a.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	client := &http.Client{Timeout: 200 * time.Millisecond}
	if resp, err := client.Get(healthzURL()); err == nil { //nolint:noctx // teardown probe
		_ = resp.Body.Close()
		t.Error("admin server still answering after Shutdown")
	}
}

func waitFor(t *testing.T, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func tryGet(url string) (int, string) {
	client := &http.Client{Timeout: 200 * time.Millisecond}
	resp, err := client.Get(url) //nolint:noctx // short-lived test probe
	if err != nil {
		return 0, ""
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

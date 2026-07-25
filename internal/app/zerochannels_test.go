// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/config"
)

// syncBuffer is a mutex-guarded bytes.Buffer so the capture handler can be
// written from any goroutine the app logs on.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestRun_zeroChannels_everythingElseAlive pins the NC-1 resolution (SP5,
// option B, 2026-07-25): a ZERO-channel config boots with everything else
// alive — admin server up, builder mounted (admin block + reloader), the
// status surface honestly reporting zero channels — warns loudly about the
// missing channels, and shuts down clean. -race carries the wiring proof.
func TestRun_zeroChannels_everythingElseAlive(t *testing.T) {
	const tokenEnv = "ZEROCHAN_TEST_ADMIN_TOKEN" //nolint:gosec // an env-var NAME, not a credential
	t.Setenv(tokenEnv, "zerochan-test-token")

	logs := &syncBuffer{}
	cfg := &config.Config{
		Channels:      []config.ChannelConfig{},
		Brains:        []config.BrainConfig{ollamaBrain()},
		Routes:        []config.RouteConfig{},
		Admin:         &config.AdminConfig{TokenEnv: tokenEnv},
		Observability: &config.ObservabilityConfig{Addr: "127.0.0.1:0"},
	}
	a, err := Build(cfg,
		WithLogger(slog.New(slog.NewTextHandler(logs, nil))),
		WithReloader(stubReloader{}),
	)
	if err != nil {
		t.Fatalf("Build with zero channels: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- a.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		sctx, sc := context.WithTimeout(context.Background(), 2*time.Second)
		defer sc()
		_ = a.Shutdown(sctx)
	})

	if !waitFor(t, func() bool {
		if a.adminServer == nil || a.adminServer.Addr() == "" {
			return false
		}
		code, _ := tryGet("http://" + a.adminServer.Addr() + "/healthz")
		return code == http.StatusOK
	}) {
		t.Fatal("admin server never became healthy on a zero-channel boot")
	}
	base := "http://" + a.adminServer.Addr()

	// The builder is mounted: the admin block did its job despite zero channels.
	resp, err := http.Get(base + "/builder/") //nolint:noctx // loopback test URL
	if err != nil {
		t.Fatalf("GET /builder/: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /builder/ = %d, want 200 (builder not mounted)", resp.StatusCode)
	}

	// The brains are alive alongside the empty channel list.
	brResp, err := http.Get(base + "/api/brains") //nolint:noctx // loopback test URL
	if err != nil {
		t.Fatalf("GET /api/brains: %v", err)
	}
	brBody, err := io.ReadAll(brResp.Body)
	_ = brResp.Body.Close()
	if err != nil {
		t.Fatalf("read /api/brains: %v", err)
	}
	if brResp.StatusCode != http.StatusOK || !strings.Contains(string(brBody), `"default"`) {
		t.Fatalf("GET /api/brains = %d %q, want 200 naming the default brain", brResp.StatusCode, brBody)
	}

	// The status surface is honest: zero channels, not an error.
	chResp, err := http.Get(base + "/api/channels") //nolint:noctx // loopback test URL
	if err != nil {
		t.Fatalf("GET /api/channels: %v", err)
	}
	chBody, err := io.ReadAll(chResp.Body)
	_ = chResp.Body.Close()
	if err != nil {
		t.Fatalf("read /api/channels: %v", err)
	}
	if chResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/channels = %d, want 200", chResp.StatusCode)
	}
	if s := strings.TrimSpace(string(chBody)); s != "[]" && !strings.Contains(s, `"channels":[]`) {
		t.Fatalf("GET /api/channels body = %q, want an empty channel list", s)
	}

	// The loud warning: an operator must learn WHY the gateway is deaf.
	if !strings.Contains(logs.String(), "no channels configured") {
		t.Fatalf("boot log does not warn about zero channels; log:\n%s", logs.String())
	}

	// Clean stop: Run returns nil on cancel, Shutdown drains with no error.
	cancel()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned %v, want nil on ctx cancel", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	sctx, sc := context.WithTimeout(context.Background(), 2*time.Second)
	defer sc()
	if err := a.Shutdown(sctx); err != nil {
		t.Fatalf("Shutdown after a zero-channel run: %v", err)
	}
}

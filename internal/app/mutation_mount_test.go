// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/supervisor"
)

// stubReloader satisfies controlapi.Reloader for the mount-decision test; its
// behavior does not matter here (we only check whether the route is mounted).
type stubReloader struct{}

func (stubReloader) RequestReload(*config.Config) (supervisor.Handle, error) {
	return "h", nil
}
func (stubReloader) Status(supervisor.Handle) supervisor.State { return supervisor.StateSucceeded }

// TestBuild_mutationMountedOnlyWithToken is the C4 load-bearing test of the ADR-0028
// default: with no admin token the mutation endpoint is NOT mounted (404) and the
// read-only surface is unchanged; with a resolvable token it is mounted and gated
// (401 without auth).
func TestBuild_mutationMountedOnlyWithToken(t *testing.T) {
	base := func(t *testing.T, admin *config.AdminConfig) string {
		t.Helper()
		cfg := cfgWith(ollamaBrain())
		cfg.Admin = admin
		cfg.Observability = &config.ObservabilityConfig{Addr: "127.0.0.1:0"} // ephemeral port
		a, err := Build(cfg,
			withChannelFactory(okFactory(newFakeChannel("telegram"))),
			WithReloader(stubReloader{}),
		)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		go func() { _ = a.Run(ctx) }()
		deadline := time.Now().Add(2 * time.Second)
		for a.adminServer.Addr() == "" && time.Now().Before(deadline) {
			time.Sleep(2 * time.Millisecond)
		}
		if a.adminServer.Addr() == "" {
			t.Fatal("admin server never bound")
		}
		t.Cleanup(func() {
			cancel()
			sctx, sc := context.WithTimeout(context.Background(), time.Second)
			defer sc()
			_ = a.Shutdown(sctx)
		})
		return "http://" + a.adminServer.Addr()
	}

	postCode := func(t *testing.T, url string) int {
		t.Helper()
		resp, err := http.Post(url, "application/json", nil) //nolint:gosec,noctx // loopback test URL
		if err != nil {
			t.Fatalf("POST %s: %v", url, err)
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode
	}
	getCode := func(t *testing.T, url string) int {
		t.Helper()
		resp, err := http.Get(url) //nolint:gosec,noctx // loopback test URL
		if err != nil {
			t.Fatalf("GET %s: %v", url, err)
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode
	}

	t.Run("no admin block: mutation not mounted, read-only intact", func(t *testing.T) {
		url := base(t, nil)
		if code := postCode(t, url+"/api/config"); code != http.StatusNotFound {
			t.Errorf("POST /api/config with no admin token: got %d, want 404 (not mounted)", code)
		}
		if code := getCode(t, url+"/api/brains"); code != http.StatusOK {
			t.Errorf("read-only /api/brains: got %d, want 200 (intact)", code)
		}
	})

	t.Run("admin token resolves: mutation mounted and gated", func(t *testing.T) {
		t.Setenv("KORVUN_ADMIN_MOUNT_TEST", "tok")
		url := base(t, &config.AdminConfig{TokenEnv: "KORVUN_ADMIN_MOUNT_TEST"})
		if code := postCode(t, url+"/api/config"); code != http.StatusUnauthorized {
			t.Errorf("POST /api/config with a token but no auth: got %d, want 401 (mounted + gated)", code)
		}
	})

	t.Run("admin token_env resolves empty: not mounted", func(t *testing.T) {
		url := base(t, &config.AdminConfig{TokenEnv: "KORVUN_ADMIN_UNSET_XYZ"})
		if code := postCode(t, url+"/api/config"); code != http.StatusNotFound {
			t.Errorf("POST /api/config with an empty token env: got %d, want 404 (not mounted)", code)
		}
	})
}

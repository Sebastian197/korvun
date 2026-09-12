// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/config"
)

// TestBuild_approvalsMountedWhereTheStoreIs is the wiring the whole train hangs
// from: until this passes, RegisterApprovals has no caller outside the tests and
// the screen paints «surface not mounted» against a real core.
//
// Two decisions are pinned here, and the second is the one that is easy to get
// wrong:
//
//   - the surface rides the SAME bearer as the rest of the write API, so a
//     profile without an admin token does not serve it at all;
//   - it is mounted even when approvals are DISABLED. A disabled profile has to
//     answer E3's 409 by name; refusing to mount would make the screen paint
//     «this window cannot find the approvals door», which is a different literal
//     with a different cure, and would send the operator to fix a profile block
//     that is not the one at fault.
func TestBuild_approvalsMountedWhereTheStoreIs(t *testing.T) {
	boot := func(t *testing.T, withStore, withApprovals bool) string {
		t.Helper()
		cfg := cfgWith(ollamaBrain())
		cfg.Admin = &config.AdminConfig{TokenEnv: "KORVUN_TEST_ADMIN"}
		cfg.Observability = &config.ObservabilityConfig{Addr: "127.0.0.1:0"}
		if withStore {
			cfg.Storage = &config.StorageConfig{Path: t.TempDir() + "/korvun.db"}
		}
		if withApprovals {
			cfg.Approvals = &config.ApprovalsConfig{Enabled: true}
		}
		t.Setenv("KORVUN_TEST_ADMIN", "s3cr3t")
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
	get := func(t *testing.T, base, token string) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, base+"/api/approvals", nil)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("do: %v", err)
		}
		defer func() { _ = res.Body.Close() }()
		return res.StatusCode
	}

	t.Run("no store: not mounted", func(t *testing.T) {
		base := boot(t, false, true)
		if got := get(t, base, "s3cr3t"); got != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 — with no action store there is nothing to serve", got)
		}
	})
	t.Run("store, approvals ON: mounted and gated", func(t *testing.T) {
		base := boot(t, true, true)
		if got := get(t, base, ""); got != http.StatusUnauthorized {
			t.Fatalf("status without a bearer = %d, want 401", got)
		}
		if got := get(t, base, "s3cr3t"); got != http.StatusOK {
			t.Fatalf("status with the bearer = %d, want 200", got)
		}
	})
	t.Run("store, approvals OFF: mounted, and it answers 409 by NAME", func(t *testing.T) {
		base := boot(t, true, false)
		if got := get(t, base, "s3cr3t"); got != http.StatusConflict {
			t.Fatalf("status = %d, want 409 — a disabled profile must not read as an absent door", got)
		}
	})
}

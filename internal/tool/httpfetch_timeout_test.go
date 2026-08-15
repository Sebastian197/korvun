// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package tool

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Estreno E-7 (performance/security/adversarial convergence): http_fetch had
// no intrinsic per-call bound — webhook_call carries DefaultWebhookTimeout —
// so a slow allow-listed host pinned the agent loop for the whole brain
// handler ceiling. The tool now owns a hard timeout with a house default.

func TestHTTPFetch_slowServerCutByOwnTimeout(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
	}))
	t.Cleanup(srv.Close)

	ft, err := HTTPFetch(HTTPFetchConfig{
		AllowHosts: []string{hostPortOf(t, srv)},
		Timeout:    150 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("HTTPFetch: %v", err)
	}
	start := time.Now()
	_, execErr := ft.Execute(context.Background(), srv.URL)
	if execErr == nil {
		t.Fatal("slow server: want a timeout error, got success")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Execute took %v; the tool's own timeout (150ms) did not cut it", elapsed)
	}
}

func TestHTTPFetch_defaultTimeoutConstant(t *testing.T) {
	t.Parallel()
	if DefaultFetchTimeout != 30*time.Second {
		t.Fatalf("DefaultFetchTimeout = %v, want 30s (house constant; webhook_call's tighter 10s fits its operator-endpoint POST, a page fetch needs headroom)", DefaultFetchTimeout)
	}
}

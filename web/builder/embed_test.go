// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package builderui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The embedded builder serves its index over HTTP with the strict same-origin CSP.
// This anchors two guarantees: (1) the go:embed pattern matched at least one file
// (a clean-clone build compiles and serves — ADR-0029 §4), and (2) every /builder
// response carries the CSP that is the real no-CDN gate (ADR-0029 §5).
func TestHandler_servesIndexWithCSP(t *testing.T) {
	h := http.StripPrefix("/builder", Handler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/builder/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /builder/: got %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Security-Policy"); !strings.Contains(got, "default-src 'self'") {
		t.Errorf("CSP header = %q, want it to contain default-src 'self'", got)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if rec.Body.Len() == 0 {
		t.Error("empty body — the embedded index did not serve")
	}
}

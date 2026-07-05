// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package builderui embeds the built Vite bundle (web/builder/dist) and serves it
// at /builder with a strict same-origin Content-Security-Policy (ADR-0029 §5 /
// ADR-0030 §4). The bundle is produced by `make build` (which runs the frontend
// build FIRST) and regenerated fresh in CI; a committed placeholder keeps
// `//go:embed dist` matching on a clean clone so a bare `go build` compiles
// (ADR-0029 §4). The package is a stdlib-only leaf — it adds zero Go dependencies,
// keeping the single-binary + go.mod-at-3 discipline intact.
package builderui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist
var distFS embed.FS

// csp is the same-origin Content-Security-Policy for /builder: the browser refuses
// any external script/style/font/image/connect BY CONSTRUCTION. This is the real
// no-CDN gate (ADR-0029 §5, not a text-grep) and it shrinks the XSS surface that
// makes the in-memory bearer defensible (ADR-0030 §6).
const csp = "default-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"

// Handler serves the embedded builder SPA with the CSP header. Mount it with
// http.StripPrefix("/builder", ...) at the pattern "GET /builder/". Callers mount it
// ONLY when an admin token is configured (ADR-0030 §4): a builder that cannot save
// is a trap.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// The embed is resolved at compile time; a Sub failure is a programming
		// error, not a runtime condition. Fail loud with a 500, never panic on a
		// normal request path (CLAUDE.md).
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "builder assets unavailable", http.StatusInternalServerError)
		})
	}
	files := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", csp)
		files.ServeHTTP(w, r)
	})
}

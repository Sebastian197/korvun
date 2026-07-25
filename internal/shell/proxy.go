// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package shell

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// ProxyHandler returns the desktop asset seam (ADR-0035 §3b, SP4 design
// spec): an http.Handler cmd/korvun-desktop mounts as the Wails
// assetserver.Options.Handler. It reverse-proxies the core's admin surface
// (/api/*, /builder/*, /ui/*, /healthz, /metrics) to the CURRENT cycle's
// effective admin address — resolved per request, never cached across
// cycles — injecting the per-cycle admin bearer server-side so the token
// never enters the DOM (ADR-0035 §4). With the core stopped the proxied
// routes answer a stable honest 503 (`{"error":"core stopped"}`) the shell
// chrome can render, and any other path answers a small 404: the shell's
// own assets are the AssetServer's Assets side, never this handler's job
// (ADR-0035 §3c).
func (c *Controller) ProxyHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isAdminPath(r.URL.Path) {
			writeJSONError(w, http.StatusNotFound, "not found")
			return
		}
		addr, bearer := c.proxyTarget()
		if addr == "" {
			writeJSONError(w, http.StatusServiceUnavailable, "core stopped")
			return
		}
		proxyForTarget(addr, bearer).ServeHTTP(w, r)
	})
}

// proxyTarget resolves the CURRENT cycle's admin address and bearer under
// the controller lock (through the same reap path as Status, so a core that
// died on its own reads as stopped). Both are "" when there is nothing to
// forward to — stopped, mid-cutover, or observability disabled.
func (c *Controller) proxyTarget() (addr, bearer string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reapLocked()
	if !c.running {
		return "", ""
	}
	a := c.cur.Load()
	if a == nil {
		return "", ""
	}
	return a.AdminAddr(), c.bearer
}

// proxyForTarget builds the reverse proxy for one resolved cycle target —
// the unit seam the streaming and dead-upstream tests drive directly. The
// negative FlushInterval flushes after every write, so the SSE live-view
// arrives incrementally (FR-PXY-3, the SP1 spike's verdict held by test).
// Honesty note: on the real Wails mount the AssetServer's ResponseWriter is
// not an http.Flusher, so the per-write flush is a no-op there and the
// incremental delivery rides the platform writers instead (the SP1 spike
// proved it live on WKWebView); the SP8 hardware validation re-checks the
// live view end to end.
// The bearer, when present, OVERWRITES any client-supplied Authorization —
// a page can neither leak nor forge it; without a bearer the header is
// stripped so the page's headers never reach the core as credentials.
func proxyForTarget(addr, bearer string) *httputil.ReverseProxy {
	target := &url.URL{Scheme: "http", Host: addr}
	return &httputil.ReverseProxy{
		FlushInterval: -1,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			if bearer != "" {
				pr.Out.Header.Set("Authorization", "Bearer "+bearer)
			} else {
				pr.Out.Header.Del("Authorization")
			}
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			writeJSONError(w, http.StatusServiceUnavailable, "core unreachable")
		},
	}
}

// isAdminPath reports whether p belongs to the proxied admin surface:
// exact /healthz and /metrics, plus the /api, /builder and /ui subtrees
// (segment-aware — /uix is not /ui).
func isAdminPath(p string) bool {
	switch p {
	case "/healthz", "/metrics":
		return true
	}
	for _, root := range [3]string{"/api", "/builder", "/ui"} {
		if p == root || strings.HasPrefix(p, root+"/") {
			return true
		}
	}
	return false
}

// writeJSONError writes the proxy's small stable JSON error contract —
// two 503 reasons ("core stopped" / "core unreachable") and the 404, the
// bodies SP6's chrome switches on.
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, "{\"error\":%q}", msg)
}

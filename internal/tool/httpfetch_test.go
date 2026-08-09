// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package tool

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// The http_fetch cage (ADR-0041 §3/§4, mandate SP3.2): GET only, exact-host
// allow-list (case-insensitive, optional port), hard response cap, redirects
// only to listed hosts with a hop cap, and — under the shield — the dial-time
// private-address check where rebinding and redirects die at the socket.

// countingServer is an httptest server that counts the requests it receives,
// so "nothing contacted" is assertable, not assumed.
func countingServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// hostPortOf extracts "host:port" from an httptest server URL.
func hostPortOf(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	return u.Host
}

func mustHTTPFetch(t *testing.T, cfg HTTPFetchConfig) Tool {
	t.Helper()
	f, err := HTTPFetch(cfg)
	if err != nil {
		t.Fatalf("HTTPFetch(%+v): %v", cfg, err)
	}
	return f
}

func TestHTTPFetch_getsAnAllowedHost(t *testing.T) {
	t.Parallel()
	srv, hits := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_, _ = fmt.Fprint(w, "fetched body")
	})
	f := mustHTTPFetch(t, HTTPFetchConfig{AllowHosts: []string{hostPortOf(t, srv)}})

	got, err := f.Execute(context.Background(), srv.URL+"/page")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got != "fetched body" {
		t.Fatalf("got %q, want the body", got)
	}
	if hits.Load() != 1 {
		t.Fatalf("server hit %d times, want 1", hits.Load())
	}
}

func TestHTTPFetch_hostOffTheListDiesWithoutContact(t *testing.T) {
	t.Parallel()
	srv, hits := countingServer(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, "x") })
	f := mustHTTPFetch(t, HTTPFetchConfig{AllowHosts: []string{"allowed.example"}})

	_, err := f.Execute(context.Background(), srv.URL+"/page")
	if !errors.Is(err, ErrCageViolation) {
		t.Fatalf("err = %v, want errors.Is(_, ErrCageViolation)", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("off-list server was contacted %d times, want 0", hits.Load())
	}
}

func TestHTTPFetch_allowListIsCaseInsensitive(t *testing.T) {
	t.Parallel()
	srv, _ := countingServer(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, "ok") })
	hp := hostPortOf(t, srv)
	f := mustHTTPFetch(t, HTTPFetchConfig{AllowHosts: []string{strings.ToUpper(hp)}})

	if _, err := f.Execute(context.Background(), srv.URL); err != nil {
		t.Fatalf("Execute with a case-differing allow-list entry: %v", err)
	}
}

func TestHTTPFetch_responseCapDiesAtTheCage(t *testing.T) {
	t.Parallel()
	srv, _ := countingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, strings.Repeat("x", 100))
	})
	f := mustHTTPFetch(t, HTTPFetchConfig{AllowHosts: []string{hostPortOf(t, srv)}, MaxBytes: 10})

	_, err := f.Execute(context.Background(), srv.URL)
	if !errors.Is(err, ErrCageViolation) {
		t.Fatalf("err = %v, want errors.Is(_, ErrCageViolation) — cap not enforced", err)
	}
}

func TestHTTPFetch_redirectToListedHostFollows(t *testing.T) {
	t.Parallel()
	target, targetHits := countingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "redirected body")
	})
	origin, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	})
	f := mustHTTPFetch(t, HTTPFetchConfig{
		AllowHosts: []string{hostPortOf(t, origin), hostPortOf(t, target)},
	})

	got, err := f.Execute(context.Background(), origin.URL)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got != "redirected body" {
		t.Fatalf("got %q, want the redirected body", got)
	}
	if targetHits.Load() != 1 {
		t.Fatalf("target hit %d times, want 1", targetHits.Load())
	}
}

func TestHTTPFetch_redirectOffTheListDiesWithoutContact(t *testing.T) {
	t.Parallel()
	leak, leakHits := countingServer(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, "x") })
	origin, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, leak.URL, http.StatusFound)
	})
	f := mustHTTPFetch(t, HTTPFetchConfig{AllowHosts: []string{hostPortOf(t, origin)}})

	_, err := f.Execute(context.Background(), origin.URL)
	if !errors.Is(err, ErrCageViolation) {
		t.Fatalf("err = %v, want errors.Is(_, ErrCageViolation) — off-list redirect followed", err)
	}
	if leakHits.Load() != 0 {
		t.Fatalf("off-list redirect target contacted %d times, want 0", leakHits.Load())
	}
}

func TestHTTPFetch_redirectHopCap(t *testing.T) {
	t.Parallel()
	var srv *httptest.Server
	srv, _ = countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL, http.StatusFound) // redirect loop to itself
	})
	f := mustHTTPFetch(t, HTTPFetchConfig{AllowHosts: []string{hostPortOf(t, srv)}, MaxRedirects: 2})

	_, err := f.Execute(context.Background(), srv.URL)
	if !errors.Is(err, ErrCageViolation) {
		t.Fatalf("err = %v, want errors.Is(_, ErrCageViolation) — hop cap not enforced", err)
	}
}

func TestHTTPFetch_nonHTTPSchemeDiesAtTheCage(t *testing.T) {
	t.Parallel()
	f := mustHTTPFetch(t, HTTPFetchConfig{AllowHosts: []string{"example.com"}})

	_, err := f.Execute(context.Background(), "file:///etc/passwd")
	if !errors.Is(err, ErrCageViolation) {
		t.Fatalf("err = %v, want errors.Is(_, ErrCageViolation) — non-http scheme allowed", err)
	}
}

func TestHTTPFetch_httpErrorStatusIsOrdinaryError(t *testing.T) {
	t.Parallel()
	srv, _ := countingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	})
	f := mustHTTPFetch(t, HTTPFetchConfig{AllowHosts: []string{hostPortOf(t, srv)}})

	_, err := f.Execute(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("Execute succeeded on a 404")
	}
	if errors.Is(err, ErrCageViolation) || errors.Is(err, ErrShieldViolation) {
		t.Fatalf("HTTP 404 misclassified as a cage/shield violation: %v", err)
	}
}

// THE SHIELD AT THE DIAL (mandate SP3.2): under PrivateOnly, a PUBLIC address
// — even one ON the allow-list — dies at the dial-time resolved-IP check with
// nothing contacted; a private address on the list executes normally.
func TestHTTPFetch_shieldBeatsAllowListAtTheDial(t *testing.T) {
	t.Parallel()
	// 203.0.113.7 is TEST-NET-3 (never routable); the literal-IP URL keeps
	// DNS out of the test. The allow-list EXPLICITLY permits it — the shield
	// must win anyway (spec AS-10).
	f := mustHTTPFetch(t, HTTPFetchConfig{
		AllowHosts:  []string{"203.0.113.7"},
		PrivateOnly: true,
	})

	_, err := f.Execute(context.Background(), "http://203.0.113.7/x")
	if !errors.Is(err, ErrShieldViolation) {
		t.Fatalf("err = %v, want errors.Is(_, ErrShieldViolation)", err)
	}
}

func TestHTTPFetch_shieldAllowsPrivateAddresses(t *testing.T) {
	t.Parallel()
	// httptest binds 127.0.0.1 — loopback is private, so the shield lets it
	// through and the allow-list still applies (spec AS-11).
	srv, hits := countingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "private ok")
	})
	f := mustHTTPFetch(t, HTTPFetchConfig{
		AllowHosts:  []string{hostPortOf(t, srv)},
		PrivateOnly: true,
	})

	got, err := f.Execute(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Execute under shield to loopback: %v", err)
	}
	if got != "private ok" || hits.Load() != 1 {
		t.Fatalf("got %q (hits %d), want the body with exactly one hit", got, hits.Load())
	}
}

func TestHTTPFetch_constructionFailsLoudWithoutAllowList(t *testing.T) {
	t.Parallel()
	if _, err := HTTPFetch(HTTPFetchConfig{}); err == nil {
		t.Fatal("HTTPFetch with no allow-list must fail")
	}
	if _, err := HTTPFetch(HTTPFetchConfig{AllowHosts: []string{"  "}}); err == nil {
		t.Fatal("HTTPFetch with a blank allow-list entry must fail")
	}
}

func TestHTTPFetch_identity(t *testing.T) {
	t.Parallel()
	f := mustHTTPFetch(t, HTTPFetchConfig{AllowHosts: []string{"example.com"}})
	if f.Name() != "http_fetch" {
		t.Fatalf("Name() = %q, want http_fetch", f.Name())
	}
	if f.Description() == "" {
		t.Fatal("Description() empty")
	}
}

// THE ATTRS TRIPWIRE (spec SP3 rider): http_fetch MUST declare Network=true —
// a forgotten declaration would silently bypass the shield.
func TestBuiltinAttrs_httpFetchIsNetworkClassed(t *testing.T) {
	t.Parallel()
	a, ok := BuiltinAttrs("http_fetch")
	if !ok {
		t.Fatal("BuiltinAttrs does not know http_fetch")
	}
	if !a.Network {
		t.Fatal("http_fetch MUST be Network-classed (ADR-0041 §4, R-1)")
	}
	if a.Sensitive {
		t.Fatal("http_fetch is not sensitive by house default (R-2)")
	}
}

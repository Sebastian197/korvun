// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package tool

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

// The webhook_call cage (ADR-0041 §4, mandate SP3.3): POST JSON to
// allow-listed hosts (same allow-list + shield semantics as http_fetch),
// response cap, hard timeout, and NO redirects — the user's no-code tool
// factory and the n8n door.

func mustWebhookCall(t *testing.T, cfg WebhookCallConfig) Tool {
	t.Helper()
	w, err := WebhookCall(cfg)
	if err != nil {
		t.Fatalf("WebhookCall(%+v): %v", cfg, err)
	}
	return w
}

func TestWebhookCall_postsJSONToAnAllowedHost(t *testing.T) {
	t.Parallel()
	srv, hits := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"event":"ping"}` {
			t.Errorf("body = %s, want the JSON payload verbatim", body)
		}
		_, _ = fmt.Fprint(w, "ack")
	})
	wc := mustWebhookCall(t, WebhookCallConfig{AllowHosts: []string{hostPortOf(t, srv)}})

	got, err := wc.Execute(context.Background(), srv.URL+` {"event":"ping"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got != "ack" {
		t.Fatalf("got %q, want the response body", got)
	}
	if hits.Load() != 1 {
		t.Fatalf("server hit %d times, want 1", hits.Load())
	}
}

func TestWebhookCall_hostOffTheListDiesWithoutContact(t *testing.T) {
	t.Parallel()
	srv, hits := countingServer(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, "x") })
	wc := mustWebhookCall(t, WebhookCallConfig{AllowHosts: []string{"allowed.example"}})

	_, err := wc.Execute(context.Background(), srv.URL+` {"a":1}`)
	if !errors.Is(err, ErrCageViolation) {
		t.Fatalf("err = %v, want errors.Is(_, ErrCageViolation)", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("off-list server contacted %d times, want 0", hits.Load())
	}
}

func TestWebhookCall_invalidJSONIsOrdinaryError(t *testing.T) {
	t.Parallel()
	wc := mustWebhookCall(t, WebhookCallConfig{AllowHosts: []string{"allowed.example"}})

	_, err := wc.Execute(context.Background(), "http://allowed.example/hook not-json")
	if err == nil {
		t.Fatal("Execute succeeded with a non-JSON body")
	}
	if errors.Is(err, ErrCageViolation) {
		t.Fatalf("invalid JSON misclassified as a cage violation: %v", err)
	}
}

func TestWebhookCall_missingBodyIsOrdinaryError(t *testing.T) {
	t.Parallel()
	wc := mustWebhookCall(t, WebhookCallConfig{AllowHosts: []string{"allowed.example"}})

	_, err := wc.Execute(context.Background(), "http://allowed.example/hook")
	if err == nil {
		t.Fatal("Execute succeeded without a body")
	}
	if errors.Is(err, ErrCageViolation) {
		t.Fatalf("missing body misclassified as a cage violation: %v", err)
	}
}

func TestWebhookCall_responseCapDiesAtTheCage(t *testing.T) {
	t.Parallel()
	srv, _ := countingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		for i := 0; i < 100; i++ {
			_, _ = fmt.Fprint(w, "x")
		}
	})
	wc := mustWebhookCall(t, WebhookCallConfig{AllowHosts: []string{hostPortOf(t, srv)}, MaxBytes: 10})

	_, err := wc.Execute(context.Background(), srv.URL+` {"a":1}`)
	if !errors.Is(err, ErrCageViolation) {
		t.Fatalf("err = %v, want errors.Is(_, ErrCageViolation) — cap not enforced", err)
	}
}

// A webhook POST never follows a redirect: the first hop dies at the cage
// (a redirected POST is how a listed host smuggles the payload elsewhere).
func TestWebhookCall_redirectDiesAtTheCage(t *testing.T) {
	t.Parallel()
	leak, leakHits := countingServer(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, "x") })
	origin, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, leak.URL, http.StatusTemporaryRedirect)
	})
	wc := mustWebhookCall(t, WebhookCallConfig{
		AllowHosts: []string{hostPortOf(t, origin), hostPortOf(t, leak)},
	})

	_, err := wc.Execute(context.Background(), origin.URL+` {"a":1}`)
	if !errors.Is(err, ErrCageViolation) {
		t.Fatalf("err = %v, want errors.Is(_, ErrCageViolation) — redirect followed", err)
	}
	if leakHits.Load() != 0 {
		t.Fatalf("redirect target contacted %d times, want 0", leakHits.Load())
	}
}

// The hard timeout: a slow endpoint fails as an ordinary error within the
// configured bound, never hanging the loop.
func TestWebhookCall_hardTimeout(t *testing.T) {
	t.Parallel()
	srv, _ := countingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		_, _ = fmt.Fprint(w, "late")
	})
	wc := mustWebhookCall(t, WebhookCallConfig{
		AllowHosts: []string{hostPortOf(t, srv)},
		Timeout:    50 * time.Millisecond,
	})

	start := time.Now()
	_, err := wc.Execute(context.Background(), srv.URL+` {"a":1}`)
	if err == nil {
		t.Fatal("Execute succeeded against a slow endpoint under a hard timeout")
	}
	if errors.Is(err, ErrCageViolation) || errors.Is(err, ErrShieldViolation) {
		t.Fatalf("timeout misclassified as a cage/shield violation: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("timeout took %v, want well under the endpoint's delay ceiling", elapsed)
	}
}

// Same shield semantics as http_fetch (spec AS-10/AS-11).
func TestWebhookCall_shieldBeatsAllowListAtTheDial(t *testing.T) {
	t.Parallel()
	wc := mustWebhookCall(t, WebhookCallConfig{
		AllowHosts:  []string{"203.0.113.7"},
		PrivateOnly: true,
	})

	_, err := wc.Execute(context.Background(), `http://203.0.113.7/hook {"a":1}`)
	if !errors.Is(err, ErrShieldViolation) {
		t.Fatalf("err = %v, want errors.Is(_, ErrShieldViolation)", err)
	}
}

func TestWebhookCall_shieldAllowsPrivateAddresses(t *testing.T) {
	t.Parallel()
	srv, hits := countingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "private ack")
	})
	wc := mustWebhookCall(t, WebhookCallConfig{
		AllowHosts:  []string{hostPortOf(t, srv)},
		PrivateOnly: true,
	})

	got, err := wc.Execute(context.Background(), srv.URL+` {"a":1}`)
	if err != nil {
		t.Fatalf("Execute under shield to loopback: %v", err)
	}
	if got != "private ack" || hits.Load() != 1 {
		t.Fatalf("got %q (hits %d), want the ack with exactly one hit", got, hits.Load())
	}
}

func TestWebhookCall_emptyResponseReportsStatus(t *testing.T) {
	t.Parallel()
	srv, _ := countingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	wc := mustWebhookCall(t, WebhookCallConfig{AllowHosts: []string{hostPortOf(t, srv)}})

	got, err := wc.Execute(context.Background(), srv.URL+` {"a":1}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got != "HTTP 204" {
		t.Fatalf("got %q, want the status marker for an empty response", got)
	}
}

func TestWebhookCall_constructionFailsLoudWithoutAllowList(t *testing.T) {
	t.Parallel()
	if _, err := WebhookCall(WebhookCallConfig{}); err == nil {
		t.Fatal("WebhookCall with no allow-list must fail")
	}
}

func TestWebhookCall_identity(t *testing.T) {
	t.Parallel()
	wc := mustWebhookCall(t, WebhookCallConfig{AllowHosts: []string{"example.com"}})
	if wc.Name() != "webhook_call" {
		t.Fatalf("Name() = %q, want webhook_call", wc.Name())
	}
	if wc.Description() == "" {
		t.Fatal("Description() empty")
	}
}

// THE ATTRS TRIPWIRE (spec SP3 rider): webhook_call MUST declare Network=true.
func TestBuiltinAttrs_webhookCallIsNetworkClassed(t *testing.T) {
	t.Parallel()
	a, ok := BuiltinAttrs("webhook_call")
	if !ok {
		t.Fatal("BuiltinAttrs does not know webhook_call")
	}
	if !a.Network {
		t.Fatal("webhook_call MUST be Network-classed (ADR-0041 §4, R-1)")
	}
	if a.Sensitive {
		t.Fatal("webhook_call is not sensitive by house default (R-2)")
	}
}

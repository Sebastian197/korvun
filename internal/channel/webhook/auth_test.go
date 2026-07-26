// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// This file is the SP3 TDD contract for inbound authentication + edge validation
// (ADR-0038 §§3,6). RED-first: it references the seams the implementation must add —
// the unexported authGate(next) middleware and the secretsMatch comparison seam
// (sha256 + subtle.ConstantTimeCompare) — neither of which exists yet, so the webhook
// test package does not compile until they land. Start will mount authGate IN FRONT
// of the existing InboundHandler; the only InboundHandler change in SP3 is
// distinguishing an oversized body (*http.MaxBytesError → 413) from an unreadable one
// (400). Reuses newTestAdapter / testSecret / postJSON / defaultMapping from the
// SP2/Stage-2 test files (same package).

// countingReader counts Read calls so a test can prove the request body was never
// touched on an auth rejection. Single-goroutine on the authGate path, so a plain int
// is race-free here.
type countingReader struct {
	reads int
	data  []byte
	pos   int
}

func (c *countingReader) Read(p []byte) (int, error) {
	c.reads++
	if c.pos >= len(c.data) {
		return 0, context.Canceled // any non-nil, non-io.EOF sentinel; never reached in these tests
	}
	n := copy(p, c.data[c.pos:])
	c.pos += n
	return n, nil
}

// spyHandler records whether it was reached and answers 200 so a "reached the next
// handler" outcome is observable through the recorder.
func spyHandler(reached *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*reached = true
		w.WriteHeader(http.StatusOK)
	})
}

// TestAuthGate_seam pins ADR-0038 §3: authGate lets a correct Bearer through to next,
// and rejects everything else with 401 WITHOUT invoking next.
func TestAuthGate_seam(t *testing.T) {
	cases := []struct {
		name       string
		authHeader string // "" means the header is absent
		wantStatus int
		wantNext   bool
	}{
		{name: "correct bearer reaches next", authHeader: "Bearer " + testSecret, wantStatus: http.StatusOK, wantNext: true},
		{name: "wrong secret", authHeader: "Bearer wrong-secret", wantStatus: http.StatusUnauthorized, wantNext: false},
		{name: "absent header", authHeader: "", wantStatus: http.StatusUnauthorized, wantNext: false},
		{name: "bearer without token", authHeader: "Bearer ", wantStatus: http.StatusUnauthorized, wantNext: false},
		{name: "bare bearer word", authHeader: "Bearer", wantStatus: http.StatusUnauthorized, wantNext: false},
		{name: "different scheme", authHeader: "Basic " + testSecret, wantStatus: http.StatusUnauthorized, wantNext: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestAdapter(t, "/hook")
			reached := false
			gate := a.authGate(spyHandler(&reached))

			req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader([]byte(`{}`)))
			// A valid Content-Type so the happy case reaches next; the 401 rows
			// short-circuit on auth before the content-type check either way.
			req.Header.Set("Content-Type", "application/json")
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rec := httptest.NewRecorder()
			gate.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if reached != tc.wantNext {
				t.Errorf("next reached = %v, want %v", reached, tc.wantNext)
			}
		})
	}
}

// TestAuthGate_failClosed pins the fail-closed rule: an adapter with an EMPTY secret
// rejects every request with 401, no matter what header it carries (a
// misconfigured/unresolved secret must never leave the endpoint open).
func TestAuthGate_failClosed(t *testing.T) {
	a := NewWithOptions("test-webhook", Options{
		Bind:        "127.0.0.1:0",
		Path:        "/hook",
		Secret:      "", // unresolved
		OutboundURL: "https://downstream.example/in",
		Mapping:     defaultMapping(),
	})
	reached := false
	gate := a.authGate(spyHandler(&reached))

	for _, hdr := range []string{"", "Bearer ", "Bearer anything", "Bearer "} {
		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader([]byte(`{}`)))
		if hdr != "" {
			req.Header.Set("Authorization", hdr)
		}
		rec := httptest.NewRecorder()
		gate.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("empty-secret adapter, header %q: status = %d, want 401", hdr, rec.Code)
		}
	}
	if reached {
		t.Error("next handler was reached despite an empty secret (not fail-closed)")
	}
}

// TestAuthGate_bodyNotReadOnReject pins that a rejected request's body is never read:
// authGate must decide on the header alone, before touching the body.
func TestAuthGate_bodyNotReadOnReject(t *testing.T) {
	a := newTestAdapter(t, "/hook")
	reached := false
	gate := a.authGate(spyHandler(&reached))

	cr := &countingReader{data: []byte(`{"sender_id":"u","text":"hi"}`)}
	req := httptest.NewRequest(http.MethodPost, "/hook", cr)
	req.Header.Set("Authorization", "Bearer wrong-secret")
	rec := httptest.NewRecorder()
	gate.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if cr.reads != 0 {
		t.Errorf("request body was read %d times on a 401, want 0", cr.reads)
	}
	if reached {
		t.Error("next handler reached on a rejected request")
	}
}

// TestSecretsMatch_mechanism pins the comparison seam (ADR-0038 §3, ADR-0028 §1):
// secretsMatch hashes both sides (sha256) and compares in constant time
// (subtle.ConstantTimeCompare). Behavior is fixed by calling the seam directly — not
// by timing: equal secrets match; different secrets do not; and unequal-LENGTH inputs
// compare cleanly to false (no panic) because the fixed-length hashes normalize length.
func TestSecretsMatch_mechanism(t *testing.T) {
	cases := []struct {
		name      string
		got, want string
		match     bool
	}{
		{name: "equal", got: "s3cr3t-inbound", want: "s3cr3t-inbound", match: true},
		{name: "mismatch same length", got: "s3cr3t-inboundX", want: "s3cr3t-inboundY", match: false},
		{name: "mismatch different length", got: "short", want: "a-much-longer-secret-value", match: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := secretsMatch(tc.got, tc.want); got != tc.match {
				t.Errorf("secretsMatch(%q, %q) = %v, want %v", tc.got, tc.want, got, tc.match)
			}
		})
	}
}

// TestBorderValidation_server pins ADR-0038 §6 over the REAL server (ephemeral bind,
// SP2 style): every request is AUTHENTICATED (edge validation is downstream of auth),
// and each row asserts the HTTP status and that a rejected request enqueues nothing.
func TestBorderValidation_server(t *testing.T) {
	oversized := bytes.Repeat([]byte("a"), (1<<20)+1) // 1 MiB + 1 byte

	cases := []struct {
		name         string
		method       string
		contentType  string
		body         []byte
		wantStatus   int
		wantEnvelope bool
	}{
		{name: "GET is rejected", method: http.MethodGet, contentType: "", body: nil, wantStatus: http.StatusMethodNotAllowed},
		{name: "missing content-type", method: http.MethodPost, contentType: "", body: []byte(`{"sender_id":"u","text":"hi"}`), wantStatus: http.StatusUnsupportedMediaType},
		{name: "text/plain content-type", method: http.MethodPost, contentType: "text/plain", body: []byte(`{"sender_id":"u","text":"hi"}`), wantStatus: http.StatusUnsupportedMediaType},
		{name: "json with charset passes", method: http.MethodPost, contentType: "application/json; charset=utf-8", body: []byte(`{"sender_id":"u","text":"hi"}`), wantStatus: http.StatusOK, wantEnvelope: true},
		{name: "oversized body", method: http.MethodPost, contentType: "application/json", body: oversized, wantStatus: http.StatusRequestEntityTooLarge},
		{name: "malformed json", method: http.MethodPost, contentType: "application/json", body: []byte("not json"), wantStatus: http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestAdapter(t, "/hook")
			if err := a.Start(context.Background()); err != nil {
				t.Fatalf("Start() error: %v", err)
			}
			defer func() { _ = a.Stop(context.Background()) }()

			url := "http://" + a.BoundAddr() + "/hook"
			var bodyReader *bytes.Reader
			if tc.body != nil {
				bodyReader = bytes.NewReader(tc.body)
			} else {
				bodyReader = bytes.NewReader(nil)
			}
			req, err := http.NewRequest(tc.method, url, bodyReader)
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			req.Header.Set("Authorization", "Bearer "+testSecret)
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}
			resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tc.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}

			if tc.wantEnvelope {
				select {
				case <-a.Inbound():
				case <-time.After(2 * time.Second):
					t.Error("expected an Envelope on Inbound(), got none")
				}
				return
			}
			// A rejected request must enqueue nothing.
			select {
			case env := <-a.Inbound():
				t.Errorf("rejected request enqueued an Envelope: %+v", env)
			case <-time.After(200 * time.Millisecond):
			}
		})
	}
}

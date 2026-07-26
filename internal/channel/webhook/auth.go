// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"crypto/sha256"
	"crypto/subtle"
	"log/slog"
	"mime"
	"net/http"
	"strings"
)

// maxBodyBytes caps an inbound request body at 1 MiB (ADR-0038 §6): a generous floor
// for JSON control payloads and a cheap DoS guard. Enforced via http.MaxBytesReader,
// whose overflow error the InboundHandler maps to 413.
const maxBodyBytes = 1 << 20

// secretsMatch reports whether two secrets are equal, comparing in constant time over
// their SHA-256 digests (ADR-0028 §1, ADR-0038 §3). Hashing to a fixed length means
// the length-mismatch early-return of subtle.ConstantTimeCompare never leaks the
// secret's length, and unequal-length inputs compare cleanly to false.
func secretsMatch(got, want string) bool {
	g := sha256.Sum256([]byte(got))
	w := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(g[:], w[:]) == 1
}

// authGate wraps next with the inbound-authentication and edge-validation middleware
// (ADR-0038 §§3,6). It runs BEFORE the payload translator so a rejected request never
// reaches the InboundHandler and its body is never read on a 401. Order:
//
//	a) method must be POST → else 405 (nothing else is inspected).
//	b) FAIL-CLOSED: an empty resolved secret → 401 WITHOUT calling secretsMatch
//	   (two empty-string hashes would match; the cut must come first).
//	c) Authorization must be "Bearer <token>" with a non-empty token whose value
//	   matches the secret → else 401. On every 401 the body is untouched and a slog
//	   Warn records the channel and remote address — NEVER the secret nor the header.
//	d) Content-Type must parse (mime.ParseMediaType) to exactly "application/json"
//	   → else 415 (an absent or unparseable type is 415; the charset parameter rides
//	   separately, so "application/json; charset=utf-8" passes).
//	e) the body is capped with http.MaxBytesReader(maxBodyBytes) before next runs.
func (a *Adapter) authGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// (b) fail-closed, then (c) Bearer scheme + constant-time secret match.
		if a.secret == "" || !bearerMatches(r.Header.Get("Authorization"), a.secret) {
			a.rejectUnauthorized(w, r)
			return
		}

		// (d) content type must be exactly application/json (charset parameter allowed).
		mediatype, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediatype != "application/json" {
			http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
			return
		}

		// (e) cap the body before the translator reads it.
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next.ServeHTTP(w, r)
	})
}

// bearerMatches reports whether the Authorization header is a well-formed
// "Bearer <token>" whose token matches secret in constant time. A missing header, a
// different scheme, or an empty token all return false. secret is assumed non-empty
// (the fail-closed cut in authGate runs first).
func bearerMatches(header, secret string) bool {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	token := strings.TrimPrefix(header, prefix)
	if token == "" {
		return false
	}
	return secretsMatch(token, secret)
}

// rejectUnauthorized answers 401 without touching the request body and logs the
// rejection with the channel and remote address only — never the secret nor the
// received Authorization header (ADR-0010 / ADR-0038 §3).
func (a *Adapter) rejectUnauthorized(w http.ResponseWriter, r *http.Request) {
	slog.Default().WarnContext(r.Context(),
		"webhook: inbound request rejected (unauthorized)",
		"channel", a.name, "remote_addr", r.RemoteAddr)
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

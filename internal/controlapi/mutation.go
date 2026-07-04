// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// This file adds the WRITE surface of the control API (Stage 14 Phase 2a): a
// bearer-gated config-mutation endpoint plus a read-only reload-status endpoint.
// It is mounted ONLY when a bearer token is configured (ADR-0028 §1: no token =>
// mutation not mounted, the read-only default). The gate wraps only the write
// handler; the read-only endpoints (Register) are untouched.
package controlapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/supervisor"
)

// Reloader is the seam to the supervisor (ADR-0027): the write handler hands it a
// validated config and gets an opaque handle; the status handler reads a handle's
// state. *supervisor.Supervisor satisfies it. Keeping it an interface keeps the
// handlers testable with a fake and the coupling one-directional.
type Reloader interface {
	RequestReload(*config.Config) (supervisor.Handle, error)
	Status(supervisor.Handle) supervisor.State
}

// RegisterMutation mounts the write + status endpoints on m. Call it ONLY when a
// non-empty bearer token is configured; with no token the caller does not call it
// and the mutation surface simply is not mounted (ADR-0028 §1). The write route is
// wrapped by the bearer gate; the status route is a read and stays open on loopback
// (ADR-0028 §2). Call before the server starts (the mux is not safe to mutate once
// serving).
func RegisterMutation(m Mounter, rl Reloader, token string) {
	m.Handle("POST /api/config", bearerAuth(token)(configHandler(rl)))
	m.Handle("GET /api/reload/{handle}", statusHandler(rl))
}

// bearerAuth wraps a handler with a constant-time bearer-token check. It compares
// the FIXED-LENGTH SHA-256 of the presented token against that of the configured
// token (F12: comparing the raw variable-length tokens would leak length through
// timing). The token travels only in the Authorization header, never a cookie, so a
// cross-site page cannot forge it (CSRF defended by construction, ADR-0028 §3).
func bearerAuth(token string) func(http.Handler) http.Handler {
	want := sha256.Sum256([]byte(token))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := sha256.Sum256([]byte(bearerToken(r.Header.Get("Authorization"))))
			if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// bearerToken extracts the token from an "Authorization: Bearer <token>" header, or
// "" if the header is absent or not a bearer scheme.
func bearerToken(h string) string {
	const prefix = "Bearer "
	if len(h) >= len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return h[len(prefix):]
	}
	return ""
}

// configHandler accepts a full config document, validates it, refuses a self-locking
// config (F11), then hands it to the supervisor and returns 202 + an opaque handle.
func configHandler(rl Reloader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var cfg config.Config
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeError(w, http.StatusBadRequest, "malformed config JSON")
			return
		}
		if err := cfg.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		// F11: the new config must keep the mutation surface mounted — it must name
		// an admin token_env that resolves non-empty. Otherwise applying it would
		// lock the operator out of the builder, irrecoverably across a restart.
		if wouldSelfLock(&cfg) {
			writeErrorCode(w, http.StatusConflict, "config_would_self_lock",
				"the new config would remove the admin token and lock the builder out of itself; edit the -config file and restart to recover")
			return
		}
		h, err := rl.RequestReload(&cfg)
		if err != nil {
			if errors.Is(err, supervisor.ErrReloadInProgress) {
				writeErrorCode(w, http.StatusConflict, "reload_in_progress", "a reload is already in progress")
				return
			}
			writeError(w, http.StatusInternalServerError, "reload could not be started")
			return
		}
		writeJSONStatus(w, http.StatusAccepted, map[string]string{"handle": string(h)})
	})
}

// statusHandler serves the state of a reload handle. The state lives in the
// supervisor and survives the cutover (ADR-0027 §F4); this handler only exposes it.
func statusHandler(rl Reloader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st := rl.Status(supervisor.Handle(r.PathValue("handle")))
		if st == "" {
			writeError(w, http.StatusNotFound, "unknown reload handle")
			return
		}
		writeJSON(w, map[string]string{"state": string(st)})
	})
}

// wouldSelfLock reports whether applying cfg would leave the mutation surface
// unmounted: no admin block, or an admin token_env that resolves to an empty value.
func wouldSelfLock(cfg *config.Config) bool {
	if cfg.Admin == nil {
		return true
	}
	return os.Getenv(cfg.Admin.TokenEnv) == ""
}

// writeError writes a JSON {"error": msg} with the given status.
func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSONStatus(w, code, map[string]string{"error": msg})
}

// writeErrorCode writes a JSON body carrying a machine-readable error_code (so the
// 2b UI can tell the two 409s apart) plus a human message.
func writeErrorCode(w http.ResponseWriter, code int, errCode, msg string) {
	writeJSONStatus(w, code, map[string]string{"error_code": errCode, "message": msg})
}

// writeJSONStatus marshals v FIRST so a marshal error becomes a 500 before any
// header is written, then writes it with the given status.
func writeJSONStatus(w http.ResponseWriter, code int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(b)
}

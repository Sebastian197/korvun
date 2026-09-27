// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// This file adds the WRITE surface of the control API (Stage 14 Phase 2a): a
// bearer-gated config-mutation endpoint plus a read-only reload-status endpoint.
// It is mounted ONLY when a bearer token is configured (ADR-0028 §1: no token =>
// mutation not mounted, the read-only default). The gate wraps only the write
// handler; the read-only endpoints (Register) are untouched.
package controlapi

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
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
	// CurrentConfig returns the config the running app was built from, so the
	// builder can load it as an editing baseline (ADR-0030 §4). nil before the
	// first app has started.
	CurrentConfig() *config.Config
}

// RegisterMutation mounts the write + status endpoints on m. Call it ONLY when a
// non-empty bearer token is configured; with no token the caller does not call it
// and the mutation surface simply is not mounted (ADR-0028 §1). The write route is
// wrapped by the bearer gate; the status route stays open on loopback (ADR-0028
// §2). Since 2026-09-24 that route is not a pure read: it answers the act bound
// to a handle with its receipt, and a poll that arrives before the process's
// own observer has closed the act performs that close — always with the
// SUPERVISOR's outcome, never anything the caller sent. No client input reaches
// the ledger through it; what an unauthenticated loopback caller can do is learn
// the ids of a change already made. Call before the server starts (the mux is
// not safe to mutate once serving).
// rec records the operator act behind every write. A nil rec is NOT «record
// nothing»: the write door then refuses by name. The builder has written the
// profile through this door since Stage 14 and left no trace in the action
// ledger; from 2026-09-24 a profile change that cannot be recorded is not applied
// (director's ruling).
func RegisterMutation(m Mounter, rl Reloader, token string, rec ActRecorder) {
	m.Handle("POST /api/config", bearerAuth(token)(configHandler(rl, rec)))
	m.Handle("GET /api/config", bearerAuth(token)(configGetHandler(rl)))
	m.Handle("GET /api/reload/{handle}", statusHandler(rl, rec))
}

// configGetHandler serves the raw current config as the builder's editing baseline
// (ADR-0030 §4). It marshals the config STRUCT, which carries only env-var NAMES
// (token_env, api_key_env), never secret VALUES: os.Getenv is never called on this
// path, so no secret can leave the process. Gated by the same bearer as the write.
func configGetHandler(rl Reloader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		cfg := rl.CurrentConfig()
		if cfg == nil {
			writeError(w, http.StatusServiceUnavailable, "no current config")
			return
		}
		writeJSON(w, cfg)
	})
}

// bearerAuth wraps a handler with a constant-time bearer-token check. It compares
// the FIXED-LENGTH SHA-256 of the presented token against that of the configured
// token (F12: comparing the raw variable-length tokens would leak length through
// timing). The token travels only in the Authorization header, never a cookie, so a
// cross-site page cannot forge it (CSRF defended by construction, ADR-0028 §3).
//
// A SECOND COPY of this check lives in approvals.go as approvalsAuth, which
// differs only in the body it writes: that surface renders by error NAME, and
// the name it needs is "forbidden", not "unauthorized". Two copies of a
// security check are two places to hide, so the unification is FILED
// (2026-09-12) for after the v0.15.0 tag. Until then: edit one, edit both.
func bearerAuth(token string) func(http.Handler) http.Handler {
	want := sha256.Sum256([]byte(token))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// A configured EMPTY token must authenticate NO ONE. Without this,
			// sha256("") on both sides lets an empty presented token match and bypass
			// the gate. app.Build only mounts with a non-empty token, but the guard
			// makes that safe BY CONSTRUCTION rather than by the caller's invariant, so
			// a future second caller of RegisterMutation cannot reopen the bypass.
			if token == "" {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
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

// maxConfigBodyBytes caps the POST /api/config request body. A full Korvun config
// is well under 64 KiB, so 1 MiB is generous headroom while bounding the memory an
// authenticated admin can force the process to buffer (Phase 2b.0 hardening).
const maxConfigBodyBytes = 1 << 20 // 1 MiB

// configHandler accepts a full config document, validates it, refuses a self-locking
// config (F11), then hands it to the supervisor and returns 202 + an opaque handle.
func configHandler(rl Reloader, rec ActRecorder) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Cap the body BEFORE decoding so an oversized document is cut at the reader,
		// never fully buffered (a MaxBytesError surfaces from Decode as a 413).
		r.Body = http.MaxBytesReader(w, r.Body, maxConfigBodyBytes)
		// Strict decode, mirroring config.Load (audit A-1 / estreno E-2): a
		// typo'd key must fail LOUDLY naming the key — before this, an
		// unknown key was silently dropped, the reload succeeded, and the
		// persisted file lost the typo ("governence" => an ungoverned agent
		// brain the operator believed governed).
		// The raw bytes are kept so the act can seal their digest. The decoder
		// reads from a tee rather than a second read of the body, which is gone
		// once decoded.
		var rawBuf bytes.Buffer
		dec := json.NewDecoder(io.TeeReader(r.Body, &rawBuf))
		dec.DisallowUnknownFields()
		var cfg config.Config
		if err := dec.Decode(&cfg); err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				writeError(w, http.StatusRequestEntityTooLarge, "config document exceeds the 1 MiB limit")
				return
			}
			if strings.Contains(err.Error(), "unknown field") {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeError(w, http.StatusBadRequest, "malformed config JSON")
			return
		}
		// json.Unmarshal rejected trailing garbage; the streaming decoder
		// must not regress that (config.Load parity).
		if err := dec.Decode(new(json.RawMessage)); err != io.EOF {
			writeError(w, http.StatusBadRequest, "trailing data after the config document")
			return
		}
		raw := rawBuf.Bytes()
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
		// THE ACT COMES BEFORE THE CHANGE, exactly as on the screen's four doors.
		// Everything above could refuse with nothing attempted; from here a
		// change is going to be asked for, and the book says so first.
		if rec == nil {
			writeErrorCode(w, http.StatusServiceUnavailable, "no_ledger", noLedgerMessage)
			return
		}
		// The act seals the DIGEST of the document, not the document: a config
		// carries env-var names and shapes an operator may not want copied into
		// the ledger's parameters, and what the book needs is proof of WHICH
		// change this was.
		sum := sha256.Sum256(raw)
		act, err := rec.BeginConfigAct(r.Context(), actVerb("post-config"),
			[]byte(`{"door":"post-config","document_sha256":"`+hex.EncodeToString(sum[:])+`"}`))
		if err != nil {
			// A recorder that exists and has no ledger (a profile with no
			// storage block) says so by name, and the operator reads the same
			// refusal a nil recorder gives — not «the book refused».
			if errors.Is(err, ErrNoLedger) {
				writeErrorCode(w, http.StatusServiceUnavailable, "no_ledger", noLedgerMessage)
				return
			}
			if errors.Is(err, ErrLedgerForeign) {
				writeErrorCode(w, http.StatusConflict, "ledger_foreign_profile", ledgerForeignMessage)
				return
			}
			writeErrorCode(w, http.StatusServiceUnavailable, "act_not_recorded",
				"the change was not attempted because the operator act could not be recorded: "+err.Error())
			return
		}
		h, err := rl.RequestReload(&cfg)
		if err != nil {
			// The act closes FAILED — the supervisor never took the change —
			// and the refusal names the closed act with its receipt, so the
			// builder can show what the book recorded.
			act = rec.SettleAct(r.Context(), act.ActionID, false, err.Error())
			if errors.Is(err, supervisor.ErrReloadInProgress) {
				writeJSONStatus(w, http.StatusConflict, map[string]string{
					"error_code": "reload_in_progress", "message": "a reload is already in progress",
					"action_id": act.ActionID, "receipt_id": act.ReceiptID,
				})
				return
			}
			writeJSONStatus(w, http.StatusInternalServerError, map[string]string{
				"error":     "reload could not be started",
				"action_id": act.ActionID, "receipt_id": act.ReceiptID,
			})
			return
		}
		rec.BindReload(act.ActionID, string(h))
		if settled, applied := actOutcome(rl.Status(h)); settled {
			act = rec.SettleAct(r.Context(), act.ActionID, applied, string(rl.Status(h)))
		}
		writeJSONStatus(w, http.StatusAccepted, map[string]string{
			"handle": string(h), "action_id": act.ActionID, "receipt_id": act.ReceiptID,
		})
	})
}

// noLedgerMessage is the builder door's refusal on a profile with no action
// store. The builder paints it verbatim, so it names the way out: the one door
// open on such a profile, and the hand edit.
const noLedgerMessage = "this profile has no action store, so a change could not be recorded as an operator act; nothing is applied without a book to write it in. Press «Activar almacén» on the app's «¿Qué pasa hoy?» screen, or add storage.path to the profile and restart"

// ledgerForeignMessage is the builder door's refusal on a ledger another
// profile founded or adopted. The builder paints it verbatim.
const ledgerForeignMessage = "this profile's action store was founded or adopted by another profile, so no act of this profile can be recorded in it; nothing is applied. Press «Adoptar libro» on the app's «¿Qué pasa hoy?» screen to take the book for this profile"

// statusHandler serves the state of a reload handle. The state lives in the
// supervisor and survives the cutover (ADR-0027 §F4); this handler only exposes it.
func statusHandler(rl Reloader, rec ActRecorder) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handle := r.PathValue("handle")
		st := rl.Status(supervisor.Handle(handle))
		if st == "" {
			writeError(w, http.StatusNotFound, "unknown reload handle")
			return
		}
		// The act is normally closed by the process itself, the moment the
		// supervisor stores a terminal state (internal/app's registry, through
		// the supervisor's observer). This door READS that close and answers the
		// receipt; when a poll arrives first, it performs the close with the same
		// outcome, once, and the rest of the polls answer the same receipt.
		// `RequestReload` is asynchronous — it answers a `pending` handle — so a
		// close in the write handler would have recorded a wish.
		answer := map[string]string{"state": string(st)}
		if rec != nil {
			if settled, applied := actOutcome(st); settled {
				// The receipt is born at the close, so this is the first moment
				// it can travel. The screen polling a cutover gets it here.
				if act := rec.SettleReload(r.Context(), handle, applied, string(st)); act.ActionID != "" {
					answer["action_id"] = act.ActionID
					if act.ReceiptID != "" {
						answer["receipt_id"] = act.ReceiptID
					}
				}
			}
		}
		writeJSON(w, answer)
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

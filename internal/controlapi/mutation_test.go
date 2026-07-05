// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package controlapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/supervisor"
)

// validCfgBody is a schema-valid config document whose admin block names a token
// env-var; tests set that env-var so the config does not self-lock.
const (
	adminEnv     = "KORVUN_ADMIN_TOKEN_TEST"
	validCfgBody = `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],` +
		`"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},` +
		`"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],` +
		`"routes":[{"channel":"telegram","brain":"d"}],"admin":{"token_env":"` + adminEnv + `"}}`
)

// fakeReloader is the supervisor seam for handler tests.
type fakeReloader struct {
	mu     sync.Mutex
	handle supervisor.Handle
	err    error
	states map[supervisor.Handle]supervisor.State
	calls  int
	gotCfg *config.Config
	cfg    *config.Config // returned by CurrentConfig (the editing baseline)
}

func (f *fakeReloader) CurrentConfig() *config.Config {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cfg
}

func (f *fakeReloader) RequestReload(cfg *config.Config) (supervisor.Handle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.gotCfg = cfg
	if f.err != nil {
		return "", f.err
	}
	return f.handle, nil
}

func (f *fakeReloader) Status(h supervisor.Handle) supervisor.State {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.states[h]
}

func (f *fakeReloader) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

var _ Reloader = (*fakeReloader)(nil)

// mutationMux mounts the read-only API always and the mutation surface only when a
// token is configured (ADR-0028 §1) — mirroring the app.Build call site.
func mutationMux(token string, rl Reloader) *http.ServeMux {
	mux := http.NewServeMux()
	Register(mux, fakeReader{})
	if token != "" {
		RegisterMutation(mux, rl, token)
	}
	return mux
}

func do(mux *http.ServeMux, method, path, auth, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	if auth != "" {
		r.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	return rec
}

func bodyCode(rec *httptest.ResponseRecorder) string {
	var m map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	return m["error_code"]
}

// ---- C1 (LOAD-BEARING): the bearer gate + read-only intact -------------------

func TestMutation_authGate_401paths_readOnlyIntact(t *testing.T) {
	t.Setenv(adminEnv, "adminval") // so a correct request is not self-lock/400
	rl := &fakeReloader{handle: "reload-1"}
	mux := mutationMux("secret", rl)

	if rec := do(mux, "POST", "/api/config", "", validCfgBody); rec.Code != http.StatusUnauthorized {
		t.Errorf("no token: got %d, want 401", rec.Code)
	}
	if rec := do(mux, "POST", "/api/config", "Bearer wrong", validCfgBody); rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong token: got %d, want 401", rec.Code)
	}
	if rec := do(mux, "POST", "/api/config", "Bearer ", validCfgBody); rec.Code != http.StatusUnauthorized {
		t.Errorf("empty bearer: got %d, want 401", rec.Code)
	}
	if rec := do(mux, "POST", "/api/config", "Bearer secret", validCfgBody); rec.Code == http.StatusUnauthorized {
		t.Errorf("correct token got 401; the handler must be reached")
	}

	// Read-only endpoints stay reachable WITHOUT a token (ADR-0028 §2, unchanged).
	for _, p := range []string{"/api/brains", "/api/channels"} {
		if rec := do(mux, "GET", p, "", ""); rec.Code != http.StatusOK {
			t.Errorf("%s without token: got %d, want 200 (read-only intact)", p, rec.Code)
		}
	}
}

// ---- C2 (F12): fixed-length hash compare, no length oracle -------------------

func TestMutation_wrongLengthToken_401(t *testing.T) {
	t.Setenv(adminEnv, "adminval")
	rl := &fakeReloader{handle: "r1"}
	mux := mutationMux("secret-token-of-some-length", rl)
	if rec := do(mux, "POST", "/api/config", "Bearer x", validCfgBody); rec.Code != http.StatusUnauthorized {
		t.Errorf("a much shorter token: got %d, want 401 (compared via fixed-length SHA-256)", rec.Code)
	}
}

// ---- C3 (CSRF): token in a cookie is ignored --------------------------------

func TestMutation_tokenInCookie_401(t *testing.T) {
	t.Setenv(adminEnv, "adminval")
	rl := &fakeReloader{handle: "r1"}
	mux := mutationMux("secret", rl)
	r := httptest.NewRequest("POST", "/api/config", strings.NewReader(validCfgBody))
	r.AddCookie(&http.Cookie{Name: "token", Value: "secret"}) // token in a cookie, not Authorization
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("token in a cookie: got %d, want 401 (header-not-cookie defeats CSRF)", rec.Code)
	}
}

// ---- C5: invalid config -> 400 ----------------------------------------------

func TestMutation_invalidConfig_400(t *testing.T) {
	t.Setenv(adminEnv, "adminval")
	rl := &fakeReloader{handle: "r1"}
	mux := mutationMux("secret", rl)
	if rec := do(mux, "POST", "/api/config", "Bearer secret", `{"channels":[]}`); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid config: got %d, want 400", rec.Code)
	}
	if rl.callCount() != 0 {
		t.Error("an invalid config was handed to the supervisor")
	}
}

// ---- C6 (F11): a config that removes the token -> 409 config_would_self_lock --

func TestMutation_selfLock_409(t *testing.T) {
	// adminEnv is intentionally NOT set, so the incoming config's admin token
	// resolves empty => it would unmount the mutation surface => self-lock.
	rl := &fakeReloader{handle: "r1"}
	mux := mutationMux("secret", rl)
	rec := do(mux, "POST", "/api/config", "Bearer secret", validCfgBody)
	if rec.Code != http.StatusConflict {
		t.Fatalf("self-lock config: got %d, want 409", rec.Code)
	}
	if bodyCode(rec) != "config_would_self_lock" {
		t.Errorf("error_code = %q, want config_would_self_lock", bodyCode(rec))
	}
	if rl.callCount() != 0 {
		t.Error("a self-locking config was handed to the supervisor")
	}
}

// ---- C7 + C8: single-flight -> 409 reload_in_progress, distinct body code -----

func TestMutation_reloadInProgress_409_distinctCode(t *testing.T) {
	t.Setenv(adminEnv, "adminval")
	rl := &fakeReloader{err: supervisor.ErrReloadInProgress}
	mux := mutationMux("secret", rl)
	rec := do(mux, "POST", "/api/config", "Bearer secret", validCfgBody)
	if rec.Code != http.StatusConflict {
		t.Fatalf("reload in progress: got %d, want 409", rec.Code)
	}
	if got := bodyCode(rec); got != "reload_in_progress" {
		t.Errorf("error_code = %q, want reload_in_progress", got)
	}
	// C8: the two 409s must be distinguishable by body code for the 2b UI.
	if bodyCode(rec) == "config_would_self_lock" {
		t.Error("reload_in_progress and config_would_self_lock share a body code; the UI cannot tell them apart")
	}
}

// ---- C10: success -> 202 + opaque handle ------------------------------------

func TestMutation_success_202_handle(t *testing.T) {
	t.Setenv(adminEnv, "adminval")
	rl := &fakeReloader{handle: "reload-42"}
	mux := mutationMux("secret", rl)
	rec := do(mux, "POST", "/api/config", "Bearer secret", validCfgBody)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("success: got %d, want 202", rec.Code)
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("202 body not JSON: %v", err)
	}
	if resp["handle"] != "reload-42" {
		t.Errorf("handle = %q, want reload-42", resp["handle"])
	}
	if rl.callCount() != 1 {
		t.Errorf("RequestReload called %d times, want 1", rl.callCount())
	}
}

// ---- extra handler branches (malformed body, generic error, no-admin self-lock)

func TestMutation_malformedJSON_400(t *testing.T) {
	rl := &fakeReloader{handle: "r1"}
	mux := mutationMux("secret", rl)
	if rec := do(mux, "POST", "/api/config", "Bearer secret", `{not valid json`); rec.Code != http.StatusBadRequest {
		t.Errorf("malformed JSON: got %d, want 400", rec.Code)
	}
	if rl.callCount() != 0 {
		t.Error("malformed JSON reached the supervisor")
	}
}

// The handler special-cases ONLY ErrReloadInProgress (409). Every other reload
// error — including supervisor.ErrShuttingDown, which is an in-process guard the
// HTTP path does not translate and in practice never even receives — falls to a
// generic 500. The fake's "shutting down" text stands in for that whole class.
func TestMutation_genericReloadError_500(t *testing.T) {
	t.Setenv(adminEnv, "adminval")
	rl := &fakeReloader{err: errors.New("supervisor is shutting down")}
	mux := mutationMux("secret", rl)
	if rec := do(mux, "POST", "/api/config", "Bearer secret", validCfgBody); rec.Code != http.StatusInternalServerError {
		t.Errorf("a non-single-flight reload error: got %d, want 500", rec.Code)
	}
}

func TestMutation_selfLock_noAdminBlock_409(t *testing.T) {
	// A valid config with NO admin block would unmount the mutation surface.
	const noAdminBody = `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],` +
		`"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},` +
		`"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],` +
		`"routes":[{"channel":"telegram","brain":"d"}]}`
	rl := &fakeReloader{handle: "r1"}
	mux := mutationMux("secret", rl)
	rec := do(mux, "POST", "/api/config", "Bearer secret", noAdminBody)
	if rec.Code != http.StatusConflict {
		t.Fatalf("no-admin-block config: got %d, want 409", rec.Code)
	}
	if bodyCode(rec) != "config_would_self_lock" {
		t.Errorf("error_code = %q, want config_would_self_lock", bodyCode(rec))
	}
	if rl.callCount() != 0 {
		t.Error("a self-locking config reached the supervisor")
	}
}

// ---- C11: status endpoint ---------------------------------------------------

func TestMutation_statusEndpoint(t *testing.T) {
	rl := &fakeReloader{states: map[supervisor.Handle]supervisor.State{"reload-1": supervisor.StateSucceeded}}
	mux := mutationMux("secret", rl)

	rec := do(mux, "GET", "/api/reload/reload-1", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status of a known handle: got %d, want 200", rec.Code)
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["state"] != string(supervisor.StateSucceeded) {
		t.Errorf("state = %q, want succeeded", resp["state"])
	}

	if rec := do(mux, "GET", "/api/reload/does-not-exist", "", ""); rec.Code != http.StatusNotFound {
		t.Errorf("status of an unknown handle: got %d, want 404", rec.Code)
	}
}

// ---- 2b.1: GET /api/config (gated round-trip baseline) ----------------------

// GET /api/config returns the raw current config as the builder's editing baseline
// (ADR-0030 §4), gated by the same bearer as the write. It exposes env-var NAMES
// (the baseline needs them) but NEVER secret VALUES: the handler marshals the config
// struct, so os.Getenv is never called and no secret can leave the process. This
// test BITES if the handler ever resolves and embeds a secret value.
func TestConfig_get_gated_exposesNamesNeverValues(t *testing.T) {
	const secretVal = "SUPER-SECRET-TOKEN-VALUE-MUST-NOT-LEAK"
	t.Setenv(adminEnv, secretVal) // the admin token VALUE lives only in the env
	var cfg config.Config
	if err := json.Unmarshal([]byte(validCfgBody), &cfg); err != nil {
		t.Fatalf("seed cfg: %v", err)
	}
	rl := &fakeReloader{cfg: &cfg}
	mux := mutationMux("secret", rl)

	if rec := do(mux, "GET", "/api/config", "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("GET /api/config without token: got %d, want 401", rec.Code)
	}

	rec := do(mux, "GET", "/api/config", "Bearer secret", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/config with token: got %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, adminEnv) {
		t.Errorf("response missing the token_env NAME %q (needed as the editing baseline)", adminEnv)
	}
	if strings.Contains(body, secretVal) {
		t.Error("SECRET VALUE leaked in GET /api/config — the handler must not resolve os.Getenv")
	}
}

// ---- 2b.0 hardening: empty-token guard + request-body cap --------------------

// A configured EMPTY token must authenticate no one. Without a guard, bearerAuth
// hashes sha256("") on both sides, so an empty presented token matches and bypasses
// the gate (a full config-takeover). This is safe today only by the app.Build
// invariant (mount only when the token is non-empty); the guard makes it safe by
// construction so a future second caller of RegisterMutation cannot reopen it.
func TestMutation_emptyConfiguredToken_neverAuthenticates(t *testing.T) {
	t.Setenv(adminEnv, "adminval") // wouldSelfLock is false, so a bypass would reach 202
	rl := &fakeReloader{handle: "r1"}
	mux := http.NewServeMux()
	RegisterMutation(mux, rl, "") // deliberately mount with an empty token (the footgun)

	if rec := do(mux, "POST", "/api/config", "Bearer ", validCfgBody); rec.Code != http.StatusUnauthorized {
		t.Errorf("empty configured token + empty presented bearer: got %d, want 401 (no sha256(\"\") bypass)", rec.Code)
	}
	if rec := do(mux, "POST", "/api/config", "", validCfgBody); rec.Code != http.StatusUnauthorized {
		t.Errorf("empty configured token + no auth header: got %d, want 401", rec.Code)
	}
	if rl.callCount() != 0 {
		t.Error("an empty-token bypass reached the supervisor")
	}
}

// A config document past the 1 MiB cap is cut before Decode (413), bounding an
// authenticated admin's memory footprint. The body below is schema-valid but padded
// past the cap: without the cap it would be accepted (202); with it, it is refused.
func TestMutation_bodyTooLarge_413(t *testing.T) {
	t.Setenv(adminEnv, "adminval")
	rl := &fakeReloader{handle: "r1"}
	mux := mutationMux("secret", rl)
	huge := `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],` +
		`"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},` +
		`"models":[{"provider":"ollama","model_id":"` + strings.Repeat("m", 2<<20) + `","locality":"local"}]}],` +
		`"routes":[{"channel":"telegram","brain":"d"}],"admin":{"token_env":"` + adminEnv + `"}}`

	rec := do(mux, "POST", "/api/config", "Bearer secret", huge)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("body > 1 MiB: got %d, want 413 (cut before Decode)", rec.Code)
	}
	if rl.callCount() != 0 {
		t.Error("an oversized body reached the supervisor")
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.16.2 · THE OPERATOR ACT on the BUILDER's door (director's ruling,
// 2026-09-24).
//
// `POST /api/config` is how the builder has written the profile since Stage 14,
// and it left NO trace in the action ledger. That is not a gap of the screen's
// four new doors — it is older and wider — and the ruling covers it by name.
//
// The three moulds the ruling asks for, plus the status door's close, which is
// the only surface that learns an asynchronous cutover's outcome.
//
// Evidence level, honest: in-process, over a ServeMux and httptest recorders, a
// fake reloader and a fake ledger. No store, no supervisor, no disk.

package controlapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/supervisor"
)

// mutationMuxWithActs is mutationMux with a ledger the mould can inspect.
func mutationMuxWithActs(token string, rl Reloader, rec ActRecorder) *http.ServeMux {
	mux := http.NewServeMux()
	Register(mux, fakeReader{})
	RegisterMutation(mux, rl, token, rec)
	return mux
}

// TestConfigAct_theBuilderDoorRecordsAnOperatorAct: the act names the door and
// seals the DIGEST of the document, and the 202 carries both ids so the operator
// can look the change up in the book.
//
// The digest rather than the document is deliberate: a config carries env-var
// names and a shape an operator may not want copied into the ledger's parameters,
// and what the book needs is proof of WHICH change this was.
//
// PROBING MUTATION (executed): drop the BeginConfigAct call. The ledger records
// nothing, the 202 carries no ids, and this reddens on both.
func TestConfigAct_theBuilderDoorRecordsAnOperatorAct(t *testing.T) {
	t.Setenv(adminEnv, "tok")
	rl := &fakeReloader{handle: "h1", states: map[supervisor.Handle]supervisor.State{"h1": supervisor.StateSucceeded}}
	acts := newFakeActs()
	mux := mutationMuxWithActs("secret", rl, acts)

	rec := do(mux, "POST", "/api/config", "Bearer secret", validCfgBody)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", rec.Code, rec.Body.String())
	}
	if got := bodyField(rec, "action_id"); got == "" {
		t.Error("the 202 carries no action_id")
	}
	if got := bodyField(rec, "receipt_id"); got == "" {
		t.Error("the 202 carries no receipt_id")
	}
	verbs := acts.verbs()
	if len(verbs) != 1 {
		t.Fatalf("the ledger recorded %d acts, want 1: %v", len(verbs), verbs)
	}
	if !strings.HasPrefix(verbs[0], "config.post-config ") {
		t.Fatalf("the act reads %q, which does not name the builder's door", verbs[0])
	}
	if !strings.Contains(verbs[0], "document_sha256") {
		t.Fatalf("the act reads %q, which seals no digest of the document", verbs[0])
	}
}

// TestConfigAct_noActMeansNoChange: the ledger refuses and the supervisor is
// never asked. By impossibility.
//
// PROBING MUTATION (executed): ignore BeginConfigAct's error. The reloader is
// called once and this reddens on the counter.
func TestConfigAct_noActMeansNoChange(t *testing.T) {
	t.Setenv(adminEnv, "tok")
	rl := &fakeReloader{handle: "h1", states: map[supervisor.Handle]supervisor.State{"h1": supervisor.StateSucceeded}}
	acts := newFakeActs()
	acts.beginErr = errors.New("the book is not writable")
	mux := mutationMuxWithActs("secret", rl, acts)

	rec := do(mux, "POST", "/api/config", "Bearer secret", validCfgBody)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body.String())
	}
	if code := bodyCode(rec); code != "act_not_recorded" {
		t.Fatalf("error_code = %q, want act_not_recorded", code)
	}
	if n := rl.callCount(); n != 0 {
		t.Fatalf("the reloader was asked %d times: a change with no act must never reach the supervisor", n)
	}
}

// TestConfigAct_noLedgerMeansNoChange: a profile with no action store. The door
// refuses rather than writing the profile unrecorded.
//
// PROBING MUTATION (executed): treat a nil recorder as «record nothing». The
// reloader is asked and this reddens.
func TestConfigAct_noLedgerMeansNoChange(t *testing.T) {
	t.Setenv(adminEnv, "tok")
	rl := &fakeReloader{handle: "h1", states: map[supervisor.Handle]supervisor.State{"h1": supervisor.StateSucceeded}}
	mux := mutationMuxWithActs("secret", rl, nil)

	rec := do(mux, "POST", "/api/config", "Bearer secret", validCfgBody)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body.String())
	}
	if code := bodyCode(rec); code != "no_ledger" {
		t.Fatalf("error_code = %q, want no_ledger", code)
	}
	if n := rl.callCount(); n != 0 {
		t.Fatalf("the reloader was asked %d times with no ledger", n)
	}
}

// TestConfigAct_theStatusDoorClosesTheActWhenItLearnsTheOutcome is THE ROW THE
// RULING NAMED, in its asynchronous form. `RequestReload` answers a `pending`
// handle, so the write handler cannot know the result; the status door is the
// surface that learns it, and that is where the act closes — once, however many
// times the screen polls.
//
// PROBING MUTATION (executed): remove the SettleReload call from statusHandler.
// The act is never closed and this reddens.
func TestConfigAct_theStatusDoorClosesTheActWhenItLearnsTheOutcome(t *testing.T) {
	t.Setenv(adminEnv, "tok")
	rl := &fakeReloader{handle: "h1", states: map[supervisor.Handle]supervisor.State{"h1": supervisor.StatePending}}
	acts := newFakeActs()
	mux := mutationMuxWithActs("secret", rl, acts)

	if rec := do(mux, "POST", "/api/config", "Bearer secret", validCfgBody); rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	// Still pending: nothing may be closed, because nothing is known.
	if closes := acts.closes(); len(closes) != 0 {
		t.Fatalf("the ledger closed %v over a cutover still running", closes)
	}
	if rec := do(mux, "GET", "/api/reload/h1", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("status door = %d, want 200", rec.Code)
	}
	// Polled while pending: still nothing.
	if closes := acts.closes(); len(closes) != 0 {
		t.Fatalf("the ledger closed %v on a pending poll", closes)
	}

	rl.mu.Lock()
	rl.states["h1"] = supervisor.StateRolledBack
	rl.mu.Unlock()

	do(mux, "GET", "/api/reload/h1", "", "")
	closes := acts.closes()
	if len(closes) != 1 || closes[0] != "act_01:false" {
		t.Fatalf("the ledger closed %v; a rolled-back cutover must close its act as NOT applied", closes)
	}
}

// WHERE «ONCE» IS PROVED, and why not here. The screen polls, so this surface
// asks the recorder to settle on EVERY poll that sees a terminal state — and the
// first version of the mould above asserted the ledger was asked exactly once,
// which the dumb double could never satisfy. It was measuring the double, not the
// surface: the class the doctrine calls a test that passes for the wrong reason.
//
// The once-only guarantee belongs to the recorder, which owns the state that can
// know, and it is held by `TestConfigActRecorder_closesTheActExactlyOnce` in
// internal/app over a REAL store — where a second close is a real error from a
// real ledger rather than an agreement between two test doubles.

// TestConfigAct_theStatusDoorSettlesOnlyOnATerminalState is the surface's own
// half: it must not ask the recorder to settle anything it has not learned.
//
// PROBING MUTATION (executed): settle on every state. The pending poll then
// settles and this reddens.
func TestConfigAct_theStatusDoorSettlesOnlyOnATerminalState(t *testing.T) {
	t.Setenv(adminEnv, "tok")
	rl := &fakeReloader{handle: "h1", states: map[supervisor.Handle]supervisor.State{"h1": supervisor.StatePending}}
	acts := newFakeActs()
	mux := mutationMuxWithActs("secret", rl, acts)

	do(mux, "POST", "/api/config", "Bearer secret", validCfgBody)
	for i := 0; i < 3; i++ {
		do(mux, "GET", "/api/reload/h1", "", "")
	}
	if closes := acts.closes(); len(closes) != 0 {
		t.Fatalf("the ledger was asked to close %v while the cutover was still pending", closes)
	}
}

// TestConfigAct_theStatusDoorLeavesForeignHandlesAlone: the status door serves
// handles this surface never created — a reload requested before a restart, or by
// another path. Closing an act it does not own would be inventing one.
//
// PROBING MUTATION (executed): drop the empty-actionID guard in SettleReload.
// A foreign handle then closes act "" and this reddens.
func TestConfigAct_theStatusDoorLeavesForeignHandlesAlone(t *testing.T) {
	t.Setenv(adminEnv, "tok")
	rl := &fakeReloader{handle: "h1", states: map[supervisor.Handle]supervisor.State{
		"h1": supervisor.StateSucceeded, "foreign": supervisor.StateSucceeded,
	}}
	acts := newFakeActs()
	mux := mutationMuxWithActs("secret", rl, acts)

	do(mux, "GET", "/api/reload/foreign", "", "")

	if closes := acts.closes(); len(closes) != 0 {
		t.Fatalf("the ledger closed %v for a handle nobody bound", closes)
	}
}

// bodyField reads one string field of a JSON answer.
func bodyField(rec *httptest.ResponseRecorder, key string) string {
	var m map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	return m[key]
}

// TestConfigAct_aRefusedReloadAnswersTheClosedAct is the builder door's half of
// the internal pass's P3: a reload the supervisor refuses closes the act FAILED
// and the 409 names the closed act with its receipt, so the builder can show
// what the book recorded. Before, the 409 carried neither id.
//
// PROBING MUTATION: answer the plain error body. Both ids are missing and this
// reddens.
func TestConfigAct_aRefusedReloadAnswersTheClosedAct(t *testing.T) {
	t.Setenv(adminEnv, "tok")
	rl := &fakeReloader{handle: "h1", err: supervisor.ErrReloadInProgress}
	acts := newFakeActs()
	mux := mutationMuxWithActs("secret", rl, acts)

	rec := do(mux, "POST", "/api/config", "Bearer secret", validCfgBody)
	if rec.Code != http.StatusConflict || bodyCode(rec) != "reload_in_progress" {
		t.Fatalf("status %d code %q, want 409 reload_in_progress", rec.Code, bodyCode(rec))
	}
	if closes := acts.closes(); len(closes) != 1 || closes[0] != "act_01:false" {
		t.Fatalf("the ledger closed %v, want the act closed as NOT applied", closes)
	}
	if bodyField(rec, "action_id") == "" || bodyField(rec, "receipt_id") == "" {
		t.Fatalf("the 409 body %s carries no closed act", rec.Body.String())
	}
}

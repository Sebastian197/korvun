// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.16.2 · the builder's door on a profile with no ledger, driven through a
// recorder that EXISTS and answers ErrNoLedger — the shape a real app without a
// store mounts from this piece on. The refusal is the same `no_ledger` the nil
// recorder gives, and its message names the way out, because the builder paints
// that message verbatim.

package controlapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/supervisor"
)

// TestConfigAct_aLedgerlessRecorderRefusesByName: ErrNoLedger from the recorder
// is `no_ledger`, the reloader is never asked, and the operator is told how to
// get a book.
//
// PROBING MUTATION: map ErrNoLedger to act_not_recorded like any other seal
// error. This reddens on the code.
func TestConfigAct_aLedgerlessRecorderRefusesByName(t *testing.T) {
	t.Setenv(adminEnv, "tok")
	rl := &fakeReloader{handle: "h1", states: map[supervisor.Handle]supervisor.State{"h1": supervisor.StateSucceeded}}
	acts := newFakeActs()
	acts.beginErr = ErrNoLedger
	mux := mutationMuxWithActs("secret", rl, acts)

	rec := do(mux, "POST", "/api/config", "Bearer secret", validCfgBody)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body.String())
	}
	if code := bodyCode(rec); code != "no_ledger" {
		t.Fatalf("error_code = %q, want no_ledger", code)
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if !strings.Contains(body["message"], "storage.path") {
		t.Fatalf("the message %q does not tell the operator how to get a ledger", body["message"])
	}
	if n := rl.callCount(); n != 0 {
		t.Fatalf("the reloader was asked %d times with no ledger", n)
	}
}

// TestConfigAct_aForeignLedgerRefusesTheBuilderDoor is G7 for the builder's
// door: ErrLedgerForeign from the recorder is `ledger_foreign_profile`, the
// reloader is never asked, and the message tells the operator to adopt.
//
// PROBING MUTATION: map ErrLedgerForeign to act_not_recorded. This reddens on
// the code.
func TestConfigAct_aForeignLedgerRefusesTheBuilderDoor(t *testing.T) {
	t.Setenv(adminEnv, "tok")
	rl := &fakeReloader{handle: "h1", states: map[supervisor.Handle]supervisor.State{"h1": supervisor.StateSucceeded}}
	acts := newFakeActs()
	acts.beginErr = ErrLedgerForeign
	mux := mutationMuxWithActs("secret", rl, acts)

	rec := do(mux, "POST", "/api/config", "Bearer secret", validCfgBody)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
	if code := bodyCode(rec); code != "ledger_foreign_profile" {
		t.Fatalf("error_code = %q, want ledger_foreign_profile", code)
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if !strings.Contains(body["message"], "Adoptar libro") {
		t.Fatalf("the message %q does not tell the operator how to adopt the ledger", body["message"])
	}
	if n := rl.callCount(); n != 0 {
		t.Fatalf("the reloader was asked %d times over a foreign ledger", n)
	}
}

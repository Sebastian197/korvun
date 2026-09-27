// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.16.2 · THE ONE DOOR OPEN ON A PROFILE WITH NO LEDGER (director's decision,
// 2026-09-24): `enable-storage` founds the action store, seals the founding act
// in it BEFORE the change is asked for, and is the only mutation such a profile
// accepts.
//
// What each mould attacks: the order (ledger, then act, then reloader); the
// refusals by name (a profile that already has a store; a file already there
// that this process did not create; a file that could not be created; a seal
// that failed); and the rolled-back cutover, whose founding act must say FAILED.
//
// Evidence level, honest: in-process over a real httptest server, a fake reloader
// and a fake ledger. The real founding of a store on disk is `internal/app`'s
// mould; the real cutover is `internal/shell`'s.
//
// Plan: docs/superpowers/specs/2026-09-24-v0162-el-almacen-y-el-cierre-del-acto-pretest.md,
// D7, D8, F1, F3, F4b, F5, F6.

package controlapi_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/controlapi"
	"github.com/Sebastian197/korvun/internal/supervisor"
)

// storelessProfile is a valid profile with no storage block: the state the
// decision is about.
func storelessProfile() *config.Config {
	p := whatsHappeningProfile()
	p.Storage = nil
	return p
}

// TestBootstrap_theLedgerIsCreatedBeforeTheChangeIsAsked is the door's order:
// the ledger is founded and the founding act sealed, and only then is the
// supervisor handed a config that NAMES the path the ledger was created at.
//
// PROBING MUTATION: call `RequestReload` before `CreateLedger`. The reloader
// receives a config with no storage and this reddens on the path.
func TestBootstrap_theLedgerIsCreatedBeforeTheChangeIsAsked(t *testing.T) {
	withAdminToken(t)
	acts := newExternalFakeActs()
	acts.createdPath = "/profile/korvun.db"
	srv, s := actServer(t, storelessProfile(), supervisor.StateSucceeded, acts)

	resp := post(t, srv, "/api/whats-happening/enable-storage", whatsToken, `{}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out controlapi.WhatsHappeningOutcome
	decodeInto(t, resp, &out)
	if out.Outcome != controlapi.OutcomeApplied {
		t.Fatalf("outcome = %q, want %q (detail %s)", out.Outcome, controlapi.OutcomeApplied, out.Detail)
	}
	if doors := acts.createdDoors(); len(doors) != 1 || doors[0] != "enable-storage" {
		t.Fatalf("the ledger was founded for %v, want exactly [enable-storage]", doors)
	}
	if s.requested != 1 {
		t.Fatalf("the reloader was asked %d times, want 1", s.requested)
	}
	if s.got == nil || s.got.Storage == nil || s.got.Storage.Path != acts.createdPath {
		t.Fatalf("the config handed to the supervisor names storage %+v, want the created path %q", s.got.Storage, acts.createdPath)
	}
	// The founding act is the one answered, and the operator is told where the
	// book lives.
	if out.Act.ActionID == "" || out.Act.ReceiptID == "" {
		t.Fatalf("the answer carries act %+v: the founding act must be shown with its receipt", out.Act)
	}
	if !strings.Contains(out.Detail, acts.createdPath) {
		t.Fatalf("the detail %q does not name the path of the ledger it created", out.Detail)
	}
	// And the act seals the path.
	verbs := acts.verbs()
	if len(verbs) != 1 || !strings.HasPrefix(verbs[0], "config.enable-storage ") || !strings.Contains(verbs[0], acts.createdPath) {
		t.Fatalf("the founding act reads %v, want config.enable-storage sealing the path", verbs)
	}
}

// TestBootstrap_needsNoConfirmation: founding the book that records changes does
// not open real execution, so the door is not one of the confirming doors.
//
// PROBING MUTATION: add enable-storage to `confirmingDoors`. This reddens with
// 428.
func TestBootstrap_needsNoConfirmation(t *testing.T) {
	withAdminToken(t)
	acts := newExternalFakeActs()
	acts.createdPath = "/profile/korvun.db"
	srv, _ := actServer(t, storelessProfile(), supervisor.StateSucceeded, acts)

	resp := post(t, srv, "/api/whats-happening/enable-storage", whatsToken, `{"confirm":false}`)
	if resp.StatusCode == http.StatusPreconditionRequired {
		t.Fatal("enable-storage asked for a confirmation it does not need: it opens no execution")
	}
}

// TestBootstrap_aProfileWithAStoreRefusesASecondOne is F6. The refusal comes
// BEFORE the ledger seam is touched: nothing is founded, nothing is asked.
//
// PROBING MUTATION: drop the `cur.Storage != nil` refusal. CreateLedger is called
// and this reddens on the created list.
func TestBootstrap_aProfileWithAStoreRefusesASecondOne(t *testing.T) {
	withAdminToken(t)
	profile := storelessProfile()
	profile.Storage = &config.StorageConfig{Path: "/already/korvun.db"}
	acts := newExternalFakeActs()
	acts.createdPath = "/would/be/wrong.db"
	srv, s := actServer(t, profile, supervisor.StateSucceeded, acts)

	resp := post(t, srv, "/api/whats-happening/enable-storage", whatsToken, `{}`)

	refused(t, resp, http.StatusUnprocessableEntity, controlapi.OutcomeRefused, "ya tiene")
	if doors := acts.createdDoors(); len(doors) != 0 {
		t.Fatalf("a ledger was founded (%v) for a profile that already has one", doors)
	}
	if verbs := acts.verbs(); len(verbs) != 0 {
		t.Fatalf("acts were recorded (%v) for a refused change", verbs)
	}
	if s.requested != 0 {
		t.Fatalf("the reloader was asked %d times", s.requested)
	}
}

// TestBootstrap_refusalsByName drives the ledger seam's three failure classes
// and demands each one's OWN outcome — no either/or: a file already there is not
// «could not create», and a seal that failed over a file that exists is neither.
//
// PROBING MUTATION: map every CreateLedger error to `act_not_recorded`. The first
// two rows redden.
func TestBootstrap_refusalsByName(t *testing.T) {
	cases := []struct {
		name        string
		err         error
		wantStatus  int
		wantOutcome string
	}{
		{"a file already there is never adopted", controlapi.ErrLedgerExists, http.StatusConflict, controlapi.OutcomeLedgerExists},
		{"a file that could not be created", controlapi.ErrLedgerNotCreated, http.StatusServiceUnavailable, controlapi.OutcomeLedgerNotCreated},
		{"a seal that failed", errors.New("record the founding act: disk full"), http.StatusServiceUnavailable, controlapi.OutcomeActNotRecorded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withAdminToken(t)
			acts := newExternalFakeActs()
			acts.createErr = tc.err
			srv, s := actServer(t, storelessProfile(), supervisor.StateSucceeded, acts)

			resp := post(t, srv, "/api/whats-happening/enable-storage", whatsToken, `{}`)

			refused(t, resp, tc.wantStatus, tc.wantOutcome, "")
			if s.requested != 0 {
				t.Fatalf("the reloader was asked %d times after the ledger seam refused", s.requested)
			}
			if verbs := acts.verbs(); len(verbs) != 0 {
				t.Fatalf("acts recorded %v over a refused founding", verbs)
			}
		})
	}
}

// TestBootstrap_aRolledBackCutoverClosesTheFoundingActAsFailed is F3 at the
// door's level: the founding act says what happened.
//
// PROBING MUTATION: close the founding act as applied whatever the state. This
// reddens on the close.
func TestBootstrap_aRolledBackCutoverClosesTheFoundingActAsFailed(t *testing.T) {
	withAdminToken(t)
	acts := newExternalFakeActs()
	acts.createdPath = "/profile/korvun.db"
	srv, _ := actServer(t, storelessProfile(), supervisor.StateRolledBack, acts)

	resp := post(t, srv, "/api/whats-happening/enable-storage", whatsToken, `{}`)

	var out controlapi.WhatsHappeningOutcome
	decodeInto(t, resp, &out)
	if out.Outcome != controlapi.OutcomeNotApplied {
		t.Fatalf("outcome = %q, want %q", out.Outcome, controlapi.OutcomeNotApplied)
	}
	if closes := acts.closes(); len(closes) != 1 || closes[0] != "act_01:false" {
		t.Fatalf("the ledger closed %v; the founding act of a rolled-back cutover must close as NOT applied", closes)
	}
}

// TestAct_aLedgerlessRecorderRefusesByName is F1 for a recorder that EXISTS but
// has no ledger — the shape a real app with no storage now mounts, so its status
// door can still close a founding act. Its refusal must read `no_ledger`, not
// `act_not_recorded`: the operator is told the profile has no book, not that a
// book refused.
//
// PROBING MUTATION: return a plain error instead of ErrNoLedger from the
// recorder. This reddens with act_not_recorded.
func TestAct_aLedgerlessRecorderRefusesByName(t *testing.T) {
	withAdminToken(t)
	acts := newExternalFakeActs()
	acts.beginErr = controlapi.ErrNoLedger
	srv, s := actServer(t, storelessProfile(), supervisor.StateSucceeded, acts)

	resp := post(t, srv, "/api/whats-happening/enable-approvals", whatsToken, `{"confirm":true}`)

	refused(t, resp, http.StatusServiceUnavailable, controlapi.OutcomeNoLedger, "Activar almacén")
	if s.requested != 0 {
		t.Fatalf("the reloader was asked %d times with no ledger", s.requested)
	}
}

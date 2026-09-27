// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.16.2 · the durable mark «ledger founded by this profile» at the doors
// (director's order, 2026-09-24): a FOREIGN ledger refuses every change by
// name (`ledger_foreign_profile`) with nothing attempted, the one door open on
// it is the adoption — an explicit act, with confirmation, that leaves a
// receipt — and the screen's contract carries the row for it.
//
// Evidence level, honest: in-process over httptest, a fake reloader and a fake
// ledger that answers the store's own refusal. The real block is the store's
// mould; the real app is internal/app's mount mould.
//
// Plan: docs/superpowers/specs/2026-09-24-v0162-el-libro-fundado-por-este-perfil-pretest.md, G7, D7, D8, D9.

package controlapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/controlapi"
	"github.com/Sebastian197/korvun/internal/supervisor"
)

// foreignDoors are the doors a foreign ledger must refuse: the five edits plus
// the builder's, driven by name; enable-storage is refused earlier (the profile
// has a store) and adopt-ledger is the exception.
var foreignDoors = []struct{ door, body string }{
	{"enable-approvals", `{"confirm":true}`},
	{"set-ceiling", `{"confirm":true,"brain":"default"}`},
	{"lift-shadow", `{"confirm":true,"brain":"default","tool":"webhook_call"}`},
	{"allow-host", `{"confirm":true,"brain":"default","tool":"webhook_call","host":"hooks.acme.io"}`},
}

// TestAct_aForeignLedgerRefusesEveryDoor is G7 at the door: the recorder
// answers the store's refusal, the door names it, the reloader is never asked.
//
// PROBING MUTATION: map ErrLedgerForeign to act_not_recorded. This reddens on
// the outcome of every door.
func TestAct_aForeignLedgerRefusesEveryDoor(t *testing.T) {
	for _, tc := range foreignDoors {
		t.Run(tc.door, func(t *testing.T) {
			withAdminToken(t)
			acts := newExternalFakeActs()
			acts.beginErr = controlapi.ErrLedgerForeign
			srv, s := actServer(t, actProfile(), supervisor.StateSucceeded, acts)

			resp := post(t, srv, "/api/whats-happening/"+tc.door, whatsToken, tc.body)

			refused(t, resp, http.StatusConflict, controlapi.OutcomeLedgerForeign, "otro perfil")
			if s.requested != 0 {
				t.Fatalf("the reloader was asked %d times over a foreign ledger", s.requested)
			}
		})
	}
}

// TestAdopt_theDoorAdoptsAndAnswersTheReceipt is G3 at the door: with
// confirmation, the adoption act is recorded through the recorder and the
// answer carries the CLOSED act with its receipt; the supervisor is never
// involved (the profile does not change).
//
// PROBING MUTATIONS: answer before the recorder adopted (no receipt → reddens);
// call RequestReload (the counter reddens).
func TestAdopt_theDoorAdoptsAndAnswersTheReceipt(t *testing.T) {
	withAdminToken(t)
	acts := newExternalFakeActs()
	srv, s := actServer(t, actProfile(), supervisor.StateSucceeded, acts)

	resp := post(t, srv, "/api/whats-happening/adopt-ledger", whatsToken, `{"confirm":true}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out controlapi.WhatsHappeningOutcome
	decodeInto(t, resp, &out)
	if out.Outcome != controlapi.OutcomeAdopted {
		t.Fatalf("outcome = %q, want %q (detail %s)", out.Outcome, controlapi.OutcomeAdopted, out.Detail)
	}
	if out.Act.ActionID == "" || out.Act.ReceiptID == "" {
		t.Fatalf("the answer carries %+v: the adoption act must travel closed, with its receipt", out.Act)
	}
	if adopted := acts.adoptions(); adopted != 1 {
		t.Fatalf("the recorder adopted %d times, want 1", adopted)
	}
	if s.requested != 0 {
		t.Fatalf("the reloader was asked %d times: adopting changes no profile", s.requested)
	}
	if !strings.Contains(out.Detail, "adopt") && !strings.Contains(out.Detail, "adopta") {
		t.Fatalf("the detail %q does not say the book was adopted", out.Detail)
	}
}

// TestAdopt_needsConfirmation: taking a book for this profile is a consent, so
// the door refuses without it and touches nothing.
//
// PROBING MUTATION: leave adopt-ledger out of the confirming doors. This
// reddens with 200.
func TestAdopt_needsConfirmation(t *testing.T) {
	withAdminToken(t)
	acts := newExternalFakeActs()
	srv, _ := actServer(t, actProfile(), supervisor.StateSucceeded, acts)

	resp := post(t, srv, "/api/whats-happening/adopt-ledger", whatsToken, `{}`)

	refused(t, resp, http.StatusPreconditionRequired, controlapi.OutcomeNeedsConfirmation, "confirmación")
	if adopted := acts.adoptions(); adopted != 0 {
		t.Fatalf("the recorder adopted %d times without confirmation", adopted)
	}
}

// TestAdopt_aRefusedAdoptionAnswersByName: the recorder's refusals travel with
// their names — no ledger (`no_ledger`), and any other failure as
// `act_not_recorded` with the cause.
//
// PROBING MUTATION: answer `adopted` on error. Both rows redden.
func TestAdopt_aRefusedAdoptionAnswersByName(t *testing.T) {
	cases := []struct {
		name        string
		err         error
		wantStatus  int
		wantOutcome string
	}{
		{"no ledger", controlapi.ErrNoLedger, http.StatusServiceUnavailable, controlapi.OutcomeNoLedger},
		{"the store refused", errTestAdoptRefused, http.StatusServiceUnavailable, controlapi.OutcomeActNotRecorded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withAdminToken(t)
			acts := newExternalFakeActs()
			acts.adoptErr = tc.err
			srv, _ := actServer(t, actProfile(), supervisor.StateSucceeded, acts)
			resp := post(t, srv, "/api/whats-happening/adopt-ledger", whatsToken, `{"confirm":true}`)
			refused(t, resp, tc.wantStatus, tc.wantOutcome, "")
		})
	}
}

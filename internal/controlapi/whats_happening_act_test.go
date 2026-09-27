// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.16.2 · THE OPERATOR ACT behind every profile change through the Control API
// (director's ruling, 2026-09-24, revoking the reduction of G3).
//
// Three moulds per door, as ordered: the act is recorded; without the act there
// is no change; and the act carries a receipt. Plus the row the ruling named
// explicitly — «the reload fails AFTER the act was sealed» — because that is the
// case where a book could be caught recording a wish.
//
// The ORDER is the guarantee, and two of these prove it by impossibility rather
// than by inspection: a supervisor that was never called cannot have changed
// anything by any path, and the counter on the seam is the only oracle that can
// see «never called» as opposed to «called and rolled back».
//
// Evidence level, honest: in-process, over a real httptest server, a fake
// reloader that returns the supervisor's real states, and a fake ledger that can
// refuse. Not a real store, not a real supervisor, no disk — the store-backed
// half is `internal/app`'s, and `receipt verify` over a sealed receipt is the
// CLI's own mould.
//
// Plan: docs/superpowers/specs/2026-09-24-v0162-el-acto-del-operador-pretest.md,
// guarantees A1…A6, attacks T1…T8.

package controlapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/controlapi"
	"github.com/Sebastian197/korvun/internal/supervisor"
)

// actDoors is every door the ruling covers on this surface, with a body each
// accepts. Driving the list rather than four hand-written cases means a fifth
// door added tomorrow is covered here the moment it appears in the contract.
var actDoors = []struct {
	door string
	body string
}{
	{"enable-approvals", `{"confirm":true}`},
	{"set-ceiling", `{"confirm":true,"brain":"default"}`},
	{"lift-shadow", `{"confirm":true,"brain":"default","tool":"webhook_call"}`},
	{"allow-host", `{"confirm":true,"brain":"default","tool":"webhook_call","host":"hooks.acme.io"}`},
}

// actProfile is a valid profile every door can act on: no ceiling to raise, a
// shadowed tool to lift, a cage to widen.
func actProfile() *config.Config {
	p := whatsHappeningProfile()
	a := p.Brains[0].Agent
	a.EffectCeiling = ""
	a.Governance = []config.ToolGrantConfig{{Tool: "webhook_call", Mode: "shadow"}}
	a.WebhookCall = &config.WebhookCallToolConfig{AllowHosts: []string{"127.0.0.1:8765"}}
	return p
}

func actServer(t *testing.T, profile *config.Config, st supervisor.State,
	rec controlapi.ActRecorder) (*httptest.Server, *shapedActions) {
	t.Helper()
	s := &shapedActions{profile: profile}
	s.state = st
	mux := http.NewServeMux()
	registerWhatsWithActs(mux, s, rec)
	return serve(t, mux), s
}

// TestAct_everyDoorRecordsAnOperatorAct is A1. Every door seals an act naming
// itself, and the answer carries the act so the operator can look it up.
//
// PROBING MUTATION (executed): drop the BeginConfigAct call from whatsHandler.
// The ledger records nothing and this reddens on every door.
func TestAct_everyDoorRecordsAnOperatorAct(t *testing.T) {
	for _, tc := range actDoors {
		t.Run(tc.door, func(t *testing.T) {
			withAdminToken(t)
			acts := newExternalFakeActs()
			srv, _ := actServer(t, actProfile(), supervisor.StateSucceeded, acts)

			resp := post(t, srv, "/api/whats-happening/"+tc.door, whatsToken, tc.body)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			var out controlapi.WhatsHappeningOutcome
			decodeInto(t, resp, &out)

			verbs := acts.verbs()
			if len(verbs) != 1 {
				t.Fatalf("the ledger recorded %d acts, want exactly 1: %v", len(verbs), verbs)
			}
			// The act NAMES the door. An act that recorded «a config changed»
			// would leave the operator unable to tell which button they pressed.
			if !strings.HasPrefix(verbs[0], "config."+tc.door+" ") {
				t.Fatalf("the act reads %q, which does not name the door %q", verbs[0], tc.door)
			}
			if out.Act.ActionID == "" {
				t.Fatal("the answer carries no action id: the operator cannot look the act up")
			}
			if out.Act.ReceiptID == "" {
				t.Fatal("the answer carries no receipt id")
			}
		})
	}
}

// TestAct_theActSealsWhatWasAsked is A5. The act's parameters carry the door and
// the request's own fields, so the book says WHICH change was attempted — not
// merely that one was.
//
// PROBING MUTATION (executed): seal `{}` instead of the canonical params. This
// reddens naming the missing host.
func TestAct_theActSealsWhatWasAsked(t *testing.T) {
	withAdminToken(t)
	acts := newExternalFakeActs()
	srv, _ := actServer(t, actProfile(), supervisor.StateSucceeded, acts)

	post(t, srv, "/api/whats-happening/allow-host", whatsToken,
		`{"confirm":true,"brain":"default","tool":"webhook_call","host":"hooks.acme.io"}`)

	verbs := acts.verbs()
	if len(verbs) != 1 {
		t.Fatalf("acts recorded: %v", verbs)
	}
	for _, want := range []string{"allow-host", "default", "webhook_call", "hooks.acme.io"} {
		if !strings.Contains(verbs[0], want) {
			t.Errorf("the act reads %q, which does not seal %q", verbs[0], want)
		}
	}
}

// TestAct_noActMeansNoChange is A2, and it is the half that holds by
// IMPOSSIBILITY. A ledger that refuses to seal must stop the change dead: the
// supervisor is never asked, so nothing can have changed by any path — not by the
// cutover, not by the persist, not by a pointer.
//
// PROBING MUTATION (executed): ignore BeginConfigAct's error and carry on. The
// reloader is then asked once and this reddens on the counter, which is the only
// oracle that can tell «never called» from «called and rolled back».
func TestAct_noActMeansNoChange(t *testing.T) {
	for _, tc := range actDoors {
		t.Run(tc.door, func(t *testing.T) {
			withAdminToken(t)
			acts := newExternalFakeActs()
			acts.beginErr = errors.New("the book is not writable")
			srv, s := actServer(t, actProfile(), supervisor.StateSucceeded, acts)

			resp := post(t, srv, "/api/whats-happening/"+tc.door, whatsToken, tc.body)

			refused(t, resp, http.StatusServiceUnavailable,
				controlapi.OutcomeActNotRecorded, "no se pudo registrar el acto")
			if s.requested != 0 {
				t.Fatalf("the reloader was asked %d times: a change with no act must never reach the supervisor", s.requested)
			}
		})
	}
}

// TestAct_noLedgerMeansNoChange is A6. A profile with no action store has no book
// to write the act into, and the door refuses rather than applying a change
// nobody can audit. It stays MOUNTED so the screen can explain the state — an
// unmounted door would make the window say «this screen has no door», which
// points at a different profile block than the one at fault.
//
// PROBING MUTATION (executed): treat a nil recorder as «record nothing» and carry
// on. The reloader is asked and this reddens.
func TestAct_noLedgerMeansNoChange(t *testing.T) {
	withAdminToken(t)
	srv, s := actServer(t, actProfile(), supervisor.StateSucceeded, nil)

	resp := post(t, srv, "/api/whats-happening/enable-approvals", whatsToken, `{"confirm":true}`)

	refused(t, resp, http.StatusServiceUnavailable, controlapi.OutcomeNoLedger, "almacén de acciones")
	if s.requested != 0 {
		t.Fatalf("the reloader was asked %d times with no ledger to record the act in", s.requested)
	}
}

// TestAct_aRolledBackReloadClosesTheActAsFailed is THE ROW THE RULING NAMED: the
// reload fails AFTER the act was sealed. The act must say what happened, not what
// was wanted — `failed`, next to an answer that says the profile is untouched.
//
// PROBING MUTATION (executed): close the act as applied whatever the state. The
// book then records a wish and this reddens on the close.
func TestAct_aRolledBackReloadClosesTheActAsFailed(t *testing.T) {
	withAdminToken(t)
	acts := newExternalFakeActs()
	srv, _ := actServer(t, actProfile(), supervisor.StateRolledBack, acts)

	resp := post(t, srv, "/api/whats-happening/enable-approvals", whatsToken, `{"confirm":true}`)

	var out controlapi.WhatsHappeningOutcome
	decodeInto(t, resp, &out)
	if out.Outcome != controlapi.OutcomeNotApplied {
		t.Fatalf("outcome = %q, want %q", out.Outcome, controlapi.OutcomeNotApplied)
	}
	closes := acts.closes()
	if len(closes) != 1 || closes[0] != "act_01:false" {
		t.Fatalf("the ledger closed %v; the act of a rolled-back cutover must close as NOT applied", closes)
	}
}

// TestAct_aSucceededReloadClosesTheActAsApplied is the other half of A3, so the
// mould above cannot pass by closing everything as failed.
//
// PROBING MUTATION (executed): close every act as failed. This reddens while its
// sibling stays green.
func TestAct_aSucceededReloadClosesTheActAsApplied(t *testing.T) {
	withAdminToken(t)
	acts := newExternalFakeActs()
	srv, _ := actServer(t, actProfile(), supervisor.StateSucceeded, acts)

	post(t, srv, "/api/whats-happening/enable-approvals", whatsToken, `{"confirm":true}`)

	if closes := acts.closes(); len(closes) != 1 || closes[0] != "act_01:true" {
		t.Fatalf("the ledger closed %v, want the act applied", closes)
	}
}

// TestAct_aCutoverStillRunningLeavesTheActOpen is the honesty boundary the paper
// declares. `Supervisor.RequestReload` is ASYNCHRONOUS — it answers a `pending`
// handle — so an act closed in the write handler would be recording a wish. It
// stays OPEN, bound to its handle, and the status door closes it.
//
// PROBING MUTATION (executed): close the act on every state. The pending cutover
// then gets a verdict nobody has observed, and this reddens.
func TestAct_aCutoverStillRunningLeavesTheActOpen(t *testing.T) {
	withAdminToken(t)
	acts := newExternalFakeActs()
	srv, _ := actServer(t, actProfile(), supervisor.StatePending, acts)

	resp := post(t, srv, "/api/whats-happening/enable-approvals", whatsToken, `{"confirm":true}`)

	var out controlapi.WhatsHappeningOutcome
	decodeInto(t, resp, &out)
	if out.Outcome != controlapi.OutcomeApplying {
		t.Fatalf("outcome = %q, want %q", out.Outcome, controlapi.OutcomeApplying)
	}
	if closes := acts.closes(); len(closes) != 0 {
		t.Fatalf("the ledger closed %v over a cutover still running: that is a wish, not a result", closes)
	}
	// And it is BOUND, so the status door can close it when it knows.
	if out.Handle == "" {
		t.Fatal("the answer carries no handle: nothing can ever close this act")
	}
	acts.SettleReload(context.Background(), out.Handle, true, "succeeded")
	if closes := acts.closes(); len(closes) != 1 || closes[0] != "act_01:true" {
		t.Fatalf("settling by handle closed %v: the act was not bound to its reload", closes)
	}
}

// TestAct_aRefusedEditRecordsNoAct is T8. A `deny` handed to `lift-shadow`, or a
// ceiling that would come down, is refused BEFORE anything is attempted — so
// there is no attempt to record. An act for a change that was never tried would
// fill the book with noise and make a real attempt harder to find.
//
// PROBING MUTATION (executed): move BeginConfigAct above the edit. The refusals
// then each leave an act and this reddens.
func TestAct_aRefusedEditRecordsNoAct(t *testing.T) {
	withAdminToken(t)
	profile := actProfile()
	profile.Brains[0].Agent.Governance = []config.ToolGrantConfig{
		{Tool: "webhook_call", Mode: "deny"},
	}
	acts := newExternalFakeActs()
	srv, s := actServer(t, profile, supervisor.StateSucceeded, acts)

	resp := post(t, srv, "/api/whats-happening/lift-shadow", whatsToken,
		`{"confirm":true,"brain":"default","tool":"webhook_call"}`)

	refused(t, resp, http.StatusUnprocessableEntity, controlapi.OutcomeRefused, `"deny"`)
	if verbs := acts.verbs(); len(verbs) != 0 {
		t.Fatalf("the ledger recorded %v for a change that was never attempted", verbs)
	}
	if s.requested != 0 {
		t.Fatalf("the reloader was asked %d times", s.requested)
	}
}

// TestAct_anotherChangeInFlightAnswersTheClosedAct is the internal pass's P3:
// when the supervisor refuses the reload (one cutover at a time), the act is
// closed FAILED — the change was never attempted — and the answer must carry
// the CLOSED act, receipt included. The first version answered the sealed one,
// so the operator saw an act with no receipt to check.
//
// PROBING MUTATION: discard SettleAct's result and answer the sealed act. The
// receipt is missing and this reddens.
func TestAct_anotherChangeInFlightAnswersTheClosedAct(t *testing.T) {
	withAdminToken(t)
	acts := newExternalFakeActs()
	srv, s := actServer(t, actProfile(), supervisor.StateSucceeded, acts)
	s.requestErr = supervisor.ErrReloadInProgress

	resp := post(t, srv, "/api/whats-happening/enable-approvals", whatsToken, `{"confirm":true}`)

	var out controlapi.WhatsHappeningOutcome
	decodeInto(t, resp, &out)
	if resp.StatusCode != http.StatusConflict || out.Outcome != controlapi.OutcomeAnotherChangeInFlight {
		t.Fatalf("status %d outcome %q, want 409 another_change_in_flight", resp.StatusCode, out.Outcome)
	}
	if closes := acts.closes(); len(closes) != 1 || closes[0] != "act_01:false" {
		t.Fatalf("the ledger closed %v, want the act closed as NOT applied", closes)
	}
	if out.Act.ActionID == "" || out.Act.ReceiptID == "" {
		t.Fatalf("the answer carries %+v: the closed act must travel with its receipt", out.Act)
	}
}

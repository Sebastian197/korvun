// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The state machine wakes its approval edges — Etapa 5, lote 2, pieza 1
// (spec FR-STATE-1, sealed): EXACTLY three edge groups and nothing
// else — NORMALIZED→PENDING_APPROVAL, PENDING_APPROVAL→{REJECTED,
// APPROVED}, APPROVED→{SUCCEEDED, FAILED} — and Terminal() gains
// REJECTED. Every OTHER reserved state (the E6 set) stays unreachable,
// pinned by the exhaustive complement sweep in the E1 table test,
// whose helper sets this piece updates UNDER THE SEALED SPEC'S
// AUTHORITY (the approved-red law's ask-first satisfied by the seal).

package action

import (
	"errors"
	"testing"
)

func TestTransition_theApprovalEdgesWake(t *testing.T) {
	t.Parallel()
	woken := []struct{ from, to State }{
		{StateNormalized, StatePendingApproval},
		{StatePendingApproval, StateRejected},
		{StatePendingApproval, StateApproved},
		{StateApproved, StateSucceeded},
		{StateApproved, StateFailed},
	}
	for _, e := range woken {
		if err := Transition(e.from, e.to); err != nil {
			t.Fatalf("the sealed edge %s -> %s must be accepted, got %v", e.from, e.to, err)
		}
	}
}

func TestTransition_nothingElseWakes(t *testing.T) {
	t.Parallel()
	// The tempting-but-forbidden edges around the new states.
	forbidden := []struct{ from, to State }{
		{StateAuthorized, StatePendingApproval}, // the gate decides BEFORE authorization
		{StatePendingApproval, StateSucceeded},  // no execution without APPROVED
		{StatePendingApproval, StateFailed},
		{StatePendingApproval, StateShadowed},
		{StateApproved, StateRejected},  // a consumed approval never reopens
		{StateRejected, StateApproved},  // terminal is terminal
		{StateApproved, StatePreparing}, // E6's edge, not ours
		{StateRejected, StatePendingApproval},
		{StatePendingApproval, StatePendingApproval}, // no self-loops
	}
	for _, e := range forbidden {
		if err := Transition(e.from, e.to); !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("edge %s -> %s must stay closed, got %v", e.from, e.to, err)
		}
	}
}

func TestTerminal_rejectedJoinsTheTerminals(t *testing.T) {
	t.Parallel()
	if !StateRejected.Terminal() {
		t.Fatal("REJECTED is a terminal outcome (it births a receipt)")
	}
	if StatePendingApproval.Terminal() || StateApproved.Terminal() {
		t.Fatal("PENDING_APPROVAL and APPROVED are NOT terminal — no receipt until the real outcome")
	}
}

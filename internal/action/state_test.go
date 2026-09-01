// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// State machine contract (spec FR-DOM-4): the §12 subset for Etapa 1 —
// RECEIVED→NORMALIZED→{DENIED,SHADOWED,AUTHORIZED}, AUTHORIZED→{SUCCEEDED,
// FAILED} — with a sentinel on every invalid transition and every reserved
// state unreachable. Approved-red contract: not edited to fit an
// implementation.

package action

import (
	"errors"
	"testing"
)

// reachableStates are the states the machine can actually reach (E1 +
// the sealed Etapa-5 approval trio).
var etapa1States = []State{
	StateReceived, StateNormalized, StateDenied, StateShadowed,
	StateAuthorized, StateSucceeded, StateFailed,
	StatePendingApproval, StateRejected, StateApproved,
	StateOutcomeUnknown,
}

// reservedStates are declared for later stages and unreachable today
// (the Etapa-5 seal woke the approval trio; the C5 consolidation woke
// OUTCOME_UNKNOWN as the honest crash close past the claim; the rest
// of the E6 set stays closed).
var reservedStates = []State{
	StatePreparing, StatePrepareFailed, StatePrepared,
	StateCommitting,
	StateCompensating, StateCompensated, StateCompensationFailed,
}

// validTransitions is the COMPLETE transition table (E1 + the sealed
// Etapa-5 approval edges).
var validTransitions = map[State][]State{
	StateReceived:        {StateNormalized},
	StateNormalized:      {StateDenied, StateShadowed, StateAuthorized, StatePendingApproval},
	StateAuthorized:      {StateSucceeded, StateFailed},
	StatePendingApproval: {StateRejected, StateApproved},
	StateApproved:        {StateSucceeded, StateFailed},
}

func allowed(from, to State) bool {
	for _, next := range validTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

func TestTransition_acceptsExactlyTheEtapa1Table(t *testing.T) {
	t.Parallel()
	for from, nexts := range validTransitions {
		for _, to := range nexts {
			if err := Transition(from, to); err != nil {
				t.Fatalf("valid transition %s -> %s must be accepted, got %v", from, to, err)
			}
		}
	}
	// Exhaustive complement over every declared state (reachable and
	// reserved): anything outside the table is the sentinel.
	all := append(append([]State(nil), etapa1States...), reservedStates...)
	for _, from := range all {
		for _, to := range all {
			if allowed(from, to) {
				continue
			}
			err := Transition(from, to)
			if err == nil {
				t.Fatalf("invalid transition %s -> %s must be rejected", from, to)
			}
			if !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("rejections carry the sentinel: %s -> %s got %v", from, to, err)
			}
		}
	}
}

func TestTransition_reservedStatesAreUnreachable(t *testing.T) {
	t.Parallel()
	for _, reserved := range reservedStates {
		for _, s := range etapa1States {
			if err := Transition(s, reserved); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("reserved state %s must be unreachable (from %s), got %v", reserved, s, err)
			}
			if err := Transition(reserved, s); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("reserved state %s must not transition (to %s), got %v", reserved, s, err)
			}
		}
	}
}

func TestTransition_unknownStatesFailClosed(t *testing.T) {
	t.Parallel()
	if err := Transition(State("BOGUS"), StateNormalized); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("an unknown from-state must fail closed, got %v", err)
	}
	if err := Transition(StateReceived, State("BOGUS")); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("an unknown to-state must fail closed, got %v", err)
	}
}

func TestTerminal_truthTable(t *testing.T) {
	t.Parallel()
	terminal := map[State]bool{
		StateReceived: false, StateNormalized: false, StateAuthorized: false,
		StateDenied: true, StateShadowed: true, StateSucceeded: true, StateFailed: true,
		StatePendingApproval: false, StateApproved: false, StateRejected: true,
	}
	for s, want := range terminal {
		if got := s.Terminal(); got != want {
			t.Fatalf("Terminal(%s) = %v, want %v", s, got, want)
		}
	}
	// Reserved and unknown states are NOT Etapa-1 terminals.
	for _, s := range append(append([]State(nil), reservedStates...), State("BOGUS")) {
		if s.Terminal() {
			t.Fatalf("state %s must not report terminal in Etapa 1", s)
		}
	}
}

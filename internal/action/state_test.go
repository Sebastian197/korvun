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
	"os"
	"regexp"
	"slices"
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
	StateReceived:   {StateNormalized},
	StateNormalized: {StateDenied, StateShadowed, StateAuthorized, StatePendingApproval},
	// Mirrors production: AUTHORIZED gained OUTCOME_UNKNOWN on 2026-09-13.
	StateAuthorized:      {StateSucceeded, StateFailed, StateOutcomeUnknown},
	StatePendingApproval: {StateRejected, StateApproved},
	// OUTCOME_UNKNOWN joins APPROVED on 2026-09-13. This table mirrors
	// production and whoever moves one moves the other: the recovery pass
	// already wrote that close for an execution that died past its claim, and
	// an in-process DEADLINE is the same fact learned another way. Without the
	// edge the only reachable close was FAILED, a definite claim nobody can
	// support.
	StateApproved: {StateSucceeded, StateFailed, StateOutcomeUnknown},
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
		// OUTCOME_UNKNOWN was missing from a table that calls itself a truth
		// table: flipping Terminal(OUTCOME_UNKNOWN) to false left this whole
		// package green. The coverage assert below is the real cure, and it
		// reads state.go — its first shape crossed against a slice typed in
		// this file, so the hole simply moved to the next entry nobody typed.
		StateOutcomeUnknown: true,
	}
	// EVERY state the package DECLARES answers here — read out of state.go,
	// not out of a slice typed in this file. A table of ten over a set of
	// eleven is a hole nothing reports, and it stayed one until the state this
	// train woke fell through it; crossing against etapa1States alone would
	// only have moved the hole to the next entry somebody forgot to type,
	// which is exactly what the coverage assert claims to prevent.
	for _, name := range declaredStateNames(t) {
		s := State(name)
		if _, ok := terminal[s]; ok {
			continue
		}
		if slices.Contains(reservedStates, s) {
			continue // answered by the reserved loop below
		}
		t.Fatalf("the truth table has no row for %s and it is not reserved — it is not a truth table", s)
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

// declaredStateNames reads the string value of every `State = "..."` constant
// out of internal/action/state.go. The lists in this file are conveniences;
// the SOURCE is the set.
func declaredStateNames(t *testing.T) []string {
	t.Helper()
	src, err := os.ReadFile("state.go")
	if err != nil {
		t.Fatalf("read the state set: %v", err)
	}
	var names []string
	for _, m := range regexp.MustCompile(`State[A-Za-z]+ State = "([A-Z_]+)"`).
		FindAllStringSubmatch(string(src), -1) {
		names = append(names, m[1])
	}
	if len(names) < 15 {
		t.Fatalf("read only %d state constants — the scan is broken, not the set", len(names))
	}
	return names
}

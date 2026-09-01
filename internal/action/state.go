// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package action

import "errors"

// State is one node of the action state machine (spec FR-DOM-4, blueprint
// §12 subset). Etapa 1 reaches exactly seven states; the remaining §12
// states are declared RESERVED below so their names are pinned, but the
// transition table rejects them entirely — an unreachable state cannot be
// misused.
type State string

// The Etapa-1 reachable states.
const (
	// StateReceived is the birth state of every action request.
	StateReceived State = "RECEIVED"
	// StateNormalized means the envelope was canonicalized and digested.
	StateNormalized State = "NORMALIZED"
	// StateDenied is terminal: the decision refused the action.
	StateDenied State = "DENIED"
	// StateShadowed is terminal: observed, simulated, NEVER executed.
	StateShadowed State = "SHADOWED"
	// StateAuthorized means the decision allowed execution to proceed.
	StateAuthorized State = "AUTHORIZED"
	// StateSucceeded is terminal: the executor completed without error.
	StateSucceeded State = "SUCCEEDED"
	// StateFailed is terminal: execution errored (or was closed by crash
	// recovery — the store marks that distinction, not the state).
	StateFailed State = "FAILED"
)

// RESERVED states (blueprint §12), unreachable in Etapa 1. Their arrival:
// approvals (Etapa 5), prepare/commit and outcome reconciliation and
// compensation (Etapa 6).
const (
	// StatePendingApproval parks an action awaiting the human (Etapa 5).
	StatePendingApproval State = "PENDING_APPROVAL"
	// StateRejected closes an action whose approval was denied/expired.
	StateRejected State = "REJECTED"
	// StateApproved marks the human's yes; execution follows (Etapa 5).
	StateApproved State = "APPROVED"
	// StatePreparing is reserved for Etapa 6.
	StatePreparing State = "PREPARING"
	// StatePrepareFailed is reserved for Etapa 6.
	StatePrepareFailed State = "PREPARE_FAILED"
	// StatePrepared is reserved for Etapa 6.
	StatePrepared State = "PREPARED"
	// StateCommitting is reserved for Etapa 6.
	StateCommitting State = "COMMITTING"
	// StateOutcomeUnknown WOKE with the C5 consolidation as the honest
	// terminal for a crash caught past the approval-params claim: the
	// external effect may or may not have fired. Reconciliation of the
	// uncertainty stays with Etapa 6.
	StateOutcomeUnknown State = "OUTCOME_UNKNOWN"
	// StateCompensating is reserved for Etapa 6.
	StateCompensating State = "COMPENSATING"
	// StateCompensated is reserved for Etapa 6.
	StateCompensated State = "COMPENSATED"
	// StateCompensationFailed is reserved for Etapa 6.
	StateCompensationFailed State = "COMPENSATION_FAILED"
)

// ErrInvalidTransition is the sentinel every rejected transition wraps.
var ErrInvalidTransition = errors.New("action: invalid state transition")

// transitions is the COMPLETE table; anything absent is invalid, which
// makes reserved and unknown states fail closed by construction. The
// Etapa-5 seal woke EXACTLY the approval edges (spec FR-STATE-1): the
// gate parks BEFORE authorization (NORMALIZED→PENDING_APPROVAL), the
// human decides (→REJECTED | →APPROVED), and an approved action
// executes directly in E5 (→SUCCEEDED | →FAILED; PREPARING waits for
// E6). Everything else stays closed.
var transitions = map[State]map[State]bool{
	StateReceived:        {StateNormalized: true},
	StateNormalized:      {StateDenied: true, StateShadowed: true, StateAuthorized: true, StatePendingApproval: true},
	StateAuthorized:      {StateSucceeded: true, StateFailed: true},
	StatePendingApproval: {StateRejected: true, StateApproved: true},
	StateApproved:        {StateSucceeded: true, StateFailed: true},
}

// Transition validates one state-machine edge. It returns nil for the
// Etapa-1 table and wraps ErrInvalidTransition for everything else —
// reserved states, unknown states, terminal states, self-loops included.
func Transition(from, to State) error {
	if transitions[from][to] {
		return nil
	}
	return errInvalidTransition(from, to)
}

// errInvalidTransition wraps the sentinel with the offending edge.
func errInvalidTransition(from, to State) error {
	return &invalidTransitionError{from: from, to: to}
}

// invalidTransitionError names the rejected edge while matching the
// sentinel through errors.Is.
type invalidTransitionError struct {
	from, to State
}

// Error renders the rejected edge.
func (e *invalidTransitionError) Error() string {
	return "action: invalid state transition " + string(e.from) + " -> " + string(e.to)
}

// Unwrap ties the error to the ErrInvalidTransition sentinel.
func (e *invalidTransitionError) Unwrap() error {
	return ErrInvalidTransition
}

// Terminal reports whether s is a reachable terminal state — a state
// with no outgoing edges. REJECTED joined with the Etapa-5 seal: a
// rejected (or expired-at-touch) approval closes its action and births
// a receipt like every other terminal outcome.
func (s State) Terminal() bool {
	switch s {
	case StateDenied, StateShadowed, StateSucceeded, StateFailed, StateRejected,
		// OUTCOME_UNKNOWN woke with the C5 consolidation as the HONEST
		// crash close for an execution already past its claim: the
		// external effect may or may not have happened, and saying
		// FAILED there would be a lie. Reconciliation is E6's stage —
		// today the state closes the action and names the uncertainty.
		StateOutcomeUnknown:
		return true
	default:
		return false
	}
}

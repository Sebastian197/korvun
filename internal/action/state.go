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
	// StatePendingApproval is reserved for Etapa 5.
	StatePendingApproval State = "PENDING_APPROVAL"
	// StateRejected is reserved for Etapa 5.
	StateRejected State = "REJECTED"
	// StateApproved is reserved for Etapa 5.
	StateApproved State = "APPROVED"
	// StatePreparing is reserved for Etapa 6.
	StatePreparing State = "PREPARING"
	// StatePrepareFailed is reserved for Etapa 6.
	StatePrepareFailed State = "PREPARE_FAILED"
	// StatePrepared is reserved for Etapa 6.
	StatePrepared State = "PREPARED"
	// StateCommitting is reserved for Etapa 6.
	StateCommitting State = "COMMITTING"
	// StateOutcomeUnknown is reserved for Etapa 6.
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

// transitions is the COMPLETE Etapa-1 table; anything absent is invalid,
// which makes reserved and unknown states fail closed by construction.
var transitions = map[State]map[State]bool{
	StateReceived:   {StateNormalized: true},
	StateNormalized: {StateDenied: true, StateShadowed: true, StateAuthorized: true},
	StateAuthorized: {StateSucceeded: true, StateFailed: true},
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

// Terminal reports whether s is an Etapa-1 terminal state — a state with
// no outgoing edges that the stage can actually reach.
func (s State) Terminal() bool {
	switch s {
	case StateDenied, StateShadowed, StateSucceeded, StateFailed:
		return true
	default:
		return false
	}
}

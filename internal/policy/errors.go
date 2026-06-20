// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package policy

import "errors"

// Sentinel errors returned by Policy implementations. Callers match
// with errors.Is rather than string comparison.
var (
	// ErrNilResult is returned by Policy.Apply when called with a nil
	// *fanout.Result. A nil Result is caller misuse — Run returns a
	// non-nil Result on mechanism success and an error otherwise, so a
	// Policy never legitimately receives nil.
	ErrNilResult = errors.New("policy: nil result")

	// ErrNoUsableOutcome is returned by Policy.Apply (alongside a
	// non-nil *Decision whose Response is nil) when no Outcome was
	// usable — typically every provider failed, or the Result carried no
	// outcomes at all. The per-provider causes are joined behind this
	// sentinel via errors.Join, so errors.Is against the sentinel AND
	// against any upstream model error (e.g. model.ErrAuthInvalid) both
	// succeed on the returned error (ADR-0012 §5a). The Brain (Stage 7)
	// surfaces an operator-configured fallback on this sentinel rather
	// than crashing or going silent.
	ErrNoUsableOutcome = errors.New("policy: no usable outcome")

	// ErrNoConsensus is returned by a consensus Policy (alongside a
	// non-nil *Decision with Response == nil) when usable outcomes
	// existed but no class reached the agreement threshold — the answers
	// disagreed. Distinct from ErrNoUsableOutcome, where there were no
	// usable answers at all. It is a bare sentinel (no errors.Join of
	// causes): disagreement is the absence of a majority, not a set of
	// provider failures. The vote breakdown lives in Decision.Provenance
	// (and the paired fanout.Result). The Brain surfaces an
	// operator-configured fallback on this sentinel. (ADR-0013 §7.)
	ErrNoConsensus = errors.New("policy: no consensus")

	// ErrNoEligibleModels is returned by SelectModels when the catalog is
	// non-empty (or empty) but the sensitivity filter leaves no model
	// permitted — typically a Private Brain wired with only Cloud models,
	// or an empty catalog. It is an operator misconfiguration that must
	// fail LOUD at construction (where SelectModels runs in the per-Brain
	// cut) rather than yield a Brain that can never answer (ADR-0015 §4).
	// Distinct from fanout.ErrNoModels, which means the model slice handed
	// to Run was empty; this one means the filter emptied it.
	ErrNoEligibleModels = errors.New("policy: no eligible models")

	// ErrUnknownSensitivity is returned by SelectModels when handed a
	// Sensitivity value it does not recognise — including the zero value of
	// an unconfigured Brain. Selection fails loud rather than silently
	// defaulting to Public (which would leak to cloud providers) or Private
	// (which would hide the misconfiguration). Privacy is declared, never
	// guessed (ADR-0012 §5e, ADR-0015 §8); an undeclared sensitivity is an
	// error, not a default. The offending value is wrapped behind this
	// sentinel via %w so errors.Is keeps working.
	ErrUnknownSensitivity = errors.New("policy: unknown sensitivity")
)

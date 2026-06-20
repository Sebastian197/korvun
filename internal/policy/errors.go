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
)

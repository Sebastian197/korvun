// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package fanout

import "errors"

// Sentinel errors returned by Coordinator.Run for mechanism-level
// configuration bugs surfaced BEFORE any goroutine spawns. Callers
// match with errors.Is rather than string comparison. Per-model
// failures live inside Outcome.Err and preserve the upstream
// internal/model sentinel grammar (ErrAuthInvalid, ErrRateLimited,
// ErrProviderUnavailable, ErrProviderResponse) untouched — the
// fanout package does NOT introduce per-provider error categories.
// See ADR-0011 §3.
var (
	// ErrNoModels is returned by Coordinator.Run when the models
	// slice is empty. A fan-out with zero participants has no
	// meaningful outcome; this is a caller-side configuration bug.
	ErrNoModels = errors.New("fanout: no models")

	// ErrNilModel is returned by Coordinator.Run when any element of
	// the models slice is nil. Same category as ErrNoModels:
	// caller-side configuration bug, surfaced before any goroutine
	// spawns.
	ErrNilModel = errors.New("fanout: nil model entry")
)

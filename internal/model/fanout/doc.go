// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package fanout coordinates the parallel dispatch of a single
// model.Request to N model.Model implementations and collects every
// outcome. It is the mechanism layer of Stage 4's last phase
// (Phase 4.3); the policy of choosing among outcomes — consensus,
// majority, first-OK, privacy-aware, cost-bounded — lives in the
// policy engine of Stages 5–6 driven by the no-code visual builder.
//
// The Coordinator never picks a "winning" response, never retries
// on its own, never decides which provider to call. Run launches
// every supplied model in its own goroutine, waits for every one
// of them to return, and hands the caller a deterministic-order
// slice of Outcomes (one per input model, in input order). Per-model
// failures live inside Outcome.Err and preserve the upstream
// internal/model sentinel grammar untouched.
//
// See ADR-0011 for the full design rationale.
package fanout

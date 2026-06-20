// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package sequential coordinates the SERIAL dispatch of a single
// model.Request to N model.Model implementations, stopping at the first
// success. It is the cost-saving sibling of internal/model/fanout: where the
// fan-out calls every provider in parallel and pays for all of them, the
// sequential coordinator calls providers in order and stops as soon as one
// returns without error — so a paid cloud provider is contacted only if the
// cheap/local one failed (ADR-0016).
//
// It is mechanism, not policy: it never picks a "winning" response among
// successes, never retries, never decides which provider to call. The stop
// predicate is minimal and mechanical — Outcome.Err == nil is success
// (ADR-0016 §3); a quality-based stop would be policy and belongs elsewhere,
// preserving the mechanism/policy boundary of ADR-0011.
//
// Run reuses the fan-out's shared primitives so the two dispatch shapes never
// diverge: fanout.ValidateRunInputs for entry validation and fanout.CallOne
// for the per-call recover+%w, latency, and per-model timeout discipline. It
// returns a *fanout.Result, so every policy reducer consumes its output
// unchanged. The Result holds an Outcome only for the models actually called,
// in call order; a model skipped after an earlier success is absent.
//
// See ADR-0016 for the full design rationale.
package sequential

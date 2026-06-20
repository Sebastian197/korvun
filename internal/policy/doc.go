// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package policy turns the outcomes of a model fan-out into a single
// Decision according to a configurable dispatch policy. It is the
// post-dispatch half of the two-phase policy model pinned by ADR-0012:
// the pre-dispatch Selector (privacy / cost routing) is deferred; this
// package ships the reducer that decides what to do with the []Outcome
// a completed fan-out returns.
//
// The central abstraction is the Policy interface, which CONSUMES a
// *fanout.Result and produces a rich *Decision (chosen Response plus
// Provenance and per-provider Accounting). A Policy is deliberately NOT
// a model.Model: model.Response is lossy for the provenance and
// consensus reasoning the engine exists to do (ADR-0012 §1). The
// mechanism/policy boundary from ADR-0011 is preserved by construction —
// internal/model/fanout never imports this package.
//
// The first concrete Policy is PriorityReducer, which selects the
// highest-priority successful Outcome by operator-declared provider
// order. It is a demonstration of SELECTION, not cost-saving: the
// wait-all fan-out has already called and paid for every provider by
// the time a Policy runs (ADR-0012 §4).
package policy

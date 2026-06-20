// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"fmt"

	"github.com/Sebastian197/korvun/internal/model"
)

// Sensitivity is the per-Brain declared data-sensitivity level. It is the
// pre-dispatch routing constraint the Selector filters on (ADR-0015). It is
// DECLARED by the operator at Brain construction, NEVER inferred from message
// content — inferring sensitivity is a recursive privacy trap (ADR-0012 §5e,
// ADR-0015 §8). The zero value is intentionally invalid so an unconfigured
// Brain fails loud (ErrUnknownSensitivity) rather than silently defaulting.
type Sensitivity int

const (
	// Public payloads may go to any provider, local or cloud.
	Public Sensitivity = iota + 1
	// Private payloads are local-only: cloud providers are excluded from the
	// fan-out before it runs, so a cloud provider is never contacted for a
	// Private Brain.
	Private
)

// Locality classifies where a model runs, for privacy-aware selection. It is
// the model attribute the Selector routes on. Locality is DECLARED by the
// operator in the catalog at wiring time, NOT read from model.Model (which is
// Generate + Name() only) — attribute declaration stays out of the provider
// contract so adding an attribute never touches every adapter (ADR-0015 §3).
type Locality int

const (
	// Local models run on the operator's own hardware (e.g. Ollama).
	Local Locality = iota + 1
	// Cloud models are third-party hosted APIs (e.g. Groq).
	Cloud
)

// CatalogEntry pairs a model with the attributes the Selector routes on. The
// operator builds a catalog at wiring time, one entry per model. CostTier is
// added additively here when cost-aware selection ships (ADR-0015 §7); it is
// the same filter mechanism, a different attribute.
type CatalogEntry struct {
	// Model is the provider adapter this entry describes.
	Model model.Model
	// Locality declares where Model runs (Local or Cloud).
	Locality Locality
}

// SelectModels filters a catalog down to the models permitted for the given
// declared sensitivity, returning them in catalog order. Public keeps every
// entry; Private keeps only Local entries (cloud providers are dropped before
// the fan-out, so they are never contacted for a Private Brain).
//
// It is the pre-dispatch Selector in its per-Brain form (ADR-0015 §4): a PURE
// function with no per-call state and no I/O, run ONCE at Brain construction.
// It therefore takes no context.Context (unlike the deferred per-message
// Selector), exactly as PriorityReducer is free to ignore the ctx it is handed.
//
// Order is preserved deterministically — the result lists permitted models in
// their catalog position — reusing the determinism discipline the fan-out
// relies on for reproducible attribution.
//
// Errors (both fail loud at construction rather than at the first message):
//   - ErrUnknownSensitivity when s is not Public or Private (including the
//     zero value of an unconfigured Brain). The offending value is wrapped via
//     %w so errors.Is(err, ErrUnknownSensitivity) holds.
//   - ErrNoEligibleModels when the filter leaves no model (e.g. a Private Brain
//     wired with only Cloud models, or an empty catalog).
//
// On any error the returned slice is nil. Nil model.Model entries are NOT
// rejected here; the fan-out's own ErrNilModel guard (ADR-0011) is the single
// place that check lives, so SelectModels does not duplicate it.
func SelectModels(catalog []CatalogEntry, s Sensitivity) ([]model.Model, error) {
	var keep func(Locality) bool
	switch s {
	case Public:
		keep = func(Locality) bool { return true }
	case Private:
		keep = func(l Locality) bool { return l == Local }
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnknownSensitivity, int(s))
	}

	selected := make([]model.Model, 0, len(catalog))
	for _, e := range catalog {
		if keep(e.Locality) {
			selected = append(selected, e.Model)
		}
	}
	if len(selected) == 0 {
		return nil, ErrNoEligibleModels
	}
	return selected, nil
}

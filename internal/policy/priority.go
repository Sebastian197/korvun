// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"context"
	"errors"
	"math"

	"github.com/Sebastian197/korvun/internal/model/fanout"
)

// Compile-time assertion that PriorityReducer satisfies Policy.
var _ Policy = PriorityReducer{}

// PriorityReducer is the first concrete Policy. It selects the
// highest-priority SUCCESSFUL Outcome by operator-declared provider
// order. Order holds provider names (matching Outcome.Provider ==
// Model.Name()), highest priority first; a provider not in Order ranks
// below every listed provider. Ties — equal rank, or two outcomes from
// the same provider — are broken by the lower fan-out index, reusing the
// deterministic order of fanout.Result.Outcomes so selection is
// reproducible (ADR-0012 §5b).
//
// PriorityReducer is a demonstration of SELECTION, not cost-saving: the
// wait-all fan-out has already called and paid for every provider before
// Apply runs (ADR-0012 §4). The zero value (empty Order) is valid: every
// provider ranks equal, so the lowest-index successful outcome wins.
type PriorityReducer struct {
	// Order lists provider names from highest to lowest priority.
	Order []string
}

// Apply selects the highest-priority successful Outcome. See the type
// doc for the ranking and tie-break rules. Apply does not consult ctx:
// priority reduction is a pure function of result.
//
// On success it returns a *Decision whose Response is the chosen
// outcome's, with full Provenance and Accounting, and a nil error. When
// no outcome is usable (every outcome failed, or there are none) it
// returns a non-nil *Decision (Response nil, provenance/accounting
// intact for the log) and an error joining ErrNoUsableOutcome with every
// per-provider failure (ADR-0012 §5a). A nil result is caller misuse and
// yields ErrNilResult with a nil Decision.
func (r PriorityReducer) Apply(ctx context.Context, result *fanout.Result) (*Decision, error) {
	if result == nil {
		return nil, ErrNilResult
	}

	n := len(result.Outcomes)
	considered := make([]Contribution, n)
	accounting := make([]ProviderCost, n)

	// Single pass: record provenance + accounting for every outcome, and
	// track the best (lowest-rank, then lowest-index) successful one.
	//
	// Usability assumes the exactly-one-of-Response/Err invariant that
	// fanout.Outcome guarantees (ADR-0011 §3) — this is the documented
	// implicit coupling between policy and fanout. The predicate is
	// conservative if that invariant is ever violated, in BOTH directions:
	//   - both non-nil (Err != nil AND Response != nil): Err != nil is
	//     checked first, so the outcome is treated as a failure — skipped,
	//     never chosen, with its Err preserved in provenance and added to
	//     the all-failed join.
	//   - both nil (Err == nil AND Response == nil): the Response == nil
	//     guard skips it too; it yields a Contribution with a nil Err that
	//     is NOT added to the all-failed join (there is no cause to record).
	// We trust the invariant only for the converse (Err == nil implies a
	// usable Response).
	bestIdx := -1
	// bestRank starts above any real rank (ranks run 0..len(Order)) so the
	// `rank < bestRank` comparison stands on its own: the first usable
	// success always claims the slot, and a genuine rank 0 can never
	// collide with the initial value. bestIdx == -1 separately tracks
	// "nothing chosen yet" for the all-failed path below.
	bestRank := math.MaxInt
	for i := range result.Outcomes {
		oc := result.Outcomes[i]
		considered[i] = Contribution{Provider: oc.Provider, Err: oc.Err}
		accounting[i] = ProviderCost{Provider: oc.Provider, Latency: oc.Latency}

		if oc.Err != nil || oc.Response == nil {
			continue // not a usable success
		}
		// Strict `<` keeps the earlier (lower-index) outcome on a tie, so
		// ties resolve to the lower fan-out index deterministically.
		if rank := rankByOrder(r.Order, oc.Provider); rank < bestRank {
			bestIdx = i
			bestRank = rank
		}
	}

	decision := &Decision{
		Provenance: Provenance{Considered: considered},
		Accounting: accounting,
	}

	if bestIdx == -1 {
		// No usable outcome. Join the per-provider failures behind the
		// sentinel so errors.Is(err, ErrNoUsableOutcome) and
		// errors.Is(err, <upstream sentinel>) both hold.
		errs := make([]error, 0, n+1)
		errs = append(errs, ErrNoUsableOutcome)
		for i := range result.Outcomes {
			if oc := result.Outcomes[i]; oc.Err != nil {
				errs = append(errs, oc.Err)
			}
		}
		return decision, errors.Join(errs...)
	}

	considered[bestIdx].Used = true
	decision.Response = result.Outcomes[bestIdx].Response
	return decision, nil
}

// rankByOrder returns the priority of provider within order: its index
// (lower is higher priority), or len(order) for a provider not listed
// (ranked below every listed provider). Shared by PriorityReducer and
// ConsensusReducer so both compute provider priority identically
// (ADR-0013 §9).
func rankByOrder(order []string, provider string) int {
	for i, name := range order {
		if name == provider {
			return i
		}
	}
	return len(order)
}

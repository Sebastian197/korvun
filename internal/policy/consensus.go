// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"context"
	"errors"
	"strings"

	"github.com/Sebastian197/korvun/internal/model/fanout"
)

// defaultNormalize is the comparison key used when ConsensusReducer.Normalize
// is nil: lowercase after trimming surrounding whitespace, so label votes
// like "Yes", "yes", and "  YES " collapse to one class.
func defaultNormalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// Compile-time assertion that ConsensusReducer satisfies Policy.
var _ Policy = ConsensusReducer{}

// ConsensusReducer is the second concrete Policy: it returns the reply
// of the class agreed on by a strict majority of the successful
// Outcomes. It votes over a NORMALIZED form of Response.Message.Content,
// so it is for structured / label output (a class), never free prose —
// two models never write the same paragraph, so prose consensus would
// never fire (ADR-0013 §2).
//
// Both fields are optional; the zero value is valid:
//   - Order is the provider priority used ONLY to pick the
//     representative reply within the winning class and to break
//     representative ties (highest priority first, then lowest fan-out
//     index — the same ranking as PriorityReducer). nil/empty means
//     ties resolve to the lowest fan-out index.
//   - Normalize maps a Response's content to its comparison key. nil
//     defaults to defaultNormalize (trim + lowercase), which makes label
//     voting fire across trivial casing/whitespace differences.
//
// Consensus requires a class held by strictly more than half of the
// successful outcomes AND by at least two of them (ADR-0013 §6): a lone
// successful answer is not a consensus. When usable outcomes disagree
// (no such class), Apply returns ErrNoConsensus. When no outcome is
// usable at all, it returns ErrNoUsableOutcome (ADR-0013 §8).
type ConsensusReducer struct {
	// Order is the provider priority for representative selection.
	Order []string
	// Normalize maps content to its comparison key; nil → defaultNormalize.
	Normalize func(string) string
}

// Apply votes the successful Outcomes by normalized content and returns
// the strict-majority class's representative reply. See the type doc for
// the threshold and tie-break rules. Apply does not consult ctx:
// consensus is a pure function of result and the reducer's config.
//
// Check order (ADR-0013 §8): nil result → ErrNilResult; no usable
// outcome → ErrNoUsableOutcome (with the failures joined behind it);
// usable outcomes but no strict-majority class → ErrNoConsensus; a
// majority → a *Decision whose Response is the winning class's
// representative, with every winning-class member marked Used.
func (r ConsensusReducer) Apply(ctx context.Context, result *fanout.Result) (*Decision, error) {
	if result == nil {
		return nil, ErrNilResult
	}

	normalize := r.Normalize
	if normalize == nil {
		normalize = defaultNormalize
	}

	n := len(result.Outcomes)
	considered := make([]Contribution, n)
	accounting := make([]ProviderCost, n)

	// votes maps a normalized class to the fan-out indices of the usable
	// outcomes that voted it, kept in fan-out order (append in the index
	// loop), so representative selection stays deterministic.
	votes := make(map[string][]int)
	usable := 0
	for i := range result.Outcomes {
		oc := result.Outcomes[i]
		considered[i] = Contribution{Provider: oc.Provider, Err: oc.Err}
		accounting[i] = ProviderCost{Provider: oc.Provider, Latency: oc.Latency}

		// Same usability predicate and conservative invariant handling as
		// PriorityReducer: Err != nil (incl. a contract-breaking both-non-nil
		// Outcome) or a nil Response means no vote.
		if oc.Err != nil || oc.Response == nil {
			continue
		}
		usable++
		// Compute the class key once: Normalize may be an expensive or
		// side-effecting operator function (the ADR keeps the seam open to
		// heavier canonicalizers), so it must run exactly once per outcome.
		key := normalize(oc.Response.Message.Content)
		votes[key] = append(votes[key], i)
	}

	decision := &Decision{
		Provenance: Provenance{Considered: considered},
		Accounting: accounting,
	}

	// No usable outcome: nothing to vote on. Reuse ErrNoUsableOutcome with
	// the failures joined behind it (ADR-0013 §8 / ADR-0012 §5a).
	if usable == 0 {
		errs := make([]error, 0, n+1)
		errs = append(errs, ErrNoUsableOutcome)
		for i := range result.Outcomes {
			if oc := result.Outcomes[i]; oc.Err != nil {
				errs = append(errs, oc.Err)
			}
		}
		return decision, errors.Join(errs...)
	}

	// Find the consensus class: held by a strict majority of the usable
	// outcomes AND by at least two of them. At most one class can satisfy
	// the strict-majority test, so the winner is unique (ADR-0013 §5/§6).
	var winners []int
	for _, idxs := range votes {
		if len(idxs) >= 2 && len(idxs)*2 > usable {
			winners = idxs
			break
		}
	}
	if winners == nil {
		// Usable answers existed but disagreed.
		return decision, ErrNoConsensus
	}

	// Representative: the best-ranked member of the winning class — highest
	// priority by Order, then lowest fan-out index. winners is in fan-out
	// order, so winners[0] is the lowest index; strict `<` keeps it on a
	// rank tie (ADR-0013 §4).
	repIdx := winners[0]
	bestRank := rankByOrder(r.Order, result.Outcomes[repIdx].Provider)
	for _, idx := range winners[1:] {
		if rank := rankByOrder(r.Order, result.Outcomes[idx].Provider); rank < bestRank {
			repIdx = idx
			bestRank = rank
		}
	}

	for _, idx := range winners {
		considered[idx].Used = true // every winning-class member contributed
	}
	decision.Response = result.Outcomes[repIdx].Response
	return decision, nil
}

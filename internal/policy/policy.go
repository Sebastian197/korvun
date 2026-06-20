// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"context"
	"time"

	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/fanout"
)

// Policy reduces the outcomes of a completed fan-out into a single
// Decision. It is the post-dispatch half of the two-phase model
// (ADR-0012 §3): it runs AFTER fanout.Run has returned every Outcome
// and decides what to do with them (select one, vote, merge). A Policy
// never dispatches and never becomes a model.Model — model.Response is
// lossy for the provenance and consensus reasoning the engine exists to
// do (ADR-0012 §1).
//
// Apply receives ctx for interface consistency with future reducers
// that perform I/O (a semantic-equivalence reducer calling a judge
// model needs cancellation). A pure reducer such as PriorityReducer is
// free to ignore it.
type Policy interface {
	Apply(ctx context.Context, result *fanout.Result) (*Decision, error)
}

// Decision is what a Policy produces from a fan-out Result. It is the
// rich type the Brain (Stage 7) consumes: the chosen reply plus enough
// provenance and accounting to log, debug, and (later) reason about the
// choice.
//
// The shape accommodates the full two-phase engine; the first reducer
// (PriorityReducer) fills only the subset selection needs. Fields are
// NOT invented for cases that do not exist yet: there is deliberately no
// consensus-score and no confidence field until a consensus reducer
// needs one and adds it additively (ADR-0012 §2).
type Decision struct {
	// Response is the chosen assistant reply the Brain will fold into an
	// outbound Envelope. It is nil if and only if no usable outcome
	// existed; in that case Apply also returns an error that satisfies
	// errors.Is(err, ErrNoUsableOutcome) (ADR-0012 §5a).
	Response *model.Response

	// Provenance records which Outcomes the policy considered and how
	// each was treated. It is the debugging surface for the no-code
	// builder (Stage 14) and the seed the consensus reducer extends
	// later.
	Provenance Provenance

	// Accounting is the per-provider cost/latency record, in fan-out
	// order. In v1 it carries Latency only (free from Outcome.Latency); a
	// monetary cost field is added when a cost model exists (ADR-0012 §5c).
	Accounting []ProviderCost
}

// Provenance answers "which Outcomes contributed, and how?". Considered
// lists every Outcome the policy looked at, in deterministic fan-out
// order (Considered[i] corresponds to Result.Outcomes[i]).
type Provenance struct {
	// Considered holds one Contribution per Outcome the policy examined.
	Considered []Contribution
}

// Contribution is one Outcome's role in the Decision. It records that an
// outcome "contributed", not that it "won" — a future merge/consensus
// reducer may mark several entries Used when it synthesises a reply.
//
// There is intentionally no vote / agreement / score field yet; a
// consensus reducer adds one additively when it ships (ADR-0012 §2).
type Contribution struct {
	// Provider is the source provider name (Outcome.Provider, i.e.
	// Model.Name()).
	Provider string

	// Used marks the outcome(s) that fed Decision.Response — exactly one
	// for selection, possibly several for a future synthesis reducer.
	Used bool

	// Err is non-nil when this outcome failed in the fan-out. It carries
	// the upstream sentinel grammar untouched, so errors.Is / errors.As
	// keep working against it.
	Err error
}

// ProviderCost is the per-provider accounting row. Latency is copied
// from Outcome.Latency. A monetary cost field is deferred until a cost
// model exists (ADR-0012 §5c); adding it later is additive.
type ProviderCost struct {
	// Provider is the source provider name (Outcome.Provider).
	Provider string

	// Latency is the wall-clock time the provider's call spent inside
	// Model.Generate, copied from Outcome.Latency.
	Latency time.Duration
}

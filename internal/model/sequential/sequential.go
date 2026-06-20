// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sequential

import (
	"context"
	"time"

	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/fanout"
)

// Coordinator dispatches a single model.Request to N model.Model
// implementations SERIALLY, stopping at the first success (ADR-0016).
//
// Construct with New. The zero value works for ONE-SHOT use (Run lazily
// defaults the clock on first call), but reuse — and especially CONCURRENT
// reuse — requires construction via New. Reason: the zero-value defense in Run
// writes c.now without synchronization, which would race against a concurrent
// Run; New initializes c.now eagerly so subsequent Run calls only read it. This
// mirrors fanout.Coordinator exactly, reusing the P2 lesson rather than
// re-learning it. Run itself spawns no goroutine; all per-call state is local.
type Coordinator struct {
	perModelTimeout time.Duration
	// now is the package-private clock seam passed through to fanout.CallOne.
	// New sets it to time.Now; Run defends a nil value (zero-value Coordinator).
	now func() time.Time
}

// Option configures a Coordinator at New time.
type Option func(*Coordinator)

// WithPerModelTimeout bounds each model's call duration. The bound is applied
// inside fanout.CallOne (each call derives its own ctx via
// context.WithTimeout). A non-positive d is rejected (no-op), matching
// fanout.WithPerModelTimeout: passing 0 or a negative duration to
// context.WithTimeout returns an already-cancelled ctx, which would fail every
// call instantly with no upstream signal.
func WithPerModelTimeout(d time.Duration) Option {
	return func(c *Coordinator) {
		if d > 0 {
			c.perModelTimeout = d
		}
	}
}

// New constructs a Coordinator with the given options. The clock is set to
// time.Now; perModelTimeout is zero (unset) unless WithPerModelTimeout is
// supplied with a positive duration.
func New(opts ...Option) *Coordinator {
	c := &Coordinator{
		now: time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Run dispatches req to each model in input order and returns at the first
// success. It returns a *fanout.Result so any policy reducer consumes it
// unchanged (ADR-0016 §5).
//
// Returns a non-nil error ONLY for mechanism-level problems, via the shared
// fanout.ValidateRunInputs: nil ctx, request validation failure, empty models
// (ErrNoModels), nil model entry (ErrNilModel), or a ctx already cancelled at
// entry. None of these call any model.
//
// Otherwise Run is mechanism success and returns (*Result, nil):
//   - Result.Outcomes holds one Outcome per model ACTUALLY called, in call
//     order. A model skipped after an earlier success — or after the caller's
//     ctx is cancelled between calls — is absent (the slice is shorter than
//     models). Absence means "not called"; a present Outcome with Err != nil
//     means "called and failed".
//   - The stop predicate is mechanical: the first Outcome with Err == nil is a
//     success and ends the loop (ADR-0016 §3). By the Outcome invariant
//     (exactly one of Response/Err non-nil), Err == nil implies a usable
//     Response.
//   - If every called model fails (all-failed), Run still returns (*Result,
//     nil) with every Outcome present; a downstream reducer turns that into
//     ErrNoUsableOutcome, joining the per-provider causes. The mechanism
//     reports what happened; the policy decides it means "no usable answer".
//
// Per-model panic, error, and latency are handled identically to the fan-out
// because Run calls the same fanout.CallOne primitive; the upstream sentinel
// grammar is preserved (errors.Is / errors.As keep working through Outcome.Err).
func (c *Coordinator) Run(
	ctx context.Context,
	req *model.Request,
	models []model.Model,
) (*fanout.Result, error) {
	if err := fanout.ValidateRunInputs(ctx, req, models); err != nil {
		return nil, err
	}

	// Zero-value defense: a Coordinator constructed by `var c Coordinator`
	// (skipping New) has c.now == nil; default lazily. SAFETY: this write is
	// one-shot-only — concurrent reuse MUST go through New (see the type doc).
	// Run is single-goroutine, so there is no in-Run race; the only hazard is
	// two concurrent Runs on a zero-value Coordinator, which New removes.
	if c.now == nil {
		c.now = time.Now
	}

	outcomes := make([]fanout.Outcome, 0, len(models))
	for _, m := range models {
		// Honour a caller cancellation observed BETWEEN calls: stop rather
		// than call the next model with a doomed ctx. The remaining models are
		// left absent from Outcomes. A cancellation DURING a call surfaces as
		// that Outcome's Err (ctx flows into Generate through CallOne).
		if ctx.Err() != nil {
			break
		}

		oc := fanout.CallOne(ctx, req, m, c.perModelTimeout, c.now)
		outcomes = append(outcomes, oc)

		if oc.Err == nil {
			break // first success — stop (ADR-0016 §3); the rest are not called.
		}
	}

	return &fanout.Result{Outcomes: outcomes}, nil
}

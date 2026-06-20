// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package fanout

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Sebastian197/korvun/internal/model"
)

// Outcome is one model's contribution to a fan-out call. Exactly one
// of Response or Err is non-nil. Provider is always Model.Name() of
// the source adapter (not the provider-self-declared
// model.Response.Provider) so attribution is consistent between
// success and failure outcomes and so the policy layer can route by
// the identity Korvun controls, not the value the adapter declares
// about itself. The self-declared label remains available at
// Outcome.Response.Provider for callers that want it. Latency is the
// wall-clock time the goroutine spent inside Model.Generate; observability
// that costs nothing to capture at this layer.
type Outcome struct {
	Provider string
	Response *model.Response
	Err      error
	Latency  time.Duration
}

// Result is what Coordinator.Run returns when the mechanism completed
// (every goroutine finished; the caller's ctx may or may not have
// fired during the call). Per-model failures live in Outcome.Err;
// they are NOT Result-level errors.
//
// Outcomes has the same length as the input []Model, in the same
// order. Outcomes[i] is the result for the i-th input model.
type Result struct {
	Outcomes []Outcome
}

// Coordinator dispatches a single model.Request to N model.Model
// implementations in parallel and collects every outcome. Construct
// with New; the zero value is defended against (Run lazily defaults
// the clock) but explicit construction is preferred.
//
// A Coordinator instance is safe for concurrent reuse: multiple Run
// invocations on the same Coordinator do not interfere because all
// per-call state (outcomes slice, derived ctx, WaitGroup) is created
// inside Run.
type Coordinator struct {
	perModelTimeout time.Duration
	// now is the package-private clock seam. New sets it to time.Now;
	// tests in the same package may assign a fake. Run defends against
	// a nil value (zero-value Coordinator).
	now func() time.Time
}

// Option configures a Coordinator at New time.
type Option func(*Coordinator)

// WithPerModelTimeout bounds the per-model call duration. Each child
// goroutine derives its own ctx via context.WithTimeout(runCtx, d).
// A non-positive d is rejected (no-op) — passing 0 or a negative
// duration to context.WithTimeout returns an already-cancelled ctx,
// which would fire ctx.Err() on every child at zero ms with no
// upstream signal. Use a positive value, or omit the option to leave
// every child sharing the caller-derived ctx alone.
func WithPerModelTimeout(d time.Duration) Option {
	return func(c *Coordinator) {
		if d > 0 {
			c.perModelTimeout = d
		}
	}
}

// New constructs a Coordinator with the given options. The clock is
// set to time.Now; perModelTimeout is zero (unset) unless
// WithPerModelTimeout is supplied with a positive duration.
func New(opts ...Option) *Coordinator {
	c := &Coordinator{
		now: time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Run dispatches req to every model in parallel and returns when
// every goroutine has finished. Returns *Result for mechanism-level
// success (input was valid, every goroutine returned) regardless of
// per-model failures.
//
// Returns a non-nil error ONLY for mechanism-level problems: nil
// ctx, nil request, validation failure on req (any of the
// model.Err* validation sentinels), empty models slice (ErrNoModels),
// nil model entry (ErrNilModel), or the caller's ctx is already
// cancelled at entry. None of these spawn goroutines.
//
// A model that fails — for any reason: provider error, ctx
// cancellation mid-flight, panic — surfaces as Outcomes[i].Err with
// the upstream sentinel grammar preserved. Per-model failures are
// NOT Run-level errors.
//
// Run blocks until every spawned goroutine returns. An adapter that
// ignores ctx will block Run; this is the cooperative-cancellation
// invariant documented in ADR-0011 §2.
func (c *Coordinator) Run(
	ctx context.Context,
	req *model.Request,
	models []model.Model,
) (*Result, error) {
	if ctx == nil {
		return nil, errors.New("fanout: nil ctx")
	}
	if err := model.ValidateRequest(req); err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, ErrNoModels
	}
	for _, m := range models {
		if m == nil {
			return nil, ErrNilModel
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Zero-value defense: a Coordinator constructed by `var c Coordinator`
	// (skipping New) has c.now == nil; default lazily rather than panic.
	if c.now == nil {
		c.now = time.Now
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	outcomes := make([]Outcome, len(models))
	var wg sync.WaitGroup
	wg.Add(len(models))

	for i, m := range models {
		go c.callOne(runCtx, req, m, &outcomes[i], &wg)
	}
	wg.Wait()

	return &Result{Outcomes: outcomes}, nil
}

// callOne runs Model.Generate for a single model and writes the
// outcome into the pre-allocated slot. The combination of
// defer wg.Done (runs last) and defer recover (runs first) ensures
// that any user-mode panic in Generate or Name surfaces as a failed
// Outcome with the "fanout: provider panicked:" prefix, and wg.Wait
// always returns. Per-slot isolation means each goroutine writes
// only to outcomes[i] for a unique i; no two goroutines touch the
// same memory location. WaitGroup.Done → Wait is the synchronisation
// fence the post-Wait read of outcomes relies on.
func (c *Coordinator) callOne(
	runCtx context.Context, req *model.Request, m model.Model,
	out *Outcome, wg *sync.WaitGroup,
) {
	defer wg.Done()
	defer func() {
		if r := recover(); r != nil {
			// If the panic value is itself an error, wrap it with %w so
			// errors.Is / errors.As against the original sentinel (e.g.
			// model.ErrAuthInvalid panicked by a buggy adapter) keep
			// working through the fan-out boundary. ADR-0011 §3 promises
			// the upstream sentinel grammar is preserved untouched; %v
			// would stringify it and lose the chain.
			if e, ok := r.(error); ok {
				out.Err = fmt.Errorf("fanout: provider panicked: %w", e)
			} else {
				out.Err = fmt.Errorf("fanout: provider panicked: %v", r)
			}
		}
	}()

	out.Provider = m.Name()

	callCtx := runCtx
	if c.perModelTimeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(runCtx, c.perModelTimeout)
		defer cancel()
	}

	start := c.now()
	resp, err := m.Generate(callCtx, req)
	out.Latency = c.now().Sub(start)
	if err != nil {
		out.Err = err
		return
	}
	out.Response = resp
}

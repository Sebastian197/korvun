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
// implementations in parallel and collects every outcome.
//
// Construct with New. The zero value works for ONE-SHOT use (Run
// lazily defaults the clock on first call), but reuse — and
// especially CONCURRENT reuse — requires construction via New.
// Reason: the zero-value defense at the top of Run writes c.now
// without synchronization, which races against the c.now() reads
// inside still-running child goroutines of a concurrent Run. New
// initializes c.now eagerly so subsequent Run calls only read the
// field, making the post-New Coordinator safe for concurrent reuse:
// all per-call state (outcomes slice, derived ctx, WaitGroup) is
// created inside Run, and the configuration fields (perModelTimeout,
// now) are read-only after construction.
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

// ValidateRunInputs checks the mechanism-level preconditions shared by
// every dispatch shape before any model is called: a non-nil ctx, a valid
// request (any of the model.Err* validation sentinels), a non-empty model
// slice (ErrNoModels), no nil model entry (ErrNilModel), and a ctx not
// already cancelled at entry. It returns the same sentinels callers match
// with errors.Is. Both fanout.Coordinator.Run and the sequential
// coordinator (ADR-0016) call it so the two reject identical
// misconfigurations identically. The nil-ctx message is
// dispatch-shape-neutral so a sequential error never falsely reads
// "fanout".
func ValidateRunInputs(ctx context.Context, req *model.Request, models []model.Model) error {
	if ctx == nil {
		return errors.New("model dispatch: nil ctx")
	}
	if err := model.ValidateRequest(req); err != nil {
		return err
	}
	if len(models) == 0 {
		return ErrNoModels
	}
	for _, m := range models {
		if m == nil {
			return ErrNilModel
		}
	}
	return ctx.Err()
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
//
// Callers MUST supply distinct model.Model instances. The model.Model
// contract does NOT promise Generate is concurrency-safe on a single
// instance; passing the same adapter twice in models would invoke
// Generate from two goroutines on one instance and risk data races
// in any adapter that holds per-call mutable state. The two
// production adapters (internal/model/ollama and internal/model/groq)
// happen to be safe because they only read fields after construction,
// but future adapters are not required to be. Run does not detect
// duplicates.
func (c *Coordinator) Run(
	ctx context.Context,
	req *model.Request,
	models []model.Model,
) (*Result, error) {
	if err := ValidateRunInputs(ctx, req, models); err != nil {
		return nil, err
	}

	// Zero-value defense: a Coordinator constructed by `var c Coordinator`
	// (skipping New) has c.now == nil; default lazily rather than panic.
	// SAFETY: this write is one-shot-only — callers that reuse a
	// Coordinator concurrently MUST go through New (see the type doc).
	// The single-Run path is race-free because the c.now() reads inside
	// the child goroutines are synchronized by the WaitGroup against
	// THIS write (Wait happens-after Done; Done happens-after every
	// callOne read). Concurrent Run calls on a zero-value Coordinator
	// would NOT have that fence and would race; New initializes c.now
	// eagerly to remove the question.
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

// callOne is the fan-out's goroutine body: it runs the shared CallOne
// primitive for a single model and writes the result into the
// pre-allocated slot. The combination of defer wg.Done and CallOne's own
// internal recover ensures any user-mode panic in Generate or Name
// surfaces as a failed Outcome (never escaping the goroutine) and wg.Wait
// always returns. Per-slot isolation means each goroutine writes only to
// outcomes[i] for a unique i; no two goroutines touch the same memory
// location. WaitGroup.Done → Wait is the synchronisation fence the
// post-Wait read of outcomes relies on.
func (c *Coordinator) callOne(
	runCtx context.Context, req *model.Request, m model.Model,
	out *Outcome, wg *sync.WaitGroup,
) {
	defer wg.Done()
	*out = CallOne(runCtx, req, m, c.perModelTimeout, c.now)
}

// CallOne runs Model.Generate for a single model and returns its Outcome.
// It is the shared per-call primitive of the model-dispatch mechanism: the
// parallel Coordinator wraps it in a goroutine + WaitGroup, and the
// sequential coordinator (ADR-0016) calls it in a serial loop. Having ONE
// implementation keeps the upstream sentinel grammar preserved in exactly
// one place — re-deriving it per dispatch shape is how the P1 %w bug crept
// in (the e633874 fix; ADR-0011 §3). CallOne holds no state and spawns no
// goroutine.
//
// It applies, in order:
//   - a recover that turns any user-mode panic in Name or Generate into a
//     failed Outcome. An error panic value is wrapped with %w so
//     errors.Is / errors.As against the original sentinel (e.g.
//     model.ErrAuthInvalid panicked by a buggy adapter) keep working
//     through the boundary; a non-error value is rendered with %v. The
//     prefix is dispatch-shape-neutral ("model dispatch: provider
//     panicked:") so a sequential outcome never falsely reads "fanout".
//   - the optional per-model timeout (skipped when perModelTimeout <= 0):
//     the call derives its own ctx via context.WithTimeout.
//   - latency capture via the now clock. start is read AFTER the potential
//     m.Name() panic, and the Latency defer is registered AFTER start, so
//     an unset start can never produce a bogus epoch-relative Latency; when
//     m.Name() panics first, neither is reached and Latency reads 0, which
//     matches "wall-clock time spent inside Model.Generate" (Generate was
//     never entered).
//
// now MUST be non-nil; both Coordinators guarantee it (New sets time.Now;
// Run defends a zero-value Coordinator before calling).
func CallOne(
	ctx context.Context, req *model.Request, m model.Model,
	perModelTimeout time.Duration, now func() time.Time,
) (out Outcome) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				out.Err = fmt.Errorf("model dispatch: provider panicked: %w", e)
			} else {
				out.Err = fmt.Errorf("model dispatch: provider panicked: %v", r)
			}
		}
	}()

	out.Provider = m.Name()

	callCtx := ctx
	if perModelTimeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, perModelTimeout)
		defer cancel()
	}

	start := now()
	defer func() {
		out.Latency = now().Sub(start)
	}()

	resp, err := m.Generate(callCtx, req)
	if err != nil {
		out.Err = err
		return
	}
	out.Response = resp
	return
}

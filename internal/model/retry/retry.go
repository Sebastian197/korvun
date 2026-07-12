// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package retry provides a per-instance decorator over model.Model that owns the
// per-attempt deadline for every dispatch shape and retries ONLY genuinely
// transient post-load errors (ADR-0031 Decisions 4 and 5). It is built so it
// never fires for the cold-load case (F6): a per-attempt deadline expiry is
// non-retryable. The decorator mirrors the brain.WithModelID pattern (wrap,
// delegate, delegate Name()).
package retry

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/Sebastian197/korvun/internal/model"
)

// Backoff schedule constants (ADR-0031 Decision 4, FR-B1/FR-A1). Exported so the
// app's ceiling derivation can budget the worst-case backoff
// (backoffBudget_i = MaxRetries_i × MaxBackoffPerWait, FR-A3).
const (
	// BackoffBase is the first backoff step; each retry doubles it.
	BackoffBase = 200 * time.Millisecond
	// MaxBackoffPerWait caps a single backoff wait (the exponential is clamped
	// here). Full jitter draws uniformly in [0, min(cap, base·2ⁿ)).
	MaxBackoffPerWait = 2 * time.Second
	// RetryAfterCap caps how long a 429's Retry-After hint may push a wait, so a
	// hostile or erroneous Retry-After cannot blow the derived ceiling. The wait
	// is further bounded at runtime by the remaining parent budget (FR-A2).
	RetryAfterCap = 30 * time.Second
)

// Clock abstracts the backoff sleep so tests never wait in wall-clock time.
// Sleep must honor ctx: a cancel during the wait returns promptly with a
// non-nil error.
type Clock interface {
	Sleep(ctx context.Context, d time.Duration) error
}

// Config holds the per-instance retry parameters resolved from config.
// PerAttempt is the per-attempt deadline (Config.EffectiveRequestTimeout);
// MaxRetries is the extra-attempt budget (0 disables retry — the decorator then
// still applies the per-attempt deadline, SV3).
type Config struct {
	PerAttempt time.Duration
	MaxRetries int
}

// Option configures the decorator's test seams.
type Option func(*decorator)

// WithClock overrides the backoff clock (default: a real time-backed clock).
func WithClock(c Clock) Option { return func(d *decorator) { d.clock = c } }

// WithRand overrides the full-jitter source, a func returning a fraction in
// [0,1) (default: math/rand/v2's concurrent-safe Float64).
func WithRand(next func() float64) Option { return func(d *decorator) { d.rnd = next } }

// decorator wraps a model.Model with the per-attempt deadline + transient-only
// retry loop. One decorator per model instance; safe under concurrent Generate
// (no shared mutable state; the default rand source is concurrent-safe).
type decorator struct {
	inner model.Model
	cfg   Config
	clock Clock
	rnd   func() float64
}

// New builds a retry decorator over inner. Defaults: a real clock and
// math/rand/v2's Float64 for jitter, overridable via options for deterministic
// tests.
func New(inner model.Model, cfg Config, opts ...Option) model.Model {
	d := &decorator{
		inner: inner,
		cfg:   cfg,
		clock: realClock{},
		rnd:   rand.Float64,
	}
	for _, o := range opts {
		o(d)
	}
	return d
}

// Name delegates to the wrapped model, so fan-out attribution stays the provider
// identity (mirrors named.Name()).
func (d *decorator) Name() string { return d.inner.Name() }

// Generate applies a per-attempt deadline on every attempt (including the 0th,
// with retry on or off) and retries only transient post-load errors. The
// classification order is load-bearing (ADR-0031 Decision 5):
//
//  1. parent ctx cancelled/expired  -> stop (F3): shutdown, ceiling, or a
//     fan-out-cancelled sibling. The PARENT ctx is inspected, not the derived
//     per-attempt ctx.
//  2. context.DeadlineExceeded in the error, parent still alive -> stop (F6):
//     it was OUR per-attempt that expired; retrying re-triggers a cold load.
//  3. *RateLimitError -> retry, waiting max(jittered backoff, capped
//     Retry-After); give up without sleeping if the wait exceeds the remaining
//     parent budget (FR-A2).
//  4. ErrProviderUnavailable (fast, not a deadline) -> retry with full jitter.
//  5. anything else (auth, bad response, validation) -> stop, not retryable.
func (d *decorator) Generate(ctx context.Context, req *model.Request) (*model.Response, error) {
	for attempt := 0; ; attempt++ {
		resp, err := d.attempt(ctx, req)
		if err == nil {
			return resp, nil
		}

		// (1) F3: the parent/ceiling context is gone — give up now, never retry
		// against a dead context. Checked before the error is classified because
		// a cancelled parent and an expired per-attempt both surface as
		// ErrProviderUnavailable wrapping a ctx error.
		if ctx.Err() != nil {
			return nil, err
		}
		// (2) F6: our own per-attempt deadline expired (parent still alive) —
		// non-retryable; the fix is a larger timeout, not more retries.
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}

		// (3)/(4) classify and compute the wait; a non-retryable error stops.
		wait, retryable := d.waitFor(err, attempt)
		if !retryable {
			return nil, err
		}
		// Attempt budget exhausted — return the last error.
		if attempt >= d.cfg.MaxRetries {
			return nil, err
		}
		// (FR-A2) never sleep past the remaining parent budget.
		if dl, ok := ctx.Deadline(); ok && wait > time.Until(dl) {
			return nil, err
		}
		if serr := d.clock.Sleep(ctx, wait); serr != nil {
			return nil, err
		}
	}
}

// attempt runs one underlying Generate under a fresh per-attempt deadline
// derived from the parent ctx (PerAttempt <= 0 leaves the call bounded only by
// the parent, though config always resolves a positive value).
func (d *decorator) attempt(ctx context.Context, req *model.Request) (*model.Response, error) {
	if d.cfg.PerAttempt <= 0 {
		return d.inner.Generate(ctx, req)
	}
	attemptCtx, cancel := context.WithTimeout(ctx, d.cfg.PerAttempt)
	defer cancel()
	return d.inner.Generate(attemptCtx, req)
}

// waitFor classifies a retryable error and returns the wait before the next
// attempt. It returns retryable=false for non-transient errors.
func (d *decorator) waitFor(err error, attempt int) (time.Duration, bool) {
	var rle *model.RateLimitError
	switch {
	case errors.As(err, &rle):
		return maxDuration(d.backoff(attempt), cappedRetryAfter(rle.RetryAfter)), true
	case errors.Is(err, model.ErrProviderUnavailable):
		return d.backoff(attempt), true
	default:
		return 0, false
	}
}

// backoff returns the full-jitter wait for the given 0-based attempt index:
// a uniform draw in [0, min(MaxBackoffPerWait, BackoffBase·2ⁿ)).
func (d *decorator) backoff(attempt int) time.Duration {
	step := BackoffBase << attempt
	if step <= 0 || step > MaxBackoffPerWait { // clamp (and guard shift overflow)
		step = MaxBackoffPerWait
	}
	return time.Duration(d.rnd() * float64(step))
}

// cappedRetryAfter clamps a Retry-After hint to [0, RetryAfterCap].
func cappedRetryAfter(ra time.Duration) time.Duration {
	switch {
	case ra <= 0:
		return 0
	case ra > RetryAfterCap:
		return RetryAfterCap
	default:
		return ra
	}
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

// realClock is the default time-backed Clock. Sleep returns early with the ctx
// error if the context is cancelled during the wait.
type realClock struct{}

func (realClock) Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package retry

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/fanout"
)

// This file is the RED phase of ADR-0031 sub-phase 4 (the retry decorator).
// It references retry.New, retry.Config, retry.WithClock, retry.WithRand, and
// the exported backoff/cap constants — none of which exist yet, so the package
// fails to build. That compile failure IS the red for the decorator.
//
// Every test uses the injected fake clock and a deterministic rand: ZERO real
// sleeps, no wall-clock waits except the tiny real per-attempt ctx expiry in the
// F6/SV3 tests (a context deadline, not a time.Sleep).

// --- test doubles -----------------------------------------------------------

// scriptedModel returns a programmed (resp, err) per invocation; once the script
// is exhausted the LAST entry repeats (handy for "persistent transient error").
// An optional block hook lets a test make Generate wait on the per-attempt ctx.
type scriptedModel struct {
	name    string
	mu      sync.Mutex
	calls   int
	results []result
	block   func(ctx context.Context) (*model.Response, error, bool) // returns (resp,err,handled)
}

type result struct {
	resp *model.Response
	err  error
}

func (s *scriptedModel) Generate(ctx context.Context, _ *model.Request) (*model.Response, error) {
	s.mu.Lock()
	i := s.calls
	s.calls++
	s.mu.Unlock()
	if s.block != nil {
		if resp, err, handled := s.block(ctx); handled {
			return resp, err
		}
	}
	if i < len(s.results) {
		return s.results[i].resp, s.results[i].err
	}
	last := s.results[len(s.results)-1]
	return last.resp, last.err
}

func (s *scriptedModel) Name() string { return s.name }

func (s *scriptedModel) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// fakeClock records requested sleep durations and never waits in wall-clock
// time. If sleepErr is set it is returned (simulating a cancel-during-sleep).
type fakeClock struct {
	mu       sync.Mutex
	waits    []time.Duration
	sleepErr error
}

func (c *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	c.waits = append(c.waits, d)
	err := c.sleepErr
	c.mu.Unlock()
	return err
}

func (c *fakeClock) recorded() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]time.Duration, len(c.waits))
	copy(out, c.waits)
	return out
}

// fixedRand returns a constant jitter fraction, so full-jitter waits are
// deterministic: wait = frac * min(cap, base·2ⁿ).
func fixedRand(frac float64) func() float64 { return func() float64 { return frac } }

func okResp(name string) *model.Response {
	return &model.Response{
		Message:  model.Message{Role: model.RoleAssistant, Content: "ok"},
		Provider: name,
	}
}

func validReq() *model.Request {
	return &model.Request{
		Model:    "m",
		Messages: []model.Message{{Role: model.RoleUser, Content: "hi"}},
	}
}

// deadlineErr builds the EXACT error the adapters return on an in-flight
// per-attempt expiry: ErrProviderUnavailable wrapping context.DeadlineExceeded
// (ollama.go:117 / groq.go:171 use the same `%w: %w` double-wrap).
func deadlineErr() error {
	return fmt.Errorf("%w: %w", model.ErrProviderUnavailable, context.DeadlineExceeded)
}

// --- FR-R* classification table --------------------------------------------

func TestGenerate_classification(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		results    []result
		maxRetries int
		wantCalls  int
		wantErrIs  error // errors.Is on the final error; nil => expect success
		fr         string
	}{
		{
			name:       "fast Unavailable then success -> retried",
			results:    []result{{err: fmt.Errorf("%w: refused", model.ErrProviderUnavailable)}, {resp: okResp("m")}},
			maxRetries: 2, wantCalls: 2, wantErrIs: nil, fr: "FR-R4",
		},
		{
			name:       "AuthInvalid -> never retried",
			results:    []result{{err: fmt.Errorf("%w: 401", model.ErrAuthInvalid)}},
			maxRetries: 3, wantCalls: 1, wantErrIs: model.ErrAuthInvalid, fr: "FR-R5",
		},
		{
			name:       "ProviderResponse -> never retried",
			results:    []result{{err: fmt.Errorf("%w: 400", model.ErrProviderResponse)}},
			maxRetries: 3, wantCalls: 1, wantErrIs: model.ErrProviderResponse, fr: "FR-R6",
		},
		{
			name:       "validation sentinel -> never retried",
			results:    []result{{err: model.ErrEmptyContent}},
			maxRetries: 3, wantCalls: 1, wantErrIs: model.ErrEmptyContent, fr: "FR-R7",
		},
		{
			name:       "persistent Unavailable -> exactly maxRetries+1 calls",
			results:    []result{{err: fmt.Errorf("%w: 503", model.ErrProviderUnavailable)}},
			maxRetries: 2, wantCalls: 3, wantErrIs: model.ErrProviderUnavailable, fr: "FR-R8",
		},
		{
			name:       "maxRetries 0 disables retry for a retryable error",
			results:    []result{{err: fmt.Errorf("%w: 503", model.ErrProviderUnavailable)}},
			maxRetries: 0, wantCalls: 1, wantErrIs: model.ErrProviderUnavailable, fr: "FR-C2/off",
		},
		{
			name:       "success on first attempt -> passthrough",
			results:    []result{{resp: okResp("m")}},
			maxRetries: 2, wantCalls: 1, wantErrIs: nil, fr: "passthrough",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &scriptedModel{name: "m", results: tc.results}
			clk := &fakeClock{}
			d := New(m, Config{PerAttempt: time.Second, MaxRetries: tc.maxRetries},
				WithClock(clk), WithRand(fixedRand(1.0)))
			resp, err := d.Generate(context.Background(), validReq())
			if tc.wantErrIs == nil {
				if err != nil {
					t.Fatalf("[%s] err = %v, want success", tc.fr, err)
				}
				if resp == nil {
					t.Fatalf("[%s] resp = nil, want a response", tc.fr)
				}
			} else if !errors.Is(err, tc.wantErrIs) {
				t.Fatalf("[%s] err = %v, want errors.Is %v", tc.fr, err, tc.wantErrIs)
			}
			if got := m.callCount(); got != tc.wantCalls {
				t.Errorf("[%s] calls = %d, want %d", tc.fr, got, tc.wantCalls)
			}
		})
	}
}

// --- THE load-bearing test: F6 (deadline) beats R4 (Unavailable) ------------

// TestGenerate_deadlineBeatsUnavailable_orderIsLoadBearing pins FR-R2 BEFORE
// FR-R4: an error shaped EXACTLY like the adapters' in-flight per-attempt expiry
// (ErrProviderUnavailable wrapping context.DeadlineExceeded), with the PARENT
// ALIVE, must NOT be retried. If the classifier tested the Unavailable class
// first, it would retry (3 calls) and this test would bite.
func TestGenerate_deadlineBeatsUnavailable_orderIsLoadBearing(t *testing.T) {
	t.Parallel()
	m := &scriptedModel{name: "m", results: []result{{err: deadlineErr()}}}
	clk := &fakeClock{}
	d := New(m, Config{PerAttempt: time.Second, MaxRetries: 2},
		WithClock(clk), WithRand(fixedRand(1.0)))

	_, err := d.Generate(context.Background(), validReq()) // parent alive (background)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want wrap of context.DeadlineExceeded", err)
	}
	if got := m.callCount(); got != 1 {
		t.Errorf("calls = %d, want exactly 1 (per-attempt expiry is non-retryable — F6)", got)
	}
	if got := clk.recorded(); len(got) != 0 {
		t.Errorf("recorded sleeps = %v, want none (no retry)", got)
	}
}

// --- FR-R1 parent guard ------------------------------------------------------

// TestGenerate_parentCancelled_stops covers FR-R1(a): a parent cancelled between
// attempts yields zero further calls.
func TestGenerate_parentCancelled_stops(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	m := &scriptedModel{name: "m", block: func(c context.Context) (*model.Response, error, bool) {
		cancel() // cancel the parent mid-flight, before returning a retryable error
		return nil, fmt.Errorf("%w: 503", model.ErrProviderUnavailable), true
	}}
	clk := &fakeClock{}
	d := New(m, Config{PerAttempt: time.Second, MaxRetries: 3},
		WithClock(clk), WithRand(fixedRand(1.0)))

	_, err := d.Generate(ctx, validReq())
	if err == nil {
		t.Fatal("err = nil, want the underlying failure")
	}
	if got := m.callCount(); got != 1 {
		t.Errorf("calls = %d, want 1 (parent cancelled -> no retry, F3)", got)
	}
	if got := clk.recorded(); len(got) != 0 {
		t.Errorf("recorded sleeps = %v, want none", got)
	}
}

// TestGenerate_parentDeadlineExpired_R1BeatsR2 covers FR-R1(b): when the PARENT
// deadline itself has expired, the decorator stops even though the error wraps
// DeadlineExceeded — R1 (give up) is evaluated before R2. Distinct from the
// load-bearing test where the parent is alive (R2 fires there).
func TestGenerate_parentDeadlineExpired_R1BeatsR2(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done() // parent is now expired before the call

	m := &scriptedModel{name: "m", results: []result{{err: deadlineErr()}}}
	clk := &fakeClock{}
	d := New(m, Config{PerAttempt: time.Second, MaxRetries: 3},
		WithClock(clk), WithRand(fixedRand(1.0)))

	_, err := d.Generate(ctx, validReq())
	if err == nil {
		t.Fatal("err = nil, want a failure")
	}
	if got := m.callCount(); got != 1 {
		t.Errorf("calls = %d, want 1 (expired parent -> stop, R1 beats R2)", got)
	}
}

// --- FR-R3 + FR-A1/A2 rate-limit + cap + budget ----------------------------

func TestGenerate_rateLimit_honorsRetryAfter(t *testing.T) {
	t.Parallel()
	// RetryAfter 7s (< cap) then success: wait = max(backoff, 7s) = 7s.
	m := &scriptedModel{name: "m", results: []result{
		{err: fmt.Errorf("rate: %w", &model.RateLimitError{Provider: "m", RetryAfter: 7 * time.Second})},
		{resp: okResp("m")},
	}}
	clk := &fakeClock{}
	d := New(m, Config{PerAttempt: time.Second, MaxRetries: 2},
		WithClock(clk), WithRand(fixedRand(1.0)))

	if _, err := d.Generate(context.Background(), validReq()); err != nil {
		t.Fatalf("err = %v, want success after honoring RetryAfter", err)
	}
	waits := clk.recorded()
	if len(waits) != 1 || waits[0] != 7*time.Second {
		t.Errorf("recorded waits = %v, want exactly [7s]", waits)
	}
}

func TestGenerate_rateLimit_capsRetryAfter(t *testing.T) {
	t.Parallel()
	// RetryAfter 120s is capped to RetryAfterCap (30s). Parent has no deadline,
	// so the budget guard does not trip.
	m := &scriptedModel{name: "m", results: []result{
		{err: fmt.Errorf("rate: %w", &model.RateLimitError{Provider: "m", RetryAfter: 120 * time.Second})},
		{resp: okResp("m")},
	}}
	clk := &fakeClock{}
	d := New(m, Config{PerAttempt: time.Second, MaxRetries: 2},
		WithClock(clk), WithRand(fixedRand(1.0)))

	if _, err := d.Generate(context.Background(), validReq()); err != nil {
		t.Fatalf("err = %v, want success", err)
	}
	waits := clk.recorded()
	if len(waits) != 1 || waits[0] != RetryAfterCap {
		t.Errorf("recorded waits = %v, want exactly [%v] (capped)", waits, RetryAfterCap)
	}
}

func TestGenerate_rateLimit_budgetGuard_givesUpWithoutSleeping(t *testing.T) {
	t.Parallel()
	// Parent deadline ~50ms; the capped wait (30s) far exceeds the remaining
	// budget, so the decorator gives up WITHOUT sleeping (FR-A2).
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	m := &scriptedModel{name: "m", results: []result{
		{err: fmt.Errorf("rate: %w", &model.RateLimitError{Provider: "m", RetryAfter: 30 * time.Second})},
	}}
	clk := &fakeClock{}
	d := New(m, Config{PerAttempt: time.Second, MaxRetries: 3},
		WithClock(clk), WithRand(fixedRand(1.0)))

	if _, err := d.Generate(ctx, validReq()); err == nil {
		t.Fatal("err = nil, want give-up error")
	}
	if got := m.callCount(); got != 1 {
		t.Errorf("calls = %d, want 1 (give up, no retry)", got)
	}
	if got := clk.recorded(); len(got) != 0 {
		t.Errorf("recorded sleeps = %v, want none (never sleep past the budget — FR-A2)", got)
	}
}

// --- FR-B1 exact backoff shape ---------------------------------------------

// TestGenerate_backoffShape pins the full-jitter schedule with a deterministic
// rand (frac=1.0): wait_n = min(MaxBackoffPerWait, BackoffBase·2ⁿ). With 5
// retries the last waits saturate at the per-wait cap.
func TestGenerate_backoffShape(t *testing.T) {
	t.Parallel()
	m := &scriptedModel{name: "m", results: []result{{err: fmt.Errorf("%w: 503", model.ErrProviderUnavailable)}}}
	clk := &fakeClock{}
	d := New(m, Config{PerAttempt: time.Second, MaxRetries: 5},
		WithClock(clk), WithRand(fixedRand(1.0)))

	_, _ = d.Generate(context.Background(), validReq())

	want := []time.Duration{
		200 * time.Millisecond,  // n=0: 200·1
		400 * time.Millisecond,  // n=1: 200·2
		800 * time.Millisecond,  // n=2: 200·4
		1600 * time.Millisecond, // n=3: 200·8
		MaxBackoffPerWait,       // n=4: 200·16=3200ms capped to 2s
	}
	got := clk.recorded()
	if len(got) != len(want) {
		t.Fatalf("recorded %d waits (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("wait[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	if m.callCount() != 6 {
		t.Errorf("calls = %d, want 6 (maxRetries 5 + 1)", m.callCount())
	}
}

// TestGenerate_fullJitterUsesRandFraction proves the wait scales with the
// injected rand fraction (full jitter, not fixed backoff): frac=0.5 halves it.
func TestGenerate_fullJitterUsesRandFraction(t *testing.T) {
	t.Parallel()
	m := &scriptedModel{name: "m", results: []result{
		{err: fmt.Errorf("%w: 503", model.ErrProviderUnavailable)},
		{resp: okResp("m")},
	}}
	clk := &fakeClock{}
	d := New(m, Config{PerAttempt: time.Second, MaxRetries: 2},
		WithClock(clk), WithRand(fixedRand(0.5)))

	_, _ = d.Generate(context.Background(), validReq())
	waits := clk.recorded()
	if len(waits) != 1 || waits[0] != 100*time.Millisecond { // 0.5 · 200ms
		t.Errorf("waits = %v, want [100ms] (0.5 · 200ms full jitter)", waits)
	}
}

// --- cancel during sleep ----------------------------------------------------

func TestGenerate_cancelDuringSleep_stops(t *testing.T) {
	t.Parallel()
	m := &scriptedModel{name: "m", results: []result{{err: fmt.Errorf("%w: 503", model.ErrProviderUnavailable)}}}
	clk := &fakeClock{sleepErr: context.Canceled} // the backoff sleep is interrupted
	d := New(m, Config{PerAttempt: time.Second, MaxRetries: 3},
		WithClock(clk), WithRand(fixedRand(1.0)))

	if _, err := d.Generate(context.Background(), validReq()); err == nil {
		t.Fatal("err = nil, want the failure surfaced after an interrupted sleep")
	}
	if got := m.callCount(); got != 1 {
		t.Errorf("calls = %d, want 1 (sleep interrupted -> no further attempt)", got)
	}
}

// --- SV3 / BP-c: per-attempt deadline applies even with retry off -----------

// TestGenerate_retryOff_perAttemptDeadlineStillApplies is BP-c / SV3: with
// maxRetries 0 and a model that hangs, the decorator's per-attempt ctx expires
// at PerAttempt and returns after EXACTLY ONE call — it never hangs.
func TestGenerate_retryOff_perAttemptDeadlineStillApplies(t *testing.T) {
	t.Parallel()
	m := &scriptedModel{name: "m", block: func(c context.Context) (*model.Response, error, bool) {
		<-c.Done() // wait for the decorator's per-attempt ctx to fire
		return nil, fmt.Errorf("%w: %w", model.ErrProviderUnavailable, c.Err()), true
	}}
	clk := &fakeClock{}
	d := New(m, Config{PerAttempt: 20 * time.Millisecond, MaxRetries: 0},
		WithClock(clk), WithRand(fixedRand(1.0)))

	_, err := d.Generate(context.Background(), validReq())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want a per-attempt DeadlineExceeded", err)
	}
	if got := m.callCount(); got != 1 {
		t.Errorf("calls = %d, want exactly 1 (SV3: per-attempt deadline, no retry)", got)
	}
}

// TestGenerate_deadlineExpiresOnce is BP-c with retry ENABLED: a model that
// always exceeds its per-attempt window is invoked exactly once, never twice,
// because deadline-expiry is non-retryable (F6).
func TestGenerate_deadlineExpiresOnce(t *testing.T) {
	t.Parallel()
	m := &scriptedModel{name: "m", block: func(c context.Context) (*model.Response, error, bool) {
		<-c.Done()
		return nil, fmt.Errorf("%w: %w", model.ErrProviderUnavailable, c.Err()), true
	}}
	clk := &fakeClock{}
	d := New(m, Config{PerAttempt: 20 * time.Millisecond, MaxRetries: 3},
		WithClock(clk), WithRand(fixedRand(1.0)))

	_, _ = d.Generate(context.Background(), validReq())
	if got := m.callCount(); got != 1 {
		t.Errorf("calls = %d, want exactly 1 (F6: deadline-expiry never retried)", got)
	}
}

// --- Name() delegation + concurrency ---------------------------------------

func TestName_delegatesToInner(t *testing.T) {
	t.Parallel()
	d := New(&scriptedModel{name: "ollama"}, Config{PerAttempt: time.Second})
	if got := d.Name(); got != "ollama" {
		t.Errorf("Name() = %q, want %q (delegates to inner)", got, "ollama")
	}
}

// TestFanoutCancellation_composesWithR1_noRetryOnCancelledSibling proves the F3
// guard composes with the sub-phase-2 fan-out cancellation (SV1): a fast OK
// model makes the coordinator cancel the remaining calls; the slow sibling —
// decorated with retry ON — blocks until that cancel, returns a transient error,
// and its decorator's R1 guard (parent ctx cancelled) fires ZERO retries.
// Deterministic: the slow model only unblocks on cancellation, so the fast one
// always wins first — no sleeps, no timing.
func TestFanoutCancellation_composesWithR1_noRetryOnCancelledSibling(t *testing.T) {
	t.Parallel()
	fast := &scriptedModel{name: "fast", results: []result{{resp: okResp("fast")}}}
	slowInner := &scriptedModel{name: "slow", block: func(ctx context.Context) (*model.Response, error, bool) {
		<-ctx.Done() // unblocks only when the fan-out cancels this sibling
		return nil, fmt.Errorf("%w: transient", model.ErrProviderUnavailable), true
	}}
	fastD := New(fast, Config{PerAttempt: time.Second, MaxRetries: 2}, WithClock(&fakeClock{}), WithRand(fixedRand(1.0)))
	slowD := New(slowInner, Config{PerAttempt: time.Second, MaxRetries: 2}, WithClock(&fakeClock{}), WithRand(fixedRand(1.0)))

	coord := fanout.New(fanout.WithCancelOnFirstUsableSuccess())
	if _, err := coord.Run(context.Background(), validReq(), []model.Model{fastD, slowD}); err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if got := slowInner.callCount(); got != 1 {
		t.Errorf("slow inner calls = %d, want 1 (cancelled sibling -> zero retries, F3 composes with SV1)", got)
	}
}

// TestRealClock_Sleep covers the default time-backed clock: a non-positive
// duration returns the ctx error (nil for a live ctx), a tiny positive wait
// completes, and a cancelled ctx returns promptly with its error.
func TestRealClock_Sleep(t *testing.T) {
	t.Parallel()
	c := realClock{}
	if err := c.Sleep(context.Background(), 0); err != nil {
		t.Errorf("Sleep(0) = %v, want nil", err)
	}
	if err := c.Sleep(context.Background(), time.Millisecond); err != nil {
		t.Errorf("Sleep(1ms) = %v, want nil", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Sleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Errorf("Sleep(cancelled) = %v, want context.Canceled", err)
	}
}

// TestNew_defaults_realClockAndRand builds the decorator with NO seams (real
// clock + math/rand/v2 jitter) and takes a first-attempt success, covering the
// default construction path without any wall-clock backoff.
func TestNew_defaults_realClockAndRand(t *testing.T) {
	t.Parallel()
	m := &scriptedModel{name: "m", results: []result{{resp: okResp("m")}}}
	d := New(m, Config{PerAttempt: time.Second, MaxRetries: 2})
	if _, err := d.Generate(context.Background(), validReq()); err != nil {
		t.Fatalf("err = %v, want success", err)
	}
}

// TestGenerate_noPerAttemptDeadline covers the PerAttempt <= 0 branch: the call
// runs bounded only by the parent ctx (no derived per-attempt deadline).
func TestGenerate_noPerAttemptDeadline(t *testing.T) {
	t.Parallel()
	m := &scriptedModel{name: "m", results: []result{{resp: okResp("m")}}}
	d := New(m, Config{PerAttempt: 0, MaxRetries: 0}, WithClock(&fakeClock{}), WithRand(fixedRand(1.0)))
	if _, err := d.Generate(context.Background(), validReq()); err != nil {
		t.Fatalf("err = %v, want success", err)
	}
	if got := m.callCount(); got != 1 {
		t.Errorf("calls = %d, want 1", got)
	}
}

// TestGenerate_concurrent_raceClean exercises one decorator instance under many
// concurrent Generate calls (run with -race -count=20). The shared rand/clock
// seams must be safe (FR-C3).
func TestGenerate_concurrent_raceClean(t *testing.T) {
	t.Parallel()
	m := &scriptedModel{name: "m", results: []result{
		{err: fmt.Errorf("%w: 503", model.ErrProviderUnavailable)},
		{resp: okResp("m")},
	}}
	clk := &fakeClock{}
	d := New(m, Config{PerAttempt: time.Second, MaxRetries: 2},
		WithClock(clk), WithRand(fixedRand(1.0)))

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = d.Generate(context.Background(), validReq())
		}()
	}
	wg.Wait()
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package retry

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/metrics"
	"github.com/Sebastian197/korvun/internal/model"
)

// This file is the RED for ADR-0031 sub-phase 7's retry metrics (FR-M1/FR-M2):
// the decorator emits IncProviderRetry per effective retry and
// IncProviderRetryBudgetExhausted on a retryable give-up, via an injected
// metrics.Metrics (retry.WithMetrics). retry.WithMetrics does not exist yet, so
// this file fails to build — the red for the option + emission. It reuses the
// scriptedModel / fakeClock / fixedRand / okResp / validReq helpers from
// retry_test.go (same package).

// fakeMetrics implements metrics.Metrics and counts the two retry counters by
// provider. The other methods no-op.
type fakeMetrics struct {
	mu        sync.Mutex
	retries   map[string]int
	exhausted map[string]int
}

func newFakeMetrics() *fakeMetrics {
	return &fakeMetrics{retries: map[string]int{}, exhausted: map[string]int{}}
}

func (f *fakeMetrics) IncMessages(string)                                  {}
func (f *fakeMetrics) ObserveProviderDuration(string, bool, time.Duration) {}
func (f *fakeMetrics) IncProviderFailure(string)                           {}
func (f *fakeMetrics) IncRouterError(string)                               {}
func (f *fakeMetrics) ObserveTurnsPersisted(int)                           {}
func (f *fakeMetrics) IncProviderRetry(p string) {
	f.mu.Lock()
	f.retries[p]++
	f.mu.Unlock()
}
func (f *fakeMetrics) IncProviderRetryBudgetExhausted(p string) {
	f.mu.Lock()
	f.exhausted[p]++
	f.mu.Unlock()
}
func (f *fakeMetrics) nRetry(p string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.retries[p]
}
func (f *fakeMetrics) nExh(p string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.exhausted[p]
}

// compile-time assertion the fake is a full Metrics (also flags a missing
// interface method in green).
var _ metrics.Metrics = (*fakeMetrics)(nil)

// AS-5: one transient failure then success (max_retries 2) → exactly ONE
// effective retry counted, zero budget-exhausted.
func TestMetrics_effectiveRetryIncrementsOnce(t *testing.T) {
	t.Parallel()
	fm := newFakeMetrics()
	m := &scriptedModel{name: "ollama", results: []result{
		{err: fmt.Errorf("%w: 503", model.ErrProviderUnavailable)},
		{resp: okResp("ollama")},
	}}
	d := New(m, Config{PerAttempt: time.Second, MaxRetries: 2},
		WithClock(&fakeClock{}), WithRand(fixedRand(1.0)), WithMetrics(fm))
	if _, err := d.Generate(context.Background(), validReq()); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got := fm.nRetry("ollama"); got != 1 {
		t.Errorf("IncProviderRetry(ollama) = %d, want 1 (one effective retry)", got)
	}
	if got := fm.nExh("ollama"); got != 0 {
		t.Errorf("budget exhausted = %d, want 0", got)
	}
}

// AS-6: budget exhausted — both by max_retries and by the FR-A2 parent-budget
// give-up — bumps IncProviderRetryBudgetExhausted exactly once.
func TestMetrics_budgetExhausted(t *testing.T) {
	t.Parallel()

	t.Run("max_retries exhausted", func(t *testing.T) {
		fm := newFakeMetrics()
		m := &scriptedModel{name: "ollama", results: []result{{err: fmt.Errorf("%w: 503", model.ErrProviderUnavailable)}}}
		d := New(m, Config{PerAttempt: time.Second, MaxRetries: 2},
			WithClock(&fakeClock{}), WithRand(fixedRand(1.0)), WithMetrics(fm))
		_, _ = d.Generate(context.Background(), validReq())
		if got := fm.nRetry("ollama"); got != 2 {
			t.Errorf("retries = %d, want 2 (max_retries)", got)
		}
		if got := fm.nExh("ollama"); got != 1 {
			t.Errorf("budget exhausted = %d, want 1", got)
		}
	})

	t.Run("FR-A2 parent-budget give-up", func(t *testing.T) {
		fm := newFakeMetrics()
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		m := &scriptedModel{name: "ollama", results: []result{
			{err: fmt.Errorf("rl: %w", &model.RateLimitError{Provider: "ollama", RetryAfter: 30 * time.Second})},
		}}
		d := New(m, Config{PerAttempt: time.Second, MaxRetries: 3},
			WithClock(&fakeClock{}), WithRand(fixedRand(1.0)), WithMetrics(fm))
		_, _ = d.Generate(ctx, validReq())
		if got := fm.nRetry("ollama"); got != 0 {
			t.Errorf("retries = %d, want 0 (gave up before any retry)", got)
		}
		if got := fm.nExh("ollama"); got != 1 {
			t.Errorf("budget exhausted = %d, want 1 (FR-A2 give-up)", got)
		}
	})
}

// AS-7: a non-retryable failure moves NEITHER counter.
func TestMetrics_nonRetryable_movesNothing(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		results []result
		cancel  bool // pre-cancel the parent ctx
	}{
		{"auth", []result{{err: fmt.Errorf("%w: 401", model.ErrAuthInvalid)}}, false},
		{"bad response", []result{{err: fmt.Errorf("%w: 400", model.ErrProviderResponse)}}, false},
		{"per-attempt deadline (F6)", []result{{err: deadlineErr()}}, false},
		{"parent cancelled (F3)", []result{{err: fmt.Errorf("%w: 503", model.ErrProviderUnavailable)}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fm := newFakeMetrics()
			ctx := context.Background()
			if tc.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			m := &scriptedModel{name: "ollama", results: tc.results}
			d := New(m, Config{PerAttempt: time.Second, MaxRetries: 3},
				WithClock(&fakeClock{}), WithRand(fixedRand(1.0)), WithMetrics(fm))
			_, _ = d.Generate(ctx, validReq())
			if r, e := fm.nRetry("ollama"), fm.nExh("ollama"); r != 0 || e != 0 {
				t.Errorf("[%s] retries=%d exhausted=%d, want 0/0 (non-retryable)", tc.name, r, e)
			}
		})
	}
}

// AS-9: without WithMetrics the default is Nop — Generate still works and no
// panic. (The full existing retry suite also exercises the no-metrics path.)
func TestMetrics_defaultNop_noPanic(t *testing.T) {
	t.Parallel()
	m := &scriptedModel{name: "ollama", results: []result{
		{err: fmt.Errorf("%w: 503", model.ErrProviderUnavailable)},
		{resp: okResp("ollama")},
	}}
	d := New(m, Config{PerAttempt: time.Second, MaxRetries: 2}, WithClock(&fakeClock{}), WithRand(fixedRand(1.0)))
	if _, err := d.Generate(context.Background(), validReq()); err != nil {
		t.Fatalf("Generate with default Nop metrics: %v", err)
	}
}

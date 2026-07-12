// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"fmt"
	"testing"

	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/fanout"
	"github.com/Sebastian197/korvun/internal/policy"
)

// This file is the RED for ADR-0031 sub-phase 7's differentiated fallback
// (FR-F1..F5). It drives the Orchestrator end-to-end with a REAL PriorityReducer
// over failing models, so res.Outcomes[].Err carries the real error grammar the
// brain must classify. The brain classifies with model sentinels +
// context.DeadlineExceeded ONLY — it must NOT import internal/model/retry
// (verified: this file imports model, fanout, policy — not retry).
//
// AS-1/2/3 are RED by behaviour (today the brain returns the single generic
// fallback, not the differentiated text); AS-4 is a born-GREEN backward-compat
// guard (an explicit WithFallback override still wins).

// The exact texts Chano decided (option A).
const (
	wantRetrySoon   = "The model is still starting up or busy. Please try again in a moment."
	wantUnavailable = "The model provider is currently unavailable. Please try again later."
)

// erroringModel always fails with a fixed error, so the fan-out produces an
// all-failed Result (PriorityReducer → ErrNoUsableOutcome → fallback).
type erroringModel struct {
	name string
	err  error
}

func (m erroringModel) Generate(context.Context, *model.Request) (*model.Response, error) {
	return nil, m.err
}
func (m erroringModel) Name() string { return m.name }

func deadlineWrapped() error {
	return fmt.Errorf("%w: %w", model.ErrProviderUnavailable, context.DeadlineExceeded)
}
func hardUnavailable() error {
	return fmt.Errorf("%w: connection refused", model.ErrProviderUnavailable)
}
func rateLimited() error {
	return fmt.Errorf("rl: %w", &model.RateLimitError{Provider: "x", RetryAfter: 0})
}

func runFallback(t *testing.T, models []model.Model, order []string, opts ...Option) string {
	t.Helper()
	base := append([]Option{WithLogger(quietLogger())}, opts...)
	o := NewOrchestrator(fanout.New(), models, policy.PriorityReducer{Order: order}, base...)
	out, err := o.Handle(context.Background(), inboundText("telegram", "c", "hi"))
	if err != nil {
		t.Fatalf("Handle (no-answer must not error): %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("want a single fallback reply, got %d envelopes", len(out))
	}
	return out[0].Parts[0].Content
}

// AS-1: at least one hopeful error (deadline-expiry or rate-limit) → retry-soon.
func TestFallback_retrySoon_onHopefulError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		hopeful error
	}{
		{"deadline expiry", deadlineWrapped()},
		{"rate limited", rateLimited()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			models := []model.Model{
				erroringModel{name: "slow", err: tc.hopeful},
				erroringModel{name: "down", err: hardUnavailable()},
			}
			if got := runFallback(t, models, []string{"slow", "down"}); got != wantRetrySoon {
				t.Errorf("fallback = %q, want retry-soon %q", got, wantRetrySoon)
			}
		})
	}
}

// AS-2: no hopeful error (hard-down / auth / bad-response) → unavailable.
func TestFallback_unavailable_onHardFailures(t *testing.T) {
	t.Parallel()
	models := []model.Model{
		erroringModel{name: "a", err: hardUnavailable()},
		erroringModel{name: "b", err: fmt.Errorf("%w: 401", model.ErrAuthInvalid)},
		erroringModel{name: "c", err: fmt.Errorf("%w: 400", model.ErrProviderResponse)},
	}
	if got := runFallback(t, models, []string{"a", "b", "c"}); got != wantUnavailable {
		t.Errorf("fallback = %q, want unavailable %q", got, wantUnavailable)
	}
}

// AS-3: mixed classes — one hopeful + two hard-down → retry-soon wins (FR-F2).
// This BITES if the rule is changed to majority (2 hard vs 1 hopeful).
func TestFallback_mixed_anyHopefulWins(t *testing.T) {
	t.Parallel()
	models := []model.Model{
		erroringModel{name: "slow", err: deadlineWrapped()},
		erroringModel{name: "d1", err: hardUnavailable()},
		erroringModel{name: "d2", err: hardUnavailable()},
	}
	if got := runFallback(t, models, []string{"slow", "d1", "d2"}); got != wantRetrySoon {
		t.Errorf("fallback = %q, want retry-soon (any hopeful wins, not majority)", got)
	}
}

// No-consensus guard: when models DID answer but reached no majority
// (ErrNoConsensus), the reply is the generic defaultFallback — NOT retry-soon or
// unavailable (both describe failures, and the providers responded). This pins
// the policyErr-based split (ADR-0031 sub-phase 7) so a regression to
// "unavailable" is caught.
func TestFallback_noConsensus_usesGeneric(t *testing.T) {
	t.Parallel()
	dec := &policy.Decision{Provenance: policy.Provenance{
		Considered: []policy.Contribution{{Provider: "a-provider"}},
	}}
	o := NewOrchestrator(fanout.New(), okModels("a", "b"),
		fakePolicy{dec: dec, err: policy.ErrNoConsensus}, WithLogger(quietLogger()))
	out, err := o.Handle(context.Background(), inboundText("telegram", "c", "hi"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(out) != 1 || out[0].Parts[0].Content != defaultFallback {
		t.Fatalf("want generic defaultFallback on no-consensus, got %+v", out)
	}
}

// AS-4 (born GREEN): an explicit WithFallback override still wins over both
// differentiated defaults (backward compat).
func TestFallback_overrideStillWins(t *testing.T) {
	t.Parallel()
	const custom = "operator says: come back later"
	models := []model.Model{erroringModel{name: "slow", err: deadlineWrapped()}}
	if got := runFallback(t, models, []string{"slow"}, WithFallback(custom)); got != custom {
		t.Errorf("fallback = %q, want the operator override %q", got, custom)
	}
}

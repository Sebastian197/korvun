// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"
	"time"
)

// This file pins the PURE ceiling-derivation contract of ADR-0031 Decision 2:
// given a brain's resolved per-attempt timeouts, backoff budgets, and dispatch
// shape, deriveBrainCeiling returns the router ceiling. The three shape
// formulas are pinned verbatim from the ADR (after SV1/SV2):
//
//   - fan-out    (cancel-on-first-usable-success, SV1):
//       ceiling = max_i( perAttempt_i + backoffBudget_i ) + margin
//   - sequential (retry off by construction, SV2):
//       ceiling = Σ_i( perAttempt_i ) + margin
//   - agent      (bounded single-model loop, SV1):
//       ceiling = maxIterations × ( perAttempt_model + perToolTimeout ) + margin
//
// RED note: brainCeilingSpec and deriveBrainCeiling do not exist yet, so this
// file fails to build. That compile failure IS the red for the derivation;
// GREEN adds the type and the pure function.

// TestDeriveBrainCeiling_fanoutMaxOfPerAttemptPlusBackoff pins the fan-out
// formula: the WORST INDIVIDUAL model (a max), never a sum, plus its backoff
// budget, plus margin (ADR-0031 Decision 2, SV1).
func TestDeriveBrainCeiling_fanoutMaxOfPerAttemptPlusBackoff(t *testing.T) {
	t.Parallel()
	got := deriveBrainCeiling(brainCeilingSpec{
		shape:         "fanout",
		perAttempt:    []time.Duration{100 * time.Millisecond, 300 * time.Millisecond},
		backoffBudget: []time.Duration{50 * time.Millisecond, 20 * time.Millisecond},
		margin:        10 * time.Millisecond,
	})
	// max(100+50, 300+20) + 10 = 320 + 10 = 330ms.
	if want := 330 * time.Millisecond; got != want {
		t.Errorf("fan-out ceiling = %v, want %v", got, want)
	}
}

// TestDeriveBrainCeiling_sequentialSumOfPerAttempt pins the sequential formula:
// the SUM of per-attempt windows (the fail-over walks them serially) plus
// margin — and, per SV2, no retry multiplication and no backoff term enters the
// sum (ADR-0031 Decision 2).
func TestDeriveBrainCeiling_sequentialSumOfPerAttempt(t *testing.T) {
	t.Parallel()
	got := deriveBrainCeiling(brainCeilingSpec{
		shape:      "sequential",
		perAttempt: []time.Duration{100 * time.Millisecond, 300 * time.Millisecond, 200 * time.Millisecond},
		// backoffBudget deliberately set: it MUST be ignored for sequential.
		backoffBudget: []time.Duration{999 * time.Millisecond, 999 * time.Millisecond, 999 * time.Millisecond},
		margin:        10 * time.Millisecond,
	})
	// (100 + 300 + 200) + 10 = 610ms.
	if want := 610 * time.Millisecond; got != want {
		t.Errorf("sequential ceiling = %v, want %v", got, want)
	}
}

// TestDeriveBrainCeiling_agentFromMaxIterations pins the agent formula: the
// hard loop cap times one model-call-plus-tool-call window, plus margin
// (ADR-0031 Decision 2, SV1 — the third dispatch shape the pre-SV1 draft did
// not model). The AgentBrain is single-model, so perAttempt has one element.
func TestDeriveBrainCeiling_agentFromMaxIterations(t *testing.T) {
	t.Parallel()
	got := deriveBrainCeiling(brainCeilingSpec{
		shape:          "agent",
		perAttempt:     []time.Duration{200 * time.Millisecond},
		agentMaxIter:   4,
		perToolTimeout: 50 * time.Millisecond,
		margin:         10 * time.Millisecond,
	})
	// 4 × (200 + 50) + 10 = 1000 + 10 = 1010ms.
	if want := 1010 * time.Millisecond; got != want {
		t.Errorf("agent ceiling = %v, want %v", got, want)
	}
}

// TestDeriveBrainCeiling_fanoutNotAmplifiedByRetries pins the load-bearing SV1
// property: because deadline-expiry is non-retryable, retries never multiply
// the per-attempt window. The ceiling stays on the order of ONE per-attempt
// window plus a bounded backoff — it must NOT approach N×perAttempt. With a 1s
// per-attempt and a 100ms bounded backoff, the ceiling is 1.1s, not 2s+ (the
// pre-SV1 ×(maxRetries+1) derivation).
func TestDeriveBrainCeiling_fanoutNotAmplifiedByRetries(t *testing.T) {
	t.Parallel()
	got := deriveBrainCeiling(brainCeilingSpec{
		shape:         "fanout",
		perAttempt:    []time.Duration{1 * time.Second},
		backoffBudget: []time.Duration{100 * time.Millisecond},
		margin:        0,
	})
	if want := 1100 * time.Millisecond; got != want {
		t.Errorf("fan-out ceiling = %v, want %v (one attempt window + bounded backoff)", got, want)
	}
	if got >= 2*time.Second {
		t.Errorf("fan-out ceiling = %v — retries must NOT multiply the per-attempt window (SV1)", got)
	}
}

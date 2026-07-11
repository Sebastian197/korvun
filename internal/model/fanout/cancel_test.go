// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package fanout

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/model"
)

// This file pins ADR-0031 sub-phase 2 (SV1): fan-out cancellation on the first
// USABLE success. The mechanism is a new opt-in Option; the default stays
// wait-all (today's behavior). It reuses the package's fakeModel, okResponse,
// and validRequest helpers.
//
// RED note: WithCancelOnFirstUsableSuccess does not exist yet, so the package
// test binary fails to build. That compile failure IS the red for the mechanism;
// GREEN adds the Option and the cancellation in Run.

// TestRun_cancelMode_cancelsOnFirstUsableSuccess is the ADR test verbatim: with
// cancellation ON, a fast OK reply makes Run return promptly and the slow
// sibling — which blocks until its ctx is cancelled — observes cancellation
// (its Outcome carries an Err, not a Response). Without cancellation the slow
// model's 10s delay would dominate.
func TestRun_cancelMode_cancelsOnFirstUsableSuccess(t *testing.T) {
	fast := &fakeModel{name: "fast", response: okResponse("fast", "fast", "quick")}
	slow := &fakeModel{name: "slow", delay: 10 * time.Second} // returns only via ctx cancel

	c := New(WithCancelOnFirstUsableSuccess())
	start := time.Now()
	res, err := c.Run(context.Background(), validRequest(), []model.Model{fast, slow})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("Run took %v — it did not return on the first usable success (slow sibling was not cancelled)", elapsed)
	}
	if res.Outcomes[0].Response == nil {
		t.Errorf("fast outcome = %+v, want a usable Response", res.Outcomes[0])
	}
	if res.Outcomes[1].Err == nil || res.Outcomes[1].Response != nil {
		t.Errorf("slow outcome = %+v, want a cancellation Err and no Response", res.Outcomes[1])
	}
	// The slow sibling was called exactly once and returned (no leaked goroutine
	// still running Generate after Run returned).
	if got := atomic.LoadInt32(&slow.callCount); got != 1 {
		t.Errorf("slow callCount = %d, want 1 (called once, then cancelled)", got)
	}
}

// TestRun_cancelMode_fastFailureDoesNotCancel pins "first USABLE success": a
// model that FAILS fast is not a usable success, so it must NOT trigger
// cancellation. The fan-out keeps waiting for the first Outcome with Err == nil
// — the slow success wins.
func TestRun_cancelMode_fastFailureDoesNotCancel(t *testing.T) {
	fastFail := &fakeModel{name: "fastfail", err: fmt.Errorf("down: %w", model.ErrProviderUnavailable)}
	slowOK := &fakeModel{name: "slowok", delay: 50 * time.Millisecond, response: okResponse("slowok", "slowok", "answer")}

	c := New(WithCancelOnFirstUsableSuccess())
	res, err := c.Run(context.Background(), validRequest(), []model.Model{fastFail, slowOK})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if res.Outcomes[0].Err == nil {
		t.Errorf("fast-fail outcome = %+v, want its Err", res.Outcomes[0])
	}
	if res.Outcomes[1].Response == nil {
		t.Errorf("slow-ok outcome = %+v, want its Response (a fast FAILURE must not cancel it)", res.Outcomes[1])
	}
}

// TestRun_defaultWaitAll_doesNotCancelOnSuccess pins the default: with NO Option,
// Run stays wait-all — an early success does NOT cancel a slow sibling, which
// completes with its own Response. This test MUST bite if anyone flips the
// default to cancel-on-first-success.
func TestRun_defaultWaitAll_doesNotCancelOnSuccess(t *testing.T) {
	fast := &fakeModel{name: "fast", response: okResponse("fast", "fast", "quick")}
	slow := &fakeModel{name: "slow", delay: 50 * time.Millisecond, response: okResponse("slow", "slow", "late")}

	c := New() // default: wait-all, no cancellation
	res, err := c.Run(context.Background(), validRequest(), []model.Model{fast, slow})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if res.Outcomes[0].Response == nil {
		t.Errorf("fast outcome = %+v, want its Response", res.Outcomes[0])
	}
	if res.Outcomes[1].Response == nil || res.Outcomes[1].Err != nil {
		t.Errorf("slow outcome = %+v, want its Response — default wait-all must not cancel on an early success", res.Outcomes[1])
	}
}

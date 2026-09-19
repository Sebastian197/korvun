// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// ONE classifier for what a tool run closes on — the third of the four class
// cures the director ordered on 2026-09-16.
//
// The v0.15.0 train cured the two execution paths one at a time: the approved
// path in internal/app and the brain path in internal/brain each grew their
// own copy of «delivered or a context that ended ⇒ OUTCOME_UNKNOWN». Two
// copies of one rule is the class: the next branch added to one of them is a
// disagreement nobody notices, and the pair already disagreed once.
//
// Evidence level, honest: in-process unit test over the exported function.
package tool

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
)

// TestCloseStateAfterRun_namesEveryClass is the classifier's own mould.
//
// Probing mutation (executed, red, declared in the canto): return
// action.StateFailed for the cancelled arm, or drop the ErrEffectDelivered
// arm ⇒ the matching row reddens.
func TestCloseStateAfterRun_namesEveryClass(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want action.State
	}{
		{"nothing failed", nil, action.StateSucceeded},
		{"the receiver answered and the answer was not a usable success (label corrected in v0.15.1 block A)",
			fmt.Errorf("webhook_call: %w", ErrEffectDelivered), action.StateOutcomeUnknown},
		{"the deadline fired", fmt.Errorf("webhook_call: %w", context.DeadlineExceeded), action.StateOutcomeUnknown},
		{"the context was cancelled", fmt.Errorf("webhook_call: %w", context.Canceled), action.StateOutcomeUnknown},
		{"the tool refused before anything left", errors.New("webhook_call: host not allow-listed"), action.StateFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := CloseStateAfterRun(tc.err); got != tc.want {
				t.Fatalf("CloseStateAfterRun(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

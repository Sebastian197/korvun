// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.15.1 block B — P2-9 by the director's name. RED BY COMPILATION: the
// sentinel ErrApprovalWriteFailed does not exist yet (director, 2026-09-19:
// «`unavailable` + new sentinel ErrApprovalWriteFailed in approvals.go;
// already_closed only for a genuinely moved action»). While this file does not
// compile, NO test of this package runs in this branch; the other red captures
// of the package were taken before it was added.

package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// TestV0151B_P2_9_aDriverFailureInTheTransitionIsWriteFailedOnBothVerbs is
// Codex's trigger on BOTH verbs. A cure that types only the approve branch
// leaves the reject branch free to regress (the adversary's finding 2), so the
// reject row is here with the same trigger.
//
// Evidence: real sealed store, trigger installed through a second real
// connection, in-process.
// Probing mutation (planned): type one branch's exec failure as
// ErrApprovalActionNotPending ⇒ that row reddens.
func TestV0151B_P2_9_aDriverFailureInTheTransitionIsWriteFailedOnBothVerbs(t *testing.T) {
	t.Parallel()
	for _, verb := range []string{action.DecisionApproved, action.DecisionRejected} {
		t.Run(verb, func(t *testing.T) {
			t.Parallel()
			store, _ := sealedStore(t)
			id := "act_wf_" + verb
			a, _ := pendingRequest(t, store, id)
			attack(t, store, `CREATE TRIGGER forced_driver_failure BEFORE UPDATE OF state ON actions
			  WHEN OLD.action_id = '`+id+`'
			  BEGIN SELECT RAISE(ABORT, 'forced driver failure'); END`) // #nosec G202 -- test-owned literal
			env, ident := operatorDecisionEnv(verb, a.ApprovalID)
			_, err := store.DecideApprovalUnderLaw(context.Background(), a.ApprovalID, verb,
				a.RequestedAt.Add(time.Minute), env, ident, "", parkedLaw)
			if !errors.Is(err, ErrApprovalWriteFailed) {
				t.Fatalf("err = %v, want ErrApprovalWriteFailed", err)
			}
			if errors.Is(err, ErrApprovalActionNotPending) {
				t.Fatalf("err = %v: a driver failure typed as a moved action", err)
			}
			after, _, gerr := store.GetApproval(context.Background(), a.ApprovalID)
			if gerr != nil || after.Status != action.ApprovalPending {
				t.Fatalf("approval must stay PENDING: %+v %v", after.Status, gerr)
			}
		})
	}
}

// TestV0151B_P2_9_theExpiryTouchInsideADecideIsWriteFailedToo: a decide that
// lands after the expiry closes the approval by the clock and moves the parked
// action to REJECTED (decideApprovalWithLaw's «approval_expired» branch, through
// rejectParkedActionTx). A driver failure there is the same class.
//
// Evidence: real sealed store, trigger through a second real connection,
// in-process.
// Probing mutation (planned): leave that branch's transition error untyped ⇒ red.
func TestV0151B_P2_9_theExpiryTouchInsideADecideIsWriteFailedToo(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	id := "act_wf_expiry"
	a, _ := pendingRequest(t, store, id)
	attack(t, store, `CREATE TRIGGER forced_driver_failure BEFORE UPDATE OF state ON actions
	  WHEN OLD.action_id = '`+id+`'
	  BEGIN SELECT RAISE(ABORT, 'forced driver failure'); END`) // #nosec G202 -- test-owned literal
	env, ident := operatorDecisionEnv("approve", a.ApprovalID)
	_, err := store.DecideApprovalUnderLaw(context.Background(), a.ApprovalID, action.DecisionApproved,
		a.ExpiresAt.Add(time.Minute), env, ident, "", parkedLaw)
	if !errors.Is(err, ErrApprovalWriteFailed) {
		t.Fatalf("err = %v, want ErrApprovalWriteFailed", err)
	}
}

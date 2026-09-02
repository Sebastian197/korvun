// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R4 Phase 3 (FR-R4F3): atomic ownership. Every crash/sweep close
// carries its COMPLETE eligibility predicate in the UPDATE and
// verifies RowsAffected — zero rows means another process owned the
// row legitimately: no receipt, no drama, changed=false. The sweep
// SKIPS a concurrently-decided approval with a note, never a
// boot-fatal for losing a clean race; real errors (context
// cancellation included) still abort, and per-row transactions leave
// no partial row behind. The auditor's six scenarios, permanent.
// Reproduction-first contract.

package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// Scenario 1 (deterministic core): the recovery close runs AFTER a
// concurrent Finish already owned the row — zero rows updated, no
// second receipt, no error.
func TestRecoveryClose_lostRaceMeansNoReceiptNoDrama(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	mustRecord(t, store, "act_f3_race", action.StateAuthorized)
	// The "concurrent" Finish wins first.
	if err := store.FinishWithResult(ctx, "act_f3_race", action.StateSucceeded,
		time.Now().UTC(), "sha256:result"); err != nil {
		t.Fatalf("finish: %v", err)
	}
	changed, err := store.closeCrashOrphan(ctx, "act_f3_race", action.StateFailed,
		recoveryMarkerCrash, crashOrphanPredicate, time.Now().UTC())
	if err != nil {
		t.Fatalf("AUDIT R4-F3: losing a clean race is not an error: %v", err)
	}
	if changed {
		t.Fatal("zero eligible rows means changed=false")
	}
	rec, _ := store.Get(ctx, "act_f3_race")
	if rec.State != action.StateSucceeded {
		t.Fatalf("the legitimate owner's close stands: %v", rec.State)
	}
	receipts, _ := store.ReceiptsByAction(ctx, "act_f3_race")
	if len(receipts) != 1 {
		t.Fatalf("exactly ONE receipt — the winner's: %d", len(receipts))
	}
}

// Scenario 1 (racing form) + Scenario 2: a concurrent Finish and TWO
// recoveries all compete under -race; every orphan closes exactly
// once, one receipt per action, zero errors.
func TestRecovery_twoRecoveriesAndAFinishCompete(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	for i := 0; i < 8; i++ {
		mustRecord(t, store, actionID("act_f3_c", i), action.StateAuthorized)
	}
	var wg sync.WaitGroup
	recErrs := make(chan error, 2)
	finishErr := make(chan error, 1)
	for r := 0; r < 2; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			recErrs <- store.RecoverPreviousLife(ctx)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		finishErr <- store.FinishWithResult(ctx, "act_f3_c0", action.StateSucceeded,
			time.Now().UTC(), "sha256:r")
	}()
	wg.Wait()
	close(recErrs)
	close(finishErr)
	// The SWEEPERS must never error on a clean race (F3's contract).
	for err := range recErrs {
		if err != nil {
			t.Fatalf("a recovery losing a clean race must never error: %v", err)
		}
	}
	// The FINISH is the live executor's close: losing to the recovery
	// legitimately refuses BY NAME (invalid transition — the action
	// already closed; fail-closed, no double receipt). Flake-hunt
	// diagnosis 2026-09-02: 18 captured lines proved the PRODUCT right
	// and this test's original "never error" expectation wrong for the
	// finish arm — the cure is the expectation, not the product.
	for err := range finishErr {
		if err != nil && !errors.Is(err, action.ErrInvalidTransition) {
			t.Fatalf("the finish may only lose by the named invalid transition: %v", err)
		}
	}
	for i := 0; i < 8; i++ {
		id := actionID("act_f3_c", i)
		rec, err := store.Get(ctx, id)
		if err != nil || !rec.State.Terminal() {
			t.Fatalf("%s must close terminal: %v %v", id, err, rec.State)
		}
		receipts, _ := store.ReceiptsByAction(ctx, id)
		if len(receipts) != 1 {
			t.Fatalf("%s: exactly one receipt whoever won: %d", id, len(receipts))
		}
	}
}

func actionID(prefix string, i int) string {
	return prefix + string(rune('0'+i))
}

// Scenarios 3+4: the sweep meets rows an operator decided between the
// candidate SELECT and the per-row close — rejected AND approved both
// SKIP with a note, never an error.
func TestSweep_operatorDecidedRowsAreSkippedWithANote(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	rejected := expiredParked(t, store, "act_f3_rej")
	approved := expiredParked(t, store, "act_f3_app")
	stillPending := expiredParked(t, store, "act_f3_pen")
	// The operator decides two of them "concurrently" (between the
	// sweep's SELECT and its per-row transactions — simulated by
	// deciding before the per-row close runs; the per-row predicate is
	// what must protect, not the SELECT snapshot).
	envR, identR := operatorDecisionEnv("reject", rejected.ApprovalID)
	if _, err := store.decideApproval(ctx, rejected.ApprovalID, "rejected",
		rejected.RequestedAt.Add(time.Second), envR, identR, ""); err != nil {
		t.Fatalf("reject: %v", err)
	}
	envA, identA := operatorDecisionEnv("approve", approved.ApprovalID)
	if _, err := store.decideApproval(ctx, approved.ApprovalID, "approved",
		approved.RequestedAt.Add(time.Second), envA, identA, ""); err != nil {
		t.Fatalf("approve: %v", err)
	}
	// The per-row close on ALREADY-DECIDED rows: skipped, not an error.
	for _, id := range []string{rejected.ApprovalID, approved.ApprovalID} {
		won, err := store.sweepExpiredOne(ctx, id, time.Now().UTC())
		if err != nil {
			t.Fatalf("AUDIT R4-F3: losing to the operator is a SKIP, not an error: %v", err)
		}
		if won {
			t.Fatalf("%s was decided — the sweep must not win it", id)
		}
	}
	// And the whole sweep over the mix: exactly the pending one sweeps.
	swept, skipped, err := store.SweepExpiredApprovals(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if swept != 1 || skipped != 0 {
		t.Fatalf("exactly the still-pending one sweeps (decided rows left the candidate set): swept=%d skipped=%d", swept, skipped)
	}
	if got, _, err := store.GetApproval(ctx, stillPending.ApprovalID); err != nil || got.Status != action.ApprovalExpired {
		t.Fatalf("the pending one swept: %v %v", err, got.Status)
	}
}

// Scenario 5: context cancellation between rows aborts cleanly — a
// real error, zero partial rows.
func TestSweep_contextCancellationAbortsCleanly(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	expiredParked(t, store, "act_f3_ctx1")
	expiredParked(t, store, "act_f3_ctx2")
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	swept, _, err := store.SweepExpiredApprovals(cancelled, time.Now().UTC())
	if err == nil {
		t.Fatal("a cancelled context is a REAL error and aborts")
	}
	if swept != 0 {
		t.Fatalf("nothing sweeps under a dead context: %d", swept)
	}
	// No partial rows: both stay fully PENDING with params held.
	for _, id := range []string{"act_f3_ctx1", "act_f3_ctx2"} {
		rec, err := store.Get(context.Background(), id)
		if err != nil || rec.State != action.StatePendingApproval {
			t.Fatalf("%s untouched: %v %v", id, err, rec.State)
		}
	}
}

// Scenario 6: 200 expired → 200 purges + 200 VALID receipts + zero
// partial rows.
func TestSweep_twoHundredExpiredAllPurgedAllReceiptsValid(t *testing.T) {
	t.Parallel()
	store, pub := sealedStore(t)
	ctx := context.Background()
	for i := 0; i < 200; i++ {
		expiredParked(t, store, actID200(i))
	}
	swept, skipped, err := store.SweepExpiredApprovals(ctx, time.Now().UTC())
	if err != nil || swept != 200 || skipped != 0 {
		t.Fatalf("200 sweeps: swept=%d skipped=%d err=%v", swept, skipped, err)
	}
	for i := 0; i < 200; i++ {
		id := actID200(i)
		rec, err := store.Get(ctx, id)
		if err != nil || rec.State != action.StateRejected {
			t.Fatalf("%s closes REJECTED: %v %v", id, err, rec.State)
		}
		receipts, err := store.ReceiptsByAction(ctx, id)
		if err != nil || len(receipts) != 1 {
			t.Fatalf("%s: exactly one receipt: %v %d", id, err, len(receipts))
		}
		if err := action.VerifyReceiptSignature(pub, receipts[0]); err != nil {
			t.Fatalf("%s: the receipt must VERIFY: %v", id, err)
		}
		a, _, err := store.GetApprovalByAction(ctx, id)
		if err != nil || a.Status != action.ApprovalExpired {
			t.Fatalf("%s approval EXPIRED: %v %v", id, err, a.Status)
		}
		if _, err := store.ApprovalParams(ctx, a.ApprovalID); err == nil {
			t.Fatalf("%s params purged", id)
		}
	}
}

func actID200(i int) string {
	return "act_f3_m" + string(rune('a'+i/26)) + string(rune('a'+i%26))
}

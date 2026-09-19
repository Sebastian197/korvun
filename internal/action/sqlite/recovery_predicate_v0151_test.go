// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The recovery pass's UPDATE carries its COMPLETE eligibility — v0.15.1 block
// A, A3 of the seventeenth external pass.
//
// RecoverPreviousLife selects the crash orphans with a SELECT that excludes
// nine states, AUTHORIZED among them, and closes each one with an UPDATE whose
// predicate, crashOrphanPredicate, excludes only eight: AUTHORIZED is missing.
// A row selected while it was, say, PREPARING, and moved to AUTHORIZED by
// another connection before the UPDATE ran, is closed FAILED with a crash
// receipt — the «FAILED lie» over a tool that is running.
//
// Evidence level, honest: multiple real database connections. The selection
// is the production collectIDs over a COPY of the crash pass's SELECT (the
// query is inline in RecoverPreviousLife, not a named constant), and the close
// is the production closeCrashOrphan with the production predicate;
// the interleaving is placed by the test between the two calls, NOT by a hook
// inside RecoverPreviousLife. In-process; not a compiled binary.
package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// TestRecovery_theCrashCloseRefusesARowThatBecameAuthorized is A3's mould: a
// second connection moves the selected row to AUTHORIZED between the
// selection and the UPDATE. The UPDATE must affect zero rows — changed=false —
// append no receipt, and leave the row AUTHORIZED.
//
// Planned probing mutation (after green): drop 'AUTHORIZED' from
// crashOrphanPredicate again ⇒ changed=true and the row closes FAILED.
func TestRecovery_theCrashCloseRefusesARowThatBecameAuthorized(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	// Only decision states enter through RecordAttempt, so the row is recorded
	// AUTHORIZED and placed in PREPARING — a non-terminal state the crash pass
	// selects — from the second connection.
	mustRecord(t, store, "act_race_authorized", action.StateAuthorized)
	hand := claimAttackConn(t, store)
	if _, err := hand.Exec(`UPDATE actions SET state = 'PREPARING' WHERE action_id = ?`, "act_race_authorized"); err != nil {
		t.Fatalf("place the row in PREPARING: %v", err)
	}

	// The selection: the production reader over a copy of the crash pass's
	// SELECT.
	ids, err := store.collectIDs(ctx, `SELECT action_id FROM actions
		WHERE state NOT IN (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(action.StateDenied), string(action.StateShadowed),
		string(action.StateSucceeded), string(action.StateFailed),
		string(action.StateRejected), string(action.StatePendingApproval),
		string(action.StateApproved), string(action.StateOutcomeUnknown),
		string(action.StateAuthorized))
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if len(ids) != 1 || ids[0] != "act_race_authorized" {
		t.Fatalf("the crash pass did not select the row: %v — the mould attacks nothing", ids)
	}

	// The competitor commits AUTHORIZED from a second real connection.
	if _, err := hand.Exec(`UPDATE actions SET state = 'AUTHORIZED' WHERE action_id = ?`, ids[0]); err != nil {
		t.Fatalf("the competitor's move did not land: %v", err)
	}
	receiptsBefore, err := store.ReceiptsByAction(ctx, ids[0])
	if err != nil {
		t.Fatalf("receipts before: %v", err)
	}

	changed, err := store.closeCrashOrphan(ctx, ids[0], action.StateFailed, recoveryMarkerCrash,
		crashOrphanPredicate, time.Now().UTC())
	if err != nil {
		t.Fatalf("closeCrashOrphan: %v", err)
	}
	if changed {
		t.Fatal("the crash close owned a row that had become AUTHORIZED (RowsAffected 1, want 0)")
	}
	var state string
	if err := hand.QueryRow(`SELECT state FROM actions WHERE action_id = ?`, ids[0]).Scan(&state); err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if state != string(action.StateAuthorized) {
		t.Fatalf("state = %q, want AUTHORIZED — the running attempt is not the crash pass's", state)
	}
	receiptsAfter, err := store.ReceiptsByAction(ctx, ids[0])
	if err != nil {
		t.Fatalf("receipts after: %v", err)
	}
	if len(receiptsAfter) != len(receiptsBefore) {
		t.Fatalf("receipts %d -> %d: a crash receipt was appended over a row the pass does not own",
			len(receiptsBefore), len(receiptsAfter))
	}
}

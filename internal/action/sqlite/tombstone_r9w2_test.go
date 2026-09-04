// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R9-W2 (eighth Codex pass, P2): idempotence compares the stored
// PROJECTION — the ten persisted columns that the tombstone IS — on
// BOTH sides. The full Approval struct carries fields the tombstone
// never persists (Status, RequestedAt, ExpiresAt, Reason...), so the
// old whole-struct equality was structurally false for every real
// approval: an identical legitimate re-insert (the loser of a
// decide-vs-sweep race writing the SAME decision) drew a FALSE
// tombstone_conflict. Now: identical stored story = harmless no-op;
// any difference in what is STORED = the named conflict, exactly as
// before. Reproduction-first contract.

package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"
)

// R5 — the auditor's reproduction: re-inserting the SAME decision
// from the FULL Approval (Status and window populated) is a no-op.
func TestTombstone_identicalReinsertIsANoOpOverStoredProjection(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	a0 := expiredParked(t, store, "act_w2_noop")
	env, id := operatorDecisionEnv("reject", a0.ApprovalID)
	if _, err := store.decideApproval(ctx, a0.ApprovalID, "rejected",
		a0.RequestedAt.Add(time.Second), env, id, "the decision"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	full, _, err := store.GetApproval(ctx, a0.ApprovalID)
	if err != nil {
		t.Fatalf("get full approval: %v", err)
	}
	if full.Status == "" || full.RequestedAt.IsZero() {
		t.Fatalf("precondition: the full struct must carry never-persisted fields: %+v", full)
	}
	tx, err := store.db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := store.tombstoneTx(ctx, tx, full, full.DecisionPrincipalID,
		full.Decision, full.DecisionAt); err != nil {
		t.Fatalf("AUDIT R9-W2: an identical stored story must be a no-op, not a false conflict: %v", err)
	}
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM approval_tombstones
	    WHERE approval_id = ?`, full.ApprovalID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("exactly one tombstone survives the no-op: %v n=%d", err, n)
	}
}

// R7 — a different STORED instant is a different story: named conflict.
func TestTombstone_differentStoredDecisionAtIsConflict(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	a0 := expiredParked(t, store, "act_w2_drift")
	env, id := operatorDecisionEnv("reject", a0.ApprovalID)
	if _, err := store.decideApproval(ctx, a0.ApprovalID, "rejected",
		a0.RequestedAt.Add(time.Second), env, id, "the decision"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	full, _, err := store.GetApproval(ctx, a0.ApprovalID)
	if err != nil {
		t.Fatalf("get full approval: %v", err)
	}
	tx, err := store.db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	err = store.tombstoneTx(ctx, tx, full, full.DecisionPrincipalID,
		full.Decision, full.DecisionAt.Add(time.Hour))
	if err == nil || !strings.Contains(err.Error(), "tombstone_conflict") {
		t.Fatalf("a drifted stored instant must be the named conflict: %v", err)
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R7-Y2 (sixth Codex pass, P1): the tombstone is IMMUTABLE with its
// OWN identity. INSERT OR REPLACE died: the key is the APPROVAL's
// identity (PK approval_id, UNIQUE approval_digest), plain INSERT — a
// colliding id with different content is tombstone_conflict; an
// identical row is harmless idempotence. The auditor's reproduction:
// an action_id REUSED after the prune must never overwrite the old
// approval's history — both tombstones live, and each receipt
// reconstructs ITS OWN story by the sealed digest, forever.
// Reproduction-first contract.

package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestTombstone_reusedActionIDNeverOverwritesHistory(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	// First life: parked, rejected, receipted, pruned (cascade).
	a1 := expiredParked(t, store, "act_y2_reuse")
	env1, id1 := operatorDecisionEnv("reject", a1.ApprovalID)
	if _, err := store.decideApproval(ctx, a1.ApprovalID, "rejected",
		a1.RequestedAt.Add(time.Second), env1, id1, "first life"); err != nil {
		t.Fatalf("reject 1: %v", err)
	}
	firstDigest := mustConsumedDigest(t, store, a1.ApprovalID)
	if _, err := store.db.Exec(`PRAGMA foreign_keys=ON; DELETE FROM actions WHERE action_id='act_y2_reuse'`); err != nil {
		t.Fatalf("prune: %v", err)
	}
	// Second life: the SAME action_id parks and gets rejected again.
	a2 := expiredParked(t, store, "act_y2_reuse")
	env2, id2 := operatorDecisionEnv("reject", a2.ApprovalID)
	if _, err := store.decideApproval(ctx, a2.ApprovalID, "rejected",
		a2.RequestedAt.Add(time.Second), env2, id2, "second life"); err != nil {
		t.Fatalf("reject 2: %v", err)
	}
	secondDigest := mustConsumedDigest(t, store, a2.ApprovalID)
	// BOTH tombstones live — history is immutable.
	var n int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM approval_tombstones WHERE action_id='act_y2_reuse'`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("AUDIT R7-Y2: both lives keep their tombstones: %v n=%d", err, n)
	}
	// And each reconstructs by ITS digest.
	for _, digest := range []string{firstDigest, secondDigest} {
		tomb, err := store.ApprovalTombstoneByDigest(ctx, digest)
		if err != nil {
			t.Fatalf("reconstruct %s: %v", digest, err)
		}
		if tomb.Digest() != digest {
			t.Fatalf("the preimage must re-derive its own digest: %s vs %s", tomb.Digest(), digest)
		}
	}
}

func TestTombstone_conflictRefusesIdenticalIsIdempotent(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	a := expiredParked(t, store, "act_y2_conf")
	env, ident := operatorDecisionEnv("reject", a.ApprovalID)
	if _, err := store.decideApproval(ctx, a.ApprovalID, "rejected",
		a.RequestedAt.Add(time.Second), env, ident, ""); err != nil {
		t.Fatalf("reject: %v", err)
	}
	tomb, err := store.ApprovalTombstone(ctx, "act_y2_conf")
	if err != nil {
		t.Fatalf("tombstone: %v", err)
	}
	tx, err := store.db.Begin()
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
	// Identical re-write: harmless idempotence.
	if err := store.tombstoneTx(ctx, tx, tomb, tomb.DecisionPrincipalID, tomb.Decision, tomb.DecisionAt); err != nil {
		t.Fatalf("identical tombstone must be idempotent: %v", err)
	}
	// Same approval id, DIFFERENT content: named conflict.
	lying := tomb
	lying.Decision = "approved"
	err = store.tombstoneTx(ctx, tx, lying, lying.DecisionPrincipalID, "approved", lying.DecisionAt)
	_ = tx.Rollback()
	if err == nil {
		t.Fatal("AUDIT R7-Y2: rewriting history must refuse")
	}
	if !strings.Contains(err.Error(), "tombstone_conflict") {
		t.Fatalf("the refusal must name tombstone_conflict: %v", err)
	}
}

func mustConsumedDigest(t *testing.T, store *Store, approvalID string) string {
	t.Helper()
	a, _, err := store.GetApproval(context.Background(), approvalID)
	if err != nil {
		t.Fatalf("get %s: %v", approvalID, err)
	}
	return a.Digest()
}

// R8-Z2 (seventh Codex pass, P2): whole-row idempotence. The digest
// excludes ActionID, so a lying row with the same digest and a
// FOREIGN action_id passed as idempotent. The collision now compares
// EVERY stored column: total identity = no-op, ANY difference =
// tombstone_conflict — the auditor's lying row, permanent.
func TestTombstone_lyingRowSameDigestForeignActionConflicts(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	a := expiredParked(t, store, "act_z2_lie")
	env, ident := operatorDecisionEnv("reject", a.ApprovalID)
	if _, err := store.decideApproval(ctx, a.ApprovalID, "rejected",
		a.RequestedAt.Add(time.Second), env, ident, ""); err != nil {
		t.Fatalf("reject: %v", err)
	}
	tomb, err := store.ApprovalTombstone(ctx, "act_z2_lie")
	if err != nil {
		t.Fatalf("tombstone: %v", err)
	}
	tx, err := store.db.Begin()
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	// The auditor's lying row: SAME digest terms, FOREIGN action_id
	// (ActionID is not a digest term — only the whole-row comparison
	// can catch it).
	lying := tomb
	lying.ActionID = "act_somebody_else"
	err = store.tombstoneTx(ctx, tx, lying, lying.DecisionPrincipalID, lying.Decision, lying.DecisionAt)
	if err == nil {
		t.Fatal("AUDIT R8-Z2: a same-digest foreign-action row must refuse")
	}
	if !strings.Contains(err.Error(), "tombstone_conflict") {
		t.Fatalf("the refusal must name tombstone_conflict: %v", err)
	}
}

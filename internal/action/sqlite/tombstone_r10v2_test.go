// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R10-V2 (ninth Codex pass, P1): the idempotence comparison judges
// STORED-vs-STORED. The R9-W2 projection recomputed the digest from
// the preimage on the existing side — a mutated approval_digest
// column made the original story's re-insert a nil no-op over a lying
// row, and NULL decision_at projected identically to a zero-time
// string. Now the existing side is read AS STORED (raw digest column,
// raw decision_at) and compared against what the INSERT would store.
// Reproduction-first contract.

package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// (i) — mutated stored digest + re-insert of the ORIGINAL story must
// be the named conflict, never a nil no-op over the lying column.
func TestTombstone_mutatedStoredDigestMakesReinsertAConflict(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	a0 := expiredParked(t, store, "act_v2_mut")
	env, id := operatorDecisionEnv("reject", a0.ApprovalID)
	if _, err := store.decideApproval(ctx, a0.ApprovalID, "rejected",
		a0.RequestedAt.Add(time.Second), env, id, "the decision"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if _, err := store.db.Exec(`UPDATE approval_tombstones SET approval_digest = 'sha256:mutado'
	    WHERE approval_id = ?`, a0.ApprovalID); err != nil {
		t.Fatalf("auditor's UPDATE: %v", err)
	}
	full, _, err := store.GetApproval(ctx, a0.ApprovalID)
	if err != nil {
		t.Fatalf("get full: %v", err)
	}
	tx, err := store.db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	err = store.tombstoneTx(ctx, tx, full, full.DecisionPrincipalID, full.Decision, full.DecisionAt)
	if err == nil || !strings.Contains(err.Error(), "tombstone_conflict") {
		t.Fatalf("AUDIT R10-V2: a lying stored digest must make the original story's re-insert a CONFLICT, never nil: %v", err)
	}
}

// (iii) — a stored NULL decision_at and a zero-time string are
// DIFFERENT stored stories: re-insert is a conflict, not a no-op.
func TestTombstone_storedNullIsNotZeroTimeString(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	// A migrated-from-v10 shape: decision_at NULL, digest computed
	// over the zero-time preimage (exactly what the v11 copy stores).
	a := actionpkgApproval("apr_v2_null000000000000000000001", "act_v2_null")
	if _, err := store.db.Exec(`INSERT INTO approval_tombstones
	    (approval_id, approval_digest, action_id, action_digest, preview_digest,
	     policy_version, policy_digest, decision_principal_id, decision, decision_at)
	 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		a.ApprovalID, a.Digest(), a.ActionID, a.ActionDigest, a.PreviewDigest,
		a.PolicyVersion, a.PolicyDigest, a.DecisionPrincipalID, a.Decision); err != nil {
		t.Fatalf("seed migrated NULL row: %v", err)
	}
	tx, err := store.db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	// Re-insert the same story with a zero-time INSTANT — production
	// always stores a string, so stored-NULL vs would-store-string
	// must be judged DIFFERENT.
	err = store.tombstoneTx(ctx, tx, a, a.DecisionPrincipalID, a.Decision, time.Time{})
	if err == nil || !strings.Contains(err.Error(), "tombstone_conflict") {
		t.Fatalf("AUDIT R10-V2: stored NULL is not a zero-time string — different stored stories must conflict: %v", err)
	}
}

// actionpkgApproval builds a minimal decided preimage for seeding.
func actionpkgApproval(aprID, actID string) (a action.Approval) {
	a.ApprovalID = aprID
	a.ActionID = actID
	a.ActionDigest = "sha256:aaaa"
	a.PreviewDigest = "sha256:pppp"
	a.PolicyVersion = 3
	a.PolicyDigest = "sha256:llll"
	a.DecisionPrincipalID = "principal_operator"
	a.Decision = "rejected"
	return a
}

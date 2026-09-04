// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R6-X2 (fifth Codex pass, P1): the NO's history is RECONSTRUCTED,
// not just hashed. Design (a) chosen — a bounded tombstone table
// holding the SCALAR preimage of Approval.Digest() (who, what
// decision, when, under which law, which preview) — written in the
// same transaction as every decided close, no FK (it IS evidence and
// survives the cascade, exemption declared), no bodies (fixed width).
// After the prune the verifier reconstructs the preimage and PROVES
// it against the sealed digest. Reproduction-first contract.

package sqlite

import (
	"context"
	"testing"
	"time"
)

func TestDecidedCloses_writeTheTombstonePreimage(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	// Operator reject AND sweep expiry both leave their tombstone.
	aRej := expiredParked(t, store, "act_x2_rej")
	envR, identR := operatorDecisionEnv("reject", aRej.ApprovalID)
	if _, err := store.decideApproval(ctx, aRej.ApprovalID, "rejected",
		aRej.RequestedAt.Add(time.Second), envR, identR, "no"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	aExp := expiredParked(t, store, "act_x2_exp")
	if _, _, err := store.SweepExpiredApprovals(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	for _, tc := range []struct{ actionID, approvalID string }{
		{"act_x2_rej", aRej.ApprovalID},
		{"act_x2_exp", aExp.ApprovalID},
	} {
		consumed, _, err := store.GetApproval(ctx, tc.approvalID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		got, _, err := store.ApprovalTombstone(ctx, tc.actionID)
		if err != nil {
			t.Fatalf("AUDIT R6-X2: %s must have its tombstone: %v", tc.actionID, err)
		}
		// The tombstone IS the digest preimage: it must re-derive the
		// consumed approval's digest exactly.
		if got.Digest() != consumed.Digest() {
			t.Fatalf("%s: the reconstructed preimage must re-derive the digest: %s vs %s",
				tc.actionID, got.Digest(), consumed.Digest())
		}
		if got.DecisionPrincipalID != consumed.DecisionPrincipalID || got.Decision != consumed.Decision {
			t.Fatalf("%s: who and what must survive: %+v", tc.actionID, got)
		}
	}
	// An UNDECIDED approval leaves no tombstone.
	expiredParked(t, store, "act_x2_pen_probe")
	corruptCell(t, store, "approvals", "expires_at", "approval_id",
		mustApprovalID(t, store, "act_x2_pen_probe"), time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano))
	if _, _, err := store.ApprovalTombstone(ctx, "act_x2_pen_probe"); err == nil {
		t.Fatal("a pending approval has no tombstone")
	}
}

func mustApprovalID(t *testing.T, store *Store, actionID string) string {
	t.Helper()
	var id string
	if err := store.db.QueryRow(`SELECT approval_id FROM approvals WHERE action_id = ?`, actionID).Scan(&id); err != nil {
		t.Fatalf("approval id: %v", err)
	}
	return id
}

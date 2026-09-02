// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R5 S1 (fourth Codex pass, P1): the evidence of the NO survives.
// "Honest empty for unapproved outcomes" was correct in isolation and
// became FALSE under the F4 cascade: a rejected/expired/cancelled
// approval's story lived only in its row, and the prune took the row.
// REVOKED: every close of an action that HAD an approval seals the
// DECIDED approval's digest in its receipt — approved and refused
// alike; the empty remains honest only for actions that never had an
// approval. The auditor's reproduction (reject → prune → verify)
// rides in the CLI test. Reproduction-first contract.

package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

func TestReceipts_theEvidenceOfTheNoSurvivesInTheSeal(t *testing.T) {
	t.Parallel()
	store, pub := sealedStore(t)
	ctx := context.Background()

	// REJECTED by the operator: the receipt seals the decided approval.
	aRej := expiredParked(t, store, "act_s1_rej")
	envR, identR := operatorDecisionEnv("reject", aRej.ApprovalID)
	if _, err := store.decideApproval(ctx, aRej.ApprovalID, "rejected",
		aRej.RequestedAt.Add(time.Second), envR, identR, "no"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	// EXPIRED by the sweep: same law.
	aExp := expiredParked(t, store, "act_s1_exp")
	if _, _, err := store.SweepExpiredApprovals(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	for _, tc := range []struct {
		actionID   string
		approvalID string
	}{
		{"act_s1_rej", aRej.ApprovalID},
		{"act_s1_exp", aExp.ApprovalID},
	} {
		receipts, err := store.ReceiptsByAction(ctx, tc.actionID)
		if err != nil || len(receipts) != 1 {
			t.Fatalf("%s: one receipt: %v %d", tc.actionID, err, len(receipts))
		}
		r := receipts[0]
		if r.ApprovalDigest == "" {
			t.Fatalf("AUDIT R5-S1: the NO's receipt must seal the decided approval: %s", tc.actionID)
		}
		consumed, _, err := store.GetApproval(ctx, tc.approvalID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if r.ApprovalDigest != consumed.Digest() {
			t.Fatalf("%s: the sealed digest is the DECIDED approval's: %s vs %s",
				tc.actionID, r.ApprovalDigest, consumed.Digest())
		}
		if err := action.VerifyReceiptSignature(pub, r); err != nil {
			t.Fatalf("%s: signed: %v", tc.actionID, err)
		}
	}

	// The empty stays honest ONLY where no approval ever existed.
	mustRecord(t, store, "act_s1_plain", action.StateDenied)
	receipts, _ := store.ReceiptsByAction(ctx, "act_s1_plain")
	if len(receipts) != 1 || receipts[0].ApprovalDigest != "" {
		t.Fatalf("no approval, no seal — the empty stays honest: %+v", receipts)
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R5-S2 (fourth Codex pass, P1): the STORY re-verification (effect,
// operation, principal, outcome/rule, law — against actions AND
// action_decisions) runs INSIDE the consuming transactions, over
// re-read rows. The auditor's saboteur mutates the story between any
// earlier read and the consume: both the decision and the claim must
// refuse BY NAME with the approval intact. Reproduction-first.

package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDecide_storyVerifiedInsideTheConsumingTransaction(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	a := boundPark(t, store, "act_s2_dec")
	// The saboteur strikes AFTER any read the caller did, BEFORE the
	// consume: the action row's effect class changes under our feet.
	corruptCell(t, store, "actions", "effect_class", "action_id", a.ActionID, "pure")
	envD, identD := operatorDecisionEnv("approve", a.ApprovalID)
	_, err := store.DecideApprovalUnderLaw(ctx, a.ApprovalID, "approved",
		a.RequestedAt.Add(time.Minute), envD, identD, "",
		PolicyPin{Version: 7, Digest: "sha256:law"})
	if err == nil {
		t.Fatal("AUDIT R5-S2: the decision must judge the story inside its transaction")
	}
	if !strings.Contains(err.Error(), "preview_effect_mismatch") {
		t.Fatalf("the refusal must NAME the moved dimension: %v", err)
	}
	var status string
	if err := store.db.QueryRow(`SELECT status FROM approvals WHERE approval_id = ?`,
		a.ApprovalID).Scan(&status); err != nil || status != "PENDING" {
		t.Fatalf("the approval stays intact: %v %s", err, status)
	}
}

func TestClaim_storyVerifiedInsideItsTransaction(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	a := boundPark(t, store, "act_s2_clm")
	envD, identD := operatorDecisionEnv("approve", a.ApprovalID)
	if _, err := store.decideApproval(ctx, a.ApprovalID, "approved",
		a.RequestedAt.Add(time.Minute), envD, identD, ""); err != nil {
		t.Fatalf("approve: %v", err)
	}
	// The saboteur moves the DECISION row's law before the claim.
	corruptCell(t, store, "action_decisions", "policy_digest", "action_id",
		a.ActionID, "sha256:moved-under-you")
	_, err := store.ClaimApprovalParams(ctx, a.ApprovalID,
		&PolicyPin{Version: 7, Digest: "sha256:law"})
	if err == nil {
		t.Fatal("AUDIT R5-S2: the claim must judge the story inside its transaction")
	}
	if !strings.Contains(err.Error(), "decision_policy_mismatch") {
		t.Fatalf("the refusal must NAME the moved dimension: %v", err)
	}
	// Params intact: the claim did not consume.
	if _, err := store.ApprovalParams(ctx, a.ApprovalID); err != nil {
		t.Fatalf("params must remain held on refusal: %v", err)
	}
}

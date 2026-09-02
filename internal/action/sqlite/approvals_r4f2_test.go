// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R4 Phase 2 (FR-R4F2-2/3/4): the store accepts ONLY the born-whole
// bundle; GetApproval re-verifies the persisted STORY against the
// actions row AND the action_decisions row, each dimension refusing BY
// NAME; and the law is validated INSIDE the consuming transaction over
// the re-read row. The auditor's saboteurs (b)-(f) as permanent
// members ((a) refuses at the factory, (g) is R1/F1's re-sealed
// preview, re-verified over the bundle elsewhere).
// Reproduction-first contract.

package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// boundPark parks through the REAL door: the factory, then the store.
func boundPark(t *testing.T, store *Store, id string) action.Approval {
	t.Helper()
	env := testEnvelope(id)
	env.IntentID = action.RootIntentID
	env.Principal = action.PrincipalRef{PrincipalID: "principal_brain_a"}
	env.Effect = action.Effect{Class: string(action.EffectWriteIrreversible)}
	b, err := action.NewBoundApprovalRequest(env, `{"a":1}`, action.ApprovalContext{
		IntentPurpose: "semana de pruebas",
		GrantID:       "grant_1", GrantDepth: 1, CostLine: "1 of 5",
		ToolCage: "echo cage",
		Descriptor: action.EffectDescriptor{
			Class: action.EffectWriteIrreversible, DataEgress: true,
		},
		HasDescriptor: true,
		LawVersion:    7, LawDigest: "sha256:law",
		Rule: "require_approval",
		Now:  time.Now().UTC(), TTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if err := store.CreateApprovalRequest(context.Background(), b); err != nil {
		t.Fatalf("park: %v", err)
	}
	return b.Approval()
}

func TestGetApproval_reverifiesTheStoryAgainstActionsAndDecisions(t *testing.T) {
	t.Parallel()
	saboteurs := map[string]func(t *testing.T, s *Store, actionID string){
		"preview_effect_mismatch": func(t *testing.T, s *Store, id string) {
			corruptCell(t, s, "actions", "effect_class", "action_id", id, string(action.EffectPure))
		},
		"preview_operation_mismatch": func(t *testing.T, s *Store, id string) {
			corruptCell(t, s, "actions", "op_name", "action_id", id, "calc")
		},
		"preview_principal_mismatch": func(t *testing.T, s *Store, id string) {
			corruptCell(t, s, "actions", "principal_id", "action_id", id, "principal_brain_impostor")
		},
		"decision_outcome_mismatch": func(t *testing.T, s *Store, id string) {
			corruptCell(t, s, "action_decisions", "outcome", "action_id", id, "allow")
		},
		"decision_policy_mismatch": func(t *testing.T, s *Store, id string) {
			corruptCell(t, s, "action_decisions", "policy_digest", "action_id", id, "sha256:another-law")
		},
	}
	for wantName, sabotage := range saboteurs {
		store, _ := sealedStore(t)
		a := boundPark(t, store, "act_f2_"+wantName)
		sabotage(t, store, a.ActionID)
		_, _, err := store.GetApproval(context.Background(), a.ApprovalID)
		if err == nil {
			t.Fatalf("AUDIT R4-F2: the %s saboteur must refuse the read", wantName)
		}
		if !strings.Contains(err.Error(), wantName) {
			t.Fatalf("the refusal must NAME %s: %v", wantName, err)
		}
	}
}

// Saboteur (f): the law changes BETWEEN read and consume — with the
// validation inside the consuming transaction over the re-read row,
// the sabotaged row's law is what gets judged, whatever any earlier
// read saw.
func TestDecide_lawValidatedInsideTheConsumingTransaction(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	a := boundPark(t, store, "act_f2_race")
	// The row's pinned law is rewritten out of band AFTER the park —
	// the operator's pin (the original law) no longer matches the ROW.
	corruptCell(t, store, "approvals", "policy_digest", "approval_id",
		a.ApprovalID, "sha256:law-changed-under-you")
	envD, identD := operatorDecisionEnv("approve", a.ApprovalID)
	_, err := store.DecideApprovalUnderLaw(ctx, a.ApprovalID, "approved",
		a.RequestedAt.Add(time.Minute), envD, identD, "",
		PolicyPin{Version: 7, Digest: "sha256:law"})
	if err == nil {
		t.Fatal("AUDIT R4-F2(f): the re-read row's law must be judged inside the transaction")
	}
	if !strings.Contains(err.Error(), "approval_invalidated") && !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("the refusal must name the invalidation: %v", err)
	}
	// Raw status read (GetApproval itself would now refuse the
	// sabotaged row — correct, but the evidence needs the raw cell).
	var status string
	if err := store.db.QueryRow(
		`SELECT status FROM approvals WHERE approval_id = ?`, a.ApprovalID).Scan(&status); err != nil {
		t.Fatalf("raw status: %v", err)
	}
	if status != string(action.ApprovalPending) {
		t.Fatalf("nothing consumed on refusal: %v", status)
	}
}

// The claim (execute's consume point) validates the law in ITS
// transaction too.
func TestClaim_lawValidatedInsideItsTransaction(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	a := boundPark(t, store, "act_f2_claim")
	envD, identD := operatorDecisionEnv("approve", a.ApprovalID)
	if _, err := store.decideApproval(ctx, a.ApprovalID, "approved",
		a.RequestedAt.Add(time.Minute), envD, identD, ""); err != nil {
		t.Fatalf("approve: %v", err)
	}
	corruptCell(t, store, "approvals", "policy_digest", "approval_id",
		a.ApprovalID, "sha256:law-changed-under-you")
	_, err := store.ClaimApprovalParams(ctx, a.ApprovalID,
		&PolicyPin{Version: 7, Digest: "sha256:law"})
	if err == nil {
		t.Fatal("AUDIT R4-F2(f): the claim must judge the re-read row's law")
	}
	if !strings.Contains(err.Error(), "approval_invalidated") && !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("the refusal must name the invalidation: %v", err)
	}
}

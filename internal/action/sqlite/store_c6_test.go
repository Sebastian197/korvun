// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// C6 of the E5 consolidation (second external audit, P2): the
// retention prune must KNOW the new terminals (REJECTED and
// OUTCOME_UNKNOWN would otherwise grow without bound — the
// resource-bound invariant) while the evidence exemption stays intact
// (receipts survive their pruned actions); and the approval params
// are CAPPED at birth — no user-driven unbounded blob rides into the
// store. Reproduction-first contract.

package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

func TestPrune_knowsTheNewTerminalsAndSparesTheEvidence(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t) // sealed: the evidence axis needs real receipts
	ctx := context.Background()
	// Two new-terminal actions and one legitimately parked (live) one.
	a1, _ := pendingRequest(t, store, "act_c6_r1")
	env1, ident1 := operatorDecisionEnv("reject", a1.ApprovalID)
	if _, err := store.decideApproval(ctx, a1.ApprovalID, "rejected",
		a1.RequestedAt.Add(time.Minute), env1, ident1, ""); err != nil {
		t.Fatalf("reject 1: %v", err)
	}
	a2, _ := pendingRequest(t, store, "act_c6_r2")
	env2, ident2 := operatorDecisionEnv("reject", a2.ApprovalID)
	if _, err := store.decideApproval(ctx, a2.ApprovalID, "rejected",
		a2.RequestedAt.Add(time.Minute), env2, ident2, ""); err != nil {
		t.Fatalf("reject 2: %v", err)
	}
	corruptCell(t, store, "actions", "state", "action_id", "act_c6_r2",
		string(action.StateOutcomeUnknown))
	pendingRequest(t, store, "act_c6_live")

	// The decision acts above added their own terminal rows; count what
	// is actually there and cap so that EXACTLY the two new-terminal
	// rows are the excess.
	total, err := store.Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	store.capRows = total - 2
	removed, err := store.Prune(ctx)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if removed != 2 {
		t.Fatalf("AUDIT C6: REJECTED and OUTCOME_UNKNOWN are prunable terminals: removed %d", removed)
	}
	// The OLDEST terminals are exactly the two new-terminal rows (their
	// envelopes carry the old fixed instant) — they must be the pruned
	// ones, not the fresher operator proof acts.
	for _, id := range []string{"act_c6_r1", "act_c6_r2"} {
		if _, err := store.Get(ctx, id); err == nil {
			t.Fatalf("AUDIT C6: %s is an old new-terminal and must be pruned", id)
		}
	}
	if _, err := store.Get(ctx, "act_c6_live"); err != nil {
		t.Fatalf("the parked (live) action must survive: %v", err)
	}
	// The evidence exemption is INTACT: the receipts of the pruned
	// actions still verify from the ledger.
	for _, id := range []string{"act_c6_r1"} {
		receipts, err := store.ReceiptsByAction(ctx, id)
		if err != nil || len(receipts) == 0 {
			t.Fatalf("the pruned action's receipt is EVIDENCE and survives: %v %d", err, len(receipts))
		}
	}
}

func TestCreateApprovalRequest_capsParamsAtBirth(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	big := `{"blob":"` + strings.Repeat("x", 64*1024) + `"}`
	env := action.NewEnvelope("act_c6_big", "env-1",
		action.Source{Kind: "agent_brain", Protocol: "text", Channel: "console"},
		action.Operation{Namespace: "tool", Name: "echo", Version: 1},
		big, time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC))
	a, p := testApprovalFor(env)
	err := store.CreateApprovalRequest(context.Background(), env,
		Decision{Outcome: "require_approval", Rule: "require_approval"}, a, p, big)
	if err == nil {
		t.Fatal("AUDIT C6: unbounded params must refuse at birth")
	}
	if !strings.Contains(err.Error(), "params") || !strings.Contains(err.Error(), "cap") {
		t.Fatalf("the refusal must name the params cap: %v", err)
	}
}

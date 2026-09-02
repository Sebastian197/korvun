// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// C5 of the E5 consolidation (second external audit): the E6 border,
// bounded HONESTLY. The atomic claim is the intermediate state before
// the external effect — params purged means an executor may have fired
// the effect already. A crash there cannot close FAILED (a lie when
// the webhook DID leave): it falls to OUTCOME_UNKNOWN, named, with its
// own recovery marker. Idempotency/reconciliation stay E6 and are
// declared, not implied. Reproduction-first contract.

package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

func TestRecovery_crashAfterClaimIsOutcomeUnknownNotFailed(t *testing.T) {
	t.Parallel()
	path := t.TempDir() + "/korvun.db"
	store, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	a, _ := pendingRequest(t, store, "act_c5")
	env, ident := operatorDecisionEnv("approve", a.ApprovalID)
	if _, err := store.decideApproval(ctx, a.ApprovalID, "approved",
		a.RequestedAt.Add(time.Minute), env, ident, ""); err != nil {
		t.Fatalf("approve: %v", err)
	}
	// The claim fires; the process dies before FinishWithResult — the
	// external effect may or may not have happened. Nobody knows.
	if _, err := store.ClaimApprovalParams(ctx, a.ApprovalID, nil); err != nil {
		t.Fatalf("claim: %v", err)
	}
	_ = store.Close()

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if err := reopened.RecoverPreviousLife(context.Background()); err != nil {
		t.Fatalf("recovery pass: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	rec, err := reopened.Get(ctx, "act_c5")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rec.State != action.StateOutcomeUnknown {
		t.Fatalf("AUDIT C5: a crash after the claim is UNCERTAIN, never a FAILED lie: %v", rec.State)
	}
	if rec.RecoveryMarker != "outcome_unknown" {
		t.Fatalf("the uncertainty must be NAMED in the marker: %q", rec.RecoveryMarker)
	}
	if !action.State(rec.State).Terminal() {
		t.Fatal("OUTCOME_UNKNOWN closes the action (terminal) — reconciliation is E6's stage")
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R4 of the third Codex pass (adjudicated 2026-09-01): expiry judged
// ONLY at the consume touch let PENDING rows outlive their window
// forever when nobody touched them — aggregate growth the server owns.
// The server-side sweep closes every expired PENDING (EXPIRED approval,
// REJECTED action WITH receipt, params purged) at boot and on the
// existing prune cadence. Approval rows themselves are retention-exempt
// BY DESIGN, declared: they are the 8th check's evidence.
// Reproduction-first contract.

package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// expiredParked parks a request whose window is already closed.
func expiredParked(t *testing.T, store *Store, id string) action.Approval {
	t.Helper()
	env := testEnvelope(id)
	a, p := testApprovalFor(env)
	a.ExpiresAt = env.RequestedAt.Add(time.Minute) // long past by now
	a.PreviewDigest = p.Digest()
	if err := store.CreateApprovalRequest(context.Background(), env,
		Decision{Outcome: "require_approval", Rule: a.Reason,
			PolicyVersion: a.PolicyVersion, PolicyDigest: a.PolicyDigest},
		a, p, `{"a":1}`); err != nil {
		t.Fatalf("park: %v", err)
	}
	return a
}

func TestSweepExpiredApprovals_closesWithReceiptAndPurgesParams(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	a := expiredParked(t, store, "act_r4_exp")
	alive := expiredParked(t, store, "act_r4_alive")
	// The second one is NOT expired: its window reopens far ahead.
	corruptCell(t, store, "approvals", "expires_at", "approval_id",
		alive.ApprovalID, time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano))

	swept, err := store.SweepExpiredApprovals(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if swept != 1 {
		t.Fatalf("exactly the expired one sweeps: %d", swept)
	}
	got, _, err := store.GetApproval(ctx, a.ApprovalID)
	if err != nil || got.Status != action.ApprovalExpired {
		t.Fatalf("the swept approval closes EXPIRED: %v %v", err, got.Status)
	}
	rec, err := store.Get(ctx, "act_r4_exp")
	if err != nil || rec.State != action.StateRejected {
		t.Fatalf("the parked action closes REJECTED: %v %v", err, rec.State)
	}
	receipts, err := store.ReceiptsByAction(ctx, "act_r4_exp")
	if err != nil || len(receipts) != 1 || receipts[0].SigningKeyID == "" {
		t.Fatalf("the sweep close is IN THE LEDGER, signed: %v %d", err, len(receipts))
	}
	if _, err := store.ApprovalParams(ctx, a.ApprovalID); err == nil {
		t.Fatal("swept params must be purged")
	}
	// The live one is untouched.
	if got2, _, _ := store.GetApproval(ctx, alive.ApprovalID); got2.Status != action.ApprovalPending {
		t.Fatalf("an unexpired request survives the sweep: %v", got2.Status)
	}
}

func TestSweep_runsOnThePruneCadence(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	store.pruneEvery = 1 // every committed attempt pays the sweep
	ctx := context.Background()
	a := expiredParked(t, store, "act_r4_cad")
	mustRecord(t, store, "act_r4_tick", action.StateDenied)
	got, _, err := store.GetApproval(ctx, a.ApprovalID)
	if err != nil || got.Status != action.ApprovalExpired {
		t.Fatalf("AUDIT R4: the prune cadence must sweep expiry too: %v %v", err, got.Status)
	}
}

// F3 of the third-pass self-audit (adjudicated 2026-09-02): the birth
// of an approval did not pay the prune cadence, so a server whose ONLY
// traffic is parking requests never swept its expired ones until the
// next boot — "in practice" turned into a pin: the park itself counts
// as a write.
func TestSweep_aServerThatOnlyParksStillSweeps(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	store.pruneEvery = 1
	ctx := context.Background()
	stale := expiredParked(t, store, "act_f3_stale")
	// The ONLY further traffic is another park — it must pay the sweep.
	expiredParked(t, store, "act_f3_next")
	got, _, err := store.GetApproval(ctx, stale.ApprovalID)
	if err != nil || got.Status != action.ApprovalExpired {
		t.Fatalf("AUDIT F3: the park itself pays the cadence — the stale one sweeps: %v %v", err, got.Status)
	}
}

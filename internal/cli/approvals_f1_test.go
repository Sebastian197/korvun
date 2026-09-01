// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// F1 of the second-pass self-audit (adjudicated 2026-09-01): the
// expired+law-changed cross left a zombie PENDING row — the law
// validation ran BEFORE the expiry touch, so the close-at-touch that
// FR-APR-4 promises never happened. The clock is consulted FIRST: an
// expired request closes at this touch (EXPIRED approval, REJECTED
// action, the E2 mold) whatever the law did; only a still-consumable
// approve must happen under the current law. The auditscratch
// reproduction, permanent. Reproduction-first contract.

package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
)

func TestApprovalsApprove_expiredPlusChangedLawClosesAtTheTouch(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, approvalID := parkedRequestExpiring(t, time.Now().UTC().Add(-time.Minute))
	mutateConfig(t, cfgPath, func(cfg map[string]any) {
		cfg["brains"].([]any)[0].(map[string]any)["sensitivity"] = "private"
	})
	code, _, stderr := runIntentCLI(t, "approvals", "approve", "--config", cfgPath, approvalID)
	if code == 0 {
		t.Fatal("an expired request must never approve")
	}
	// The CLOCK wins the cross: the refusal names the expiry, not the
	// law — and the touch CLOSES the row (FR-APR-4, no zombie PENDING).
	if !strings.Contains(stderr, "approval_expired") {
		t.Fatalf("AUDIT F1: the expiry touch must win the cross and name itself: %q", stderr)
	}
	store, err := actionsqlite.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	a, _, err := store.GetApproval(ctx, approvalID)
	if err != nil || a.Status != action.ApprovalExpired {
		t.Fatalf("FR-APR-4: the touch closes the approval EXPIRED, no zombie PENDING: %v %v", err, a.Status)
	}
	rec, err := store.Get(ctx, a.ActionID)
	if err != nil || rec.State != action.StateRejected {
		t.Fatalf("the parked action closes REJECTED at the expiry touch: %v %v", err, rec.State)
	}
}

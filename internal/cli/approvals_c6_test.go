// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// C6 of the E5 consolidation (second external audit, P2): the clock's
// truth reaches the CONSULTS — a PENDING request whose window already
// closed shows EXPIRED on list and show (the read-only door stays
// read-only: the row itself is closed at the next mutating touch, the
// E2 mold; the display must not wait for it).
// Reproduction-first contract.

package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
)

func TestApprovalsListAndShow_expiryReachesTheConsult(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, approvalID := parkedRequestExpiring(t, time.Now().UTC().Add(-time.Minute))
	code, stdout, stderr := runIntentCLI(t, "approvals", "list", "--config", cfgPath)
	if code != 0 {
		t.Fatalf("list: %d %q", code, stderr)
	}
	if !strings.Contains(stdout, "EXPIRED") {
		t.Fatalf("AUDIT C6: the consult must show the clock's truth: %q", stdout)
	}
	code, stdout, stderr = runIntentCLI(t, "approvals", "show", "--config", cfgPath, approvalID)
	if code != 0 {
		t.Fatalf("show: %d %q", code, stderr)
	}
	if !strings.Contains(stdout, "EXPIRED") {
		t.Fatalf("show must render the effective status: %q", stdout)
	}
	// The read-only door stayed read-only: the ROW is still PENDING
	// (the closing touch belongs to a mutating act, the E2 mold).
	store, err := actionsqlite.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	a, _, err := store.GetApproval(context.Background(), approvalID)
	if err != nil || a.Status != action.ApprovalPending {
		t.Fatalf("the consult must not mutate the row: %v %v", err, a.Status)
	}
}

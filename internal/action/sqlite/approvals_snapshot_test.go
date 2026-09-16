// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The claim judges the WHOLE approval row, not two of its columns — the
// first of the four class cures the director ordered on 2026-09-16.
//
// The P1-1 cure of the v0.15.0 train made the claim re-read `status` and the
// action's `state` inside its transaction. That closed the two columns the
// external review named and left the class open: every OTHER column of the
// row — the digests, the law pin, the window, the decision fields — was still
// read by the caller BEFORE the transaction and never compared inside it. A
// row that moved in any of them was consumed and handed to the executor.
//
// The cure: the caller hands the claim the snapshot its prechecks read, and
// the claim compares that snapshot against the row ITS transaction read,
// field by field, before consuming anything.
//
// Evidence level, honest: multiple real database connections — the attacking
// hand is a second *sql.DB on the same file, committing before the claim runs.
// In-process; not a compiled binary.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"

	_ "modernc.org/sqlite"
)

// snapshotAttackConn opens a SECOND real connection on the store's file.
func snapshotAttackConn(t *testing.T, store *Store) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+store.Path()+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open the second connection: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// approvedForSnapshot parks a request and approves it, returning the approval
// as the caller's prechecks would read it.
func approvedForSnapshot(t *testing.T, store *Store, actionID string) action.Approval {
	t.Helper()
	ctx := context.Background()
	a := boundPark(t, store, actionID)
	env, ident := operatorDecisionEnv("approve", a.ApprovalID)
	if _, err := store.decideApproval(ctx, a.ApprovalID, "approved",
		a.RequestedAt.Add(time.Minute), env, ident, ""); err != nil {
		t.Fatalf("approve: %v", err)
	}
	seen, _, err := store.GetApproval(ctx, a.ApprovalID)
	if err != nil {
		t.Fatalf("read the approval back: %v", err)
	}
	return seen
}

// TestClaim_refusesARowThatMovedInAnyColumn is the class mould. Each row moves
// ONE column that the P1-1 cure does not look at, from a second connection,
// leaving status, state, parameters and digests exactly as the caller saw
// them. The claim must refuse by name and consume nothing.
//
// Probing mutations (executed, red, declared in the canto): drop the snapshot
// comparison ⇒ every row reddens; compare only Status and the action state ⇒
// every row reddens, which is the difference between the P1-1 cure and this
// one.
func TestClaim_refusesARowThatMovedInAnyColumn(t *testing.T) {
	t.Parallel()
	// Each case carries its COMPLETE statement as a literal: building the
	// column name by concatenation is what gosec's G202 refuses, and a #nosec
	// exemption would buy nothing here — the set of columns is fixed and known.
	cases := []struct {
		name  string
		stmt  string
		value string
	}{
		{"the comment the operator will read",
			`UPDATE approvals SET comment = ? WHERE approval_id = ?`, "moved under the claim"},
		{"the principal the decision names",
			`UPDATE approvals SET decision_principal_id = ? WHERE approval_id = ?`, "principal_someone_else"},
		{"the receipt the decision points at",
			`UPDATE approvals SET decision_receipt_id = ? WHERE approval_id = ?`, "rcpt_not_the_one"},
		{"the risk line the human was shown",
			`UPDATE approvals SET risk_summary = ? WHERE approval_id = ?`, "harmless, honest"},
		{"the window the request was born with",
			`UPDATE approvals SET expires_at = ? WHERE approval_id = ?`, "2099-01-01T00:00:00Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, _ := sealedStore(t)
			ctx := context.Background()
			seen := approvedForSnapshot(t, store, "act_snapshot")

			hand := snapshotAttackConn(t, store)
			if _, err := hand.Exec(tc.stmt, tc.value, seen.ApprovalID); err != nil {
				t.Fatalf("the attack did not land: %v", err)
			}

			params, _, err := store.ClaimApprovalParamsUnderDigest(ctx, seen.ApprovalID, nil, seen.ActionDigest, &seen)
			if !errors.Is(err, ErrApprovalMovedUnderTheClaim) {
				t.Fatalf("err = %v, want ErrApprovalMovedUnderTheClaim", err)
			}
			if params != nil {
				t.Fatalf("a refused claim handed back parameters: %q", params)
			}
			var raw string
			if err := hand.QueryRow(
				`SELECT canonical_params FROM approvals WHERE approval_id = ?`,
				seen.ApprovalID).Scan(&raw); err != nil {
				t.Fatalf("re-read the parameters: %v", err)
			}
			if raw != `{"a":1}` {
				t.Fatalf("canonical_params = %q — a refused claim must consume nothing", raw)
			}
		})
	}
}

// TestClaim_theUnmovedRowStillClaims keeps the cure from becoming a wall: the
// snapshot the caller read is the row the transaction reads, so the claim wins.
//
// Probing mutation (N11, executed, red, declared in the canto): make the
// comparison judge a column the claim itself rewrites — `canonical_params`,
// which the purge empties — so an honest claim looks like a moved row ⇒ this
// reddens.
func TestClaim_theUnmovedRowStillClaims(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	seen := approvedForSnapshot(t, store, "act_snapshot_ok")

	params, op, err := store.ClaimApprovalParamsUnderDigest(ctx, seen.ApprovalID, nil, seen.ActionDigest, &seen)
	if err != nil {
		t.Fatalf("an unmoved row must claim: %v", err)
	}
	if string(params) != `{"a":1}` {
		t.Fatalf("params = %q", string(params))
	}
	if op.Name == "" {
		t.Fatal("the claim must hand back the operation it judged")
	}
}

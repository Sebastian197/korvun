// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The claim owns the AUTHORIZATION, not only the parameters — P1-1 of the
// v0.15.0 external review.
//
// ExecuteApprovedAction checks approval=APPROVED and action=APPROVED before the
// claim, outside its transaction. The claim re-read both rows, ignored both
// states, and its UPDATE only required the parameters to still be there. A
// request whose state moved after those prechecks was consumed and handed to
// the executor.
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

	_ "modernc.org/sqlite"
)

// claimAttackConn opens a SECOND real connection on the store's file: the
// external hand the reproduction needs.
func claimAttackConn(t *testing.T, store *Store) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+store.Path()+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open the second connection: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestClaim_refusesARowWhoseAuthorityMovedBeforeIt is the reproduction of the
// review, verbatim at store level: approve, then from a second connection move
// the state to REJECTED while keeping the operation triple, the parameters and
// the digests, then claim.
//
// The purge probe is the oracle by impossibility for the UPDATE's own
// condition: an AFTER UPDATE trigger that aborts. If the purge ever reaches a
// row whose authority moved, the probe turns the refusal into a driver error
// and the named assertion reddens.
//
// Probing mutations (executed, red, declared in the canto):
//   - delete the authority re-read inside the claim ⇒ the conditioned UPDATE
//     affects zero rows and the claim answers ErrApprovalClaimSkipped;
//   - drop the state conditions from the purge's WHERE ⇒ the probe aborts and
//     the claim answers ErrApprovalUnreadable.
func TestClaim_refusesARowWhoseAuthorityMovedBeforeIt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		attack string
		target func(approvalID, actionID string) string
	}{
		{
			name:   "the action row moved to REJECTED",
			attack: `UPDATE actions SET state = 'REJECTED' WHERE action_id = ?`,
			target: func(_, actionID string) string { return actionID },
		},
		{
			name:   "the approval row moved to REJECTED",
			attack: `UPDATE approvals SET status = 'REJECTED' WHERE approval_id = ?`,
			target: func(approvalID, _ string) string { return approvalID },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, _ := sealedStore(t)
			ctx := context.Background()
			a := boundPark(t, store, "act_authority")
			envD, identD := operatorDecisionEnv("approve", a.ApprovalID)
			if _, err := store.decideApproval(ctx, a.ApprovalID, "approved",
				a.RequestedAt.Add(time.Minute), envD, identD, ""); err != nil {
				t.Fatalf("approve: %v", err)
			}

			hand := claimAttackConn(t, store)
			if _, err := hand.Exec(tc.attack, tc.target(a.ApprovalID, a.ActionID)); err != nil {
				t.Fatalf("the attack did not land: %v", err)
			}
			if _, err := hand.Exec(`CREATE TRIGGER purge_probe AFTER UPDATE OF canonical_params ON approvals
				BEGIN SELECT RAISE(ABORT, 'the purge reached a row whose authority had moved'); END`); err != nil {
				t.Fatalf("install the purge probe: %v", err)
			}

			params, _, err := store.ClaimApprovalParamsUnderDigest(ctx, a.ApprovalID, nil, a.ActionDigest)
			if !errors.Is(err, ErrApprovalNoLongerApproved) {
				t.Fatalf("err = %v, want ErrApprovalNoLongerApproved", err)
			}
			if params != nil {
				t.Fatalf("a refused claim handed back parameters: %q", params)
			}
			// Nothing consumed: the bytes the attacker preserved are still there.
			var raw string
			if err := hand.QueryRow(`SELECT canonical_params FROM approvals WHERE approval_id = ?`,
				a.ApprovalID).Scan(&raw); err != nil {
				t.Fatalf("re-read the parameters: %v", err)
			}
			if raw != `{"a":1}` {
				t.Fatalf("canonical_params = %q — a refused claim must consume nothing", raw)
			}
		})
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// TestV0151B_P2_6_aWrongKindCellIsEvidenceCorrupt: a cell of the wrong storage
// class in an INTEGER column the raw reader converts (approvals.policy_version)
// is corrupt evidence — and only that — on every door that reads the row:
// GetApproval, the decide (both verbs) and ListApprovals.
//
// Two kinds, because they reach two different branches of approvalRawInt: a
// non-integral REAL (3.5) arrives from the driver as a float64, the
// wrong-KIND branch; a BLOB (x'00ff') arrives as []byte, the text-that-is-not-
// an-integer branch. The TEXT columns have no such mould: with this schema's
// declared types and the store's DSN, modernc hands a TEXT column only
// string, []byte, int64, float64 or nil, all of which approvalRawText accepts
// as text exactly as the typed Scan did — its wrong-kind branch is unreachable
// from the driver and is declared, not moulded (canto §4).
//
// An instrument proves the storage class through the second connection
// (typeof), so the row cannot pass for a well-typed one. The decide rows also
// prove nothing was decided: the approval stays PENDING and the action
// PENDING_APPROVAL.
//
// Evidence: real store file, the cell written through a second real
// connection, in-process.
// Probing mutation EXECUTED (m09, the paper's «type only parse errors»):
// untype approvalRawText's and approvalRawInt's wrong-kind branches ⇒ the REAL
// rows red; the BLOB rows stay green under it (their branch is the parse one,
// watched by m09b).
func TestV0151B_P2_6_aWrongKindCellIsEvidenceCorrupt(t *testing.T) {
	t.Parallel()
	kinds := []struct {
		name, value, class string
	}{
		{"real-3.5", `3.5`, "real"},
		{"blob-00ff", `x'00ff'`, "blob"},
	}
	type door struct {
		name string
		read func(ctx context.Context, store *Store, a action.Approval) error
	}
	decide := func(verb string) func(context.Context, *Store, action.Approval) error {
		return func(ctx context.Context, store *Store, a action.Approval) error {
			env, ident := operatorDecisionEnv(verb, a.ApprovalID)
			_, err := store.DecideApprovalUnderLaw(ctx, a.ApprovalID, verb,
				a.RequestedAt.Add(time.Minute), env, ident, "", parkedLaw)
			return err
		}
	}
	doors := []door{
		{"GetApproval", func(ctx context.Context, store *Store, a action.Approval) error {
			_, _, err := store.GetApproval(ctx, a.ApprovalID)
			return err
		}},
		{"decide-approve", decide(action.DecisionApproved)},
		{"decide-reject", decide(action.DecisionRejected)},
		{"ListApprovals", func(ctx context.Context, store *Store, _ action.Approval) error {
			_, err := store.ListApprovals(ctx, action.ApprovalPending)
			return err
		}},
	}
	for _, k := range kinds {
		for _, d := range doors {
			t.Run(k.name+"/"+d.name, func(t *testing.T) {
				t.Parallel()
				store, _ := sealedStore(t)
				a, _ := pendingRequest(t, store, "act_kind_"+k.class+"_"+d.name)
				attack(t, store, `UPDATE approvals SET policy_version = `+k.value+` WHERE approval_id = ?`, a.ApprovalID) // #nosec G202 -- test-owned literals
				db := secondConn(t, store)
				var class string
				if err := db.QueryRow(`SELECT typeof(policy_version) FROM approvals WHERE approval_id = ?`, a.ApprovalID).Scan(&class); err != nil || class != k.class {
					t.Fatalf("instrument: policy_version stored as %q (%v), want %q", class, err, k.class)
				}

				err := d.read(context.Background(), store, a)
				if !errors.Is(err, ErrApprovalEvidenceCorrupt) {
					t.Fatalf("%s err = %v, want ErrApprovalEvidenceCorrupt", d.name, err)
				}
				if errors.Is(err, ErrApprovalUnreadable) {
					t.Fatalf("err = %v carries BOTH sentinels: corrupt and unreadable are exclusive (class i)", err)
				}

				var status, state string
				if err := db.QueryRow(`SELECT status FROM approvals WHERE approval_id = ?`, a.ApprovalID).Scan(&status); err != nil || status != string(action.ApprovalPending) {
					t.Fatalf("approval status = %q (%v), want PENDING: nothing may be decided over corrupt evidence", status, err)
				}
				if err := db.QueryRow(`SELECT state FROM actions WHERE action_id = ?`, a.ActionID).Scan(&state); err != nil || state != string(action.StatePendingApproval) {
					t.Fatalf("action state = %q (%v), want PENDING_APPROVAL", state, err)
				}
			})
		}
	}
}

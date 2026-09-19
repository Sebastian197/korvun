// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The claim names a refusal by ONE class — v0.15.1 block A, cross-block item
// X2 routed from block B's pass. Its sibling X1 travels in block B's train
// (director, 2026-09-19).
//
// A refusal is either corrupt evidence (the bytes are there and do not verify:
// permanent) or an unreadable store (the store did not answer: transient). A
// caller acts on that difference — retry or not — so an error that carries
// both sentinels, or the wrong one, lies to it.
//
// Evidence level, honest: multiple real database connections — the attacking
// hand is a second *sql.DB on the same file. In-process; not a compiled
// binary.
package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// approvedForClaim parks and approves one request and hands back its ids.
func approvedForClaim(t *testing.T, store *Store, id string) (approvalID, actionDigest string) {
	t.Helper()
	a := boundPark(t, store, id)
	envD, identD := operatorDecisionEnv("approve", a.ApprovalID)
	if _, err := store.decideApproval(context.Background(), a.ApprovalID, "approved",
		a.RequestedAt.Add(time.Minute), envD, identD, ""); err != nil {
		t.Fatalf("approve: %v", err)
	}
	return a.ApprovalID, a.ActionDigest
}

// TestClaim_aCorruptHistoryRowIsCorruptEvidence is X2 (block B's pass, P3
// sister, the inverse of P2-6): a decision row whose policy_version holds text
// fails its Scan into int64, and the claim's story check wraps every Scan
// error in ErrApprovalUnreadable — so bytes that are THERE and will never
// verify are published as a transient store failure. They are corrupt
// evidence: corrupt=true, unreadable=false.
//
// Planned probing mutation (after green): route the conversion failure back
// through the Unreadable wrap ⇒ this reddens.
func TestClaim_aCorruptHistoryRowIsCorruptEvidence(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	approvalID, digest := approvedForClaim(t, store, "act_x2_history")
	hand := claimAttackConn(t, store)
	var actionID string
	if err := hand.QueryRow(`SELECT action_id FROM approvals WHERE approval_id = ?`, approvalID).Scan(&actionID); err != nil {
		t.Fatalf("read the action id: %v", err)
	}
	if _, err := hand.Exec(`UPDATE action_decisions SET policy_version = 'x' WHERE action_id = ?`, actionID); err != nil {
		t.Fatalf("corrupt the history row: %v", err)
	}

	params, _, err := store.ClaimApprovalParamsUnderDigest(context.Background(), approvalID, nil, digest, nil)
	if params != nil {
		t.Fatalf("a refused claim handed back parameters: %q", params)
	}
	corrupt, unreadable := errors.Is(err, ErrApprovalEvidenceCorrupt), errors.Is(err, ErrApprovalUnreadable)
	if !corrupt || unreadable {
		t.Fatalf("err = %v: corrupt=%v unreadable=%v, want corrupt=true unreadable=false — the bytes are there and do not verify", err, corrupt, unreadable)
	}
}

// assertOneClass fails unless err carries exactly the wanted class of the two.
func assertOneClass(t *testing.T, err error, wantCorrupt bool) {
	t.Helper()
	corrupt, unreadable := errors.Is(err, ErrApprovalEvidenceCorrupt), errors.Is(err, ErrApprovalUnreadable)
	if corrupt != wantCorrupt || unreadable == wantCorrupt {
		t.Fatalf("err = %v: corrupt=%v unreadable=%v, want corrupt=%v unreadable=%v",
			err, corrupt, unreadable, wantCorrupt, !wantCorrupt)
	}
}

// TestClaim_aHistoryTableThatCannotBeReadIsUnreadable is X2's DRIVER half
// (Delta 6): the decision table renamed away by a second connection is a
// store that did not answer, not corrupt evidence. Green today; it pins the
// cure against the wrong one that classes every history-row error as corrupt
// (the adversary's W-X2a).
//
// Planned probing mutation (after green): class every history-row error as
// corrupt ⇒ this reddens.
func TestClaim_aHistoryTableThatCannotBeReadIsUnreadable(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	approvalID, digest := approvedForClaim(t, store, "act_x2_driver")
	hand := claimAttackConn(t, store)
	if _, err := hand.Exec(`ALTER TABLE action_decisions RENAME TO action_decisions_gone`); err != nil {
		t.Fatalf("rename the history table: %v", err)
	}
	params, _, err := store.ClaimApprovalParamsUnderDigest(context.Background(), approvalID, nil, digest, nil)
	if params != nil {
		t.Fatalf("a refused claim handed back parameters: %q", params)
	}
	// A LOCATOR, by text (Delta 7): it proves the refusal came from the
	// history read and imposes the wording «read the decision of» on the
	// driver branch — a correct cure that rewords it false-reds here.
	if !strings.Contains(err.Error(), "read the decision of") {
		t.Fatalf("err = %v: the refusal is not the history read — the row does not attack what it names", err)
	}
	assertOneClass(t, err, false)
}

// TestClaim_ternaOfNamesConversionAndDriverApart is ternaOf's pair (Delta 6,
// P3 sister of X2, same file and same claim): the operation triple is read
// with a typed Scan, so text in op_version fails conversion and is wrapped
// ErrApprovalUnreadable — bytes that are there and never verify, published as
// transient. The conversion row must be corrupt; the driver row (the column
// dropped by a second connection, so the SELECT cannot prepare) must stay
// unreadable.
//
// Doors this cure reclassifies beyond the claim, declared (Delta 7, Delta 8):
// ternaOf is also read by ReReadParams, claimAuthorityTx and approvalDetail
// (approvals_v15.go), and approvalDetail also runs verifyApprovalStoryTyped;
// approvalDetail feeds ApprovalDetail / ApprovalDetailUnderLaw, which feed
// ApprovalsAdapter.Detail and its nameRead. At the operator's detail door a
// history or terna conversion failure now reads corrupt instead of
// unavailable (executed by the adversary). Unmoulded at those doors; the
// direction is the correct one.
//
// Planned probing mutations (after green): route the conversion back through
// the Unreadable wrap ⇒ the conversion row reddens; class every ternaOf error
// as corrupt ⇒ the driver row reddens.
func TestClaim_ternaOfNamesConversionAndDriverApart(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		attack      string
		wantCorrupt bool
	}{
		{"op_version holds text (conversion)", `UPDATE actions SET op_version = 'x' WHERE action_id = ?`, true},
		{"op_version dropped (driver)", `ALTER TABLE actions DROP COLUMN op_version`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, _ := sealedStore(t)
			approvalID, digest := approvedForClaim(t, store, "act_terna")
			hand := claimAttackConn(t, store)
			var actionID string
			if err := hand.QueryRow(`SELECT action_id FROM approvals WHERE approval_id = ?`, approvalID).Scan(&actionID); err != nil {
				t.Fatalf("read the action id: %v", err)
			}
			args := []any{}
			if tc.wantCorrupt {
				args = append(args, actionID)
			}
			if _, err := hand.Exec(tc.attack, args...); err != nil {
				t.Fatalf("the attack did not land: %v", err)
			}
			params, _, err := store.ClaimApprovalParamsUnderDigest(context.Background(), approvalID, nil, digest, nil)
			if params != nil {
				t.Fatalf("a refused claim handed back parameters: %q", params)
			}
			// A LOCATOR, by text, on the driver row only (Delta 7): it proves the
			// refusal came from ternaOf's read and imposes that wording on the
			// driver branch — a correct cure that rewords it false-reds here. The
			// conversion row carries no locator: its cure changes the wrap.
			if !tc.wantCorrupt && !strings.Contains(err.Error(), "read the actions row for") {
				t.Fatalf("err = %v: the refusal is not ternaOf's read — the row does not attack what it names", err)
			}
			assertOneClass(t, err, tc.wantCorrupt)
		})
	}
}

// TestClaim_anIncompleteHistoryRowIsCorruptEvidence moulds X2's NULL branch
// (the diff pass's P3-c): the decision table is recreated without its
// constraints (CREATE TABLE … AS SELECT drops NOT NULL) and policy_digest set
// to NULL. A value that is not there is corrupt evidence, never a transient
// failure.
//
// Planned probing mutation: wrap the incomplete branch unreadable ⇒ this
// reddens.
func TestClaim_anIncompleteHistoryRowIsCorruptEvidence(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	approvalID, digest := approvedForClaim(t, store, "act_x2_null")
	hand := claimAttackConn(t, store)
	for _, q := range []string{
		`ALTER TABLE action_decisions RENAME TO action_decisions_old`,
		`CREATE TABLE action_decisions AS SELECT * FROM action_decisions_old`,
		`UPDATE action_decisions SET policy_digest = NULL`,
	} {
		if _, err := hand.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	params, _, err := store.ClaimApprovalParamsUnderDigest(context.Background(), approvalID, nil, digest, nil)
	if params != nil {
		t.Fatalf("a refused claim handed back parameters: %q", params)
	}
	assertOneClass(t, err, true)
}

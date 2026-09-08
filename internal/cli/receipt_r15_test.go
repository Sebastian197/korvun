// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R15-P1A (Codex's fourteenth pass, reproduction verbatim): the
// tombstone evidence is judged WHENEVER a row carries the receipt's
// sealed digest — not only after retention took the approval AND the
// action row. Before the cure the reconstruction arms hung inside the
// both-rows-absent branch, so a tombstone mutated beside a LIVE
// approval was never read: the verifier printed OK and exited 0 over
// forged evidence.
//
// Evidence level: in-process CLI suite over a real SQLite file, the
// mutation applied through a SEPARATE raw connection.

package cli

import (
	"database/sql"
	"strings"
	"testing"
)

func TestReceiptVerify_corruptTombstoneBesideALiveApprovalIsNamed(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, approvalID := parkedRequest(t)
	// Approve — which fires the deferred execution and seals the
	// terminal receipt: approval row, action row and tombstone all
	// alive, the state the old ladder never looked at the tombstone in.
	receiptID := approvedReceiptID(t, cfgPath, dbPath, approvalID)

	// The SANE half of the pair, over this very store: a live approval
	// beside a healthy tombstone stays OK, and names no tombstone arm.
	code, stdout, stderr := runIntentCLI(t, "receipt", "verify", "--config", cfgPath, receiptID)
	out := stdout + stderr
	if code != 0 || !strings.Contains(out, "OK") {
		t.Fatalf("a healthy tombstone beside a live approval must stay OK: %d %q", code, out)
	}
	if strings.Contains(out, "tombstone_") {
		t.Fatalf("nothing is wrong with this evidence — no tombstone arm may fire: %q", out)
	}

	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatalf("raw: %v", err)
	}
	// The auditor's UPDATE, verbatim: the deciding hand is forged while
	// the stored digest column stays intact, so the row is still
	// selected by the receipt's sealed digest and the reader's contract
	// judges its preimage.
	res, err := db.Exec(`UPDATE approval_tombstones SET decision_principal_id = 'principal_forged'
	    WHERE action_id = 'act_inbox1'`)
	if err != nil {
		t.Fatalf("auditor's UPDATE: %v", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		t.Fatalf("the attack must land on exactly one tombstone, mutated %d", n)
	}
	// The scenario's own premise, pinned: NOTHING was pruned — both
	// rows the old branch demanded absent are alive.
	var approvals, actions int
	if err := db.QueryRow(`SELECT COUNT(*) FROM approvals WHERE action_id = 'act_inbox1'`).Scan(&approvals); err != nil {
		t.Fatalf("count approvals: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM actions WHERE action_id = 'act_inbox1'`).Scan(&actions); err != nil {
		t.Fatalf("count actions: %v", err)
	}
	_ = db.Close()
	if approvals != 1 || actions != 1 {
		t.Fatalf("the premise is a LIVE approval beside a live action: approvals=%d actions=%d", approvals, actions)
	}

	code, stdout, stderr = runIntentCLI(t, "receipt", "verify", "--config", cfgPath, receiptID)
	out = stdout + stderr
	// The verdict is logged so the canto's transcript is reproducible
	// from the tree (R14's L5 rule: a cited execution leaves an artifact).
	t.Logf("verdict over the forged tombstone: exit %d %q", code, out)
	if code != 1 || !strings.Contains(out, "FAIL tombstone_corrupt:") {
		t.Fatalf("AUDIT R15-P1A: forged tombstone evidence beside a live approval is the named FAIL, never OK: %d %q", code, out)
	}
	if !strings.Contains(out, "approval_digest") {
		t.Fatalf("the corruption names its stable column: %q", out)
	}
	if strings.Contains(out, "tombstone_read_failed") {
		t.Fatalf("the bytes were read perfectly — 'cannot read' would be a lie: %q", out)
	}
}

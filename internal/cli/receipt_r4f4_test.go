// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R4 Phase 4 (FR-R4F4-3, ADR-0046): the verifier distinguishes
// retention from sabotage. Both rows gone (the prune cascaded the
// approval with its terminal action) = the honest NOTE
// approval_row_absent — the digest-sealed receipt IS the surviving
// evidence, the action_row_absent mold verbatim. Action row PRESENT
// with the approval gone = approval_mismatch, a FAIL: a cascade
// cannot remove the approval while its action remains — only a
// saboteur can. Reproduction-first contract.

package cli

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
)

func approvedReceiptID(t *testing.T, cfgPath, dbPath, approvalID string) string {
	t.Helper()
	if code, _, stderr := runIntentCLI(t, "approvals", "approve", "--config", cfgPath, approvalID); code != 0 {
		t.Fatalf("approve: %q", stderr)
	}
	store, err := actionsqlite.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	receipts, err := store.ReceiptsByAction(context.Background(), "act_inbox1")
	if err != nil || len(receipts) != 1 {
		t.Fatalf("outcome receipt: %v %d", err, len(receipts))
	}
	return receipts[0].ReceiptID
}

func TestReceiptVerify_retentionIsANoteSabotageIsAFail(t *testing.T) {
	t.Parallel()
	// RETENTION: the prune cascades action AND approval — honest note.
	cfgPath, dbPath, approvalID := parkedRequest(t)
	receiptID := approvedReceiptID(t, cfgPath, dbPath, approvalID)
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatalf("raw: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM actions WHERE action_id = 'act_inbox1'`); err != nil {
		t.Fatalf("prune-style delete: %v", err)
	}
	_ = db.Close()
	code, stdout, stderr := runIntentCLI(t, "receipt", "verify", "--config", cfgPath, receiptID)
	out := stdout + stderr
	if code != 0 || !strings.Contains(out, "OK") {
		t.Fatalf("AUDIT R4-F4: retention is NOT a failure — the sealed receipt is the evidence: %d %q", code, out)
	}
	// R6-X2 raised the bar: with the tombstone present the verifier
	// RECONSTRUCTS and proves; the bare note remains only for pre-v10
	// history (no tombstone) — exercised by deleting it.
	if !strings.Contains(out, "approval_evidence_reconstructed") {
		t.Fatalf("the reconstruction must prove the history: %q", out)
	}
	db3, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("raw: %v", err)
	}
	if _, err := db3.Exec(`DELETE FROM approval_tombstones WHERE action_id = 'act_inbox1'`); err != nil {
		t.Fatalf("erase tombstone: %v", err)
	}
	_ = db3.Close()
	code, stdout, stderr = runIntentCLI(t, "receipt", "verify", "--config", cfgPath, receiptID)
	out = stdout + stderr
	if code != 0 || !strings.Contains(out, "approval_row_absent") {
		t.Fatalf("without a tombstone (pre-v10 history) the honest degraded note remains: %d %q", code, out)
	}

	// SABOTAGE: the action remains, the approval alone vanishes — FAIL.
	cfgPath2, dbPath2, approvalID2 := parkedRequest(t)
	receiptID2 := approvedReceiptID(t, cfgPath2, dbPath2, approvalID2)
	db2, err := sql.Open("sqlite", "file:"+dbPath2+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("raw: %v", err)
	}
	if _, err := db2.Exec(`DELETE FROM approvals WHERE approval_id = ?`, approvalID2); err != nil {
		t.Fatalf("sabotage: %v", err)
	}
	_ = db2.Close()
	code, stdout, stderr = runIntentCLI(t, "receipt", "verify", "--config", cfgPath2, receiptID2)
	out = stdout + stderr
	if code != 1 || !strings.Contains(out, "approval_mismatch") {
		t.Fatalf("an approval gone while its action remains is SABOTAGE: %d %q", code, out)
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R5-S1 (fourth Codex pass, P1): the auditor's reproduction — reject,
// then prune, then verify. The NO's receipt seals the decided
// approval's digest, so after the cascade takes both rows the verify
// answers OK with the honest retention note: the surviving evidence is
// TRUE for every outcome now. Reproduction-first contract.

package cli

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
)

func TestReceiptVerify_theNoSurvivesThePrune(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, approvalID := parkedRequest(t)
	if code, _, stderr := runIntentCLI(t, "approvals", "reject", "--config", cfgPath, approvalID); code != 0 {
		t.Fatalf("reject: %q", stderr)
	}
	store, err := actionsqlite.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	receipts, err := store.ReceiptsByAction(context.Background(), "act_inbox1")
	_ = store.Close()
	if err != nil || len(receipts) != 1 {
		t.Fatalf("the REJECTED close receipts: %v %d", err, len(receipts))
	}
	if receipts[0].ApprovalDigest == "" {
		t.Fatal("AUDIT R5-S1: the NO's receipt must seal the decided approval")
	}
	// The prune-shaped removal cascades both rows.
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatalf("raw: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM actions WHERE action_id = 'act_inbox1'`); err != nil {
		t.Fatalf("prune-style delete: %v", err)
	}
	_ = db.Close()
	code, stdout, stderr := runIntentCLI(t, "receipt", "verify", "--config", cfgPath, receipts[0].ReceiptID)
	out := stdout + stderr
	if code != 0 || !strings.Contains(out, "OK") {
		t.Fatalf("the NO's evidence survives retention: %d %q", code, out)
	}
	// R6-X2: the verifier RECONSTRUCTS and PROVES the history — who
	// decided what, when, under which law — from the tombstone
	// preimage, re-deriving the sealed digest. A bare note is no
	// longer enough.
	for _, must := range []string{
		"approval_evidence_reconstructed",
		"decision=rejected",
		"by=principal_operator",
		"law=v",
		"digest re-derived",
	} {
		if !strings.Contains(out, must) {
			t.Fatalf("AUDIT R6-X2: the verify must PROVE the history (missing %q): %q", must, out)
		}
	}
}

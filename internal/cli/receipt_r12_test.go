// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R12-A11: with the reader applying the one contract, a tombstone
// whose preimage was mutated under an intact stored digest surfaces
// as the typed corruption — the verifier names it tombstone_corrupt,
// never the lying "cannot read" nor the old unreachable
// approval_mismatch arm (declared class change, adversary-reviewed).
// Evidence level: in-process CLI suite. Reproduction-first contract.

package cli

import (
	"database/sql"
	"strings"
	"testing"
)

func TestReceiptVerify_mutatedPreimageUnderIntactDigestIsCorruptNamed(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, approvalID := parkedRequest(t)
	receiptID := approvedReceiptID(t, cfgPath, dbPath, approvalID)
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatalf("raw: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM actions WHERE action_id = 'act_inbox1'`); err != nil {
		t.Fatalf("prune-style delete: %v", err)
	}
	// The adversary's reproduction: mutate the PREIMAGE, keep the
	// stored digest column intact — the row stays selectable by the
	// receipt's sealed digest.
	if _, err := db.Exec(`UPDATE approval_tombstones SET decision = 'rejected'
	    WHERE action_id = 'act_inbox1'`); err != nil {
		t.Fatalf("auditor's UPDATE: %v", err)
	}
	_ = db.Close()
	code, stdout, stderr := runIntentCLI(t, "receipt", "verify", "--config", cfgPath, receiptID)
	out := stdout + stderr
	if code != 1 || !strings.Contains(out, "tombstone_corrupt") {
		t.Fatalf("AUDIT R12-A11: a mutated preimage under an intact digest is typed corruption, named: %d %q", code, out)
	}
	if strings.Contains(out, "tombstone_read_failed") {
		t.Fatalf("the bytes were read perfectly — 'cannot read' would be a lie: %q", out)
	}
}

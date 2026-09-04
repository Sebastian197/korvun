// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R10-V2(ii) (ninth Codex pass, P1): a tombstone whose STORED digest
// was mutated must be a NAMED verify failure (tombstone_corrupt) —
// never the approval_row_absent note, which is reserved for honest
// pre-v10 history. Corruption never masquerades as absence, on the
// verifier's arm too. Reproduction-first contract.

package cli

import (
	"database/sql"
	"strings"
	"testing"
)

func TestReceiptVerify_mutatedStoredTombstoneDigestFailsNamed(t *testing.T) {
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
	if _, err := db.Exec(`UPDATE approval_tombstones SET approval_digest = 'sha256:mutado'
	    WHERE action_id = 'act_inbox1'`); err != nil {
		t.Fatalf("auditor's UPDATE: %v", err)
	}
	_ = db.Close()
	code, stdout, stderr := runIntentCLI(t, "receipt", "verify", "--config", cfgPath, receiptID)
	out := stdout + stderr
	if code != 1 || !strings.Contains(out, "tombstone_corrupt") {
		t.Fatalf("AUDIT R10-V2: a mutated stored digest must FAIL by name: %d %q", code, out)
	}
	if strings.Contains(out, "approval_row_absent") {
		t.Fatalf("corruption must never masquerade as pre-v10 absence: %q", out)
	}
}

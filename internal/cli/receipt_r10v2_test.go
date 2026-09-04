// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R10-V2(ii) HISTORY, R11 SUBSTITUTION (direction decision, recorded):
// the by-action integrity arm died in R11 — three design vetoes proved
// it unrepairable without sealed provenance (its false positive
// condemned healthy receipts across reused action_ids; its false
// negative missed digests mutated into unattributability). A mutated
// stored digest is now INDISTINGUISHABLE from absence, and the
// verifier says exactly that — the ambiguous note, verbatim. The
// v2-era limit lives in SECURITY.md until the v3 sealed-provenance
// era. This pin holds the substituted truth.

package cli

import (
	"database/sql"
	"strings"
	"testing"
)

func TestReceiptVerify_mutatedStoredTombstoneDigestIsAmbiguousAbsence(t *testing.T) {
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
	if code != 0 || !strings.Contains(out,
		"no tombstone with the sealed digest exists; legacy history, deletion, or a coherent rewrite are indistinguishable") {
		t.Fatalf("R11 SUBSTITUTED PIN: unattributable corruption is honestly ambiguous absence (SECURITY.md v2-era limit): %d %q", code, out)
	}
}

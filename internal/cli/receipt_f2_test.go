// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// F2 of the second-pass self-audit (adjudicated 2026-09-01): the 8th
// verifier check swallowed the REAL cause when the consumed approval's
// preview was re-pointed — the FAIL verdict was right, the name lied
// ("no approval row exists"). The exact sabotage, permanent: a
// preview_digest rewritten out of band must FAIL naming the C2
// preview_digest_mismatch. Reproduction-first contract.

package cli

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
)

func TestReceiptVerify_repointedPreviewFailsByItsRealName(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, approvalID := parkedRequest(t)
	if code, _, stderr := runIntentCLI(t, "approvals", "approve", "--config", cfgPath, approvalID); code != 0 {
		t.Fatalf("approve: %q", stderr)
	}
	store, err := actionsqlite.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	receipts, _ := store.ReceiptsByAction(context.Background(), "act_inbox1")
	_ = store.Close()
	if len(receipts) != 1 {
		t.Fatalf("one outcome receipt: %d", len(receipts))
	}
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("raw: %v", err)
	}
	if _, err := db.Exec(`UPDATE approvals SET preview_digest = 'sha256:otra-historia' WHERE approval_id = ?`, approvalID); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	_ = db.Close()
	code, stdout, stderr := runIntentCLI(t, "receipt", "verify", "--config", cfgPath, receipts[0].ReceiptID)
	if code != 1 {
		t.Fatalf("the sabotage must FAIL the verify: %d %q", code, stdout)
	}
	out := stdout + stderr
	if !strings.Contains(out, "approval_mismatch") || !strings.Contains(out, "preview_digest_mismatch") {
		t.Fatalf("AUDIT F2: the failure must name its REAL cause: %q", out)
	}
	if strings.Contains(out, "no approval row exists") {
		t.Fatalf("AUDIT F2: the row exists — the old fictitious message must not appear: %q", out)
	}
}

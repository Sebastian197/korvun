// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The operator's verifier, chain half — Etapa 4, lote 4, pieza 2 (spec
// FR-VER §19.2): `korvun ledger check` walks one partition's WHOLE
// chain — sequence continuity (no gaps, no duplicates), hash-to-hash
// linkage from the genesis, every signature — and names the FIRST
// broken link with its reason. Gap and tamper detection as a
// first-class operation: a deleted receipt is denounced by its hole, an
// edited one by its hash. Read-only. Approved-red contract.

package cli

import (
	"database/sql"
	"strings"
	"testing"
)

// seedChain drives N operator acts so the ledger carries a real chain.
func seedChain(t *testing.T, n int) (cfgPath, dbPath string) {
	t.Helper()
	cfgPath, dbPath = intentTestConfig(t)
	for i := 0; i < n; i++ {
		code, _, stderr := runIntentCLI(t, "intent", "create", "--config", cfgPath,
			"--purpose", "chain seed", "--operations", "calc")
		if code != 0 {
			t.Fatalf("seed %d: %d %q", i, code, stderr)
		}
	}
	return cfgPath, dbPath
}

func TestLedgerCheck_intactChainReportsEveryLink(t *testing.T) {
	t.Parallel()
	cfgPath, _ := seedChain(t, 3)
	code, stdout, stderr := runIntentCLI(t, "ledger", "check", "--config", cfgPath)
	if code != 0 {
		t.Fatalf("an intact chain must check green: %d %q", code, stderr)
	}
	if !strings.Contains(stdout, "chain intact") || !strings.Contains(stdout, "main") {
		t.Fatalf("the verdict names the partition and the count: %q", stdout)
	}
}

func TestLedgerCheck_deletedReceiptDenouncedByItsHole(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath := seedChain(t, 3)
	// The saboteur deletes a MIDDLE receipt from outside the domain API.
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM receipts WHERE chain_seq = 1 AND partition = 'main'`); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_ = db.Close()
	code, stdout, stderr := runIntentCLI(t, "ledger", "check", "--config", cfgPath)
	if code != 1 {
		t.Fatalf("a hole in the chain must fail the check: %d %q %q", code, stdout, stderr)
	}
	out := stdout + stderr
	if !strings.Contains(out, "chain_seq_gap") || !strings.Contains(out, "1") {
		t.Fatalf("the hole is denounced naming the missing seq: %q", out)
	}
}

func TestLedgerCheck_editedReceiptNamedAtItsLink(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath := seedChain(t, 3)
	// Grab the seq-1 receipt and tamper its outcome.
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	var receiptID string
	if err := db.QueryRow(`SELECT receipt_id FROM receipts WHERE chain_seq = 1 AND partition = 'main'`).Scan(&receiptID); err != nil {
		t.Fatalf("find seq 1: %v", err)
	}
	if _, err := db.Exec(`UPDATE receipts SET outcome = 'FORGED' WHERE receipt_id = ?`, receiptID); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	_ = db.Close()
	code, stdout, stderr := runIntentCLI(t, "ledger", "check", "--config", cfgPath)
	if code != 1 {
		t.Fatalf("a tampered link must fail the check: %d %q %q", code, stdout, stderr)
	}
	out := stdout + stderr
	if !strings.Contains(out, receiptID) || !strings.Contains(out, "hash_mismatch") {
		t.Fatalf("the FIRST broken link is named with its receipt and reason: %q", out)
	}
}

func TestLedgerCheck_duplicateSeqDenounced(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath := seedChain(t, 2)
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	// First defense, pinned: the schema's UNIQUE constraint stops a
	// direct clone even outside the domain API.
	if _, err := db.Exec(`UPDATE receipts SET chain_seq = 0 WHERE chain_seq = 1 AND partition = 'main'`); err == nil {
		t.Fatal("the UNIQUE constraint must stop a direct seq clone")
	}
	// The realistic saboteur rebuilds the table WITHOUT the constraint
	// and injects the clone — the walk must still denounce it.
	rebuild := `
		CREATE TABLE receipts_forged AS SELECT * FROM receipts;
		DROP TABLE receipts;
		ALTER TABLE receipts_forged RENAME TO receipts;
		UPDATE receipts SET chain_seq = 0 WHERE chain_seq = 1 AND partition = 'main';`
	if _, err := db.Exec(rebuild); err != nil {
		t.Fatalf("rebuild without constraint: %v", err)
	}
	_ = db.Close()
	code, stdout, stderr := runIntentCLI(t, "ledger", "check", "--config", cfgPath)
	if code != 1 {
		t.Fatalf("a duplicated seq must fail the check: %d %q %q", code, stdout, stderr)
	}
	out := stdout + stderr
	if !strings.Contains(out, "chain_seq_duplicate") {
		t.Fatalf("the duplicate is denounced by name: %q", out)
	}
}

func TestLedgerCheck_emptyPartitionIsHonestlyEmpty(t *testing.T) {
	t.Parallel()
	cfgPath, _ := intentTestConfig(t)
	code, stdout, _ := runIntentCLI(t, "ledger", "check", "--config", cfgPath)
	if code != 0 || !strings.Contains(stdout, "0 receipts") {
		t.Fatalf("an empty ledger checks green saying so: %d %q", code, stdout)
	}
}

func TestLedgerCheck_usage(t *testing.T) {
	t.Parallel()
	if code, _, stderr := runIntentCLI(t, "ledger"); code != 2 || !strings.Contains(stderr, "expected a subcommand") {
		t.Fatalf("bare ledger: %d %q", code, stderr)
	}
	if code, _, stderr := runIntentCLI(t, "ledger", "burn"); code != 2 || !strings.Contains(stderr, "unknown subcommand") {
		t.Fatalf("unknown verb: %d %q", code, stderr)
	}
	if code, _, stderr := runIntentCLI(t, "ledger", "check"); code != 2 || !strings.Contains(stderr, "--config") {
		t.Fatalf("missing config: %d %q", code, stderr)
	}
}

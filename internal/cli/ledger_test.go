// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The operator's verifier, chain half — Etapa 4, lote 4, pieza 2 (spec
// FR-VER §19.2): `korvun ledger check` walks one partition's WHOLE
// chain — sequence continuity (no gaps, no duplicates), hash-to-hash
// linkage from the genesis, every signature — and names the FIRST
// broken link with its reason. Gap and tamper detection INSIDE the
// profile the config names: a receipt deleted from inside the chain is
// denounced by its hole, an edited one by its hash. What no test here
// can catch — a tail cut, a chain re-signed with a self-registered
// key, a redirected profile, and a row whose lookup column changed
// storage class — is SECURITY.md's honest scope of FOUR, the first
// three captured by execution in the R14 canto. Read-only. Approved-red contract.

package cli

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
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
	// D7 (R14): "1" alone matched any digit anywhere in the output — a
	// vacuous half. The hole's position and the receipt that follows it
	// are the content of the denunciation, so both are demanded here.
	if !strings.Contains(out, "FAIL chain_seq_gap: position 1 is missing (next receipt ") ||
		!strings.Contains(out, " sits at seq 2)") {
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
	// The store must EXIST (a mutating act creates it); the R1 read-only
	// door never creates the STORE when its path is absent AT THE CHECK
	// (not a general "creates no file": R14 scoped that) — adjusted
	// under the consolidation
	// mandate when the door changed.
	cfgPath, dbPath := intentTestConfig(t)
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("seed store: %v", err)
	}
	_ = store.Close()
	code, stdout, _ := runIntentCLI(t, "ledger", "check", "--config", cfgPath)
	if code != 0 || !strings.Contains(stdout, "0 receipts") {
		t.Fatalf("an empty EXISTING ledger checks green saying so: %d %q", code, stdout)
	}
}

func TestLedgerCheck_missingStoreFailsHonestWithoutCreatingIt(t *testing.T) {
	t.Parallel()
	// R1: a consult over a profile with no store fails honest and does
	// not create the store at that path (the old door silently created
	// the db). The assert below stats THAT path and nothing else — it is
	// not a "leaves no files behind" pin, and R14 scoped the comment
	// that used to say so.
	cfgPath, dbPath := intentTestConfig(t)
	code, _, stderr := runIntentCLI(t, "ledger", "check", "--config", cfgPath)
	// R14, the sixteenth diff pass: exit 1 alone was satisfied by ANY
	// exit-1 path — a config parse failure would have passed it. The
	// refusal is named: the read-only door's own, over an absent path.
	if code != 1 || !strings.Contains(stderr, "action/sqlite: read-only open") ||
		!strings.Contains(stderr, "no such file or directory") {
		t.Fatalf("a missing store must be refused BY NAME by the read-only door: %d %q", code, stderr)
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("the read-only door must not create the store: %v", err)
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

// AS-3 (blueprint mandatory): the chain carries its own evidence — a
// file-level backup restored on a clean profile verifies identically.
func TestLedgerCheck_backupRestoresVerifiable(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath := seedChain(t, 3)
	// Snapshot the states before backup.
	statesBefore := chainStates(t, cfgPath, dbPath)
	// File-level backup: the store is WAL — copy the whole family.
	restoredDir := t.TempDir()
	restoredDB := filepath.Join(restoredDir, "korvun.db")
	for _, suffix := range []string{"", "-wal", "-shm"} {
		src := dbPath + suffix
		data, err := os.ReadFile(src) // #nosec G304 -- test-owned paths
		if err != nil {
			if os.IsNotExist(err) && suffix != "" {
				continue
			}
			t.Fatalf("backup read %s: %v", src, err)
		}
		if err := os.WriteFile(restoredDB+suffix, data, 0o600); err != nil {
			t.Fatalf("backup write: %v", err)
		}
	}
	// A clean profile: a fresh config pointing at the restored file.
	restoredCfg := writeConfigFor(t, restoredDB)
	code, stdout, stderr := runIntentCLI(t, "ledger", "check", "--config", restoredCfg)
	if code != 0 {
		t.Fatalf("the restored chain must verify identically: %d %q %q", code, stdout, stderr)
	}
	// D7 (R14): this was an OR — either half satisfied it. The restored
	// verdict is ONE line and it is demanded whole.
	if !strings.Contains(stdout, "ledger main: 3 receipts, chain intact") {
		t.Fatalf("restored verdict: %q", stdout)
	}
	statesAfter := chainStates(t, restoredCfg, restoredDB)
	if len(statesBefore) != len(statesAfter) {
		t.Fatalf("restore must carry every state: %d vs %d", len(statesBefore), len(statesAfter))
	}
	for id, st := range statesBefore {
		if statesAfter[id] != st {
			t.Fatalf("state of %s diverged after restore: %q vs %q", id, st, statesAfter[id])
		}
	}
}

// chainStates maps action_id -> state for every receipted action.
func chainStates(t *testing.T, cfgPath, dbPath string) map[string]string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = db.Close() }()
	rows, err := db.Query(`SELECT r.action_id, a.state FROM receipts r JOIN actions a ON a.action_id = r.action_id`)
	if err != nil {
		t.Fatalf("states: %v", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var id, st string
		if err := rows.Scan(&id, &st); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out[id] = st
	}
	return out
}

// AS-1's third hand: a REORDERED chain (two receipts swapping places)
// is denounced — chain_seq is a sealed field, so the swap breaks the
// moved receipts' own hashes.
func TestLedgerCheck_reorderedChainDenounced(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath := seedChain(t, 3)
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	// Swap seq 0 and 1 through a temporary slot (the UNIQUE constraint
	// blocks a direct swap).
	for _, q := range []string{
		`UPDATE receipts SET chain_seq = 99 WHERE chain_seq = 0 AND partition = 'main'`,
		`UPDATE receipts SET chain_seq = 0 WHERE chain_seq = 1 AND partition = 'main'`,
		`UPDATE receipts SET chain_seq = 1 WHERE chain_seq = 99 AND partition = 'main'`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("swap: %v", err)
		}
	}
	_ = db.Close()
	code, stdout, stderr := runIntentCLI(t, "ledger", "check", "--config", cfgPath)
	if code != 1 {
		t.Fatalf("a reordered chain must fail the check: %d %q %q", code, stdout, stderr)
	}
	out := stdout + stderr
	// D7 (R14): this was an either/or, and an either/or hides which of
	// the two arms is reachable. The reorder is denounced by ONE named
	// check and that name is demanded.
	if !strings.Contains(out, "hash_mismatch") {
		t.Fatalf("the reorder is denounced at its first broken link: %q", out)
	}
}

// writeConfigFor writes a minimal strict config pointing at dbPath.
func writeConfigFor(t *testing.T, dbPath string) string {
	t.Helper()
	cfg := map[string]any{
		"schema_version": 1,
		"storage":        map[string]any{"path": dbPath},
		"brains": []map[string]any{{
			"name": "a", "sensitivity": "public",
			"policy": map[string]any{"kind": "priority"},
			"models": []map[string]any{{"provider": "ollama", "model_id": "m", "locality": "local"}},
		}},
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	cfgPath := filepath.Join(filepath.Dir(dbPath), "korvun.json")
	if err := os.WriteFile(cfgPath, raw, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return cfgPath
}

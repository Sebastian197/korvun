// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The operator's verifier, verify half — Etapa 4, lote 4, pieza 1 (spec
// FR-VER, the E2 CLI mold): `korvun receipt verify` re-judges one
// receipt OFFLINE against the store file — canonical roundtrip through
// the fuzzed parser, hash recompute, signature against the REGISTERED
// public key by its signing_key_id, the key's validity window, the
// chain link to the predecessor, and coherence with the aduana row.
// Every failure NAMED — never a generic "invalid". The operator's own
// CLI acts leave signed receipts that verify (the judge judges itself).
// Approved-red contract.

package cli

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"

	_ "modernc.org/sqlite"
)

// operatorReceipt drives one CLI mutation and returns the receipt id of
// the operator's own act plus its action id.
func operatorReceipt(t *testing.T) (cfgPath, dbPath, receiptID, actionID string) {
	t.Helper()
	cfgPath, dbPath = intentTestConfig(t)
	code, _, stderr := runIntentCLI(t, "intent", "create", "--config", cfgPath,
		"--purpose", "verify me", "--operations", "calc")
	if code != 0 {
		t.Fatalf("create: %d %q", code, stderr)
	}
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	receipts, err := store.ListReceipts(context.Background(), "main")
	if err != nil {
		t.Fatalf("list receipts: %v", err)
	}
	if len(receipts) == 0 {
		t.Fatal("the operator's CLI act must leave its signed receipt (the judge judges itself)")
	}
	last := receipts[len(receipts)-1]
	return cfgPath, dbPath, last.ReceiptID, last.ActionID
}

// corruptReceiptCell rewrites one receipts column from OUTSIDE the
// domain API — the saboteur's hand.
func corruptReceiptCell(t *testing.T, dbPath, column, receiptID, value string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = db.Close() }()
	// #nosec G202 -- test-owned column name, sabotage on purpose
	query := `UPDATE receipts SET ` + column + ` = ? WHERE receipt_id = ?`
	if _, err := db.Exec(query, value, receiptID); err != nil {
		t.Fatalf("corrupt %s: %v", column, err)
	}
}

func TestReceiptVerify_theJudgeJudgesItself(t *testing.T) {
	t.Parallel()
	cfgPath, _, receiptID, _ := operatorReceipt(t)
	code, stdout, stderr := runIntentCLI(t, "receipt", "verify", "--config", cfgPath, receiptID)
	if code != 0 {
		t.Fatalf("a sound receipt must verify: %d %q", code, stderr)
	}
	if !strings.Contains(stdout, receiptID) || !strings.Contains(stdout, "OK") {
		t.Fatalf("the verdict names the receipt: %q", stdout)
	}
}

func TestReceiptVerify_byActionIDVerifiesItsReceipts(t *testing.T) {
	t.Parallel()
	cfgPath, _, _, actionID := operatorReceipt(t)
	code, stdout, _ := runIntentCLI(t, "receipt", "verify", "--config", cfgPath, actionID)
	if code != 0 || !strings.Contains(stdout, "OK") {
		t.Fatalf("verify by action id: %d %q", code, stdout)
	}
}

func TestReceiptVerify_namesEveryFailure(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		sabotage func(t *testing.T, dbPath, receiptID string)
		want     string
	}{
		{"tampered outcome breaks the hash", func(t *testing.T, dbPath, receiptID string) {
			corruptReceiptCell(t, dbPath, "outcome", receiptID, "SUCCEEDED_FORGED")
		}, "hash_mismatch"},
		{"re-hashed forgery breaks the signature", func(t *testing.T, dbPath, receiptID string) {
			// The saboteur edits AND recomputes the hash — the ink catches it.
			rehashForgedOutcome(t, dbPath, receiptID)
		}, "signature_invalid"},
		{"unknown key id", func(t *testing.T, dbPath, receiptID string) {
			corruptReceiptCell(t, dbPath, "signing_key_id", receiptID, "ed25519:deadbeefdeadbeef")
		}, "key_unknown"},
		{"receipt signed outside its key's life", func(t *testing.T, dbPath, receiptID string) {
			retireKeyBefore(t, dbPath, receiptID)
		}, "key_window_violated"},
		{"broken chain link", func(t *testing.T, dbPath, receiptID string) {
			corruptReceiptCell(t, dbPath, "previous_receipt_hash", receiptID,
				"sha256:0000000000000000000000000000000000000000000000000000000000000000")
		}, "chain_link_broken"},
		{"aduana row divergence", func(t *testing.T, dbPath, receiptID string) {
			divergeAduanaRow(t, dbPath, receiptID)
		}, "custody_mismatch"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfgPath, dbPath, receiptID, _ := operatorReceipt(t)
			tc.sabotage(t, dbPath, receiptID)
			code, stdout, stderr := runIntentCLI(t, "receipt", "verify", "--config", cfgPath, receiptID)
			if code != 1 {
				t.Fatalf("a sabotaged receipt must fail verification: %d %q %q", code, stdout, stderr)
			}
			out := stdout + stderr
			if !strings.Contains(out, tc.want) {
				t.Fatalf("the failure must be NAMED %q, got: %q", tc.want, out)
			}
			if strings.Contains(out, "invalid receipt") {
				t.Fatalf("never a generic verdict: %q", out)
			}
		})
	}
}

func TestReceiptVerify_unknownReceiptFailsLoud(t *testing.T) {
	t.Parallel()
	cfgPath, _, _, _ := operatorReceipt(t)
	code, _, stderr := runIntentCLI(t, "receipt", "verify", "--config", cfgPath, "rcpt_missing00000000")
	if code != 1 || !strings.Contains(stderr, "rcpt_missing00000000") {
		t.Fatalf("an unknown receipt fails naming the id: %d %q", code, stderr)
	}
}

func TestReceiptVerify_isReadOnly(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, receiptID, _ := operatorReceipt(t)
	before := countRows(t, dbPath, "receipts")
	beforeActs := countRows(t, dbPath, "actions")
	if code, _, stderr := runIntentCLI(t, "receipt", "verify", "--config", cfgPath, receiptID); code != 0 {
		t.Fatalf("verify: %d %q", code, stderr)
	}
	if countRows(t, dbPath, "receipts") != before || countRows(t, dbPath, "actions") != beforeActs {
		t.Fatal("verify is READ-ONLY: it must record nothing")
	}
}

func countRows(t *testing.T, dbPath, table string) int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// rehashForgedOutcome edits the outcome AND recomputes a consistent
// hash — only the signature can catch this forgery.
func rehashForgedOutcome(t *testing.T, dbPath, receiptID string) {
	t.Helper()
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	r, err := store.GetReceipt(context.Background(), receiptID)
	if err != nil {
		t.Fatalf("get receipt: %v", err)
	}
	r.Outcome = "SUCCEEDED_FORGED"
	forgedHash := action.ComputeReceiptHash(r)
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(
		`UPDATE receipts SET outcome = ?, receipt_hash = ? WHERE receipt_id = ?`,
		r.Outcome, forgedHash, receiptID); err != nil {
		t.Fatalf("forge: %v", err)
	}
}

// retireKeyBefore retires the receipt's signing key BEFORE the receipt
// was sealed — a receipt signed outside its key's life.
func retireKeyBefore(t *testing.T, dbPath, receiptID string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(
		`UPDATE signing_keys SET retired_at = '2020-01-01T00:00:00Z'
		  WHERE key_id = (SELECT signing_key_id FROM receipts WHERE receipt_id = ?)`,
		receiptID); err != nil {
		t.Fatalf("retire: %v", err)
	}
}

// divergeAduanaRow flips the aduana row's terminal state out from under
// the receipt — outcome coherence must catch it.
func divergeAduanaRow(t *testing.T, dbPath, receiptID string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(
		`UPDATE actions SET state = 'FAILED'
		  WHERE action_id = (SELECT action_id FROM receipts WHERE receipt_id = ?)`,
		receiptID); err != nil {
		t.Fatalf("diverge: %v", err)
	}
}

func TestReceiptCmd_usageAndErrorPaths(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		code int
		want string
	}{
		{"no subcommand", []string{"receipt"}, 2, "expected a subcommand"},
		{"unknown subcommand", []string{"receipt", "burn"}, 2, "unknown subcommand"},
		{"verify without config", []string{"receipt", "verify", "rcpt_x"}, 2, "--config"},
		{"verify without id", []string{"receipt", "verify", "--config", "x.json"}, 2, "usage"},
		{"verify with unreadable config", []string{"receipt", "verify", "--config", "/nonexistent/korvun.json", "rcpt_x"}, 1, ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			code, _, stderr := runIntentCLI(t, tc.args...)
			if code != tc.code {
				t.Fatalf("exit %d, want %d (%q)", code, tc.code, stderr)
			}
			if tc.want != "" && !strings.Contains(stderr, tc.want) {
				t.Fatalf("stderr %q must mention %q", stderr, tc.want)
			}
		})
	}
}

func TestReceiptVerify_actionWithNoReceiptsFailsLoud(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, _, _ := operatorReceipt(t)
	// An AUTHORIZED action has no receipt yet — verify by its action id
	// must say so, not report OK on nothing.
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	env := operatorProbeEnvelope("act_norcpt")
	if err := store.RecordAttempt(context.Background(), env,
		actionsqlite.Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized); err != nil {
		t.Fatalf("record: %v", err)
	}
	_ = store.Close()
	code, _, stderr := runIntentCLI(t, "receipt", "verify", "--config", cfgPath, "act_norcpt")
	if code != 1 || !strings.Contains(stderr, "no receipts") {
		t.Fatalf("no-receipt action must fail loud: %d %q", code, stderr)
	}
}

// operatorProbeEnvelope builds a minimal identified-less envelope for
// seeding aduana rows in verifier tests.
func operatorProbeEnvelope(id string) action.Envelope {
	return action.NewEnvelope(id, "env-probe",
		action.Source{Kind: "operator", Protocol: "cli", Channel: "cli"},
		action.Operation{Namespace: "tool", Name: "echo", Version: 1},
		`{}`, time.Now().UTC())
}

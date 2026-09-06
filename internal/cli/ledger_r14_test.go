// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R14: the forgery pin. `korvun ledger check` and `korvun receipt
// verify` say a chain is intact after a forger who can WRITE the store
// registers a signing key of his own, re-signs every receipt, re-links
// the chain and fixes the action row. This test PINS that blind spot
// so the day an external anchor lands (v0.15.1) it goes red BY DESIGN
// and is rewritten by that train — a red here is the anchor arriving,
// never a regression.
//
// Evidence level: in-process. The BINARY capture of the same forgery,
// with its raw-SQL sabotage and both verifiers' exit 0, lives in the
// R14 canto; this pin is the durable half so the capture cannot decay.
//
// Its conforming probing mutation, executed and recorded in the canto
// (M7): delete the forger's key registration and leave the re-sign
// loop intact — the pin goes RED before either command runs, at the
// active-key probe below ("1 active keys in the registry, want 2"),
// because its green DEPENDS on the key being self-registered. That is
// the mutation that proves what this test is named for, and the red it
// produces is a precondition red, not a ladder name. Beside it (M6): invert the
// key-lookup arm of the ladder
// (`internal/cli/receipt.go`, the `key_unknown` branch) so a FOUND key
// fails — the PIN'S OWN assertion goes RED with that exact name. The
// first form of this test carried an honest-chain precondition, and
// the mutation reddened THAT line instead, proving nothing; the
// precondition is gone for exactly that reason. Its separation half
// (M4): `want := int64(i)` → `want := r.ChainSeq` in ledgerCheck's
// pass 1 leaves this pin GREEN while reddening the gap and duplicate
// tests. Red under M6, green under M4: that pair is the evidence.

package cli

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// TestLedgerCheck_chainReSignedWithASelfRegisteredKeyIsNOTDetected pins
// the third thing the verifier does not prove (SECURITY.md's honest
// scope): that the keys are the AUTHORITATIVE ones.
func TestLedgerCheck_chainReSignedWithASelfRegisteredKeyIsNOTDetected(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath := seedChain(t, 2)

	// NO honest-chain precondition here, on purpose: it would share the
	// blast radius of this pin's own mutation (M6 breaks verification
	// for every receipt), so the red would land on the precondition and
	// prove nothing about the pin. The intact chain has its own test.
	//
	// THE SABOTAGE, by direct SQL only: a key of the forger's own,
	// registered active, then every receipt re-hashed, re-linked and
	// re-signed with it, and the action row fixed to agree.
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate the forger's key: %v", err)
	}
	keyID := action.SigningKeyID(pub)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO signing_keys (key_id, public_key, created_at, retired_at)
		 VALUES (?, ?, ?, NULL)`,
		keyID, hex.EncodeToString(pub), "1970-01-01T00:00:00Z"); err != nil {
		t.Fatalf("register the forger's key: %v", err)
	}

	receipts := readChain(ctx, t, db)
	if len(receipts) != 2 {
		t.Fatalf("expected 2 receipts, got %d", len(receipts))
	}
	forged := "FAILED"
	if receipts[0].Outcome == "FAILED" {
		forged = "SUCCEEDED"
	}
	receipts[0].Outcome = forged
	for i := range receipts {
		if i > 0 {
			receipts[i].PreviousReceiptHash = receipts[i-1].ReceiptHash
		}
		receipts[i].ReceiptHash = action.ComputeReceiptHash(receipts[i])
		receipts[i] = action.SignReceipt(priv, receipts[i])
		if _, err := db.ExecContext(ctx,
			`UPDATE receipts SET outcome = ?, previous_receipt_hash = ?, receipt_hash = ?,
			        signing_key_id = ?, signature = ? WHERE receipt_id = ?`,
			receipts[i].Outcome, receipts[i].PreviousReceiptHash, receipts[i].ReceiptHash,
			receipts[i].SigningKeyID, receipts[i].Signature, receipts[i].ReceiptID); err != nil {
			t.Fatalf("re-sign %s: %v", receipts[i].ReceiptID, err)
		}
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE actions SET state = ? WHERE action_id = ?`, forged, receipts[0].ActionID); err != nil {
		t.Fatalf("fix the action row: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close the store: %v", err)
	}

	// THE FORGERY LANDED — observed, not assumed. Without this the pin's
	// oracle would be satisfied by an untouched honest chain (exit 0,
	// the same summary line, the same OK): the assertions below cannot
	// distinguish the attack from no attack, so the attack is proved
	// here, from the store, before they run.
	db3, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("reopen to observe the sabotage: %v", err)
	}
	var storedKey, storedOutcome string
	if err := db3.QueryRowContext(ctx,
		`SELECT signing_key_id, outcome FROM receipts WHERE chain_seq = 0`).
		Scan(&storedKey, &storedOutcome); err != nil {
		t.Fatalf("read back the forged receipt: %v", err)
	}
	var actives int
	if err := db3.QueryRowContext(ctx,
		`SELECT count(*) FROM signing_keys WHERE retired_at IS NULL`).Scan(&actives); err != nil {
		t.Fatalf("count the active keys: %v", err)
	}
	_ = db3.Close()
	if storedKey != keyID {
		t.Fatalf("the sabotage did not land: seq 0 is signed by %q, not by the forger's %q",
			storedKey, keyID)
	}
	if storedOutcome != forged {
		t.Fatalf("the sabotage did not land: seq 0 attests %q, not the forged %q",
			storedOutcome, forged)
	}
	if actives != 2 {
		t.Fatalf("the sabotage did not land: %d active keys in the registry, want 2", actives)
	}

	// THE PIN. Both verifiers bless the forgery. The assertions name the
	// EXACT outcome, never "some success": the summary line with its
	// count, and the receipt's OK line with its sequence.
	code, stdout, stderr := runIntentCLI(t, "ledger", "check", "--config", cfgPath)
	if code != 0 || !strings.Contains(stdout, "ledger main: 2 receipts, chain intact") {
		t.Fatalf("AUDIT R14: `ledger check` does NOT detect a chain re-signed with a "+
			"self-registered key — if this reddened, the external anchor of v0.15.1 has "+
			"landed and this pin must be rewritten by that train: %d %q %q", code, stdout, stderr)
	}
	code, stdout, stderr = runIntentCLI(t, "receipt", "verify", "--config", cfgPath, receipts[0].ReceiptID)
	if code != 0 || !strings.Contains(stdout, "(seq 0): OK") {
		t.Fatalf("AUDIT R14: `receipt verify` does NOT detect the forged receipt — "+
			"same reading as above: %d %q %q", code, stdout, stderr)
	}

	// The control, in the same test so the pin cannot pass by being
	// blind: the LAZY forger, who edits without re-signing, IS named.
	db2, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("reopen the store: %v", err)
	}
	if _, err := db2.ExecContext(ctx,
		`UPDATE receipts SET outcome = 'DENIED' WHERE chain_seq = 0`); err != nil {
		t.Fatalf("the lazy edit: %v", err)
	}
	_ = db2.Close()
	code, stdout, _ = runIntentCLI(t, "ledger", "check", "--config", cfgPath)
	if code != 1 || !strings.Contains(stdout, "hash_mismatch") {
		t.Fatalf("AUDIT R14 control: the LAZY forger must be named `hash_mismatch`, "+
			"which proves this pin is watching a live verifier: %d %q", code, stdout)
	}
}

// readChain reads one partition's receipts as the domain sees them.
func readChain(ctx context.Context, t *testing.T, db *sql.DB) []action.Receipt {
	t.Helper()
	rows, err := db.QueryContext(ctx,
		`SELECT receipt_id, action_id, intent_digest, principal_id, authority_digest,
		        decision_digest, action_digest, effect_class, attempt, outcome,
		        result_digest, started_at, finished_at, partition, chain_seq,
		        previous_receipt_hash, receipt_hash, signing_key_id, signature,
		        schema_version, approval_digest
		   FROM receipts ORDER BY chain_seq ASC`)
	if err != nil {
		t.Fatalf("read the chain: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []action.Receipt
	for rows.Next() {
		var (
			r        action.Receipt
			class    string
			started  sql.NullString
			finished sql.NullString
			approval sql.NullString
		)
		if err := rows.Scan(&r.ReceiptID, &r.ActionID, &r.IntentDigest, &r.PrincipalID,
			&r.AuthorityDigest, &r.DecisionDigest, &r.ActionDigest, &class, &r.Attempt,
			&r.Outcome, &r.ResultDigest, &started, &finished, &r.Partition, &r.ChainSeq,
			&r.PreviousReceiptHash, &r.ReceiptHash, &r.SigningKeyID, &r.Signature,
			&r.SchemaVersion, &approval); err != nil {
			t.Fatalf("scan a receipt: %v", err)
		}
		r.EffectClass = action.EffectClass(class)
		if approval.Valid {
			r.ApprovalDigest = approval.String
		}
		if started.Valid && started.String != "" {
			if ts, err := time.Parse(time.RFC3339Nano, started.String); err == nil {
				r.StartedAt = ts.UTC()
			}
		}
		if finished.Valid && finished.String != "" {
			if ts, err := time.Parse(time.RFC3339Nano, finished.String); err == nil {
				r.FinishedAt = ts.UTC()
			}
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate the chain: %v", err)
	}
	return out
}

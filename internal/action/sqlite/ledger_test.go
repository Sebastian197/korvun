// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The persisted ledger contract — Etapa 4, lote 3, pieza 1 (spec
// FR-LED): migration v5→v6 brings the receipts table on the anti-zombie
// runner; every TERMINAL outcome appends its signed receipt in the SAME
// transaction (DENIED/SHADOWED at record — the sealed NC-1/NC-2 yeses;
// SUCCEEDED/FAILED at Finish), through a domain API with NO update or
// delete paths; the chain is monotonic per partition from the genesis
// link, gap-free and duplicate-free under the -race hammer; an append
// failure fails the WHOLE record closed (record_failed elevated); and
// the evidence SURVIVES the E1 prune. A nil sealer keeps the pre-stage
// behavior byte-for-byte. Approved-red contract.

package sqlite

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// rawV5Delta is the v4→v5 DDL restated literally (oracle discipline).
const rawV5Delta = `
CREATE TABLE signing_keys (
    key_id     TEXT NOT NULL PRIMARY KEY,
    public_key TEXT NOT NULL,
    created_at TEXT NOT NULL,
    retired_at TEXT
) WITHOUT ROWID;
UPDATE action_schema SET version = 5;`

// buildV5File hand-builds a real v5 store file.
func buildV5File(t *testing.T) string {
	t.Helper()
	path := buildV4File(t)
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(rawV5Delta); err != nil {
		t.Fatalf("build v5 delta: %v", err)
	}
	return path
}

// sealerKeys registers each test sealer's private half so mixed-era
// tests can hand-sign historical receipts with the SAME key.
var (
	sealerKeysMu sync.Mutex
	sealerKeys   = map[string]ed25519.PrivateKey{}
)

// testSealer wires a REAL Ed25519 signer as the store's sealer seam.
func testSealer(t *testing.T) (func(action.Receipt) action.Receipt, ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	sealerKeysMu.Lock()
	sealerKeys[string(pub)] = priv
	sealerKeysMu.Unlock()
	return func(r action.Receipt) action.Receipt {
		return action.SignReceipt(priv, r)
	}, pub
}

func sealedStore(t *testing.T) (*Store, ed25519.PublicKey) {
	t.Helper()
	store, _ := openTemp(t)
	t.Cleanup(func() { _ = store.Close() })
	sealer, pub := testSealer(t)
	store.SetReceiptSealer(sealer)
	return store, pub
}

func TestMigrationV6_freshFileLandsOnV6WithReceipts(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "korvun.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	v, err := store.SchemaVersion(context.Background())
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if v != schemaVersionCurrent {
		t.Fatalf("a fresh store lands on the current version, got %d", v)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if n := inspect(t, path,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='receipts'`); n != 1 {
		t.Fatalf("the receipts table must exist, got %d", n)
	}
}

func TestMigrationV6_crashMidMigrationNeverLeavesAZombie(t *testing.T) {
	t.Parallel()
	path := buildV5File(t)
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := db.Exec(
		`CREATE TRIGGER crash_mid_v6 BEFORE UPDATE ON action_schema
		 BEGIN SELECT RAISE(ABORT, 'crash mid v6'); END;`,
	); err != nil {
		t.Fatalf("install crash trigger: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("an aborted v6 migration must be boot-fatal")
	}
	if v := inspect(t, path, `SELECT version FROM action_schema`); v != 5 {
		t.Fatalf("aborted migration must leave version 5, got %d", v)
	}
	if n := inspect(t, path,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='receipts'`); n != 0 {
		t.Fatalf("ZOMBIE: receipts survived the rollback (%d)", n)
	}
	db2, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw 2: %v", err)
	}
	if _, err := db2.Exec(`DROP TRIGGER crash_mid_v6`); err != nil {
		t.Fatalf("drop trigger: %v", err)
	}
	if err := db2.Close(); err != nil {
		t.Fatalf("close raw 2: %v", err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatalf("the next boot must complete: %v", err)
	}
	defer func() { _ = store.Close() }()
	if v, _ := store.SchemaVersion(context.Background()); v != schemaVersionCurrent {
		t.Fatalf("completed migration must land on current, got %v", v)
	}
}

func TestReceipt_deniedIsBornSealedWithItsRow(t *testing.T) {
	t.Parallel()
	store, pub := sealedStore(t)
	ctx := context.Background()
	if err := store.RecordAttempt(ctx, testEnvelope("act_led1"),
		Decision{Outcome: "deny", Rule: "not_granted", PolicyVersion: 5, PolicyDigest: "sha256:law"},
		action.StateDenied); err != nil {
		t.Fatalf("record: %v", err)
	}
	receipts, err := store.ReceiptsByAction(ctx, "act_led1")
	if err != nil {
		t.Fatalf("receipts: %v", err)
	}
	if len(receipts) != 1 {
		t.Fatalf("a DENIED outcome births exactly one receipt, got %d", len(receipts))
	}
	r := receipts[0]
	if r.Outcome != string(action.StateDenied) || r.Partition != "main" {
		t.Fatalf("receipt shape: %+v", r)
	}
	if r.ChainSeq != 0 || r.PreviousReceiptHash != action.GenesisPreviousHash {
		t.Fatalf("the first receipt opens the chain at the genesis link: %+v", r)
	}
	if r.ReceiptHash != action.ComputeReceiptHash(r) {
		t.Fatal("the stored hash must equal the recomputed hash")
	}
	if err := action.VerifyReceiptSignature(pub, r); err != nil {
		t.Fatalf("the born receipt must verify: %v", err)
	}
	// SHADOWED births its receipt too (sealed NC-2).
	if err := store.RecordAttempt(ctx, testEnvelope("act_led2"),
		Decision{Outcome: "shadow", Rule: "shadow"}, action.StateShadowed); err != nil {
		t.Fatalf("record shadow: %v", err)
	}
	shadowReceipts, err := store.ReceiptsByAction(ctx, "act_led2")
	if err != nil || len(shadowReceipts) != 1 {
		t.Fatalf("SHADOWED births a receipt (NC-2): %v %d", err, len(shadowReceipts))
	}
	if shadowReceipts[0].ChainSeq != 1 || shadowReceipts[0].PreviousReceiptHash != r.ReceiptHash {
		t.Fatalf("the chain links: %+v", shadowReceipts[0])
	}
}

func TestReceipt_authorizedSealsAtFinishWithResultDigest(t *testing.T) {
	t.Parallel()
	store, pub := sealedStore(t)
	ctx := context.Background()
	if err := store.RecordAttempt(ctx, testEnvelope("act_led3"),
		Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized); err != nil {
		t.Fatalf("record: %v", err)
	}
	// AUTHORIZED is not terminal: no receipt yet.
	if receipts, _ := store.ReceiptsByAction(ctx, "act_led3"); len(receipts) != 0 {
		t.Fatalf("no receipt before the terminal close, got %d", len(receipts))
	}
	resultDigest := action.HashCanonical(`{"answer":4}`)
	if err := store.FinishWithResult(ctx, "act_led3", action.StateSucceeded,
		time.Date(2026, 8, 30, 16, 0, 0, 0, time.UTC), resultDigest); err != nil {
		t.Fatalf("finish: %v", err)
	}
	receipts, err := store.ReceiptsByAction(ctx, "act_led3")
	if err != nil || len(receipts) != 1 {
		t.Fatalf("the terminal close births the receipt: %v %d", err, len(receipts))
	}
	r := receipts[0]
	if r.Outcome != string(action.StateSucceeded) || r.ResultDigest != resultDigest {
		t.Fatalf("outcome + result digest on the receipt: %+v", r)
	}
	if err := action.VerifyReceiptSignature(pub, r); err != nil {
		t.Fatalf("verify: %v", err)
	}
	// Plain Finish keeps working (empty result digest).
	if err := store.RecordAttempt(ctx, testEnvelope("act_led4"),
		Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized); err != nil {
		t.Fatalf("record 2: %v", err)
	}
	if err := store.Finish(ctx, "act_led4", action.StateFailed, time.Now().UTC()); err != nil {
		t.Fatalf("finish 2: %v", err)
	}
	failed, _ := store.ReceiptsByAction(ctx, "act_led4")
	if len(failed) != 1 || failed[0].Outcome != string(action.StateFailed) || failed[0].ResultDigest != "" {
		t.Fatalf("FAILED receipts with empty result digest: %+v", failed)
	}
}

// TestReceipt_appendFailureFailsTheWholeRecordClosed: record_failed
// elevated — no receipt, no row, no decision. Together or nothing.
func TestReceipt_appendFailureFailsTheWholeRecordClosed(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	if _, err := store.db.Exec(
		`CREATE TRIGGER block_receipts BEFORE INSERT ON receipts
		 BEGIN SELECT RAISE(ABORT, 'blocked by test'); END;`,
	); err != nil {
		t.Fatalf("install blocker: %v", err)
	}
	err := store.RecordAttempt(ctx, testEnvelope("act_led5"),
		Decision{Outcome: "deny", Rule: "not_granted"}, action.StateDenied)
	if err == nil {
		t.Fatal("an unappendable receipt must fail the WHOLE record")
	}
	if _, err := store.Get(ctx, "act_led5"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("nothing may land when the receipt cannot: %v", err)
	}
}

// TestReceipt_concurrentHammerNoGapsNoDuplicates (-race): the chain
// sequence stays contiguous and unique under concurrent appends.
func TestReceipt_concurrentHammerNoGapsNoDuplicates(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	const n = 60
	var wg sync.WaitGroup
	var failures atomic.Int64
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			env := testEnvelope("act_h" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)) + string(rune('a'+i%7)))
			env.ActionID = env.ActionID + "_" + string(rune('0'+i%10)) + string(rune('0'+(i/10)%10))
			if err := store.RecordAttempt(ctx, env,
				Decision{Outcome: "deny", Rule: "not_granted"}, action.StateDenied); err != nil {
				failures.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if failures.Load() != 0 {
		t.Fatalf("%d concurrent appends failed", failures.Load())
	}
	receipts, err := store.ListReceipts(ctx, "main")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(receipts) != n {
		t.Fatalf("want %d receipts, got %d", n, len(receipts))
	}
	seen := map[int64]bool{}
	for i, r := range receipts {
		if seen[r.ChainSeq] {
			t.Fatalf("duplicate chain seq %d", r.ChainSeq)
		}
		seen[r.ChainSeq] = true
		if int64(i) != r.ChainSeq {
			t.Fatalf("gap or disorder at position %d: seq %d", i, r.ChainSeq)
		}
		if i > 0 && r.PreviousReceiptHash != receipts[i-1].ReceiptHash {
			t.Fatalf("broken link at seq %d", r.ChainSeq)
		}
	}
}

// TestReceipt_evidenceSurvivesThePrune: the sealed exemption — the E1
// prune may take action rows; the receipts and their chain stay INTACT.
func TestReceipt_evidenceSurvivesThePrune(t *testing.T) {
	t.Parallel()
	store, err := openWithCap(t.TempDir()+"/korvun.db", 1)
	if err != nil {
		t.Fatalf("open with cap: %v", err)
	}
	defer func() { _ = store.Close() }()
	sealer, pub := testSealer(t)
	store.SetReceiptSealer(sealer)
	ctx := context.Background()
	for _, id := range []string{"act_p1", "act_p2", "act_p3"} {
		if err := store.RecordAttempt(ctx, testEnvelope(id),
			Decision{Outcome: "deny", Rule: "not_granted"}, action.StateDenied); err != nil {
			t.Fatalf("record %s: %v", id, err)
		}
	}
	if _, err := store.Prune(ctx); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if _, err := store.Get(ctx, "act_p1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("sanity: the prune must have taken the oldest action rows: %v", err)
	}
	receipts, err := store.ListReceipts(ctx, "main")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(receipts) != 3 {
		t.Fatalf("the evidence must SURVIVE the prune: got %d of 3", len(receipts))
	}
	for i, r := range receipts {
		if err := action.VerifyReceiptSignature(pub, r); err != nil {
			t.Fatalf("receipt %d must still verify after the prune: %v", i, err)
		}
	}
}

// TestReceipt_nilSealerKeepsPreStageBehavior: the exterior pinned — no
// sealer wired means no receipts and byte-for-byte E3 behavior.
func TestReceipt_nilSealerKeepsPreStageBehavior(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	if err := store.RecordAttempt(ctx, testEnvelope("act_nil"),
		Decision{Outcome: "deny", Rule: "not_granted"}, action.StateDenied); err != nil {
		t.Fatalf("record: %v", err)
	}
	receipts, err := store.ReceiptsByAction(ctx, "act_nil")
	if err != nil || len(receipts) != 0 {
		t.Fatalf("no sealer, no receipts — the pre-stage behavior: %v %d", err, len(receipts))
	}
}

func TestLedger_deepErrorBranches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// Closed store: loud failures on the read surfaces.
	closed, _ := openTemp(t)
	_ = closed.Close()
	if _, err := closed.ReceiptsByAction(ctx, "act_x"); err == nil {
		t.Fatal("closed store must fail loud on ReceiptsByAction")
	}
	if _, err := closed.ListReceipts(ctx, "main"); err == nil {
		t.Fatal("closed store must fail loud on ListReceipts")
	}
	// Corrupt receipt cells fail the scan loud.
	store, _ := sealedStore(t)
	if err := store.RecordAttempt(ctx, testEnvelope("act_corr"),
		Decision{Outcome: "deny", Rule: "not_granted"}, action.StateDenied); err != nil {
		t.Fatalf("record: %v", err)
	}
	corruptCell(t, store, "receipts", "started_at", "action_id", "act_corr", "garbage")
	if _, err := store.ReceiptsByAction(ctx, "act_corr"); err == nil {
		t.Fatal("a corrupt receipt cell must fail loud")
	}
	// Finish on a sealed store whose receipt append is blocked: the whole
	// close rolls back — the action stays AUTHORIZED.
	store2, _ := sealedStore(t)
	if err := store2.RecordAttempt(ctx, testEnvelope("act_fin"),
		Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized); err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, err := store2.db.Exec(
		`CREATE TRIGGER block_fin_receipt BEFORE INSERT ON receipts
		 BEGIN SELECT RAISE(ABORT, 'blocked by test'); END;`,
	); err != nil {
		t.Fatalf("install blocker: %v", err)
	}
	if err := store2.Finish(ctx, "act_fin", action.StateSucceeded, time.Now().UTC()); err == nil {
		t.Fatal("an unappendable finish receipt must fail the whole close")
	}
	rec, err := store2.Get(ctx, "act_fin")
	if err != nil || rec.State != action.StateAuthorized {
		t.Fatalf("the failed close must roll back whole: %v %v", err, rec.State)
	}
}

func TestLedger_intentDigestAndFinishBranches(t *testing.T) {
	t.Parallel()
	store, pub := sealedStore(t)
	ctx := context.Background()
	// An identified terminal outcome under a STORED intent carries the
	// contract's digest on its receipt (the intent-digest resolution).
	if err := store.CreateIntent(ctx, draftIntent("int_led")); err != nil {
		t.Fatalf("create intent: %v", err)
	}
	ident := testIdentity()
	ident.IntentID = "int_led"
	env := testEnvelope("act_intd")
	env.IntentID = "int_led"
	if err := store.RecordAttemptIdentified(ctx, env,
		Decision{Outcome: "deny", Rule: "not_granted"}, action.StateDenied, ident); err != nil {
		t.Fatalf("record identified: %v", err)
	}
	receipts, err := store.ReceiptsByAction(ctx, "act_intd")
	if err != nil || len(receipts) != 1 {
		t.Fatalf("identified terminal births its receipt: %v %d", err, len(receipts))
	}
	if receipts[0].IntentDigest != draftIntent("int_led").Digest() {
		t.Fatalf("the receipt carries the stored contract's digest, got %q", receipts[0].IntentDigest)
	}
	if err := action.VerifyReceiptSignature(pub, receipts[0]); err != nil {
		t.Fatalf("verify: %v", err)
	}
	// An unknown intent id resolves to the honest empty digest.
	env2 := testEnvelope("act_noint")
	env2.IntentID = "int_ghost"
	if err := store.RecordAttempt(ctx, env2,
		Decision{Outcome: "deny", Rule: "not_granted"}, action.StateDenied); err != nil {
		t.Fatalf("record: %v", err)
	}
	ghost, _ := store.ReceiptsByAction(ctx, "act_noint")
	if len(ghost) != 1 || ghost[0].IntentDigest != "" {
		t.Fatalf("unknown intents yield an empty digest honestly: %+v", ghost)
	}
	// Finishing a ghost action still reports not-found.
	if err := store.FinishWithResult(ctx, "act_ghost", action.StateSucceeded, time.Now().UTC(), ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ghost finish: %v", err)
	}
	// A corrupt requested_at breaks receiptForFinish loudly.
	if err := store.RecordAttempt(ctx, testEnvelope("act_badts"),
		Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized); err != nil {
		t.Fatalf("record: %v", err)
	}
	corruptCell(t, store, "actions", "requested_at", "action_id", "act_badts", "garbage")
	if err := store.Finish(ctx, "act_badts", action.StateSucceeded, time.Now().UTC()); err == nil {
		t.Fatal("a corrupt requested_at must fail the receipted close loud")
	}
}

func TestLedger_getReceiptAndReceiptAt(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	if err := store.RecordAttempt(ctx, testEnvelope("act_get1"),
		Decision{Outcome: "deny", Rule: "not_granted"}, action.StateDenied); err != nil {
		t.Fatalf("record: %v", err)
	}
	receipts, err := store.ReceiptsByAction(ctx, "act_get1")
	if err != nil || len(receipts) != 1 {
		t.Fatalf("seed receipt: %v %d", err, len(receipts))
	}
	got, err := store.GetReceipt(ctx, receipts[0].ReceiptID)
	if err != nil || got.ReceiptHash != receipts[0].ReceiptHash {
		t.Fatalf("GetReceipt round-trips the row: %v %+v", err, got)
	}
	at, err := store.ReceiptAt(ctx, ledgerPartition, 0)
	if err != nil || at.ReceiptID != receipts[0].ReceiptID {
		t.Fatalf("ReceiptAt(0) is the genesis receipt: %v %+v", err, at)
	}
	if _, err := store.GetReceipt(ctx, "rcpt_ghost"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown receipt id: %v", err)
	}
	if _, err := store.ReceiptAt(ctx, ledgerPartition, 99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty chain position: %v", err)
	}
	closed, _ := openTemp(t)
	_ = closed.Close()
	if _, err := closed.GetReceipt(ctx, "rcpt_x"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("closed store must fail loud, not not-found: %v", err)
	}
	if _, err := closed.ReceiptAt(ctx, ledgerPartition, 0); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("closed store must fail loud, not not-found: %v", err)
	}
}

// The cross-check family (external audit 2026-08-31, law point 4):
// the REAL retention prune meets every read the verifier performs.
// Prune takes action rows (the sealed exemption keeps receipts); every
// verifier read — receipt fetch, chain walk, key lookup, the action
// row's honest absence — must keep working over the pruned store.
func TestCrossCheck_realPruneMeetsTheVerifierReads(t *testing.T) {
	t.Parallel()
	store, err := openWithCap(t.TempDir()+"/korvun.db", 1)
	if err != nil {
		t.Fatalf("open with cap: %v", err)
	}
	defer func() { _ = store.Close() }()
	sealer, pub := testSealer(t)
	store.SetReceiptSealer(sealer)
	ctx := context.Background()
	ids := []string{"act_x1", "act_x2", "act_x3"}
	for _, id := range ids {
		if err := store.RecordAttempt(ctx, testEnvelope(id),
			Decision{Outcome: "deny", Rule: "not_granted"}, action.StateDenied); err != nil {
			t.Fatalf("record %s: %v", id, err)
		}
	}
	pruned, err := store.Prune(ctx)
	if err != nil || pruned == 0 {
		t.Fatalf("the REAL prune must take rows: %v %d", err, pruned)
	}
	receipts, err := store.ListReceipts(ctx, "main")
	if err != nil || len(receipts) != 3 {
		t.Fatalf("every receipt survives the real prune: %v %d", err, len(receipts))
	}
	prunedRows := 0
	for i, r := range receipts {
		// The verifier's reads, one by one, over the pruned store.
		if r.ReceiptHash != action.ComputeReceiptHash(r) {
			t.Fatalf("receipt %d hash must recompute", i)
		}
		if err := action.VerifyReceiptSignature(pub, r); err != nil {
			t.Fatalf("receipt %d signature: %v", i, err)
		}
		if i > 0 {
			pred, err := store.ReceiptAt(ctx, "main", r.ChainSeq-1)
			if err != nil || r.PreviousReceiptHash != pred.ReceiptHash {
				t.Fatalf("receipt %d chain link over pruned store: %v", i, err)
			}
		}
		if _, err := store.Get(ctx, r.ActionID); errors.Is(err, ErrNotFound) {
			prunedRows++
		}
	}
	if prunedRows == 0 {
		t.Fatal("the scenario needs at least one receipt whose action row is GONE")
	}
}

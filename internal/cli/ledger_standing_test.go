// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.16.2 · the durable mark «ledger founded by this profile» at the operator's
// tools (director's order, 2026-09-24): `ledger check` and `receipt verify`
// NAME the standing without changing their verdict, and every CLI writer
// refuses a foreign ledger by name, writing nothing.
//
// Evidence level, honest: the CLI called in process through Run over a real
// store on disk. Not a compiled binary in a separate OS process.
//
// Plan: docs/superpowers/specs/2026-09-24-v0162-el-libro-fundado-por-este-perfil-pretest.md, G1, G11, D6.

package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/app"
)

// foundLedgerFor marks the profile's ledger as founded by owner, the way a
// completed bootstrap leaves it: one act closed with the founder's mark, sealed
// with the profile's own key.
func foundLedgerFor(t *testing.T, cfgPath, dbPath, owner string) {
	t.Helper()
	store, err := openOperatorStoreSealed(cfgPath)
	if err != nil {
		t.Fatalf("open sealed: %v", err)
	}
	defer func() { _ = store.Close() }()
	env := action.NewEnvelope(action.NewID(), "a",
		action.Source{Kind: "operator", Protocol: "http", Channel: "console"},
		action.Operation{Namespace: "config", Name: "config.enable-storage", Version: 1}, `{"door":"enable-storage"}`, time.Now().UTC())
	if err := store.RecordAttempt(context.Background(), env, actionsqlite.Decision{Outcome: "allow", Rule: "operator"}, action.StateAuthorized); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := store.FinishFounding(context.Background(), env.ActionID, owner); err != nil {
		t.Fatalf("found: %v", err)
	}
	_ = dbPath
}

// copyProfile writes the same profile document at another path — a profile
// COPIED elsewhere, pointing at the same ledger.
func copyProfile(t *testing.T, cfgPath string) string {
	t.Helper()
	raw, err := os.ReadFile(cfgPath) // #nosec G304 -- test-owned temp path
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	other := filepath.Join(t.TempDir(), "copy", "korvun.json")
	if err := os.MkdirAll(filepath.Dir(other), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(other, raw, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return other
}

// TestLedgerCheck_namesTheStanding is G1/G2/G5 through the reader: legacy
// before any mark, ok for the founder, foreign for a copy elsewhere — and the
// verdict on the chain does not change.
//
// PROBING MUTATION: print nothing about the standing. All three legs redden.
func TestLedgerCheck_namesTheStanding(t *testing.T) {
	cfgPath, dbPath := seedChain(t, 2)
	code, stdout, _ := runIntentCLI(t, "ledger", "check", "--config", cfgPath)
	if code != 0 || !strings.Contains(stdout, "ledger standing: legacy_unfounded") {
		t.Fatalf("before any mark: %d %q, want chain intact and legacy_unfounded named", code, stdout)
	}
	foundLedgerFor(t, cfgPath, dbPath, app.ProfileIdentity(cfgPath))
	code, stdout, _ = runIntentCLI(t, "ledger", "check", "--config", cfgPath)
	if code != 0 || !strings.Contains(stdout, "ledger standing: ok") || !strings.Contains(stdout, "chain intact") {
		t.Fatalf("for the founder: %d %q, want ok and chain intact", code, stdout)
	}
	copy := copyProfile(t, cfgPath)
	code, stdout, _ = runIntentCLI(t, "ledger", "check", "--config", copy)
	if code != 0 || !strings.Contains(stdout, "ledger standing: ledger_foreign_profile") || !strings.Contains(stdout, "chain intact") {
		t.Fatalf("for a copy elsewhere: %d %q, want ledger_foreign_profile named and the chain still intact", code, stdout)
	}
	if !strings.Contains(stdout, app.ProfileIdentity(cfgPath)) || !strings.Contains(stdout, app.ProfileIdentity(copy)) {
		t.Fatalf("the foreign line names neither the founder nor this profile: %q", stdout)
	}
}

// TestReceiptVerify_namesTheStanding: the per-receipt verifier says the
// standing once, and its OK/FAIL on the receipt is unchanged by it.
//
// PROBING MUTATION: fail the verify on a foreign ledger. This reddens on the
// exit code.
func TestReceiptVerify_namesTheStanding(t *testing.T) {
	cfgPath, dbPath := seedChain(t, 1)
	foundLedgerFor(t, cfgPath, dbPath, app.ProfileIdentity(cfgPath))
	store, err := openOperatorStore(cfgPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	receipts, err := store.ListReceipts(context.Background(), "main")
	_ = store.Close()
	if err != nil || len(receipts) == 0 {
		t.Fatalf("receipts: %d err %v", len(receipts), err)
	}
	copy := copyProfile(t, cfgPath)
	code, stdout, _ := runIntentCLI(t, "receipt", "verify", "--config", copy, receipts[0].ReceiptID)
	if code != 0 || !strings.Contains(stdout, ": OK") {
		t.Fatalf("verify over a foreign ledger changed its verdict: %d %q", code, stdout)
	}
	if !strings.Contains(stdout, "ledger standing: ledger_foreign_profile") {
		t.Fatalf("verify did not name the standing: %q", stdout)
	}
}

// TestCLI_aForeignLedgerRefusesEveryWriter is G11: the operator's own writers
// carry the profile's identity and refuse a foreign ledger by name, exit 1,
// nothing written — no act, and no key moved.
//
// PROBING MUTATIONS: (1) open the sealed store without setting the identity —
// the intent lands and this reddens on the count; (2) open rotate-key's store
// without it (the official pass's find: it opened the store by hand) — the
// rotation lands, exit 0, and this reddens on the row, the count and the key.
func TestCLI_aForeignLedgerRefusesEveryWriter(t *testing.T) {
	cfgPath, dbPath := seedChain(t, 1)
	foundLedgerFor(t, cfgPath, dbPath, app.ProfileIdentity(cfgPath))
	copy := copyProfile(t, cfgPath)
	keyPath := filepath.Join(filepath.Dir(dbPath), "keys", "receipt-signing.key")
	keyBefore, err := os.ReadFile(keyPath) // #nosec G304 -- test-owned temp path
	if err != nil {
		t.Fatalf("read the founder's key: %v", err)
	}
	countIntents := func() int {
		s, err := actionsqlite.OpenReadOnlyFor(dbPath, testProfileIdentity)
		if err != nil {
			t.Fatalf("open ro: %v", err)
		}
		defer func() { _ = s.Close() }()
		n, err := s.Count(context.Background())
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}
	before := countIntents()
	writers := [][]string{
		{"intent", "create", "--config", copy, "--purpose", "foreign", "--operations", "calc"},
		// The one writer that opens the store by hand rather than through the
		// sealed opener: it must set the identity itself.
		{"receipt", "rotate-key", "--config", copy},
	}
	for _, args := range writers {
		code, stdout, stderr := runIntentCLI(t, args...)
		if code != 1 || !strings.Contains(stdout+stderr, "ledger_foreign_profile") {
			t.Fatalf("%v over a foreign ledger = %d %q %q, want exit 1 naming ledger_foreign_profile", args, code, stdout, stderr)
		}
	}
	if after := countIntents(); after != before {
		t.Fatalf("a foreign ledger gained rows: %d → %d", before, after)
	}
	keyAfter, err := os.ReadFile(keyPath) // #nosec G304 -- test-owned temp path
	if err != nil {
		t.Fatalf("read the founder's key again: %v", err)
	}
	if !bytes.Equal(keyBefore, keyAfter) {
		t.Fatal("a foreign profile rotated the founder's signing key")
	}
	// The founder itself still writes — and still rotates.
	if code, _, stderr := runIntentCLI(t, "intent", "create", "--config", cfgPath, "--purpose", "own", "--operations", "calc"); code != 0 {
		t.Fatalf("the founder was refused: %d %q", code, stderr)
	}
	if code, _, stderr := runIntentCLI(t, "receipt", "rotate-key", "--config", cfgPath); code != 0 {
		t.Fatalf("the founder's rotation was refused: %d %q", code, stderr)
	}
}

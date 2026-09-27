// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R04, R06, R19 and R22 of the redesign plan at the operator's tools.
// Evidence level, honest: the CLI called in process through Run over a real
// store on disk; the tampers through a second real connection.

package cli

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/app"
	"github.com/Sebastian197/korvun/internal/config"
)

func rawLedger(t *testing.T, dbPath string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// R04 · the row is the state and the receipt is the evidence: `ledger check`
// names the row's owner, says the row is not covered by the chain, and keeps
// its chain verdict.
//
// PROBING MUTATION: derive the standing from the receipt → reddens on the owner.
func TestLedgerCheck_theRowIsTheStateAndTheReceiptIsTheEvidence(t *testing.T) {
	cfgPath, dbPath := seedChain(t, 1)
	foundLedgerFor(t, cfgPath, dbPath, app.ProfileIdentity(cfgPath))
	other := action.HashCanonical(`{"profile":"/srv/elsewhere/korvun.json"}`)
	raw := rawLedger(t, dbPath)
	if _, err := raw.Exec(`UPDATE ledger_identity SET owner_digest = ?`, other); err != nil {
		t.Fatalf("hand edit: %v", err)
	}
	code, stdout, _ := runIntentCLI(t, "ledger", "check", "--config", cfgPath)
	if code != 0 || !strings.Contains(stdout, "ledger standing: ledger_foreign_profile") || !strings.Contains(stdout, other) || !strings.Contains(stdout, "chain intact") {
		t.Fatalf("ledger check after a hand edit of the row = %d %q, want foreign owned by the edited digest with the chain intact", code, stdout)
	}
	if !strings.Contains(stdout, "identity row: not covered by the chain") {
		t.Fatalf("ledger check does not say that the row is outside the chain: %q", stdout)
	}
	code, stdout, stderr := runIntentCLI(t, "intent", "create", "--config", cfgPath, "--purpose", "x", "--operations", "calc")
	if code != 1 || !strings.Contains(stdout+stderr, "ledger_foreign_profile") {
		t.Fatalf("intent create after the hand edit = %d %q %q, want exit 1 naming ledger_foreign_profile", code, stdout, stderr)
	}
}

// R06 · a receipt mark that the migration could not turn into a row is named
// by `ledger check` and refuses the writers.
//
// PROBING MUTATION: turn a junk mark into a row at migration → reddens.
func TestLedgerCheck_namesAMalformedReceiptMarkAfterTheMigration(t *testing.T) {
	cfgPath, dbPath := seedChain(t, 1)
	foundLedgerFor(t, cfgPath, dbPath, app.ProfileIdentity(cfgPath))
	raw := rawLedger(t, dbPath)
	for _, q := range []string{
		`UPDATE receipts SET result_digest = 'profile:xyz' WHERE result_digest LIKE 'profile:%'`,
		`DROP TABLE ledger_identity`,
		`UPDATE action_schema SET version = 15`,
	} {
		if _, err := raw.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	// The boot's door migrates; the reader then names what the migration left.
	store, err := openBootStoreForTest(cfgPath)
	if err != nil {
		t.Fatalf("boot open: %v", err)
	}
	_ = store.Close()
	code, stdout, _ := runIntentCLI(t, "ledger", "check", "--config", cfgPath)
	if code != 1 || !strings.Contains(stdout, "ledger standing: ledger_unreadable") || !strings.Contains(stdout, "ledger_mark_malformed") {
		t.Fatalf("ledger check after migrating a junk mark = %d %q, want unreadable naming ledger_mark_malformed", code, stdout)
	}
	code, stdout, stderr := runIntentCLI(t, "intent", "create", "--config", cfgPath, "--purpose", "x", "--operations", "calc")
	if code != 1 || !strings.Contains(stdout+stderr, "ledger_mark_malformed") {
		t.Fatalf("intent create = %d %q %q, want exit 1 naming ledger_mark_malformed", code, stdout, stderr)
	}
}

// R19 · the identity tables are under the guard: a foreign ledger refuses the
// CLI's identity registration itself, so no principal of another profile
// lands before the act is refused. The ledger is founded WITHOUT the CLI's
// principal (through the store, not through a CLI verb), so the copy's first
// verb is the one that would register it.
//
// PROBING MUTATION: leave principals out of the guard → the principal lands
// and reddens on the count.
func TestCLI_identityRegistrationIsRefusedOnAForeignLedger(t *testing.T) {
	cfgPath, dbPath := intentTestConfig(t)
	foundLedgerWithoutTheCLIPrincipal(t, cfgPath, dbPath)
	copy := copyProfile(t, cfgPath)
	raw := rawLedger(t, dbPath)
	var before int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM principals`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runIntentCLI(t, "intent", "create", "--config", copy, "--purpose", "foreign", "--operations", "calc")
	if code != 1 || !strings.Contains(stdout+stderr, "ledger_foreign_profile") {
		t.Fatalf("intent create from a copy = %d %q %q, want exit 1 naming ledger_foreign_profile", code, stdout, stderr)
	}
	var after int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM principals`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("principals %d → %d: a foreign profile registered its principal before the refusal", before, after)
	}
	// The founder registers and writes.
	if code, _, stderr := runIntentCLI(t, "intent", "create", "--config", cfgPath, "--purpose", "own", "--operations", "calc"); code != 0 {
		t.Fatalf("the founder was refused: %d %q", code, stderr)
	}
}

// foundLedgerWithoutTheCLIPrincipal founds the ledger for the profile at
// cfgPath through the store and the profile's ink, WITHOUT the CLI's identity
// wiring, so the ledger has never seen the CLI's principal.
func foundLedgerWithoutTheCLIPrincipal(t *testing.T, cfgPath, dbPath string) {
	t.Helper()
	store, err := actionsqlite.OpenOperatorFor(dbPath, app.ProfileIdentity(cfgPath))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	priv, err := app.EnsureSigningKey(ctx, store, filepath.Dir(dbPath))
	if err != nil {
		t.Fatalf("ink: %v", err)
	}
	store.SetReceiptSealer(func(r action.Receipt) action.Receipt { return action.SignReceipt(priv, r) })
	env := action.NewEnvelope(action.NewID(), "a",
		action.Source{Kind: "operator", Protocol: "http", Channel: "console"},
		action.Operation{Namespace: "config", Name: "config.enable-storage", Version: 1}, `{"door":"enable-storage"}`, time.Now().UTC())
	if err := store.RecordAttempt(ctx, env, actionsqlite.Decision{Outcome: "allow", Rule: "operator"}, action.StateAuthorized); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := store.FinishFounding(ctx, env.ActionID, app.ProfileIdentity(cfgPath)); err != nil {
		t.Fatalf("found: %v", err)
	}
}

// openBootStoreForTest opens the store through the boot's door for the
// profile at cfgPath, so a migration runs the way the server boot runs it.
func openBootStoreForTest(cfgPath string) (*actionsqlite.Store, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, err
	}
	return actionsqlite.OpenFor(app.StoragePath(cfg), app.ProfileIdentity(cfgPath))
}

// R22 · the readers name every standing from the row.
//
// PROBING MUTATION: print nothing about the standing → every leg reddens.
func TestLedgerCheck_namesEveryStandingFromTheRow(t *testing.T) {
	cfgPath, dbPath := seedChain(t, 1)
	raw := rawLedger(t, dbPath)
	expect := func(t *testing.T, want string) {
		t.Helper()
		var receiptID string
		if err := raw.QueryRow(`SELECT receipt_id FROM receipts ORDER BY chain_seq LIMIT 1`).Scan(&receiptID); err != nil {
			t.Fatalf("first receipt: %v", err)
		}
		for _, args := range [][]string{{"ledger", "check", "--config", cfgPath}, {"receipt", "verify", "--config", cfgPath, receiptID}} {
			_, stdout, _ := runIntentCLI(t, args...)
			if !strings.Contains(stdout, "ledger standing: "+want) {
				t.Fatalf("%v: %q, want %q", args, stdout, want)
			}
		}
	}
	expect(t, "legacy_unfounded")
	foundLedgerFor(t, cfgPath, dbPath, app.ProfileIdentity(cfgPath))
	expect(t, "ok")
	other := action.HashCanonical(`{"profile":"/srv/elsewhere/korvun.json"}`)
	if _, err := raw.Exec(`UPDATE ledger_identity SET owner_digest = ?`, other); err != nil {
		t.Fatal(err)
	}
	expect(t, "ledger_foreign_profile")
	if _, err := raw.Exec(`DELETE FROM ledger_identity`); err != nil {
		t.Fatal(err)
	}
	expect(t, "ledger_unreadable")
}

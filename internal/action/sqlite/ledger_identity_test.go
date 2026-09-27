// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The redesigned mark, at the store (train B, 2026-09-24): the moulds of the
// plan's rows R01–R18 and R25 that live here. Plan:
// docs/superpowers/specs/2026-09-24-v0162-el-marcador-rediseñado-pretest.md.
//
// Evidence level, per mould: in-process store on a real file; «raw» is a
// SECOND real connection to the same file; «immediate» moulds synchronise on
// a raw BEGIN IMMEDIATE confirmed before the store's write is attempted.

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/identity"
)

// profileC is a third canonical identity for the moulds that need a third profile.
var profileC = action.HashCanonical(`{"profile":"/srv/korvun/c/korvun.json"}`)

// rawConn opens a SECOND real connection to the store's file.
func rawConn(t *testing.T, store *Store) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+store.path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func rawExec(t *testing.T, db *sql.DB, q string, args ...any) sql.Result {
	t.Helper()
	res, err := db.Exec(q, args...)
	if err != nil {
		t.Fatalf("raw %q: %v", q, err)
	}
	return res
}

func rawCount(t *testing.T, db *sql.DB, q string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q).Scan(&n); err != nil {
		t.Fatalf("raw count %q: %v", q, err)
	}
	return n
}

// foundedFor founds a ledger for owner through the sealed fixture and returns
// the fixture's handle (identity owner) plus the evidence makers.
func foundedFor(t *testing.T, owner string) (*Store, func(id string) (action.Envelope, identity.Evidence)) {
	t.Helper()
	store, resolver, issuer, _, now := identityStoreFixture(t)
	mustRecord(t, store, "act_1", action.StateAuthorized)
	if err := store.FinishFounding(context.Background(), "act_1", owner); err != nil {
		t.Fatalf("found: %v", err)
	}
	// The handle the moulds get is a GUARDED one, born for the owner through
	// the operator's door, with the fixture's ink: the fixture's own handle
	// (no profile, no guard) is the package's door, not a profile's.
	guarded, err := OpenOperatorFor(store.path, owner)
	if err != nil {
		t.Fatalf("open the owner's guarded handle: %v", err)
	}
	t.Cleanup(func() { _ = guarded.Close() })
	wireSealedLike(t, guarded, store)
	return guarded, func(id string) (action.Envelope, identity.Evidence) {
		return identityAttempt(t, resolver, issuer, id, now)
	}
}

func adoptionEnv(env action.Envelope) action.Envelope {
	env.Operation = action.Operation{Namespace: "config", Name: AdoptionVerb, Version: 1}
	env.ParametersDigest = action.Digest(env.Operation, `{"door":"adopt-ledger"}`)
	return env
}

// R01 · a missing identity row while a marked receipt exists is UNREADABLE:
// named, blocking every act and the adoption, for every profile.
//
// PROBING MUTATION: read «no row» as legacy_unfounded → the fresh handle
// records and adopts; reddens by name.
func TestIdentity_aMissingRowWithAMarkedReceiptIsUnreadable(t *testing.T) {
	store, _ := foundedFor(t, profileA)
	raw := rawConn(t, store)
	if n, _ := rawExec(t, raw, `DELETE FROM ledger_identity`).RowsAffected(); n != 1 {
		t.Fatalf("deleted %d identity rows, want 1", n)
	}
	ctx := context.Background()
	for _, who := range []string{profileA, profileB} {
		fresh, err := OpenOperatorFor(store.path, who)
		if err != nil {
			t.Fatalf("open fresh for %s: %v", who, err)
		}
		defer func() { _ = fresh.Close() }()
		standing, _, err := fresh.Standing(ctx)
		if !errors.Is(err, ErrLedgerUnreadable) || standing != LedgerStandingUnreadable {
			t.Fatalf("standing for %s = %q %v, want unreadable/ErrLedgerUnreadable", who, standing, err)
		}
		if err := fresh.RecordAttempt(ctx, testEnvelope("act_"+who[7:12]), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized); !errors.Is(err, ErrLedgerUnreadable) {
			t.Fatalf("an act for %s = %v, want ErrLedgerUnreadable", who, err)
		}
	}
	if got := countRows(t, store, "actions"); got != 1 {
		t.Fatalf("%d actions, want the founding one", got)
	}
	if got := rawCount(t, raw, `SELECT COUNT(*) FROM ledger_identity`); got != 0 {
		t.Fatalf("%d identity rows appeared", got)
	}
}

// R02 · the adoption reads the identity INSIDE its own transaction, never from
// anything the handle saw before: the screen's sequence (look, rewrite, press)
// refuses when the row vanished, and follows the row when it changed.
//
// PROBING MUTATION: read the identity before BEGIN IMMEDIATE from a value the
// handle kept → the «vanished» leg adopts; reddens by name.
func TestAdopt_readsTheIdentityInsideItsOwnTransaction(t *testing.T) {
	t.Run("the row vanished after the look", func(t *testing.T) {
		store, evidenceFor := foundedFor(t, profileA)
		b, err := OpenOperatorFor(store.path, profileB)
		if err != nil {
			t.Fatalf("open B: %v", err)
		}
		defer func() { _ = b.Close() }()
		wireSealedLike(t, b, store)
		ctx := context.Background()
		if standing, owner, err := b.Standing(ctx); err != nil || standing != LedgerStandingForeignProfile || owner != profileA {
			t.Fatalf("the look: %q %q %v, want foreign owned by A", standing, owner, err)
		}
		raw := rawConn(t, store)
		rawExec(t, raw, `DELETE FROM ledger_identity`)
		env, evidence := evidenceFor("act_adopt")
		if _, err := b.AdoptLedger(ctx, adoptionEnv(env), Decision{Outcome: "allow", Rule: "operator"}, evidence, profileB); !errors.Is(err, ErrLedgerUnreadable) {
			t.Fatalf("adopting after the row vanished = %v, want ErrLedgerUnreadable", err)
		}
		if got := rawCount(t, raw, `SELECT COUNT(*) FROM receipts`); got != 1 {
			t.Fatalf("%d receipts, want the founding one", got)
		}
	})
	t.Run("the row changed hands after the look", func(t *testing.T) {
		store, evidenceFor := foundedFor(t, profileA)
		b, err := OpenOperatorFor(store.path, profileB)
		if err != nil {
			t.Fatalf("open B: %v", err)
		}
		defer func() { _ = b.Close() }()
		wireSealedLike(t, b, store)
		ctx := context.Background()
		if _, owner, _ := b.Standing(ctx); owner != profileA {
			t.Fatalf("the look: owner %q, want A", owner)
		}
		raw := rawConn(t, store)
		rawExec(t, raw, `UPDATE ledger_identity SET owner_digest = ?`, profileC)
		env, evidence := evidenceFor("act_adopt")
		if _, err := b.AdoptLedger(ctx, adoptionEnv(env), Decision{Outcome: "allow", Rule: "operator"}, evidence, profileB); err != nil {
			t.Fatalf("adopting a ledger that now says C = %v, want adopted (L6: the last adopter owns)", err)
		}
		var owner string
		if err := raw.QueryRow(`SELECT owner_digest FROM ledger_identity`).Scan(&owner); err != nil || owner != profileB {
			t.Fatalf("row after the adoption = %q %v, want B", owner, err)
		}
	})
}

// The third order, and the one that tells the in-transaction read from a read
// made before it: the row vanishes INSIDE another writer's transaction while
// the adoption waits behind it. Read inside its own transaction, the adoption
// sees the deletion once it enters and refuses; read before, it would have
// seen the founder and adopted over nothing.
func TestAdopt_seesARowThatVanishedWhileItWaited(t *testing.T) {
	store, evidenceFor := foundedFor(t, profileA)
	b, err := OpenOperatorFor(store.path, profileB)
	if err != nil {
		t.Fatalf("open B: %v", err)
	}
	defer func() { _ = b.Close() }()
	wireSealedLike(t, b, store)
	ctx := context.Background()
	if _, owner, _ := b.Standing(ctx); owner != profileA {
		t.Fatalf("the look: owner %q, want A", owner)
	}
	raw := rawConn(t, store)
	tx, err := raw.Begin()
	if err != nil {
		t.Fatalf("raw begin: %v", err)
	}
	if _, err := tx.Exec(`DELETE FROM ledger_identity`); err != nil {
		t.Fatalf("raw delete: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		env, evidence := evidenceFor("act_adopt_late")
		_, err := b.AdoptLedger(ctx, adoptionEnv(env), Decision{Outcome: "allow", Rule: "operator"}, evidence, profileB)
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("B adopted while the other writer held the ledger: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("raw commit: %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrLedgerUnreadable) {
			t.Fatalf("B after the row vanished under it = %v, want ErrLedgerUnreadable", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("B never finished")
	}
	if got := rawCount(t, raw, `SELECT COUNT(*) FROM receipts`); got != 1 {
		t.Fatalf("%d receipts, want the founding one", got)
	}
	if got := rawCount(t, raw, `SELECT COUNT(*) FROM ledger_identity`); got != 0 {
		t.Fatalf("%d identity rows: the adoption wrote over a vanished row", got)
	}
}

// wireSealedLike gives a second handle the same sealer and signers as the
// fixture's, so its adoption can seal.
func wireSealedLike(t *testing.T, dst, src *Store) {
	t.Helper()
	dst.sealer = src.sealer
	dst.identityEvidenceSigner, dst.principalEventSigner = src.identityEvidenceSigner, src.principalEventSigner
	dst.intentContractSigner, dst.intentEventSigner = src.intentContractSigner, src.intentEventSigner
	dst.authoritySigner = src.authoritySigner
	dst.identityNow = src.identityNow
}

// R03 · the identity row refuses every non-canonical form at SQLite.
//
// PROBING MUTATION: drop the CHECK from the schema → the forms land.
func TestIdentity_theRowRefusesEveryNonCanonicalForm(t *testing.T) {
	store, _ := foundedFor(t, profileA)
	raw := rawConn(t, store)
	for _, form := range []string{"PROFILE:" + profileA, "", "profile:", "profile: ", "profile:xyz", "profile:" + strings.ToUpper(profileA), "sha256:zz", profileA + "x"} {
		if _, err := raw.Exec(`UPDATE ledger_identity SET owner_digest = ?`, form); err == nil {
			t.Fatalf("the row accepted %q", form)
		}
	}
	if _, err := raw.Exec(`UPDATE ledger_identity SET owner_digest = NULL`); err == nil {
		t.Fatal("the row accepted NULL")
	}
	if standing, owner, err := store.Standing(context.Background()); err != nil || standing != LedgerStandingOK || owner != profileA {
		t.Fatalf("after the refused rewrites: %q %q %v, want ok owned by A", standing, owner, err)
	}
}

// R04 · the row is the state, the receipt is the evidence: a hand-edited row
// with a valid digest is followed by the readers (declared limit) and the
// chain's verdict does not change.
//
// PROBING MUTATION: derive the standing from the receipt's mark → the A handle
// reads ok; reddens.
func TestStanding_followsTheRowNotTheReceipt(t *testing.T) {
	store, _ := foundedFor(t, profileA)
	raw := rawConn(t, store)
	rawExec(t, raw, `UPDATE ledger_identity SET owner_digest = ?`, profileC)
	ctx := context.Background()
	if standing, owner, err := store.Standing(ctx); err != nil || standing != LedgerStandingForeignProfile || owner != profileC {
		t.Fatalf("A after the hand edit: %q %q %v, want foreign owned by C", standing, owner, err)
	}
	if err := store.RecordAttempt(ctx, testEnvelope("act_a"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized); !errors.Is(err, ErrLedgerForeignProfile) {
		t.Fatalf("A's act = %v, want ErrLedgerForeignProfile", err)
	}
	c, err := OpenOperatorFor(store.path, profileC)
	if err != nil {
		t.Fatalf("open C: %v", err)
	}
	defer func() { _ = c.Close() }()
	if standing, _, err := c.Standing(ctx); err != nil || standing != LedgerStandingOK {
		t.Fatalf("C: %q %v, want ok", standing, err)
	}
}

// R05 · two adoptions serialise under BEGIN IMMEDIATE, and a writer that
// waits the busy timeout out is named, not swallowed.
//
// PROBING MUTATION: begin the adoption deferred → the second adopter reads the
// old row while the first is inside; reddens on the final row or the order.
func TestAdopt_twoAdoptionsSerialize(t *testing.T) {
	store, evidenceFor := foundedFor(t, profileA)
	b, err := OpenOperatorFor(store.path, profileB)
	if err != nil {
		t.Fatalf("open B: %v", err)
	}
	defer func() { _ = b.Close() }()
	wireSealedLike(t, b, store)
	ctx := context.Background()
	raw := rawConn(t, store)
	// The raw connection plays the OTHER adopter: it holds an immediate
	// transaction with the row rewritten to C, uncommitted.
	tx, err := raw.Begin()
	if err != nil {
		t.Fatalf("raw begin: %v", err)
	}
	if _, err := tx.Exec(`UPDATE ledger_identity SET owner_digest = ?`, profileC); err != nil {
		t.Fatalf("raw update: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		env, evidence := evidenceFor("act_adopt_b")
		_, err := b.AdoptLedger(ctx, adoptionEnv(env), Decision{Outcome: "allow", Rule: "operator"}, evidence, profileB)
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("B adopted while the other writer held the ledger: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("raw commit: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("B after the other writer committed = %v, want adopted over C", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("B never finished")
	}
	var owner string
	if err := raw.QueryRow(`SELECT owner_digest FROM ledger_identity`).Scan(&owner); err != nil || owner != profileB {
		t.Fatalf("final row = %q %v, want B (the last adopter)", owner, err)
	}
}

func TestWriteTx_busyIsNamed(t *testing.T) {
	store, _ := foundedFor(t, profileA)
	raw := rawConn(t, store)
	tx, err := raw.Begin()
	if err != nil {
		t.Fatalf("raw begin: %v", err)
	}
	if _, err := tx.Exec(`UPDATE ledger_identity SET written_at = written_at`); err != nil {
		t.Fatalf("raw touch: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	start := time.Now()
	err = store.RecordAttempt(context.Background(), testEnvelope("act_busy"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized)
	if !errors.Is(err, ErrLedgerBusy) {
		t.Fatalf("a write behind a held ledger = %v after %s, want ErrLedgerBusy", err, time.Since(start))
	}
	if got := countRows(t, store, "actions"); got != 1 {
		t.Fatalf("%d actions after busy, want 1", got)
	}
}

// R06 · migration: a ledger of the previous schema with the mark in a receipt
// and no identity table gets its row from the last canonical mark, or is
// named malformed when the mark is not canonical.
//
// PROBING MUTATION: turn a non-canonical mark into a row → the junk leg reddens.
func TestMigrate_identityRowFromTheReceiptMark(t *testing.T) {
	downgrade := func(t *testing.T, store *Store) {
		t.Helper()
		raw := rawConn(t, store)
		rawExec(t, raw, `DROP TABLE ledger_identity`)
		rawExec(t, raw, `UPDATE action_schema SET version = 15`)
	}
	t.Run("a canonical mark becomes the row", func(t *testing.T) {
		store, _ := foundedFor(t, profileA)
		downgrade(t, store)
		_ = store.Close()
		again, err := OpenFor(store.path, profileA)
		if err != nil {
			t.Fatalf("reopen: %v", err)
		}
		defer func() { _ = again.Close() }()
		if standing, owner, err := again.Standing(context.Background()); err != nil || standing != LedgerStandingOK || owner != profileA {
			t.Fatalf("after the migration: %q %q %v, want ok owned by A", standing, owner, err)
		}
	})
	t.Run("a junk mark becomes no row and is named", func(t *testing.T) {
		store, _ := foundedFor(t, profileA)
		raw := rawConn(t, store)
		rawExec(t, raw, `UPDATE receipts SET result_digest = 'profile:xyz' WHERE result_digest LIKE 'profile:%'`)
		downgrade(t, store)
		_ = store.Close()
		again, err := OpenFor(store.path, profileA)
		if err != nil {
			t.Fatalf("reopen: %v", err)
		}
		defer func() { _ = again.Close() }()
		ctx := context.Background()
		if _, _, err := again.Standing(ctx); !errors.Is(err, ErrLedgerMarkMalformed) {
			t.Fatalf("after the migration of a junk mark: %v, want ErrLedgerMarkMalformed", err)
		}
		if err := again.RecordAttempt(ctx, testEnvelope("act_x"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized); !errors.Is(err, ErrLedgerMarkMalformed) {
			t.Fatalf("an act = %v, want ErrLedgerMarkMalformed", err)
		}
		if got := rawCount(t, raw, `SELECT COUNT(*) FROM ledger_identity`); got != 0 {
			t.Fatalf("%d rows from a junk mark", got)
		}
	})
	t.Run("two marked receipts: the last one is the row", func(t *testing.T) {
		store, evidenceFor := foundedFor(t, profileA)
		b, err := OpenOperatorFor(store.path, profileB)
		if err != nil {
			t.Fatalf("open B: %v", err)
		}
		wireSealedLike(t, b, store)
		env, evidence := evidenceFor("act_adopt")
		if _, err := b.AdoptLedger(context.Background(), adoptionEnv(env), Decision{Outcome: "allow", Rule: "operator"}, evidence, profileB); err != nil {
			t.Fatalf("adopt: %v", err)
		}
		_ = b.Close()
		downgrade(t, store)
		_ = store.Close()
		again, err := OpenFor(store.path, profileB)
		if err != nil {
			t.Fatalf("reopen: %v", err)
		}
		defer func() { _ = again.Close() }()
		if standing, owner, err := again.Standing(context.Background()); err != nil || standing != LedgerStandingOK || owner != profileB {
			t.Fatalf("after the migration: %q %q %v, want ok owned by B", standing, owner, err)
		}
	})
}

// R07 · no row, no mark: legacy — named, blocking nothing, adoptable.
//
// PROBING MUTATION: read «no row» as foreign → reddens.
func TestStanding_aLedgerWithNoRowAndNoMarkIsLegacy(t *testing.T) {
	store, resolver, issuer, _, now := identityStoreFixture(t)
	if err := store.setIdentity(profileB); err != nil {
		t.Fatalf("identity: %v", err)
	}
	ctx := context.Background()
	if standing, _, err := store.Standing(ctx); err != nil || standing != LedgerStandingLegacyUnfounded {
		t.Fatalf("%q %v, want legacy_unfounded", standing, err)
	}
	if err := store.RecordAttempt(ctx, testEnvelope("act_l"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized); err != nil {
		t.Fatalf("an act on a legacy ledger = %v, want nil", err)
	}
	env, evidence := identityAttempt(t, resolver, issuer, "act_adopt", now)
	if _, err := store.AdoptLedger(ctx, adoptionEnv(env), Decision{Outcome: "allow", Rule: "operator"}, evidence, profileB); err != nil {
		t.Fatalf("adopting a legacy ledger = %v, want adopted", err)
	}
	if standing, owner, err := store.Standing(ctx); err != nil || standing != LedgerStandingOK || owner != profileB {
		t.Fatalf("after adopting: %q %q %v", standing, owner, err)
	}
}

// R08 · the guard lives in the connection, not in the file.
//
// PROBING MUTATION: CREATE TRIGGER without TEMP → the raw connection sees
// guard triggers in sqlite_master and its INSERT dies; reddens.
func TestGuard_livesInTheConnectionNotInTheFile(t *testing.T) {
	store, _ := foundedFor(t, profileA)
	b, err := OpenOperatorFor(store.path, profileB)
	if err != nil {
		t.Fatalf("open B: %v", err)
	}
	defer func() { _ = b.Close() }()
	var inB int
	if err := b.db.QueryRow(`SELECT COUNT(*) FROM sqlite_temp_master WHERE type = 'trigger' AND name LIKE 'korvun_guard_%'`).Scan(&inB); err != nil || inB != 36 {
		t.Fatalf("B has %d guard triggers (%v), want 36 (twelve table events — 7 INSERT on the act tables, 2 UPDATE on intents/grants, 3 INSERT on the identity tables — times three standings that name themselves)", inB, err)
	}
	raw := rawConn(t, store)
	if n := rawCount(t, raw, `SELECT COUNT(*) FROM sqlite_temp_master WHERE type = 'trigger'`); n != 0 {
		t.Fatalf("the raw connection has %d temp triggers", n)
	}
	if n := rawCount(t, raw, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'trigger' AND name LIKE 'korvun_guard_%'`); n != 0 {
		t.Fatalf("the FILE has %d guard triggers", n)
	}
	rawExec(t, raw, `INSERT INTO actions (action_id, schema_version, correlation_id, source_kind, source_protocol, source_channel, op_namespace, op_name, op_version, parameters_digest, effect_class, state, requested_at)
		VALUES ('act_raw', 1, 'c', 'operator', 'http', 'console', 'config', 'raw', 1, 'sha256:0', 'read', 'AUTHORIZED', '2026-09-24T00:00:00Z')`)
	if err := b.RecordAttempt(context.Background(), testEnvelope("act_b"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized); !errors.Is(err, ErrLedgerForeignProfile) {
		t.Fatalf("B's act = %v, want ErrLedgerForeignProfile", err)
	}
}

// R09 · a door written on purpose without any guard, in six spellings, dies
// in SQLite for a foreign profile and passes for the owner.
//
// PROBING MUTATION: drop the trigger on actions → the door writes; reddens.
func (s *Store) unguardedDoorForTest(ctx context.Context, sqlText, id string) error {
	_, err := s.db.ExecContext(ctx, sqlText, id)
	return s.mapGuardError(err)
}

func TestGuard_anUnguardedDoorDiesAnyway(t *testing.T) {
	store, _ := foundedFor(t, profileA)
	b, err := OpenOperatorFor(store.path, profileB)
	if err != nil {
		t.Fatalf("open B: %v", err)
	}
	defer func() { _ = b.Close() }()
	ctx := context.Background()
	const cols = `(action_id, schema_version, correlation_id, source_kind, source_protocol, source_channel, op_namespace, op_name, op_version, parameters_digest, effect_class, state, requested_at)`
	const vals = `VALUES (?, 1, 'c', 'operator', 'http', 'console', 'config', 'door', 1, 'sha256:0', 'read', 'AUTHORIZED', '2026-09-24T00:00:00Z')`
	spellings := []string{
		`INSERT INTO actions ` + cols + ` ` + vals,
		`insert into actions ` + cols + ` ` + vals,
		`REPLACE INTO 'actions' ` + cols + ` ` + vals,
		`INSERT /*x*/ INTO "main"."actions" ` + cols + ` ` + vals,
		"INSERT INTO\n\tactions " + cols + " " + vals,
		`INSERT OR ROLLBACK INTO [actions] ` + cols + ` ` + vals,
	}
	for i, q := range spellings {
		if err := b.unguardedDoorForTest(ctx, q, "act_s"+string(rune('a'+i))); !errors.Is(err, ErrLedgerForeignProfile) {
			t.Fatalf("spelling %d on B = %v, want ErrLedgerForeignProfile", i, err)
		}
	}
	if got := countRows(t, store, "actions"); got != 1 {
		t.Fatalf("%d actions after the six spellings, want 1", got)
	}
	if err := store.unguardedDoorForTest(ctx, spellings[0], "act_owner"); err != nil {
		t.Fatalf("the owner's unguarded door = %v, want nil", err)
	}
}

// R10 · maintenance refuses on a foreign ledger by name and runs for the owner.
//
// PROBING MUTATION: let RecoverPreviousLife/Prune skip the judgement → the
// foreign legs return nil; reddens.
func TestOpen_aForeignLedgerGetsNoMaintenance(t *testing.T) {
	store, _ := foundedFor(t, profileA)
	mustRecord(t, store, "act_open", action.StateAuthorized) // something a recovery would close
	b, err := OpenOperatorFor(store.path, profileB)
	if err != nil {
		t.Fatalf("open B: %v", err)
	}
	defer func() { _ = b.Close() }()
	ctx := context.Background()
	if _, err := b.RecoverPreviousLife(ctx); !errors.Is(err, ErrLedgerForeignProfile) {
		t.Fatalf("recovery on a foreign ledger = %v, want ErrLedgerForeignProfile", err)
	}
	if _, err := b.Prune(ctx); !errors.Is(err, ErrLedgerForeignProfile) {
		t.Fatalf("prune on a foreign ledger = %v, want ErrLedgerForeignProfile", err)
	}
	if got := countRows(t, store, "receipts"); got != 1 {
		t.Fatalf("%d receipts: a foreign handle closed the owner's act", got)
	}
	if _, err := store.RecoverPreviousLife(ctx); err != nil {
		t.Fatalf("the owner's recovery = %v, want nil", err)
	}
	if got := countRows(t, store, "receipts"); got != 2 {
		t.Fatalf("%d receipts after the owner's recovery, want 2", got)
	}
}

// R11 · the founding receipt and the identity row are one transaction.
//
// PROBING MUTATION: write the row after the transaction → the seam leaves a
// receipt without a row; reddens.
func TestFounding_receiptAndRowAreOneTransaction(t *testing.T) {
	store, _, _, _, _ := identityStoreFixture(t)
	mustRecord(t, store, "act_1", action.StateAuthorized)
	store.seams.beforeIdentityRow = func() error { return errors.New("crash before the identity row") }
	err := store.FinishFounding(context.Background(), "act_1", profileA)
	if err == nil || !strings.Contains(err.Error(), "crash before the identity row") {
		t.Fatalf("founding through the seam = %v, want the seam's error", err)
	}
	raw := rawConn(t, store)
	if n := rawCount(t, raw, `SELECT COUNT(*) FROM receipts`); n != 0 {
		t.Fatalf("%d receipts after the aborted founding, want 0", n)
	}
	if n := rawCount(t, raw, `SELECT COUNT(*) FROM ledger_identity`); n != 0 {
		t.Fatalf("%d identity rows after the aborted founding, want 0", n)
	}
	if err := store.setIdentity(profileA); err != nil {
		t.Fatalf("identity: %v", err)
	}
	if standing, _, err := store.Standing(context.Background()); err != nil || standing != LedgerStandingLegacyUnfounded {
		t.Fatalf("after the aborted founding: %q %v, want legacy", standing, err)
	}
}

// R12 · no exported opener without an identity; no identity setter.
//
// PROBING MUTATION: export OpenOperator again → reddens naming it.
func TestOpeners_everyExportedOpenerTakesAnIdentity(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	// The package's named types, so a Store hidden behind one of them is
	// seen (the round-2 find: a []*Store, a struct field, a method on another
	// receiver all passed a walk that looked for a bare *Store).
	types := map[string]ast.Expr{}
	for _, f := range files {
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gd.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok {
					types[ts.Name.Name] = ts.Type
				}
			}
		}
	}
	var offenders []string
	seen := map[string]bool{}
	for _, f := range files {
		name := fset.Position(f.Pos()).Filename
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || !fn.Name.IsExported() {
				continue
			}
			// Any exported function or method that hands out a Store — in a
			// result of any type, through a callback parameter, or through a
			// pointer parameter — is an opener, whatever its name.
			if handsOutStore(fn, types) {
				seen[fn.Name.Name] = true
				var params []string
				for _, field := range fn.Type.Params.List {
					for _, id := range field.Names {
						params = append(params, id.Name)
					}
				}
				if !strings.HasSuffix(fn.Name.Name, "For") || len(params) < 2 || params[1] != "identity" {
					offenders = append(offenders, name+": "+fn.Name.Name)
				}
			}
			if fn.Recv != nil && fn.Name.Name == "SetProfileIdentity" {
				offenders = append(offenders, name+": SetProfileIdentity (a handle serves the profile it was opened for)")
			}
		}
	}
	for _, want := range []string{"OpenFor", "OpenOperatorFor", "OpenReadOnlyFor"} {
		if !seen[want] {
			offenders = append(offenders, "missing "+want)
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("openers outside the contract:\n  %s", strings.Join(offenders, "\n  "))
	}
}

// handsOutStore reports whether fn hands a Store to its caller: in any
// result (bare, behind a pointer, inside a slice, array, map, channel,
// function result, struct field, generic instantiation, or behind a named
// type of this package that does), or through a parameter that delivers one
// (deliversStore: a callback, bare or a named function type, whose own
// parameters carry a Store; a channel that carries one; a pointer to
// something that carries one). types is the package's type declarations by
// name. Declared limits: a Store behind an interface, and a delivery into a
// caller-sized slice or map parameter, are not visible statically.
func handsOutStore(fn *ast.FuncDecl, types map[string]ast.Expr) bool {
	if fn.Type.Results != nil {
		for _, r := range fn.Type.Results.List {
			if carriesStore(r.Type, types, map[string]bool{}) {
				return true
			}
		}
	}
	for _, p := range fn.Type.Params.List {
		if deliversStore(p.Type, types) {
			return true
		}
	}
	return false
}

// deliversStore reports whether a PARAMETER of type e is a way to hand a
// Store to the caller: a callback (a function type, bare or named in this
// package) whose own parameters carry one, a channel that carries one, or a
// pointer to something that carries one (`**Store`, `*[]*Store`; a bare
// `*Store` is an input).
func deliversStore(e ast.Expr, types map[string]ast.Expr) bool {
	switch x := e.(type) {
	case *ast.FuncType:
		if x.Params == nil {
			return false
		}
		for _, cp := range x.Params.List {
			if carriesStore(cp.Type, types, map[string]bool{}) {
				return true
			}
		}
		return false
	case *ast.ChanType:
		return carriesStore(x.Value, types, map[string]bool{})
	case *ast.StarExpr:
		if id, ok := x.X.(*ast.Ident); ok && id.Name == "Store" {
			return false
		}
		return carriesStore(x.X, types, map[string]bool{})
	case *ast.ParenExpr:
		return deliversStore(x.X, types)
	case *ast.Ident:
		if decl, ok := types[x.Name]; ok {
			return deliversStore(decl, types)
		}
		return false
	default:
		return false
	}
}

func carriesStore(e ast.Expr, types map[string]ast.Expr, seen map[string]bool) bool {
	switch x := e.(type) {
	case *ast.Ident:
		if x.Name == "Store" {
			return true
		}
		if seen[x.Name] {
			return false
		}
		seen[x.Name] = true
		if decl, ok := types[x.Name]; ok {
			return carriesStore(decl, types, seen)
		}
		return false
	case *ast.StarExpr:
		return carriesStore(x.X, types, seen)
	case *ast.ParenExpr:
		return carriesStore(x.X, types, seen)
	case *ast.ArrayType:
		return carriesStore(x.Elt, types, seen)
	case *ast.Ellipsis:
		return carriesStore(x.Elt, types, seen)
	case *ast.MapType:
		return carriesStore(x.Key, types, seen) || carriesStore(x.Value, types, seen)
	case *ast.ChanType:
		return carriesStore(x.Value, types, seen)
	case *ast.IndexExpr:
		return carriesStore(x.X, types, seen) || carriesStore(x.Index, types, seen)
	case *ast.IndexListExpr:
		if carriesStore(x.X, types, seen) {
			return true
		}
		for _, i := range x.Indices {
			if carriesStore(i, types, seen) {
				return true
			}
		}
		return false
	case *ast.StructType:
		for _, f := range x.Fields.List {
			if carriesStore(f.Type, types, seen) {
				return true
			}
		}
		return false
	case *ast.FuncType:
		if x.Results == nil {
			return false
		}
		for _, r := range x.Results.List {
			if carriesStore(r.Type, types, seen) {
				return true
			}
		}
		return false
	default:
		return false // a selector (another package's type) or an interface
	}
}

// R14 · the guard survives a pool reconnect — triggers AND the judged
// standing: the new connection is judged by the hook on itself, so a door
// that never re-judges (the test's unguarded door) dies on it too, and the
// production doors that once wrote outside beginWrite (CreateIntent,
// CreateGrant) refuse by name.
//
// PROBING MUTATIONS: install the guard only at open, not on every connection
// → after the reconnect the act lands; make the hook write «ok» instead of
// judging → the unguarded door lands; reddens.
func TestGuard_survivesAPoolReconnect(t *testing.T) {
	store, _ := foundedFor(t, profileA)
	b, err := OpenOperatorFor(store.path, profileB)
	if err != nil {
		t.Fatalf("open B: %v", err)
	}
	defer func() { _ = b.Close() }()
	b.db.SetConnMaxLifetime(time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	var n int
	if err := b.db.QueryRow(`SELECT COUNT(*) FROM sqlite_temp_master WHERE type = 'trigger' AND name LIKE 'korvun_guard_%'`).Scan(&n); err != nil || n != 36 {
		t.Fatalf("after the reconnect B has %d guard triggers (%v), want 36", n, err)
	}
	var row string
	if err := b.db.QueryRow(`SELECT standing FROM temp.profile_guard`).Scan(&row); err != nil || row != string(LedgerStandingForeignProfile) {
		t.Fatalf("after the reconnect the guard row says %q (%v), want the judged foreign", row, err)
	}
	ctx := context.Background()
	const q = `INSERT INTO actions (action_id, schema_version, correlation_id, source_kind, source_protocol, source_channel, op_namespace, op_name, op_version, parameters_digest, effect_class, state, requested_at)
		VALUES (?, 1, 'c', 'operator', 'http', 'console', 'config', 'door', 1, 'sha256:0', 'read', 'AUTHORIZED', '2026-09-24T00:00:00Z')`
	if err := b.unguardedDoorForTest(ctx, q, "act_after_reconnect"); !errors.Is(err, ErrLedgerForeignProfile) {
		t.Fatalf("the unguarded door after the reconnect = %v, want ErrLedgerForeignProfile (the hook judged the new connection)", err)
	}
	if err := b.CreateIntent(ctx, action.IntentContract{IntentID: "int_r", Purpose: "r", AllowedOperations: []string{"calc"}}); !errors.Is(err, ErrLedgerForeignProfile) {
		t.Fatalf("CreateIntent after the reconnect = %v, want ErrLedgerForeignProfile", err)
	}
	if err := b.CreateGrant(ctx, action.AuthorityGrant{GrantID: "grant_r", IntentID: "int_r"}); !errors.Is(err, ErrLedgerForeignProfile) {
		t.Fatalf("CreateGrant after the reconnect = %v, want ErrLedgerForeignProfile", err)
	}
	if err := b.RecordAttempt(ctx, testEnvelope("act_b"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized); !errors.Is(err, ErrLedgerForeignProfile) {
		t.Fatalf("B's act after the reconnect = %v, want ErrLedgerForeignProfile", err)
	}
	if got := countRows(t, store, "actions") + countRows(t, store, "intents") + countRows(t, store, "grants"); got != 1 {
		t.Fatalf("%d rows across actions/intents/grants after the reconnect, want the founding act only", got)
	}
}

// The two exits of an adoption that must leave the connection's guard saying
// the standing it judged (the internal pass over the redesign, and the
// official pass that retired the Go mirror): a refused adoption on an
// unreadable ledger, and an aborted adoption on a foreign one. After each, a
// door outside beginWrite dies by the standing's own name — never «unset»,
// never open.
//
// PROBING MUTATIONS: let the refusal leave the guard row untouched (M255,
// reddens the refused leg); lift the guard through the pool before the
// transaction, so the abort cannot revert it (M253-bis, reddens the aborted
// leg).
func TestAdopt_aRefusedOrAbortedAdoptionLeavesTheGuardConsistent(t *testing.T) {
	ctx := context.Background()
	const q = `INSERT INTO actions (action_id, schema_version, correlation_id, source_kind, source_protocol, source_channel, op_namespace, op_name, op_version, parameters_digest, effect_class, state, requested_at)
		VALUES (?, 1, 'c', 'operator', 'http', 'console', 'config', 'door', 1, 'sha256:0', 'read', 'AUTHORIZED', '2026-09-24T00:00:00Z')`
	guardRow := func(t *testing.T, s *Store) string {
		t.Helper()
		var row string
		if err := s.db.QueryRow(`SELECT standing FROM temp.profile_guard`).Scan(&row); err != nil {
			t.Fatalf("guard row: %v", err)
		}
		return row
	}
	t.Run("refused on an unreadable ledger, by the owner", func(t *testing.T) {
		store, evidenceFor := foundedFor(t, profileA)
		raw := rawConn(t, store)
		rawExec(t, raw, `DELETE FROM ledger_identity`)
		env, evidence := evidenceFor("act_adopt")
		if _, err := store.AdoptLedger(ctx, adoptionEnv(env), Decision{Outcome: "allow", Rule: "operator"}, evidence, profileA); !errors.Is(err, ErrLedgerUnreadable) {
			t.Fatalf("adopting an unreadable ledger = %v, want ErrLedgerUnreadable", err)
		}
		if row := guardRow(t, store); row != string(LedgerStandingUnreadable) {
			t.Fatalf("after the refused adoption the guard row says %q, want unreadable", row)
		}
		if err := store.unguardedDoorForTest(ctx, q, "act_after_refusal"); !errors.Is(err, ErrLedgerUnreadable) {
			t.Fatalf("the unguarded door after the refused adoption = %v, want ErrLedgerUnreadable", err)
		}
		if err := store.CreateIntent(ctx, action.IntentContract{IntentID: "int_u", Purpose: "u", AllowedOperations: []string{"calc"}}); !errors.Is(err, ErrLedgerUnreadable) {
			t.Fatalf("CreateIntent after the refused adoption = %v, want ErrLedgerUnreadable", err)
		}
	})
	t.Run("aborted on a foreign ledger", func(t *testing.T) {
		store, evidenceFor := foundedFor(t, profileA)
		b, err := OpenOperatorFor(store.path, profileB)
		if err != nil {
			t.Fatalf("open B: %v", err)
		}
		defer func() { _ = b.Close() }()
		wireSealedLike(t, b, store)
		b.sealer = func(r action.Receipt) action.Receipt { r.Signature = ""; return r } // the adoption aborts on its unsigned receipt
		env, evidence := evidenceFor("act_adopt")
		// The abort has ITS name (the receipt born unsigned), not any error:
		// an adoption killed by the guard instead would be a different story.
		if _, err := b.AdoptLedger(ctx, adoptionEnv(env), Decision{Outcome: "allow", Rule: "operator"}, evidence, profileB); err == nil || !strings.Contains(err.Error(), "receipt_unsigned") {
			t.Fatalf("an adoption whose receipt was born unsigned = %v, want the receipt_unsigned refusal", err)
		}
		if row := guardRow(t, b); row != string(LedgerStandingForeignProfile) {
			t.Fatalf("after the aborted adoption the guard row says %q, want foreign", row)
		}
		if err := b.unguardedDoorForTest(ctx, q, "act_after_abort"); !errors.Is(err, ErrLedgerForeignProfile) {
			t.Fatalf("the unguarded door after the aborted adoption = %v, want ErrLedgerForeignProfile (named by the standing, not unset)", err)
		}
		if got := countRows(t, store, "actions"); got != 1 {
			t.Fatalf("%d actions, want the founding one", got)
		}
	})
}

// R15 · an empty guard table fails CLOSED.
//
// PROBING MUTATION: drop the COALESCE from the WHEN → the act lands; reddens.
func TestGuard_anEmptyGuardTableFailsClosed(t *testing.T) {
	store, _ := foundedFor(t, profileA)
	if _, err := store.db.Exec(`DELETE FROM temp.profile_guard`); err != nil {
		t.Fatalf("empty the guard: %v", err)
	}
	if err := store.RecordAttempt(context.Background(), testEnvelope("act_a"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized); !errors.Is(err, ErrLedgerGuardUnset) {
		t.Fatalf("an act with the guard row missing = %v, want ErrLedgerGuardUnset", err)
	}
	if got := countRows(t, store, "actions"); got != 1 {
		t.Fatalf("%d actions, want 1", got)
	}
}

// R16 · every act door reads the identity inside its IMMEDIATE transaction:
// a rewrite committed by another writer while the door waits is seen.
//
// PROBING MUTATION: begin deferred → the door reads the old row before the
// other writer commits and lands under a stale judgement; reddens.
func TestWriteTx_theIdentityIsReadInsideTheImmediateTransaction(t *testing.T) {
	store, _ := foundedFor(t, profileA)
	raw := rawConn(t, store)
	tx, err := raw.Begin()
	if err != nil {
		t.Fatalf("raw begin: %v", err)
	}
	if _, err := tx.Exec(`UPDATE ledger_identity SET owner_digest = ?`, profileC); err != nil {
		t.Fatalf("raw update: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- store.RecordAttempt(context.Background(), testEnvelope("act_a"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized)
	}()
	select {
	case err := <-done:
		t.Fatalf("A's act finished while the other writer held the ledger: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("raw commit: %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrLedgerForeignProfile) {
			t.Fatalf("A's act after the other writer committed = %v, want ErrLedgerForeignProfile (the row now says C)", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("A's act never finished")
	}
	if got := countRows(t, store, "actions"); got != 1 {
		t.Fatalf("%d actions, want 1", got)
	}
}

// R17 · a door OUTSIDE writeTx sees the last judged standing (declared limit).
func TestGuard_aDoorOutsideWriteTxSeesTheLastJudgedStanding(t *testing.T) {
	store, _ := foundedFor(t, profileA)
	ctx := context.Background()
	if err := store.RecordAttempt(ctx, testEnvelope("act_a1"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized); err != nil {
		t.Fatalf("A's act: %v", err)
	}
	raw := rawConn(t, store)
	rawExec(t, raw, `UPDATE ledger_identity SET owner_digest = ?`, profileC)
	const q = `INSERT INTO actions (action_id, schema_version, correlation_id, source_kind, source_protocol, source_channel, op_namespace, op_name, op_version, parameters_digest, effect_class, state, requested_at)
		VALUES (?, 1, 'c', 'operator', 'http', 'console', 'config', 'door', 1, 'sha256:0', 'read', 'AUTHORIZED', '2026-09-24T00:00:00Z')`
	if err := store.unguardedDoorForTest(ctx, q, "act_outside"); err != nil {
		t.Fatalf("the door outside writeTx = %v: the LIMIT is that it lands under the last judged standing (ok)", err)
	}
	if err := store.RecordAttempt(ctx, testEnvelope("act_a2"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized); !errors.Is(err, ErrLedgerForeignProfile) {
		t.Fatalf("A's next act through writeTx = %v, want ErrLedgerForeignProfile", err)
	}
	if err := store.unguardedDoorForTest(ctx, q, "act_outside_2"); !errors.Is(err, ErrLedgerForeignProfile) {
		t.Fatalf("the door outside writeTx after the refresh = %v, want ErrLedgerForeignProfile", err)
	}
}

// R18 · the trigger's message is constant; the store names the standing.
//
// PROBING MUTATION: map every 1811 to foreign → the unreadable leg and the
// CHECK leg redden.
func TestGuard_theErrorIsNamedByTheStanding(t *testing.T) {
	ctx := context.Background()
	const q = `INSERT INTO actions (action_id, schema_version, correlation_id, source_kind, source_protocol, source_channel, op_namespace, op_name, op_version, parameters_digest, effect_class, state, requested_at)
		VALUES (?, 1, 'c', 'operator', 'http', 'console', 'config', 'door', 1, 'sha256:0', 'read', 'AUTHORIZED', '2026-09-24T00:00:00Z')`
	// Every leg writes through the door that skips beginWrite, so the refusal
	// is the trigger's RAISE and the name is mapGuardError's (the round-2
	// find: through RecordAttempt the refusal was beginWrite's, never the
	// trigger's, and a broken mapper stayed green).
	t.Run("foreign", func(t *testing.T) {
		store, _ := foundedFor(t, profileA)
		raw := rawConn(t, store)
		rawExec(t, raw, `UPDATE ledger_identity SET owner_digest = ?`, profileC)
		if row := guardRowAfterReconnect(t, store); row != string(LedgerStandingForeignProfile) {
			t.Fatalf("guard row = %q, want foreign", row)
		}
		if err := store.unguardedDoorForTest(ctx, q, "act_named_foreign"); !errors.Is(err, ErrLedgerForeignProfile) {
			t.Fatalf("%v, want ErrLedgerForeignProfile", err)
		}
	})
	t.Run("unreadable", func(t *testing.T) {
		store, _ := foundedFor(t, profileA)
		raw := rawConn(t, store)
		rawExec(t, raw, `DELETE FROM ledger_identity`)
		if row := guardRowAfterReconnect(t, store); row != string(LedgerStandingUnreadable) {
			t.Fatalf("guard row = %q, want unreadable", row)
		}
		if err := store.unguardedDoorForTest(ctx, q, "act_named_unreadable"); !errors.Is(err, ErrLedgerUnreadable) {
			t.Fatalf("%v, want ErrLedgerUnreadable", err)
		}
	})
	t.Run("unset", func(t *testing.T) {
		store, _ := foundedFor(t, profileA)
		if _, err := store.db.Exec(`UPDATE temp.profile_guard SET standing = 'junk'`); err != nil {
			t.Fatal(err)
		}
		if err := store.unguardedDoorForTest(ctx, q, "act_named_unset"); !errors.Is(err, ErrLedgerGuardUnset) {
			t.Fatalf("%v, want ErrLedgerGuardUnset", err)
		}
	})
	t.Run("a CHECK of another table is not a guard error", func(t *testing.T) {
		store, _ := foundedFor(t, profileA)
		_, err := store.db.Exec(`UPDATE ledger_identity SET owner_digest = 'junk'`)
		mapped := store.mapGuardError(err)
		if mapped == nil || errors.Is(mapped, ErrLedgerForeignProfile) || errors.Is(mapped, ErrLedgerUnreadable) || errors.Is(mapped, ErrLedgerGuardUnset) {
			t.Fatalf("a CHECK error was mapped to a guard standing: %v", mapped)
		}
	})
}

// Every production door judges the identity row inside its own transaction
// (beginWrite) — the first line — and does not lean on the connection's guard
// alone: with the guard row forced to «ok» on a FOREIGN ledger (a stale
// guard), the doors still refuse by name, and the guard row is re-judged by
// the first of them.
//
// PROBING MUTATION: let CreateIntent write through the pool without
// beginWrite → it lands on the stale guard; reddens.
func TestGuard_everyDoorJudgesEvenWithAStaleGuard(t *testing.T) {
	store, _ := foundedFor(t, profileA)
	b, err := OpenOperatorFor(store.path, profileB)
	if err != nil {
		t.Fatalf("open B: %v", err)
	}
	defer func() { _ = b.Close() }()
	ctx := context.Background()
	for _, door := range []struct {
		name string
		call func() error
	}{
		{"CreateIntent", func() error {
			return b.CreateIntent(ctx, action.IntentContract{IntentID: "int_s", Purpose: "s", AllowedOperations: []string{"calc"}})
		}},
		{"CreateGrant", func() error { return b.CreateGrant(ctx, action.AuthorityGrant{GrantID: "grant_s", IntentID: "int_s"}) }},
		{"RecordAttempt", func() error {
			return b.RecordAttempt(ctx, testEnvelope("act_s"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized)
		}},
	} {
		t.Run(door.name, func(t *testing.T) {
			if _, err := b.db.Exec(`UPDATE temp.profile_guard SET standing = 'ok'`); err != nil {
				t.Fatalf("stale the guard: %v", err)
			}
			if err := door.call(); !errors.Is(err, ErrLedgerForeignProfile) {
				t.Fatalf("%s with a stale guard = %v, want ErrLedgerForeignProfile (the door judges the row itself)", door.name, err)
			}
			var row string
			if err := b.db.QueryRow(`SELECT standing FROM temp.profile_guard`).Scan(&row); err != nil || row != string(LedgerStandingForeignProfile) {
				t.Fatalf("after %s the guard row says %q (%v), want re-judged foreign", door.name, row, err)
			}
		})
	}
	if got := countRows(t, store, "actions") + countRows(t, store, "intents") + countRows(t, store, "grants"); got != 1 {
		t.Fatalf("%d rows across actions/intents/grants, want the founding act only", got)
	}
}

// A dropped identity table on a schema that HAS the row is corruption, never
// absence — for the reader (judgeIn) AND for the hook that judges a new
// connection (the official pass's find: the hook read «no such table» as a
// fresh file and opened the guard on every pool reconnect).
//
// PROBING MUTATION: let the hook answer «ok» on a missing table regardless of
// the schema version → the door after the reconnect lands; reddens.
func TestGuard_aDroppedIdentityTableIsUnreadableForTheHook(t *testing.T) {
	store, _ := foundedFor(t, profileA)
	raw := rawConn(t, store)
	rawExec(t, raw, `DROP TABLE ledger_identity`)
	b, err := OpenOperatorFor(store.path, profileB)
	if err != nil {
		t.Fatalf("open B: %v", err)
	}
	defer func() { _ = b.Close() }()
	ctx := context.Background()
	if err := b.RecordAttempt(ctx, testEnvelope("act_b"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized); !errors.Is(err, ErrLedgerUnreadable) {
		t.Fatalf("B's act with the table dropped = %v, want ErrLedgerUnreadable", err)
	}
	b.db.SetConnMaxLifetime(time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	var row string
	if err := b.db.QueryRow(`SELECT standing FROM temp.profile_guard`).Scan(&row); err != nil || row != string(LedgerStandingUnreadable) {
		t.Fatalf("after the reconnect the guard row says %q (%v), want unreadable (a schema that has the row and no table is corruption)", row, err)
	}
	const q = `INSERT INTO actions (action_id, schema_version, correlation_id, source_kind, source_protocol, source_channel, op_namespace, op_name, op_version, parameters_digest, effect_class, state, requested_at)
		VALUES (?, 1, 'c', 'operator', 'http', 'console', 'config', 'door', 1, 'sha256:0', 'read', 'AUTHORIZED', '2026-09-24T00:00:00Z')`
	if err := b.unguardedDoorForTest(ctx, q, "act_after_drop"); !errors.Is(err, ErrLedgerUnreadable) {
		t.Fatalf("the unguarded door after the reconnect = %v, want ErrLedgerUnreadable", err)
	}
	if got := countRows(t, store, "actions"); got != 1 {
		t.Fatalf("%d actions, want the founding one", got)
	}
}

// A read never blocks: the approval detail of a request the founder parked
// is readable by a foreign profile, and by the owner over an unreadable
// identity (the official pass's find: a mechanical rewrite had routed the
// detail through the judged transaction).
//
// PROBING MUTATION: route approvalDetail through beginWrite → reddens.
func TestApprovalDetail_isAReadAndNeverBlocks(t *testing.T) {
	store, evidenceFor := foundedFor(t, profileA)
	env, _ := evidenceFor("act_parked")
	env.Effect = action.Effect{Class: string(action.EffectWriteIrreversible)}
	env.ParametersDigest = action.Digest(env.Operation, `{"a":1}`)
	bound, err := action.NewBoundApprovalRequest(env, `{"a":1}`, action.ApprovalContext{
		IntentPurpose: "x", ToolCage: "x",
		Descriptor: action.EffectDescriptor{Class: action.EffectWriteIrreversible}, HasDescriptor: true,
		LawVersion: 1, LawDigest: "sha256:law", Rule: "require_approval", Now: time.Now().UTC(), TTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("bound request: %v", err)
	}
	ctx := context.Background()
	if err := store.CreateApprovalRequest(ctx, bound); err != nil {
		t.Fatalf("park: %v", err)
	}
	b, err := OpenOperatorFor(store.path, profileB)
	if err != nil {
		t.Fatalf("open B: %v", err)
	}
	defer func() { _ = b.Close() }()
	if _, err := b.ApprovalDetail(ctx, bound.Approval().ApprovalID); err != nil {
		t.Fatalf("a foreign profile reading the detail = %v, want the detail (reads never block)", err)
	}
	raw := rawConn(t, store)
	rawExec(t, raw, `DELETE FROM ledger_identity`)
	if _, err := store.ApprovalDetail(ctx, bound.Approval().ApprovalID); err != nil {
		t.Fatalf("the owner reading the detail over an unreadable identity = %v, want the detail", err)
	}
}

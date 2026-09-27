// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The connection hook's polarity (train C, 2026-09-25): the moulds of the
// plan's rows C01–C06, C09, C10, C13, C14 and the internal pass's C15–C16
// (§13 of the redesign paper; C17 was retired by train D: on the shared file
// a wiped action schema is a fresh store, §14.4). The
// criterion of C01–C04 is the round-2 auditor's reproduction, step by step:
// corrupt the file from a second connection, expire the pool's connection,
// read the guard the hook wrote on the replacement, push a door that skips
// beginWrite, count the rows.
//
// Evidence level: in-process store on a real file; «raw» is a SECOND real
// connection to the same file; the reconnect is the pool's own
// (ConnMaxLifetime); C05/C06 judge the hook's function on a driver-level
// connection to a real file; C09 is two real handles in one process.

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	msqlite "modernc.org/sqlite"
)

// hookDoorInsert is the INSERT a door that skips beginWrite would run.
const hookDoorInsert = `INSERT INTO actions (action_id, schema_version, correlation_id, source_kind, source_protocol, source_channel, op_namespace, op_name, op_version, parameters_digest, effect_class, state, requested_at)
	VALUES (?, 1, 'c', 'operator', 'http', 'console', 'config', 'door', 1, 'sha256:0', 'read', 'AUTHORIZED', '2026-09-25T00:00:00Z')`

// guardRowAfterReconnect expires the pool's connection and reads the guard
// row the hook wrote on its replacement.
func guardRowAfterReconnect(t *testing.T, s *Store) string {
	t.Helper()
	s.db.SetConnMaxLifetime(time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	var row string
	if err := s.db.QueryRow(`SELECT standing FROM temp.profile_guard`).Scan(&row); err != nil {
		t.Fatalf("guard row after the reconnect: %v", err)
	}
	return row
}

// judgeOnRawConn runs the hook's judgement on a driver-level connection to
// path, the way the driver hands it to the hook.
func judgeOnRawConn(t *testing.T, path, identity string) string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("conn: %v", err)
	}
	defer func() { _ = conn.Close() }()
	var got string
	if err := conn.Raw(func(dc any) error {
		q, ok := dc.(msqlite.ExecQuerierContext)
		if !ok {
			t.Fatalf("driver connection %T does not execute", dc)
		}
		standing, err := judgeOnConn(ctx, q, identity)
		if err != nil {
			t.Fatalf("judgeOnConn: %v", err)
		}
		got = standing
		return nil
	}); err != nil {
		t.Fatalf("raw: %v", err)
	}
	return got
}

// C01–C04 · the round-2 auditor's four shapes, verbatim: each one is
// corruption of a v16 ledger, and the hook must open the replacement
// connection unreadable, never ok.
//
// PROBING MUTATIONS (one per shape): let the hook answer ok when receipts is
// missing (A1); when action_schema is missing (A2); when the version does not
// parse (A3); without counting the schema rows (A4). Each reddens its row.
func TestGuard_theHookFailsClosedOnEveryCorruptShape(t *testing.T) {
	ctx := context.Background()
	shapes := []struct {
		name    string
		corrupt []string
	}{
		{"A1 receipts dropped and no identity row", []string{`DELETE FROM ledger_identity`, `DROP TABLE receipts`}},
		{"A2 identity and schema tables dropped", []string{`DROP TABLE ledger_identity`, `DROP TABLE action_schema`}},
		{"A3 schema version not numeric", []string{`DROP TABLE ledger_identity`, `UPDATE action_schema SET version = 'sixteen'`}},
		{"A4 two schema rows", []string{`DROP TABLE ledger_identity`, `DELETE FROM action_schema`, `INSERT INTO action_schema (version) VALUES (1)`, `INSERT INTO action_schema (version) VALUES (16)`}},
	}
	for _, sh := range shapes {
		t.Run(sh.name, func(t *testing.T) {
			store, _ := foundedFor(t, profileA)
			raw := rawConn(t, store)
			for _, q := range sh.corrupt {
				rawExec(t, raw, q)
			}
			if standing, _, err := store.Standing(ctx); standing != LedgerStandingUnreadable || !errors.Is(err, ErrLedgerUnreadable) {
				t.Fatalf("Standing = %q %v, want unreadable, named", standing, err)
			}
			if row := guardRowAfterReconnect(t, store); row != string(LedgerStandingUnreadable) {
				t.Fatalf("guard row after the reconnect = %q, want %q (the hook read corruption as absence)", row, LedgerStandingUnreadable)
			}
			if err := store.unguardedDoorForTest(ctx, hookDoorInsert, "act_door"); !errors.Is(err, ErrLedgerUnreadable) {
				t.Fatalf("the unguarded door = %v, want ErrLedgerUnreadable", err)
			}
			if got := countRows(t, store, "actions"); got != 1 {
				t.Fatalf("actions rows = %d, want the founding act only", got)
			}
			if err := store.RecordAttempt(ctx, testEnvelope("act_x"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized); !errors.Is(err, ErrLedgerUnreadable) {
				t.Fatalf("RecordAttempt = %v, want ErrLedgerUnreadable", err)
			}
		})
	}
}

// C14 · a v16 ledger that lacks a guarded table opens unreadable even with
// the owner's canonical row: the hook could not install that table's
// triggers, so it does not open at all (the guard's own completeness).
//
// PROBING MUTATION: the hook stops requiring the guarded tables → reddens.
func TestGuard_theHookRequiresEveryGuardedTable(t *testing.T) {
	ctx := context.Background()
	store, _ := foundedFor(t, profileA)
	raw := rawConn(t, store)
	rawExec(t, raw, `DROP TABLE intents`)
	if row := guardRowAfterReconnect(t, store); row != string(LedgerStandingUnreadable) {
		t.Fatalf("guard row after the reconnect = %q, want %q (a guarded table is missing)", row, LedgerStandingUnreadable)
	}
	if err := store.unguardedDoorForTest(ctx, hookDoorInsert, "act_door"); !errors.Is(err, ErrLedgerUnreadable) {
		t.Fatalf("the unguarded door = %v, want ErrLedgerUnreadable", err)
	}
	if got := countRows(t, store, "actions"); got != 1 {
		t.Fatalf("actions rows = %d, want the founding act only", got)
	}
}

// C05–C06 · the two benign states open ok: a file with no table at all, and
// a ledger older than the identity row (one numeric schema row below 16, no
// identity table). The verdict is judged on a driver-level connection, the
// only place it is observable: the open sequence re-judges after the schema
// (installGuard), so a wrong verdict here would not break the open — declared
// in §13.4 of the paper.
//
// PROBING MUTATIONS: a file with zero tables answers unreadable (reddens
// «fresh»); a version below 16 answers unreadable (reddens «v15»).
func TestGuard_theHookOpensTheTwoBenignStates(t *testing.T) {
	ctx := context.Background()
	t.Run("fresh: a file with no table at all", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "fresh.db")
		if got := judgeOnRawConn(t, path, profileA); got != "ok" {
			t.Fatalf("a fresh file judged %q, want ok", got)
		}
		s, err := OpenFor(path, profileA)
		if err != nil {
			t.Fatalf("OpenFor on the fresh file: %v", err)
		}
		defer func() { _ = s.Close() }()
		if standing, _, err := s.Standing(ctx); err != nil || standing != LedgerStandingLegacyUnfounded {
			t.Fatalf("a fresh file opened as %q %v, want legacy_unfounded", standing, err)
		}
	})
	t.Run("v15: a ledger older than the row", func(t *testing.T) {
		store, _ := foundedFor(t, profileA)
		raw := rawConn(t, store)
		rawExec(t, raw, `DROP TABLE ledger_identity`)
		rawExec(t, raw, `UPDATE action_schema SET version = 15`)
		path := store.path
		_ = store.Close()
		if got := judgeOnRawConn(t, path, profileA); got != "ok" {
			t.Fatalf("a v15 ledger judged %q, want ok (the migration comes next)", got)
		}
		again, err := OpenFor(path, profileA)
		if err != nil {
			t.Fatalf("OpenFor on the v15 ledger: %v", err)
		}
		defer func() { _ = again.Close() }()
		if standing, owner, err := again.Standing(ctx); err != nil || standing != LedgerStandingOK || owner != profileA {
			t.Fatalf("after the migration: %q %q %v, want ok owned by A", standing, owner, err)
		}
	})
}

// C15 · the receipts table is required with the owner's row too (the
// internal pass of train C: G1-ter named it and the hook did not look), and
// the hook's unreadable verdict holds against a first line that finds the
// row ok: the act refuses by name and nothing lands.
//
// PROBING MUTATION: the hook stops requiring receipts on a ledger with a row
// → the guard row says ok → reddens.
func TestGuard_theHookRequiresTheReceiptsTableWithTheOwnersRow(t *testing.T) {
	ctx := context.Background()
	store, _ := foundedFor(t, profileA)
	raw := rawConn(t, store)
	rawExec(t, raw, `DROP TABLE receipts`)
	if row := guardRowAfterReconnect(t, store); row != string(LedgerStandingUnreadable) {
		t.Fatalf("guard row after the reconnect = %q, want %q (the receipts table is gone)", row, LedgerStandingUnreadable)
	}
	if err := store.unguardedDoorForTest(ctx, hookDoorInsert, "act_door"); !errors.Is(err, ErrLedgerUnreadable) {
		t.Fatalf("the unguarded door = %v, want ErrLedgerUnreadable", err)
	}
	if err := store.RecordAttempt(ctx, testEnvelope("act_x"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized); !errors.Is(err, ErrLedgerUnreadable) {
		t.Fatalf("RecordAttempt on a ledger without its receipts table = %v, want ErrLedgerUnreadable", err)
	}
	if got := countRows(t, store, "actions"); got != 1 {
		t.Fatalf("actions rows = %d, want the founding act only", got)
	}
}

// C16 · an unreadable verdict is sticky for the connection: with the guard
// row saying unreadable (written where the hook writes it) and the identity
// row intact, the first line says ok and the act still refuses by name; only
// a NEW connection, whose hook re-judges, lifts it. The row is set directly
// because a pool that expires connections every millisecond would re-judge
// the intact row on the next query and hide what is under test.
//
// PROBING MUTATION: beginWrite lifts the guard without asking it and the
// guard's UPDATE loses its WHERE → the act lands on the old connection →
// reddens.
func TestGuard_anUnreadableVerdictIsStickyForTheConnection(t *testing.T) {
	ctx := context.Background()
	store, _ := foundedFor(t, profileA)
	if _, err := store.db.ExecContext(ctx, `UPDATE temp.profile_guard SET standing = ?`, string(LedgerStandingUnreadable)); err != nil {
		t.Fatal(err)
	}
	if standing, _, err := store.Standing(ctx); err != nil || standing != LedgerStandingOK {
		t.Fatalf("the first line: %q %v, want ok (the row is intact)", standing, err)
	}
	if err := store.RecordAttempt(ctx, testEnvelope("act_sticky"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized); !errors.Is(err, ErrLedgerUnreadable) {
		t.Fatalf("an act on the connection whose guard says unreadable = %v, want ErrLedgerUnreadable (sticky)", err)
	}
	if got := countRows(t, store, "actions"); got != 1 {
		t.Fatalf("actions rows = %d after the refused act, want 1", got)
	}
	var row string
	if err := store.db.QueryRow(`SELECT standing FROM temp.profile_guard`).Scan(&row); err != nil || row != string(LedgerStandingUnreadable) {
		t.Fatalf("guard row after the refused act = %q (%v), want unreadable still", row, err)
	}
	if row := guardRowAfterReconnect(t, store); row != "ok" {
		t.Fatalf("guard row after a NEW connection = %q, want ok (the hook re-judged the intact row)", row)
	}
	if err := store.RecordAttempt(ctx, testEnvelope("act_after"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized); err != nil {
		t.Fatalf("an act on the new connection: %v", err)
	}
	if got := countRows(t, store, "actions"); got != 2 {
		t.Fatalf("actions rows = %d after the new connection, want 2", got)
	}
}

// C10 · a handle forgotten before its hook runs is named, never a panic (the
// registry is read ONCE). The race itself — Close between the hook's reads —
// is closed by construction and not reproduced: declared in §13.4.
//
// PROBING MUTATION: assert the registry's value without checking it was
// found → a nil interface assertion panics → reddens.
func TestGuard_theHookNamesAForgottenHandle(t *testing.T) {
	ctx := context.Background()
	nonce := registerGuard(profileA)
	forgetGuard(nonce)
	path := filepath.Join(t.TempDir(), "forgotten.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("conn: %v", err)
	}
	defer func() { _ = conn.Close() }()
	var hookErr error
	if err := conn.Raw(func(dc any) error {
		hookErr = guardHook(dc.(msqlite.ExecQuerierContext), "file:"+path+"?"+guardParam+"="+nonce)
		return nil
	}); err != nil {
		t.Fatalf("raw: %v", err)
	}
	if !errors.Is(hookErr, ErrGuardHandleUnknown) {
		t.Fatalf("the hook on a forgotten handle = %v, want ErrGuardHandleUnknown", hookErr)
	}
}

// C09 · the prune judges and deletes in ONE immediate transaction: an
// adoption from another handle cannot commit between the judgement and the
// DELETE. The seam between the two launches B's adoption; it must still be
// waiting when the seam returns, and land only after the prune committed.
//
// PROBING MUTATION: judge through the pool and delete through the pool (the
// shape before the cure) → B adopts inside the seam → reddens.
func TestPrune_judgesAndDeletesInOneImmediateTransaction(t *testing.T) {
	ctx := context.Background()
	a, evidenceFor := foundedFor(t, profileA)
	mustRecord(t, a, "act_d1", action.StateDenied)
	mustRecord(t, a, "act_d2", action.StateDenied)
	terminals := countRows(t, a, "actions") // the founding act closed SUCCEEDED, and two denials
	a.capRows = 0
	b, err := OpenOperatorFor(a.path, profileB)
	if err != nil {
		t.Fatalf("open B: %v", err)
	}
	defer func() { _ = b.Close() }()
	wireSealedLike(t, b, a)
	env, evidence := evidenceFor("act_adopt")
	adopted := make(chan error, 1)
	seamRan := false
	// The oracle by impossibility: a THIRD real connection with no busy
	// timeout must be REFUSED a write lock while the prune holds its
	// transaction (SQLITE_BUSY, primary code 5); B's adoption, launched in
	// the same window, must land only after the prune returns.
	third, err := sql.Open("sqlite", "file:"+a.path+"?_pragma=busy_timeout(0)")
	if err != nil {
		t.Fatalf("open the third connection: %v", err)
	}
	defer func() { _ = third.Close() }()
	third.SetMaxOpenConns(1)
	a.seams.beforePruneDelete = func() {
		seamRan = true
		_, lockErr := third.ExecContext(ctx, `BEGIN IMMEDIATE`)
		var coded interface{ Code() int }
		if lockErr == nil {
			_, _ = third.ExecContext(ctx, `ROLLBACK`)
			t.Errorf("a third connection took the write lock between the prune's judgement and its DELETE")
		} else if !errors.As(lockErr, &coded) || coded.Code()&0xff != 5 {
			t.Errorf("the third connection's BEGIN IMMEDIATE = %v, want SQLITE_BUSY (5)", lockErr)
		}
		go func() {
			_, err := b.AdoptLedger(ctx, adoptionEnv(env), Decision{Outcome: "allow", Rule: "operator"}, evidence, profileB)
			adopted <- err
		}()
		select {
		case err := <-adopted:
			t.Errorf("B's adoption landed (%v) between the prune's judgement and its DELETE", err)
			adopted <- err
		case <-time.After(300 * time.Millisecond):
		}
	}
	removed, err := a.Prune(ctx)
	if err != nil {
		t.Fatalf("the owner's prune: %v", err)
	}
	if !seamRan {
		t.Fatal("the prune never reached its seam")
	}
	if removed != terminals {
		t.Fatalf("removed %d, want %d (the owner prunes every terminal of its own ledger)", removed, terminals)
	}
	select {
	case err := <-adopted:
		if err != nil {
			t.Fatalf("B's adoption after the prune released the lock: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("B's adoption never landed after the prune")
	}
	if standing, owner, err := a.Standing(ctx); err != nil || standing != LedgerStandingForeignProfile || owner != profileB {
		t.Fatalf("after B's adoption A sees %q %q %v, want foreign owned by B", standing, owner, err)
	}
}

// C13 · the three openers refuse a malformed or empty identity by name,
// BEFORE touching the file: the path does not exist and must not exist
// afterwards (OpenFor and OpenOperatorFor would create it; OpenReadOnlyFor
// would name the missing file instead of the identity).
//
// PROBING MUTATION: checkIdentity accepts everything → the file is created,
// or the path is named instead of the identity → reddens. (M269 mutated the
// check with an existing ledger and stayed green: setIdentity refused the
// same identity after the open — the round's own finding about the mould.)
func TestOpeners_refuseAMalformedIdentityByName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "never.db")
	openers := []struct {
		name string
		open func(string, string) (*Store, error)
	}{{"OpenFor", OpenFor}, {"OpenOperatorFor", OpenOperatorFor}, {"OpenReadOnlyFor", OpenReadOnlyFor}}
	bad := []struct {
		id   string
		want error
	}{
		{"", ErrNoProfileIdentity},
		{"junk", ErrProfileIdentityMalformed},
		{strings.ToUpper(profileA), ErrProfileIdentityMalformed},
		{"sha256:" + strings.Repeat("0", 63), ErrProfileIdentityMalformed},
	}
	for _, o := range openers {
		for _, b := range bad {
			s, err := o.open(path, b.id)
			if s != nil {
				_ = s.Close()
				t.Fatalf("%s(%q) handed out a handle", o.name, b.id)
			}
			if !errors.Is(err, b.want) {
				t.Fatalf("%s(%q) = %v, want %v", o.name, b.id, err, b.want)
			}
			if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("%s(%q) touched the file before judging the identity (stat: %v)", o.name, b.id, statErr)
			}
		}
	}
}

// C13 · a read-only handle names a foreign ledger and never writes: the
// foreign refusal is judged before any write, and on its own ledger the file
// itself refuses (SQLITE_READONLY, primary code 8).
//
// PROBING MUTATION: the read-only opener skips `PRAGMA query_only` → the
// owner's read-only handle writes → reddens.
func TestReadOnly_aForeignLedgerIsReadAndNeverWritten(t *testing.T) {
	ctx := context.Background()
	store, _ := foundedFor(t, profileA)
	ro, err := OpenReadOnlyFor(store.path, profileB)
	if err != nil {
		t.Fatalf("read-only open for B: %v", err)
	}
	defer func() { _ = ro.Close() }()
	if standing, owner, err := ro.Standing(ctx); err != nil || standing != LedgerStandingForeignProfile || owner != profileA {
		t.Fatalf("B's read-only handle sees %q %q %v, want foreign owned by A", standing, owner, err)
	}
	if err := ro.RecordAttempt(ctx, testEnvelope("act_ro"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized); !errors.Is(err, ErrLedgerForeignProfile) {
		t.Fatalf("a write through B's read-only handle = %v, want ErrLedgerForeignProfile", err)
	}
	own, err := OpenReadOnlyFor(store.path, profileA)
	if err != nil {
		t.Fatalf("read-only open for A: %v", err)
	}
	defer func() { _ = own.Close() }()
	err = own.RecordAttempt(ctx, testEnvelope("act_ro"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized)
	var coded interface{ Code() int }
	if !errors.As(err, &coded) || coded.Code()&0xff != 8 {
		t.Fatalf("a write through the owner's read-only handle = %v, want SQLITE_READONLY (8)", err)
	}
	if got := countRows(t, store, "actions"); got != 1 {
		t.Fatalf("actions rows = %d, want the founding act only", got)
	}
}

// C13 · a foreign ledger written by a NEWER binary is neither migrated nor
// downgraded by this one: both writer openers refuse and name the versions.
//
// PROBING MUTATION: requireCurrentSchema returns nil → reddens.
func TestOpen_aForeignLedgerFromANewerSchemaIsNotMigrated(t *testing.T) {
	store, _ := foundedFor(t, profileA)
	raw := rawConn(t, store)
	rawExec(t, raw, `UPDATE action_schema SET version = 17`)
	for name, open := range map[string]func(string, string) (*Store, error){"OpenFor": OpenFor, "OpenOperatorFor": OpenOperatorFor} {
		s, err := open(store.path, profileB)
		if s != nil {
			_ = s.Close()
			t.Fatalf("%s handed out a handle on a foreign v17 ledger", name)
		}
		if !errors.Is(err, ErrSchemaFromTheFuture) {
			t.Fatalf("%s on a foreign v17 ledger = %v, want ErrSchemaFromTheFuture", name, err)
		}
	}
	if got := rawCount(t, raw, `SELECT version FROM action_schema`); got != 17 {
		t.Fatalf("schema version %d after the refusals, want 17 untouched", got)
	}
}

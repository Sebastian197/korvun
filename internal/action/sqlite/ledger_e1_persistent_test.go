// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Train E, batch 1 — PERSISTENT damage the judge must name (plan v3, §6
// rows TE13–TE18, TE63–TE65): the official reproductions and their adjacent
// cases, replayed literally on real files. Every damaged schema stays as the
// attacker left it: the stores observe, they never repair.
//
// Evidence level: in process on a real file; the attacker is a SECOND real
// connection; the app's and the compiled CLI's halves of these rows live in
// their own packages.

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// e1Openers are the three profile-aware opener families.
var e1Openers = []struct {
	name   string
	open   func(string, string) (*Store, error)
	writer bool
}{
	{"OpenFor", OpenFor, true},
	{"OpenOperatorFor", OpenOperatorFor, true},
	{"OpenReadOnlyFor", OpenReadOnlyFor, false},
}

// e1Blocked asserts the one outcome of a persistent structural defect through
// every opener: a handle, Standing ErrLedgerUnreadable naming want, and — on
// the writers — an unreadable guard that refuses a write by name.
func e1Blocked(t *testing.T, path, profile string, want ...string) {
	t.Helper()
	ctx := context.Background()
	for _, o := range e1Openers {
		h, err := o.open(path, profile)
		if err != nil || h == nil {
			t.Fatalf("%s over the damaged ledger = %v, want a handle that names it", o.name, err)
		}
		st, owner, serr := h.Standing(ctx)
		if st != LedgerStandingUnreadable || owner != "" || !errors.Is(serr, ErrLedgerUnreadable) {
			_ = h.Close()
			t.Fatalf("%s: Standing = %q %q %v, want unreadable with ErrLedgerUnreadable", o.name, st, owner, serr)
		}
		for _, w := range want {
			if !strings.Contains(serr.Error(), w) {
				_ = h.Close()
				t.Fatalf("%s: the standing's cause %q does not name %q", o.name, serr, w)
			}
		}
		if o.writer {
			if row := guardRow(t, h); row != string(LedgerStandingUnreadable) {
				_ = h.Close()
				t.Fatalf("%s: the guard row = %q, want ledger_unreadable", o.name, row)
			}
			if err := guardedWrite(h, "e1_refused"); !errors.Is(err, ErrLedgerUnreadable) {
				_ = h.Close()
				t.Fatalf("%s: a write = %v, want ErrLedgerUnreadable", o.name, err)
			}
		}
		_ = h.Close()
	}
}

// e1Founded is a founded ledger of profileA, its raw attacker, and the path;
// the guarded handle is closed so that every open below births afresh.
func e1Founded(t *testing.T) (string, func(q string, args ...any)) {
	t.Helper()
	store, _ := foundedFor(t, profileA)
	raw := rawConn(t, store)
	_ = store.Close()
	return store.path, func(q string, args ...any) { rawExec(t, raw, q, args...) }
}

// e1Legacy is the identity fixture never founded: registered and inked, no
// identity row, no mark; one UNMARKED receipt when withReceipt.
func e1Legacy(t *testing.T, withReceipt bool) (string, func(q string, args ...any)) {
	t.Helper()
	store, _, _, _, now := identityStoreFixture(t)
	if withReceipt {
		mustRecord(t, store, "act_unmarked", action.StateAuthorized)
		if err := store.FinishWithResult(context.Background(), "act_unmarked", action.StateSucceeded, now.Add(time.Second), "sha256:"+strings.Repeat("ab", 32)); err != nil {
			t.Fatalf("close the unmarked act: %v", err)
		}
		raw := rawConn(t, store)
		if n := rawCount(t, raw, `SELECT COUNT(*) FROM receipts WHERE result_digest NOT LIKE 'profile:%'`); n < 1 {
			t.Fatalf("the legacy fixture has %d unmarked receipts, want at least 1", n)
		}
	}
	raw := rawConn(t, store)
	if n := rawCount(t, raw, `SELECT COUNT(*) FROM ledger_identity`); n != 0 {
		t.Fatalf("the legacy fixture has %d identity rows", n)
	}
	return store.path, func(q string, args ...any) { rawExec(t, raw, q, args...) }
}

// e1PermissiveIdentity rebuilds ledger_identity without its constraints, the
// rows kept: the only way a NULL, a second row or another id gets in.
func e1PermissiveIdentity(exec func(q string, args ...any)) {
	exec(`PRAGMA foreign_keys = OFF`)
	exec(`CREATE TABLE ledger_identity_p (id INTEGER, owner_digest TEXT, founded_by_action TEXT, adopted_by_action TEXT, written_at TEXT)`)
	exec(`INSERT INTO ledger_identity_p SELECT id, owner_digest, founded_by_action, adopted_by_action, written_at FROM ledger_identity`)
	exec(`DROP TABLE ledger_identity`)
	exec(`ALTER TABLE ledger_identity_p RENAME TO ledger_identity`)
}

// e1PermissiveSchema rebuilds action_schema without NOT NULL.
func e1PermissiveSchema(exec func(q string, args ...any)) {
	exec(`CREATE TABLE action_schema_p (version)`)
	exec(`INSERT INTO action_schema_p SELECT version FROM action_schema`)
	exec(`DROP TABLE action_schema`)
	exec(`ALTER TABLE action_schema_p RENAME TO action_schema`)
}

// TE13 · the official reproduction (P2-2, step 1): the version column of a
// founded ledger renamed. Every opener hands out a handle that NAMES it —
// Standing ErrLedgerUnreadable, the writers' guard unreadable and every write
// refused by name — and nothing is repaired: the catalog and the receipts are
// the attacker's.
//
// PROBING MUTATION (MU13): judgeShape propagates the code-1 failure of its
// version read as an operational error → the openers die → reddens.
func TestE1_TE13_aRenamedVersionColumnOpensBlockedEverywhere(t *testing.T) {
	t.Parallel()
	path, exec := e1Founded(t)
	raw := rawConnPath(t, path)
	receipts := rawCount(t, raw, `SELECT COUNT(*) FROM receipts`)
	exec(`ALTER TABLE action_schema RENAME COLUMN version TO v`)
	before := catalogOf(t, raw)
	e1Blocked(t, path, profileA, "version")
	if after := catalogOf(t, raw); !reflect.DeepEqual(before, after) {
		t.Fatal("an opener repaired the renamed column: the catalog changed")
	}
	if n := rawCount(t, raw, `SELECT COUNT(*) FROM receipts`); n != receipts {
		t.Fatalf("receipts %d → %d", receipts, n)
	}
}

// TE14 · the owner column of the identity row renamed: blocked handles
// through every opener, the standing naming owner_digest; the opener neither
// dies on its own owner read nor takes the ledger for one with no row, and
// nothing is migrated, seeded, adopted or repaired.
//
// PROBING MUTATIONS (MU14): remove the structural branch of readOwner (the
// writer opener dies) → reddens; of judgeIn's owner read (the refresh fails,
// the open dies) → reddens; of judgeOnConn's owner read (the hook refuses the
// connection, the open dies) → reddens.
func TestE1_TE14_aRenamedOwnerColumnOpensBlockedEverywhere(t *testing.T) {
	t.Parallel()
	path, exec := e1Founded(t)
	raw := rawConnPath(t, path)
	exec(`ALTER TABLE ledger_identity RENAME COLUMN owner_digest TO o`)
	before := catalogOf(t, raw)
	e1Blocked(t, path, profileA, "owner_digest")
	if after := catalogOf(t, raw); !reflect.DeepEqual(before, after) {
		t.Fatal("an opener repaired or migrated the renamed owner column")
	}
	if n := rawCount(t, raw, `SELECT COUNT(*) FROM ledger_identity`); n != 1 {
		t.Fatalf("identity rows = %d, want the one row, untouched", n)
	}
}

// TE15 (the store's half) · N9-1, steps 1–4, literally: a legacy ledger (no
// identity row, no mark) with a valid UNMARKED receipt, and the receipts'
// result_digest renamed to rd. The mark read fails; that failure is a verdict
// of every opener, never a dead open: blocked handles naming result_digest.
//
// PROBING MUTATION (MU15): judgeWithoutRow's failure without its class (an
// operational error) → the refresh fails, the open dies → reddens.
func TestE1_TE15_aRenamedMarkColumnOpensBlockedEverywhere(t *testing.T) {
	t.Parallel()
	path, exec := e1Legacy(t, true)
	raw := rawConnPath(t, path)
	exec(`ALTER TABLE receipts RENAME COLUMN result_digest TO rd`)
	before := catalogOf(t, raw)
	e1Blocked(t, path, profileA, "result_digest")
	if after := catalogOf(t, raw); !reflect.DeepEqual(before, after) {
		t.Fatal("an opener repaired the renamed mark column")
	}
	if n := rawCount(t, raw, `SELECT COUNT(*) FROM ledger_identity`); n != 0 {
		t.Fatalf("identity rows = %d: a mark or an owner was fabricated", n)
	}
}

// TE16 · a SUCCESSFUL read of an invalid cell is a verdict naming the field
// and what is wrong with it — never «no row», never a scan error: a NULL
// owner (founded, and legacy with no mark, where «NULL means no row» would
// read legacy), a non-canonical owner, a NULL version and a version whose
// bytes are not a number. The rows stay as the attacker left them.
//
// PROBING MUTATIONS (MU16): «NULL means no row» at the owner read → the
// legacy subcase reads legacy_unfounded → reddens; every uncoded Scan failure
// taken as shape — killed by TE19, the converse.
func TestE1_TE16_anInvalidCellIsAVerdictNamingItsField(t *testing.T) {
	t.Parallel()
	t.Run("a NULL owner on a founded ledger", func(t *testing.T) {
		t.Parallel()
		path, exec := e1Founded(t)
		e1PermissiveIdentity(exec)
		exec(`UPDATE ledger_identity SET owner_digest = NULL`)
		e1Blocked(t, path, profileA, "owner_digest", "NULL")
		raw := rawConnPath(t, path)
		if n := rawCount(t, raw, `SELECT COUNT(*) FROM ledger_identity WHERE owner_digest IS NULL`); n != 1 {
			t.Fatalf("the NULL row was touched (%d NULL rows)", n)
		}
	})
	t.Run("a NULL owner on a ledger with no mark", func(t *testing.T) {
		t.Parallel()
		path, exec := e1Legacy(t, false)
		e1PermissiveIdentity(exec)
		exec(`INSERT INTO ledger_identity (id, owner_digest, founded_by_action, adopted_by_action, written_at) VALUES (1, NULL, 'act_x', NULL, '2026-09-26T00:00:00Z')`)
		e1Blocked(t, path, profileA, "owner_digest", "NULL")
	})
	t.Run("a non-canonical owner", func(t *testing.T) {
		t.Parallel()
		path, exec := e1Founded(t)
		e1PermissiveIdentity(exec)
		exec(`UPDATE ledger_identity SET owner_digest = 'garbage'`)
		e1Blocked(t, path, profileA, "canonical")
	})
	t.Run("a NULL version", func(t *testing.T) {
		t.Parallel()
		path, exec := e1Founded(t)
		e1PermissiveSchema(exec)
		exec(`UPDATE action_schema SET version = NULL`)
		e1Blocked(t, path, profileA, "version", "NULL")
	})
	t.Run("a version whose bytes are not a number", func(t *testing.T) {
		t.Parallel()
		path, exec := e1Founded(t)
		exec(`UPDATE action_schema SET version = CAST('1x' AS BLOB)`)
		e1Blocked(t, path, profileA, "version")
	})
}

// TE17 · ONE parser reads a stored version everywhere: `16 ` stored as a
// BLOB is version 16 for the shape, for the migration's reader (the owner's
// open) and for the foreign version check (another profile's open) — a
// handle, no scan error — and the stored BLOB is kept as it is.
//
// PROBING MUTATIONS (MU17): keep migrate's direct Scan into an int → the
// owner's open dies → reddens; keep requireCurrentSchema's → the foreign
// open dies → reddens.
func TestE1_TE17_oneParserReadsTheStoredVersion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path, exec := e1Founded(t)
	exec(`UPDATE action_schema SET version = CAST('16 ' AS BLOB)`)
	for _, who := range []struct {
		name, profile string
		want          LedgerStanding
	}{{"the owner", profileA, LedgerStandingOK}, {"another profile", profileB, LedgerStandingForeignProfile}} {
		h, err := OpenFor(path, who.profile)
		if err != nil {
			t.Fatalf("OpenFor by %s over version `16 ` = %v, want a handle", who.name, err)
		}
		if st, _, err := h.Standing(ctx); err != nil || st != who.want {
			_ = h.Close()
			t.Fatalf("%s stands %q %v, want %q", who.name, st, err, who.want)
		}
		_ = h.Close()
	}
	for _, o := range e1Openers[1:] {
		h, err := o.open(path, profileA)
		if err != nil {
			t.Fatalf("%s over version `16 ` = %v, want a handle", o.name, err)
		}
		if st, _, err := h.Standing(ctx); err != nil || st != LedgerStandingOK {
			_ = h.Close()
			t.Fatalf("%s stands %q %v, want ok", o.name, st, err)
		}
		_ = h.Close()
	}
	raw := rawConnPath(t, path)
	var kind, hexed string
	if err := raw.QueryRow(`SELECT typeof(version), hex(version) FROM action_schema`).Scan(&kind, &hexed); err != nil || kind != "blob" || hexed != "313620" {
		t.Fatalf("the stored version is %s %s (%v), want the BLOB `16 ` kept", kind, hexed, err)
	}
}

// TE18 · N10-1 literally: a v1 file lifted to version 99 has no identity
// table, and a v16 file at version 17 has a renamed owner column; a readable
// version above 16 is ErrSchemaFromTheFuture for all three openers BEFORE the
// owner is read — the owner read is never reached — and nothing is changed.
//
// PROBING MUTATIONS (MU18): read the owner before the future check → the v1
// case dies on «no such table», the owner read is reached → reddens; route
// the owner read's verdict to a blocked open → a handle → reddens.
func TestE1_TE18_aFutureVersionIsNamedBeforeTheOwnerIsRead(t *testing.T) {
	for name, build := range map[string]func(*testing.T) string{
		"a v1 file at version 99": func(t *testing.T) string {
			path := buildV1File(t)
			raw := rawConnPath(t, path)
			rawExec(t, raw, `UPDATE action_schema SET version = 99`)
			return path
		},
		"a v16 file at version 17 with its owner column renamed": func(t *testing.T) string {
			path, exec := e1Founded(t)
			exec(`ALTER TABLE ledger_identity RENAME COLUMN owner_digest TO o`)
			exec(`UPDATE action_schema SET version = 17`)
			return path
		},
	} {
		t.Run(name, func(t *testing.T) {
			path := build(t)
			raw := rawConnPath(t, path)
			before := catalogOf(t, raw)
			version := rawCount(t, raw, `SELECT version FROM action_schema`)
			observer := armJudgeFault(t, path, originReadOwnerAtOpen, siteOwnerDB, stageQuery, errors.New("TE18: the owner read was reached"))
			for _, o := range e1Openers {
				h, err := o.open(path, profileA)
				if h != nil {
					_ = h.Close()
					t.Fatalf("%s handed out a handle over a future schema", o.name)
				}
				if !errors.Is(err, ErrSchemaFromTheFuture) {
					t.Fatalf("%s over a future schema = %v, want ErrSchemaFromTheFuture", o.name, err)
				}
			}
			if r := observer.disarm(); r.matches != 0 {
				t.Fatalf("the owner read was reached %d time(s) before the future check", r.matches)
			}
			if after := catalogOf(t, raw); !reflect.DeepEqual(before, after) {
				t.Fatal("an opener changed the future ledger's catalog")
			}
			if v := rawCount(t, raw, `SELECT version FROM action_schema`); v != version {
				t.Fatalf("the version moved %d → %d", version, v)
			}
		})
	}
}

// TE63 · C1-13's two-row reconstruction: ledger_identity rebuilt without its
// CHECK, id 1 owned by A kept, id 2 owned by B added. The DB judge reads the
// SAME stored rows as the hook — at most two in one query — and both call it
// unreadable, for A and for B; both rows are kept, nothing is adopted.
//
// PROBING MUTATION (MU63): the DB judge filters WHERE id = 1 (or drops the
// second-row check) → A stands ok while the hook says unreadable → reddens.
func TestE1_TE63_twoIdentityRowsAreUnreadableForEveryJudge(t *testing.T) {
	t.Parallel()
	path, exec := e1Founded(t)
	e1PermissiveIdentity(exec)
	exec(`INSERT INTO ledger_identity (id, owner_digest, founded_by_action, adopted_by_action, written_at) VALUES (2, ?, 'act_b', NULL, '2026-09-26T00:00:00Z')`, profileB)
	e1Blocked(t, path, profileA)
	e1Blocked(t, path, profileB)
	raw := rawConnPath(t, path)
	if n := rawCount(t, raw, `SELECT COUNT(*) FROM ledger_identity`); n != 2 {
		t.Fatalf("identity rows = %d, want both kept", n)
	}
}

// TE64 · C1-13's adjacent case: a legacy ledger (no mark) whose only identity
// row carries id 2 and a canonical owner. It is unreadable — never «no row»,
// never legacy — and the row stays where it is.
//
// PROBING MUTATION (MU64): the wrong-id row read as no row → legacy →
// reddens.
func TestE1_TE64_aWrongIdRowIsUnreadableNeverLegacy(t *testing.T) {
	t.Parallel()
	path, exec := e1Legacy(t, false)
	e1PermissiveIdentity(exec)
	exec(`INSERT INTO ledger_identity (id, owner_digest, founded_by_action, adopted_by_action, written_at) VALUES (2, ?, 'act_b', NULL, '2026-09-26T00:00:00Z')`, profileB)
	e1Blocked(t, path, profileA)
	raw := rawConnPath(t, path)
	if n := rawCount(t, raw, `SELECT COUNT(*) FROM ledger_identity WHERE id = 2`); n != 1 {
		t.Fatalf("the id-2 row was moved or dropped (%d left)", n)
	}
	if n := rawCount(t, raw, `SELECT COUNT(*) FROM ledger_identity WHERE id = 1`); n != 0 {
		t.Fatalf("a row with id 1 was fabricated")
	}
}

// TE65 · SchemaVersion reads its version through the same parser: `16 ` as a
// BLOB is 16; a NULL and bytes that are not a number are 0 and
// ErrLedgerUnreadable; a closed pool is its own error, no class; 99 is 99 —
// the accessor answers the version, the openers decide what it means.
//
// PROBING MUTATIONS (MU65): keep the direct Scan into an int → the BLOB case
// reddens; NULL read as 0 with no error → reddens.
func TestE1_TE65_SchemaVersionReadsThroughTheSharedParser(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	open := func(t *testing.T) (*Store, func(q string, args ...any)) {
		path, exec := e1Founded(t)
		h, err := OpenFor(path, profileA)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = h.Close() })
		return h, exec
	}
	t.Run("`16 ` as a BLOB", func(t *testing.T) {
		t.Parallel()
		h, exec := open(t)
		exec(`UPDATE action_schema SET version = CAST('16 ' AS BLOB)`)
		if v, err := h.SchemaVersion(ctx); err != nil || v != 16 {
			t.Fatalf("SchemaVersion = %d %v, want 16", v, err)
		}
	})
	t.Run("NULL", func(t *testing.T) {
		t.Parallel()
		h, exec := open(t)
		e1PermissiveSchema(exec)
		exec(`UPDATE action_schema SET version = NULL`)
		if v, err := h.SchemaVersion(ctx); v != 0 || !errors.Is(err, ErrLedgerUnreadable) {
			t.Fatalf("SchemaVersion over a NULL = %d %v, want 0 and ErrLedgerUnreadable", v, err)
		}
	})
	t.Run("bytes that are not a number", func(t *testing.T) {
		t.Parallel()
		h, exec := open(t)
		exec(`UPDATE action_schema SET version = CAST('abc' AS BLOB)`)
		if v, err := h.SchemaVersion(ctx); v != 0 || !errors.Is(err, ErrLedgerUnreadable) {
			t.Fatalf("SchemaVersion over `abc` = %d %v, want 0 and ErrLedgerUnreadable", v, err)
		}
	})
	t.Run("a closed pool", func(t *testing.T) {
		t.Parallel()
		h, _ := open(t)
		_ = h.Close()
		closed := h.db.Ping()
		v, err := h.SchemaVersion(ctx)
		if v != 0 || closed == nil || !errors.Is(err, closed) || !namesNoClass(err) {
			t.Fatalf("SchemaVersion on a closed pool = %d %v, want 0 and the pool's own error %v, unnamed", v, err, closed)
		}
	})
	t.Run("99", func(t *testing.T) {
		t.Parallel()
		h, exec := open(t)
		exec(`UPDATE action_schema SET version = 99`)
		if v, err := h.SchemaVersion(ctx); err != nil || v != 99 {
			t.Fatalf("SchemaVersion over 99 = %d %v, want 99 (the accessor answers, the openers decide)", v, err)
		}
	})
}

// rawConnPath is rawConn for a path with no store at hand.
func rawConnPath(t *testing.T, path string) *sql.DB {
	t.Helper()
	return rawConn(t, &Store{path: filepath.Clean(path)})
}

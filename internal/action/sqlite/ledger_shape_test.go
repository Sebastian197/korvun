// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The ledger's SHAPE, judged in one place (train D, 2026-09-25): the moulds
// of the plan's rows D01–D04, D07 and D08 (§14 of the redesign paper; D06,
// the real exclusive lock, could not reach the hook — captured in red.txt —
// and was retired for D07's seam). The
// shape is what the connection hook, the openers and the readers all
// consult: fresh (no action-store table: the shared file may carry the
// conversation store), older than the row, current, newer, or bad — and a
// bad shape is opened unreadable and never repaired.
//
// Evidence level: in-process store on a real file; «raw» is a SECOND real
// connection; D01 opens the REAL conversation store first; D06 holds a real
// exclusive lock from a raw connection; D07 injects the hook's query
// failure through a seam (declared); D08 is two real handles with a seam.

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	convsqlite "github.com/Sebastian197/korvun/internal/conversation/sqlite"
)

// catalogOf is every object of the file but SQLite's own, by type and name,
// with its DDL: the oracle «nothing was repaired».
func catalogOf(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	rows, err := db.Query(`SELECT type || ':' || name, COALESCE(sql, '') FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			t.Fatal(err)
		}
		out[k] = v
	}
	return out
}

// D01 · the shared file: the conversation store owns the file first, and for
// the action store that file is FRESH (no action-store table), not corrupt.
//
// PROBING MUTATION: fresh = no object at all in sqlite_master → reddens.
func TestShape_theSharedFileIsFreshForTheActionStore(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "korvun.db")
	conv, err := convsqlite.Open(path)
	if err != nil {
		t.Fatalf("open the conversation store: %v", err)
	}
	defer func() { _ = conv.Close() }()
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	if n := rawCount(t, raw, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name IN ('sessions', 'turns', 'notes')`); n != 3 {
		t.Fatalf("the conversation store left %d of its 3 tables: the shape under test did not happen", n)
	}
	if got := judgeOnRawConn(t, path, profileA); got != "ok" {
		t.Fatalf("the hook judged the shared file %q, want ok (fresh for the action store)", got)
	}
	s, err := OpenFor(path, profileA)
	if err != nil {
		t.Fatalf("OpenFor on the shared file: %v", err)
	}
	defer func() { _ = s.Close() }()
	if standing, _, err := s.Standing(ctx); err != nil || standing != LedgerStandingLegacyUnfounded {
		t.Fatalf("the shared file opened as %q %v, want legacy_unfounded", standing, err)
	}
	mustRecord(t, s, "act_shared", action.StateAuthorized)
	if got := countRows(t, s, "actions"); got != 1 {
		t.Fatalf("actions rows = %d, want 1", got)
	}
	if n := rawCount(t, raw, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name IN ('sessions', 'turns', 'notes')`); n != 3 {
		t.Fatalf("the action store's open removed the conversation store's tables (%d of 3 left)", n)
	}
}

// D02 · the constant list the shape is judged against IS the fresh store's.
//
// PROBING MUTATION: drop one name from schemaTablesV16 → reddens.
func TestShape_theTableListIsTheFreshStores(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")
	s, err := OpenFor(path, profileA)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	rows, err := s.db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var got []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		got = append(got, n)
	}
	want := append([]string(nil), schemaTablesV16...)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("schemaTablesV16 is not the fresh store's table list\n got %v\nwant %v", got, want)
	}
}

// D03–D04 · a bad shape — one table of the schema missing, the schema table
// included — opens UNREADABLE through both writer openers and is never
// repaired: the catalog is identical before and after, the standing names
// the missing table, every act refuses by name, and the reader names it.
//
// PROBING MUTATIONS: createStmt unconditional (the table comes back:
// reddens on the catalog); judgeIn without the shape (Standing says ok:
// reddens).
func TestShape_aBadShapeOpensUnreadableAndIsNeverRepaired(t *testing.T) {
	ctx := context.Background()
	for _, table := range []string{"actions", "action_decisions", "receipts", "approval_tombstones", "authorization_snapshots", "intents", "action_schema"} {
		t.Run(table, func(t *testing.T) {
			store, _ := foundedFor(t, profileA)
			path := store.path
			raw := rawConn(t, store)
			rawExec(t, raw, `PRAGMA foreign_keys = OFF`)
			rawExec(t, raw, `DROP TABLE `+table)
			_ = store.Close()
			before := catalogOf(t, raw)
			actsBefore := rawCount(t, raw, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'actions'`)
			for _, o := range []struct {
				name string
				open func(string, string) (*Store, error)
			}{{"OpenOperatorFor", OpenOperatorFor}, {"OpenFor", OpenFor}} {
				h, err := o.open(path, profileA)
				if err != nil {
					t.Fatalf("%s on a ledger without %s: %v, want an open with the guard unreadable", o.name, table, err)
				}
				var row string
				if err := h.db.QueryRow(`SELECT standing FROM temp.profile_guard`).Scan(&row); err != nil || row != string(LedgerStandingUnreadable) {
					t.Fatalf("%s: guard row = %q (%v), want unreadable", o.name, row, err)
				}
				standing, _, err := h.Standing(ctx)
				if standing != LedgerStandingUnreadable || !errors.Is(err, ErrLedgerUnreadable) || !strings.Contains(err.Error(), table) {
					t.Fatalf("%s: Standing = %q %v, want unreadable naming %s", o.name, standing, err, table)
				}
				if err := h.RecordAttempt(ctx, testEnvelope("act_bad"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized); !errors.Is(err, ErrLedgerUnreadable) {
					t.Fatalf("%s: RecordAttempt = %v, want ErrLedgerUnreadable", o.name, err)
				}
				_ = h.Close()
				if after := catalogOf(t, raw); !reflect.DeepEqual(before, after) {
					t.Fatalf("%s REPAIRED the ledger without %s: the catalog changed\nbefore %d objects, after %d", o.name, table, len(before), len(after))
				}
			}
			if actsBefore == 1 {
				if got := rawCount(t, raw, `SELECT COUNT(*) FROM actions`); got != 1 {
					t.Fatalf("actions rows = %d, want the founding act only", got)
				}
			}
			ro, err := OpenReadOnlyFor(path, profileA)
			if err != nil {
				t.Fatalf("the reader on a ledger without %s: %v, want an open that names it", table, err)
			}
			defer func() { _ = ro.Close() }()
			if standing, _, err := ro.Standing(ctx); standing != LedgerStandingUnreadable || !errors.Is(err, ErrLedgerUnreadable) || !strings.Contains(err.Error(), table) {
				t.Fatalf("the reader's Standing = %q %v, want unreadable naming %s", standing, err, table)
			}
		})
	}
}

// D07 · the hook's own branch, through a seam: a failure that is not a shape
// verdict — here an UNCODED error, met while judging a new connection —
// REFUSES the connection with that error; the next call, with the seam
// clear, births a judged connection and writes. (A structural code on a
// judge read is a verdict instead: train E's TE20–TE23.)
//
// PROBING MUTATION: the hook maps this non-verdict failure to
// ledger_unreadable → reddens.
func TestShape_theHookRefusesANonVerdictFailure(t *testing.T) {
	ctx := context.Background()
	store, _ := foundedFor(t, profileA)
	store.db.SetConnMaxLifetime(time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	fault := func() error { return errors.New("injected: the catalog could not be read") }
	hookShapeFault.Store(&fault)
	t.Cleanup(func() { hookShapeFault.Store(nil) })
	err := store.RecordAttempt(ctx, testEnvelope("act_faulted"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized)
	if err == nil || !strings.Contains(err.Error(), "injected") {
		t.Fatalf("a write on a connection whose hook failed = %v, want the injected error", err)
	}
	if errors.Is(err, ErrLedgerUnreadable) {
		t.Fatalf("the hook's non-verdict failure was judged a verdict: %v", err)
	}
	hookShapeFault.Store(nil)
	store.db.SetConnMaxLifetime(0)
	if err := store.RecordAttempt(ctx, testEnvelope("act_after_fault"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized); err != nil {
		t.Fatalf("the write on the next connection: %v", err)
	}
	if got := countRows(t, store, "actions"); got != 2 {
		t.Fatalf("actions rows = %d, want 2", got)
	}
}

// D08 · an adoption confirmed between OpenFor's judgement and its prune is
// «no prune», never a failed boot: the handle comes back, foreign.
//
// PROBING MUTATION: OpenFor fails on the prune's refusal → reddens.
func TestOpen_anAdoptionBetweenTheJudgementAndThePruneIsNotBootFatal(t *testing.T) {
	ctx := context.Background()
	a, evidenceFor := foundedFor(t, profileA)
	path := a.path
	b, err := OpenOperatorFor(path, profileB)
	if err != nil {
		t.Fatalf("open B: %v", err)
	}
	defer func() { _ = b.Close() }()
	wireSealedLike(t, b, a)
	env, evidence := evidenceFor("act_adopt")
	_ = a.Close()
	seamRan := false
	seam := func() {
		seamRan = true
		if _, err := b.AdoptLedger(ctx, adoptionEnv(env), Decision{Outcome: "allow", Rule: "operator"}, evidence, profileB); err != nil {
			t.Errorf("B's adoption inside the seam: %v", err)
		}
	}
	openPruneSeam.Store(&seam)
	t.Cleanup(func() { openPruneSeam.Store(nil) })
	h, err := OpenFor(path, profileA)
	if err != nil {
		t.Fatalf("OpenFor after an adoption between the judgement and the prune = %v, want a handle (no prune, not a failed boot)", err)
	}
	defer func() { _ = h.Close() }()
	if !seamRan {
		t.Fatal("the open never reached its seam")
	}
	if standing, owner, err := h.Standing(ctx); err != nil || standing != LedgerStandingForeignProfile || owner != profileB {
		t.Fatalf("after the adoption the boot's handle sees %q %q %v, want foreign owned by B", standing, owner, err)
	}
}

// D15 · every door names the shape it refuses: an existing file with no
// action store yet (the conversation store's alone) is «not a korvun store»
// for the operator's door and the reader; an older schema is «never
// migrates» for both; a newer one is ErrSchemaFromTheFuture for both and
// for the owner's boot (which would migrate and cannot); and a shape asked
// through a closed pool is an error, not a verdict.
//
// PROBING MUTATIONS: the reader opens an older schema (reddens «older»);
// the operator's door loses the sentinel on a newer one (reddens «newer»).
func TestShape_theDoorsNameEveryShapeTheyRefuse(t *testing.T) {
	ctx := context.Background()
	t.Run("an existing file with no action store", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "korvun.db")
		conv, err := convsqlite.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = conv.Close() }()
		for name, open := range map[string]func(string, string) (*Store, error){"OpenOperatorFor": OpenOperatorFor, "OpenReadOnlyFor": OpenReadOnlyFor} {
			h, err := open(path, profileA)
			if h != nil {
				_ = h.Close()
				t.Fatalf("%s handed out a handle on a file with no action store", name)
			}
			if !errors.Is(err, ErrNoActionStore) {
				t.Fatalf("%s on a file with no action store = %v, want ErrNoActionStore", name, err)
			}
		}
	})
	t.Run("an older schema", func(t *testing.T) {
		store, _ := foundedFor(t, profileA)
		raw := rawConn(t, store)
		rawExec(t, raw, `UPDATE action_schema SET version = 15`)
		_ = store.Close()
		for name, open := range map[string]func(string, string) (*Store, error){"OpenOperatorFor": OpenOperatorFor, "OpenReadOnlyFor": OpenReadOnlyFor} {
			h, err := open(store.path, profileA)
			if h != nil {
				_ = h.Close()
				t.Fatalf("%s handed out a handle on an older schema", name)
			}
			if !errors.Is(err, ErrSchemaBehind) {
				t.Fatalf("%s on an older schema = %v, want ErrSchemaBehind", name, err)
			}
		}
	})
	t.Run("a newer schema", func(t *testing.T) {
		store, _ := foundedFor(t, profileA)
		raw := rawConn(t, store)
		rawExec(t, raw, `UPDATE action_schema SET version = 17`)
		_ = store.Close()
		for name, open := range map[string]func(string, string) (*Store, error){"OpenOperatorFor": OpenOperatorFor, "OpenReadOnlyFor": OpenReadOnlyFor, "OpenFor (the owner, who would migrate)": OpenFor} {
			h, err := open(store.path, profileA)
			if h != nil {
				_ = h.Close()
				t.Fatalf("%s handed out a handle on a newer schema", name)
			}
			if !errors.Is(err, ErrSchemaFromTheFuture) {
				t.Fatalf("%s on a newer schema = %v, want ErrSchemaFromTheFuture", name, err)
			}
		}
	})
	t.Run("a shape asked through a closed pool is an error, not a verdict", func(t *testing.T) {
		store, _ := foundedFor(t, profileA)
		db, err := sql.Open("sqlite", "file:"+store.path)
		if err != nil {
			t.Fatal(err)
		}
		_ = db.Close()
		v, err := judgeShape(ctx, dbShapeQuerier{q: db})
		if err == nil {
			t.Fatalf("judgeShape through a closed pool = %v, want an error", v.shape)
		}
		if standing, _, jerr := store.judgeIn(ctx, db, profileA); standing != LedgerStandingUnreadable || jerr == nil || isVerdict(jerr) {
			t.Fatalf("judgeIn through a closed pool = %q %v, want unreadable with a failure that is not a verdict", standing, jerr)
		}
	})
}

// D16 · the third door of the verdict: a failure of the moment while
// refreshGuard judges (the hook of the connection it births fails once) is
// returned as an error and never written into the guard; the next
// connection judges afresh and writes.
//
// PROBING MUTATION: refreshGuard sets the guard unreadable on any error →
// the healthy ledger refuses every act until a new connection → reddens.
func TestGuard_refreshGuardReturnsATransientFailureInsteadOfAVerdict(t *testing.T) {
	ctx := context.Background()
	store, _ := foundedFor(t, profileA)
	store.db.SetConnMaxLifetime(time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	calls := 0
	fault := func() error {
		calls++
		if calls == 1 {
			return errors.New("injected: the catalog could not be read")
		}
		return nil
	}
	hookShapeFault.Store(&fault)
	t.Cleanup(func() { hookShapeFault.Store(nil) })
	err := store.refreshGuard(ctx)
	if err == nil || !strings.Contains(err.Error(), "injected") || isVerdict(err) {
		t.Fatalf("refreshGuard under a transient failure = %v, want that failure, not a verdict", err)
	}
	hookShapeFault.Store(nil)
	store.db.SetConnMaxLifetime(0)
	var row string
	if err := store.db.QueryRow(`SELECT standing FROM temp.profile_guard`).Scan(&row); err != nil || row != "ok" {
		t.Fatalf("guard row after the transient failure = %q (%v), want ok on the connection born after it", row, err)
	}
	if err := store.RecordAttempt(ctx, testEnvelope("act_after_transient"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized); err != nil {
		t.Fatalf("an act on the healthy ledger after the transient failure: %v", err)
	}
}

// D17 · SQLite resolves identifiers without regard to case, and so must the
// shape and the guard's belt: a table renamed to `Actions` is still the
// actions table — the shape stays current, the triggers land on it, and a
// door outside beginWrite dies by the standing's name.
//
// PROBING MUTATION: the belt matches names byte for byte → no trigger on the
// renamed table → the door lands → reddens.
func TestGuard_theBeltResolvesTableNamesLikeSQLite(t *testing.T) {
	ctx := context.Background()
	store, _ := foundedFor(t, profileA)
	raw := rawConn(t, store)
	rawExec(t, raw, `ALTER TABLE actions RENAME TO tmp_actions`)
	rawExec(t, raw, `ALTER TABLE tmp_actions RENAME TO Actions`)
	_ = store.Close()
	if v, err := judgeShape(ctx, dbShapeQuerier{q: raw}); err != nil || v.shape != shapeCurrent {
		t.Fatalf("the shape with actions renamed to Actions = %v %v, want current", v.shape, err)
	}
	b, err := OpenOperatorFor(store.path, profileB)
	if err != nil {
		t.Fatalf("open B: %v", err)
	}
	defer func() { _ = b.Close() }()
	var row string
	if err := b.db.QueryRow(`SELECT standing FROM temp.profile_guard`).Scan(&row); err != nil || row != string(LedgerStandingForeignProfile) {
		t.Fatalf("B's guard row = %q (%v), want foreign", row, err)
	}
	var n int
	if err := b.db.QueryRow(`SELECT COUNT(*) FROM sqlite_temp_master WHERE type = 'trigger' AND name LIKE 'korvun_guard_actions_%'`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("triggers on the renamed actions table = %d (%v), want 3", n, err)
	}
	if err := b.unguardedDoorForTest(ctx, hookDoorInsert, "act_case"); !errors.Is(err, ErrLedgerForeignProfile) {
		t.Fatalf("the unguarded door on the renamed table = %v, want ErrLedgerForeignProfile", err)
	}
	if got := rawCount(t, raw, `SELECT COUNT(*) FROM Actions`); got != 1 {
		t.Fatalf("Actions rows = %d, want the founding act only", got)
	}
}

// D18 · a foreign ledger downgraded to an older schema and re-lifted by
// another profile keeps its row: the migration's seed never overwrites the
// state, and the lifter finds the ledger foreign, not dead by driver text.
//
// PROBING MUTATION: the seed inserts without asking → UNIQUE constraint →
// the open dies → reddens.
func TestMigrate_theSeedKeepsARowAlreadyThere(t *testing.T) {
	ctx := context.Background()
	store, _ := foundedFor(t, profileB)
	raw := rawConn(t, store)
	rawExec(t, raw, `UPDATE action_schema SET version = 15`)
	_ = store.Close()
	a, err := OpenFor(store.path, profileA)
	if err != nil {
		t.Fatalf("OpenFor by A on B's downgraded ledger = %v, want a handle (the migration keeps the row)", err)
	}
	defer func() { _ = a.Close() }()
	if standing, owner, err := a.Standing(ctx); err != nil || standing != LedgerStandingForeignProfile || owner != profileB {
		t.Fatalf("after the re-lift A sees %q %q %v, want foreign owned by B", standing, owner, err)
	}
}

// D19 · a file that wears one of the schema's names on an object of another
// kind or case — a table `Actions`, a view `actions` — is not fresh: the
// bootstrap would trip on it. It is a bad shape: the internal opener refuses
// by name, the profile's opener opens it unreadable and repairs nothing.
//
// PROBING MUTATION: the shape counts tables only, byte for byte → fresh →
// the bootstrap dies by driver text → reddens.
func TestShape_aHomonymousObjectIsNotAFreshFile(t *testing.T) {
	for name, ddl := range map[string]string{"a table in another case": `CREATE TABLE Actions (x INTEGER)`, "a view": `CREATE VIEW actions AS SELECT 1`} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "odd.db")
			raw, err := sql.Open("sqlite", "file:"+path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = raw.Close() }()
			rawExec(t, raw, ddl)
			if v, err := judgeShape(context.Background(), dbShapeQuerier{q: raw}); err != nil || v.shape != shapeBad {
				t.Fatalf("the shape = %v %v, want bad", v.shape, err)
			}
			if s, err := open(path); !errors.Is(err, ErrLedgerUnreadable) {
				if s != nil {
					_ = s.Close()
				}
				t.Fatalf("the internal opener = %v, want ErrLedgerUnreadable", err)
			}
			before := catalogOf(t, raw)
			h, err := OpenFor(path, profileA)
			if err != nil {
				t.Fatalf("OpenFor = %v, want an open with the guard unreadable", err)
			}
			defer func() { _ = h.Close() }()
			if standing, _, err := h.Standing(context.Background()); standing != LedgerStandingUnreadable || !errors.Is(err, ErrLedgerUnreadable) {
				t.Fatalf("Standing = %q %v, want unreadable", standing, err)
			}
			if after := catalogOf(t, raw); !reflect.DeepEqual(before, after) {
				t.Fatalf("the opener changed the file: %d objects before, %d after", len(before), len(after))
			}
		})
	}
}

// D20 · a failure to judge the standing during OpenFor is the open's error,
// never swallowed into «no prune»: the seam expires the pool's connection
// and makes the hook of the next one fail, so Standing fails without a
// verdict.
//
// PROBING MUTATION: OpenFor reads any error as «skip the prune» → a handle
// comes back → reddens.
func TestOpen_aFailureToJudgeTheStandingIsTheOpensError(t *testing.T) {
	store, _ := foundedFor(t, profileA)
	path := store.path
	_ = store.Close()
	fault := func() error { return errors.New("injected: the catalog could not be read") }
	seam := func() {
		// The judgement obtains ONE connection for all its reads (train E,
		// plan §13.6b) and the pool's lifetime here is 1 ms: wait past it, so
		// the connection the judgement obtains is a NEW one, born through the
		// hook that then fails once.
		time.Sleep(30 * time.Millisecond)
		hookShapeFault.Store(&fault)
	}
	openStandingSeam.Store(&seam)
	t.Cleanup(func() { openStandingSeam.Store(nil); hookShapeFault.Store(nil) })
	// The pool's first connection is born in openWithIdentity (the shape
	// judgement); the seam runs after installGuard, so the fault must reach
	// a NEW connection: expire the one the pool has through its lifetime.
	prev := poolLifetimeForTest.Load()
	lifetime := time.Millisecond
	poolLifetimeForTest.Store(&lifetime)
	t.Cleanup(func() { poolLifetimeForTest.Store(prev) })
	h, err := OpenFor(path, profileA)
	if err == nil || !strings.Contains(err.Error(), "injected") || isVerdict(err) {
		if h != nil {
			_ = h.Close()
		}
		t.Fatalf("OpenFor while the standing cannot be judged = %v, want the failure as the open's error", err)
	}
}

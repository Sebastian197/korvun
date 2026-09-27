// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Train E, batch 2 — the SEED (plan v3, §6 rows TE01, TE04, TE06, TE07 and
// TE57, §7 «Locked seed» and «Crash, race and impossible-write oracles»):
// the five bootstrap statements and the version row commit in ONE immediate
// transaction whose own judgement of the file, inside the lock, decides.
//
// Evidence level, per mould: TE01 and TE04 — crash and race in SEPARATE OS
// processes (this test binary re-executed), the file read raw before anything
// reopens it; TE57 — its schedules in separate processes, its read-failure
// grid in process over multiple real connections with synthetic coded faults;
// TE06 — in process, a real SQLITE_FULL from a page cap on the seed's own
// connection; TE07 — in process, real files.

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	msqlite "modernc.org/sqlite"
)

// e2SeedPoints are the seed's named points in their order.
var e2SeedPoints = []string{"seed:ddl#1", "seed:ddl#2", "seed:ddl#3", "seed:ddl#4", "seed:ddl#5", "seed:insert", "seed:before-commit", "seed:after-commit"}

// TE01 · a process that dies INSIDE the seed leaves nothing of it. A writer
// killed (exit 9, no defers run) right after any bootstrap statement, right
// after the version row or right before the commit leaves the file exactly as
// it found it — read raw BEFORE anything reopens it — and the next OpenFor
// seeds, lifts to v16 and records. A writer killed right after the commit
// leaves the whole v1 seed, which the next open lifts. Over a file that
// already carried a historical prefix the same holds: the prefix is still
// exactly there after the crash, and the next open completes it. The
// conversation store's rows and DDL are untouched throughout.
//
// PROBING MUTATION (MU01): send one bootstrap DDL or the version INSERT
// outside the seed's transaction → the raw read before reopening finds it →
// reddens.
func TestE2_TE01_aCrashInsideTheSeedLeavesNothingOfIt(t *testing.T) {
	t.Parallel()
	for _, residue := range []int{0, 2} {
		for _, point := range e2SeedPoints {
			residue, point := residue, point
			t.Run(fmt.Sprintf("residue%d/%s", residue, point), func(t *testing.T) {
				t.Parallel()
				path := e2ConversationFile(t)
				if residue > 0 {
					e2Prefix(t, path, residue)
				}
				traps := e2ArmTraps(t, path)
				before := e2Snapshot(t, path)
				c := e2StartChild(t, e2ChildSpec{Path: path, Profile: profileA, Crash: point})
				code, lines := c.finish(t, time.Minute)
				acks, done := e2Acks(lines)
				if code != 9 {
					report := "no report"
					if done != nil {
						report = done.text
					}
					t.Fatalf("the writer did not die at %s: exit %d, %s\nacknowledged %v", point, code, report, acks)
				}
				if len(acks) == 0 || !strings.HasPrefix(acks[len(acks)-1], point+" ") {
					t.Fatalf("the writer died after %v, want %s last", acks, point)
				}
				after := e2Snapshot(t, path) // RAW, before anything reopens the file
				if point == "seed:after-commit" {
					for _, k := range e2BootstrapObjects {
						if _, ok := after.objects[k]; !ok {
							t.Fatalf("after the commit the seed's %s is missing", k)
						}
					}
					raw := e2Raw(t, path)
					var version int
					if n := rawCount(t, raw, `SELECT COUNT(*) FROM action_schema`); n != 1 {
						t.Fatalf("after the commit action_schema holds %d rows, want 1", n)
					}
					if err := raw.QueryRow(`SELECT version FROM action_schema`).Scan(&version); err != nil || version != 1 {
						t.Fatalf("after the commit the version is %d (%v), want 1", version, err)
					}
					e2Same(t, "the conversation store after the commit", before.conversation(), after.conversation())
				} else {
					e2Same(t, "the raw file before reopening", before, after)
				}
				e2Healthy(t, "the restart", path)
				e2Same(t, "the conversation store after the restart", before.conversation(), e2Snapshot(t, path).conversation())
				if n := traps.hits(); n != 0 {
					t.Fatalf("%d conversation writes were attempted", n)
				}
				// Instrumentation, after the behaviour: every point inside the
				// seed was acknowledged inside its transaction.
				if point != "seed:after-commit" && !strings.HasSuffix(acks[len(acks)-1], "tx=true") {
					t.Fatalf("the point %s was acknowledged outside a transaction: %v", point, acks)
				}
			})
		}
	}
}

// e2ReferenceObjects is the action store's catalog as a fresh OpenFor writes
// it over a conversation file: every object that is not the conversation
// store's, with its DDL.
func e2ReferenceObjects(t *testing.T) map[string]string {
	t.Helper()
	path := e2ConversationFile(t)
	h, err := OpenFor(path, profileA)
	if err != nil {
		t.Fatalf("the reference store: %v", err)
	}
	_ = h.Close()
	return e2ActionObjects(e2Snapshot(t, path))
}

// e2ActionObjects are the objects of a state that the conversation store
// does not own.
func e2ActionObjects(s e2State) map[string]string {
	conv := s.conversation().objects
	out := map[string]string{}
	for k, v := range s.objects {
		if _, ok := conv[k]; !ok {
			out[k] = v
		}
	}
	return out
}

func e2SameObjects(t *testing.T, label string, want, got map[string]string) {
	t.Helper()
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s: %s is %q, want %q", label, k, got[k], v)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Fatalf("%s: %s is not the fresh store's", label, k)
		}
	}
}

// TE04 · two writers that both judged the file fresh. The seed judges the
// file again INSIDE its immediate transaction, so the writer that takes the
// lock second finds the first one's schema and sends no bootstrap statement
// at all. A and B are separate processes, both held at «observed fresh»; A
// is released, commits its seed and finishes its open; only then B takes
// the lock. Both opens succeed and both record; one version row, at 16; A
// dispatched exactly the five DDL and one version INSERT, B none — counted
// where the store dispatches them, so an idempotent statement cannot hide.
//
// PROBING MUTATION (MU04): remove or ignore the locked rejudge → B dispatches
// the bootstrap → its counters are not zero → reddens.
func TestE2_TE04_theSecondSeedFindsTheFirstsSchemaUnderTheLock(t *testing.T) {
	t.Parallel()
	want := e2ReferenceObjects(t)
	path := e2ConversationFile(t)
	traps := e2ArmTraps(t, path)
	before := e2Snapshot(t, path)
	a := e2StartChild(t, e2ChildSpec{Path: path, Profile: profileA, Barriers: []string{"seed:observed-fresh"}, Write: true})
	b := e2StartChild(t, e2ChildSpec{Path: path, Profile: profileA, Barriers: []string{"seed:observed-fresh"}, Write: true})
	a.until(t, "E2ACK seed:observed-fresh", time.Minute)
	b.until(t, "E2ACK seed:observed-fresh", time.Minute)
	// Both judged the file fresh. A goes first: its seed's commit is
	// acknowledged and its open ends.
	a.release(t, "seed:observed-fresh")
	a.until(t, "E2ACK seed:after-commit", time.Minute)
	aCode, aLines := a.finish(t, time.Minute)
	_, aDone := e2Acks(aLines)
	if aCode != 0 || aDone == nil {
		t.Fatalf("A ended with exit %d and no report\n%s", aCode, a.said())
	}
	ar := e2ReportOf(t, *aDone)
	// Only now B takes the lock.
	b.release(t, "seed:observed-fresh")
	bCode, bLines := b.finish(t, time.Minute)
	bAcks, bDone := e2Acks(bLines)
	if bCode != 0 || bDone == nil {
		t.Fatalf("B ended with exit %d and no report\n%s", bCode, b.said())
	}
	br := e2ReportOf(t, *bDone)
	if !ar.Opened || ar.Write != "" {
		t.Fatalf("A = %+v, want opened and recorded", ar)
	}
	if !br.Opened || br.Write != "" {
		t.Fatalf("B = %+v, want opened and recorded", br)
	}
	if ar.DDL != 5 || ar.Inserts != 1 {
		t.Fatalf("A dispatched %d bootstrap DDL and %d version INSERT, want 5 and 1", ar.DDL, ar.Inserts)
	}
	if br.DDL != 0 || br.Inserts != 0 {
		t.Fatalf("B, which found A's schema under the lock, dispatched %d bootstrap DDL and %d version INSERT, want none", br.DDL, br.Inserts)
	}
	after := e2Snapshot(t, path)
	e2SameObjects(t, "the schema after both opens", want, e2ActionObjects(after))
	raw := e2Raw(t, path)
	var version int
	if n := rawCount(t, raw, `SELECT COUNT(*) FROM action_schema`); n != 1 {
		t.Fatalf("action_schema holds %d rows, want 1", n)
	}
	if err := raw.QueryRow(`SELECT version FROM action_schema`).Scan(&version); err != nil || version != schemaVersionCurrent {
		t.Fatalf("the version is %d (%v), want %d", version, err, schemaVersionCurrent)
	}
	if n := rawCount(t, raw, `SELECT COUNT(*) FROM actions`); n != 2 {
		t.Fatalf("actions holds %d rows, want the two acts", n)
	}
	e2Same(t, "the conversation store", before.conversation(), after.conversation())
	if traps.hits()+ar.Traps+br.Traps != 0 {
		t.Fatalf("conversation writes were attempted: here %d, A %d, B %d", traps.hits(), ar.Traps, br.Traps)
	}
	// Instrumentation, after the behaviour: B took the lock and judged, and
	// never began the bootstrap.
	if !e2Has(bAcks, "seed:locked") || !e2Has(bAcks, "seed:rejudged") || e2Has(bAcks, "seed:begin") {
		t.Fatalf("B acknowledged %v, want the lock and the rejudge and no bootstrap", bAcks)
	}
}

// e2Has reports whether an acknowledged point starts the list's entries.
func e2Has(acks []string, point string) bool {
	for _, a := range acks {
		if a == point || strings.HasPrefix(a, point+" ") {
			return true
		}
	}
	return false
}

// TE06 · a disk that fills INSIDE the seed. The page cap is set on the seed's
// own connection, one page above the file's size, before its first statement
// grows the file: the first bootstrap table fits, the second meets a real
// SQLITE_FULL (13). OpenFor hands out nothing and names ledger_environment,
// with the native code in the chain; nothing of the seed persists — the raw
// file is the one it found — and the conversations are untouched. The next
// open, with no cap, seeds.
//
// PROBING MUTATIONS (MU06): classify FULL as unreadable → reddens; split the
// seed's transaction → the first table persists → reddens.
func TestE2_TE06_aFullDiskInsideTheSeedIsTheEnvironmentAndLeavesNothing(t *testing.T) {
	path := e2ConversationFile(t)
	traps := e2ArmTraps(t, path)
	before := e2Snapshot(t, path)
	var pages, capAt int
	var capErr error
	o := e2WatchSeed(t, path, func(point string, on seedConn) {
		if point != "seed:begin" {
			return
		}
		ctx := context.Background()
		if capErr = on.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&pages); capErr != nil {
			return
		}
		capErr = on.QueryRowContext(ctx, fmt.Sprintf(`PRAGMA max_page_count = %d`, pages+1)).Scan(&capAt)
	})
	h, err := OpenFor(path, profileA)
	if h != nil {
		_ = h.Close()
		t.Fatal("OpenFor handed out a handle over a seed the disk refused")
	}
	if !errors.Is(err, ErrLedgerEnvironment) || errors.Is(err, ErrLedgerUnreadable) || !strings.Contains(err.Error(), "ledger_environment") {
		t.Fatalf("OpenFor over a full disk = %v, want ErrLedgerEnvironment (ledger_environment), never unreadable", err)
	}
	var native *msqlite.Error
	if !errors.As(err, &native) || native.Code()&0xff != 13 {
		t.Fatalf("OpenFor over a full disk = %v, want the native FULL (13) in the chain", err)
	}
	e2Same(t, "the raw file after the refused seed", before, e2Snapshot(t, path))
	if n := traps.hits(); n != 0 {
		t.Fatalf("%d conversation writes were attempted", n)
	}
	// The cap and the first statement it stopped, observed.
	if capErr != nil || pages == 0 || capAt != pages+1 {
		t.Fatalf("the page cap: pages %d, cap %d, %v", pages, capAt, capErr)
	}
	seen := o.seen()
	if len(seen.failures) != 1 || seen.failures[0].statement != 2 {
		t.Fatalf("the seed's failed statements = %+v, want the second bootstrap DDL alone", seen.failures)
	}
	if !errors.As(seen.failures[0].err, &native) || native.Code()&0xff != 13 {
		t.Fatalf("the second bootstrap DDL failed with %v, want FULL (13)", seen.failures[0].err)
	}
	seedObserverSeam.CompareAndSwap(o, nil)
	e2Healthy(t, "the next open, with no cap", path)
}

// TE07 · a second open never seeds again and never overwrites the owner. A
// current ledger reopened by its owner: no bootstrap statement dispatched,
// one version row, the owner as it was. A ledger at v15 whose identity row
// already names an owner the receipts' mark does not: the migration's
// identity copy keeps the row as it finds it (retained D18), so the lifter
// finds the ledger foreign, owned by the row's owner.
//
// PROBING MUTATIONS (MU07): the version INSERT sent on every open without
// its guard → a second version row → reddens; the identity copy upserts →
// the row takes the mark's owner → reddens.
func TestE2_TE07_aSecondOpenNeverReseedsNorOverwritesTheOwner(t *testing.T) {
	ctx := context.Background()
	t.Run("a current ledger reopened by its owner", func(t *testing.T) {
		path, _ := e1Founded(t)
		o := e2WatchSeed(t, path, nil)
		h, err := OpenFor(path, profileA)
		if err != nil {
			t.Fatalf("OpenFor on the owner's current ledger = %v", err)
		}
		st, owner, serr := h.Standing(ctx)
		_ = h.Close()
		if serr != nil || st != LedgerStandingOK || owner != profileA {
			t.Fatalf("Standing = %q %q %v, want ok owned by A", st, owner, serr)
		}
		raw := e2Raw(t, path)
		if n := rawCount(t, raw, `SELECT COUNT(*) FROM action_schema`); n != 1 {
			t.Fatalf("action_schema holds %d rows after the reopen, want 1", n)
		}
		var stored string
		if err := raw.QueryRow(`SELECT owner_digest FROM ledger_identity WHERE id = 1`).Scan(&stored); err != nil || stored != profileA {
			t.Fatalf("the identity row names %q (%v), want A", stored, err)
		}
		if seen := o.seen(); seen.ddl != 0 || seen.inserts != 0 {
			t.Fatalf("the reopen dispatched %d bootstrap DDL and %d version INSERT, want none", seen.ddl, seen.inserts)
		}
	})
	t.Run("an owner row already there before the identity copy", func(t *testing.T) {
		store, _ := foundedFor(t, profileB)
		raw := rawConn(t, store)
		_ = store.Close()
		// The row names a third profile; the receipts' mark still names B.
		rawExec(t, raw, `UPDATE ledger_identity SET owner_digest = ? WHERE id = 1`, profileC)
		rawExec(t, raw, `UPDATE action_schema SET version = 15`)
		o := e2WatchSeed(t, store.path, nil)
		h, err := OpenFor(store.path, profileA)
		if err != nil {
			t.Fatalf("OpenFor by A on the v15 ledger = %v, want a handle (the migration keeps the row)", err)
		}
		st, owner, serr := h.Standing(ctx)
		_ = h.Close()
		if serr != nil || st != LedgerStandingForeignProfile || owner != profileC {
			t.Fatalf("after the lift A sees %q %q %v, want foreign owned by the row's owner", st, owner, serr)
		}
		if n := rawCount(t, raw, `SELECT COUNT(*) FROM ledger_identity`); n != 1 {
			t.Fatalf("ledger_identity holds %d rows, want 1", n)
		}
		var stored string
		if err := raw.QueryRow(`SELECT owner_digest FROM ledger_identity WHERE id = 1`).Scan(&stored); err != nil || stored != profileC {
			t.Fatalf("the identity row names %q (%v), want the owner it had", stored, err)
		}
		if n := rawCount(t, raw, `SELECT COUNT(*) FROM action_schema`); n != 1 {
			t.Fatalf("action_schema holds %d rows, want 1", n)
		}
		if seen := o.seen(); seen.ddl != 0 || seen.inserts != 0 {
			t.Fatalf("the lift dispatched %d bootstrap DDL and %d version INSERT, want none", seen.ddl, seen.inserts)
		}
	})
}

// e2Schedule is one of TE57's: what commits the file while B waits at
// «observed fresh», and the one outcome B must meet.
type e2Schedule struct {
	name string
	// commit changes the file while B waits; the func it returns, if any,
	// ends what commit started once B has finished.
	commit func(t *testing.T, path string) func()
	class  string // B's opener class: nil, future, unreadable
	reason string // a word the error names, when it fails
	seeds  bool   // B's seed dispatches the bootstrap
}

// e2Raw5 lays the five bootstrap statements and then extra in ONE raw
// transaction: an attacker's committed shape.
func e2Raw5(t *testing.T, path string, extra ...string) {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	tx, err := raw.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range append(e2Statements(t), extra...) {
		if _, err := tx.Exec(q); err != nil {
			_ = tx.Rollback()
			t.Fatalf("raw %q: %v", q, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

// TE57 · the seed's judgement inside its lock decides alone, through the
// transaction it holds — never through the pool, whose one connection that
// transaction holds. B judged the file fresh and is held there; A — another
// writer, or a raw attacker — commits the file into one shape; B is released,
// and from the moment it acknowledges the lock its judgement must answer
// within 2 s. Fresh: B seeds. Current: no seed, the normal route. Older: no
// seed, B migrates after the lock is released. Future: no seed,
// ErrSchemaFromTheFuture. Bad: no seed, a fatal ErrLedgerUnreadable carrying
// the reason. B writes nothing on any branch but the fresh one.
//
// PROBING MUTATIONS (MU57): route the rejudge through the pool → it waits on
// the connection its own transaction holds → the 2 s deadline fails →
// reddens; ignore a future or bad rejudge → B seeds → reddens; delete the
// rejudge → B seeds → reddens.
func TestE2_TE57_theLockedRejudgeDecidesAlone(t *testing.T) {
	schedules := []e2Schedule{
		{name: "fresh", class: "nil", seeds: true},
		{name: "current", class: "nil", commit: func(t *testing.T, path string) func() {
			a := e2StartChild(t, e2ChildSpec{Path: path, Profile: profileA, Write: true})
			code, lines := a.finish(t, time.Minute)
			if _, done := e2Acks(lines); code != 0 || done == nil || !e2ReportOf(t, *done).Opened {
				t.Fatalf("A did not open the file: exit %d\n%s", code, a.said())
			}
			return nil
		}},
		{name: "older", class: "nil", commit: func(t *testing.T, path string) func() {
			a := e2StartChild(t, e2ChildSpec{Path: path, Profile: profileA, Barriers: []string{"seed:after-commit"}, Write: true})
			a.until(t, "E2ACK seed:after-commit", time.Minute)
			return func() {
				a.release(t, "seed:after-commit")
				code, lines := a.finish(t, time.Minute)
				_, done := e2Acks(lines)
				if code != 0 || done == nil {
					t.Fatalf("A did not finish after B: exit %d\n%s", code, a.said())
				}
				if r := e2ReportOf(t, *done); !r.Opened || r.Write != "" || r.DDL != 5 || r.Inserts != 1 {
					t.Fatalf("A after B = %+v, want opened with its own seed", r)
				}
			}
		}},
		{name: "future", class: "future", reason: "17", commit: func(t *testing.T, path string) func() {
			e2Raw5(t, path, `INSERT INTO action_schema (version) VALUES (17)`)
			return nil
		}},
		{name: "bad", class: "unreadable", reason: "2 rows", commit: func(t *testing.T, path string) func() {
			e2Raw5(t, path, `INSERT INTO action_schema (version) VALUES (1)`, `INSERT INTO action_schema (version) VALUES (1)`)
			return nil
		}},
	}
	for _, s := range schedules {
		s := s
		t.Run(s.name, func(t *testing.T) {
			path := e2ConversationFile(t)
			traps := e2ArmTraps(t, path)
			before := e2Snapshot(t, path)
			b := e2StartChild(t, e2ChildSpec{Path: path, Profile: profileA, Barriers: []string{"seed:observed-fresh"}, Write: true})
			b.until(t, "E2ACK seed:observed-fresh", time.Minute)
			var resume func()
			if s.commit != nil {
				resume = s.commit(t, path)
			}
			met := e2Snapshot(t, path)
			b.release(t, "seed:observed-fresh")
			// From the acknowledged lock, the rejudge answers within 2 s.
			var acks []string
			var done *e2Line
			var lockedAt time.Time
			for {
				wait := time.Minute
				if !lockedAt.IsZero() && !e2Has(acks, "seed:rejudged") {
					wait = time.Until(lockedAt.Add(2 * time.Second))
				}
				l, ok, late := b.nextWithin(wait)
				if late {
					_ = b.cmd.Process.Kill()
					t.Fatalf("B's judgement inside its lock did not answer within 2 s of the acknowledged lock\n%s", b.said())
				}
				if !ok {
					break
				}
				switch {
				case strings.HasPrefix(l.text, "E2ACK "):
					acks = append(acks, strings.TrimPrefix(l.text, "E2ACK "))
					if strings.HasPrefix(l.text, "E2ACK seed:locked") {
						lockedAt = l.at
					}
				case strings.HasPrefix(l.text, "E2DONE "):
					l := l
					done = &l
				}
			}
			code, _ := b.finish(t, time.Minute)
			if resume != nil {
				resume()
			}
			if code != 0 || done == nil {
				t.Fatalf("B ended with exit %d and no report\n%s", code, b.said())
			}
			r := e2ReportOf(t, *done)
			if r.Class != s.class {
				t.Fatalf("B = %+v, want the class %s", r, s.class)
			}
			if s.reason != "" && !strings.Contains(r.Err, s.reason) {
				t.Fatalf("B's error %q does not name %q", r.Err, s.reason)
			}
			if s.class == "nil" && (!r.Opened || r.Write != "") {
				t.Fatalf("B = %+v, want opened and recorded", r)
			}
			switch {
			case s.seeds && (r.DDL != 5 || r.Inserts != 1):
				t.Fatalf("B on a fresh file dispatched %d DDL and %d INSERT, want 5 and 1", r.DDL, r.Inserts)
			case !s.seeds && (r.DDL != 0 || r.Inserts != 0):
				t.Fatalf("B dispatched %d bootstrap DDL and %d version INSERT over a %s file, want none", r.DDL, r.Inserts, s.name)
			}
			if s.class != "nil" {
				e2Same(t, "the file B refused", met, e2Snapshot(t, path))
			}
			e2Same(t, "the conversation store", before.conversation(), e2Snapshot(t, path).conversation())
			if traps.hits()+r.Traps != 0 {
				t.Fatalf("conversation writes were attempted: here %d, B %d", traps.hits(), r.Traps)
			}
			// Instrumentation, after the behaviour: B acknowledged its lock and
			// its judgement inside it, and began the bootstrap only on a fresh file.
			if !e2Has(acks, "seed:locked") || !e2Has(acks, "seed:rejudged") || e2Has(acks, "seed:begin") != s.seeds {
				t.Fatalf("B acknowledged %v", acks)
			}
		})
	}
}

// nextWithin is next without failing: late is true when d passed first.
func (c *e2Child) nextWithin(d time.Duration) (l e2Line, ok, late bool) {
	if d <= 0 {
		d = time.Millisecond
	}
	select {
	case l, ok := <-c.lines:
		return l, ok, false
	case <-time.After(d):
		return e2Line{}, false, true
	}
}

// TE57 (the grid) · a failed read of the locked judgement keeps its class and
// its cause, and seeds nothing: every read the judgement makes through its
// transaction — the catalog and the residue on a file carrying an empty
// prefix, the version and the UNIQUE indexes on a file another writer lifted
// to v16 while B waited — at each stage, with a structural, a busy, a locked,
// an environment, an uncategorised and an uncoded fault. A structural one is
// the fatal unreadable of the seed's race; the others are their class, or
// themselves; no handle, no bootstrap statement, the file as B met it; the
// next open is healthy.
//
// PROBING MUTATIONS (MU57): lose the class of the locked read → reddens;
// seed on an unknown shape → reddens.
func TestE2_TE57_aFailedLockedReadKeepsItsClassAndSeedsNothing(t *testing.T) {
	kinds := append(append(append(append(codedKinds(classStructural, 11), codedKinds(classBusy, 5, 6)...), codedKinds(classEnvironment, 10)...), codedKinds(classNone, 2)...),
		e1Kind{name: "uncoded", make: func() error { return errors.New("injected uncoded fault (grid)") }, class: classNone})
	type spot struct {
		site    judgeQuerySite
		current bool
	}
	for _, sp := range []spot{{siteCatalog, false}, {siteResidue, false}, {siteVersion, true}, {siteIndex, true}} {
		for _, stage := range e1DBStages {
			for _, kind := range kinds {
				sp, stage, kind := sp, stage, kind
				label := fmt.Sprintf("%s/%s/%s", sp.site, stage, kind.name)
				t.Run(label, func(t *testing.T) {
					path := e2ConversationFile(t)
					if !sp.current {
						e2Prefix(t, path, 2)
					}
					gate := newE2Gate("seed:observed-fresh")
					o := e2WatchSeed(t, path, gate.at)
					type result struct {
						h   *Store
						err error
					}
					done := make(chan result, 1)
					go func() {
						h, err := OpenFor(path, profileA)
						done <- result{h, err}
					}()
					select {
					case <-gate.reached:
					case r := <-done:
						t.Fatalf("B ended (%v) before it judged the file fresh", r.err)
					case <-time.After(time.Minute):
						t.Fatal("B never judged the file fresh")
					}
					if sp.current {
						// A, a second writer, seeds and lifts the file while B waits.
						a, err := OpenFor(path, profileA)
						if err != nil {
							t.Fatalf("A: %v", err)
						}
						_ = a.Close()
					}
					met := e2Snapshot(t, path)
					sent := o.seen()
					fault := kind.make()
					f := armJudgeFault(t, path, originSeedLockedRejudge, sp.site, stage, fault)
					close(gate.release)
					var r result
					select {
					case r = <-done:
					case <-time.After(10 * time.Second):
						t.Fatal("B's open did not return within 10 s of its release")
					}
					rcpt := f.disarm()
					if r.h != nil {
						_ = r.h.Close()
						t.Fatalf("%s: B handed out a handle over a failed judgement inside its lock", label)
					}
					if !errors.Is(r.err, fault) {
						t.Fatalf("%s: B = %v, want the fault in the chain", label, r.err)
					}
					if kind.class == classNone {
						if !namesNoClass(r.err) {
							t.Fatalf("%s: B = %v, want the fault unnamed", label, r.err)
						}
					} else if want := classSentinel(kind.class); !errors.Is(r.err, want) {
						t.Fatalf("%s: B = %v, want %v", label, r.err, want)
					}
					if now := o.seen(); now.ddl != sent.ddl || now.inserts != sent.inserts {
						t.Fatalf("%s: B dispatched %d bootstrap DDL and %d version INSERT, want none", label, now.ddl-sent.ddl, now.inserts-sent.inserts)
					}
					e2Same(t, label, met, e2Snapshot(t, path))
					checkReceipt(t, label, rcpt, sp.site, kind.class == classStructural, kind.class)
					seedObserverSeam.CompareAndSwap(o, nil)
					e2Healthy(t, label+": the next open", path)
				})
			}
		}
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Train E, batch 3 (GE6) — the migration: TE33–TE35, TE38 and TE39 (plan v3,
// §4 «Migration», §§5–7 and §8). The step is the transaction boundary: a step
// reads the version stored NOW, inside its own transaction and before any of
// its writes, and lifts only that version; its bump changes exactly the row
// it reread; a step that fails leaves its version and data as they were. The
// step itself names unreadable only what it found in the ledger — a stored
// version it cannot lift, a bump left with no row to change, a tombstone the
// typed judge refuses, a constraint the copied rows break — and every other
// failure reaches the public door as it is, to be classed there by its code.

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TE33 · a migrator that read an old version before another committed the
// whole lift does no stale work: its step rereads the stored version inside
// its own transaction, finds the other's commit, skips, and its open
// converges on the one v16 row the other committed — both handles live.
//
// PROBING MUTATION (MU33): remove the reread (the step trusts the outer
// loop's version) → A runs its stale step's script and bump → reddens.
//
// Evidence level: in process, two writer handles with their own native
// connections on one file; an acknowledged in-process barrier.
func TestE3_TE33_aStaleMigratorDoesNoWork(t *testing.T) {
	template := e3OldFile(t, 12)
	t.Run("alone, every step once", func(t *testing.T) {
		path := e3Copy(t, template)
		o := e3WatchMigration(t, path, nil)
		h, err := OpenFor(path, profileA)
		if err != nil {
			t.Fatalf("OpenFor over v12 = %v, want the lift", err)
		}
		defer func() { _ = h.Close() }()
		if w := o.work(); w.ddl != 4 || w.bumps != 4 || w.skipped != 0 {
			t.Fatalf("a lone lift from v12 ran %d step script(s), %d bump(s) and skipped %d, want 4, 4 and 0", w.ddl, w.bumps, w.skipped)
		}
		e3WantVersion(t, "the lone lift", path, 16)
	})
	for _, hold := range []int{12, 15} {
		t.Run(fmt.Sprintf("stale at v%d", hold), func(t *testing.T) {
			path := e3Copy(t, template)
			gate := newE3Gate(fmt.Sprintf("migrate:read#%d", hold))
			o := e3WatchMigration(t, path, gate.at)
			aDone := e3OpenAsync(t, path, gate)
			select {
			case <-gate.reached:
			case r := <-aDone:
				t.Fatalf("A ended (%v) before its read of v%d", r.err, hold)
			case <-time.After(60 * time.Second):
				t.Fatalf("A never read v%d", hold)
			}
			// A is held after its outer read of v<hold>, before its step's
			// Begin; B lifts the file to v16 and commits.
			b, err := OpenFor(path, profileA)
			if err != nil {
				t.Fatalf("B, the second migrator, = %v, want the lift", err)
			}
			defer func() { _ = b.Close() }()
			seen := o.work()
			if !slices.Contains(seen.points, "migrate:after-commit#15") {
				t.Fatalf("B's commit of v16 was never acknowledged: %v", seen.points)
			}
			e3WantVersion(t, "B's commit", path, 16)
			committed := e2Snapshot(t, path)
			gate.open()
			var a e3Open
			select {
			case a = <-aDone:
			case <-time.After(120 * time.Second):
				t.Fatal("A never answered once released")
			}
			after := o.work()
			if ddl, bumps := after.ddl-seen.ddl, after.bumps-seen.bumps; ddl != 0 || bumps != 0 {
				t.Fatalf("A, stale at v%d, ran %d step script(s) and %d version bump(s) after B committed v16, want none", hold, ddl, bumps)
			}
			if a.err != nil {
				t.Fatalf("A = %v, want its open to converge on B's v16", a.err)
			}
			if skipped := after.skipped - seen.skipped; skipped != 1 {
				t.Fatalf("A skipped %d stale step(s), want exactly its one", skipped)
			}
			e3WantVersion(t, "after A", path, 16)
			e2Same(t, "B's committed evidence under A's open", committed, e2Snapshot(t, path))
			for who, h := range map[string]*Store{"A": a.h, "B": b} {
				if err := guardedWrite(h, "te33_"+strings.ToLower(who)); err != nil {
					t.Fatalf("a guarded write through %s = %v, want both handles live", who, err)
				}
			}
		})
	}
}

// TE33, the reread's other answers · a migrator held after its outer read of
// v12 while a second real connection changes what is stored decides on what
// is stored: a lower version, a version that is not a number and a second
// version row are unreadable, named; a version above this binary's is
// ErrSchemaFromTheFuture. No step script and no bump run, and the file keeps
// what the second connection committed.
//
// PROBING MUTATION (MU33): remove the reread → the step runs over the stale
// read → reddens.
//
// Evidence level: in process, native connections, an acknowledged barrier
// and a second real connection that commits before the release.
func TestE3_TE33_theRereadDecidesOnWhatIsStored(t *testing.T) {
	template := e3OldFile(t, 12)
	cases := []struct {
		name, change, names string
		future              bool
	}{
		{"a lower version", `UPDATE action_schema SET version = 11`, "reread v11", false},
		{"a version that is not a number", `UPDATE action_schema SET version = 'twelve'`, `"twelve"`, false},
		{"a second version row", `INSERT INTO action_schema (version) VALUES (12)`, "reread 2 version rows", false},
		{"a version from the future", `UPDATE action_schema SET version = 99`, "reread v99", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := e3Copy(t, template)
			gate := newE3Gate("migrate:read#12")
			o := e3WatchMigration(t, path, gate.at)
			aDone := e3OpenAsync(t, path, gate)
			select {
			case <-gate.reached:
			case r := <-aDone:
				t.Fatalf("A ended (%v) before its read of v12", r.err)
			case <-time.After(60 * time.Second):
				t.Fatal("A never read v12")
			}
			e2Lay(t, path, c.change)
			changed := e2Snapshot(t, path)
			gate.open()
			var a e3Open
			select {
			case a = <-aDone:
			case <-time.After(120 * time.Second):
				t.Fatal("A never answered once released")
			}
			if w := o.work(); w.ddl != 0 || w.bumps != 0 {
				t.Fatalf("A ran %d step script(s) and %d version bump(s) over a version it read before %s was stored, want none", w.ddl, w.bumps, c.name)
			}
			if a.h != nil {
				t.Fatalf("OpenFor handed out a handle over %s", c.name)
			}
			want := ErrLedgerUnreadable
			if c.future {
				want = ErrSchemaFromTheFuture
			}
			if !errors.Is(a.err, want) || (c.future && errors.Is(a.err, ErrLedgerUnreadable)) {
				t.Fatalf("OpenFor over %s = %v, want %v", c.name, a.err, want)
			}
			if !strings.Contains(a.err.Error(), c.names) {
				t.Fatalf("OpenFor over %s = %v, want it to name %s", c.name, a.err, c.names)
			}
			e2Same(t, "the stored step", changed, e2Snapshot(t, path))
		})
	}
}

// TE33, the bump · the step's bump changes exactly the version row it reread:
// a copy whose own writes move the stored version (a trigger planted on the
// identity table the 15→16 copy seeds) leaves the bump nothing to change, and
// the step is unreadable, named, with every write it made rolled back — the
// version still 15, no identity row — twice.
//
// PROBING MUTATIONS (MU33): drop the bump's predicate → it overwrites the
// moved version with 16 and commits → reddens; drop its row count → it
// commits the copy under the moved version → reddens.
//
// Evidence level: in process, native connection; the lying dependency is a
// real trigger in the file.
func TestE3_TE33_theBumpOwnsTheVersionItReread(t *testing.T) {
	store, _ := foundedFor(t, profileA)
	raw := rawConn(t, store)
	rawExec(t, raw, `DELETE FROM ledger_identity`)
	rawExec(t, raw, `UPDATE action_schema SET version = 15`)
	rawExec(t, raw, `CREATE TRIGGER e3_moves_the_version AFTER INSERT ON ledger_identity BEGIN UPDATE action_schema SET version = 20; END`)
	path := e3Copy(t, store.path)
	before := e2Snapshot(t, path)
	for attempt := 1; attempt <= 2; attempt++ {
		h, err := OpenFor(path, profileA)
		if h != nil {
			_ = h.Close()
			t.Fatalf("attempt %d: OpenFor handed out a handle over a step whose copy moved the version it lifts", attempt)
		}
		if !errors.Is(err, ErrLedgerUnreadable) || !strings.Contains(err.Error(), "changed 0 version rows") {
			t.Fatalf("attempt %d: OpenFor = %v, want ErrLedgerUnreadable naming the bump that changed 0 version rows", attempt, err)
		}
		e3WantVersion(t, fmt.Sprintf("attempt %d", attempt), path, 15)
		e2Same(t, fmt.Sprintf("attempt %d: the refused step", attempt), before, e2Snapshot(t, path))
	}
}

// TE33, the two-process variant · child A, held after its outer read of v12,
// and child B, both entering the public opener: B lifts the file to v16 and
// acknowledges its last commit before A is released; A then runs no step
// script and no bump, skips its one stale step and opens on v16. Three
// rounds; every protocol line is logged with its time.
//
// PROBING MUTATION (MU33): remove the reread → A's report counts the stale
// step's script and bump → reddens.
//
// Evidence level: two processes (this test binary re-executed), real
// connections, acknowledged barriers.
func TestE3_TE33_twoProcessesMigrateOnce(t *testing.T) {
	t.Parallel()
	template := e3OldFile(t, 12)
	for round := 1; round <= 3; round++ {
		label := fmt.Sprintf("TE33 round %d", round)
		path := e3Copy(t, template)
		t0 := time.Now()
		a := e3StartChild(t, e3ChildSpec{Path: path, Profile: profileA, Barriers: []string{"migrate:read#12"}})
		held := a.until(t, "E2ACK migrate:read#12", 60*time.Second)
		e3Lines(t, label, "A", t0, held)
		b := e3StartChild(t, e3ChildSpec{Path: path, Profile: profileA})
		codeB, linesB := b.finish(t, 120*time.Second)
		e3Lines(t, label, "B", t0, linesB...)
		acksB, doneB := e2Acks(linesB)
		if codeB != 0 || doneB == nil {
			t.Fatalf("%s: B exited %d without its report\n%s", label, codeB, b.said())
		}
		repB := e3ReportOf(t, *doneB)
		if !repB.Opened || repB.Version != 16 || repB.DDL != 4 || repB.Bumps != 4 || repB.Skipped != 0 {
			t.Fatalf("%s: B = %+v, want the whole lift from v12: opened on v16 after 4 step scripts and 4 bumps", label, repB)
		}
		if !slices.Contains(acksB, "migrate:after-commit#15") {
			t.Fatalf("%s: B never acknowledged its commit of v16: %v", label, acksB)
		}
		e3WantVersion(t, label+": B's commit", path, 16)
		committed := e2Snapshot(t, path)
		e3Evidence(t, "%s parent: B acknowledged its commit of v16 and ended; releasing A", label)
		a.release(t, "migrate:read#12")
		codeA, linesA := a.finish(t, 120*time.Second)
		e3Lines(t, label, "A", t0, linesA...)
		_, doneA := e2Acks(linesA)
		if codeA != 0 || doneA == nil {
			t.Fatalf("%s: A exited %d without its report\n%s", label, codeA, a.said())
		}
		repA := e3ReportOf(t, *doneA)
		if repA.DDL != 0 || repA.Bumps != 0 {
			t.Fatalf("%s: A, stale at v12, ran %d step script(s) and %d version bump(s) after B committed v16, want none (%+v)", label, repA.DDL, repA.Bumps, repA)
		}
		if !repA.Opened || repA.Class != "nil" || repA.Version != 16 || repA.Skipped != 1 {
			t.Fatalf("%s: A = %+v, want opened on v16 after skipping its one stale step", label, repA)
		}
		e3WantVersion(t, label+": after A", path, 16)
		e2Same(t, label+": B's committed evidence under A's open", committed, e2Snapshot(t, path))
	}
}

// TE33, the stress replay (plan §7, S4/S5; Annex C, P3-3) · the original
// reproduction: two concurrent OpenFor on a fresh shared file — the
// conversation store's, with no action store in it, as the boot meets it —
// forty rounds, eighty opens. Every result — class, native code and the
// error, which names the phase — and every round's stored versions are
// logged; the replay accepts eighty successes, each round bounded. It is
// never the deterministic proof of TE33. A path that does not exist yet is
// not its fixture: there, two raw connections born together can already fail
// with SQLITE_BUSY, before either reaches the store.
//
// Evidence level: in process, two writer handles per round on one file.
func TestE3_TE33_theEightyOpenStressReplay(t *testing.T) {
	t.Parallel()
	type result struct {
		round, opener int
		err           error
	}
	var results []result
	versions := map[int]string{}
	failed := 0
	for round := 1; round <= 40; round++ {
		path := e2ConversationFile(t)
		start := make(chan struct{})
		out := make(chan result, 2)
		for opener := 1; opener <= 2; opener++ {
			go func() {
				<-start
				h, err := OpenFor(path, profileA)
				if h != nil {
					_ = h.Close()
				}
				out <- result{round: round, opener: opener, err: err}
			}()
		}
		close(start)
		for i := 0; i < 2; i++ {
			select {
			case r := <-out:
				if r.err != nil {
					failed++
				}
				results = append(results, r)
			case <-time.After(60 * time.Second):
				t.Fatalf("round %d: an open did not answer within 60 s", round)
			}
		}
		versions[round] = e3StoredVersions(path)
	}
	for _, r := range results {
		e3Evidence(t, "TE33-stress round %2d opener %d: class %-11s code %4d versions %s err %v", r.round, r.opener, e2Class(r.err), e3Code(r.err), versions[r.round], r.err)
	}
	e3Evidence(t, "TE33-stress: %d of %d opens succeeded", len(results)-failed, len(results))
	if failed != 0 {
		t.Fatalf("%d of the %d opens failed (every result is logged above)", failed, len(results))
	}
}

// e3StoredVersions reads the stored versions of path for a log line; a read
// that fails is named in the line, never fatal.
func e3StoredVersions(path string) string {
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return "unreadable: " + err.Error()
	}
	defer func() { _ = raw.Close() }()
	rows, err := raw.Query(`SELECT CAST(version AS TEXT) FROM action_schema`)
	if err != nil {
		return "unreadable: " + err.Error()
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return "unreadable: " + err.Error()
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return "unreadable: " + err.Error()
	}
	return fmt.Sprint(out)
}

// TE34 · a migration whose step cannot take the write lock — another process
// holds it past the writer's busy_timeout — is ErrLedgerBusy with the native
// BUSY code, no handle, no step script, no bump, the file as it was; the
// holder is released only after the answer, and the next open lifts the file
// to v16.
//
// PROBING MUTATIONS (MU34): a failed Begin classified as a shape (a verdict)
// → reddens; a failed Begin skipped as if the step were done → the open
// never answers while the lock is held → reddens at its bound.
//
// Evidence level: two processes (a child holds BEGIN IMMEDIATE on a raw
// connection and acknowledges it), real connections.
func TestE3_TE34_aHeldLockIsBusyAndLeavesTheStep(t *testing.T) {
	path := e3Copy(t, e3OldFile(t, 12))
	t0 := time.Now()
	b := e3StartChild(t, e3ChildSpec{Path: path, Hold: true})
	held := b.until(t, "E2ACK lock:held", 60*time.Second)
	e3Lines(t, "TE34", "B", t0, held)
	before := e2Snapshot(t, path)
	o := e3WatchMigration(t, path, nil)
	start := time.Now()
	type opened struct {
		h   *Store
		err error
	}
	answer := make(chan opened, 1)
	go func() {
		h, err := OpenFor(path, profileA)
		answer <- opened{h: h, err: err}
	}()
	var a opened
	select {
	case a = <-answer:
	case <-time.After(30 * time.Second):
		b.release(t, "lock:held")
		t.Fatal("A did not answer within 30 s while another process held the write lock")
	}
	e3Evidence(t, "TE34 parent: A answered %v after it started, the lock still held: %v", time.Since(start).Round(time.Millisecond), a.err)
	if a.h != nil {
		_ = a.h.Close()
		t.Fatal("OpenFor handed out a handle while another process held the write lock")
	}
	if !errors.Is(a.err, ErrLedgerBusy) || errors.Is(a.err, ErrLedgerUnreadable) || errors.Is(a.err, ErrLedgerEnvironment) {
		t.Fatalf("OpenFor under a held lock = %v, want ErrLedgerBusy and no other class", a.err)
	}
	if code := e3Code(a.err); code&0xff != 5 {
		t.Fatalf("OpenFor under a held lock carries code %d, want the native BUSY (5)", code)
	}
	if w := o.work(); w.ddl != 0 || w.bumps != 0 || w.skipped != 0 {
		t.Fatalf("the refused step ran %d script(s), %d bump(s) and skipped %d, want nothing", w.ddl, w.bumps, w.skipped)
	}
	e2Same(t, "the step while the lock was held", before, e2Snapshot(t, path))
	e3Evidence(t, "TE34 parent: the file is as it was; releasing B")
	b.release(t, "lock:held")
	code, lines := b.finish(t, 30*time.Second)
	e3Lines(t, "TE34", "B", t0, lines...)
	if code != 0 {
		t.Fatalf("B exited %d\n%s", code, b.said())
	}
	h, err := OpenFor(path, profileA)
	if err != nil {
		t.Fatalf("the open after the release = %v, want the lift", err)
	}
	defer func() { _ = h.Close() }()
	e3WantVersion(t, "after the release", path, 16)
}

// e3OneStep is template lifted by exactly its first step, by migrateStep on
// a raw connection: the state a commit of that one step leaves.
func e3OneStep(t *testing.T, template string, from int) e2State {
	t.Helper()
	path := e3Copy(t, template)
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateStep(db, migrations[from], migrationCopies[from], from); err != nil {
		t.Fatalf("the one-step reference from v%d: %v", from, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return e2Snapshot(t, path)
}

// e3IdentityRow is the identity row of path without the time it was
// written, raw: its owner, its founding act, and whether an adoption is named.
func e3IdentityRow(t *testing.T, path string) string {
	t.Helper()
	raw := e2Raw(t, path)
	var owner, founded string
	var adopted sql.NullString
	if err := raw.QueryRow(`SELECT owner_digest, founded_by_action, adopted_by_action FROM ledger_identity`).Scan(&owner, &founded, &adopted); err != nil {
		t.Fatalf("read the identity row of %s: %v", path, err)
	}
	return fmt.Sprintf("owner=%s founded_by=%s adopted=%t", owner, founded, adopted.Valid)
}

// TE35 · a process that dies inside a step leaves the step or its commit,
// never a part of it: before the commit — after its begin, DDL, copy, tail,
// bump, or right before the commit — the raw file is the fixture's, byte
// for byte in its catalog and rows; after the commit it is the fixture lifted
// by exactly that one step. The raw file is read before anything reopens it;
// the reopen then converges on v16, with the catalog of a clean lift.
//
// PROBING MUTATION (MU35): commit the step's script in its own transaction,
// before the copy and the bump → a crash after the DDL leaves the script
// without its version → reddens.
//
// Evidence level: a child process killed at each point (os.Exit(9), no
// defers), a crash and a restart; the parent reads the raw file first.
func TestE3_TE35_aCrashInsideAStepLeavesTheStepOrItsCommit(t *testing.T) {
	t.Parallel()
	v10, _ := buildV10File(t)
	fixtures := []struct {
		name     string
		template string
		from     int
		stages   []string
	}{
		{"v15", e3V15Founded(t), 15, []string{"begin", "ddl", "copy", "bump", "before-commit", "after-commit"}},
		{"v10", v10, 10, []string{"begin", "ddl", "copy", "tail", "bump", "before-commit", "after-commit"}},
	}
	for _, fx := range fixtures {
		fixture := e2Snapshot(t, fx.template)
		reference := e3OneStep(t, fx.template, fx.from)
		cleanPath := e3Copy(t, fx.template)
		clean, err := OpenFor(cleanPath, profileA)
		if err != nil {
			t.Fatalf("%s: the clean lift = %v", fx.name, err)
		}
		cleanStanding, _, err := clean.Standing(context.Background())
		_ = clean.Close()
		if err != nil {
			t.Fatalf("%s: the clean lift's standing: %v", fx.name, err)
		}
		lifted := e2Snapshot(t, cleanPath)
		for _, stage := range fx.stages {
			point := fmt.Sprintf("migrate:%s#%d", stage, fx.from)
			label := fmt.Sprintf("TE35 %s crash at %s", fx.name, point)
			path := e3Copy(t, fx.template)
			c := e3StartChild(t, e3ChildSpec{Path: path, Profile: profileA, Crash: point})
			code, lines := c.finish(t, 120*time.Second)
			acks, done := e2Acks(lines)
			if code != 9 || done != nil || len(acks) == 0 || acks[len(acks)-1] != point {
				t.Fatalf("%s: the child exited %d, last ack %v, want death by exit 9 right at the point\n%s", label, code, acks, c.said())
			}
			raw := e2Snapshot(t, path)
			e3Evidence(t, "%s · exit %d · last ack %q · versions %v · raw before reopening: %s", label, code, acks[len(acks)-1], e3Versions(t, path), e3Describe(fixture, raw))
			if stage == "after-commit" {
				want, got := reference, raw
				if fx.name == "v15" {
					// The seeded identity row carries the time it was written.
					want, got = e3Without(reference, "ledger_identity"), e3Without(raw, "ledger_identity")
					if row := e3IdentityRow(t, path); row != fmt.Sprintf("owner=%s founded_by=act_1 adopted=false", profileA) {
						t.Fatalf("%s: the committed identity row is %q, want owner %s founded by act_1", label, row, profileA)
					}
				}
				e2Same(t, label+": the raw file against the one-step commit", want, got)
			} else {
				e2Same(t, label+": the raw file against the fixture", fixture, raw)
			}
			h, err := OpenFor(path, profileA)
			if err != nil {
				t.Fatalf("%s: the reopen = %v, want the lift to converge", label, err)
			}
			st, _, err := h.Standing(context.Background())
			_ = h.Close()
			if err != nil || st != cleanStanding {
				t.Fatalf("%s: the reopened standing = %q %v, want %q as a clean lift", label, st, err, cleanStanding)
			}
			e3WantVersion(t, label+": the reopen", path, 16)
			e3SameObjects(t, label+": the reopen against a clean lift", lifted, e2Snapshot(t, path))
		}
	}
}

// e3V10WithRows is buildV10File's v10 ledger with its one tombstone changed
// by edit, or with extra tombstones inserted after it.
func e3V10WithRows(t *testing.T, statements ...string) string {
	t.Helper()
	path, _ := buildV10File(t)
	e2Lay(t, path, statements...)
	return path
}

// TE38 · a v10 tombstone the copy's typed judge refuses (a human verb with no
// deciding principal) makes the step unreadable at its site, with the
// TombstoneFault reachable through errors.As — its row and field — and the
// ledger left at v10 with its tombstone as it was: twice, nothing repaired.
//
// PROBING MUTATIONS (MU38): the migration names the fault with %v → the type
// is lost → reddens; the typed fault exempted from the migration's class →
// no ErrLedgerUnreadable → reddens; the partial copy committed → reddens.
//
// Evidence level: in process, native DB.
func TestE3_TE38_aTombstoneTheCopyRefusesIsUnreadable(t *testing.T) {
	t.Parallel()
	path := e3V10WithRows(t, `UPDATE approval_tombstones SET decision_principal_id = ''`)
	before := e2Snapshot(t, path)
	for attempt := 1; attempt <= 2; attempt++ {
		h, err := OpenFor(path, profileA)
		if h != nil {
			_ = h.Close()
			t.Fatalf("attempt %d: OpenFor handed out a handle over a tombstone the copy refuses", attempt)
		}
		if !errors.Is(err, ErrLedgerUnreadable) || errors.Is(err, ErrLedgerBusy) || errors.Is(err, ErrLedgerEnvironment) {
			t.Fatalf("attempt %d: OpenFor = %v, want ErrLedgerUnreadable and no other class", attempt, err)
		}
		var fault *TombstoneFault
		if !errors.As(err, &fault) || fault.Field != "decision_principal_id" || fault.ApprovalID != "apr_v10seed0000000000000000000000001" {
			t.Fatalf("attempt %d: OpenFor = %v, want the TombstoneFault of apr_v10seed…1 at decision_principal_id reachable", attempt, err)
		}
		e3WantVersion(t, fmt.Sprintf("attempt %d", attempt), path, 10)
		e2Same(t, fmt.Sprintf("attempt %d: the refused step", attempt), before, e2Snapshot(t, path))
	}
}

// TE38, the collision · two valid v10 tombstones of one approval (the
// digest does not cover the action id, v11 keys the approval): the copy's
// second INSERT meets the native constraint — calibrated first on its own
// copy of the file by migrateStep — and the step is unreadable at its site
// with the native code kept; the first row the copy inserted is rolled back
// with the rest, twice.
//
// PROBING MUTATIONS (MU38): %v → the code is lost → reddens; the partial copy
// committed → v11's table appears → reddens.
//
// Evidence level: in process, native DB; the constraint is SQLite's own.
func TestE3_TE38_aCollisionTheCopyMeetsIsUnreadableWithItsCode(t *testing.T) {
	t.Parallel()
	path := e3V10WithRows(t, `INSERT INTO approval_tombstones
	    (action_id, approval_id, action_digest, preview_digest, policy_version,
	     policy_digest, decision_principal_id, decision, decision_at)
	 SELECT 'act_v10seed_b', approval_id, action_digest, preview_digest, policy_version,
	        policy_digest, decision_principal_id, decision, decision_at
	   FROM approval_tombstones WHERE action_id = 'act_v10seed'`)
	calibration := e3Copy(t, path)
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(calibration)))
	if err != nil {
		t.Fatal(err)
	}
	cerr := migrateStep(db, migrations[10], migrationCopies[10], 10)
	_ = db.Close()
	native := e3Code(cerr)
	e3Evidence(t, "TE38 calibration: migrateStep from v10 over the colliding pair = code %d: %v", native, cerr)
	if native&0xff != 19 {
		t.Fatalf("calibration: the colliding pair did not meet a native constraint (code %d): %v", native, cerr)
	}
	before := e2Snapshot(t, path)
	for attempt := 1; attempt <= 2; attempt++ {
		h, err := OpenFor(path, profileA)
		if h != nil {
			_ = h.Close()
			t.Fatalf("attempt %d: OpenFor handed out a handle over a copy that met a constraint", attempt)
		}
		if !errors.Is(err, ErrLedgerUnreadable) || errors.Is(err, ErrLedgerBusy) || errors.Is(err, ErrLedgerEnvironment) {
			t.Fatalf("attempt %d: OpenFor = %v, want ErrLedgerUnreadable and no other class", attempt, err)
		}
		if code := e3Code(err); code != native {
			t.Fatalf("attempt %d: OpenFor carries code %d, want the calibrated native %d", attempt, code, native)
		}
		e3WantVersion(t, fmt.Sprintf("attempt %d", attempt), path, 10)
		e2Same(t, fmt.Sprintf("attempt %d: the refused step", attempt), before, e2Snapshot(t, path))
	}
}

// e3Fault is one failure a TE39 subcase injects, and its class.
type e3Fault struct {
	name  string
	make  func() error
	class ledgerClass
}

func e3Coded(class ledgerClass, code int) e3Fault {
	return e3Fault{name: fmt.Sprintf("code%d", code), make: func() error { return &codedFault{code: code, tag: "TE39"} }, class: class}
}

// e3WantClass fails unless err carries exactly class's name — none for
// classNone — and the injected cause itself.
func e3WantClass(t *testing.T, label string, err, cause error, class ledgerClass) {
	t.Helper()
	if want := classSentinel(class); want != nil {
		for _, other := range []error{ErrLedgerUnreadable, ErrLedgerBusy, ErrLedgerEnvironment} {
			if errors.Is(err, other) != (other == want) {
				t.Fatalf("%s: OpenFor = %v, want %v and no other class", label, err, want)
			}
		}
	} else if !namesNoClass(err) {
		t.Fatalf("%s: OpenFor = %v, want the original error with no class", label, err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("%s: OpenFor = %v, the injected cause is not in its chain", label, err)
	}
}

// TE39 · a failure at any stage of a step — its begin, DDL, copy, tail,
// version bump, or right before its commit — keeps its own class: BUSY is
// ErrLedgerBusy, IOERR is ErrLedgerEnvironment, an unknown code and an
// uncoded failure are themselves, with no class; a CONSTRAINT outside the
// stages that transform data is itself too (the migration's verdict is
// confined to TE38). No handle, the step's input as it was, the cause in the
// chain; the next open lifts the file.
//
// PROBING MUTATIONS (MU39): every migration failure made unreadable →
// reddens; the original chain stripped (%v) → reddens.
//
// Evidence level: in process, native DB + a controlled failure put on the
// result of the stage's real operation (openStageFaultSeam).
func TestE3_TE39_aFailedStageKeepsItsClass(t *testing.T) {
	v10, _ := buildV10File(t)
	faults := []e3Fault{
		e3Coded(classBusy, 5), e3Coded(classEnvironment, 10), e3Coded(classNone, 2),
		{name: "uncoded", make: func() error { return errors.New("TE39: an uncoded failure") }, class: classNone},
	}
	for _, stage := range []string{"begin", "ddl", "copy", "tail", "bump", "pre-commit"} {
		kinds := faults
		if stage == "begin" || stage == "bump" || stage == "pre-commit" {
			kinds = append(slices.Clone(faults), e3Coded(classNone, 19))
		}
		for _, k := range kinds {
			label := fmt.Sprintf("%s/%s", stage, k.name)
			t.Run(label, func(t *testing.T) {
				path := e3Copy(t, v10)
				before := e2Snapshot(t, path)
				fault := k.make()
				f := armStageFault(t, path, "migrate:"+stage, fault)
				h, err := OpenFor(path, profileA)
				openStageFaultSeam.CompareAndSwap(f, nil)
				if h != nil {
					_ = h.Close()
					t.Fatalf("%s: OpenFor handed out a handle over a failed step", label)
				}
				e3WantClass(t, label, err, fault, k.class)
				if !f.wasConsumed() {
					t.Fatalf("%s: the stage was never reached", label)
				}
				e2Same(t, label+": the failed step", before, e2Snapshot(t, path))
				e3WantVersion(t, label, path, 10)
				h, err = OpenFor(path, profileA)
				if err != nil {
					t.Fatalf("%s: the next open = %v, want the lift", label, err)
				}
				_ = h.Close()
				e3WantVersion(t, label+": the next open", path, 16)
			})
		}
	}
}

// TE39, the reread · a failed read of the version inside the step's
// transaction (QversionTx, origin migration-reread) — at its query, its
// iteration or its cell — keeps its class: a structural code is the plan's
// single-hit row, a fatal ErrLedgerUnreadable carrying the cause; BUSY and
// IOERR their classes; an unknown code and an uncoded failure themselves. No
// handle, no step script, no bump, the step as it was; the site decided once;
// the next open lifts the file.
//
// PROBING MUTATIONS (MU39, MU33): every reread failure made unreadable →
// reddens; the reread removed → reddens.
//
// Evidence level: in process, native DB + a synthetic coded failure put on
// the result of the reread that really ran.
func TestE3_TE39_aFailedVersionRereadKeepsItsClass(t *testing.T) {
	template := e3OldFile(t, 12)
	faults := []e3Fault{
		e3Coded(classStructural, 11), e3Coded(classBusy, 5), e3Coded(classEnvironment, 10), e3Coded(classNone, 2),
		{name: "uncoded", make: func() error { return errors.New("TE39: an uncoded reread failure") }, class: classNone},
	}
	for _, stage := range []readStage{stageQuery, stageNext, stageScan} {
		for _, k := range faults {
			label := fmt.Sprintf("%s/%s", stage, k.name)
			t.Run(label, func(t *testing.T) {
				path := e3Copy(t, template)
				before := e2Snapshot(t, path)
				o := e3WatchMigration(t, path, nil)
				fault := k.make()
				f := armJudgeFault(t, path, originMigrationReread, siteVersionTx, stage, fault)
				h, err := OpenFor(path, profileA)
				r := f.disarm()
				if h != nil {
					_ = h.Close()
					t.Fatalf("%s: OpenFor handed out a handle over a step whose reread failed", label)
				}
				e3WantClass(t, label, err, fault, k.class)
				if w := o.work(); w.ddl != 0 || w.bumps != 0 {
					t.Fatalf("%s: the step ran %d script(s) and %d bump(s) after its reread failed, want none", label, w.ddl, w.bumps)
				}
				e2Same(t, label+": the step whose reread failed", before, e2Snapshot(t, path))
				checkReceipt(t, label, r, siteVersionTx, k.class == classStructural, k.class)
				h, err = OpenFor(path, profileA)
				if err != nil {
					t.Fatalf("%s: the next open = %v, want the lift", label, err)
				}
				_ = h.Close()
				e3WantVersion(t, label+": the next open", path, 16)
			})
		}
	}
}

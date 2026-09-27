// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Train E, batch 2 — the RESIDUE and the confirming judgement (plan v3, §6
// rows TE02, TE03, TE08, §13.1 rows TE68–TE70, Annex C's P2-1): an empty
// prefix of the v1 bootstrap is a fresh file, anything else that the action
// store owns is not; the namespace of tables, views and indexes is not the
// namespace of triggers; a bad verdict met once is judged again before an
// open acts on it, and the operator's door never seeds a file that existed.
//
// Evidence level: in process, native connections on real files; TE02's
// official reproduction has its historical seed die in a SEPARATE OS
// process; TE68–TE70's faults are SYNTHETIC (codedFault) put on the result of
// a read that really ran. The compiled CLI's and the app's halves of these
// rows live in their own packages.

package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// e2Fixture is one file shape a mould lays over a conversation file.
type e2Fixture struct {
	name string
	lay  func(t *testing.T, path string)
}

// e2PrefixFixtures are the empty prefixes k = 1…5, the five in another
// order, and the five beside a conversation trigger wearing a schema name.
func e2PrefixFixtures() []e2Fixture {
	var out []e2Fixture
	for k := 1; k <= 5; k++ {
		k := k
		out = append(out, e2Fixture{name: fmt.Sprintf("k%d", k), lay: func(t *testing.T, path string) { e2Prefix(t, path, k) }})
	}
	out = append(out,
		e2Fixture{name: "k5-reordered", lay: e2PrefixReordered},
		e2Fixture{name: "k5-and-a-trigger-named-actions-on-sessions", lay: func(t *testing.T, path string) {
			e2Prefix(t, path, 5)
			e2Lay(t, path, `CREATE TRIGGER actions AFTER INSERT ON sessions BEGIN SELECT 1; END`)
		}})
	return out
}

// TE02 · an EMPTY prefix of the v1 bootstrap — the residue the historical
// seed left when it died between its statements — is a fresh file for every
// judge: the shape, the connection hook (judged on a raw connection AND at a
// real writer's birth), the operator's probe. The writer completes it: v16,
// legacy_unfounded, an act recorded, every object of a fresh store and no
// other. The operator's door and the reader, on the existing prefix, name
// ErrNoActionStore and change nothing. Each k = 1…5, the five in another
// order, and a trigger on the conversations wearing a schema name.
//
// PROBING MUTATIONS (MU02): classify a prefix bad → reddens; require the
// catalog's order → the reordered prefix reddens; judge a trigger by its
// name → the trigger case reddens.
func TestE2_TE02_anEmptyBootstrapPrefixIsFresh(t *testing.T) {
	ctx := context.Background()
	want := e2ReferenceObjects(t)
	for _, fx := range e2PrefixFixtures() {
		fx := fx
		t.Run(fx.name, func(t *testing.T) {
			build := func() string {
				path := e2ConversationFile(t)
				fx.lay(t, path)
				return path
			}
			path := build()
			if v, err := judgeShape(ctx, dbShapeQuerier{q: e2Raw(t, path)}); err != nil || v.shape != shapeFresh {
				t.Fatalf("the shape of the prefix = %v (%s) %v, want fresh", v.shape, v.reason, err)
			}
			if got := judgeOnRawConn(t, path, profileA); got != "ok" {
				t.Fatalf("the hook judged the prefix %q on a raw connection, want ok", got)
			}
			if v, err := probeShape(e2Abs(t, path)); err != nil || v.shape != shapeFresh {
				t.Fatalf("the operator's probe of the prefix = %v (%s) %v, want fresh", v.shape, v.reason, err)
			}
			for _, o := range e1Openers[1:] {
				p := build()
				traps := e2ArmTraps(t, p)
				before := e2Snapshot(t, p)
				w := e2WatchSeed(t, p, nil)
				h, err := o.open(p, profileA)
				if h != nil {
					_ = h.Close()
					t.Fatalf("%s handed out a handle on an existing prefix", o.name)
				}
				if !errors.Is(err, ErrNoActionStore) {
					t.Fatalf("%s on an existing prefix = %v, want ErrNoActionStore", o.name, err)
				}
				e2Same(t, o.name, before, e2Snapshot(t, p))
				if seen := w.seen(); seen.ddl != 0 || seen.inserts != 0 {
					t.Fatalf("%s dispatched %d bootstrap DDL and %d version INSERT", o.name, seen.ddl, seen.inserts)
				}
				if n := traps.hits(); n != 0 {
					t.Fatalf("%s attempted %d conversation writes", o.name, n)
				}
				seedObserverSeam.CompareAndSwap(w, nil)
			}
			p := build()
			traps := e2ArmTraps(t, p)
			before := e2Snapshot(t, p)
			hook := e2WatchHook(t, p)
			e2Healthy(t, "the writer over the prefix", p)
			after := e2Snapshot(t, p)
			e2SameObjects(t, "the completed schema", want, e2ActionObjects(after))
			e2Same(t, "the conversation store", before.conversation(), after.conversation())
			if n := traps.hits(); n != 0 {
				t.Fatalf("%d conversation writes were attempted", n)
			}
			if births := hook.seen(); len(births) == 0 || births[0] != "ok" {
				t.Fatalf("the hook judged the prefix %v at the writer's births, want ok at the first", births)
			}
		})
	}
}

// TE02 (the official reproduction, replayed) · P2-1's steps (plan Annex C),
// on the residue itself. Steps 1–3: a child lays the historical seed's five
// statements over a conversation file in autocommit and dies (exit 9) before
// its version row; the catalog is the reproduction's, with action_schema
// empty. Steps 4–5: two restarts, each OpenFor, Standing and SchemaVersion —
// where the report saw ledger_unreadable «action_schema has 0 rows» and
// version 0, both now open legacy_unfounded at 16. Step 6 (train C's binary
// healing the residue) is not replayed: that binary is not in this tree.
// Step 7: the state the FIRST statement leaves, opened by two writers at
// once — both open, both record.
//
// PROBING MUTATION (MU02): classify the prefix bad → the restarts stand
// unreadable → reddens.
func TestE2_TE02_theOfficialReproductionHealsOnRestart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := e2ConversationFile(t)
	c := e2StartChild(t, e2ChildSpec{Path: path, Historical: 5})
	code, lines := c.finish(t, time.Minute)
	if acks, _ := e2Acks(lines); code != 9 || len(acks) != 1 || acks[0] != "historical:5" {
		t.Fatalf("the historical seed ended with exit %d after %v, want exit 9 after its five statements\n%s", code, acks, c.said())
	}
	st := e2Snapshot(t, path)
	var names []string
	for k := range st.objects {
		kind, name, _ := strings.Cut(k, ":")
		if (kind == "table" || kind == "index") && !strings.HasPrefix(name, "sqlite_") {
			names = append(names, name)
		}
	}
	wantNames := map[string]bool{"sessions": true, "turns": true, "notes": true, "action_schema": true, "actions": true, "actions_by_correlation": true, "actions_by_requested": true, "action_decisions": true}
	if len(names) != len(wantNames) {
		t.Fatalf("the residue's catalog is %v, want the reproduction's eight objects", names)
	}
	for _, n := range names {
		if !wantNames[n] {
			t.Fatalf("the residue's catalog is %v, want the reproduction's eight objects", names)
		}
	}
	if st.counts["action_schema"] != 0 {
		t.Fatalf("action_schema holds %d rows in the residue, want 0", st.counts["action_schema"])
	}
	for restart := 1; restart <= 2; restart++ {
		h, err := OpenFor(path, profileA)
		if err != nil {
			t.Fatalf("restart %d: OpenFor = %v", restart, err)
		}
		standing, owner, serr := h.Standing(ctx)
		version, verr := h.SchemaVersion(ctx)
		_ = h.Close()
		if serr != nil || standing != LedgerStandingLegacyUnfounded || owner != "" || verr != nil || version != schemaVersionCurrent {
			t.Fatalf("restart %d: standing=%q owner=%q err=%v · schema version=%d err=%v; want legacy_unfounded at %d", restart, standing, owner, serr, version, verr, schemaVersionCurrent)
		}
	}
	// Step 7, on the state the first statement leaves.
	p := e2ConversationFile(t)
	e2Prefix(t, p, 1)
	a, err := OpenFor(p, profileA)
	if err != nil {
		t.Fatalf("the first writer over the first statement's residue = %v", err)
	}
	defer func() { _ = a.Close() }()
	b, err := OpenFor(p, profileA)
	if err != nil {
		t.Fatalf("the second writer over the first statement's residue = %v", err)
	}
	defer func() { _ = b.Close() }()
	for i, h := range []*Store{a, b} {
		if err := guardedWrite(h, fmt.Sprintf("e2_step7_%d", i)); err != nil {
			t.Fatalf("writer %d: %v, want the act recorded", i, err)
		}
	}
}

// TE03 · anything the action store owns that is NOT an empty prefix of the
// v1 bootstrap is a bad shape, never fresh: a row, a view where a table
// belongs, an altered definition, a set that is no prefix, an index the
// prefix skips, an extra index or trigger on an action table, a UNIQUE whose
// autoindex the bootstrap never makes. The writer opens blocked — Standing
// and every act name ErrLedgerUnreadable — the anonymous opener refuses by
// name, nothing is seeded or repaired, no row changes.
//
// PROBING MUTATIONS (MU03): recognise a prefix by names alone → the altered
// and view cases redden; accept any subset → the no-prefix and omitted-index
// cases redden; ignore the target table or the extra objects → the extra
// index and trigger cases redden.
func TestE2_TE03_whatIsNotAnEmptyPrefixIsBad(t *testing.T) {
	ctx := context.Background()
	stmt := func(t *testing.T, i int) string { return e2Statements(t)[i] }
	fixtures := []e2Fixture{
		{name: "a row", lay: func(t *testing.T, path string) {
			e2Prefix(t, path, 2)
			e2Lay(t, path, `INSERT INTO actions (action_id, schema_version, correlation_id, source_kind, source_protocol, source_channel, op_namespace, op_name, op_version, parameters_digest, effect_class, state, requested_at) VALUES ('act_x', 1, 'c', 'k', 'p', 'ch', 'ns', 'op', 1, 'd', 'read', 'AUTHORIZED', '2026-09-26T00:00:00Z')`)
		}},
		{name: "a view where actions belongs", lay: func(t *testing.T, path string) {
			e2Lay(t, path, stmt(t, 0), `CREATE VIEW actions AS SELECT 1 AS action_id`)
		}},
		{name: "an altered actions", lay: func(t *testing.T, path string) {
			e2Lay(t, path, stmt(t, 0), strings.Replace(stmt(t, 1), "finished_at       TEXT", "finished_at       TEXT,\n    extra             TEXT", 1))
		}},
		{name: "an altered action_schema", lay: func(t *testing.T, path string) {
			e2Lay(t, path, strings.Replace(stmt(t, 0), "version INTEGER NOT NULL", "version INTEGER NOT NULL DEFAULT 1", 1))
		}},
		{name: "a set that is no prefix", lay: func(t *testing.T, path string) {
			e2Lay(t, path, stmt(t, 0), stmt(t, 4))
		}},
		{name: "an index the prefix skips", lay: func(t *testing.T, path string) {
			e2Lay(t, path, stmt(t, 0), stmt(t, 1), stmt(t, 3))
		}},
		{name: "an extra index on actions", lay: func(t *testing.T, path string) {
			e2Prefix(t, path, 4)
			e2Lay(t, path, `CREATE INDEX e2_extra ON actions(state)`)
		}},
		{name: "an extra trigger on actions", lay: func(t *testing.T, path string) {
			e2Prefix(t, path, 2)
			e2Lay(t, path, `CREATE TRIGGER e2_extra AFTER INSERT ON actions BEGIN SELECT 1; END`)
		}},
		{name: "a UNIQUE the bootstrap never makes", lay: func(t *testing.T, path string) {
			e2Lay(t, path, strings.Replace(stmt(t, 0), "version INTEGER NOT NULL", "version INTEGER NOT NULL UNIQUE", 1))
		}},
	}
	for _, fx := range fixtures {
		fx := fx
		t.Run(fx.name, func(t *testing.T) {
			path := e2ConversationFile(t)
			fx.lay(t, path)
			traps := e2ArmTraps(t, path)
			before := e2Snapshot(t, path)
			w := e2WatchSeed(t, path, nil)
			h, err := OpenFor(path, profileA)
			if err != nil || h == nil {
				t.Fatalf("OpenFor = %v, want a handle that names the shape", err)
			}
			st, _, serr := h.Standing(ctx)
			werr := guardedWrite(h, "e2_refused")
			_ = h.Close()
			if st != LedgerStandingUnreadable || !errors.Is(serr, ErrLedgerUnreadable) {
				t.Fatalf("Standing = %q %v, want ErrLedgerUnreadable", st, serr)
			}
			if !errors.Is(werr, ErrLedgerUnreadable) {
				t.Fatalf("a write = %v, want ErrLedgerUnreadable", werr)
			}
			if s, err := open(path); !errors.Is(err, ErrLedgerUnreadable) {
				if s != nil {
					_ = s.Close()
				}
				t.Fatalf("the anonymous opener = %v, want ErrLedgerUnreadable", err)
			}
			e2Same(t, "the file after the openers", before, e2Snapshot(t, path))
			if seen := w.seen(); seen.ddl != 0 || seen.inserts != 0 {
				t.Fatalf("the openers dispatched %d bootstrap DDL and %d version INSERT", seen.ddl, seen.inserts)
			}
			if n := traps.hits(); n != 0 {
				t.Fatalf("%d conversation writes were attempted", n)
			}
		})
	}
}

// TE08 · triggers are not in the namespace of tables, views and indexes. A
// trigger named `actions` on the conversations is not the action store's —
// the file is fresh, the writer seeds and records, and the trigger survives
// with its DDL. An INDEX named `actions` on the conversations is: the file is
// bad, the writer opens blocked and nothing is seeded.
//
// PROBING MUTATIONS (MU08): count a trigger by its name → the trigger case
// reddens; leave indexes out of the namespace → the index case seeds →
// reddens.
func TestE2_TE08_theNamespaceIsNotTheTriggers(t *testing.T) {
	ctx := context.Background()
	t.Run("a trigger named actions on sessions", func(t *testing.T) {
		path := e2ConversationFile(t)
		e2Lay(t, path, `CREATE TRIGGER actions AFTER INSERT ON sessions BEGIN SELECT 1; END`)
		before := e2Snapshot(t, path)
		if v, err := judgeShape(ctx, dbShapeQuerier{q: e2Raw(t, path)}); err != nil || v.shape != shapeFresh {
			t.Fatalf("the shape = %v (%s) %v, want fresh", v.shape, v.reason, err)
		}
		e2Healthy(t, "the writer", path)
		e2Same(t, "the conversation store and its trigger", before.conversation(), e2Snapshot(t, path).conversation())
	})
	t.Run("an index named actions on sessions", func(t *testing.T) {
		path := e2ConversationFile(t)
		e2Lay(t, path, `CREATE INDEX actions ON sessions(key)`)
		before := e2Snapshot(t, path)
		if v, err := judgeShape(ctx, dbShapeQuerier{q: e2Raw(t, path)}); err != nil || v.shape != shapeBad {
			t.Fatalf("the shape = %v (%s) %v, want bad", v.shape, v.reason, err)
		}
		w := e2WatchSeed(t, path, nil)
		h, err := OpenFor(path, profileA)
		if err != nil || h == nil {
			t.Fatalf("OpenFor = %v, want a handle that names the shape", err)
		}
		st, _, serr := h.Standing(ctx)
		_ = h.Close()
		if st != LedgerStandingUnreadable || !errors.Is(serr, ErrLedgerUnreadable) {
			t.Fatalf("Standing = %q %v, want ErrLedgerUnreadable", st, serr)
		}
		if seen := w.seen(); seen.ddl != 0 || seen.inserts != 0 {
			t.Fatalf("the writer dispatched %d bootstrap DDL and %d version INSERT", seen.ddl, seen.inserts)
		}
		e2Same(t, "the file", before, e2Snapshot(t, path))
	})
}

// e2ResidueHit is one of TE68–TE70's subcases: a prefix k, a stage of the
// first residue read.
type e2ResidueHit struct {
	k     int
	stage readStage
}

func e2ResidueHits() []e2ResidueHit {
	var out []e2ResidueHit
	for k := 1; k <= 5; k++ {
		for _, stage := range e1DBStages {
			out = append(out, e2ResidueHit{k: k, stage: stage})
		}
	}
	return out
}

// TE68 · the writer judges a bad verdict AGAIN before it acts on it. One
// structural failure (code 11) of the residue's read at the pool's first
// judgement makes the prefix look bad once; the confirming judgement reads it
// whole and finds it fresh, so the writer seeds it: a handle, v16,
// legacy_unfounded, an act recorded — never the unreadable a file whose only
// defect is being a prefix does not deserve.
//
// PROBING MUTATION (MU68): remove the confirming judgement → the writer opens
// blocked → Standing unreadable → reddens.
func TestE2_TE68_theWriterConfirmsABadVerdictBeforeActingOnIt(t *testing.T) {
	for _, c := range e2ResidueHits() {
		c := c
		label := fmt.Sprintf("k%d/%s", c.k, c.stage)
		t.Run(label, func(t *testing.T) {
			path := e2ConversationFile(t)
			e2Prefix(t, path, c.k)
			traps := e2ArmTraps(t, path)
			before := e2Snapshot(t, path)
			fault := &codedFault{code: 11, tag: "residue at the pool's judgement"}
			f := armJudgeFault(t, path, originPoolOpenShape, siteResidue, c.stage, fault)
			h, err := OpenFor(path, profileA)
			rcpt := f.disarm()
			if err != nil || h == nil {
				t.Fatalf("%s: OpenFor = %v, want the prefix seeded", label, err)
			}
			v, verr := h.SchemaVersion(context.Background())
			st, _, serr := h.Standing(context.Background())
			werr := guardedWrite(h, "e2_after_the_confirmation")
			_ = h.Close()
			if verr != nil || v != schemaVersionCurrent {
				t.Fatalf("%s: SchemaVersion = %d %v, want %d", label, v, verr, schemaVersionCurrent)
			}
			if serr != nil || st != LedgerStandingLegacyUnfounded {
				t.Fatalf("%s: Standing = %q %v, want legacy_unfounded", label, st, serr)
			}
			if werr != nil {
				t.Fatalf("%s: a write = %v, want it recorded", label, werr)
			}
			e2Same(t, label+": the conversation store", before.conversation(), e2Snapshot(t, path).conversation())
			if n := traps.hits(); n != 0 {
				t.Fatalf("%s: %d conversation writes were attempted", label, n)
			}
			checkReceipt(t, label, rcpt, siteResidue, true, classStructural)
			if !e2Origin(rcpt.otherOrigins, originPoolOpenConfirm) {
				t.Fatalf("%s: the residue was never read by the confirming judgement (other origins %v)", label, rcpt.otherOrigins)
			}
		})
	}
}

func e2Origin(origins []judgeOrigin, want judgeOrigin) bool {
	for _, o := range origins {
		if o == want {
			return true
		}
	}
	return false
}

// TE69 · the operator's door never seeds, never migrates a file that
// existed when it was called. Its probe meets one structural failure of the
// residue's read and falls through; the operator-mode open judges the prefix
// fresh and — the file having existed — names ErrNoActionStore before any
// statement: no handle, no version row, no DDL, the file as it was.
//
// PROBING MUTATION (MU69): ignore that the file existed → the door seeds →
// the dispatch counters are not zero, a version row appears → reddens.
func TestE2_TE69_theOperatorNeverSeedsAFileThatExisted(t *testing.T) {
	for _, c := range e2ResidueHits() {
		c := c
		label := fmt.Sprintf("k%d/%s", c.k, c.stage)
		t.Run(label, func(t *testing.T) {
			path := e2ConversationFile(t)
			e2Prefix(t, path, c.k)
			traps := e2ArmTraps(t, path)
			before := e2Snapshot(t, path)
			w := e2WatchSeed(t, path, nil)
			fault := &codedFault{code: 11, tag: "residue at the operator's probe"}
			f := armJudgeFault(t, path, originOperatorProbe, siteResidue, c.stage, fault)
			h, err := OpenOperatorFor(path, profileA)
			rcpt := f.disarm()
			if h != nil {
				_ = h.Close()
				t.Fatalf("%s: OpenOperatorFor handed out a handle on a file that existed as a prefix", label)
			}
			if !errors.Is(err, ErrNoActionStore) {
				t.Fatalf("%s: OpenOperatorFor = %v, want ErrNoActionStore", label, err)
			}
			if seen := w.seen(); seen.ddl != 0 || seen.inserts != 0 {
				t.Fatalf("%s: the door dispatched %d bootstrap DDL and %d version INSERT", label, seen.ddl, seen.inserts)
			}
			e2Same(t, label, before, e2Snapshot(t, path))
			if n := traps.hits(); n != 0 {
				t.Fatalf("%s: %d conversation writes were attempted", label, n)
			}
			checkReceipt(t, label, rcpt, siteResidue, true, classStructural)
		})
	}
}

// TE70 · the reader judges a bad verdict again before it hands itself out.
// One structural failure of the residue's read at its first judgement; the
// confirming judgement finds the prefix fresh, and the reader names
// ErrNoActionStore instead of opening over it.
//
// PROBING MUTATION (MU70): accept the bad verdict without confirming it → the
// reader is handed out → reddens.
func TestE2_TE70_theReaderConfirmsABadVerdictBeforeOpening(t *testing.T) {
	for _, c := range e2ResidueHits() {
		c := c
		label := fmt.Sprintf("k%d/%s", c.k, c.stage)
		t.Run(label, func(t *testing.T) {
			path := e2ConversationFile(t)
			e2Prefix(t, path, c.k)
			before := e2Snapshot(t, path)
			fault := &codedFault{code: 11, tag: "residue at the reader's judgement"}
			f := armJudgeFault(t, path, originReadOnlyOpen, siteResidue, c.stage, fault)
			h, err := OpenReadOnlyFor(path, profileA)
			rcpt := f.disarm()
			if h != nil {
				_ = h.Close()
				t.Fatalf("%s: OpenReadOnlyFor handed out a reader over a prefix", label)
			}
			if !errors.Is(err, ErrNoActionStore) {
				t.Fatalf("%s: OpenReadOnlyFor = %v, want ErrNoActionStore", label, err)
			}
			e2Same(t, label, before, e2Snapshot(t, path))
			checkReceipt(t, label, rcpt, siteResidue, true, classStructural)
			if !e2Origin(rcpt.otherOrigins, originReadOnlyConfirm) {
				t.Fatalf("%s: the residue was never read by the confirming judgement (other origins %v)", label, rcpt.otherOrigins)
			}
		})
	}
}

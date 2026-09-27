// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Train E, batch 2 — the INDEXES (plan v3, §4 «index descriptors», §6 rows
// TE09–TE11): a current ledger requires its three UNIQUE indexes whole — not
// their names, not their UNIQUE bit — and nothing of its eight performance
// indexes; the eleven index names belong to the action store's namespace.
//
// Evidence level: in process, native connections on real files; the attacker
// is a second real connection. The reference TE11 compares the manifest with
// is a fresh store written by the real DDL and migrations.

package sqlite

import (
	"context"
	"database/sql"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// e2UniqueNames are the three UNIQUE indexes a current ledger requires.
var e2UniqueNames = []string{"execution_bindings_active_selector", "principal_bindings_equivalent_active", "budget_debits_sequence"}

// TE09 · each of the three UNIQUE indexes dropped from a current ledger: the
// ledger is not current — every opener stands unreadable naming the missing
// index, the writers refuse by name, and nothing recreates it.
//
// PROBING MUTATION (MU09): leave one UNIQUE out of the production manifest →
// its case opens healthy → reddens.
func TestE2_TE09_aMissingUniqueIndexIsNamed(t *testing.T) {
	t.Parallel()
	for _, name := range e2UniqueNames {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path, exec := e1Founded(t)
			exec(`DROP INDEX ` + name)
			before := e2Snapshot(t, path)
			e1Blocked(t, path, profileA, name, "missing")
			e2Same(t, "the ledger after every opener", before, e2Snapshot(t, path))
		})
	}
}

// e2IndexAttack is one TE10 case: the index replaced under its own name by
// ddl, the dimension the error must name, and — for an index that stops
// guarding its invariant — the raw duplicate it now admits.
type e2IndexAttack struct {
	name, index, dimension string
	ddl                    []string
	duplicate              []string
}

// TE10 · a UNIQUE index is its whole descriptor: its table, its uniqueness,
// its keys in order — a column or an expression —, each key's collation and
// direction, and its partial predicate. Each dimension changed alone, under
// the index's own name, makes the ledger not current: every opener stands
// unreadable naming the index and the dimension; the writers refuse. N10-7
// literally (`WHERE 0`), then one attack per dimension. Where the attack
// stops the index from guarding its invariant, a raw duplicate the index now
// admits calibrates the attack; it is not the product's oracle.
//
// PROBING MUTATION (MU10): ignore exactly the attacked dimension in the
// production comparison → its case opens healthy → reddens.
func TestE2_TE10_aUniqueIndexIsItsWholeDescriptor(t *testing.T) {
	t.Parallel()
	const sel = "execution_bindings_active_selector"
	const seq = "budget_debits_sequence"
	const eq = "principal_bindings_equivalent_active"
	dupBindings := []string{
		`INSERT INTO execution_bindings (binding_id, actor_principal_id, channel, conversation_id, intent_id, intent_version, intent_digest, revision, status) VALUES ('e2_b1', 'p', 'ch', NULL, 'i', 1, 'd', 1, 'ACTIVE')`,
		`INSERT INTO execution_bindings (binding_id, actor_principal_id, channel, conversation_id, intent_id, intent_version, intent_digest, revision, status) VALUES ('e2_b2', 'p', 'ch', NULL, 'i', 1, 'd', 1, 'ACTIVE')`,
	}
	attacks := []e2IndexAttack{
		{name: "N10-7 WHERE 0", index: sel, dimension: "predicate", duplicate: dupBindings, ddl: []string{
			`DROP INDEX execution_bindings_active_selector`,
			`CREATE UNIQUE INDEX execution_bindings_active_selector
ON execution_bindings(actor_principal_id, channel, ifnull(conversation_id,'')) WHERE 0`}},
		{name: "the predicate's literal", index: sel, dimension: "predicate", duplicate: dupBindings, ddl: []string{
			`DROP INDEX execution_bindings_active_selector`,
			`CREATE UNIQUE INDEX execution_bindings_active_selector ON execution_bindings(actor_principal_id, channel, ifnull(conversation_id, '')) WHERE status = 'active'`}},
		{name: "an expression", index: sel, dimension: "expression", ddl: []string{
			`DROP INDEX execution_bindings_active_selector`,
			`CREATE UNIQUE INDEX execution_bindings_active_selector ON execution_bindings(actor_principal_id, channel, ifnull(conversation_id, '-')) WHERE status = 'ACTIVE'`}},
		{name: "not unique", index: sel, dimension: "unique", duplicate: dupBindings, ddl: []string{
			`DROP INDEX execution_bindings_active_selector`,
			`CREATE INDEX execution_bindings_active_selector ON execution_bindings(actor_principal_id, channel, ifnull(conversation_id, '')) WHERE status = 'ACTIVE'`}},
		{name: "a collation", index: seq, dimension: "collation", ddl: []string{
			`DROP INDEX budget_debits_sequence`,
			`CREATE UNIQUE INDEX budget_debits_sequence ON budget_debits(account_id COLLATE NOCASE, operation_key, sequence)`}},
		{name: "a direction", index: seq, dimension: "direction", ddl: []string{
			`DROP INDEX budget_debits_sequence`,
			`CREATE UNIQUE INDEX budget_debits_sequence ON budget_debits(account_id, operation_key, sequence DESC)`}},
		{name: "the key order", index: seq, dimension: "keys", ddl: []string{
			`DROP INDEX budget_debits_sequence`,
			`CREATE UNIQUE INDEX budget_debits_sequence ON budget_debits(operation_key, account_id, sequence)`}},
		{name: "an extra budget key", index: seq, dimension: "keys", ddl: []string{
			`DROP INDEX budget_debits_sequence`,
			`CREATE UNIQUE INDEX budget_debits_sequence ON budget_debits(account_id, operation_key, sequence, signature)`}},
		{name: "another table", index: seq, dimension: "table", ddl: []string{
			`DROP INDEX budget_debits_sequence`,
			`CREATE UNIQUE INDEX budget_debits_sequence ON budget_counters(account_id, operation_key, sequence)`}},
		{name: "a predicate where none belongs", index: seq, dimension: "predicate", ddl: []string{
			`DROP INDEX budget_debits_sequence`,
			`CREATE UNIQUE INDEX budget_debits_sequence ON budget_debits(account_id, operation_key, sequence) WHERE sequence > 0`}},
		{name: "the other predicate's literal", index: eq, dimension: "predicate", ddl: []string{
			`DROP INDEX principal_bindings_equivalent_active`,
			`CREATE UNIQUE INDEX principal_bindings_equivalent_active ON principal_bindings(provider, channel, credential_ref, subject_namespace, verified_subject, principal_id) WHERE status = 'ACTIVE'`}},
		{name: "a key missing", index: eq, dimension: "keys", ddl: []string{
			`DROP INDEX principal_bindings_equivalent_active`,
			`CREATE UNIQUE INDEX principal_bindings_equivalent_active ON principal_bindings(provider, channel, credential_ref, subject_namespace, verified_subject) WHERE status = 'active'`}},
	}
	for _, a := range attacks {
		a := a
		t.Run(a.name, func(t *testing.T) {
			t.Parallel()
			path, exec := e1Founded(t)
			for _, q := range a.ddl {
				exec(q)
			}
			for _, q := range a.duplicate {
				exec(q) // the calibration: the weakened index admits the duplicate
			}
			before := e2Snapshot(t, path)
			e1Blocked(t, path, profileA, a.index, a.dimension)
			e2Same(t, "the ledger after every opener", before, e2Snapshot(t, path))
		})
	}
}

// TE11 · the eleven index names belong to the action store, and a missing
// PERFORMANCE index is not corruption. The manifest the production judges
// against — the eleven names and the three UNIQUE descriptors — is the fresh
// store's, derived here from a store the real DDL and migrations wrote. On a
// file with no action table, an index on the conversations wearing any of
// the eleven names makes the file bad and nothing is seeded around it. On a
// current ledger, each of the eight non-unique indexes missing leaves every
// opener healthy, and nothing recreates it.
//
// PROBING MUTATIONS (MU11): leave one non-unique name out of the manifest →
// the manifest differs from the fresh store's, and its homonym is seeded
// around → reddens; require the non-unique indexes of a current ledger →
// their removal opens unreadable → reddens.
func TestE2_TE11_theElevenNamesAndThePerformanceIndexes(t *testing.T) {
	ctx := context.Background()
	ref := e2ConversationFile(t)
	h, err := OpenFor(ref, profileA)
	if err != nil {
		t.Fatal(err)
	}
	_ = h.Close()
	raw := e2Raw(t, ref)
	var names []string
	rows, err := raw.Query(`SELECT name FROM sqlite_master WHERE type = 'index' AND sql IS NOT NULL AND lower(tbl_name) NOT IN ('sessions', 'turns', 'notes') ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
	}
	_ = rows.Close()
	t.Run("the manifest is the fresh store's", func(t *testing.T) {
		manifest := append([]string(nil), indexNamesV16...)
		sort.Strings(manifest)
		if strings.Join(manifest, ",") != strings.Join(names, ",") {
			t.Fatalf("the index manifest is not the fresh store's\n got %v\nwant %v", manifest, names)
		}
		for _, d := range uniqueIndexesV16 {
			e2SameDescriptor(t, raw, d)
		}
		var unique []string
		for _, n := range names {
			if e2IsUnique(t, raw, n) {
				unique = append(unique, n)
			}
		}
		var described []string
		for _, d := range uniqueIndexesV16 {
			described = append(described, d.name)
		}
		sort.Strings(described)
		if strings.Join(unique, ",") != strings.Join(described, ",") {
			t.Fatalf("the fresh store's UNIQUE indexes are %v, the manifest describes %v", unique, described)
		}
	})
	t.Run("a homonym on a file with no action table", func(t *testing.T) {
		for _, name := range names {
			path := e2ConversationFile(t)
			e2Lay(t, path, `CREATE INDEX `+name+` ON sessions(key)`)
			before := e2Snapshot(t, path)
			if v, err := judgeShape(ctx, dbShapeQuerier{q: e2Raw(t, path)}); err != nil || v.shape != shapeBad {
				t.Fatalf("%s: the shape = %v (%s) %v, want bad", name, v.shape, v.reason, err)
			}
			w := e2WatchSeed(t, path, nil)
			h, err := OpenFor(path, profileA)
			if err != nil || h == nil {
				t.Fatalf("%s: OpenFor = %v, want a handle that names the shape", name, err)
			}
			st, _, serr := h.Standing(ctx)
			_ = h.Close()
			if st != LedgerStandingUnreadable || serr == nil {
				t.Fatalf("%s: Standing = %q %v, want unreadable", name, st, serr)
			}
			if seen := w.seen(); seen.ddl != 0 || seen.inserts != 0 {
				t.Fatalf("%s: the writer dispatched %d bootstrap DDL and %d version INSERT around the homonym", name, seen.ddl, seen.inserts)
			}
			seedObserverSeam.CompareAndSwap(w, nil)
			e2Same(t, name, before, e2Snapshot(t, path))
		}
	})
	t.Run("a missing performance index on a current ledger", func(t *testing.T) {
		for _, name := range names {
			if e2IsUnique(t, raw, name) {
				continue
			}
			path, exec := e1Founded(t)
			exec(`DROP INDEX ` + name)
			for _, o := range e1Openers {
				h, err := o.open(path, profileA)
				if err != nil {
					t.Fatalf("%s without %s = %v, want the healthy ledger", o.name, name, err)
				}
				st, owner, serr := h.Standing(ctx)
				_ = h.Close()
				if serr != nil || st != LedgerStandingOK || owner != profileA {
					t.Fatalf("%s without %s: Standing = %q %q %v, want ok (a performance index is not evidence)", o.name, name, st, owner, serr)
				}
			}
			if n := rawCount(t, e2Raw(t, path), `SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = '`+name+`'`); n != 0 {
				t.Fatalf("an opener recreated %s", name)
			}
		}
	})
}

// e2IsUnique reads whether the index name is UNIQUE, from its table's
// index list.
func e2IsUnique(t *testing.T, raw *sql.DB, name string) bool {
	t.Helper()
	var table string
	if err := raw.QueryRow(`SELECT tbl_name FROM sqlite_master WHERE type = 'index' AND name = ?`, name).Scan(&table); err != nil {
		t.Fatalf("the table of %s: %v", name, err)
	}
	var unique int
	if err := raw.QueryRow(`SELECT "unique" FROM pragma_index_list(?) WHERE name = ?`, table, name).Scan(&unique); err != nil {
		t.Fatalf("the index list of %s: %v", table, err)
	}
	return unique == 1
}

// e2Words folds a piece of SQL to its words for a comparison that ignores
// spacing only.
func e2Words(s string) string {
	return strings.Join(strings.Fields(strings.NewReplacer("(", " ( ", ")", " ) ", ",", " , ").Replace(s)), " ")
}

// e2SameDescriptor compares one production UNIQUE descriptor with the fresh
// store's index of that name: table, uniqueness, partial flag, every key in
// order (column name, or the expression as the DDL wrote it), collation,
// direction, and the predicate as the DDL wrote it.
func e2SameDescriptor(t *testing.T, raw *sql.DB, d indexDescriptor) {
	t.Helper()
	var table, ddl string
	if err := raw.QueryRow(`SELECT tbl_name, sql FROM sqlite_master WHERE type = 'index' AND name = ?`, d.name).Scan(&table, &ddl); err != nil {
		t.Fatalf("the fresh store has no index %s: %v", d.name, err)
	}
	if !strings.EqualFold(table, d.table) {
		t.Fatalf("%s: the fresh store's is on %s, the manifest says %s", d.name, table, d.table)
	}
	var unique, partial int
	if err := raw.QueryRow(`SELECT "unique", partial FROM pragma_index_list(?) WHERE name = ?`, table, d.name).Scan(&unique, &partial); err != nil {
		t.Fatal(err)
	}
	if unique != 1 || (partial == 1) != (d.where != "") {
		t.Fatalf("%s: unique=%d partial=%d in the fresh store, the manifest says a predicate %q", d.name, unique, partial, d.where)
	}
	lp, rp := strings.Index(ddl, "("), strings.LastIndex(ddl, ")")
	where := ""
	if m := e2WhereWord.FindStringIndex(ddl); m != nil {
		where = strings.TrimSpace(ddl[m[1]:])
		rp = strings.LastIndex(ddl[:m[0]], ")")
	}
	if e2Words(where) != e2Words(d.where) {
		t.Fatalf("%s: the fresh store's predicate is %q, the manifest's %q", d.name, where, d.where)
	}
	var parts []string
	depth, start := 0, lp+1
	for i := lp + 1; i < rp; i++ {
		switch ddl[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(ddl[start:i]))
				start = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(ddl[start:rp]))
	rows, err := raw.Query(`SELECT cid, ifnull(name, ''), "desc", coll FROM pragma_index_xinfo(?) WHERE "key" = 1 ORDER BY seqno`, d.name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	i := 0
	for rows.Next() {
		var cid, desc int
		var name, coll string
		if err := rows.Scan(&cid, &name, &desc, &coll); err != nil {
			t.Fatal(err)
		}
		if i >= len(d.keys) || i >= len(parts) {
			t.Fatalf("%s: the fresh store's index has more keys than the manifest's %d", d.name, len(d.keys))
		}
		k := d.keys[i]
		switch {
		case cid == -2:
			if k.column != "" || e2Words(parts[i]) != e2Words(k.expr) {
				t.Fatalf("%s: key %d is the expression %q in the fresh store, the manifest says %+v", d.name, i+1, parts[i], k)
			}
		case !strings.EqualFold(name, k.column) || k.expr != "":
			t.Fatalf("%s: key %d is the column %q in the fresh store, the manifest says %+v", d.name, i+1, name, k)
		}
		if !strings.EqualFold(coll, k.coll) || (desc == 1) != k.desc {
			t.Fatalf("%s: key %d is %s desc=%d in the fresh store, the manifest says %+v", d.name, i+1, coll, desc, k)
		}
		i++
	}
	if i != len(d.keys) {
		t.Fatalf("%s: the fresh store's index has %d keys, the manifest %d", d.name, i, len(d.keys))
	}
}

// e2WhereWord finds the WHERE of an index's DDL, whatever space precedes it.
var e2WhereWord = regexp.MustCompile(`(?i)\bWHERE\b`)

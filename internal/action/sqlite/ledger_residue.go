// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The action store's objects beyond its tables (train E, plan §4): the eleven
// indexes its schema creates, which share the namespace of tables and views;
// the three UNIQUE indexes a current ledger requires whole; and the residue a
// crash of the historical, statement-by-statement seed left — an EMPTY prefix
// of the v1 bootstrap, which is a fresh file, not a bad one. Every read here
// is one of the judge's (sites Qresidue and Qindex); none repairs anything.

package sqlite

import (
	"context"
	"fmt"
	"strings"
)

// indexNamesV16 are the indexes the v16 schema creates, by name: with
// schemaTablesV16, the names the action store claims in the file's namespace
// of tables, views and indexes. TE11 ties this list to a fresh store's
// catalog.
var indexNamesV16 = []string{
	"actions_by_correlation", "actions_by_requested", "evidence_by_action",
	"receipts_by_action", "approvals_by_status", "execution_bindings_active_selector",
	"principal_bindings_equivalent_active", "grant_events_by_grant",
	"budget_debits_tail", "budget_debits_sequence", "tombstones_by_action",
}

// indexDescriptor is one UNIQUE index a current ledger requires, whole: its
// table, its keys in order and its partial predicate as the DDL writes it
// ("" for none). It describes the schema this binary writes; it is not a
// general proof of SQL equivalence.
type indexDescriptor struct {
	name, table string
	keys        []indexKey
	where       string
}

// indexKey is one key of an index: a column, or an expression as the DDL
// writes it; its collation; its direction.
type indexKey struct {
	column, expr string
	coll         string
	desc         bool
}

// uniqueIndexesV16 are the three UNIQUE indexes of the v16 schema: the
// invariants they keep are the ledger's, so a current ledger requires each
// of them whole. TE11 ties them to a fresh store's catalog.
var uniqueIndexesV16 = []indexDescriptor{
	{name: "execution_bindings_active_selector", table: "execution_bindings", where: "status = 'ACTIVE'",
		keys: []indexKey{{column: "actor_principal_id", coll: "BINARY"}, {column: "channel", coll: "BINARY"}, {expr: "ifnull(conversation_id, '')", coll: "BINARY"}}},
	{name: "principal_bindings_equivalent_active", table: "principal_bindings", where: "status = 'active'",
		keys: []indexKey{{column: "provider", coll: "BINARY"}, {column: "channel", coll: "BINARY"}, {column: "credential_ref", coll: "BINARY"},
			{column: "subject_namespace", coll: "BINARY"}, {column: "verified_subject", coll: "BINARY"}, {column: "principal_id", coll: "BINARY"}}},
	{name: "budget_debits_sequence", table: "budget_debits",
		keys: []indexKey{{column: "account_id", coll: "BINARY"}, {column: "operation_key", coll: "BINARY"}, {column: "sequence", coll: "BINARY"}}},
}

// actionNames are the names the action store claims in the namespace of
// tables, views and indexes; actionTables its tables alone. Both are read
// only.
var (
	actionTables = nameSet(schemaTablesV16)
	actionNames  = nameSet(append(append([]string(nil), schemaTablesV16...), indexNamesV16...))
)

func nameSet(names []string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, n := range names {
		out[n] = true
	}
	return out
}

// catalogObject is one object of the file as the catalog names it, its name
// and target table folded like SQLite folds identifiers (ASCII case).
type catalogObject struct {
	kind, name, table string
	virtual           bool
}

// ownedByTheStore tells whether an object belongs to the action store: a
// table, view or index wearing one of its names — they share one namespace —
// or any object standing on one of its tables. A trigger belongs only by the
// table it stands on: triggers have a namespace of their own, and a trigger
// on the conversations wearing a schema name is the conversations' (P3-1).
func ownedByTheStore(o catalogObject) bool {
	if actionTables[o.table] {
		return true
	}
	return o.kind != "trigger" && actionNames[o.name]
}

// bootstrapObjects are the objects the five bootstrap statements create, in
// their order: a WITHOUT ROWID table keeps its primary key in its own b-tree,
// so none of them brings an automatic index along.
var bootstrapObjects = []catalogObject{
	{kind: "table", name: "action_schema", table: "action_schema"},
	{kind: "table", name: "actions", table: "actions"},
	{kind: "index", name: "actions_by_correlation", table: "actions"},
	{kind: "index", name: "actions_by_requested", table: "actions"},
	{kind: "table", name: "action_decisions", table: "action_decisions"},
}

// bootstrapPrefixOf answers whether the store's objects are, as a set,
// exactly the first k objects of the bootstrap, and which k: whatever order
// the catalog lists them in, with nothing else of the store's beside them.
func bootstrapPrefixOf(owned []catalogObject) (int, bool) {
	k := len(owned)
	if k < 1 || k > len(bootstrapObjects) {
		return 0, false
	}
	want := map[string]bool{}
	for _, b := range bootstrapObjects[:k] {
		want[b.kind+":"+b.name+":"+b.table] = true
	}
	for _, o := range owned {
		key := o.kind + ":" + o.name + ":" + o.table
		if o.virtual || !want[key] {
			return 0, false
		}
		delete(want, key)
	}
	return k, len(want) == 0
}

// emptyBootstrapPrefix proves, through the judge's residue reads, that the
// file's store objects are the first k bootstrap objects AS THE BOOTSTRAP
// WRITES THEM — each object's DDL, token by token, the one its statement
// creates — and that every one of their tables is empty. A read that fails
// is returned for the judge to classify; a proof that does not hold is
// false, and the file is judged like any other.
func emptyBootstrapPrefix(ctx context.Context, q shapeQuerier, k int) (bool, error) {
	records, err := q.records(ctx, siteResidue, `SELECT type, name, tbl_name, sql FROM sqlite_master`)
	if err != nil {
		return false, err
	}
	want := map[string]string{}
	for i, b := range bootstrapObjects[:k] {
		want[b.kind+":"+b.name+":"+b.table] = strings.Join(createTokens(bootstrapStatements[i]), " ")
	}
	var tables []string
	for _, r := range records {
		if len(r) != 4 {
			return false, nil
		}
		o := catalogObject{kind: driverText(r[0]), name: asciiLower(driverText(r[1])), table: asciiLower(driverText(r[2]))}
		if !ownedByTheStore(o) {
			continue
		}
		key := o.kind + ":" + o.name + ":" + o.table
		ddl, ok := want[key]
		if !ok || r[3] == nil || strings.Join(sqlTokens(driverText(r[3])), " ") != ddl {
			return false, nil
		}
		delete(want, key)
		if o.kind == "table" {
			tables = append(tables, driverText(r[1]))
		}
	}
	if len(want) != 0 {
		return false, nil
	}
	for _, table := range tables {
		rows, err := q.scalars(ctx, siteResidue, `SELECT 1 FROM "`+strings.ReplaceAll(table, `"`, `""`)+`" LIMIT 1`)
		if err != nil {
			return false, err
		}
		if len(rows) != 0 {
			return false, nil
		}
	}
	return true, nil
}

// uniqueIndexProblem judges the three UNIQUE indexes a current ledger
// requires, in ONE of the judge's index reads (uniqueIndexQuery): "" when
// every one is whole, else the first index and the dimension that differs —
// missing, its table, its uniqueness, its keys (count, order, a column's
// name), the expression, collation or direction of a key, its partial
// predicate. Uniqueness, table, keys, collation and direction come from the
// catalog's metadata; an expression and the predicate from the index's DDL,
// token by token. A read that fails is returned for the judge to classify.
func uniqueIndexProblem(ctx context.Context, q shapeQuerier) (string, error) {
	records, err := q.records(ctx, siteIndex, uniqueIndexQuery)
	if err != nil {
		return "", err
	}
	type keyRow struct{ cid, name, desc, coll string }
	type indexRow struct {
		table, ddl, unique string
		keys               []keyRow
	}
	found := map[string]*indexRow{}
	for _, r := range records {
		if len(r) != 8 {
			continue
		}
		name := asciiLower(driverText(r[0]))
		ix := found[name]
		if ix == nil {
			ix = &indexRow{table: driverText(r[1]), ddl: driverText(r[2]), unique: driverText(r[3])}
			found[name] = ix
		}
		if r[4] != nil {
			ix.keys = append(ix.keys, keyRow{cid: driverText(r[4]), name: driverText(r[5]), desc: driverText(r[6]), coll: driverText(r[7])})
		}
	}
	for _, d := range uniqueIndexesV16 {
		ix, ok := found[d.name]
		if !ok {
			return fmt.Sprintf("index %s is missing", d.name), nil
		}
		if asciiLower(ix.table) != d.table {
			return fmt.Sprintf("index %s: its table is %s, want %s", d.name, ix.table, d.table), nil
		}
		if ix.unique != "1" {
			return fmt.Sprintf("index %s: not unique", d.name), nil
		}
		columns, where, parsed := indexShape(sqlTokens(ix.ddl))
		if len(ix.keys) != len(d.keys) || !parsed || len(columns) != len(ix.keys) {
			return fmt.Sprintf("index %s: its keys differ", d.name), nil
		}
		for i, k := range d.keys {
			r := ix.keys[i]
			if k.expr == "" {
				if r.cid == "-2" || asciiLower(r.name) != k.column {
					return fmt.Sprintf("index %s: its keys differ", d.name), nil
				}
			} else {
				if r.cid != "-2" {
					return fmt.Sprintf("index %s: its keys differ", d.name), nil
				}
				if strings.Join(columns[i], " ") != strings.Join(sqlTokens(k.expr), " ") {
					return fmt.Sprintf("index %s: the expression of key %d differs", d.name, i+1), nil
				}
			}
			if !strings.EqualFold(r.coll, k.coll) {
				return fmt.Sprintf("index %s: the collation of key %d differs", d.name, i+1), nil
			}
			if (r.desc == "1") != k.desc {
				return fmt.Sprintf("index %s: the direction of key %d differs", d.name, i+1), nil
			}
		}
		if strings.Join(where, " ") != strings.Join(sqlTokens(d.where), " ") {
			return fmt.Sprintf("index %s: its partial predicate differs", d.name), nil
		}
	}
	return "", nil
}

// uniqueIndexQuery reads, in one query, every index wearing the name of one
// of uniqueIndexesV16: its table and DDL, its UNIQUE bit from its table's
// index list, and its key columns in order from its extended information.
var uniqueIndexQuery = func() string {
	names := make([]string, 0, len(uniqueIndexesV16))
	for _, d := range uniqueIndexesV16 {
		names = append(names, sqlString(d.name))
	}
	return `SELECT m.name, m.tbl_name, m.sql, l."unique", x.cid, x.name, x."desc", x.coll
  FROM sqlite_master AS m
  LEFT JOIN pragma_index_list(m.tbl_name) AS l ON l.name = m.name
  LEFT JOIN pragma_index_xinfo(m.name) AS x ON x."key" = 1
 WHERE m.type = 'index' AND lower(m.name) IN (` + strings.Join(names, ", ") + `)
 ORDER BY m.name, x.seqno`
}()

// indexShape reads a CREATE INDEX statement's tokens: each key's own tokens
// (without a trailing COLLATE name or ASC/DESC, which the catalog's metadata
// judges) and the predicate's. parsed is false when the tokens are not an
// index's.
func indexShape(tokens []string) (keys [][]string, where []string, parsed bool) {
	on := -1
	for i, tk := range tokens {
		if tk == "on" {
			on = i
			break
		}
	}
	if on < 0 || on+2 >= len(tokens) || tokens[on+2] != "(" {
		return nil, nil, false
	}
	depth, start := 0, on+3
	end := -1
	for i := on + 2; i < len(tokens); i++ {
		switch tokens[i] {
		case "(":
			depth++
		case ")":
			depth--
			if depth == 0 {
				end = i
			}
		case ",":
			if depth == 1 {
				keys = append(keys, stripKeySuffix(tokens[start:i]))
				start = i + 1
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		return nil, nil, false
	}
	keys = append(keys, stripKeySuffix(tokens[start:end]))
	rest := tokens[end+1:]
	if len(rest) > 0 {
		if rest[0] != "where" {
			return nil, nil, false
		}
		where = rest[1:]
	}
	return keys, where, true
}

// stripKeySuffix drops a key's trailing ASC/DESC and COLLATE name.
func stripKeySuffix(key []string) []string {
	if n := len(key); n > 0 && (key[n-1] == "asc" || key[n-1] == "desc") {
		key = key[:n-1]
	}
	if n := len(key); n >= 2 && key[n-2] == "collate" {
		key = key[:n-2]
	}
	return key
}

// createTokens are a bootstrap statement's tokens as SQLite keeps them in
// its catalog: without the IF NOT EXISTS the catalog does not store.
func createTokens(stmt string) []string {
	tokens := sqlTokens(stmt)
	i := 1
	if len(tokens) > i && tokens[i] == "unique" {
		i++
	}
	if len(tokens) > i+3 && tokens[0] == "create" && (tokens[i] == "table" || tokens[i] == "index") &&
		tokens[i+1] == "if" && tokens[i+2] == "not" && tokens[i+3] == "exists" {
		return append(append([]string(nil), tokens[:i+1]...), tokens[i+4:]...)
	}
	return tokens
}

// sqlTokens cuts SQL into the tokens a comparison of this schema's own DDL
// needs: identifiers and keywords folded to lower case — SQLite resolves
// them without regard to ASCII case, quoted or not —, string literals kept
// byte for byte (they are data), numbers, and every other character alone.
// Spacing and comments are not tokens.
func sqlTokens(s string) []string {
	var out []string
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v':
			i++
		case c == '-' && i+1 < len(s) && s[i+1] == '-':
			for i < len(s) && s[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			if end := strings.Index(s[i+2:], "*/"); end >= 0 {
				i += end + 4
			} else {
				i = len(s)
			}
		case c == '\'':
			j := i + 1
			for j < len(s) && (s[j] != '\'' || (j+1 < len(s) && s[j+1] == '\'')) {
				if s[j] == '\'' {
					j++
				}
				j++
			}
			if j >= len(s) {
				out = append(out, s[i:])
				i = len(s)
				continue
			}
			out = append(out, s[i:j+1])
			i = j + 1
		case c == '"' || c == '`' || c == '[':
			closing := c
			if c == '[' {
				closing = ']'
			}
			var b strings.Builder
			j := i + 1
			for j < len(s) {
				if s[j] == closing {
					if closing != ']' && j+1 < len(s) && s[j+1] == closing {
						b.WriteByte(closing)
						j += 2
						continue
					}
					break
				}
				b.WriteByte(s[j])
				j++
			}
			out = append(out, asciiLower(b.String()))
			i = j + 1
		case identByte(c):
			j := i + 1
			for j < len(s) && (identByte(s[j]) || (s[j] >= '0' && s[j] <= '9') || s[j] == '$') {
				j++
			}
			out = append(out, asciiLower(s[i:j]))
			i = j
		case c >= '0' && c <= '9':
			j := i + 1
			for j < len(s) && ((s[j] >= '0' && s[j] <= '9') || s[j] == '.') {
				j++
			}
			out = append(out, s[i:j])
			i = j
		default:
			out = append(out, s[i:i+1])
			i++
		}
	}
	return out
}

// identByte tells a byte that starts or continues an identifier: a letter,
// an underscore, or any byte of a non-ASCII character.
func identByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

// asciiLower folds ASCII letters to lower case and nothing else, as SQLite
// folds identifiers.
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

// sqlString quotes s as an SQL string literal.
func sqlString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

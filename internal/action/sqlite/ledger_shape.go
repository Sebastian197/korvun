// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The ledger's SHAPE, judged in ONE place (train D, 2026-09-25). Three
// trains in a row found a product defect in the same class — the hook, the
// first line and the opener each with its own idea of «corrupt» — so the
// shape of a file is decided here, once, and the connection hook, the
// openers and the readers all consult it. A bad shape is opened unreadable
// and never repaired. A fresh action store — nothing of the store's in the
// file, whatever else the shared file carries, or only an EMPTY prefix of
// its v1 bootstrap, the residue of a seed that died between its statements
// (train E) — is seeded. A read of the shape that fails with a structural
// code (train E: ERROR, CORRUPT, MISMATCH, FORMAT, NOTADB) is a bad shape
// carrying its cause; any other failed read is an error of the moment, never
// a verdict.

package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	msqlite "modernc.org/sqlite"
)

// ledgerShape is what a file looks like to the action store.
type ledgerShape int

const (
	// shapeFresh has nothing of the action store's, or only an empty prefix
	// of its v1 bootstrap: the open sequence seeds the schema. The file may
	// carry the conversation store's tables and triggers.
	shapeFresh ledgerShape = iota
	// shapeOlder has action_schema at a version below this binary's: the
	// migration comes next, and which tables that version has is the
	// migration's business, not the shape's.
	shapeOlder
	// shapeCurrent has action_schema at this binary's version, every table
	// of the schema present as an ordinary table, and its three UNIQUE
	// indexes whole (uniqueIndexesV16). A missing performance index is not
	// judged.
	shapeCurrent
	// shapeNewer has action_schema at a version above this binary's.
	shapeNewer
	// shapeBad is every other shape: action-store objects without
	// action_schema that are not an empty bootstrap prefix, zero or several
	// version rows, a version that is not a number or is below 1, or — at
	// the current version — a table of the schema missing or virtual, or one
	// of its UNIQUE indexes not whole.
	shapeBad
)

// shapeVerdict is judgeShape's answer: the shape, the reason in words (the
// table that is missing, the version that does not convert), the failed read
// behind a structural verdict (cause, nil when the file was read), the
// version read (0 when none), and the TABLES found, by lower-cased name
// (SQLite resolves identifiers without regard to case; so does the shape).
// The cause reaches only the callers that return an error; it is never
// stored.
type shapeVerdict struct {
	shape   ledgerShape
	reason  string
	cause   error
	version int
	present map[string]bool
}

// unreadable is the verdict as the error the store names it with: the
// reason, and the cause when a read failed.
func (v shapeVerdict) unreadable() error {
	if v.cause != nil {
		return fmt.Errorf("%w: %s: %w", ErrLedgerUnreadable, v.reason, v.cause)
	}
	return fmt.Errorf("%w: %s", ErrLedgerUnreadable, v.reason)
}

// shapeQuerier is one query, every row's first column as text (strings) or
// as the value the storage holds (scalars), or every column of every row as
// the storage holds them (records): the driver connection the hook gets, or
// the connections and transactions everyone else has. The site names the
// read for the judge's instrumentation (siteNone for a read that is not the
// judge's).
type shapeQuerier interface {
	strings(ctx context.Context, site judgeQuerySite, query string) ([]string, error)
	scalars(ctx context.Context, site judgeQuerySite, query string) ([]any, error)
	records(ctx context.Context, site judgeQuerySite, query string) ([][]any, error)
}

// driverShapeQuerier reads through the driver's own connection.
type driverShapeQuerier struct{ conn msqlite.ExecQuerierContext }

func (d driverShapeQuerier) strings(ctx context.Context, site judgeQuerySite, query string) ([]string, error) {
	rows, err := d.conn.QueryContext(ctx, query, nil)
	if err = judgeRead(ctx, site, stageQuery, err); err != nil {
		if rows != nil {
			_ = rows.Close()
		}
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for {
		dest := make([]driver.Value, len(rows.Columns()))
		err := judgeRead(ctx, site, stageNext, rows.Next(dest))
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		out = append(out, driverText(dest[0]))
	}
}

func (d driverShapeQuerier) scalars(ctx context.Context, site judgeQuerySite, query string) ([]any, error) {
	rows, err := d.conn.QueryContext(ctx, query, nil)
	if err = judgeRead(ctx, site, stageQuery, err); err != nil {
		if rows != nil {
			_ = rows.Close()
		}
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []any
	for {
		dest := make([]driver.Value, len(rows.Columns()))
		err := judgeRead(ctx, site, stageNext, rows.Next(dest))
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		if b, ok := dest[0].([]byte); ok {
			dest[0] = append([]byte(nil), b...)
		}
		out = append(out, dest[0])
	}
}

func (d driverShapeQuerier) records(ctx context.Context, site judgeQuerySite, query string) ([][]any, error) {
	rows, err := d.conn.QueryContext(ctx, query, nil)
	if err = judgeRead(ctx, site, stageQuery, err); err != nil {
		if rows != nil {
			_ = rows.Close()
		}
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out [][]any
	for {
		dest := make([]driver.Value, len(rows.Columns()))
		err := judgeRead(ctx, site, stageNext, rows.Next(dest))
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		row := make([]any, len(dest))
		for i, v := range dest {
			if b, ok := v.([]byte); ok {
				v = append([]byte(nil), b...)
			}
			row[i] = v
		}
		out = append(out, row)
	}
}

func driverText(v driver.Value) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return fmt.Sprint(x) // an int64 count, or a NULL that no rule accepts
	}
}

// dbShapeQuerier reads through a connection or an open transaction; handed
// the pool itself, it obtains one connection for the read first
// (withLedgerConnection), so a connection that cannot be born is never taken
// for a failed read.
type dbShapeQuerier struct{ q rowsQuerier }

func (d dbShapeQuerier) strings(ctx context.Context, site judgeQuerySite, query string) ([]string, error) {
	if db, ok := d.q.(*sql.DB); ok {
		var out []string
		err := withLedgerConnection(ctx, db, func(c *sql.Conn) error {
			var rerr error
			out, rerr = dbShapeQuerier{q: c}.strings(ctx, site, query)
			return rerr
		})
		return out, err
	}
	rows, err := d.q.QueryContext(ctx, query)
	if err = judgeRead(ctx, site, stageQuery, err); err != nil {
		if rows != nil {
			_ = rows.Close()
		}
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var v sql.NullString
		if err := judgeRead(ctx, site, stageScan, rows.Scan(&v)); err != nil {
			return nil, err
		}
		out = append(out, v.String)
	}
	return out, judgeRead(ctx, site, stageNext, rows.Err())
}

func (d dbShapeQuerier) scalars(ctx context.Context, site judgeQuerySite, query string) ([]any, error) {
	if db, ok := d.q.(*sql.DB); ok {
		var out []any
		err := withLedgerConnection(ctx, db, func(c *sql.Conn) error {
			var rerr error
			out, rerr = dbShapeQuerier{q: c}.scalars(ctx, site, query)
			return rerr
		})
		return out, err
	}
	rows, err := d.q.QueryContext(ctx, query)
	if err = judgeRead(ctx, site, stageQuery, err); err != nil {
		if rows != nil {
			_ = rows.Close()
		}
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []any
	for rows.Next() {
		var v any
		if err := judgeRead(ctx, site, stageScan, rows.Scan(&v)); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, judgeRead(ctx, site, stageNext, rows.Err())
}

func (d dbShapeQuerier) records(ctx context.Context, site judgeQuerySite, query string) ([][]any, error) {
	if db, ok := d.q.(*sql.DB); ok {
		var out [][]any
		err := withLedgerConnection(ctx, db, func(c *sql.Conn) error {
			var rerr error
			out, rerr = dbShapeQuerier{q: c}.records(ctx, site, query)
			return rerr
		})
		return out, err
	}
	rows, err := d.q.QueryContext(ctx, query)
	if err = judgeRead(ctx, site, stageQuery, err); err != nil {
		if rows != nil {
			_ = rows.Close()
		}
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out [][]any
	for rows.Next() {
		row := make([]any, len(cols))
		dest := make([]any, len(cols))
		for i := range row {
			dest[i] = &row[i]
		}
		if err := judgeRead(ctx, site, stageScan, rows.Scan(dest...)); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, judgeRead(ctx, site, stageNext, rows.Err())
}

// judgeShape classifies the file behind q. A read that fails with a
// structural code is a verdict — shapeBad, the reason and the failure as its
// cause — and no error; any other failure comes back as an error and no
// shape: the caller treats it as a failure of the moment, never as a verdict
// on the file.
func judgeShape(ctx context.Context, q shapeQuerier) (shapeVerdict, error) {
	// Every object of the file, by type, kind, target table and name, folded
	// like SQLite folds identifiers. A table of the schema counts as present
	// only as a TABLE; a VIRTUAL table is a table whose kind the current
	// shape refuses. What belongs to the store (ownedByTheStore): a table,
	// view or index wearing one of its names, or any object on one of its
	// tables — never a trigger by its name alone.
	objects, err := q.strings(ctx, siteCatalog, `SELECT type || ':' || (CASE WHEN sql LIKE 'CREATE VIRTUAL TABLE%' THEN 'virtual' ELSE '' END) || ':' || hex(tbl_name) || ':' || name FROM sqlite_master`)
	if err != nil {
		if readVerdict(siteCatalog, err) {
			return shapeVerdict{shape: shapeBad, reason: "the catalog cannot be read", cause: err}, nil
		}
		return shapeVerdict{shape: shapeBad}, fmt.Errorf("action/sqlite: read the catalog: %w", err)
	}
	present, virtual := map[string]bool{}, map[string]bool{}
	var owned []catalogObject
	for _, o := range objects {
		kind, rest, _ := strings.Cut(o, ":")
		flag, rest, _ := strings.Cut(rest, ":")
		tableHex, name, _ := strings.Cut(rest, ":")
		table, herr := hex.DecodeString(tableHex)
		if herr != nil {
			table = []byte(tableHex)
		}
		obj := catalogObject{kind: kind, name: asciiLower(name), table: asciiLower(string(table)), virtual: flag == "virtual"}
		if kind == "table" {
			present[obj.name] = true
			if obj.virtual {
				virtual[obj.name] = true
			}
		}
		if ownedByTheStore(obj) {
			owned = append(owned, obj)
		}
	}
	v := shapeVerdict{present: present}
	if len(owned) == 0 {
		v.shape, v.reason = shapeFresh, "no action-store object"
		return v, nil
	}
	// The residue of a seed that died between its statements (plan §4): an
	// empty prefix of the v1 bootstrap, proved object by object and row by
	// row, is fresh; anything else of the store's is judged as it is.
	if k, ok := bootstrapPrefixOf(owned); ok {
		empty, err := emptyBootstrapPrefix(ctx, q, k)
		if err != nil {
			if readVerdict(siteResidue, err) {
				v.shape, v.reason, v.cause = shapeBad, "the bootstrap residue cannot be read", err
				return v, nil
			}
			return shapeVerdict{shape: shapeBad}, fmt.Errorf("action/sqlite: read the bootstrap residue: %w", err)
		}
		if empty {
			v.shape, v.reason = shapeFresh, fmt.Sprintf("an empty prefix of the bootstrap: %d of its %d statements", k, len(bootstrapObjects))
			return v, nil
		}
	}
	if !present["action_schema"] {
		v.shape, v.reason = shapeBad, "action-store objects without the action_schema table"
		return v, nil
	}
	versions, err := q.scalars(ctx, siteVersion, `SELECT version FROM action_schema`)
	if err != nil {
		if readVerdict(siteVersion, err) {
			v.shape, v.reason, v.cause = shapeBad, "action_schema.version cannot be read", err
			return v, nil
		}
		return shapeVerdict{shape: shapeBad}, fmt.Errorf("action/sqlite: read action_schema: %w", err)
	}
	if len(versions) != 1 {
		v.shape, v.reason = shapeBad, fmt.Sprintf("action_schema has %d rows, want 1", len(versions))
		return v, nil
	}
	version, err := parseStoredVersion(versions[0])
	if err != nil {
		v.shape, v.reason = shapeBad, err.Error()
		return v, nil
	}
	v.version = version
	switch {
	case version < 1:
		v.shape, v.reason = shapeBad, fmt.Sprintf("action_schema.version %d is below 1", version)
	case version < schemaVersionCurrent:
		// Which tables an older schema has is that version's business (the
		// migration's): the shape asks nothing more of it. A downgraded
		// fixture carrying a newer table is «older», not bad.
		v.shape, v.reason = shapeOlder, fmt.Sprintf("schema v%d, older than this binary's v%d", version, schemaVersionCurrent)
	case version == schemaVersionCurrent:
		for _, t := range schemaTablesV16 {
			if !present[t] {
				v.shape, v.reason = shapeBad, fmt.Sprintf("table %s missing at schema v%d", t, version)
				return v, nil
			}
			if virtual[t] {
				v.shape, v.reason = shapeBad, fmt.Sprintf("table %s is a virtual table at schema v%d", t, version)
				return v, nil
			}
		}
		why, err := uniqueIndexProblem(ctx, q)
		if err != nil {
			if readVerdict(siteIndex, err) {
				v.shape, v.reason, v.cause = shapeBad, "the UNIQUE indexes cannot be read", err
				return v, nil
			}
			return shapeVerdict{shape: shapeBad}, fmt.Errorf("action/sqlite: read the indexes: %w", err)
		}
		if why != "" {
			v.shape, v.reason = shapeBad, why
			return v, nil
		}
		v.shape, v.reason = shapeCurrent, fmt.Sprintf("schema v%d", version)
	default:
		v.shape, v.reason = shapeNewer, fmt.Sprintf("schema v%d, newer than this binary's v%d", version, schemaVersionCurrent)
	}
	return v, nil
}

// SchemaTablesForTest is the schema's table list for the moulds of other
// packages (the app's boot over every bad shape); it hands out a copy.
func SchemaTablesForTest() []string {
	return append([]string(nil), schemaTablesV16...)
}

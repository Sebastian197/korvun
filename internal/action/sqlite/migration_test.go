// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Migration contract — Etapa 2, lote 3, pieza 1 (spec FR-ENV-1 + AS-8):
// the kernel store's FIRST real migration, v1→v2. Transactional,
// versioned, idempotent: a crash MID-migration leaves either a clean v1
// (next boot completes it) or a committed v2 — NEVER a zombie schema.
// Existing v1 rows stay readable with NULL identity. Approved-red
// contract.

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
)

// rawV1Schema is the v1 DDL restated LITERALLY in the test — the oracle
// discipline: a hand-built v1 file, independent of production code, so
// the migration is exercised against what v1 actually shipped.
const rawV1Schema = `
CREATE TABLE action_schema (version INTEGER NOT NULL);
CREATE TABLE actions (
    action_id         TEXT    NOT NULL PRIMARY KEY,
    schema_version    INTEGER NOT NULL,
    correlation_id    TEXT    NOT NULL,
    source_kind       TEXT    NOT NULL,
    source_protocol   TEXT    NOT NULL,
    source_channel    TEXT    NOT NULL,
    op_namespace      TEXT    NOT NULL,
    op_name           TEXT    NOT NULL,
    op_version        INTEGER NOT NULL,
    parameters_digest TEXT    NOT NULL,
    effect_class      TEXT    NOT NULL,
    state             TEXT    NOT NULL,
    recovery_marker   TEXT    NOT NULL DEFAULT '',
    requested_at      TEXT    NOT NULL,
    finished_at       TEXT
) WITHOUT ROWID;
CREATE INDEX actions_by_correlation ON actions(correlation_id);
CREATE INDEX actions_by_requested ON actions(requested_at);
CREATE TABLE action_decisions (
    action_id  TEXT NOT NULL PRIMARY KEY REFERENCES actions(action_id) ON DELETE CASCADE,
    outcome    TEXT NOT NULL,
    rule       TEXT NOT NULL,
    decided_at TEXT NOT NULL
) WITHOUT ROWID;
INSERT INTO action_schema (version) VALUES (1);`

// buildV1File hand-builds a real v1 store file with one terminal action
// row, bypassing production Open entirely.
func buildV1File(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "korvun.db")
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(rawV1Schema); err != nil {
		t.Fatalf("build v1 schema: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO actions (action_id, schema_version, correlation_id,
		    source_kind, source_protocol, source_channel,
		    op_namespace, op_name, op_version,
		    parameters_digest, effect_class, state, requested_at, finished_at)
		 VALUES ('act_v1', 1, 'corr-1', 'channel', 'telegram', 'main',
		    'tool', 'calc', 1, 'sha256:abc', 'read',
		    'SUCCEEDED', '2026-08-29T10:00:00Z', '2026-08-29T10:00:01Z')`,
	); err != nil {
		t.Fatalf("insert v1 action: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO action_decisions (action_id, outcome, rule, decided_at)
		 VALUES ('act_v1', 'allow', 'granted', '2026-08-29T10:00:00Z')`,
	); err != nil {
		t.Fatalf("insert v1 decision: %v", err)
	}
	return path
}

// inspect runs one scalar query against the file with a raw connection.
func inspect(t *testing.T, path, query string) int {
	t.Helper()
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRow(query).Scan(&n); err != nil {
		t.Fatalf("inspect %q: %v", query, err)
	}
	return n
}

func countV2Artifacts(t *testing.T, path string) (tables, columns int) {
	t.Helper()
	tables = inspect(t, path,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table'
		  AND name IN ('intents','grants','evidence','budget_spent')`)
	columns = inspect(t, path,
		`SELECT COUNT(*) FROM pragma_table_info('actions')
		  WHERE name IN ('principal_id','intent_id','authority_refs')`)
	return tables, columns
}

func TestMigration_freshFileLandsOnV2(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "korvun.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	v, err := store.SchemaVersion(context.Background())
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if v != 2 {
		t.Fatalf("a fresh store lands on v2, got %d", v)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	tables, columns := countV2Artifacts(t, path)
	if tables != 4 || columns != 3 {
		t.Fatalf("v2 artifacts incomplete: %d/4 tables, %d/3 columns", tables, columns)
	}
}

func TestMigration_v1FileMigratesAndOldRowsRemainReadable(t *testing.T) {
	t.Parallel()
	path := buildV1File(t)
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open over v1: %v", err)
	}
	defer func() { _ = store.Close() }()
	v, err := store.SchemaVersion(context.Background())
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if v != 2 {
		t.Fatalf("v1 file must migrate to 2, got %d", v)
	}
	rec, err := store.Get(context.Background(), "act_v1")
	if err != nil {
		t.Fatalf("the v1 row must remain readable: %v", err)
	}
	if rec.State != action.StateSucceeded || rec.Decision.Rule != "granted" {
		t.Fatalf("v1 row corrupted by migration: %+v", rec)
	}
}

func TestMigration_idempotentAcrossReopens(t *testing.T) {
	t.Parallel()
	path := buildV1File(t)
	for i := 0; i < 3; i++ {
		store, err := Open(path)
		if err != nil {
			t.Fatalf("Open %d: %v", i, err)
		}
		v, err := store.SchemaVersion(context.Background())
		if err != nil {
			t.Fatalf("version %d: %v", i, err)
		}
		if v != 2 {
			t.Fatalf("reopen %d: version = %d", i, v)
		}
		if err := store.Close(); err != nil {
			t.Fatalf("close %d: %v", i, err)
		}
	}
}

// TestMigration_crashMidMigrationNeverLeavesAZombie is AS-8: the
// migration transaction aborts at its LAST statement (the version bump),
// AFTER all the DDL ran — the worst crash point. The file must remain a
// CLEAN v1 (no v2 artifact survives the rollback), and the next boot
// must complete the migration.
func TestMigration_crashMidMigrationNeverLeavesAZombie(t *testing.T) {
	t.Parallel()
	path := buildV1File(t)
	// Install the crash: a RAISE trigger on the version bump.
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := db.Exec(
		`CREATE TRIGGER crash_mid_migration BEFORE UPDATE ON action_schema
		 BEGIN SELECT RAISE(ABORT, 'crash mid-migration'); END;`,
	); err != nil {
		t.Fatalf("install crash trigger: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}
	// The interrupted boot must fail loudly — never open over a zombie.
	if _, err := Open(path); err == nil {
		t.Fatal("an aborted migration must be boot-fatal, not silent")
	}
	// The file is a CLEAN v1: version untouched, ZERO v2 artifacts.
	if v := inspect(t, path, `SELECT version FROM action_schema`); v != 1 {
		t.Fatalf("aborted migration must leave version 1, got %d", v)
	}
	tables, columns := countV2Artifacts(t, path)
	if tables != 0 || columns != 0 {
		t.Fatalf("ZOMBIE SCHEMA: %d v2 tables, %d v2 columns survived the rollback", tables, columns)
	}
	// Clear the crash; the next life completes the migration cleanly.
	db2, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw 2: %v", err)
	}
	if _, err := db2.Exec(`DROP TRIGGER crash_mid_migration`); err != nil {
		t.Fatalf("drop crash trigger: %v", err)
	}
	if err := db2.Close(); err != nil {
		t.Fatalf("close raw 2: %v", err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatalf("the next boot must complete the migration: %v", err)
	}
	defer func() { _ = store.Close() }()
	v, err := store.SchemaVersion(context.Background())
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if v != 2 {
		t.Fatalf("completed migration must land on 2, got %d", v)
	}
	if _, err := store.Get(context.Background(), "act_v1"); err != nil {
		t.Fatalf("the v1 row must survive the crash-and-complete cycle: %v", err)
	}
}

func TestMigration_futureSchemaFailsClosed(t *testing.T) {
	t.Parallel()
	path := buildV1File(t)
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := db.Exec(`UPDATE action_schema SET version = 99`); err != nil {
		t.Fatalf("set future version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}
	_, err = Open(path)
	if err == nil {
		t.Fatal("a schema from the future must fail closed, not be guessed at")
	}
	if !errors.Is(err, ErrSchemaFromTheFuture) && !strings.Contains(err.Error(), "future") {
		t.Fatalf("the failure must name the future schema, got %v", err)
	}
}

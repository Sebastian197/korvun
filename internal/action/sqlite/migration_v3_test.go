// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Migration v2→v3 contract — Etapa 3, lote 3, pieza 2 (spec FR-CEIL-1
// persistence): the grants table gains the additive effect_ceiling
// column on the E2 anti-zombie runner — one transaction per step, the
// AS-8 crash mold re-armed for v3. Existing v2 grants stay readable with
// an empty (unlimited) ceiling; ceilinged grants round-trip whole; the
// store's delegation frontier now judges the tenth dimension too.
// Approved-red contract.

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

// rawV2Delta is the v1→v2 DDL restated LITERALLY (the oracle discipline:
// a hand-built v2 file, independent of the production migration text).
const rawV2Delta = `
ALTER TABLE actions ADD COLUMN principal_id TEXT;
ALTER TABLE actions ADD COLUMN intent_id TEXT;
ALTER TABLE actions ADD COLUMN authority_refs TEXT;
CREATE TABLE intents (
    intent_id          TEXT    NOT NULL PRIMARY KEY,
    schema_version     INTEGER NOT NULL,
    owner_principal_id TEXT    NOT NULL,
    purpose            TEXT    NOT NULL,
    operations         TEXT    NOT NULL,
    resources          TEXT    NOT NULL,
    max_actions        INTEGER NOT NULL,
    per_operation      TEXT    NOT NULL,
    valid_from         TEXT    NOT NULL,
    expires_at         TEXT,
    status             TEXT    NOT NULL,
    version            INTEGER NOT NULL,
    digest             TEXT    NOT NULL
) WITHOUT ROWID;
CREATE TABLE grants (
    grant_id             TEXT    NOT NULL PRIMARY KEY,
    intent_id            TEXT    NOT NULL REFERENCES intents(intent_id),
    issuer_principal_id  TEXT    NOT NULL,
    subject_principal_id TEXT    NOT NULL,
    parent_grant_id      TEXT    NOT NULL DEFAULT '',
    operations           TEXT    NOT NULL,
    resources            TEXT    NOT NULL,
    max_actions          INTEGER NOT NULL,
    per_operation        TEXT    NOT NULL,
    valid_from           TEXT    NOT NULL,
    expires_at           TEXT,
    status               TEXT    NOT NULL,
    depth_remaining      INTEGER NOT NULL,
    digest               TEXT    NOT NULL
) WITHOUT ROWID;
CREATE TABLE evidence (
    evidence_id       TEXT NOT NULL PRIMARY KEY,
    action_id         TEXT NOT NULL REFERENCES actions(action_id) ON DELETE CASCADE,
    provider          TEXT NOT NULL,
    subject           TEXT NOT NULL,
    credential        TEXT NOT NULL,
    issued_at         TEXT NOT NULL,
    transport_binding TEXT NOT NULL,
    claims_digest     TEXT NOT NULL
) WITHOUT ROWID;
CREATE INDEX evidence_by_action ON evidence(action_id);
CREATE TABLE budget_spent (
    scope_id  TEXT    NOT NULL,
    operation TEXT    NOT NULL,
    spent     INTEGER NOT NULL,
    PRIMARY KEY (scope_id, operation)
) WITHOUT ROWID;
UPDATE action_schema SET version = 2;`

// buildV2File hand-builds a real v2 store file with one stored intent
// and one uncapped grant, bypassing production Open entirely.
func buildV2File(t *testing.T) string {
	t.Helper()
	path := buildV1File(t)
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(rawV2Delta); err != nil {
		t.Fatalf("build v2 delta: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO intents (intent_id, schema_version, owner_principal_id, purpose,
		    operations, resources, max_actions, per_operation, valid_from, expires_at,
		    status, version, digest)
		 VALUES ('int_v2', 1, 'principal_operator', 'v2 era', '["calc"]', '["*"]',
		    0, '', '2026-08-30T00:00:00Z', NULL, 'ACTIVE', 1, 'sha256:v2')`,
	); err != nil {
		t.Fatalf("insert v2 intent: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO grants (grant_id, intent_id, issuer_principal_id,
		    subject_principal_id, parent_grant_id, operations, resources,
		    max_actions, per_operation, valid_from, expires_at, status,
		    depth_remaining, digest)
		 VALUES ('grant_v2', 'int_v2', 'principal_operator', 'principal_brain_a',
		    '', '["calc"]', '["*"]', 5, '', '2026-08-30T00:00:00Z', NULL,
		    'ACTIVE', 1, 'sha256:g2')`,
	); err != nil {
		t.Fatalf("insert v2 grant: %v", err)
	}
	return path
}

func TestMigrationV3_freshFileLandsOnV3WithTheCeilingColumn(t *testing.T) {
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
	// Number-independent by the established precedent: a fresh store
	// lands on the CURRENT version (the ceiling column check below is
	// this test's real subject).
	if v != schemaVersionCurrent {
		t.Fatalf("a fresh store lands on the current version, got %d", v)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if n := inspect(t, path,
		`SELECT COUNT(*) FROM pragma_table_info('grants') WHERE name = 'effect_ceiling'`); n != 1 {
		t.Fatalf("the effect_ceiling column must exist, got %d", n)
	}
}

func TestMigrationV3_v2FileMigratesAndOldGrantsReadUnlimited(t *testing.T) {
	t.Parallel()
	path := buildV2File(t)
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open over v2: %v", err)
	}
	defer func() { _ = store.Close() }()
	v, err := store.SchemaVersion(context.Background())
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if v != schemaVersionCurrent {
		t.Fatalf("v2 file must migrate to the current version, got %d", v)
	}
	grant, err := store.GetGrant(context.Background(), "grant_v2")
	if err != nil {
		t.Fatalf("the v2 grant must remain readable: %v", err)
	}
	if grant.EffectCeiling != "" {
		t.Fatalf("pre-E3 grants read back UNLIMITED ceiling, got %q", grant.EffectCeiling)
	}
	if _, err := store.Get(context.Background(), "act_v1"); err != nil {
		t.Fatalf("the v1 action row still reads through v3: %v", err)
	}
}

// TestMigrationV3_crashMidMigrationNeverLeavesAZombie re-arms AS-8 for
// v3: the migration transaction aborts at its version bump AFTER the
// DDL ran — the file must remain a CLEAN v2 and the next boot completes.
func TestMigrationV3_crashMidMigrationNeverLeavesAZombie(t *testing.T) {
	t.Parallel()
	path := buildV2File(t)
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := db.Exec(
		`CREATE TRIGGER crash_mid_v3 BEFORE UPDATE ON action_schema
		 BEGIN SELECT RAISE(ABORT, 'crash mid v3'); END;`,
	); err != nil {
		t.Fatalf("install crash trigger: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("an aborted v3 migration must be boot-fatal")
	}
	if v := inspect(t, path, `SELECT version FROM action_schema`); v != 2 {
		t.Fatalf("aborted migration must leave version 2, got %d", v)
	}
	if n := inspect(t, path,
		`SELECT COUNT(*) FROM pragma_table_info('grants') WHERE name = 'effect_ceiling'`); n != 0 {
		t.Fatalf("ZOMBIE: the ceiling column survived the rollback (%d)", n)
	}
	db2, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw 2: %v", err)
	}
	if _, err := db2.Exec(`DROP TRIGGER crash_mid_v3`); err != nil {
		t.Fatalf("drop trigger: %v", err)
	}
	if err := db2.Close(); err != nil {
		t.Fatalf("close raw 2: %v", err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatalf("the next boot must complete: %v", err)
	}
	defer func() { _ = store.Close() }()
	if v, _ := store.SchemaVersion(context.Background()); v != schemaVersionCurrent {
		t.Fatalf("completed migration must land on the current version, got %v", v)
	}
}

func TestGrant_ceilingRoundTripsAndTheWallJudgesIt(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	if err := store.CreateIntent(ctx, draftIntent("int_c3")); err != nil {
		t.Fatalf("create intent: %v", err)
	}
	parent := rootGrant("grant_c3", "int_c3")
	parent.EffectCeiling = action.EffectWriteReversible
	if err := store.CreateGrant(ctx, parent); err != nil {
		t.Fatalf("create ceilinged grant: %v", err)
	}
	got, err := store.GetGrant(ctx, "grant_c3")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.EffectCeiling != action.EffectWriteReversible {
		t.Fatalf("the ceiling must round-trip, got %q", got.EffectCeiling)
	}
	if got.Digest() != parent.Digest() {
		t.Fatal("the ceilinged digest must survive the round-trip")
	}
	// The delegation frontier judges the tenth dimension: a child
	// reaching above the parent's ceiling never touches the disk.
	widening := childGrant("grant_c3w", parent)
	widening.EffectCeiling = action.EffectCritical
	err = store.DelegateGrant(ctx, widening)
	if !errors.Is(err, action.ErrAttenuationViolated) || !strings.Contains(err.Error(), "effect_ceiling") {
		t.Fatalf("the wall must name the tenth dimension, got %v", err)
	}
	if _, err := store.GetGrant(ctx, "grant_c3w"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("the widening child never touches the disk: %v", err)
	}
	// An attenuated ceiling passes and persists.
	child := childGrant("grant_c3c", parent)
	child.EffectCeiling = action.EffectReadExternal
	if err := store.DelegateGrant(ctx, child); err != nil {
		t.Fatalf("attenuated ceiling must pass: %v", err)
	}
	stored, err := store.GetGrant(ctx, "grant_c3c")
	if err != nil || stored.EffectCeiling != action.EffectReadExternal {
		t.Fatalf("child ceiling round-trip: %v %q", err, stored.EffectCeiling)
	}
}

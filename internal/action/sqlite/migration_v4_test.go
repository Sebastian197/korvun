// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Migration v3→v4 contract — Etapa 3, lote 4, pieza 1 (spec FR-POL-1):
// every gate decision persists the EXACT law that took it —
// policy_version and policy_digest as additive columns on
// action_decisions, on the anti-zombie runner with AS-8 re-armed against
// a HAND-BUILT v3 file. Pre-E3 decisions read back version 0 / empty
// digest (the honest "no pinned law" of their era); pinned decisions
// round-trip whole. Approved-red contract.

package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
)

// rawV3Delta is the v2→v3 DDL restated literally (oracle discipline).
const rawV3Delta = `
ALTER TABLE grants ADD COLUMN effect_ceiling TEXT NOT NULL DEFAULT '';
UPDATE action_schema SET version = 3;`

// buildV3File hand-builds a real v3 store file (v1 + v2 delta + v3
// delta), carrying one decision row from the pre-pin era.
func buildV3File(t *testing.T) string {
	t.Helper()
	path := buildV2File(t)
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(rawV3Delta); err != nil {
		t.Fatalf("build v3 delta: %v", err)
	}
	return path
}

func TestMigrationV4_freshFileLandsOnV4WithThePinColumns(t *testing.T) {
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
	if v != 4 {
		t.Fatalf("a fresh store lands on v4, got %d", v)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if n := inspect(t, path,
		`SELECT COUNT(*) FROM pragma_table_info('action_decisions')
		  WHERE name IN ('policy_version','policy_digest')`); n != 2 {
		t.Fatalf("the pin columns must exist, got %d", n)
	}
}

func TestMigrationV4_v3FileMigratesAndOldDecisionsReadUnpinned(t *testing.T) {
	t.Parallel()
	path := buildV3File(t)
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open over v3: %v", err)
	}
	defer func() { _ = store.Close() }()
	v, err := store.SchemaVersion(context.Background())
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if v != 4 {
		t.Fatalf("v3 file must migrate to 4, got %d", v)
	}
	rec, err := store.Get(context.Background(), "act_v1")
	if err != nil {
		t.Fatalf("the pre-pin decision must remain readable: %v", err)
	}
	if rec.Decision.PolicyVersion != 0 || rec.Decision.PolicyDigest != "" {
		t.Fatalf("pre-pin decisions read back honestly unpinned, got %+v", rec.Decision)
	}
}

// TestMigrationV4_crashMidMigrationNeverLeavesAZombie re-arms AS-8: the
// aborted v4 step leaves a CLEAN v3 and the next boot completes.
func TestMigrationV4_crashMidMigrationNeverLeavesAZombie(t *testing.T) {
	t.Parallel()
	path := buildV3File(t)
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := db.Exec(
		`CREATE TRIGGER crash_mid_v4 BEFORE UPDATE ON action_schema
		 BEGIN SELECT RAISE(ABORT, 'crash mid v4'); END;`,
	); err != nil {
		t.Fatalf("install crash trigger: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("an aborted v4 migration must be boot-fatal")
	}
	if v := inspect(t, path, `SELECT version FROM action_schema`); v != 3 {
		t.Fatalf("aborted migration must leave version 3, got %d", v)
	}
	if n := inspect(t, path,
		`SELECT COUNT(*) FROM pragma_table_info('action_decisions')
		  WHERE name IN ('policy_version','policy_digest')`); n != 0 {
		t.Fatalf("ZOMBIE: pin columns survived the rollback (%d)", n)
	}
	db2, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw 2: %v", err)
	}
	if _, err := db2.Exec(`DROP TRIGGER crash_mid_v4`); err != nil {
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
	if v, _ := store.SchemaVersion(context.Background()); v != 4 {
		t.Fatalf("completed migration must land on 4, got %v", v)
	}
}

func TestDecision_policyPinRoundTripsOnBothRecordPaths(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	pinned := Decision{Outcome: "allow", Rule: "granted",
		PolicyVersion: 1756500000000000001, PolicyDigest: "sha256:law1"}
	if err := store.RecordAttempt(ctx, testEnvelope("act_p1"), pinned, action.StateAuthorized); err != nil {
		t.Fatalf("record: %v", err)
	}
	rec, err := store.Get(ctx, "act_p1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rec.Decision.PolicyVersion != pinned.PolicyVersion || rec.Decision.PolicyDigest != "sha256:law1" {
		t.Fatalf("the pin must round-trip on RecordAttempt, got %+v", rec.Decision)
	}
	if err := store.RecordAttemptIdentified(ctx, testEnvelope("act_p2"), pinned,
		action.StateAuthorized, testIdentity()); err != nil {
		t.Fatalf("record identified: %v", err)
	}
	rec2, err := store.Get(ctx, "act_p2")
	if err != nil {
		t.Fatalf("get identified: %v", err)
	}
	if rec2.Decision.PolicyDigest != "sha256:law1" || rec2.Decision.PolicyVersion != pinned.PolicyVersion {
		t.Fatalf("the pin must round-trip on the identified path too, got %+v", rec2.Decision)
	}
}

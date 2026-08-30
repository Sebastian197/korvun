// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Signing-keys persistence contract — Etapa 4, lote 2, pieza 2 (spec
// FR-KEY-1/2): migration v4→v5 on the anti-zombie runner brings the
// signing_keys table — key_id, public key, created_at, retired_at.
// Retired keys are KEPT FOREVER (no delete path exists); at most ONE
// key is active; rotation retires the old and activates the new in ONE
// transaction. Batch decision declared: v5 = signing_keys (this batch),
// v6 = receipts (batch 3). Approved-red contract.

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// rawV4Delta is the v3→v4 DDL restated literally (oracle discipline).
const rawV4Delta = `
ALTER TABLE action_decisions ADD COLUMN policy_version INTEGER NOT NULL DEFAULT 0;
ALTER TABLE action_decisions ADD COLUMN policy_digest TEXT NOT NULL DEFAULT '';
UPDATE action_schema SET version = 4;`

// buildV4File hand-builds a real v4 store file.
func buildV4File(t *testing.T) string {
	t.Helper()
	path := buildV3File(t)
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(rawV4Delta); err != nil {
		t.Fatalf("build v4 delta: %v", err)
	}
	return path
}

func TestMigrationV5_freshFileLandsOnV5WithSigningKeys(t *testing.T) {
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
	if v != schemaVersionCurrent {
		t.Fatalf("a fresh store lands on the current version, got %d", v)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if n := inspect(t, path,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='signing_keys'`); n != 1 {
		t.Fatalf("the signing_keys table must exist, got %d", n)
	}
}

func TestMigrationV5_crashMidMigrationNeverLeavesAZombie(t *testing.T) {
	t.Parallel()
	path := buildV4File(t)
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := db.Exec(
		`CREATE TRIGGER crash_mid_v5 BEFORE UPDATE ON action_schema
		 BEGIN SELECT RAISE(ABORT, 'crash mid v5'); END;`,
	); err != nil {
		t.Fatalf("install crash trigger: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("an aborted v5 migration must be boot-fatal")
	}
	if v := inspect(t, path, `SELECT version FROM action_schema`); v != 4 {
		t.Fatalf("aborted migration must leave version 4, got %d", v)
	}
	if n := inspect(t, path,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='signing_keys'`); n != 0 {
		t.Fatalf("ZOMBIE: signing_keys survived the rollback (%d)", n)
	}
	db2, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw 2: %v", err)
	}
	if _, err := db2.Exec(`DROP TRIGGER crash_mid_v5`); err != nil {
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
		t.Fatalf("completed migration must land on current, got %v", v)
	}
}

var keysT0 = time.Date(2026, 8, 30, 15, 0, 0, 0, time.UTC)

func TestSigningKeys_firstKeyRoundTripsAndIsActive(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	// No key yet: ActiveSigningKey reports not-found.
	if _, err := store.ActiveSigningKey(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an empty keystore has no active key: %v", err)
	}
	if err := store.PutSigningKey(ctx, "ed25519:aaaa000011112222", "aabbcc", keysT0); err != nil {
		t.Fatalf("put: %v", err)
	}
	active, err := store.ActiveSigningKey(ctx)
	if err != nil {
		t.Fatalf("active: %v", err)
	}
	if active.KeyID != "ed25519:aaaa000011112222" || active.PublicKey != "aabbcc" {
		t.Fatalf("round-trip: %+v", active)
	}
	if !active.CreatedAt.Equal(keysT0) || !active.RetiredAt.IsZero() {
		t.Fatalf("lifecycle facts: %+v", active)
	}
	// At most ONE active: a second Put while one is active fails — the
	// only path forward is rotation.
	if err := store.PutSigningKey(ctx, "ed25519:bbbb000011112222", "ddeeff", keysT0); err == nil {
		t.Fatal("a second active key must be refused; rotate instead")
	}
}

func TestSigningKeys_rotationRetiresAndActivatesAtomically(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	if err := store.PutSigningKey(ctx, "ed25519:key000000000000a", "aa", keysT0); err != nil {
		t.Fatalf("put: %v", err)
	}
	rotatedAt := keysT0.Add(time.Hour)
	if err := store.RotateSigningKey(ctx, "ed25519:key000000000000b", "bb", rotatedAt); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	active, err := store.ActiveSigningKey(ctx)
	if err != nil {
		t.Fatalf("active: %v", err)
	}
	if active.KeyID != "ed25519:key000000000000b" {
		t.Fatalf("the new key must be active, got %q", active.KeyID)
	}
	// The retired key is KEPT with its validity window closed.
	old, err := store.GetSigningKey(ctx, "ed25519:key000000000000a")
	if err != nil {
		t.Fatalf("retired keys are kept forever: %v", err)
	}
	if !old.RetiredAt.Equal(rotatedAt) {
		t.Fatalf("the retirement instant closes the window: %+v", old)
	}
	keys, err := store.ListSigningKeys(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("both keys remain, got %d", len(keys))
	}
	// A second rotation keeps the whole history.
	if err := store.RotateSigningKey(ctx, "ed25519:key000000000000c", "cc", rotatedAt.Add(time.Hour)); err != nil {
		t.Fatalf("rotate 2: %v", err)
	}
	keys, _ = store.ListSigningKeys(ctx)
	if len(keys) != 3 {
		t.Fatalf("history never shrinks, got %d", len(keys))
	}
}

func TestSigningKeys_failClosedEdges(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	// Rotating with no active key fails (nothing to retire).
	if err := store.RotateSigningKey(ctx, "ed25519:keyx", "xx", keysT0); err == nil {
		t.Fatal("rotation without an active key must fail")
	}
	if _, err := store.GetSigningKey(ctx, "ed25519:ghost"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown key: %v", err)
	}
	// Closed store: loud failures on every surface.
	closed, _ := openTemp(t)
	_ = closed.Close()
	if err := closed.PutSigningKey(ctx, "k", "p", keysT0); err == nil {
		t.Fatal("closed store must fail loud on Put")
	}
	if _, err := closed.ListSigningKeys(ctx); err == nil {
		t.Fatal("closed store must fail loud on List")
	}
}

func TestSigningKeys_deepErrorBranches(t *testing.T) {
	t.Parallel()
	// Corrupt stored cells fail loud naming the cell.
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	if err := store.PutSigningKey(ctx, "ed25519:corrupt000000000", "aa", keysT0); err != nil {
		t.Fatalf("put: %v", err)
	}
	corruptCell(t, store, "signing_keys", "created_at", "key_id", "ed25519:corrupt000000000", "garbage")
	if _, err := store.GetSigningKey(ctx, "ed25519:corrupt000000000"); err == nil {
		t.Fatal("a corrupt created_at must fail loud")
	}
	if _, err := store.ListSigningKeys(ctx); err == nil {
		t.Fatal("the corrupt row must fail the listing too")
	}
	if _, err := store.ActiveSigningKey(ctx); err == nil {
		t.Fatal("the corrupt active row must fail loud")
	}
	// Closed store: the remaining surfaces.
	closed, _ := openTemp(t)
	_ = closed.Close()
	if err := closed.RotateSigningKey(ctx, "k", "p", keysT0); err == nil {
		t.Fatal("closed store must fail loud on Rotate")
	}
	if _, err := closed.ActiveSigningKey(ctx); err == nil {
		t.Fatal("closed store must fail loud on Active")
	}
	if _, err := closed.GetSigningKey(ctx, "k"); err == nil {
		t.Fatal("closed store must fail loud on Get")
	}
	// A blocked retire surfaces the error and changes nothing.
	blocked, _ := openTemp(t)
	defer func() { _ = blocked.Close() }()
	if err := blocked.PutSigningKey(ctx, "ed25519:block00000000000", "bb", keysT0); err != nil {
		t.Fatalf("put: %v", err)
	}
	if _, err := blocked.db.Exec(
		`CREATE TRIGGER block_key_update BEFORE UPDATE ON signing_keys
		 BEGIN SELECT RAISE(ABORT, 'blocked by test'); END;`,
	); err != nil {
		t.Fatalf("install blocker: %v", err)
	}
	if err := blocked.RotateSigningKey(ctx, "ed25519:new0000000000000", "cc", keysT0.Add(time.Hour)); err == nil {
		t.Fatal("a blocked rotation must surface the error")
	}
	active, err := blocked.ActiveSigningKey(ctx)
	if err != nil || active.KeyID != "ed25519:block00000000000" {
		t.Fatalf("a failed rotation must change NOTHING: %v %+v", err, active)
	}
}

func TestSigningKeys_moreBlockedWrites(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// Blocked INSERT: Put surfaces the error with nothing landed.
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	if _, err := store.db.Exec(
		`CREATE TRIGGER block_key_insert BEFORE INSERT ON signing_keys
		 BEGIN SELECT RAISE(ABORT, 'blocked by test'); END;`,
	); err != nil {
		t.Fatalf("install blocker: %v", err)
	}
	if err := store.PutSigningKey(ctx, "ed25519:blockedput000000", "aa", keysT0); err == nil {
		t.Fatal("a blocked insert must surface the error")
	}
	if _, err := store.db.Exec(`DROP TRIGGER block_key_insert`); err != nil {
		t.Fatalf("drop: %v", err)
	}
	// Rotation whose INSERT half fails: atomic — the retire rolls back.
	if err := store.PutSigningKey(ctx, "ed25519:rotatomic0000000", "bb", keysT0); err != nil {
		t.Fatalf("put: %v", err)
	}
	if _, err := store.db.Exec(
		`CREATE TRIGGER block_key_insert2 BEFORE INSERT ON signing_keys
		 BEGIN SELECT RAISE(ABORT, 'blocked by test'); END;`,
	); err != nil {
		t.Fatalf("install blocker 2: %v", err)
	}
	if err := store.RotateSigningKey(ctx, "ed25519:rotatomic0000001", "cc", keysT0.Add(time.Hour)); err == nil {
		t.Fatal("a rotation whose insert fails must surface the error")
	}
	if _, err := store.db.Exec(`DROP TRIGGER block_key_insert2`); err != nil {
		t.Fatalf("drop 2: %v", err)
	}
	active, err := store.ActiveSigningKey(ctx)
	if err != nil || active.KeyID != "ed25519:rotatomic0000000" {
		t.Fatalf("the failed rotation must leave the old key ACTIVE (atomicity): %v %+v", err, active)
	}
	// Corrupt retired_at fails the scan branch.
	corruptCell(t, store, "signing_keys", "retired_at", "key_id", "ed25519:rotatomic0000000", "garbage")
	if _, err := store.GetSigningKey(ctx, "ed25519:rotatomic0000000"); err == nil {
		t.Fatal("a corrupt retired_at must fail loud")
	}
}

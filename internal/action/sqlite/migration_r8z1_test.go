// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R8-Z1 (seventh Codex pass, P1 — revoking the retire adjudication):
// a migration NEVER destroys tombstones. v10→v11 now COPIES: the
// rows are read in Go, Approval.Digest() is computed from the scalar
// preimage (not computable in SQL — in Go it is), and every row lands
// in v11 — ZERO rows lost. Crash mid-copy lands cleanly back on v10
// with the evidence intact (the inherited AS-8 mold), and the
// auditor's reproduction — decided v10 evidence → migrate → verify
// RECONSTRUCTS — rides permanent. Reproduction-first contract.

package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// rawV9DeltaZ / rawV10DeltaZ restate the lift DDL literally (oracle
// discipline, the rawV6Delta mold).
const rawV9DeltaZ = `
CREATE TABLE approvals_v9 (
    approval_id           TEXT    NOT NULL PRIMARY KEY,
    schema_version        INTEGER NOT NULL,
    action_id             TEXT    NOT NULL UNIQUE REFERENCES actions(action_id) ON DELETE CASCADE,
    action_digest         TEXT    NOT NULL,
    preview_digest        TEXT    NOT NULL,
    canonical_preview     TEXT    NOT NULL,
    canonical_params      TEXT    NOT NULL,
    requested_from        TEXT    NOT NULL,
    reason                TEXT    NOT NULL,
    risk_summary          TEXT    NOT NULL,
    policy_version        INTEGER NOT NULL,
    policy_digest         TEXT    NOT NULL,
    requested_at          TEXT    NOT NULL,
    expires_at            TEXT,
    status                TEXT    NOT NULL,
    decision_principal_id TEXT    NOT NULL DEFAULT '',
    decision              TEXT    NOT NULL DEFAULT '',
    decision_at           TEXT,
    comment               TEXT    NOT NULL DEFAULT '',
    decision_receipt_id   TEXT    NOT NULL DEFAULT ''
) WITHOUT ROWID;
DROP TABLE approvals;
ALTER TABLE approvals_v9 RENAME TO approvals;
CREATE INDEX approvals_by_status ON approvals(status);
UPDATE action_schema SET version = 9;`

const rawV10DeltaZ = `
CREATE TABLE approval_tombstones (
    action_id             TEXT    NOT NULL PRIMARY KEY,
    approval_id           TEXT    NOT NULL,
    action_digest         TEXT    NOT NULL,
    preview_digest        TEXT    NOT NULL,
    policy_version        INTEGER NOT NULL,
    policy_digest         TEXT    NOT NULL,
    decision_principal_id TEXT    NOT NULL,
    decision              TEXT    NOT NULL,
    decision_at           TEXT
) WITHOUT ROWID;
UPDATE action_schema SET version = 10;`

// buildV10File hand-builds a real v10 store with one DECIDED tombstone.
func buildV10File(t *testing.T) (string, action.Approval) {
	t.Helper()
	path := buildV8File(t)
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(rawV9DeltaZ + rawV10DeltaZ); err != nil {
		t.Fatalf("build v9+v10 deltas: %v", err)
	}
	seed := action.Approval{
		ApprovalID: "apr_v10seed0000000000000000000000001", ActionID: "act_v10seed",
		ActionDigest: "sha256:aaaa", PreviewDigest: "sha256:pppp",
		PolicyVersion: 3, PolicyDigest: "sha256:llll",
		DecisionPrincipalID: "principal_operator", Decision: "rejected",
		DecisionAt: time.Date(2026, 9, 3, 1, 2, 3, 0, time.UTC),
	}
	if _, err := db.Exec(`INSERT INTO approval_tombstones
	    (action_id, approval_id, action_digest, preview_digest, policy_version,
	     policy_digest, decision_principal_id, decision, decision_at)
	 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		seed.ActionID, seed.ApprovalID, seed.ActionDigest, seed.PreviewDigest,
		seed.PolicyVersion, seed.PolicyDigest, seed.DecisionPrincipalID,
		seed.Decision, seed.DecisionAt.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed v10 tombstone: %v", err)
	}
	return path, seed
}

func TestMigrationV11_copiesEveryTombstoneComputingTheDigest(t *testing.T) {
	t.Parallel()
	path, seed := buildV10File(t)
	store, err := Open(path)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	// ZERO rows lost: the seeded evidence reconstructs BY ITS DIGEST.
	want := seed.Digest()
	tomb, err := store.ApprovalTombstoneByDigest(ctx, want)
	if err != nil {
		t.Fatalf("AUDIT R8-Z1: the migration must COPY, never destroy: %v", err)
	}
	if tomb.Digest() != want || tomb.ActionID != "act_v10seed" || tomb.Decision != "rejected" {
		t.Fatalf("the copied preimage must re-derive and preserve the story: %+v", tomb)
	}
}

func TestMigrationV11_crashMidCopyLandsBackOnV10Intact(t *testing.T) {
	t.Parallel()
	path, _ := buildV10File(t)
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := db.Exec(
		`CREATE TRIGGER crash_mid_v11 BEFORE UPDATE ON action_schema
		 BEGIN SELECT RAISE(ABORT, 'crash mid v11'); END;`); err != nil {
		t.Fatalf("install crash trigger: %v", err)
	}
	_ = db.Close()
	if _, err := Open(path); err == nil {
		t.Fatal("an aborted v11 migration must be boot-fatal")
	}
	if v := inspect(t, path, `SELECT version FROM action_schema`); v != 10 {
		t.Fatalf("clean rollback to v10, got %d", v)
	}
	if n := inspect(t, path, `SELECT COUNT(*) FROM approval_tombstones`); n != 1 {
		t.Fatalf("the v10 evidence must be INTACT after the rollback: %d", n)
	}
	db2, _ := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	_, _ = db2.Exec(`DROP TRIGGER crash_mid_v11`)
	_ = db2.Close()
	recovered, err := Open(path)
	if err != nil {
		t.Fatalf("the retry completes: %v", err)
	}
	defer func() { _ = recovered.Close() }()
	if n := inspect(t, path, `SELECT COUNT(*) FROM approval_tombstones`); n != 1 {
		t.Fatalf("convergence: the copy lands on retry: %d", n)
	}
}

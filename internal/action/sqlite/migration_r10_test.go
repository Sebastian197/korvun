// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R10-V1 (ninth Codex pass, P1): '' is EMPTY EVIDENCE, not absence.
// A v10 decision_at holding empty bytes is a present-but-empty
// evidence column and fails the migration CLOSED naming row and
// field — only NULL (the writer declared absence) migrates. The
// auditor's reproduction verbatim: valid row → UPDATE decision_at=''
// → open → named boot-fatal.
// R10-V3 (P2): the mid-copy crash mold carries its PROBE — the test
// drives the transaction by hand and observes, before the rollback,
// that the first row already landed in v11 when the second failed
// (a validate-all-then-insert mutation turns this mold red), and the
// copy's read order is contractual (ORDER BY), not a B-tree accident.
// Reproduction-first contract.

package sqlite

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// R10-V1 — the auditor's reproduction, verbatim steps.
func TestMigrationV11_emptyDecisionAtIsEmptyEvidenceNotAbsence(t *testing.T) {
	t.Parallel()
	path, _ := buildV10File(t) // one VALID decided row
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := db.Exec(`UPDATE approval_tombstones SET decision_at = ''`); err != nil {
		t.Fatalf("auditor's UPDATE: %v", err)
	}
	_ = db.Close()
	_, err = Open(path)
	if err == nil {
		t.Fatal("AUDIT R10-V1: empty bytes are empty EVIDENCE — the migration must fail closed, '' is not NULL")
	}
	if !strings.Contains(err.Error(), "apr_v10seed0000000000000000000000001") ||
		!strings.Contains(err.Error(), "decision_at") {
		t.Fatalf("the failure must name row and field: %v", err)
	}
	if v := inspect(t, path, `SELECT version FROM action_schema`); v != 10 {
		t.Fatalf("clean rollback to v10, got %d", v)
	}
}

// R10-V3 — the probe: drive the migration transaction by hand and
// observe the first INSERT inside v11 BEFORE the rollback.
func TestMigrationV11_probeShowsFirstInsertLandedBeforeMidCopyFailure(t *testing.T) {
	t.Parallel()
	path, _ := buildV10File(t) // good row: act_v10seed
	// The bad row sorts AFTER the good one under the copy's ORDER BY.
	seedV10RawRow(t, path, [9]any{"act_zz_bad", "apr_zz_bad000000000000000000001",
		"sha256:wwww", "sha256:xxxx", 1, "sha256:yyyy", "principal_operator",
		"rejected", "not-a-date"})
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = db.Close() }()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(migrations[10]); err != nil {
		t.Fatalf("v11 DDL: %v", err)
	}
	copyErr := copyTombstonesV10toV11(tx)
	if copyErr == nil || !strings.Contains(copyErr.Error(), "decision_at") {
		t.Fatalf("the bad second row must abort the copy by name: %v", copyErr)
	}
	// THE PROBE (pre-rollback, same tx): the good row is ALREADY in
	// v11 — the failure struck mid-copy, not before the first insert.
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM approval_tombstones_v11`).Scan(&n); err != nil {
		t.Fatalf("probe: %v", err)
	}
	if n != 1 {
		t.Fatalf("AUDIT R10-V3: the first row must land BEFORE the mid-copy failure (validate-all-then-insert would leave 0): %d", n)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if v := inspect(t, path, `SELECT version FROM action_schema`); v != 10 {
		t.Fatalf("v10 stands after rollback: %d", v)
	}
	if n := inspect(t, path, `SELECT COUNT(*) FROM approval_tombstones`); n != 2 {
		t.Fatalf("both v10 rows intact: %d", n)
	}
}

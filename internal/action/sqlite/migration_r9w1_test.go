// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R9-W1 (eighth Codex pass, P1): corruption NEVER masquerades as
// absence. The v10→v11 copy validates every row BEFORE migrating it —
// an unreadable field fails the whole migration CLOSED, naming row
// and field (boot-fatal: corrupt evidence demands human adjudication,
// never a silent zero-time normalization). NULL decision_at is honest
// absence and migrates — the Y3 line (corrupt ≠ absent), now in the
// migration too. R9-W4: the mid-copy crash is REAL — the bad row
// fails AFTER a good row already landed in v11, the transaction rolls
// back, v10 keeps every byte, and the retry after repair converges.
// Reproduction-first contract.

package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// seedV10RawRow plants one raw row directly into a v10
// approval_tombstones table — the auditor's hand, not the product's.
func seedV10RawRow(t *testing.T, path string, cols [9]any) {
	t.Helper()
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`INSERT INTO approval_tombstones
	    (action_id, approval_id, action_digest, preview_digest, policy_version,
	     policy_digest, decision_principal_id, decision, decision_at)
	 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cols[0], cols[1], cols[2], cols[3], cols[4], cols[5], cols[6], cols[7], cols[8]); err != nil {
		t.Fatalf("seed raw v10 row: %v", err)
	}
}

// v10RowBytes reads the raw stored bytes of every v10 tombstone row,
// ordered by primary key — the byte-for-byte intactness oracle.
func v10RowBytes(t *testing.T, path string) []string {
	t.Helper()
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = db.Close() }()
	rows, err := db.Query(`SELECT action_id || '|' || approval_id || '|' || action_digest
	    || '|' || preview_digest || '|' || policy_version || '|' || policy_digest
	    || '|' || decision_principal_id || '|' || decision || '|' || COALESCE(decision_at, '<NULL>')
	  FROM approval_tombstones ORDER BY action_id`)
	if err != nil {
		t.Fatalf("read raw rows: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var all []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("scan raw row: %v", err)
		}
		all = append(all, s)
	}
	return all
}

// R1 + R10: an unreadable decision_at fails the migration CLOSED,
// naming row and field — and because the bad row sorts AFTER the good
// seed, the crash lands mid-copy (a good row already inside v11 when
// the validation aborts). v10 keeps every byte of BOTH rows.
func TestMigrationV11_garbageDateFailsClosedNamingRowAndField(t *testing.T) {
	t.Parallel()
	path, _ := buildV10File(t)
	seedV10RawRow(t, path, [9]any{"act_zz_weird", "apr_zz_weird00000000000000000001",
		"sha256:wwww", "sha256:xxxx", 0, "sha256:yyyy", "principal_operator",
		"rejected", "not-a-date"})
	before := v10RowBytes(t, path)
	_, err := Open(path)
	if err == nil {
		t.Fatal("AUDIT R9-W1: garbage bytes must fail the migration closed, never normalize to zero time")
	}
	if !strings.Contains(err.Error(), "apr_zz_weird00000000000000000001") ||
		!strings.Contains(err.Error(), "decision_at") {
		t.Fatalf("the failure must name row and field for human adjudication: %v", err)
	}
	if v := inspect(t, path, `SELECT version FROM action_schema`); v != 10 {
		t.Fatalf("clean rollback to v10, got %d", v)
	}
	after := v10RowBytes(t, path)
	if len(after) != 2 || len(before) != 2 || after[0] != before[0] || after[1] != before[1] {
		t.Fatalf("v10 must keep every byte of BOTH rows:\nbefore=%v\nafter=%v", before, after)
	}
}

// R2: an empty evidence field is unreadable evidence — named fail.
func TestMigrationV11_emptyEvidenceFieldFailsClosedNamed(t *testing.T) {
	t.Parallel()
	path, _ := buildV10File(t)
	seedV10RawRow(t, path, [9]any{"act_zz_empty", "apr_zz_empty00000000000000000001",
		"sha256:wwww", "sha256:xxxx", 1, "sha256:yyyy", "",
		"rejected", "2026-09-03T01:02:03Z"})
	_, err := Open(path)
	if err == nil {
		t.Fatal("AUDIT R9-W1: an empty decision_principal_id must fail the migration closed")
	}
	if !strings.Contains(err.Error(), "apr_zz_empty00000000000000000001") ||
		!strings.Contains(err.Error(), "decision_principal_id") {
		t.Fatalf("the failure must name row and field: %v", err)
	}
	if v := inspect(t, path, `SELECT version FROM action_schema`); v != 10 {
		t.Fatalf("clean rollback to v10, got %d", v)
	}
}

// R3: NULL decision_at is honest ABSENCE, not corruption — it
// migrates, and its zero-time digest reconstructs (corrupt ≠ absent).
func TestMigrationV11_nullDecisionAtIsHonestAbsenceAndMigrates(t *testing.T) {
	t.Parallel()
	path, _ := buildV10File(t)
	seedV10RawRow(t, path, [9]any{"act_zz_null", "apr_zz_null000000000000000000001",
		"sha256:wwww", "sha256:xxxx", 1, "sha256:yyyy", "principal_operator",
		"rejected", nil})
	store, err := Open(path)
	if err != nil {
		t.Fatalf("absence is not corruption — the migration must pass: %v", err)
	}
	defer func() { _ = store.Close() }()
	tomb, err := store.ApprovalTombstone(context.Background(), "act_zz_null")
	if err != nil {
		t.Fatalf("the absent-date tombstone must land in v11: %v", err)
	}
	if !tomb.DecisionAt.IsZero() || tomb.Decision != "rejected" {
		t.Fatalf("zero time IS the honest encoding of no date: %+v", tomb)
	}
}

// R4 + R10 convergence: repair the garbage by hand (the human
// adjudication the fail-closed demands) and the retry converges.
func TestMigrationV11_repairThenRetryConverges(t *testing.T) {
	t.Parallel()
	path, seed := buildV10File(t)
	seedV10RawRow(t, path, [9]any{"act_zz_weird", "apr_zz_weird00000000000000000001",
		"sha256:wwww", "sha256:xxxx", 0, "sha256:yyyy", "principal_operator",
		"rejected", "not-a-date"})
	if _, err := Open(path); err == nil {
		t.Fatal("precondition: the garbage row must fail the migration first")
	}
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := db.Exec(`UPDATE approval_tombstones
	    SET decision_at = '2026-09-03T04:05:06Z' WHERE action_id = 'act_zz_weird'`); err != nil {
		t.Fatalf("repair: %v", err)
	}
	_ = db.Close()
	store, err := Open(path)
	if err != nil {
		t.Fatalf("the retry after repair must converge: %v", err)
	}
	defer func() { _ = store.Close() }()
	if n := inspect(t, path, `SELECT COUNT(*) FROM approval_tombstones`); n != 2 {
		t.Fatalf("both rows land on retry: %d", n)
	}
	if _, err := store.ApprovalTombstoneByDigest(context.Background(), seed.Digest()); err != nil {
		t.Fatalf("the good seed reconstructs by its digest after convergence: %v", err)
	}
}

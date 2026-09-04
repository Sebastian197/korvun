// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R11 (the trimmed round after three paper vetoes): v12 is a
// RE-VALIDATION migration — the legacy-v11 window (rows that entered
// before the R9/R10 walls) is re-judged by the ONE typed contract,
// every row before the bump, in one transaction, with ZERO writes to
// the evidence (proven by abort triggers armed during the migration,
// plus post-migration identity of DDL, indexes, content, types and
// NULLs). The migration validates and bumps, or fails closed naming
// row and field. It never normalizes. Its guarantee is
// per-snapshot: it established the invariant in ITS snapshot; any
// later audit must re-establish it in its own.
// Reproduction-first contract.

package sqlite

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// buildV11LegacyFile makes a healthy CURRENT store, then forces the
// version back to 11 — the legacy window whose rows v12 re-judges.
// Raw rows are the auditor's hand: they entered before the walls.
func buildV11LegacyFile(t *testing.T, rawRows ...[10]any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "korvun.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("open fresh: %v", err)
	}
	_ = store.Close()
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`UPDATE action_schema SET version = 11`); err != nil {
		t.Fatalf("force v11: %v", err)
	}
	for i, r := range rawRows {
		if _, err := db.Exec(`INSERT INTO approval_tombstones
		    (approval_id, approval_digest, action_id, action_digest, preview_digest,
		     policy_version, policy_digest, decision_principal_id, decision, decision_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			r[0], r[1], r[2], r[3], r[4], r[5], r[6], r[7], r[8], r[9]); err != nil {
			t.Fatalf("seed legacy row %d: %v", i, err)
		}
	}
	return path
}

// legacyGoodRow returns a row that SATISFIES the contract (digest
// re-derived in Go from the exact preimage).
func legacyGoodRow(aprID, actID string) [10]any {
	a := actionpkgApproval(aprID, actID)
	return [10]any{a.ApprovalID, a.Digest(), a.ActionID, a.ActionDigest, a.PreviewDigest,
		a.PolicyVersion, a.PolicyDigest, a.DecisionPrincipalID, a.Decision, nil}
}

// R1 — zero writes, proven by abort triggers + post identity.
func TestMigrationV12_revalidatesWithZeroWritesUnderAbortTriggers(t *testing.T) {
	t.Parallel()
	path := buildV11LegacyFile(t, legacyGoodRow("apr_r11_ok00000000000000000000001", "act_r11_ok"))
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	before := tombstoneStoreShape(t, db)
	for _, op := range []string{"INSERT", "UPDATE", "DELETE"} {
		if _, err := db.Exec(fmt.Sprintf(
			`CREATE TRIGGER no_%s_during_v12 BEFORE %s ON approval_tombstones
			 BEGIN SELECT RAISE(ABORT, 'v12 must not write'); END;`, op, op)); err != nil {
			t.Fatalf("arm %s trigger: %v", op, err)
		}
	}
	_ = db.Close()
	store, err := Open(path)
	if err != nil {
		t.Fatalf("AUDIT R11-R1: v12 must complete WITH the abort triggers armed (zero writes): %v", err)
	}
	_ = store.Close()
	if v := inspect(t, path, `SELECT version FROM action_schema`); v != 12 {
		t.Fatalf("the bump lands: %d", v)
	}
	db2, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("reopen raw: %v", err)
	}
	defer func() { _ = db2.Close() }()
	var triggers int
	if err := db2.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='trigger'
	    AND name LIKE 'no_%_during_v12'`).Scan(&triggers); err != nil || triggers != 3 {
		t.Fatalf("the three triggers must SURVIVE (a table swap would drop them): %v %d", err, triggers)
	}
	after := tombstoneStoreShape(t, db2)
	if before != after {
		t.Fatalf("post identity (DDL, indexes, content, types, NULLs) must hold:\nbefore=%s\nafter=%s", before, after)
	}
}

// tombstoneStoreShape captures table SQL, index SQL, and the logical
// content with types and NULLs — the R1 identity oracle.
func tombstoneStoreShape(t *testing.T, db *sql.DB) string {
	t.Helper()
	var shape strings.Builder
	rows, err := db.Query(`SELECT type, name, COALESCE(sql,'') FROM sqlite_master
	    WHERE tbl_name='approval_tombstones' AND type IN ('table','index') ORDER BY type, name`)
	if err != nil {
		t.Fatalf("shape master: %v", err)
	}
	for rows.Next() {
		var typ, name, ddl string
		if err := rows.Scan(&typ, &name, &ddl); err != nil {
			t.Fatalf("shape scan: %v", err)
		}
		shape.WriteString(typ + "|" + name + "|" + ddl + "\n")
	}
	_ = rows.Close()
	content, err := db.Query(`SELECT approval_id, approval_digest, action_id, action_digest,
	    preview_digest, policy_version, policy_digest, decision_principal_id, decision,
	    COALESCE(decision_at, '<NULL>'), typeof(decision_at)
	  FROM approval_tombstones ORDER BY approval_id`)
	if err != nil {
		t.Fatalf("shape content: %v", err)
	}
	defer func() { _ = content.Close() }()
	for content.Next() {
		var c [11]string
		if err := content.Scan(&c[0], &c[1], &c[2], &c[3], &c[4], &c[5], &c[6], &c[7], &c[8], &c[9], &c[10]); err != nil {
			t.Fatalf("shape row: %v", err)
		}
		shape.WriteString(strings.Join(c[:], "|") + "\n")
	}
	return shape.String()
}

// R2 — the operational contract, field by field: the seven mandatory
// columns empty, an unreadable non-empty date, and good-then-bad.
func TestMigrationV12_contractFailsClosedNamingRowAndField(t *testing.T) {
	t.Parallel()
	fields := []struct {
		name string
		idx  int
	}{
		{"approval_id", 0}, {"action_id", 2}, {"action_digest", 3},
		{"preview_digest", 4}, {"policy_digest", 6},
		{"decision_principal_id", 7}, {"decision", 8},
	}
	for _, f := range fields {
		f := f
		t.Run("empty_"+f.name, func(t *testing.T) {
			t.Parallel()
			row := legacyGoodRow("apr_r11_f"+f.name+"00000000000001", "act_r11_"+f.name)
			row[f.idx] = ""
			path := buildV11LegacyFile(t, row)
			_, err := Open(path)
			if err == nil || !strings.Contains(err.Error(), f.name) {
				t.Fatalf("AUDIT R11-R2: empty %s must fail v12 naming the field: %v", f.name, err)
			}
			if v := inspect(t, path, `SELECT version FROM action_schema`); v != 11 {
				t.Fatalf("v11 stands: %d", v)
			}
		})
	}
	t.Run("unreadable_date", func(t *testing.T) {
		t.Parallel()
		row := legacyGoodRow("apr_r11_baddate000000000000000001", "act_r11_baddate")
		row[9] = "not-a-date"
		path := buildV11LegacyFile(t, row)
		_, err := Open(path)
		if err == nil || !strings.Contains(err.Error(), "decision_at") ||
			!strings.Contains(err.Error(), "apr_r11_baddate000000000000000001") {
			t.Fatalf("unreadable date fails v12 naming row and field: %v", err)
		}
	})
	t.Run("good_then_bad_rolls_back_whole", func(t *testing.T) {
		t.Parallel()
		bad := legacyGoodRow("apr_r11_zzbad0000000000000000001", "act_r11_zzbad")
		bad[9] = ""
		path := buildV11LegacyFile(t,
			legacyGoodRow("apr_r11_aaok00000000000000000001", "act_r11_aaok"), bad)
		_, err := Open(path)
		if err == nil || !strings.Contains(err.Error(), "apr_r11_zzbad0000000000000000001") {
			t.Fatalf("the bad row fails the WHOLE migration by name: %v", err)
		}
		if v := inspect(t, path, `SELECT version FROM action_schema`); v != 11 {
			t.Fatalf("v11 stands whole: %d", v)
		}
		if n := inspect(t, path, `SELECT COUNT(*) FROM approval_tombstones`); n != 2 {
			t.Fatalf("both rows intact: %d", n)
		}
	})
}

// R3 — a stored digest that does not re-derive fails v12 closed.
func TestMigrationV12_mutatedStoredDigestFailsClosed(t *testing.T) {
	t.Parallel()
	row := legacyGoodRow("apr_r11_mut000000000000000000001", "act_r11_mut")
	row[1] = "sha256:mutado"
	path := buildV11LegacyFile(t, row)
	_, err := Open(path)
	if err == nil || !strings.Contains(err.Error(), "approval_digest") ||
		!strings.Contains(err.Error(), "apr_r11_mut000000000000000000001") {
		t.Fatalf("AUDIT R11-R3: a lying stored digest must fail v12 naming row and field: %v", err)
	}
	if v := inspect(t, path, `SELECT version FROM action_schema`); v != 11 {
		t.Fatalf("v11 stands: %d", v)
	}
}

// Regla 1 verbatim — the auditor's reproduction: decision_at=” in an
// ALREADY-migrated v11 ends in the v12 boot-fatal.
func TestMigrationV12_emptyDecisionAtInLegacyV11IsBootFatal(t *testing.T) {
	t.Parallel()
	row := legacyGoodRow("apr_r11_empty0000000000000000001", "act_r11_empty")
	row[9] = ""
	path := buildV11LegacyFile(t, row)
	_, err := Open(path)
	if err == nil || !strings.Contains(err.Error(), "decision_at") ||
		!strings.Contains(err.Error(), "apr_r11_empty0000000000000000001") {
		t.Fatalf("REGLA 1: decision_at='' in legacy v11 must be the v12 boot-fatal: %v", err)
	}
}

// R5 — the reader propagates: an unreadable stored date read back is
// tombstone CORRUPTION, never a silent zero time.
func TestTombstoneReader_propagatesUnreadableDateAsCorrupt(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	a := actionpkgApproval("apr_r11_read0000000000000000001", "act_r11_read")
	if _, err := store.db.Exec(`INSERT INTO approval_tombstones
	    (approval_id, approval_digest, action_id, action_digest, preview_digest,
	     policy_version, policy_digest, decision_principal_id, decision, decision_at)
	 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'not-a-date')`,
		a.ApprovalID, a.Digest(), a.ActionID, a.ActionDigest, a.PreviewDigest,
		a.PolicyVersion, a.PolicyDigest, a.DecisionPrincipalID, a.Decision); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, _, err := store.ApprovalTombstone(t.Context(), "act_r11_read")
	if err == nil || !strings.Contains(err.Error(), "tombstone_corrupt") {
		t.Fatalf("AUDIT R11-R5: the reader must propagate unreadable bytes as corruption, never swallow to zero time: %v", err)
	}
}

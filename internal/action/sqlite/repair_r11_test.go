// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R11-R4: the manual expert procedure is EXECUTABLE, end to end,
// against a broken fixture — stop+lock, consistent backup, inspect,
// quarantine preserving bytes/types/NULLs, adjudicate with the EXACT
// independent preimage, verify after. The boot error points at the
// procedure. This mold executes every step programmatically; the
// sqlite3-shell forms are verified per-platform in the gate.

package sqlite

import (
	"database/sql"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestManualRepairProcedure_isExecutableEndToEnd(t *testing.T) {
	t.Parallel()
	// The broken fixture: a legacy row whose date bytes are garbage.
	// The test KNOWS the exact original preimage — the "independent
	// evidence" the adjudication step demands.
	good := legacyGoodRow("apr_r11_repair000000000000000001", "act_r11_repair")
	exactDate := "2026-09-01T10:20:30Z"
	broken := good
	broken[9] = "corrupted-bytes"
	path := buildV11LegacyFile(t, broken)

	// Step 0 — the boot fails closed AND points at the procedure.
	_, err := Open(path)
	if err == nil || !strings.Contains(err.Error(), "docs/operations/tombstone-manual-repair.md") {
		t.Fatalf("the boot error must point the operator at the procedure: %v", err)
	}

	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Step 2 — the REAL documented commands (R12-X5/A9: the procedure
	// is proven with the operator's actual tools, or skipped BY NAME;
	// VACUUM INTO died here — it was not what the document orders).
	sqlite3bin, lookErr := exec.LookPath("sqlite3")
	if lookErr != nil {
		t.Skip("SKIP NAMED (R12-A9): no sqlite3 binary on this runner — the documented commands cannot be exercised; the in-driver convergence coverage remains in the rest of the suite")
	}
	backupPath := filepath.Join(t.TempDir(), "pre-repair.db")
	if out, err := exec.Command(sqlite3bin, path, ".backup "+backupPath).CombinedOutput(); err != nil { //nolint:gosec // G204: the DOCUMENTED operator command; inputs are LookPath + TempDir paths
		t.Fatalf("documented .backup command: %v %s", err, out)
	}
	// The documented bind-parameter inspection and the dump quarantine,
	// with the operator's real shell.
	if out, err := exec.Command(sqlite3bin, path, //nolint:gosec // G204: the DOCUMENTED operator command
		".param set @apr 'apr_r11_repair000000000000000001'",
		"SELECT decision FROM approval_tombstones WHERE approval_id = @apr;").CombinedOutput(); err != nil ||
		!strings.Contains(string(out), "rejected") {
		t.Fatalf("documented .param bind inspection: %v %q", err, out)
	}
	if out, err := exec.Command(sqlite3bin, path, ".dump approval_tombstones").CombinedOutput(); err != nil || //nolint:gosec // G204: the DOCUMENTED operator command
		!strings.Contains(string(out), "INSERT INTO approval_tombstones") {
		t.Fatalf("documented .dump quarantine: %v %q", err, out)
	}
	// R12-P3-3: the documented hex recipe, exercised for real — the
	// corrupted bytes come back as their exact hex. Its red lives in
	// mutation m-hex: seed different bytes and the expected hex stops
	// matching — the assert bites on content, not on command success.
	// (This whole procedure test's skip path is probed by mutation
	// m-skip: an emptied PATH must produce the NAMED skip above.)
	if out, err := exec.Command(sqlite3bin, path, //nolint:gosec // G204: the DOCUMENTED operator command
		"SELECT hex(decision_at) FROM approval_tombstones WHERE approval_id = 'apr_r11_repair000000000000000001';").CombinedOutput(); err != nil ||
		!strings.Contains(string(out), "636F727275707465642D6279746573") { // hex("corrupted-bytes")
		t.Fatalf("documented hex recipe: %v %q", err, out)
	}
	if n := inspectAt(t, backupPath, `SELECT COUNT(*) FROM approval_tombstones`); n != 1 {
		t.Fatalf("the backup holds the evidence: %d", n)
	}

	// Step 3+4 — inspect with a BIND parameter (never interpolation)
	// and quarantine preserving bytes, types and NULLs.
	var qRow [11]string
	if err := db.QueryRow(`SELECT approval_id, approval_digest, action_id, action_digest,
	        preview_digest, policy_version, policy_digest, decision_principal_id, decision,
	        COALESCE(decision_at,'<NULL>'), typeof(decision_at)
	   FROM approval_tombstones WHERE approval_id = ?`, good[0]).
		Scan(&qRow[0], &qRow[1], &qRow[2], &qRow[3], &qRow[4], &qRow[5], &qRow[6],
			&qRow[7], &qRow[8], &qRow[9], &qRow[10]); err != nil {
		t.Fatalf("inspect by bind param: %v", err)
	}
	if qRow[9] != "corrupted-bytes" || qRow[10] != "text" {
		t.Fatalf("the quarantine record preserves bytes and types: %v", qRow)
	}

	// Step 5 — adjudicate: the correction uses the EXACT independent
	// preimage (never "fix until the digest matches").
	if _, err := db.Exec(`UPDATE approval_tombstones SET decision_at = ? WHERE approval_id = ?`,
		exactDate, good[0]); err != nil {
		t.Fatalf("adjudicated correction: %v", err)
	}
	// The stored digest was computed over the zero-time preimage (the
	// legacy row never had a readable date), so restoring a REAL date
	// makes the digest incoherent — the contract catches exactly that:
	if _, err := Open(path); err == nil || !strings.Contains(err.Error(), "approval_digest") {
		t.Fatalf("a correction that breaks preimage coherence is caught by name: %v", err)
	}
	// The TRUE adjudication for this row: the independent evidence
	// says the date was never recorded — absence is restored as NULL.
	if _, err := db.Exec(`UPDATE approval_tombstones SET decision_at = NULL WHERE approval_id = ?`,
		good[0]); err != nil {
		t.Fatalf("restore honest absence: %v", err)
	}

	// Step 6 — verify after: the boot converges and the evidence
	// reconstructs by its digest.
	store, err := Open(path)
	if err != nil {
		t.Fatalf("AUDIT R11-R4: after the honest repair the boot must converge: %v", err)
	}
	defer func() { _ = store.Close() }()
	a := actionpkgApproval(good[0].(string), good[2].(string))
	if _, _, err := store.ApprovalTombstoneByDigest(t.Context(), a.Digest()); err != nil {
		t.Fatalf("the repaired evidence reconstructs by its digest: %v", err)
	}
}

// inspectAt runs a scalar query against an arbitrary db file.
func inspectAt(t *testing.T, path, query string) int {
	t.Helper()
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRow(query).Scan(&n); err != nil {
		t.Fatalf("inspect %s: %v", path, err)
	}
	return n
}

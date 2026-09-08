// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R15, the JOIN fail-open (director's spec, 2026-09-08). `Store.Get` is
// a JOIN of `actions` with `action_decisions`, and the verifier read its
// single ErrNotFound as proof that retention had pruned the action row.
// It is not: retention's cascade removes BOTH rows, so either one
// surviving alone is evidence no cascade can produce — and reading it as
// an absence made the ladder print a prune that never happened, skip the
// custody comparisons, AND swallow the `approval_mismatch` arm that
// exists to catch exactly that sabotage. Two child-row DELETEs, no
// foreign-key override, turned exit 1 into exit 0.
//
// Three outcomes, one mold each, asserting the NAME and the TEXT — the
// text half closes the hole this train found in itself: the two notes'
// wording was changed and the whole suite stayed green, because nothing
// watched it.
//
// Evidence level: in-process CLI suite over a real SQLite file, the
// deletions applied through a SEPARATE raw connection.

package cli

import (
	"database/sql"
	"strings"
	"testing"
)

// rawStore opens the store file the way the store itself does — foreign
// keys ON. A fixture that leaves them off produces orphan rows the
// production path cannot create, which is how the fail-open hid.
func rawStore(t *testing.T, dbPath string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) int64 {
	t.Helper()
	res, err := db.Exec(query, args...)
	if err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
	n, _ := res.RowsAffected()
	return n
}

func countWhere(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}

// Outcome (a): the cascade's own shape — both rows gone together. The
// honest note, exit 0, and no failure of any kind.
func TestReceiptVerify_joinA_bothRowsGoneIsTheHonestNote(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, receiptID, actionID := operatorReceipt(t)
	db := rawStore(t, dbPath)
	mustExec(t, db, `DELETE FROM actions WHERE action_id = ?`, actionID)
	if n := countWhere(t, db, `SELECT COUNT(*) FROM action_decisions WHERE action_id = ?`, actionID); n != 0 {
		t.Fatalf("the premise is the CASCADE: %d decision row(s) survived", n)
	}

	code, stdout, stderr := runIntentCLI(t, "receipt", "verify", "--config", cfgPath, receiptID)
	out := stdout + stderr
	t.Logf("verdict with both rows gone: exit %d %q", code, out)
	if code != 0 {
		t.Fatalf("AUDIT R15-JOIN(a): the cascade's own shape is not a failure: %d %q", code, out)
	}
	if !strings.Contains(out, "action_row_absent: the action row for "+actionID+" and its decision row are both gone, which is the shape retention's cascade leaves") {
		t.Fatalf("AUDIT R15-JOIN(a): the note's NAME and TEXT are the contract: %q", out)
	}
	if strings.Contains(out, "action_evidence_incomplete") || strings.Contains(out, "custody_mismatch") {
		t.Fatalf("nothing is incomplete here — the cascade removed both: %q", out)
	}
}

// Outcome (b): the action row is present and its decision row is gone.
// A named FAILURE, exit != 0 — the state the verifier used to narrate as
// a retention prune while exiting 0.
func TestReceiptVerify_joinB_decisionRowGoneAloneIsANamedFailure(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, receiptID, actionID := operatorReceipt(t)
	db := rawStore(t, dbPath)
	// A CHILD row: no foreign-key override needed, which is what makes
	// this cheaper than every other reproduction in this train.
	if n := mustExec(t, db, `DELETE FROM action_decisions WHERE action_id = ?`, actionID); n != 1 {
		t.Fatalf("the attack deletes exactly one decision row, deleted %d", n)
	}
	if n := countWhere(t, db, `SELECT COUNT(*) FROM actions WHERE action_id = ?`, actionID); n != 1 {
		t.Fatalf("the premise is a LIVE action row, found %d", n)
	}

	code, stdout, stderr := runIntentCLI(t, "receipt", "verify", "--config", cfgPath, receiptID)
	out := stdout + stderr
	t.Logf("verdict with the decision row gone: exit %d %q", code, out)
	if code == 0 {
		t.Fatalf("AUDIT R15-JOIN(b): evidence no cascade can produce must FAIL, never exit 0: %q", out)
	}
	if !strings.Contains(out, "FAIL action_evidence_incomplete: the action row for \""+actionID+"\" is present but its decision row is gone — retention removes both together, so this is evidence no cascade can produce") {
		t.Fatalf("AUDIT R15-JOIN(b): the NAME and the TEXT are the contract: %q", out)
	}
	if strings.Contains(out, "action_row_absent") {
		t.Fatalf("the action row is ALIVE — naming it absent is the lie this cure removes: %q", out)
	}
}

// Outcome (c): both rows present — the normal path, no premature return,
// the custody comparisons actually run.
func TestReceiptVerify_joinC_bothRowsPresentRunsTheCustodyChecks(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, receiptID, actionID := operatorReceipt(t)
	db := rawStore(t, dbPath)
	if n := countWhere(t, db, `SELECT COUNT(*) FROM actions WHERE action_id = ?`, actionID); n != 1 {
		t.Fatalf("premise: a live action row, found %d", n)
	}
	if n := countWhere(t, db, `SELECT COUNT(*) FROM action_decisions WHERE action_id = ?`, actionID); n != 1 {
		t.Fatalf("premise: a live decision row, found %d", n)
	}

	code, stdout, stderr := runIntentCLI(t, "receipt", "verify", "--config", cfgPath, receiptID)
	out := stdout + stderr
	if code != 0 || !strings.Contains(out, "OK") {
		t.Fatalf("AUDIT R15-JOIN(c): an intact pair verifies: %d %q", code, out)
	}
	if strings.Contains(out, "action_row_absent") || strings.Contains(out, "action_evidence_incomplete") {
		t.Fatalf("neither absence arm may fire over an intact pair: %q", out)
	}

	// And the custody comparison is REACHED, which the premature return
	// used to prevent: corrupt the row's digest and the ladder must say
	// so by name rather than staying silent.
	mustExec(t, db, `UPDATE actions SET parameters_digest = 'sha256:0000000000000000000000000000000000000000000000000000000000000000' WHERE action_id = ?`, actionID)
	code, stdout, stderr = runIntentCLI(t, "receipt", "verify", "--config", cfgPath, receiptID)
	out = stdout + stderr
	if code == 0 || !strings.Contains(out, "custody_mismatch") {
		t.Fatalf("AUDIT R15-JOIN(c): the custody comparison must RUN when both rows are present: %d %q", code, out)
	}
}

// The auditor's own reproduction, verbatim: the two DELETEs that turned
// the anti-sabotage arm into exit 0. Both names now fire.
func TestReceiptVerify_joinFailOpen_twoChildDeletesNoLongerBuyAPass(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, approvalID := parkedRequest(t)
	receiptID := approvedReceiptID(t, cfgPath, dbPath, approvalID)
	const actionID = "act_inbox1" // the id parkedRequest seeds
	db := rawStore(t, dbPath)

	// Step 1 — the approval row alone. This already failed before R15
	// and must keep failing: a cascade cannot leave the action row.
	mustExec(t, db, `DELETE FROM approvals WHERE action_id = ?`, actionID)
	code, stdout, stderr := runIntentCLI(t, "receipt", "verify", "--config", cfgPath, receiptID)
	out := stdout + stderr
	if code == 0 || !strings.Contains(out, "approval_mismatch") {
		t.Fatalf("AUDIT R15-JOIN: step 1 must fail approval_mismatch: %d %q", code, out)
	}

	// Step 2 — the decision row, a child row. BEFORE the cure this
	// bought exit 0 and two notes narrating a prune that never happened.
	mustExec(t, db, `DELETE FROM action_decisions WHERE action_id = ?`, actionID)
	if n := countWhere(t, db, `SELECT COUNT(*) FROM actions WHERE action_id = ?`, actionID); n != 1 {
		t.Fatalf("the whole point is that the action row is ALIVE, found %d", n)
	}
	code, stdout, stderr = runIntentCLI(t, "receipt", "verify", "--config", cfgPath, receiptID)
	out = stdout + stderr
	t.Logf("verdict after both child deletes: exit %d %q", code, out)
	if code == 0 {
		t.Fatalf("AUDIT R15-JOIN: two child DELETEs must not buy a pass: %q", out)
	}
	if !strings.Contains(out, "approval_mismatch") {
		t.Fatalf("the sabotage arm still names the missing approval: %q", out)
	}
	if !strings.Contains(out, "action_evidence_incomplete") {
		t.Fatalf("and the incomplete evidence is named too: %q", out)
	}
	if strings.Contains(out, "action_row_absent") {
		t.Fatalf("no note may claim a prune here — the action row is alive: %q", out)
	}
}

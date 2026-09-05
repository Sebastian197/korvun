// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R12 (surgical, the first round under the internal adversary): the
// contract learns the DOMAIN's truth — system-origin decisions
// (decision='clock', written by the expiry touch and the sweep,
// approvals.go:210/:577) legitimately carry an empty
// decision_principal_id; human verbs demand a principal, and a
// human hand signing as the clock is an anomaly, both ways
// (X1). Readers apply THE one contract over the RAW row (X2+X6
// unified): present-but-empty or type-corrupt bytes read back are
// FaultCorrupt through errors.As, never a silent zero or a naked
// driver error; present=false only for a real stored NULL.
// Evidence level of THIS file: in-process suite. (The X1 acceptance
// was ALSO exercised by OS-process binary in the R12 gate — that
// evidence lives in the round's canto with its captured output, not
// in this tree; the label here claims only what this file runs.)
// Reproduction-first contract.

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// rawDB opens a raw connection to the store file.
func rawDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	return db
}

// sweepTombstoneRow builds the EXACT row the product's sweep writes:
// empty principal, decision "clock", digest derived over that story.
func sweepTombstoneRow(aprID, actID string) [10]any {
	a := actionpkgApproval(aprID, actID)
	a.DecisionPrincipalID = ""
	a.Decision = "clock"
	a.DecisionAt = time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	return [10]any{a.ApprovalID, a.Digest(), a.ActionID, a.ActionDigest, a.PreviewDigest,
		a.PolicyVersion, a.PolicyDigest, a.DecisionPrincipalID, a.Decision,
		a.DecisionAt.Format(time.RFC3339Nano)}
}

// A1 — the auditor's NORMAL-USE profile: an approval closed by the
// expiry sweep migrates to v12 CLEAN. Today the contract wrongly
// rejects the domain's own truth and blocks real profiles.
func TestMigrationV12_sweepClosedApprovalMigratesClean(t *testing.T) {
	t.Parallel()
	path := buildV11LegacyFile(t, sweepTombstoneRow("apr_r12_sweep0000000000000000001", "act_r12_sweep"))
	store, err := Open(path)
	if err != nil {
		t.Fatalf("AUDIT R12-A1 (P1#1, normal use): a sweep-closed approval is the DOMAIN's truth and must migrate clean: %v", err)
	}
	defer func() { _ = store.Close() }()
	if v := inspect(t, path, `SELECT version FROM action_schema`); v != 12 {
		t.Fatalf("v12 lands: %d", v)
	}
}

// A1-v10 (R12-P3-4): the SAME domain truth observed at the v10 door —
// a sweep tombstone in a v10 profile rides the copy up to v12 clean.
// (Its red lives in mutation m-p21: re-enable the digest contrast on
// the absent v10 column and every v10 row fails the judge.)
func TestMigrationV10Copy_sweepTombstoneMigratesCleanToV12(t *testing.T) {
	t.Parallel()
	path, _ := buildV10File(t)
	a := actionpkgApproval("apr_r12_v10sweep00000000000000001", "act_r12_v10sweep")
	a.DecisionPrincipalID = ""
	a.Decision = "clock"
	a.DecisionAt = time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	seedV10RawRow(t, path, [9]any{a.ActionID, a.ApprovalID, a.ActionDigest, a.PreviewDigest,
		a.PolicyVersion, a.PolicyDigest, "", "clock", a.DecisionAt.Format(time.RFC3339Nano)})
	store, err := Open(path)
	if err != nil {
		t.Fatalf("AUDIT R12-P3-4: the v10 sweep tombstone is the domain's truth at that door too: %v", err)
	}
	defer func() { _ = store.Close() }()
	if v := inspect(t, path, `SELECT version FROM action_schema`); v != 12 {
		t.Fatalf("v12 lands from v10: %d", v)
	}
	if _, _, err := store.ApprovalTombstoneByDigest(context.Background(), a.Digest()); err != nil {
		t.Fatalf("the sweep story reconstructs by its digest after the two-door ride: %v", err)
	}
}

// A2 — the adversarial pair: a HUMAN verb with an empty principal is
// corruption, named. (Born green against the current all-empty-reject
// contract; its red lives in mutation m-x1b.)
func TestMigrationV12_humanVerbWithEmptyPrincipalIsCorrupt(t *testing.T) {
	t.Parallel()
	a := actionpkgApproval("apr_r12_nohuman00000000000000001", "act_r12_nohuman")
	a.DecisionPrincipalID = ""
	a.DecisionAt = time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	row := [10]any{a.ApprovalID, a.Digest(), a.ActionID, a.ActionDigest, a.PreviewDigest,
		a.PolicyVersion, a.PolicyDigest, "", a.Decision, a.DecisionAt.Format(time.RFC3339Nano)}
	path := buildV11LegacyFile(t, row)
	_, err := Open(path)
	var fault *TombstoneFault
	if err == nil || !errors.As(err, &fault) || fault.Field != "decision_principal_id" {
		t.Fatalf("AUDIT R12-A2: a human decision without a principal is corrupt, named by stable field: %v", err)
	}
}

// A3 — the other direction: a human hand signing as the clock.
func TestMigrationV12_clockWithPrincipalIsCorrupt(t *testing.T) {
	t.Parallel()
	a := actionpkgApproval("apr_r12_fakeclock000000000000001", "act_r12_fakeclock")
	a.Decision = "clock"
	a.DecisionAt = time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	row := [10]any{a.ApprovalID, a.Digest(), a.ActionID, a.ActionDigest, a.PreviewDigest,
		a.PolicyVersion, a.PolicyDigest, a.DecisionPrincipalID, "clock", a.DecisionAt.Format(time.RFC3339Nano)}
	path := buildV11LegacyFile(t, row)
	_, err := Open(path)
	var fault *TombstoneFault
	if err == nil || !errors.As(err, &fault) || fault.Field != "decision_principal_id" {
		t.Fatalf("AUDIT R12-A3: a clock decision carrying a principal is an anomaly, named: %v", err)
	}
}

// A4 — the reader applies the contract: ” read back is CORRUPTION
// through the typed fault, never present=false err=nil.
func TestTombstoneReader_emptyDateReadBackIsCorruptTyped(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	a := actionpkgApproval("apr_r12_read0000000000000000001", "act_r12_read")
	if _, err := store.db.Exec(`INSERT INTO approval_tombstones
	    (approval_id, approval_digest, action_id, action_digest, preview_digest,
	     policy_version, policy_digest, decision_principal_id, decision, decision_at)
	 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '')`,
		a.ApprovalID, a.Digest(), a.ActionID, a.ActionDigest, a.PreviewDigest,
		a.PolicyVersion, a.PolicyDigest, a.DecisionPrincipalID, a.Decision); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, _, err := store.ApprovalTombstone(context.Background(), "act_r12_read")
	var fault *TombstoneFault
	if err == nil || !errors.As(err, &fault) || fault.Field != "decision_at" {
		t.Fatalf("AUDIT R12-A4: '' read back must be the typed corruption, never present=false err=nil: %v", err)
	}
}

// A5 — the pair: a REAL stored NULL is honest absence (present=false,
// err=nil). Born green; its red lives in mutation m-abs.
func TestTombstoneReader_nullDateReadBackIsHonestAbsence(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	a := actionpkgApproval("apr_r12_null0000000000000000001", "act_r12_null")
	if _, err := store.db.Exec(`INSERT INTO approval_tombstones
	    (approval_id, approval_digest, action_id, action_digest, preview_digest,
	     policy_version, policy_digest, decision_principal_id, decision, decision_at)
	 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		a.ApprovalID, a.Digest(), a.ActionID, a.ActionDigest, a.PreviewDigest,
		a.PolicyVersion, a.PolicyDigest, a.DecisionPrincipalID, a.Decision); err != nil {
		t.Fatalf("seed: %v", err)
	}
	tomb, present, err := store.ApprovalTombstone(context.Background(), "act_r12_null")
	if err != nil || present || !tomb.DecisionAt.IsZero() {
		t.Fatalf("AUDIT R12-A5: a real NULL is honest absence: present=%v err=%v", present, err)
	}
}

// A6 — the migration judges TYPES through the contract: a
// policy_version that is not an integer is FaultCorrupt naming the
// stable column, never a naked driver scan error.
func TestMigrationV12_typeCorruptPolicyVersionIsFaultTyped(t *testing.T) {
	t.Parallel()
	row := legacyGoodRow("apr_r12_type0000000000000000001", "act_r12_type")
	row[5] = "abc"
	path := buildV11LegacyFile(t, row)
	_, err := Open(path)
	var fault *TombstoneFault
	if err == nil || !errors.As(err, &fault) || fault.Field != "policy_version" {
		t.Fatalf("AUDIT R12-A6: a non-integer policy_version is typed corruption naming the column: %v", err)
	}
}

// A13 — the READER judges types through the same one contract.
func TestTombstoneReader_typeCorruptPolicyVersionIsFaultTyped(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	a := actionpkgApproval("apr_r12_rtype000000000000000001", "act_r12_rtype")
	a.DecisionAt = time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	// R12-P2-4 (the adversary caught the scrambled seed): the row is
	// coherent EXCEPT policy_version, which carries the type garbage.
	if _, err := store.db.Exec(`INSERT INTO approval_tombstones
	    (approval_id, approval_digest, action_id, action_digest, preview_digest,
	     policy_version, policy_digest, decision_principal_id, decision, decision_at)
	 VALUES (?, ?, ?, ?, ?, 'abc', ?, ?, ?, ?)`,
		a.ApprovalID, a.Digest(), a.ActionID, a.ActionDigest, a.PreviewDigest,
		a.PolicyDigest, a.DecisionPrincipalID, a.Decision,
		a.DecisionAt.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, _, err := store.ApprovalTombstone(context.Background(), "act_r12_rtype")
	var fault *TombstoneFault
	if err == nil || !errors.As(err, &fault) || fault.Field != "policy_version" {
		t.Fatalf("AUDIT R12-A13: the reader judges the policy_version TYPE through the ONE contract, Field fixed: %v", err)
	}
}

// A7 — the interruption that reaches EXACTLY the v11→v12 bump: v11
// intact byte for byte, the retry converges. (Born green — migrateStep
// is one tx; its red lives in mutation m-bump.)
func TestMigrationV12_bumpInterruptLeavesV11IntactAndRetryConverges(t *testing.T) {
	t.Parallel()
	path := buildV11LegacyFile(t, legacyGoodRow("apr_r12_bump0000000000000000001", "act_r12_bump"))
	db := rawDB(t, path)
	if _, err := db.Exec(`CREATE TRIGGER crash_v12_bump BEFORE UPDATE ON action_schema
	    WHEN NEW.version = 12 BEGIN SELECT RAISE(ABORT, 'crash at the v12 bump'); END;`); err != nil {
		t.Fatalf("arm: %v", err)
	}
	_ = db.Close()
	if _, err := Open(path); err == nil {
		t.Fatal("the aborted bump must be boot-fatal")
	}
	if v := inspect(t, path, `SELECT version FROM action_schema`); v != 11 {
		t.Fatalf("v11 stands: %d", v)
	}
	if n := inspect(t, path, `SELECT COUNT(*) FROM approval_tombstones`); n != 1 {
		t.Fatalf("the evidence stands: %d", n)
	}
	db2 := rawDB(t, path)
	if _, err := db2.Exec(`DROP TRIGGER crash_v12_bump`); err != nil {
		t.Fatalf("drop: %v", err)
	}
	_ = db2.Close()
	store, err := Open(path)
	if err != nil {
		t.Fatalf("the retry converges: %v", err)
	}
	defer func() { _ = store.Close() }()
	if v := inspect(t, path, `SELECT version FROM action_schema`); v != 12 {
		t.Fatalf("v12 lands on retry: %d", v)
	}
}

// A8 — the bad row FIRST in cursor order (kills a validate-only-the-
// last-row implementation). Born green with per-row validation; its
// fresh red is mutation m5 re-executed this round.
func TestMigrationV12_badFirstRowFailsNamed(t *testing.T) {
	t.Parallel()
	bad := legacyGoodRow("apr_r12_aabad000000000000000001", "act_r12_aabad")
	bad[9] = ""
	path := buildV11LegacyFile(t, bad,
		legacyGoodRow("apr_r12_zzok00000000000000000001", "act_r12_zzok"))
	_, err := Open(path)
	var fault *TombstoneFault
	if err == nil || !errors.As(err, &fault) ||
		fault.ApprovalID != "apr_r12_aabad000000000000000001" || fault.Field != "decision_at" {
		t.Fatalf("AUDIT R12-A8: the FIRST bad row fails by name: %v", err)
	}
}

// A10 — the fault is a real TYPE end to end: errors.As reaches it
// through the migration wrapper. (Mutation m-typed: replacing the
// typed fault with an equivalent fmt.Errorf must turn this red.)
func TestMigrationV12_faultTypeSurvivesWrapping(t *testing.T) {
	t.Parallel()
	row := legacyGoodRow("apr_r12_as000000000000000000001", "act_r12_as")
	row[9] = "not-a-date"
	path := buildV11LegacyFile(t, row)
	_, err := Open(path)
	var fault *TombstoneFault
	if err == nil || !errors.As(err, &fault) {
		t.Fatalf("AUDIT R12-A10: the typed fault must survive wrapping to the boot error: %v", err)
	}
	if fault.ApprovalID != "apr_r12_as000000000000000000001" || fault.Field != "decision_at" {
		t.Fatalf("stable coordinates on the type itself: %+v", fault)
	}
}

// S2-1 — the wall at the EXPORTED door: DecideApprovalUnderLaw with
// an empty deciding principal is refused by name, before any write.
// (Its red lives in mutation m-s21: neutralize the wall in
// decideApprovalWithLaw and this test must go red.)
func TestDecideApprovalUnderLaw_refusesAnEmptyPrincipal(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	a := expiredParked(t, store, "act_s21_wall")
	env, _ := operatorDecisionEnv("reject", a.ApprovalID)
	_, err := store.DecideApprovalUnderLaw(t.Context(), a.ApprovalID, "rejected",
		a.RequestedAt.Add(time.Second), env, AttemptIdentity{PrincipalID: ""}, "", PolicyPin{})
	if err == nil || !strings.Contains(err.Error(), "empty deciding principal") {
		t.Fatalf("AUDIT R12-S2-1: the EXPORTED decide door must refuse an empty principal by name: %v", err)
	}
	// And nothing was written: the request is still PENDING.
	full, _, gerr := store.GetApproval(t.Context(), a.ApprovalID)
	if gerr != nil || string(full.Status) != "PENDING" {
		t.Fatalf("the refused decide must mutate nothing: %v %s", gerr, full.Status)
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R12 (surgical, the first round under the internal adversary): the
// contract learns the DOMAIN's truth — system-origin decisions
// (decision='clock', written by the expiry touch in
// decideApprovalWithLaw and by sweepExpiredOne — the two
// closeApprovalTx callers in approvals.go) legitimately carry an empty
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

	"github.com/Sebastian197/korvun/internal/action"
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
// rejects the domain's own truth and blocks real profiles. (Its red
// lives in mutation m-x1: neutralize the clock-accepts branch.)
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

// A3 — the other direction: a human hand signing as the clock. (Its
// red lives in mutation m-x1c: neutralize the clock-with-principal
// branch.)
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

// A13 — the READER judges types through the same one contract. (Its
// red lives in mutation m-x2 too: the dropped fault silences both
// reader molds.)
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

// H1 — the in-tx idempotence reader (tombstoneStoredRowTx) judges the
// EXISTING row through THE one contract too (the twelfth review's
// finding 1: it was the last reader outside it). A type-corrupt
// policy_version on the existing row surfaces from tombstoneTx as the
// typed fault naming the stable column — never a naked driver scan
// error. (Its red lives in mutation m-h1: the reader drops the fault.)
func TestTombstoneIdempotence_typeCorruptExistingRowIsFaultTyped(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	a := actionpkgApproval("apr_r12_h1type000000000000000001", "act_r12_h1type")
	a.DecisionAt = time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	if _, err := store.db.Exec(`INSERT INTO approval_tombstones
	    (approval_id, approval_digest, action_id, action_digest, preview_digest,
	     policy_version, policy_digest, decision_principal_id, decision, decision_at)
	 VALUES (?, ?, ?, ?, ?, 'abc', ?, ?, ?, ?)`,
		a.ApprovalID, a.Digest(), a.ActionID, a.ActionDigest, a.PreviewDigest,
		a.PolicyDigest, a.DecisionPrincipalID, a.Decision,
		a.DecisionAt.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	tx, err := store.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	err = store.tombstoneTx(context.Background(), tx, a, a.DecisionPrincipalID, a.Decision, a.DecisionAt)
	var fault *TombstoneFault
	if err == nil || !errors.As(err, &fault) || fault.Field != "policy_version" {
		t.Fatalf("AUDIT R12-H1(a): the existing row's policy_version type is judged by the ONE contract, Field fixed: %v", err)
	}
	if strings.Contains(err.Error(), "Scan error") {
		t.Fatalf("a naked driver scan error is the class X6 killed: %v", err)
	}
}

// H1 — present-but-empty decision_at on the EXISTING row is the typed
// corruption from the idempotence path, never a tombstone_conflict
// (which would disguise corruption as history rewritten) and never a
// silent acceptance. (Mutation m-h1 reddens this one too.)
func TestTombstoneIdempotence_emptyDateExistingRowIsFaultTyped(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	a := actionpkgApproval("apr_r12_h1date000000000000000001", "act_r12_h1date")
	a.DecisionAt = time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	if _, err := store.db.Exec(`INSERT INTO approval_tombstones
	    (approval_id, approval_digest, action_id, action_digest, preview_digest,
	     policy_version, policy_digest, decision_principal_id, decision, decision_at)
	 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '')`,
		a.ApprovalID, a.Digest(), a.ActionID, a.ActionDigest, a.PreviewDigest,
		a.PolicyVersion, a.PolicyDigest, a.DecisionPrincipalID, a.Decision); err != nil {
		t.Fatalf("seed: %v", err)
	}
	tx, err := store.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	err = store.tombstoneTx(context.Background(), tx, a, a.DecisionPrincipalID, a.Decision, a.DecisionAt)
	var fault *TombstoneFault
	if err == nil || !errors.As(err, &fault) || fault.Field != "decision_at" {
		t.Fatalf("AUDIT R12-H1(b): '' on the existing row is typed corruption naming decision_at: %v", err)
	}
	if strings.Contains(err.Error(), "tombstone_conflict") {
		t.Fatalf("corruption must not be disguised as a rewritten history: %v", err)
	}
}

// H1 — regression pin (R10-V2 unchanged): a well-formed existing row
// accepts the byte-identical re-insert as a nil no-op and refuses a
// different story as the named tombstone_conflict. The comparison
// semantics over the STORED projection are not touched by the cure.
func TestTombstoneIdempotence_wellFormedRowKeepsR10V2Semantics(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	a := actionpkgApproval("apr_r12_h1same000000000000000001", "act_r12_h1same")
	a.DecisionAt = time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := store.tombstoneTx(ctx, tx, a, a.DecisionPrincipalID, a.Decision, a.DecisionAt); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	tx2, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin 2: %v", err)
	}
	defer func() { _ = tx2.Rollback() }()
	if err := store.tombstoneTx(ctx, tx2, a, a.DecisionPrincipalID, a.Decision, a.DecisionAt); err != nil {
		t.Fatalf("the byte-identical re-insert is a harmless no-op: %v", err)
	}
	err = store.tombstoneTx(ctx, tx2, a, a.DecisionPrincipalID, "approved", a.DecisionAt)
	if err == nil || !strings.Contains(err.Error(), "tombstone_conflict") {
		t.Fatalf("a different story is the named conflict, exactly as before: %v", err)
	}
	var fault *TombstoneFault
	if errors.As(err, &fault) {
		t.Fatalf("a well-formed row is not corruption: %v", err)
	}
}

// ---- H2/H3 (the twelfth review's findings 2 and 3): ONE origin-and-
// vocabulary rule (judgeTombstoneOrigin) with two doors — the read
// door (judgeStoredTombstone: migrations, scanTombstone, the in-tx
// idempotence read) and the write door (tombstoneTx, before the
// INSERT; decideApprovalWithLaw's S2-1 wall delegates to it).

// outOfVocabularyVerbs are the shapes finding 2 let through: a case
// variant of the clock, a STATUS spelling, and garbage.
var outOfVocabularyVerbs = []string{"CLOCK", "expired", "bogus"}

// badVerbRow is a stored tombstone whose only flaw is its verb: the
// principal is named and the digest re-derives over that very story.
func badVerbRow(aprID, actID, verb string) ([10]any, action.Approval) {
	a := actionpkgApproval(aprID, actID)
	a.Decision = verb
	a.DecisionAt = time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	return [10]any{a.ApprovalID, a.Digest(), a.ActionID, a.ActionDigest, a.PreviewDigest,
		a.PolicyVersion, a.PolicyDigest, a.DecisionPrincipalID, a.Decision,
		a.DecisionAt.Format(time.RFC3339Nano)}, a
}

// seedTombstoneRow inserts one raw tombstone row into an open store.
func seedTombstoneRow(t *testing.T, store *Store, row [10]any) {
	t.Helper()
	if _, err := store.db.Exec(`INSERT INTO approval_tombstones
	    (approval_id, approval_digest, action_id, action_digest, preview_digest,
	     policy_version, policy_digest, decision_principal_id, decision, decision_at)
	 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row[0], row[1], row[2], row[3], row[4], row[5], row[6], row[7], row[8], row[9]); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// H2(a) — read door 1, the v11→v12 revalidation: a verb outside the
// vocabulary is the typed fault at Field "decision", never a clean
// migration. (Its red lives in mutation m-h2: the rule accepts any
// non-clock string as a human verb.)
func TestMigrationV12_outOfVocabularyVerbIsCorruptAtDecision(t *testing.T) {
	t.Parallel()
	for _, verb := range outOfVocabularyVerbs {
		row, _ := badVerbRow("apr_r12_h2mig_"+verb+"0000000000001", "act_r12_h2mig_"+verb, verb)
		path := buildV11LegacyFile(t, row)
		_, err := Open(path)
		var fault *TombstoneFault
		if err == nil || !errors.As(err, &fault) || fault.Field != "decision" {
			t.Fatalf("AUDIT R12-H2(a) migration, verb %q: must be the typed fault at decision, never err=nil: %v", verb, err)
		}
		if v := inspect(t, path, `SELECT version FROM action_schema`); v != 11 {
			t.Fatalf("v11 stands for %q: %d", verb, v)
		}
	}
}

// H2(a) — read door 2, scanTombstone through ApprovalTombstone and
// ApprovalTombstoneByDigest. (Mutation m-h2.)
func TestTombstoneReader_outOfVocabularyVerbIsCorruptAtDecision(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	for _, verb := range outOfVocabularyVerbs {
		row, a := badVerbRow("apr_r12_h2read_"+verb+"000000000001", "act_r12_h2read_"+verb, verb)
		seedTombstoneRow(t, store, row)
		_, _, err := store.ApprovalTombstone(context.Background(), a.ActionID)
		var fault *TombstoneFault
		if err == nil || !errors.As(err, &fault) || fault.Field != "decision" {
			t.Fatalf("AUDIT R12-H2(a) by action, verb %q: %v", verb, err)
		}
		_, _, err = store.ApprovalTombstoneByDigest(context.Background(), a.Digest())
		fault = nil
		if err == nil || !errors.As(err, &fault) || fault.Field != "decision" {
			t.Fatalf("AUDIT R12-H2(a) by digest, verb %q: %v", verb, err)
		}
	}
}

// H2(a) — read door 3, the in-tx idempotence read: a VALID story
// re-inserted over a stored bad-verb row (same approval_id) hits the
// UNIQUE conflict and the EXISTING row is judged — the typed fault at
// decision, never tombstone_conflict. (The valid re-insert passes the
// write door on purpose, so this mold reaches the READ side; the
// write door has its own molds in H3.) (Mutation m-h2.)
func TestTombstoneIdempotence_outOfVocabularyExistingRowIsCorruptAtDecision(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	for _, verb := range outOfVocabularyVerbs {
		row, a := badVerbRow("apr_r12_h2idem_"+verb+"000000000001", "act_r12_h2idem_"+verb, verb)
		seedTombstoneRow(t, store, row)
		tx, err := store.db.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		err = store.tombstoneTx(context.Background(), tx, a, a.DecisionPrincipalID, action.DecisionRejected, a.DecisionAt)
		_ = tx.Rollback()
		var fault *TombstoneFault
		if err == nil || !errors.As(err, &fault) || fault.Field != "decision" {
			t.Fatalf("AUDIT R12-H2(a) idempotence, verb %q: the EXISTING row's verb is judged: %v", verb, err)
		}
		if strings.Contains(err.Error(), "tombstone_conflict") {
			t.Fatalf("corruption is not a rewritten history: %v", err)
		}
	}
}

// H2(b) — the principal pairs at the reader and idempotence doors
// (the migration door is already covered by A2 and A3 above): a clock
// row carrying a principal, and a human verb with an empty one, are
// the typed fault at decision_principal_id. Born green — the origin
// rule already lived in the contract (X1); its reds are mutations
// m-x1b and m-x1c, re-executed for these molds in the H2 addendum.
func TestTombstoneReaderAndIdempotence_principalPairsAreCorruptAtPrincipal(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	shapes := []struct{ name, verb, principal string }{
		{"clock-with-principal", action.DecisionClock, "principal_operator"},
		{"human-without-principal", action.DecisionRejected, ""},
	}
	for _, sh := range shapes {
		a := actionpkgApproval("apr_r12_h2b_"+sh.name+"000000001", "act_r12_h2b_"+sh.name)
		a.Decision, a.DecisionPrincipalID = sh.verb, sh.principal
		a.DecisionAt = time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
		seedTombstoneRow(t, store, [10]any{a.ApprovalID, a.Digest(), a.ActionID, a.ActionDigest, a.PreviewDigest,
			a.PolicyVersion, a.PolicyDigest, a.DecisionPrincipalID, a.Decision, a.DecisionAt.Format(time.RFC3339Nano)})
		_, _, err := store.ApprovalTombstone(context.Background(), a.ActionID)
		var fault *TombstoneFault
		if err == nil || !errors.As(err, &fault) || fault.Field != "decision_principal_id" {
			t.Fatalf("AUDIT R12-H2(b) reader, %s: %v", sh.name, err)
		}
		tx, err := store.db.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		// A VALID story over the same approval_id reaches the read side.
		err = store.tombstoneTx(context.Background(), tx, a, "principal_operator", action.DecisionRejected, a.DecisionAt)
		_ = tx.Rollback()
		fault = nil
		if err == nil || !errors.As(err, &fault) || fault.Field != "decision_principal_id" {
			t.Fatalf("AUDIT R12-H2(b) idempotence, %s: %v", sh.name, err)
		}
	}
}

// H3(a) — the WRITE door: tombstoneTx refuses at birth, by the same
// rule, a human verb without a decider, the clock with a decider, and
// a verb outside the vocabulary — typed fault, and the table is
// untouched (COUNT(*) unchanged; the INSERT never ran). (Its red lives
// in mutation m-h3: tombstoneTx stops calling the rule.)
func TestTombstoneTx_refusesAtBirthByTheOriginRule(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	shapes := []struct{ name, verb, decider, field string }{
		{"human-without-decider", action.DecisionRejected, "", "decision_principal_id"},
		{"clock-with-decider", action.DecisionClock, "principal_operator", "decision_principal_id"},
		{"out-of-vocabulary", "bogus", "principal_operator", "decision"},
	}
	for _, sh := range shapes {
		a := actionpkgApproval("apr_r12_h3_"+sh.name+"0000000001", "act_r12_h3_"+sh.name)
		at := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
		var before int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM approval_tombstones`).Scan(&before); err != nil {
			t.Fatalf("count: %v", err)
		}
		tx, err := store.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		err = store.tombstoneTx(ctx, tx, a, sh.decider, sh.verb, at)
		var fault *TombstoneFault
		if err == nil || !errors.As(err, &fault) || fault.Field != sh.field {
			_ = tx.Rollback()
			t.Fatalf("AUDIT R12-H3(a) %s: the write door must refuse by the rule at %s: %v", sh.name, sh.field, err)
		}
		// Observed INSIDE the same tx: the refusal wrote nothing, not
		// merely "the rollback cleaned it".
		var inTx int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM approval_tombstones`).Scan(&inTx); err != nil {
			t.Fatalf("count in tx: %v", err)
		}
		_ = tx.Rollback()
		if inTx != before {
			t.Fatalf("AUDIT R12-H3(a) %s: the refused birth must write NOTHING: %d → %d", sh.name, before, inTx)
		}
	}
}

// H3(b) — regression guard: the two production doors still write —
// the sweep's clock close (empty decider) and the human decide path
// (named principal) both land their tombstone and read back clean.
// Born green.
func TestTombstoneTx_productionDoorsStillWrite(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	swept := expiredParked(t, store, "act_r12_h3b_sweep")
	if n, _, err := store.SweepExpiredApprovals(ctx, swept.ExpiresAt.Add(time.Hour)); err != nil || n != 1 {
		t.Fatalf("sweep: n=%d err=%v", n, err)
	}
	tomb, _, err := store.ApprovalTombstone(ctx, "act_r12_h3b_sweep")
	if err != nil || tomb.Decision != action.DecisionClock || tomb.DecisionPrincipalID != "" {
		t.Fatalf("the clock door still writes: %+v %v", tomb, err)
	}
	human := expiredParked(t, store, "act_r12_h3b_human")
	env, id := operatorDecisionEnv("reject", human.ApprovalID)
	if _, err := store.decideApproval(ctx, human.ApprovalID, action.DecisionRejected,
		human.RequestedAt.Add(time.Second), env, id, "the decision"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	tomb, _, err = store.ApprovalTombstone(ctx, "act_r12_h3b_human")
	if err != nil || tomb.Decision != action.DecisionRejected || tomb.DecisionPrincipalID == "" {
		t.Fatalf("the human door still writes: %+v %v", tomb, err)
	}
}

// ---- Adversary train pass (P2-5, P2-7): the fault's class follows
// where the bytes came from, and a foreign row carrying this story's
// digest is named by ITS approval id.

// P2-5 — the two entry doors refuse WITHOUT claiming corruption: no
// row was read, nothing was written, so the class is tombstone_refused
// and the repair procedure is not invoked; the read door over stored
// bytes stays tombstone_corrupt. (Mutation m-p25: stamp Stored=true at
// the doors too — both door asserts go red.)
func TestTombstoneFault_classFollowsTheDoor(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	// Write door.
	a := actionpkgApproval("apr_r12_p25_door0000000000000001", "act_r12_p25_door")
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	err = store.tombstoneTx(ctx, tx, a, "", action.DecisionRejected, time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC))
	_ = tx.Rollback()
	var fault *TombstoneFault
	if err == nil || !errors.As(err, &fault) || fault.Stored || !strings.Contains(err.Error(), "tombstone_refused") ||
		strings.Contains(err.Error(), "tombstone_corrupt") || strings.Contains(err.Error(), "manual-repair") {
		t.Fatalf("AUDIT P2-5 write door: a refusal at birth must not narrate corruption: %v", err)
	}
	// Decision door (the S2-1 wall through the exported surface).
	parked := expiredParked(t, store, "act_r12_p25_decide")
	env, _ := operatorDecisionEnv("reject", parked.ApprovalID)
	_, err = store.DecideApprovalUnderLaw(ctx, parked.ApprovalID, action.DecisionRejected,
		parked.RequestedAt.Add(time.Second), env, AttemptIdentity{PrincipalID: ""}, "", PolicyPin{})
	fault = nil
	if err == nil || !errors.As(err, &fault) || fault.Stored || !strings.Contains(err.Error(), "tombstone_refused") ||
		strings.Contains(err.Error(), "tombstone_corrupt") {
		t.Fatalf("AUDIT P2-5 decision door: a refusal at the wall must not narrate corruption: %v", err)
	}
	// Read door: stored bytes ARE corruption, and say so.
	bad := actionpkgApproval("apr_r12_p25_read0000000000000001", "act_r12_p25_read")
	bad.DecisionPrincipalID = ""
	bad.DecisionAt = time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	seedTombstoneRow(t, store, [10]any{bad.ApprovalID, bad.Digest(), bad.ActionID, bad.ActionDigest, bad.PreviewDigest,
		bad.PolicyVersion, bad.PolicyDigest, "", bad.Decision, bad.DecisionAt.Format(time.RFC3339Nano)})
	_, _, err = store.ApprovalTombstone(ctx, bad.ActionID)
	fault = nil
	if err == nil || !errors.As(err, &fault) || !fault.Stored || !strings.Contains(err.Error(), "tombstone_corrupt") ||
		!strings.Contains(err.Error(), "tombstone-manual-repair.md") {
		t.Fatalf("AUDIT P2-5 read door: stored bytes that fail the contract ARE corruption, named with the procedure: %v", err)
	}
}

// P2-7 — the adversary's reproduction verbatim: a well-formed row B
// whose approval_digest column was overwritten with story A's digest.
// Inserting A collides on the UNIQUE digest with NO row by A's id; the
// foreign row is read by the digest and judged — the typed fault names
// B (the row carrying a digest that is not its own) at
// approval_digest, Stored, never a bare sql.ErrNoRows about a row
// that "cannot be judged". (Mutation m-p27: drop the by-digest arm —
// the assert on the fault's ApprovalID goes red.)
func TestTombstoneTx_digestCollisionNamesTheForeignRow(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	b := actionpkgApproval("apr_r12_p27_b0000000000000000001", "act_r12_p27_b")
	b.DecisionAt = time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	seedTombstoneRow(t, store, [10]any{b.ApprovalID, b.Digest(), b.ActionID, b.ActionDigest, b.PreviewDigest,
		b.PolicyVersion, b.PolicyDigest, b.DecisionPrincipalID, b.Decision, b.DecisionAt.Format(time.RFC3339Nano)})
	a := actionpkgApproval("apr_r12_p27_a0000000000000000001", "act_r12_p27_a")
	a.DecisionAt = b.DecisionAt
	if _, err := store.db.Exec(`UPDATE approval_tombstones SET approval_digest = ? WHERE approval_id = ?`,
		a.Digest(), b.ApprovalID); err != nil {
		t.Fatalf("auditor's UPDATE: %v", err)
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	err = store.tombstoneTx(ctx, tx, a, a.DecisionPrincipalID, a.Decision, a.DecisionAt)
	var fault *TombstoneFault
	if err == nil || !errors.As(err, &fault) || fault.ApprovalID != b.ApprovalID || fault.Field != "approval_digest" || !fault.Stored {
		t.Fatalf("AUDIT P2-7: the foreign row must be named by ITS id at approval_digest, stored: %v", err)
	}
	if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "cannot be judged") {
		t.Fatalf("a foreign digest is not an unreadable row: %v", err)
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM approval_tombstones`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("nothing written over the foreign story: n=%d err=%v", n, err)
	}
}

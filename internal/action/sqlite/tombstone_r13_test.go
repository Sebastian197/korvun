// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R13 (surgical, under the internal adversary; the paper is
// docs/superpowers/specs/2026-09-05-r13-pre-review.md): the judge
// becomes ONE declared traversal BY COLUMNS — per column, the storage
// class read by typeof() in the SAME SELECT (1a: an empty class is
// "class unreadable"; 1b: a class outside the column's allow-list is
// "storage class <class>"), then the two NULL witnesses (2a: class and
// value must agree; 2b: a real NULL is the Detail "NULL", never
// "empty"), then emptiness (3), and only then the cross-column rules
// (4 origin/vocabulary, 5 policy_version, 6 decision_at, 7 digest).
// The v10→v11 copy declares its origin STRUCTURALLY (origin=v10copy):
// the digest column is skipped and its pair MUST be the zero pair the
// v10 SELECT leaves — a Valid digest or a non-empty class there is
// refused as "v10 origin carries a digest column (class X, valid=B)",
// judged as a PRE-CHECK before column 1.
//
// Evidence level of THIS file: in-process suite — the synthetic table
// enters judgeRawTombstone(r, origin) directly (the raw door, no
// Stored stamp: those rows assert Field and Detail, never Stored);
// the migration, reader and idempotence molds go through real SELECTs
// of an open store or a migrating file. Reproduction-first contract.

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// ---- the raw shape, synthetic (class and value set independently) ----

func rawText(s string) rawColumn {
	return rawColumn{class: "text", value: sql.NullString{String: s, Valid: true}}
}
func rawInt(s string) rawColumn {
	return rawColumn{class: "integer", value: sql.NullString{String: s, Valid: true}}
}
func rawNull() rawColumn { return rawColumn{class: "null"} }

// rawZero is the pair the v10 SELECT leaves for the digest it never
// scans: class "", Valid=false — the state the v10copy rule expects.
func rawZero() rawColumn { return rawColumn{} }

var r13At = time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)

// cleanRaw is a coherent v11+ row as a real SELECT would hand it: every
// class in its allow-list, every value Valid, the digest re-deriving.
func cleanRaw(a action.Approval) rawTombstone {
	r := rawTombstone{
		approvalID:    rawText(a.ApprovalID),
		digest:        rawText(a.Digest()),
		actionID:      rawText(a.ActionID),
		actionDigest:  rawText(a.ActionDigest),
		previewDigest: rawText(a.PreviewDigest),
		policyVersion: rawInt(fmt.Sprint(a.PolicyVersion)),
		policyDigest:  rawText(a.PolicyDigest),
		principal:     rawText(a.DecisionPrincipalID),
		decision:      rawText(a.Decision),
		decisionAt:    rawNull(),
	}
	if !a.DecisionAt.IsZero() {
		r.decisionAt = rawText(a.DecisionAt.UTC().Format(time.RFC3339Nano))
	}
	return r
}

// G2/§3 — the exact table: every row of the paper's "Combined cases"
// asserting Field AND the exact Detail through the raw door. The
// closed guards (1a, the v10copy pre-check, the disagreement guard)
// are reachable ONLY by this synthetic shape — labeled so.
func TestJudgeOrder_exactTable(t *testing.T) {
	t.Parallel()
	base := actionpkgApproval("apr_r13_order0000000000000000001", "act_r13_order")
	base.DecisionAt = r13At
	zeroTime := base
	zeroTime.DecisionAt = time.Time{}
	rows := []struct {
		name   string
		origin tombstoneOrigin
		build  func() rawTombstone
		field  string
		detail string
		pass   bool
	}{
		{"bogus verb + '' principal", originV11Plus, func() rawTombstone {
			a := base
			a.Decision, a.DecisionPrincipalID = "bogus", ""
			return cleanRaw(a)
		}, "decision", `verb "bogus" is outside the vocabulary (the human verbs or the clock)`, false},
		{"bogus verb + NULL principal (column 8 precedes column 9)", originV11Plus, func() rawTombstone {
			a := base
			a.Decision = "bogus"
			r := cleanRaw(a)
			r.principal = rawNull()
			return r
		}, "decision_principal_id", "NULL", false},
		{"decision BLOB + '' principal (the principal is never judged at 1-3)", originV11Plus, func() rawTombstone {
			a := base
			a.DecisionPrincipalID = ""
			r := cleanRaw(a)
			r.decision = rawColumn{class: "blob", value: sql.NullString{String: "rejected", Valid: true}}
			return r
		}, "decision", "storage class blob", false},
		{"decision '' (step 3, the read door)", originV11Plus, func() rawTombstone {
			r := cleanRaw(base)
			r.decision = rawText("")
			return r
		}, "decision", "empty", false},
		{"policy_version '' is text", originV11Plus, func() rawTombstone {
			r := cleanRaw(base)
			r.policyVersion = rawText("")
			return r
		}, "policy_version", "storage class text", false},
		{"policy_version NULL", originV11Plus, func() rawTombstone {
			r := cleanRaw(base)
			r.policyVersion = rawNull()
			return r
		}, "policy_version", "NULL", false},
		{"policy_version X'33'", originV11Plus, func() rawTombstone {
			r := cleanRaw(base)
			r.policyVersion = rawColumn{class: "blob", value: sql.NullString{String: "3", Valid: true}}
			return r
		}, "policy_version", "storage class blob", false},
		{"policy_version 3.5 is real", originV11Plus, func() rawTombstone {
			r := cleanRaw(base)
			r.policyVersion = rawColumn{class: "real", value: sql.NullString{String: "3.5", Valid: true}}
			return r
		}, "policy_version", "storage class real", false},
		{"approval_digest NULL, v11+ (A1)", originV11Plus, func() rawTombstone {
			r := cleanRaw(base)
			r.digest = rawNull()
			return r
		}, "approval_digest", "NULL", false},
		{"approval_digest '' on v11+ is empty, not a contrast (R2)", originV11Plus, func() rawTombstone {
			r := cleanRaw(base)
			r.digest = rawText("")
			return r
		}, "approval_digest", "empty", false},
		{"TWOFAULTS: action_id NULL + decision BLOB (by columns)", originV11Plus, func() rawTombstone {
			r := cleanRaw(base)
			r.actionID = rawNull()
			r.decision = rawColumn{class: "blob", value: sql.NullString{String: "rejected", Valid: true}}
			return r
		}, "action_id", "NULL", false},
		{"decision_at NULL + digest not re-deriving (step 7)", originV11Plus, func() rawTombstone {
			r := cleanRaw(zeroTime)
			r.digest = rawText("sha256:other")
			return r
		}, "approval_digest", fmt.Sprintf("stored %q does not re-derive from the preimage (%s)", "sha256:other", zeroTime.Digest()), false},
		{"clock + '' principal passes (R12-A1)", originV11Plus, func() rawTombstone {
			a := base
			a.Decision, a.DecisionPrincipalID = action.DecisionClock, ""
			return cleanRaw(a)
		}, "", "", true},
		{"clock + NULL principal", originV11Plus, func() rawTombstone {
			a := base
			a.Decision, a.DecisionPrincipalID = action.DecisionClock, ""
			r := cleanRaw(a)
			r.principal = rawNull()
			return r
		}, "decision_principal_id", "NULL", false},
		{"decision_at NULL is honest absence", originV11Plus, func() rawTombstone {
			return cleanRaw(zeroTime)
		}, "", "", true},
		{"decision_at present-but-empty", originV11Plus, func() rawTombstone {
			r := cleanRaw(base)
			r.decisionAt = rawText("")
			return r
		}, "decision_at", "present-but-empty bytes are empty evidence, not absence", false},
		{"decision_at unreadable bytes", originV11Plus, func() rawTombstone {
			r := cleanRaw(base)
			r.decisionAt = rawText("not-a-date")
			return r
		}, "decision_at", `unreadable bytes "not-a-date"`, false},
		{"v10copy origin, zero digest pair passes (the copy's positive control)", originV10Copy, func() rawTombstone {
			r := cleanRaw(base)
			r.digest = rawZero()
			return r
		}, "", "", true},
		{"v10copy origin, Valid digest (class text, bytes = a.Digest())", originV10Copy, func() rawTombstone {
			return cleanRaw(base)
		}, "approval_digest", "v10 origin carries a digest column (class text, valid=true)", false},
		{"v10copy origin, class null with Valid=false on the digest (the TYPEOF half)", originV10Copy, func() rawTombstone {
			r := cleanRaw(base)
			r.digest = rawNull()
			return r
		}, "approval_digest", "v10 origin carries a digest column (class null, valid=false)", false},
		{"v10copy origin, class '' on approval_id AND a Valid digest (the PRE-CHECK precedes column 1)", originV10Copy, func() rawTombstone {
			r := cleanRaw(base)
			r.approvalID = rawColumn{class: "", value: sql.NullString{String: base.ApprovalID, Valid: true}}
			return r
		}, "approval_digest", "v10 origin carries a digest column (class text, valid=true)", false},
		{"class '' on action_digest, Valid (1a, closed guard, synthetic)", originV11Plus, func() rawTombstone {
			r := cleanRaw(base)
			r.actionDigest = rawColumn{class: "", value: sql.NullString{String: base.ActionDigest, Valid: true}}
			return r
		}, "action_digest", "class unreadable", false},
		{"disagreement form (i): class null with a Valid value (2a, closed guard, synthetic)", originV11Plus, func() rawTombstone {
			r := cleanRaw(base)
			r.decision = rawColumn{class: "null", value: sql.NullString{String: "rejected", Valid: true}}
			return r
		}, "decision", "class and value disagree (null, valid=true)", false},
		{"disagreement form (ii): class text with Valid=false (2a, closed guard, synthetic)", originV11Plus, func() rawTombstone {
			r := cleanRaw(base)
			r.decision = rawColumn{class: "text"}
			return r
		}, "decision", "class and value disagree (text, valid=false)", false},
		{"integer class yet unparsable bytes (step 5, closed guard, synthetic)", originV11Plus, func() rawTombstone {
			r := cleanRaw(base)
			r.policyVersion = rawInt("abc")
			return r
		}, "policy_version", "integer class yet unparsable bytes", false},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			a, present, fault := judgeRawTombstone(row.build(), row.origin)
			if row.pass {
				if fault != nil {
					t.Fatalf("must pass, got %v", fault)
				}
				if a.ApprovalID != base.ApprovalID {
					t.Fatalf("the parsed preimage rides out: %+v", a)
				}
				_ = present
				return
			}
			if fault == nil {
				t.Fatalf("must fault at %s (%s), got a pass", row.field, row.detail)
			}
			if fault.Field != row.field || fault.Detail != row.detail {
				t.Fatalf("exact Field/Detail: want %s (%s), got %s (%s)", row.field, row.detail, fault.Field, fault.Detail)
			}
			if fault.Stored {
				t.Fatalf("the raw door never stamps Stored: %+v", fault)
			}
		})
	}
}

// ---- table rebuilds: the live DDL has NOT NULL on every column and
// UNIQUE on the digest, so a NULL cannot be INSERTed into an open
// store; each NULL mold REBUILDS approval_tombstones inside it (same
// columns, no NOT NULL, no UNIQUE, the tombstones_by_action index
// re-created). approval_id's fixture drops PK/WITHOUT ROWID too —
// SQLite enforces NOT NULL on WITHOUT ROWID primary keys. Declared.

func rebuildTombstonesNullable(t *testing.T, db *sql.DB, keepPK bool) {
	t.Helper()
	pk, wr := "PRIMARY KEY", " WITHOUT ROWID"
	if !keepPK {
		pk, wr = "", ""
	}
	if _, err := db.Exec(fmt.Sprintf(`
CREATE TABLE approval_tombstones_r13 (
    approval_id           TEXT %s,
    approval_digest       TEXT,
    action_id             TEXT,
    action_digest         TEXT,
    preview_digest        TEXT,
    policy_version        INTEGER,
    policy_digest         TEXT,
    decision_principal_id TEXT,
    decision              TEXT,
    decision_at           TEXT
)%s;
INSERT INTO approval_tombstones_r13
  SELECT approval_id, approval_digest, action_id, action_digest, preview_digest,
         policy_version, policy_digest, decision_principal_id, decision, decision_at
    FROM approval_tombstones;
DROP TABLE approval_tombstones;
ALTER TABLE approval_tombstones_r13 RENAME TO approval_tombstones;
CREATE INDEX tombstones_by_action ON approval_tombstones(action_id);`, pk, wr)); err != nil {
		t.Fatalf("rebuild nullable: %v", err)
	}
}

// rebuildV10TombstonesNullable does the same for a v10 file (PK action_id).
func rebuildV10TombstonesNullable(t *testing.T, path string) {
	t.Helper()
	db := rawDB(t, path)
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`
CREATE TABLE approval_tombstones_r13 (
    action_id             TEXT    PRIMARY KEY,
    approval_id           TEXT,
    action_digest         TEXT,
    preview_digest        TEXT,
    policy_version        INTEGER,
    policy_digest         TEXT,
    decision_principal_id TEXT,
    decision              TEXT,
    decision_at           TEXT
) WITHOUT ROWID;
INSERT INTO approval_tombstones_r13
  SELECT action_id, approval_id, action_digest, preview_digest, policy_version,
         policy_digest, decision_principal_id, decision, decision_at
    FROM approval_tombstones;
DROP TABLE approval_tombstones;
ALTER TABLE approval_tombstones_r13 RENAME TO approval_tombstones;`); err != nil {
		t.Fatalf("rebuild v10 nullable: %v", err)
	}
}

func datedGoodRow(aprID, actID string) [10]any {
	a := actionpkgApproval(aprID, actID)
	a.DecisionAt = r13At
	return [10]any{a.ApprovalID, a.Digest(), a.ActionID, a.ActionDigest, a.PreviewDigest,
		a.PolicyVersion, a.PolicyDigest, a.DecisionPrincipalID, a.Decision,
		a.DecisionAt.Format(time.RFC3339Nano)}
}

// mustFault asserts the exact typed fault out of an Open/reader error.
func mustFault(t *testing.T, label string, err error, field, detail string, stored bool) {
	t.Helper()
	var fault *TombstoneFault
	if err == nil || !errors.As(err, &fault) {
		t.Fatalf("%s: the typed fault must ride out, got %v", label, err)
	}
	if fault.Field != field || fault.Detail != detail || fault.Stored != stored {
		t.Fatalf("%s: want %s (%s) stored=%v, got %s (%s) stored=%v: %v", label, field, detail, stored, fault.Field, fault.Detail, fault.Stored, err)
	}
}

// ---- G1 at the v12 door ----

// A1 — the auditor's reproduction verbatim: a v11 table rebuilt without
// NOT NULL, the digest NULL, Open. The fault names approval_digest
// with Detail "NULL" (never "empty": NULL is absence, not a value),
// and v11 stands. (Mutation m-n1: the origin derived from Valid
// instead of declared — the row faults at the v10copy pre-check
// instead, red BY DETAIL.)
func TestMigrationV12_nullDigestOnV11OriginIsFault(t *testing.T) {
	t.Parallel()
	path := buildV11LegacyFile(t, datedGoodRow("apr_r13_a1null000000000000000001", "act_r13_a1null"))
	db := rawDB(t, path)
	rebuildTombstonesNullable(t, db, true)
	if _, err := db.Exec(`UPDATE approval_tombstones SET approval_digest = NULL`); err != nil {
		t.Fatalf("auditor's UPDATE: %v", err)
	}
	_ = db.Close()
	_, err := Open(path)
	mustFault(t, "AUDIT R13-A1", err, "approval_digest", "NULL", true)
	if v := inspect(t, path, `SELECT version FROM action_schema`); v != 11 {
		t.Fatalf("v11 stands: %d", v)
	}
}

// A2 — a real NULL in any evidence column except decision_at is the
// typed fault naming the column with Detail "NULL" at the v12 door.
// The eight columns: approval_id (PK dropped for the fixture),
// action_id, action_digest, preview_digest, policy_version,
// policy_digest, decision_principal_id, decision. (Mutation m-n1b:
// step 2b removed — the TEXT subtests fall to "empty", the principal
// to step 4, policy_version to the step-5 guard.)
func TestMigrationV12_nullEvidenceColumnIsFaultNamed(t *testing.T) {
	t.Parallel()
	for _, col := range []string{"approval_id", "action_id", "action_digest", "preview_digest",
		"policy_version", "policy_digest", "decision_principal_id", "decision"} {
		t.Run(col, func(t *testing.T) {
			t.Parallel()
			path := buildV11LegacyFile(t, datedGoodRow("apr_r13_a2_"+col+"0000000001", "act_r13_a2_"+col))
			db := rawDB(t, path)
			rebuildTombstonesNullable(t, db, col != "approval_id")
			if _, err := db.Exec(`UPDATE approval_tombstones SET ` + col + ` = NULL`); err != nil { //nolint:gosec // G202: the column name is a literal of this test's table
				t.Fatalf("auditor's UPDATE: %v", err)
			}
			_ = db.Close()
			_, err := Open(path)
			mustFault(t, "AUDIT R13-A2 "+col, err, col, "NULL", true)
			if v := inspect(t, path, `SELECT version FROM action_schema`); v != 11 {
				t.Fatalf("v11 stands: %d", v)
			}
		})
	}
}

// ---- G4 at the v12 door: the class is judged, named, at the column ----

func v12ClassMold(t *testing.T, tag, set, field, detail string) {
	t.Helper()
	path := buildV11LegacyFile(t, datedGoodRow("apr_r13_"+tag+"00000000000000001", "act_r13_"+tag))
	db := rawDB(t, path)
	if _, err := db.Exec(`UPDATE approval_tombstones SET ` + set); err != nil { //nolint:gosec // G202: the SET clause is a literal of the calling mold
		t.Fatalf("auditor's UPDATE: %v", err)
	}
	_ = db.Close()
	_, err := Open(path)
	mustFault(t, "AUDIT R13-G4 "+tag, err, field, detail, true)
	if v := inspect(t, path, `SELECT version FROM action_schema`); v != 11 {
		t.Fatalf("v11 stands: %d", v)
	}
}

// A BLOB X'33' in policy_version: the row seals PolicyVersion 3 and
// CAST(X'33' AS TEXT) is "3", so only the CLASS condemns it — the
// acceptance of G4. (Mutation m-n4b: the allow-list admits blob → the
// row PASSES, red by no-fault.)
func TestMigrationV12_blobPolicyVersionIsFault(t *testing.T) {
	t.Parallel()
	v12ClassMold(t, "pvblob", `policy_version = X'33'`, "policy_version", "storage class blob")
}

// 3.5 in an INTEGER column is real. (Mutation m-n4f.)
func TestMigrationV12_realPolicyVersionIsFault(t *testing.T) {
	t.Parallel()
	v12ClassMold(t, "pvreal", `policy_version = 3.5`, "policy_version", "storage class real")
}

// ” in an INTEGER column stays text. (Mutation m-n4e.)
func TestMigrationV12_emptyTextPolicyVersionIsFault(t *testing.T) {
	t.Parallel()
	v12ClassMold(t, "pvempty", `policy_version = ''`, "policy_version", "storage class text")
}

// A zero-length BLOB in decision scans as Valid=true, String "" under
// class blob (captured by execution, the paper's §3 step 2): step 1b
// names the class BEFORE emptiness could. (Mutation m-n4b: the class
// admitted → step 3 names "empty", red by Detail.)
func TestMigrationV12_emptyBlobIsStorageClassBlob(t *testing.T) {
	t.Parallel()
	v12ClassMold(t, "decblob", `decision = X''`, "decision", "storage class blob")
}

// ---- the v10 door: the copy judges classes and NULL too ----

// X'33' of v10 ORIGIN: the copy refuses by class and — the
// normalization G4 forbids — writes NOTHING: v10 stands with its BLOB
// intact, no row of that approval ever lands as the integer 3.
// (Mutation m-n4c at store.go:647: the class lied 'integer' → the copy
// NORMALIZES SILENTLY, v12 lands, and the integer 3 exists.)
func TestMigrationV11_blobPolicyVersionInV10IsFault(t *testing.T) {
	t.Parallel()
	path, seed := buildV10File(t)
	db := rawDB(t, path)
	if _, err := db.Exec(`UPDATE approval_tombstones SET policy_version = X'33' WHERE action_id = ?`, seed.ActionID); err != nil {
		t.Fatalf("auditor's UPDATE: %v", err)
	}
	_ = db.Close()
	_, err := Open(path)
	mustFault(t, "AUDIT R13-G4 v10 blob", err, "policy_version", "storage class blob", true)
	if v := inspect(t, path, `SELECT version FROM action_schema`); v != 10 {
		t.Fatalf("v10 stands: %d", v)
	}
	if n := inspect(t, path, `SELECT COUNT(*) FROM approval_tombstones WHERE typeof(policy_version) = 'blob'`); n != 1 {
		t.Fatalf("the BLOB stands untouched in v10: %d", n)
	}
	if n := inspect(t, path, `SELECT COUNT(*) FROM approval_tombstones WHERE typeof(policy_version) = 'integer' AND policy_version = 3`); n != 0 {
		t.Fatalf("the normalization G4 forbids: an integer 3 landed: %d", n)
	}
}

// 'abc' of v10 origin: the class is text, named at the copy door.
func TestMigrationV11_textPolicyVersionInV10IsFault(t *testing.T) {
	t.Parallel()
	path, seed := buildV10File(t)
	db := rawDB(t, path)
	if _, err := db.Exec(`UPDATE approval_tombstones SET policy_version = 'abc' WHERE action_id = ?`, seed.ActionID); err != nil {
		t.Fatalf("auditor's UPDATE: %v", err)
	}
	_ = db.Close()
	_, err := Open(path)
	mustFault(t, "AUDIT R13-G4 v10 text", err, "policy_version", "storage class text", true)
	if v := inspect(t, path, `SELECT version FROM action_schema`); v != 10 {
		t.Fatalf("v10 stands: %d", v)
	}
}

// NULL policy_version of v10 origin (the v10 table rebuilt without NOT
// NULL): the judge names "NULL" BEFORE the v11 INSERT's NOT NULL could
// refuse it. (Mutation m-n2e at this site.)
func TestMigrationV11_nullPolicyVersionInV10IsFault(t *testing.T) {
	t.Parallel()
	path, seed := buildV10File(t)
	rebuildV10TombstonesNullable(t, path)
	db := rawDB(t, path)
	if _, err := db.Exec(`UPDATE approval_tombstones SET policy_version = NULL WHERE action_id = ?`, seed.ActionID); err != nil {
		t.Fatalf("auditor's UPDATE: %v", err)
	}
	_ = db.Close()
	_, err := Open(path)
	mustFault(t, "AUDIT R13-G1 v10 NULL", err, "policy_version", "NULL", true)
	if v := inspect(t, path, `SELECT version FROM action_schema`); v != 10 {
		t.Fatalf("v10 stands: %d", v)
	}
}

// ---- G1/G4 at the readers (an open store, the table rebuilt) ----

// seedDatedRow seeds one coherent, dated story into an open store.
func seedDatedRow(t *testing.T, store *Store, aprID, actID string) action.Approval {
	t.Helper()
	a := actionpkgApproval(aprID, actID)
	a.DecisionAt = r13At
	seedTombstoneRow(t, store, [10]any{a.ApprovalID, a.Digest(), a.ActionID, a.ActionDigest, a.PreviewDigest,
		a.PolicyVersion, a.PolicyDigest, a.DecisionPrincipalID, a.Decision, a.DecisionAt.Format(time.RFC3339Nano)})
	return a
}

func mustCorruptNamed(t *testing.T, label string, err error, field, detail string) {
	t.Helper()
	mustFault(t, label, err, field, detail, true)
	if !strings.Contains(err.Error(), "tombstone_corrupt") || !strings.Contains(err.Error(), "tombstone-manual-repair.md") {
		t.Fatalf("%s: stored bytes are corruption, named with the procedure: %v", label, err)
	}
}

// A3 — by action with approval_digest NULL (reachable through action_id;
// the by-action reader is an EXPORTED path with no production caller
// at 49746b3, declared; with a reused action_id its ORDER BY
// decision_at DESC LIMIT 1 hides any corrupt row collating below a
// clean dated one — the row here is alone). (Mutation m-n1.)
func TestTombstoneReader_nullDigestByActionIsFaultNamed(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	a := seedDatedRow(t, store, "apr_r13_a3dig0000000000000000001", "act_r13_a3dig")
	rebuildTombstonesNullable(t, store.db, true)
	if _, err := store.db.Exec(`UPDATE approval_tombstones SET approval_digest = NULL WHERE approval_id = ?`, a.ApprovalID); err != nil {
		t.Fatalf("auditor's UPDATE: %v", err)
	}
	_, _, err := store.ApprovalTombstone(context.Background(), a.ActionID)
	mustCorruptNamed(t, "AUDIT R13-A3 by action", err, "approval_digest", "NULL")
}

// A3 — by digest with action_id NULL (a NULL digest is unreachable by
// digest — G4's confessed absence). (Mutation m-n2e on action_id.)
func TestTombstoneReader_nullActionIDByDigestIsFaultNamed(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	a := seedDatedRow(t, store, "apr_r13_a3act0000000000000000001", "act_r13_a3act")
	rebuildTombstonesNullable(t, store.db, true)
	if _, err := store.db.Exec(`UPDATE approval_tombstones SET action_id = NULL WHERE approval_id = ?`, a.ApprovalID); err != nil {
		t.Fatalf("auditor's UPDATE: %v", err)
	}
	_, _, err := store.ApprovalTombstoneByDigest(context.Background(), a.Digest())
	mustCorruptNamed(t, "AUDIT R13-A3 by digest", err, "action_id", "NULL")
}

// policy_version NULL at BOTH readers.
func TestTombstoneReader_nullPolicyVersionIsFaultNamed(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	a := seedDatedRow(t, store, "apr_r13_a3pv00000000000000000001", "act_r13_a3pv")
	rebuildTombstonesNullable(t, store.db, true)
	if _, err := store.db.Exec(`UPDATE approval_tombstones SET policy_version = NULL WHERE approval_id = ?`, a.ApprovalID); err != nil {
		t.Fatalf("auditor's UPDATE: %v", err)
	}
	_, _, err := store.ApprovalTombstone(context.Background(), a.ActionID)
	mustCorruptNamed(t, "AUDIT R13-A3 policy_version by action", err, "policy_version", "NULL")
	_, _, err = store.ApprovalTombstoneByDigest(context.Background(), a.Digest())
	mustCorruptNamed(t, "AUDIT R13-A3 policy_version by digest", err, "policy_version", "NULL")
}

// N2 combined case — a verb outside the vocabulary AND a NULL
// principal: the by-columns rule names the principal's NULL first
// (column 8 precedes column 9); with ” instead of NULL the verb wins
// at step 4. Both shapes, each its own exact outcome.
func TestTombstoneReader_bogusVerbAndPrincipalPrecedence(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	a := actionpkgApproval("apr_r13_n2comb000000000000000001", "act_r13_n2comb")
	a.Decision, a.DecisionAt = "bogus", r13At
	seedTombstoneRow(t, store, [10]any{a.ApprovalID, a.Digest(), a.ActionID, a.ActionDigest, a.PreviewDigest,
		a.PolicyVersion, a.PolicyDigest, "", a.Decision, a.DecisionAt.Format(time.RFC3339Nano)})
	_, _, err := store.ApprovalTombstone(context.Background(), a.ActionID)
	mustCorruptNamed(t, "AUDIT R13-N2 bogus + '' principal", err, "decision", `verb "bogus" is outside the vocabulary (the human verbs or the clock)`)
	rebuildTombstonesNullable(t, store.db, true)
	if _, err := store.db.Exec(`UPDATE approval_tombstones SET decision_principal_id = NULL WHERE approval_id = ?`, a.ApprovalID); err != nil {
		t.Fatalf("auditor's UPDATE: %v", err)
	}
	_, _, err = store.ApprovalTombstone(context.Background(), a.ActionID)
	mustCorruptNamed(t, "AUDIT R13-N2 bogus + NULL principal", err, "decision_principal_id", "NULL")
}

// clock + NULL principal is a fault even on a clock row (the product
// writes ”, never NULL).
func TestTombstoneReader_clockWithNullPrincipalIsFaultNamed(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	a := actionpkgApproval("apr_r13_clocknull000000000000001", "act_r13_clocknull")
	a.Decision, a.DecisionPrincipalID, a.DecisionAt = action.DecisionClock, "", r13At
	seedTombstoneRow(t, store, [10]any{a.ApprovalID, a.Digest(), a.ActionID, a.ActionDigest, a.PreviewDigest,
		a.PolicyVersion, a.PolicyDigest, "", a.Decision, a.DecisionAt.Format(time.RFC3339Nano)})
	rebuildTombstonesNullable(t, store.db, true)
	if _, err := store.db.Exec(`UPDATE approval_tombstones SET decision_principal_id = NULL WHERE approval_id = ?`, a.ApprovalID); err != nil {
		t.Fatalf("auditor's UPDATE: %v", err)
	}
	_, _, err := store.ApprovalTombstone(context.Background(), a.ActionID)
	mustCorruptNamed(t, "AUDIT R13 clock + NULL principal", err, "decision_principal_id", "NULL")
}

// X'33' in policy_version at BOTH readers: the class, named.
func TestTombstoneReader_blobPolicyVersionIsFaultNamed(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	a := seedDatedRow(t, store, "apr_r13_rpvblob00000000000000001", "act_r13_rpvblob")
	if _, err := store.db.Exec(`UPDATE approval_tombstones SET policy_version = X'33' WHERE approval_id = ?`, a.ApprovalID); err != nil {
		t.Fatalf("auditor's UPDATE: %v", err)
	}
	_, _, err := store.ApprovalTombstone(context.Background(), a.ActionID)
	mustCorruptNamed(t, "AUDIT R13-G4 reader by action", err, "policy_version", "storage class blob")
	_, _, err = store.ApprovalTombstoneByDigest(context.Background(), a.Digest())
	mustCorruptNamed(t, "AUDIT R13-G4 reader by digest", err, "policy_version", "storage class blob")
}

// decision_at as a BLOB of the sealed instant's RFC3339Nano bytes, the
// row ALONE in its table: unmutated, step 1b names the class; under
// m-n4c's 'text' lie at the by-action SELECT the bytes parse and the
// digest re-derives, so the row PASSES — red by no-fault, declared.
func TestTombstoneReader_blobDecisionAtByActionIsFault(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	a := seedDatedRow(t, store, "apr_r13_atblob000000000000000001", "act_r13_atblob")
	if _, err := store.db.Exec(`UPDATE approval_tombstones SET decision_at = CAST(decision_at AS BLOB) WHERE approval_id = ?`, a.ApprovalID); err != nil {
		t.Fatalf("auditor's UPDATE: %v", err)
	}
	_, _, err := store.ApprovalTombstone(context.Background(), a.ActionID)
	mustCorruptNamed(t, "AUDIT R13-G4 decision_at blob", err, "decision_at", "storage class blob")
}

// decision as a BLOB spelling "rejected" at the by-digest reader.
func TestTombstoneReader_blobDecisionByDigestIsFault(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	a := seedDatedRow(t, store, "apr_r13_decblob00000000000000001", "act_r13_decblob")
	if _, err := store.db.Exec(`UPDATE approval_tombstones SET decision = CAST(decision AS BLOB) WHERE approval_id = ?`, a.ApprovalID); err != nil {
		t.Fatalf("auditor's UPDATE: %v", err)
	}
	_, _, err := store.ApprovalTombstoneByDigest(context.Background(), a.Digest())
	mustCorruptNamed(t, "AUDIT R13-G4 decision blob", err, "decision", "storage class blob")
}

// approval_id as a BLOB at the NON-lookup readers (by action, by
// digest): the key column's class, named at the column.
func TestTombstoneReader_blobKeyAtNonLookupReadersIsFault(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	a := seedDatedRow(t, store, "apr_r13_keyblob00000000000000001", "act_r13_keyblob")
	if _, err := store.db.Exec(`UPDATE approval_tombstones SET approval_id = CAST(approval_id AS BLOB) WHERE approval_id = ?`, a.ApprovalID); err != nil {
		t.Fatalf("auditor's UPDATE: %v", err)
	}
	_, _, err := store.ApprovalTombstone(context.Background(), a.ActionID)
	mustCorruptNamed(t, "AUDIT R13-G4 key blob by action", err, "approval_id", "storage class blob")
	_, _, err = store.ApprovalTombstoneByDigest(context.Background(), a.Digest())
	mustCorruptNamed(t, "AUDIT R13-G4 key blob by digest", err, "approval_id", "storage class blob")
}

// G4's CONFESSED absence, asserted AS absence (A10b/A10c): the column a
// reader looks up BY is not judged by that reader — a BLOB there is
// INDISTINGUISHABLE FROM ABSENCE (ErrNotFound), never a fault. The
// lookups stay indexed; the remedy (a full-table scan verifier) is
// filed to v0.15.1.
func TestTombstoneReader_blobLookupColumnIsIndistinguishableFromAbsence(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	a := seedDatedRow(t, store, "apr_r13_lookup000000000000000001", "act_r13_lookup")
	if _, err := store.db.Exec(`UPDATE approval_tombstones SET action_id = CAST(action_id AS BLOB), approval_digest = CAST(approval_digest AS BLOB) WHERE approval_id = ?`, a.ApprovalID); err != nil {
		t.Fatalf("auditor's UPDATE: %v", err)
	}
	if _, _, err := store.ApprovalTombstone(context.Background(), a.ActionID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("AUDIT R13-A10b: a BLOB in the by-action lookup column IS absence for that reader, declared: %v", err)
	}
	if _, _, err := store.ApprovalTombstoneByDigest(context.Background(), a.Digest()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("AUDIT R13-A10c: a BLOB in the by-digest lookup column IS absence for that reader, declared: %v", err)
	}
}

// ---- the collision arm: a PHANTOM collision (the diff pass, P2-1) ----

// The INSERT reports a UNIQUE collision — forged here by a trigger that
// RAISEs the exact text — yet no row by this id and no row by this
// digest exists. The arm names index corruption as a TYPED, Stored
// fault at approval_digest with its exact Detail; it never carries the
// identity of absence (errors.Is sql.ErrNoRows is false), and it writes
// nothing (the count inside the tx is unchanged). (Mutation m-phantom:
// the arm returns nil — a silent no-op on a lying index — red.)
func TestTombstoneTx_phantomCollisionIsNamedAsIndexCorruption(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	a := actionpkgApproval("apr_r13_phantom0000000000000001", "act_r13_phantom")
	a.DecisionAt = r13At
	if _, err := store.db.Exec(`CREATE TRIGGER forge_unique BEFORE INSERT ON approval_tombstones
	    BEGIN SELECT RAISE(ABORT, 'UNIQUE constraint failed: forged by the auditor'); END;`); err != nil {
		t.Fatalf("auditor's trigger: %v", err)
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	err = store.tombstoneTx(ctx, tx, a, a.DecisionPrincipalID, a.Decision, a.DecisionAt)
	mustFault(t, "AUDIT R13 phantom collision", err, "approval_digest",
		"index corruption: phantom collision — the INSERT reported a UNIQUE collision, yet no row carries this approval id or this digest", true)
	if errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("index corruption must never carry the identity of absence: %v", err)
	}
	// The INSERT's own error rides as the Cause — the constraint that
	// fired, or here the forged text — the primary fact the adjudication
	// needs (never discarded).
	if !strings.Contains(err.Error(), "forged by the auditor") {
		t.Fatalf("the INSERT's error must ride out as the fault's cause: %v", err)
	}
	if !strings.Contains(err.Error(), "tombstone-manual-repair.md") {
		t.Fatalf("a human adjudicates: the repair pointer rides out: %v", err)
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM approval_tombstones`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("nothing written: n=%d err=%v", n, err)
	}
}

// ---- the funnel's idempotence read ----

func idempotenceFault(t *testing.T, store *Store, a action.Approval) error {
	t.Helper()
	tx, err := store.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	return store.tombstoneTx(context.Background(), tx, a, a.DecisionPrincipalID, a.Decision, a.DecisionAt)
}

// X'33' on the EXISTING row: the re-insert collides, the existing row
// is judged by class. (Under m-n4c's 'integer' lie at the idempotence
// SELECT the re-insert becomes a harmless no-op — "no error" where the
// fault is expected.)
func TestTombstoneIdempotence_blobPolicyVersionExistingRowIsFaultTyped(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	a := seedDatedRow(t, store, "apr_r13_idblob000000000000000001", "act_r13_idblob")
	if _, err := store.db.Exec(`UPDATE approval_tombstones SET policy_version = X'33' WHERE approval_id = ?`, a.ApprovalID); err != nil {
		t.Fatalf("auditor's UPDATE: %v", err)
	}
	err := idempotenceFault(t, store, a)
	mustCorruptNamed(t, "AUDIT R13-G4 idempotence blob", err, "policy_version", "storage class blob")
}

// NULL policy_version on the EXISTING row (the funnel's idempotence
// read is one of A3's readers).
func TestTombstoneIdempotence_nullPolicyVersionExistingRowIsFaultNamed(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	a := seedDatedRow(t, store, "apr_r13_idnull000000000000000001", "act_r13_idnull")
	rebuildTombstonesNullable(t, store.db, true)
	if _, err := store.db.Exec(`UPDATE approval_tombstones SET policy_version = NULL WHERE approval_id = ?`, a.ApprovalID); err != nil {
		t.Fatalf("auditor's UPDATE: %v", err)
	}
	// The rebuilt table has no UNIQUE on the digest: the collision the
	// funnel needs comes from the PRIMARY KEY on approval_id, kept.
	err := idempotenceFault(t, store, a)
	mustCorruptNamed(t, "AUDIT R13-A3 idempotence NULL", err, "policy_version", "NULL")
}

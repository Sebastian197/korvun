// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package sqlite persists the Action Kernel's attempts and decisions
// (Trust Layer Etapa 1, spec FR-STORE, sealed 2026-08-30). It follows the
// house SQLite mold (WAL DSN, single serialized writer, boot-fatal Open,
// idempotent bootstrap) and — sealed decision 1 — SHARES the database
// file with the conversation store while owning its OWN schema lifecycle:
// the `action_schema` version table below never mixes with the
// conversation migrations, and vice versa. The store opens its own
// connection pool on the shared file; WAL plus the busy timeout make the
// two single-writer pools coexist safely.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Sebastian197/korvun/internal/action"

	_ "modernc.org/sqlite" // the pure-Go driver the house already ships
)

// Decision is the persisted authorization outcome of one attempt: the
// gate's verdict and the rule that produced it (finite grammar, no free
// text from the model).
type Decision struct {
	// Outcome is the gate verdict: "allow", "deny" or "shadow".
	Outcome string
	// Rule names the deciding dimension (the ADR-0041 audit grammar).
	Rule string
	// PolicyVersion / PolicyDigest pin the EXACT law that took the
	// decision (Etapa 3, FR-POL-1): version is monotonic per loaded
	// config, digest is canonical over the brain's effective governance +
	// the effect-registry snapshot. Zero values = the pre-pin era.
	PolicyVersion int64
	PolicyDigest  string
}

// PolicyPin is the law identity one adapter stamps on every decision.
type PolicyPin struct {
	Version int64
	Digest  string
}

// Record is one stored action with its decision and lifecycle facts.
type Record struct {
	// Envelope is the ActionEnvelope v1 as persisted (round-trips verbatim).
	Envelope action.Envelope
	// State is the action's current state.
	State action.State
	// Decision is the persisted gate verdict.
	Decision Decision
	// RecoveryMarker is "" normally and "crash_recovered" when a previous
	// life's non-terminal action was closed by the recovery pass.
	RecoveryMarker string
	// FinishedAt is nil while the action is not terminal.
	FinishedAt *time.Time
	// Identity is nil for identity-less (v1-path) rows and carries the
	// principal/intent/authority refs for identified rows.
	Identity *StoredIdentity
}

// ErrNotFound reports an unknown action_id.
var ErrNotFound = errors.New("action/sqlite: action not found")

// ErrNotADecisionState reports a RecordAttempt with a state that is not a
// decision outcome (only DENIED, SHADOWED and AUTHORIZED enter the store).
var ErrNotADecisionState = errors.New("action/sqlite: not a decision state")

// ErrSchemaFromTheFuture reports a stored schema version newer than this
// binary understands: fail closed, never guess at unknown structure.
var ErrSchemaFromTheFuture = errors.New("action/sqlite: schema version from the future")

// dsnQuery mirrors the house connection pragmas (the conversation store's
// mold): WAL for multi-pool robustness on the SHARED file, busy_timeout as
// the cross-pool safety net, foreign_keys on because the decision row
// references its action row.
const dsnQuery = "_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)"

// createStmt is the store's v1 schema. CREATE TABLE IF NOT EXISTS keeps
// the bootstrap idempotent; `action_schema` is this store's OWN lifecycle
// marker, deliberately separate from every conversation table.
const createStmt = `
CREATE TABLE IF NOT EXISTS action_schema (
    version INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS actions (
    action_id         TEXT    NOT NULL PRIMARY KEY,
    schema_version    INTEGER NOT NULL,
    correlation_id    TEXT    NOT NULL,
    source_kind       TEXT    NOT NULL,
    source_protocol   TEXT    NOT NULL,
    source_channel    TEXT    NOT NULL,
    op_namespace      TEXT    NOT NULL,
    op_name           TEXT    NOT NULL,
    op_version        INTEGER NOT NULL,
    parameters_digest TEXT    NOT NULL,
    effect_class      TEXT    NOT NULL,
    state             TEXT    NOT NULL,
    recovery_marker   TEXT    NOT NULL DEFAULT '',
    requested_at      TEXT    NOT NULL,
    finished_at       TEXT
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS actions_by_correlation ON actions(correlation_id);
CREATE INDEX IF NOT EXISTS actions_by_requested ON actions(requested_at);
CREATE TABLE IF NOT EXISTS action_decisions (
    action_id  TEXT NOT NULL PRIMARY KEY REFERENCES actions(action_id) ON DELETE CASCADE,
    outcome    TEXT NOT NULL,
    rule       TEXT NOT NULL,
    decided_at TEXT NOT NULL
) WITHOUT ROWID;`

// schemaVersionCurrent is the version this binary writes and understands.
const schemaVersionCurrent = 12

// migrations maps a FROM-version to the DDL that lifts it one version.
// Each step runs in ONE transaction together with its version bump, so a
// crash mid-migration rolls back to a clean previous version — never a
// zombie schema (AS-8). v1→v2 (Trust Layer Etapa 2): additive identity
// columns on actions (the Etapa-1 RESERVED fields wake up on disk;
// existing rows keep NULL identity and stay readable), plus the intents,
// grants, evidence and budget_spent tables under this store's OWN
// lifecycle.
//
// Bounded growth (FR-ENV-1, reasoned not assumed): evidence rows are
// tied to their action via ON DELETE CASCADE, so the Etapa-1 retention
// cap prunes them with their actions; intents and grants are
// OPERATOR-SCALE — a human creates them deliberately (tens, not
// millions), so they carry no cap; budget_spent is bounded by
// contracts × operation names, the same operator scale.
var migrations = map[int]string{
	1: `
ALTER TABLE actions ADD COLUMN principal_id TEXT;
ALTER TABLE actions ADD COLUMN intent_id TEXT;
ALTER TABLE actions ADD COLUMN authority_refs TEXT;
CREATE TABLE intents (
    intent_id          TEXT    NOT NULL PRIMARY KEY,
    schema_version     INTEGER NOT NULL,
    owner_principal_id TEXT    NOT NULL,
    purpose            TEXT    NOT NULL,
    operations         TEXT    NOT NULL,
    resources          TEXT    NOT NULL,
    max_actions        INTEGER NOT NULL,
    per_operation      TEXT    NOT NULL,
    valid_from         TEXT    NOT NULL,
    expires_at         TEXT,
    status             TEXT    NOT NULL,
    version            INTEGER NOT NULL,
    digest             TEXT    NOT NULL
) WITHOUT ROWID;
CREATE TABLE grants (
    grant_id             TEXT    NOT NULL PRIMARY KEY,
    intent_id            TEXT    NOT NULL REFERENCES intents(intent_id),
    issuer_principal_id  TEXT    NOT NULL,
    subject_principal_id TEXT    NOT NULL,
    parent_grant_id      TEXT    NOT NULL DEFAULT '',
    operations           TEXT    NOT NULL,
    resources            TEXT    NOT NULL,
    max_actions          INTEGER NOT NULL,
    per_operation        TEXT    NOT NULL,
    valid_from           TEXT    NOT NULL,
    expires_at           TEXT,
    status               TEXT    NOT NULL,
    depth_remaining      INTEGER NOT NULL,
    digest               TEXT    NOT NULL
) WITHOUT ROWID;
CREATE TABLE evidence (
    evidence_id       TEXT NOT NULL PRIMARY KEY,
    action_id         TEXT NOT NULL REFERENCES actions(action_id) ON DELETE CASCADE,
    provider          TEXT NOT NULL,
    subject           TEXT NOT NULL,
    credential        TEXT NOT NULL,
    issued_at         TEXT NOT NULL,
    transport_binding TEXT NOT NULL,
    claims_digest     TEXT NOT NULL
) WITHOUT ROWID;
CREATE INDEX evidence_by_action ON evidence(action_id);
CREATE TABLE budget_spent (
    scope_id  TEXT    NOT NULL,
    operation TEXT    NOT NULL,
    spent     INTEGER NOT NULL,
    PRIMARY KEY (scope_id, operation)
) WITHOUT ROWID;`,
	// v2→v3 (Trust Layer Etapa 3, FR-CEIL-1): the grants table gains the
	// additive effect_ceiling column — NULL/empty = no ceiling, so every
	// pre-E3 grant reads back UNLIMITED and nothing moves. Same
	// anti-zombie discipline: this DDL and its version bump commit in one
	// transaction (the AS-8 crash mold, re-armed by test for v3).
	2: `
ALTER TABLE grants ADD COLUMN effect_ceiling TEXT NOT NULL DEFAULT '';`,
	// v3→v4 (Trust Layer Etapa 3, FR-POL-1): every gate decision pins the
	// EXACT law that took it. Additive columns; pre-pin rows read back
	// version 0 / empty digest — the honest "no pinned law" of their era.
	3: `
ALTER TABLE action_decisions ADD COLUMN policy_version INTEGER NOT NULL DEFAULT 0;
ALTER TABLE action_decisions ADD COLUMN policy_digest TEXT NOT NULL DEFAULT '';`,
	// v4→v5 (Trust Layer Etapa 4, FR-KEY): the signing_keys table — the
	// ledger's ink registry. Retired keys are KEPT FOREVER (no delete
	// path exists on this table); at most one row has retired_at NULL
	// (the active key). Receipts arrive with v6 (batch 3).
	4: `
CREATE TABLE signing_keys (
    key_id     TEXT NOT NULL PRIMARY KEY,
    public_key TEXT NOT NULL,
    created_at TEXT NOT NULL,
    retired_at TEXT
) WITHOUT ROWID;`,
	// v5→v6 (Trust Layer Etapa 4, FR-LED-1): the receipts ledger. NO
	// foreign key to actions ON PURPOSE — the sealed exemption: the E1
	// prune takes operational action rows while the evidence stays. The
	// UNIQUE (partition, chain_seq) pair is the chain's belt.
	5: `
CREATE TABLE receipts (
    receipt_id            TEXT    NOT NULL PRIMARY KEY,
    action_id             TEXT    NOT NULL,
    intent_digest         TEXT    NOT NULL,
    principal_id          TEXT    NOT NULL,
    authority_digest      TEXT    NOT NULL DEFAULT '',
    decision_digest       TEXT    NOT NULL,
    action_digest         TEXT    NOT NULL,
    effect_class          TEXT    NOT NULL,
    attempt               INTEGER NOT NULL,
    outcome               TEXT    NOT NULL,
    result_digest         TEXT    NOT NULL DEFAULT '',
    started_at            TEXT,
    finished_at           TEXT,
    partition             TEXT    NOT NULL,
    chain_seq             INTEGER NOT NULL,
    previous_receipt_hash TEXT    NOT NULL,
    receipt_hash          TEXT    NOT NULL,
    signing_key_id        TEXT    NOT NULL,
    signature             TEXT    NOT NULL,
    UNIQUE (partition, chain_seq)
) WITHOUT ROWID;
CREATE INDEX receipts_by_action ON receipts(action_id);`,

	// v6 -> v7 (Trust Layer Etapa 5, spec FR-APR/FR-PRV): the approvals
	// table — the §10.8 request with its sealed preview AND the parked
	// action's canonical parameters. The E1 no-raw law holds for resting
	// history: a parked request IS pending work, its params are the very
	// object under approval, and they are PURGED at any close without
	// execution. UNIQUE(action_id): one request per parked action.
	6: `
CREATE TABLE approvals (
    approval_id           TEXT    NOT NULL PRIMARY KEY,
    schema_version        INTEGER NOT NULL,
    action_id             TEXT    NOT NULL UNIQUE,
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
CREATE INDEX approvals_by_status ON approvals(status);`,

	// v7 -> v8 (Trust Layer Etapa 5, sealed NC-3α): the receipts gain
	// their era column and the approval reference INSIDE the sealed
	// form. Historical rows default to schema_version 1 — the frozen v1
	// era whose bytes verify forever; the live code writes 2.
	7: `
ALTER TABLE receipts ADD COLUMN schema_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE receipts ADD COLUMN approval_digest TEXT NOT NULL DEFAULT '';`,
	// v8 -> v9 (R4 Phase 4, ADR-0046): approvals.action_id becomes a
	// REAL foreign key with ON DELETE CASCADE — bounded retention: when
	// the prune removes a terminal action its approval cascades, the
	// SIGNED receipt being the surviving evidence. SQLite cannot add a
	// constraint in place, so the table is RECONSTRUCTED transactionally
	// (the runner wraps this whole step + the version bump in one
	// transaction — the AS-8 anti-zombie mold, crash-rehearsed). Orphan
	// approvals predating v9 (action pruned under the old exemption)
	// are RETIRED by the copy filter, declared in the ADR: their
	// receipt is their evidence, exactly as if the cascade had run.
	8: `
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
INSERT INTO approvals_v9 SELECT * FROM approvals
 WHERE EXISTS (SELECT 1 FROM actions WHERE actions.action_id = approvals.action_id);
DROP TABLE approvals;
ALTER TABLE approvals_v9 RENAME TO approvals;
CREATE INDEX approvals_by_status ON approvals(status);`,
	// v9 -> v10 (R6-X2): the approval TOMBSTONE — the bounded SCALAR
	// preimage of Approval.Digest() (who, what decision, when, under
	// which law, which preview), written in the same transaction as
	// every decided close. NO foreign key ON PURPOSE: like receipts, a
	// tombstone is EVIDENCE and survives the retention cascade
	// (exemption declared; fixed width, no bodies — growth is one
	// bounded row per decided approval).
	9: `
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
) WITHOUT ROWID;`,
	// v10 -> v11 (R7-Y2): the tombstone gains its OWN identity — PK is
	// the APPROVAL id (one tombstone per decided approval, immutable),
	// approval_digest is UNIQUE, and action_id becomes an indexed
	// column: an action_id REUSED after the prune never overwrites the
	// old approval's history. Transactional reconstruction (the v9
	// mold). R8-Z1 REVOKED the retire adjudication: a migration NEVER
	// destroys tombstones — the rows are COPIED by in-transaction Go
	// (copyTombstonesV10toV11 computes each Approval.Digest() from the
	// scalar preimage), and the old table drops only after every row
	// has landed (migrationsPost). Zero rows lost, crash-rehearsed.
	10: `
CREATE TABLE approval_tombstones_v11 (
    approval_id           TEXT    NOT NULL PRIMARY KEY,
    approval_digest       TEXT    NOT NULL UNIQUE,
    action_id             TEXT    NOT NULL,
    action_digest         TEXT    NOT NULL,
    preview_digest        TEXT    NOT NULL,
    policy_version        INTEGER NOT NULL,
    policy_digest         TEXT    NOT NULL,
    decision_principal_id TEXT    NOT NULL,
    decision              TEXT    NOT NULL,
    decision_at           TEXT
) WITHOUT ROWID;`,
	// v11→v12 (R11): a RE-VALIDATION migration — no DDL, no writes.
	// The legacy-v11 window (rows that entered before the R9/R10
	// walls) is re-judged by the one typed contract in
	// revalidateTombstonesV11toV12; the step below is a no-op so the
	// hybrid runner keeps its one-transaction shape.
	11: `SELECT 1;`,
}

// migrationsPost holds the destructive tail of a hybrid step (R8-Z1):
// it runs AFTER the in-transaction Go copy, same transaction — the
// old table is dropped only once every row has landed in the new one.
var migrationsPost = map[int]string{
	10: `
DROP TABLE approval_tombstones;
ALTER TABLE approval_tombstones_v11 RENAME TO approval_tombstones;
CREATE INDEX tombstones_by_action ON approval_tombstones(action_id);`,
}

// migrate lifts the store to schemaVersionCurrent, one version per
// transaction (DDL + version bump commit atomically). A version newer
// than this binary fails CLOSED with ErrSchemaFromTheFuture — the store
// never guesses at structure it does not understand.
func migrate(db *sql.DB) error {
	for {
		var v int
		if err := db.QueryRow(`SELECT version FROM action_schema`).Scan(&v); err != nil {
			return fmt.Errorf("action/sqlite: read schema version for migration: %w", err)
		}
		if v == schemaVersionCurrent {
			return nil
		}
		if v > schemaVersionCurrent {
			return fmt.Errorf("%w: stored %d, this binary understands %d",
				ErrSchemaFromTheFuture, v, schemaVersionCurrent)
		}
		step, ok := migrations[v]
		if !ok {
			return fmt.Errorf("action/sqlite: no migration from schema version %d", v)
		}
		if err := migrateStep(db, step, migrationCopies[v], v); err != nil {
			return err
		}
	}
}

// migrationCopies holds the R8-Z1 in-transaction Go copies: a step
// whose data transform cannot be expressed in SQL (computing a
// canonical digest) runs its copy INSIDE the same transaction as its
// DDL and version bump — the AS-8 anti-zombie discipline unchanged.
// A migration NEVER destroys evidence: it copies or it fails closed.
var migrationCopies = map[int]func(*sql.Tx) error{
	10: copyTombstonesV10toV11,
	11: revalidateTombstonesV11toV12,
}

// TombstoneFault names one unreadable or incoherent tombstone row —
// the typed shape the operational contract fails with, carrying the
// row and field a human needs to adjudicate.
type TombstoneFault struct {
	ApprovalID string
	Field      string
	Detail     string
	Cause      error
}

// Error names the fault class and its coordinates.
func (f *TombstoneFault) Error() string {
	msg := fmt.Sprintf("tombstone_corrupt: tombstone %q: %s", f.ApprovalID, f.Field)
	if f.Detail != "" {
		msg += " (" + f.Detail + ")"
	}
	if f.Cause != nil {
		msg += ": " + f.Cause.Error()
	}
	return msg + " — corrupt evidence demands human adjudication; see docs/operations/tombstone-manual-repair.md"
}

// Unwrap exposes the cause.
func (f *TombstoneFault) Unwrap() error { return f.Cause }

// rawTombstone is one approval_tombstones row read AS STORED — every
// column as raw text, so THE contract (judgeStoredTombstone) is the
// only judge of emptiness, types, dates and digest coherence. The
// migration and the readers share this one path (R12: X2+X6 unified).
type rawTombstone struct {
	approvalID    sql.NullString
	digest        sql.NullString
	actionID      sql.NullString
	actionDigest  sql.NullString
	previewDigest sql.NullString
	policyVersion sql.NullString
	policyDigest  sql.NullString
	principal     sql.NullString
	decision      sql.NullString
	decisionAt    sql.NullString
}

// judgeStoredTombstone is THE one operational contract over a stored
// tombstone row (R11, hardened by R12 with the DOMAIN's truth): the
// evidence columns non-empty; decision_at NULL allowed (” and
// unreadable bytes rejected); policy_version an integer; the stored
// digest re-derived from the preimage; and the origin rule — a
// system decision (decision "clock", written only by the expiry
// touch and the sweep, approvals.go:210/:577) legitimately carries an
// EMPTY principal, while a human verb demands one and a "clock" row
// carrying a principal is an anomaly, both ways. It returns the
// parsed preimage, whether decision_at is present, or the typed
// fault naming row and STABLE column.
func judgeStoredTombstone(r rawTombstone) (action.Approval, bool, *TombstoneFault) {
	text := func(v sql.NullString) string {
		if v.Valid {
			return v.String
		}
		return ""
	}
	var a action.Approval
	a.ApprovalID = text(r.approvalID)
	a.ActionID = text(r.actionID)
	a.ActionDigest = text(r.actionDigest)
	a.PreviewDigest = text(r.previewDigest)
	a.PolicyDigest = text(r.policyDigest)
	a.DecisionPrincipalID = text(r.principal)
	a.Decision = text(r.decision)
	for _, f := range []struct{ field, value string }{
		{"approval_id", a.ApprovalID},
		{"action_id", a.ActionID},
		{"action_digest", a.ActionDigest},
		{"preview_digest", a.PreviewDigest},
		{"policy_digest", a.PolicyDigest},
		{"decision", a.Decision},
	} {
		if f.value == "" {
			return action.Approval{}, false, &TombstoneFault{ApprovalID: a.ApprovalID, Field: f.field, Detail: "empty " + f.field}
		}
	}
	// The origin rule (R12-X1): the domain's system decisions are
	// decision="clock" with an empty principal — exactly that pair.
	if a.Decision == "clock" {
		if a.DecisionPrincipalID != "" {
			return action.Approval{}, false, &TombstoneFault{ApprovalID: a.ApprovalID, Field: "decision_principal_id", Detail: "a clock decision carrying a principal — a human hand signing as the clock"}
		}
	} else if a.DecisionPrincipalID == "" {
		return action.Approval{}, false, &TombstoneFault{ApprovalID: a.ApprovalID, Field: "decision_principal_id", Detail: "empty decision_principal_id on a human verb"}
	}
	pv, perr := strconv.ParseInt(text(r.policyVersion), 10, 64)
	if perr != nil {
		return action.Approval{}, false, &TombstoneFault{ApprovalID: a.ApprovalID, Field: "policy_version", Detail: fmt.Sprintf("non-integer bytes %q", text(r.policyVersion)), Cause: perr}
	}
	a.PolicyVersion = pv
	present := false
	if r.decisionAt.Valid {
		if r.decisionAt.String == "" {
			return action.Approval{}, false, &TombstoneFault{ApprovalID: a.ApprovalID, Field: "decision_at", Detail: "present-but-empty bytes are empty evidence, not absence"}
		}
		t, terr := time.Parse(time.RFC3339Nano, r.decisionAt.String)
		if terr != nil {
			return action.Approval{}, false, &TombstoneFault{ApprovalID: a.ApprovalID, Field: "decision_at", Detail: fmt.Sprintf("unreadable bytes %q", r.decisionAt.String), Cause: terr}
		}
		a.DecisionAt = t
		present = true
	}
	if got := a.Digest(); got != text(r.digest) {
		return action.Approval{}, false, &TombstoneFault{ApprovalID: a.ApprovalID, Field: "approval_digest", Detail: fmt.Sprintf("stored %q does not re-derive from the preimage (%s)", text(r.digest), got)}
	}
	return a, present, nil
}

// revalidateTombstonesV11toV12 is the R11 re-validation migration,
// R12-hardened: every row is read AS RAW TEXT and judged by THE one
// contract — streaming, deterministic order, all rows before the
// bump, one transaction, ZERO writes, no normalization ever. Its
// guarantee is per-snapshot: the migration established the invariant
// in ITS snapshot; any later audit re-establishes it in its own.
func revalidateTombstonesV11toV12(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT approval_digest, approval_id, action_id, action_digest,
	        preview_digest, CAST(policy_version AS TEXT), policy_digest,
	        decision_principal_id, decision, decision_at
	   FROM approval_tombstones ORDER BY approval_id`)
	if err != nil {
		return fmt.Errorf("action/sqlite: v12 revalidation read: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var r rawTombstone
		if err := rows.Scan(&r.digest, &r.approvalID, &r.actionID, &r.actionDigest,
			&r.previewDigest, &r.policyVersion, &r.policyDigest,
			&r.principal, &r.decision, &r.decisionAt); err != nil {
			return fmt.Errorf("action/sqlite: v12 revalidation scan: %w", err)
		}
		if _, _, fault := judgeStoredTombstone(r); fault != nil {
			return fmt.Errorf("action/sqlite: v12 revalidation: %w", fault)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("action/sqlite: v12 revalidation rows: %w", err)
	}
	return nil
}

// copyTombstonesV10toV11 reads every v10 tombstone (still present in
// the transaction: the copy runs between the v11 CREATE and the v10
// DROP encoded in the DDL split below) and inserts it into v11 with
// its Approval.Digest() computed in Go — ZERO rows lost. Every row is
// validated BEFORE it migrates (R9-W1): unreadable bytes — a
// decision_at that does not parse, an empty evidence column — fail
// the whole migration CLOSED naming row and field, because corrupt
// evidence demands human adjudication, never a silent normalization.
// A NULL decision_at is honest ABSENCE, not corruption, and migrates
// with the zero-time preimage (the Y3 line: corrupt ≠ absent).
// Semantic plausibility (e.g. policy_version 0) is NOT validated —
// the wall guards readability, not meaning.
func copyTombstonesV10toV11(tx *sql.Tx) error {
	// R10-V3: the read order is CONTRACTUAL — the mid-copy crash mold
	// probes "first row in, second row fails", so the order it relies
	// on is written here, never borrowed from a B-tree accident.
	rows, err := tx.Query(`SELECT action_id, approval_id, action_digest, preview_digest,
	        policy_version, policy_digest, decision_principal_id, decision, decision_at
	   FROM approval_tombstones ORDER BY action_id`)
	if err != nil {
		return fmt.Errorf("action/sqlite: v11 copy read: %w", err)
	}
	defer func() { _ = rows.Close() }()
	type v10row struct {
		a          action.Approval
		decisionAt sql.NullString
	}
	var all []v10row
	for rows.Next() {
		var r v10row
		if err := rows.Scan(&r.a.ActionID, &r.a.ApprovalID, &r.a.ActionDigest,
			&r.a.PreviewDigest, &r.a.PolicyVersion, &r.a.PolicyDigest,
			&r.a.DecisionPrincipalID, &r.a.Decision, &r.decisionAt); err != nil {
			return fmt.Errorf("action/sqlite: v11 copy scan: %w", err)
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("action/sqlite: v11 copy rows: %w", err)
	}
	for _, r := range all {
		if err := validateV10Tombstone(r.a, r.decisionAt); err != nil {
			return err
		}
		if r.decisionAt.Valid {
			t, perr := parseNullTime(r.decisionAt)
			if perr != nil {
				// Unreachable after validateV10Tombstone; kept closed.
				return perr
			}
			r.a.DecisionAt = t
		}
		if _, err := tx.Exec(`INSERT INTO approval_tombstones_v11
		    (approval_id, approval_digest, action_id, action_digest, preview_digest,
		     policy_version, policy_digest, decision_principal_id, decision, decision_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			r.a.ApprovalID, r.a.Digest(), r.a.ActionID, r.a.ActionDigest, r.a.PreviewDigest,
			r.a.PolicyVersion, r.a.PolicyDigest, r.a.DecisionPrincipalID, r.a.Decision,
			nullOrString(r.decisionAt)); err != nil {
			return fmt.Errorf("action/sqlite: v11 copy insert %q: %w", r.a.ApprovalID, err)
		}
	}
	return nil
}

// validateV10Tombstone is the R9-W1 wall for the v10→v11 copy — it
// delegates to THE one contract, minus the digest contrast (v10
// stores no digest column; the copy computes it). The domain rule
// rides here too: v10 sweeps wrote ("", "clock") legitimately.
func validateV10Tombstone(a action.Approval, decisionAt sql.NullString) error {
	probe := a
	probe.DecisionAt = time.Time{}
	if decisionAt.Valid && decisionAt.String != "" {
		if t, err := time.Parse(time.RFC3339Nano, decisionAt.String); err == nil {
			probe.DecisionAt = t
		}
	}
	str := func(v string) sql.NullString { return sql.NullString{Valid: true, String: v} }
	raw := rawTombstone{
		approvalID:    str(probe.ApprovalID),
		digest:        str(probe.Digest()),
		actionID:      str(probe.ActionID),
		actionDigest:  str(probe.ActionDigest),
		previewDigest: str(probe.PreviewDigest),
		policyVersion: str(strconv.FormatInt(probe.PolicyVersion, 10)),
		policyDigest:  str(probe.PolicyDigest),
		principal:     str(probe.DecisionPrincipalID),
		decision:      str(probe.Decision),
		decisionAt:    decisionAt,
	}
	if _, _, fault := judgeStoredTombstone(raw); fault != nil {
		return fmt.Errorf("action/sqlite: v11 copy: %w", fault)
	}
	return nil
}

func nullOrString(v sql.NullString) any {
	if v.Valid {
		return v.String
	}
	return nil
}

// migrateStep runs ONE migration — DDL, optional in-tx Go copy, and
// the version bump — in one transaction.
func migrateStep(db *sql.DB, step string, copy func(*sql.Tx) error, from int) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("action/sqlite: begin migration from v%d: %w", from, err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(step); err != nil {
		return fmt.Errorf("action/sqlite: migration from v%d: %w", from, err)
	}
	if copy != nil {
		if err := copy(tx); err != nil {
			return err
		}
	}
	if post, ok := migrationsPost[from]; ok {
		if _, err := tx.Exec(post); err != nil {
			return fmt.Errorf("action/sqlite: migration tail from v%d: %w", from, err)
		}
	}
	if _, err := tx.Exec(`UPDATE action_schema SET version = ?`, from+1); err != nil {
		return fmt.Errorf("action/sqlite: bump schema version to v%d: %w", from+1, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("action/sqlite: commit migration to v%d: %w", from+1, err)
	}
	return nil
}

// buildFileDSN is the house DSN builder (conversation store mold): url.URL
// percent-encodes the path so URI-significant characters cannot swallow
// the pragmas; a leading '/' canonicalizes Windows drive paths.
func buildFileDSN(slashed string) string {
	if len(slashed) == 0 || slashed[0] != '/' {
		slashed = "/" + slashed
	}
	return (&url.URL{Scheme: "file", Path: slashed, RawQuery: dsnQuery}).String()
}

// Store persists actions and decisions. All access flows through one
// serialized connection (the house single-writer discipline), so method
// calls are safe from concurrent brain workers.
type Store struct {
	db *sql.DB
	// capRows and pruneEvery implement the sealed retention decision: a
	// generous automatic cap with NO config surface. Fields (not globals)
	// so tests exercise small caps without mutable package state.
	capRows    int
	pruneEvery int
	// readOnly marks a store opened through OpenReadOnly (the R1 door):
	// the connection is sealed with PRAGMA query_only and no lifecycle
	// pass (migration/recovery/prune) has run.
	readOnly bool
	// sealer, when non-nil, signs and appends one receipt per terminal
	// outcome INSIDE the recording transaction (Etapa 4, FR-LED). The app
	// injects it with the active profile key; nil = pre-stage behavior.
	sealer func(action.Receipt) action.Receipt
	// writes counts RecordAttempt commits toward the periodic prune;
	// mutex-guarded because callers are concurrent brain workers (the DB
	// pool serializes statements, not this counter).
	writesMu sync.Mutex
	writes   int
}

// The sealed retention defaults (decision 2): generous, automatic, no
// config surface. The cap bounds TOTAL rows; pruning only ever removes
// terminal rows, oldest first — live rows are untouchable.
const (
	defaultCapRows    = 100_000
	defaultPruneEvery = 512
	// recoveryMarkerCrash marks actions closed by the Open recovery pass.
	recoveryMarkerCrash = "crash_recovered"
	// recoveryMarkerOutcomeUnknown names the C5 uncertainty: the crash
	// hit AFTER the params claim — the external effect may have fired.
	recoveryMarkerOutcomeUnknown = "outcome_unknown"
)

// openWithCap is the test seam behind Open: same mold, explicit cap.
func openWithCap(path string, capRows int) (*Store, error) {
	store, err := open(path)
	if err != nil {
		return nil, err
	}
	store.capRows = capRows
	if _, err := store.Prune(context.Background()); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

// Open opens (or creates) the action store at path — normally the SAME
// file the conversation store uses (sealed decision 1). It creates the
// parent directory, applies the single-writer pool, bootstraps this
// store's own schema and lifecycle row, pings (boot-fatal posture: a bad
// path or corrupt file fails HERE, never on the first recorded action),
// then runs the HONEST recovery pass — non-terminal actions from a
// previous life close FAILED with the recovery marker, never re-executed
// — and the retention prune.
func Open(path string) (*Store, error) {
	store, err := open(path)
	if err != nil {
		return nil, err
	}
	// R3: the recovery pass no longer runs here — its closes are
	// terminals and no terminal is born without its receipt, so the
	// BOOT calls RecoverPreviousLife AFTER wiring the keystore/sealer.
	if _, err := store.Prune(context.Background()); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

// OpenOperator is the THIRD door (E5 consolidation C4): a WRITING open
// for the operator's own acts (decide, execute, intent/grant writes)
// that touches NOTHING a live server owns — no crash recovery, no
// retention prune, and never a migration of an existing store (those
// belong to the SERVER BOOT; a CLI opened beside a running server must
// not close its in-flight work or lift the schema under it). A fresh
// profile is a clean bootstrap — there is no previous life to harm —
// which keeps the intent-create-before-first-boot flow alive.
func OpenOperator(path string) (*Store, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("action/sqlite: resolve path %q: %w", path, err)
	}
	if _, err := os.Stat(abs); err == nil {
		version, err := storedSchemaVersion(abs)
		if err != nil {
			return nil, err
		}
		if version != schemaVersionCurrent {
			return nil, fmt.Errorf("action/sqlite: store %q is at schema v%d, this binary writes v%d — an operator act never migrates an existing store; run the server boot to lift the schema", abs, version, schemaVersionCurrent)
		}
	}
	return open(path)
}

// storedSchemaVersion probes an existing store's version through a
// sealed read-only connection — the probe itself must not mutate.
func storedSchemaVersion(abs string) (int, error) {
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(abs)))
	if err != nil {
		return 0, fmt.Errorf("action/sqlite: probe %q: %w", abs, err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`PRAGMA query_only = 1`); err != nil {
		return 0, fmt.Errorf("action/sqlite: seal probe connection %q: %w", abs, err)
	}
	var version int
	if err := db.QueryRow(`SELECT version FROM action_schema`).Scan(&version); err != nil {
		return 0, fmt.Errorf("action/sqlite: read schema version of %q (not a korvun store?): %w", abs, err)
	}
	return version, nil
}

// open is the shared mold behind Open and the test seam: pool, bootstrap
// and ping, with the sealed retention defaults.
func open(path string) (*Store, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("action/sqlite: resolve path %q: %w", path, err)
	}
	if dir := filepath.Dir(abs); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("action/sqlite: create data dir %q: %w", dir, err)
		}
	}
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(abs)))
	if err != nil {
		return nil, fmt.Errorf("action/sqlite: open %q: %w", abs, err)
	}
	// Single serialized writer, the load-bearing house discipline: one
	// connection serializes whole transactions, so the write patterns
	// below are race-free without SQLITE_BUSY between our own callers.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(createStmt); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("action/sqlite: bootstrap schema in %q: %w", abs, err)
	}
	if _, err := db.Exec(
		`INSERT INTO action_schema (version) SELECT 1 WHERE NOT EXISTS (SELECT 1 FROM action_schema)`,
	); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("action/sqlite: seed schema version in %q: %w", abs, err)
	}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("action/sqlite: migrate %q: %w", abs, err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("action/sqlite: ping %q: %w", abs, err)
	}
	return &Store{db: db, capRows: defaultCapRows, pruneEvery: defaultPruneEvery}, nil
}

// RecoverPreviousLife closes every non-terminal action left behind by
// a previous process life — and NEVER re-executes (the blueprint's
// §16.4 honesty applied early: the kernel does not invent idempotency
// it does not have yet). Exported by R3: the BOOT calls it AFTER the
// keystore and sealer are wired, so every recovery close is a terminal
// like any other — recorded WITH its era's signed receipt, row by row.
//
// The Etapa-5 exemptions (found by the cross-check family): a PARKED
// action (PENDING_APPROVAL) waits for a human BY DESIGN — the expiry
// clock governs it, never this pass. An APPROVED action whose params
// are still held awaits its deferred execution and survives. C5: an
// APPROVED action whose params were CLAIMED was mid-execution — the
// external effect may or may not have fired; it closes OUTCOME_UNKNOWN
// with the uncertainty NAMED, never a FAILED lie.
func (s *Store) RecoverPreviousLife(ctx context.Context) (skipped int, err error) {
	passes := []struct {
		query     string
		args      []any
		to        action.State
		marker    string
		predicate string
	}{
		{
			query: `SELECT action_id FROM actions
			         WHERE state = ? AND NOT EXISTS (
			               SELECT 1 FROM approvals
			                WHERE approvals.action_id = actions.action_id
			                  AND approvals.canonical_params != '')`,
			args:      []any{string(action.StateApproved)},
			to:        action.StateOutcomeUnknown,
			marker:    recoveryMarkerOutcomeUnknown,
			predicate: claimedOrphanPredicate,
		},
		{
			query: `SELECT action_id FROM actions
			         WHERE state NOT IN (?, ?, ?, ?, ?, ?, ?, ?)`,
			args: []any{
				string(action.StateDenied), string(action.StateShadowed),
				string(action.StateSucceeded), string(action.StateFailed),
				string(action.StateRejected), string(action.StatePendingApproval),
				string(action.StateApproved), string(action.StateOutcomeUnknown),
			},
			to:        action.StateFailed,
			marker:    recoveryMarkerCrash,
			predicate: crashOrphanPredicate,
		},
	}
	now := time.Now().UTC()
	for _, pass := range passes {
		ids, err := s.collectIDs(ctx, pass.query, pass.args...)
		if err != nil {
			return skipped, fmt.Errorf("action/sqlite: recovery pass: %w", err)
		}
		for _, id := range ids {
			// R4-F3: real errors (a dead context included) abort; a
			// lost clean race is changed=false and the loop moves on.
			if err := ctx.Err(); err != nil {
				return skipped, fmt.Errorf("action/sqlite: recovery pass: %w", err)
			}
			if _, err := s.closeCrashOrphan(ctx, id, pass.to, pass.marker, pass.predicate, now); err != nil {
				if isBusyClass(err) {
					// R7-Y5: NEVER silent — the postponement is counted
					// and the boot logs the named note; the next pass
					// owns whatever another connection held.
					skipped++
					continue
				}
				return skipped, err
			}
		}
	}
	return skipped, nil
}

// The per-pass eligibility predicates (R4-F3): each crash close carries
// its COMPLETE condition inside the UPDATE itself, so ownership is
// decided by the row store atomically — never by an earlier SELECT.
const (
	// claimedOrphanPredicate: still APPROVED and params still absent.
	claimedOrphanPredicate = ` AND state = 'APPROVED' AND NOT EXISTS (
		SELECT 1 FROM approvals
		 WHERE approvals.action_id = actions.action_id
		   AND approvals.canonical_params != '')`
	// crashOrphanPredicate: still a non-terminal, non-exempt state.
	crashOrphanPredicate = ` AND state NOT IN ('DENIED','SHADOWED','SUCCEEDED','FAILED',
		'REJECTED','PENDING_APPROVAL','APPROVED','OUTCOME_UNKNOWN')`
)

// isBusyClass reports the SQLITE_BUSY family across its spellings —
// plain busy (5) and the WAL snapshot variant (517): another live
// connection legitimately holds the row. For the SWEEPERS that is a
// clean race (R4-F3 across real connections): skip and move on, never
// a boot-fatal; the next pass owns whatever remains.
func isBusyClass(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "SQLITE_BUSY") ||
		strings.Contains(err.Error(), "database is locked"))
}

// collectIDs runs a SELECT of action ids.
func (s *Store) collectIDs(ctx context.Context, query string, args ...any) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// closeCrashOrphan closes ONE crash orphan in its own transaction:
// the state close and its terminal receipt land together — no terminal
// is born without its receipt, recovery's included (R3). R4-F3: the
// UPDATE carries the pass's COMPLETE eligibility predicate and
// RowsAffected decides ownership — zero rows means another process
// (a concurrent Finish, another recovery) owned the row legitimately:
// no receipt, no drama, changed=false.
func (s *Store) closeCrashOrphan(ctx context.Context, actionID string, to action.State, marker, predicate string, at time.Time) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("action/sqlite: begin crash close %q: %w", actionID, err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx,
		`UPDATE actions SET state = ?, recovery_marker = ?, finished_at = ?
		  WHERE action_id = ?`+predicate, // #nosec G202 -- predicate is one of two package constants
		string(to), marker, at.Format(time.RFC3339Nano), actionID)
	if err != nil {
		return false, fmt.Errorf("action/sqlite: crash close %q: %w", actionID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, nil
	}
	r, err := s.receiptForFinish(ctx, tx, actionID, to, at, "")
	if err != nil {
		return false, err
	}
	if err := s.appendReceiptTx(ctx, tx, r); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("action/sqlite: commit crash close %q: %w", actionID, err)
	}
	return true, nil
}

// Prune enforces the retention cap: when total rows exceed it, the OLDEST
// TERMINAL rows are deleted (decisions cascade) until the total is back at
// the cap — or until no terminal remains, because live rows are never
// touched, cap or no cap. Returns how many actions were removed.
func (s *Store) Prune(ctx context.Context) (int, error) {
	total, err := s.Count(ctx)
	if err != nil {
		return 0, err
	}
	excess := total - s.capRows
	if excess <= 0 {
		return 0, nil
	}
	// C6: the E5/C5 terminals (REJECTED, OUTCOME_UNKNOWN) are prunable
	// like every other terminal — without them the retention cap leaks.
	// The evidence exemption is elsewhere by construction: receipts
	// live in their own chain and are never touched by this pass.
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM actions WHERE action_id IN (
		    SELECT action_id FROM actions
		     WHERE state IN (?, ?, ?, ?, ?, ?)
		     ORDER BY requested_at ASC, action_id ASC
		     LIMIT ?)`,
		string(action.StateDenied), string(action.StateShadowed),
		string(action.StateSucceeded), string(action.StateFailed),
		string(action.StateRejected), string(action.StateOutcomeUnknown),
		excess,
	)
	if err != nil {
		return 0, fmt.Errorf("action/sqlite: prune: %w", err)
	}
	removed, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("action/sqlite: prune rows affected: %w", err)
	}
	return int(removed), nil
}

// Close releases the store's connection pool.
func (s *Store) Close() error {
	return s.db.Close()
}

// SchemaVersion reports the store's OWN schema lifecycle version.
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	var v int
	if err := s.db.QueryRowContext(ctx, `SELECT version FROM action_schema`).Scan(&v); err != nil {
		return 0, fmt.Errorf("action/sqlite: read schema version: %w", err)
	}
	return v, nil
}

// RecordAttempt persists one attempt WITH its decision in a single
// transaction — the blueprint contract: every attempt produces an
// explainable decision and a durable record BEFORE any effect. Only the
// three decision outcomes of the machine (DENIED, SHADOWED, AUTHORIZED)
// may enter; anything else wraps ErrNotADecisionState. A duplicate
// action_id fails atomically: the transaction leaves no partial write.
func (s *Store) RecordAttempt(ctx context.Context, env action.Envelope, d Decision, state action.State) error {
	switch state {
	case action.StateDenied, action.StateShadowed, action.StateAuthorized:
	default:
		return fmt.Errorf("%w: %s", ErrNotADecisionState, state)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("action/sqlite: begin record: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO actions (action_id, schema_version, correlation_id,
		    source_kind, source_protocol, source_channel,
		    op_namespace, op_name, op_version,
		    parameters_digest, effect_class, state, requested_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		env.ActionID, env.SchemaVersion, env.CorrelationID,
		env.Source.Kind, env.Source.Protocol, env.Source.Channel,
		env.Operation.Namespace, env.Operation.Name, env.Operation.Version,
		env.ParametersDigest, env.Effect.Class, string(state),
		env.RequestedAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("action/sqlite: insert action %q: %w", env.ActionID, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO action_decisions (action_id, outcome, rule, decided_at, policy_version, policy_digest)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		env.ActionID, d.Outcome, d.Rule, env.RequestedAt.UTC().Format(time.RFC3339Nano),
		d.PolicyVersion, d.PolicyDigest,
	); err != nil {
		return fmt.Errorf("action/sqlite: insert decision %q: %w", env.ActionID, err)
	}
	// Terminal decision outcomes (DENIED, SHADOWED — the sealed NC-1/NC-2
	// yeses) birth their receipt in this same transaction.
	if state != action.StateAuthorized {
		if err := s.appendReceiptTx(ctx, tx, s.receiptForRecord(ctx, tx, env, d, state)); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("action/sqlite: commit record %q: %w", env.ActionID, err)
	}
	return s.noteWrite(ctx)
}

// noteWrite is the periodic half of the retention invariant: every
// pruneEvery-th committed attempt pays the (cheap, bounded) prune, so the
// file stays capped without any scheduler or config.
func (s *Store) noteWrite(ctx context.Context) error {
	s.writesMu.Lock()
	s.writes++
	due := s.writes%s.pruneEvery == 0
	s.writesMu.Unlock()
	if due {
		if _, err := s.Prune(ctx); err != nil {
			return err
		}
		// R4: the prune cadence also pays the expiry sweep, so a parked
		// request nobody touches cannot outlive its window forever.
		if _, _, err := s.SweepExpiredApprovals(ctx, time.Now().UTC()); err != nil {
			return err
		}
	}
	return nil
}

// Finish moved to ledger.go (Etapa 4): FinishWithResult births the
// receipt in the same closing transaction; Finish is its empty-digest
// form — the E1 seam signature unchanged.

// Get returns one stored record, envelope round-tripped verbatim.
func (s *Store) Get(ctx context.Context, actionID string) (Record, error) {
	var (
		rec           Record
		state         string
		requestedAt   string
		finishedAt    sql.NullString
		principalID   sql.NullString
		intentID      sql.NullString
		authorityRefs sql.NullString
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT a.action_id, a.schema_version, a.correlation_id,
		        a.source_kind, a.source_protocol, a.source_channel,
		        a.op_namespace, a.op_name, a.op_version,
		        a.parameters_digest, a.effect_class, a.state,
		        a.recovery_marker, a.requested_at, a.finished_at,
		        a.principal_id, a.intent_id, a.authority_refs,
		        d.outcome, d.rule, d.policy_version, d.policy_digest
		   FROM actions a JOIN action_decisions d ON d.action_id = a.action_id
		  WHERE a.action_id = ?`, actionID,
	).Scan(
		&rec.Envelope.ActionID, &rec.Envelope.SchemaVersion, &rec.Envelope.CorrelationID,
		&rec.Envelope.Source.Kind, &rec.Envelope.Source.Protocol, &rec.Envelope.Source.Channel,
		&rec.Envelope.Operation.Namespace, &rec.Envelope.Operation.Name, &rec.Envelope.Operation.Version,
		&rec.Envelope.ParametersDigest, &rec.Envelope.Effect.Class, &state,
		&rec.RecoveryMarker, &requestedAt, &finishedAt,
		&principalID, &intentID, &authorityRefs,
		&rec.Decision.Outcome, &rec.Decision.Rule,
		&rec.Decision.PolicyVersion, &rec.Decision.PolicyDigest,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, fmt.Errorf("%w: %q", ErrNotFound, actionID)
	}
	if err != nil {
		return Record{}, fmt.Errorf("action/sqlite: get %q: %w", actionID, err)
	}
	rec.State = action.State(state)
	if principalID.Valid {
		identity := StoredIdentity{PrincipalID: principalID.String, IntentID: intentID.String}
		if authorityRefs.Valid && authorityRefs.String != "" {
			if err := json.Unmarshal([]byte(authorityRefs.String), &identity.AuthorityRefs); err != nil {
				return Record{}, fmt.Errorf("action/sqlite: parse authority_refs of %q: %w", actionID, err)
			}
		}
		rec.Identity = &identity
	}
	at, err := time.Parse(time.RFC3339Nano, requestedAt)
	if err != nil {
		return Record{}, fmt.Errorf("action/sqlite: parse requested_at of %q: %w", actionID, err)
	}
	rec.Envelope.RequestedAt = at
	if finishedAt.Valid {
		f, err := time.Parse(time.RFC3339Nano, finishedAt.String)
		if err != nil {
			return Record{}, fmt.Errorf("action/sqlite: parse finished_at of %q: %w", actionID, err)
		}
		rec.FinishedAt = &f
	}
	return rec, nil
}

// Count returns the number of stored actions.
func (s *Store) Count(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM actions`).Scan(&n); err != nil {
		return 0, fmt.Errorf("action/sqlite: count: %w", err)
	}
	return n, nil
}

// OpenReadOnly opens the store for verification and consultation — the
// R1 door born from the 2026-08-31 external audit (the cross-check
// law, point 3: a "does not write" pin covers the WHOLE door). It runs
// NO bootstrap, NO migration, NO crash recovery and NO retention prune,
// never creates files or directories, and locks the connection itself
// with PRAGMA query_only so every write — domain path or hand-written
// SQL — dies at the SQLite level. WAL-compatible in every state: the
// file opens through the normal VFS path (a live writer's readers do
// not block it), only the connection is sealed. A schema OLDER than
// this binary's current version is REFUSED by name, never migrated —
// the ceremony's "verify migrated the profile" precedent can not
// recur; a NEWER schema is refused too (an older binary must not
// misread a future store).
func OpenReadOnly(path string) (*Store, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("action/sqlite: resolve path %q: %w", path, err)
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, fmt.Errorf("action/sqlite: read-only open %q: %w", abs, err)
	}
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(abs)))
	if err != nil {
		return nil, fmt.Errorf("action/sqlite: read-only open %q: %w", abs, err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA query_only = 1`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("action/sqlite: seal read-only connection %q: %w", abs, err)
	}
	var version int
	if err := db.QueryRow(`SELECT version FROM action_schema`).Scan(&version); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("action/sqlite: read schema version of %q (not a korvun store?): %w", abs, err)
	}
	if version != schemaVersionCurrent {
		_ = db.Close()
		return nil, fmt.Errorf("action/sqlite: store %q is at schema v%d, this binary reads v%d — a read-only consult never migrates; run the server boot to lift the schema", abs, version, schemaVersionCurrent)
	}
	return &Store{db: db, capRows: defaultCapRows, pruneEvery: defaultPruneEvery, readOnly: true}, nil
}

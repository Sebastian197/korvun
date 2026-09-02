// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R4 Phase 4 (FR-R4F4-1/2, ADR-0046): approvals.action_id becomes a
// REAL foreign key with ON DELETE CASCADE via v8→v9 transactional
// table reconstruction — crash-rehearsed on the AS-8 anti-zombie mold
// (an aborted migration lands cleanly back on v8 with the old table
// intact). Orphan approvals predating v9 are RETIRED by the copy
// filter, declared: their receipt is their evidence. The prune now
// cascades the approval while the SIGNED receipt survives — and the
// resource-bound invariant is DEMONSTRATED over thousands of
// park→close→prune cycles. Reproduction-first contract.

package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// rawV7Delta / rawV8Delta restate the migration DDL literally (oracle
// discipline, the rawV6Delta mold).
const rawV7DeltaV9 = `
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
CREATE INDEX approvals_by_status ON approvals(status);
UPDATE action_schema SET version = 7;`

const rawV8Delta = `
ALTER TABLE receipts ADD COLUMN schema_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE receipts ADD COLUMN approval_digest TEXT NOT NULL DEFAULT '';
UPDATE action_schema SET version = 8;`

// buildV8File hand-builds a real v8 store file.
func buildV8File(t *testing.T) string {
	t.Helper()
	path := buildV6File(t)
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(rawV7DeltaV9 + rawV8Delta); err != nil {
		t.Fatalf("build v7+v8 deltas: %v", err)
	}
	return path
}

// seedV8Approvals plants one LINKED approval (action row present) and
// one ORPHAN (action pruned under the pre-v9 exemption).
func seedV8Approvals(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = db.Close() }()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO actions (action_id, schema_version, correlation_id,
	        source_kind, source_protocol, source_channel, op_namespace, op_name, op_version,
	        parameters_digest, effect_class, state, requested_at)
	     VALUES ('act_linked', 1, 'c', 'agent_brain', 'text', 'console',
	        'tool', 'echo', 1, 'sha256:d', 'pure', 'PENDING_APPROVAL', ?)`, now); err != nil {
		t.Fatalf("seed action: %v", err)
	}
	for _, row := range []struct{ apr, act string }{
		{"apr_linked00000000000000000000000001", "act_linked"},
		{"apr_orphan00000000000000000000000001", "act_gone_pruned"},
	} {
		if _, err := db.Exec(`INSERT INTO approvals (approval_id, schema_version, action_id,
		        action_digest, preview_digest, canonical_preview, canonical_params,
		        requested_from, reason, risk_summary, policy_version, policy_digest,
		        requested_at, status)
		     VALUES (?, 1, ?, 'sha256:d', 'sha256:p', '{}', '', 'principal_operator',
		        'require_approval', 'r', 1, 'sha256:l', ?, 'PENDING')`,
			row.apr, row.act, now); err != nil {
			t.Fatalf("seed approval %s: %v", row.apr, err)
		}
	}
}

func TestMigrationV9_freshCrashAndOrphanRetirement(t *testing.T) {
	t.Parallel()
	// Fresh: lands on current with a WORKING cascade.
	path := filepath.Join(t.TempDir(), "korvun.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if v, _ := store.SchemaVersion(context.Background()); v != schemaVersionCurrent {
		t.Fatalf("fresh store lands on current, got %d", v)
	}
	_ = store.Close()

	// Existing v8 with a linked approval AND a pre-v9 orphan.
	crashPath := buildV8File(t)
	seedV8Approvals(t, crashPath)
	// Crash mid-v9: the AS-8 anti-zombie mold.
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(crashPath)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := db.Exec(
		`CREATE TRIGGER crash_mid_v9 BEFORE UPDATE ON action_schema
		 BEGIN SELECT RAISE(ABORT, 'crash mid v9'); END;`); err != nil {
		t.Fatalf("install crash trigger: %v", err)
	}
	_ = db.Close()
	if _, err := Open(crashPath); err == nil {
		t.Fatal("an aborted v9 migration must be boot-fatal")
	}
	if v := inspect(t, crashPath, `SELECT version FROM action_schema`); v != 8 {
		t.Fatalf("aborted migration must land cleanly back on v8, got %d", v)
	}
	if n := inspect(t, crashPath, `SELECT COUNT(*) FROM approvals`); n != 2 {
		t.Fatalf("the old table must be intact after the rollback: %d rows", n)
	}
	db2, _ := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(crashPath)))
	if _, err := db2.Exec(`DROP TRIGGER crash_mid_v9`); err != nil {
		t.Fatalf("drop trigger: %v", err)
	}
	_ = db2.Close()

	// The next boot completes: FK live, linked row kept, orphan RETIRED.
	recovered, err := Open(crashPath)
	if err != nil {
		t.Fatalf("the next boot must complete: %v", err)
	}
	defer func() { _ = recovered.Close() }()
	if v, _ := recovered.SchemaVersion(context.Background()); v != schemaVersionCurrent {
		t.Fatalf("completed migration lands on current, got %d", v)
	}
	if n := inspect(t, crashPath, `SELECT COUNT(*) FROM approvals`); n != 1 {
		t.Fatalf("ADR-0046: the pre-v9 orphan is RETIRED, the linked row kept: %d", n)
	}
	if n := inspect(t, crashPath,
		`SELECT COUNT(*) FROM approvals WHERE approval_id = 'apr_linked00000000000000000000000001'`); n != 1 {
		t.Fatal("the linked approval survives the reconstruction")
	}
}

func TestPrune_cascadesTheApprovalAndTheReceiptSurvives(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	a := expiredParked(t, store, "act_f4_casc")
	if _, _, err := store.SweepExpiredApprovals(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	total, _ := store.Count(ctx)
	store.capRows = total - 1
	removed, err := store.Prune(ctx)
	if err != nil || removed != 1 {
		t.Fatalf("prune the terminal: %v %d", err, removed)
	}
	if _, err := store.Get(ctx, "act_f4_casc"); err == nil {
		t.Fatal("the terminal action must be pruned")
	}
	// AUDIT R4-F4: the approval CASCADED with its action...
	var n int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM approvals WHERE approval_id = ?`,
		a.ApprovalID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("the approval row cascades with retention: %v %d", err, n)
	}
	// ...and the SIGNED receipt is the surviving evidence.
	receipts, err := store.ReceiptsByAction(ctx, "act_f4_casc")
	if err != nil || len(receipts) != 1 || receipts[0].SigningKeyID == "" {
		t.Fatalf("the receipt IS the evidence and survives: %v %d", err, len(receipts))
	}
}

// FR-R4F4-4: the resource-bound invariant, demonstrated — thousands of
// park→close→prune cycles never exceed the live-approvals bound while
// every receipt survives.
func TestResourceBound_thousandsOfCyclesStayBounded(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	store.capRows = 50
	ctx := context.Background()
	const cycles = 2000
	for i := 0; i < cycles; i++ {
		id := fmt.Sprintf("act_rb_%04d", i)
		expiredParked(t, store, id)
		if i%100 == 99 {
			if _, _, err := store.SweepExpiredApprovals(ctx, time.Now().UTC()); err != nil {
				t.Fatalf("sweep at %d: %v", i, err)
			}
			if _, err := store.Prune(ctx); err != nil {
				t.Fatalf("prune at %d: %v", i, err)
			}
		}
	}
	if _, _, err := store.SweepExpiredApprovals(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("final sweep: %v", err)
	}
	if _, err := store.Prune(ctx); err != nil {
		t.Fatalf("final prune: %v", err)
	}
	var approvals, receipts int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM approvals`).Scan(&approvals); err != nil {
		t.Fatalf("count approvals: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM receipts`).Scan(&receipts); err != nil {
		t.Fatalf("count receipts: %v", err)
	}
	if approvals > store.capRows {
		t.Fatalf("AUDIT R4-F4: %d cycles left %d approval rows — the bound (%d) leaked", cycles, approvals, store.capRows)
	}
	if receipts < cycles {
		t.Fatalf("every cycle's evidence survives: %d receipts for %d cycles", receipts, cycles)
	}
}

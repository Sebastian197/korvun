// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R7-Y5 (sixth Codex pass, P2): the recovery's busy skip is NEVER
// silent — the pass returns a skipped count (the sweep's mold) and
// the boot logs the named note. The test FORCES the busy: a second
// connection holds a write lock across the pass; the orphan stays
// visible and the count says so; once released, the next pass owns
// it. Reproduction-first contract.

package sqlite

import (
	"context"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
)

func TestRecovery_busyIsCountedNeverSilent(t *testing.T) {
	t.Parallel()
	s1, s2, _ := twoSealedStores(t)
	ctx := context.Background()
	mustRecord(t, s1, "act_y5_orphan", action.StateAuthorized)
	// The second connection HOLDS the writer across the pass.
	tx, err := s2.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("hold tx: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE actions SET recovery_marker = recovery_marker WHERE action_id = 'act_y5_orphan'`); err != nil {
		t.Fatalf("hold write: %v", err)
	}
	skipped, err := s1.RecoverPreviousLife(ctx)
	if err != nil {
		t.Fatalf("a held writer is a clean postponement, not an error: %v", err)
	}
	if skipped != 1 {
		t.Fatalf("AUDIT R7-Y5: the postponement must be COUNTED, never silent: skipped=%d", skipped)
	}
	// The orphan is VISIBLY untouched while postponed.
	_ = tx.Rollback()
	rec, err := s1.Get(ctx, "act_y5_orphan")
	if err != nil || rec.State != action.StateAuthorized {
		t.Fatalf("the orphan stays visible while postponed: %v %v", err, rec.State)
	}
	// Released: the next pass owns it.
	skipped, err = s1.RecoverPreviousLife(ctx)
	if err != nil || skipped != 0 {
		t.Fatalf("the next pass owns the orphan: skipped=%d err=%v", skipped, err)
	}
	rec, _ = s1.Get(ctx, "act_y5_orphan")
	if rec.State != action.StateFailed || rec.RecoveryMarker != "crash_recovered" {
		t.Fatalf("closed by the next pass: %v %q", rec.State, rec.RecoveryMarker)
	}
}

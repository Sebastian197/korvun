// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The crash pass keeps its mould after AUTHORIZED left it (director's class
// cure, 2026-09-16).
//
// Moving AUTHORIZED to the unknown pass took away the only non-terminal state
// the public door can write: RecordAttempt accepts DENIED, SHADOWED and
// AUTHORIZED and nothing else. The FAILED pass is therefore unreachable
// through the API from this commit on — but NOT unreachable on disk: a store
// written by an older binary, or by a future state this code does not know,
// can hold a row in any non-terminal state, and that row must still close.
// This mould forces exactly that with a raw UPDATE, so the pass keeps a test
// instead of quietly becoming decoration.
//
// Evidence level, honest: a real store, closed and reopened, with the legacy
// state written behind the domain API. In-process; not a real crash.
package sqlite

import (
	"context"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
)

// TestRecovery_aLegacyNonTerminalStillClosesFailed is that mould.
//
// Probing mutation (N10, executed, red, declared in the canto): disable the
// crash pass in RecoverPreviousLife ⇒ the row stays PREPARING and this reddens.
func TestRecovery_aLegacyNonTerminalStillClosesFailed(t *testing.T) {
	t.Parallel()
	store, path := openTemp(t)
	ctx := context.Background()

	mustRecord(t, store, "act_legacy_orphan", action.StateAuthorized)
	// A state this code never writes, as an older life could have left it.
	corruptCell(t, store, "actions", "state", "action_id", "act_legacy_orphan", "PREPARING")
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	if _, err := reopened.RecoverPreviousLife(ctx); err != nil {
		t.Fatalf("recovery pass: %v", err)
	}

	rec, err := reopened.Get(ctx, "act_legacy_orphan")
	if err != nil {
		t.Fatalf("read the recovered row: %v", err)
	}
	if rec.State != action.StateFailed || rec.RecoveryMarker != "crash_recovered" {
		t.Fatalf("state = %q marker = %q, want FAILED + crash_recovered", rec.State, rec.RecoveryMarker)
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// An AUTHORIZED row that survived a crash is UNKNOWN, never FAILED — half of
// the fourth class cure the director ordered on 2026-09-16.
//
// AUTHORIZED is the state an attempt carries WHILE its tool runs: the record
// lands before the effect (proof is part of execution), and the terminal
// close comes after. A process that dies in between leaves the row in
// AUTHORIZED with the external effect in an unknown state — the POST may have
// been delivered, the file may have been written. The recovery pass closed
// every such row FAILED, which is a definite claim about an effect nobody
// observed: the same «FAILED lie» the C5 comment names for the claimed-params
// case, on the state that precedes it.
//
// Evidence level, honest: a real store, closed and reopened — the recovery
// pass's own door. In-process; not a separate OS process and not a real crash.
package sqlite

import (
	"context"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
)

// TestRecovery_anAuthorizedRowClosesUnknownNotFailed is the mould.
//
// Probing mutations (executed, red, declared in the canto): N8 disables the
// AUTHORIZED pass, so the row stays AUTHORIZED; N12 sends AUTHORIZED back to
// the crash pass, so it closes `action.StateFailed`. Both redden this mould,
// and both were captured — the first form alone would not have told FAILED
// from «never closed».
func TestRecovery_anAuthorizedRowClosesUnknownNotFailed(t *testing.T) {
	t.Parallel()
	store, path := openTemp(t)
	ctx := context.Background()

	// The attempt is recorded AUTHORIZED — the tool is running — and the
	// process dies there.
	mustRecord(t, store, "act_authorized_orphan", action.StateAuthorized)
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

	rec, err := reopened.Get(ctx, "act_authorized_orphan")
	if err != nil {
		t.Fatalf("read the recovered row: %v", err)
	}
	if rec.State == action.StateFailed {
		t.Fatal("the recovery pass closed FAILED over an effect nobody observed")
	}
	if rec.State != action.StateOutcomeUnknown {
		t.Fatalf("state = %q, want OUTCOME_UNKNOWN", rec.State)
	}
}

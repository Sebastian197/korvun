// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Recovery and retention contract — lote 2, pieza 2 (spec FR-STORE-2/3,
// sealed decision 2): a previous life's non-terminal actions close FAILED
// with the recovery marker and are NEVER re-executed; the file never grows
// without bound, and pruning only ever touches terminal rows. Approved-red
// contract: not edited to fit an implementation.

package sqlite

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

func mustRecord(t *testing.T, store *Store, id string, state action.State) {
	t.Helper()
	outcome := map[action.State]string{
		action.StateAuthorized: "allow",
		action.StateDenied:     "deny",
		action.StateShadowed:   "shadow",
	}[state]
	env := action.NewEnvelope(
		id, "env-1",
		action.Source{Kind: "agent_brain", Protocol: "text", Channel: "console"},
		action.Operation{Namespace: "tool", Name: "echo", Version: 1},
		`{"a":1}`,
		// Monotonic instants so retention's oldest-first order is testable.
		time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC).Add(time.Duration(recordSeq(id))*time.Second),
	)
	if err := store.RecordAttempt(context.Background(), env, Decision{Outcome: outcome, Rule: "r"}, state); err != nil {
		t.Fatalf("RecordAttempt(%s, %s): %v", id, state, err)
	}
}

// recordSeq derives a stable per-id ordering from the id's numeric suffix.
func recordSeq(id string) int {
	var n int
	_, _ = fmt.Sscanf(id, "act_%d", &n)
	return n
}

func TestOpen_recoveryClosesNonTerminalsAndNeverReexecutes(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "korvun.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	mustRecord(t, store, "act_1", action.StateAuthorized) // will crash mid-flight
	mustRecord(t, store, "act_2", action.StateDenied)     // terminal already
	mustRecord(t, store, "act_3", action.StateAuthorized)
	if err := store.Finish(context.Background(), "act_3", action.StateSucceeded, time.Date(2026, 8, 30, 11, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if err := store.Close(); err != nil { // the "crash": act_1 never finished
		t.Fatalf("close: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if err := reopened.RecoverPreviousLife(context.Background()); err != nil {
		t.Fatalf("recovery pass: %v", err)
	}
	defer func() { _ = reopened.Close() }()

	crashed, err := reopened.Get(context.Background(), "act_1")
	if err != nil {
		t.Fatalf("Get(act_1): %v", err)
	}
	if crashed.State != action.StateFailed || crashed.RecoveryMarker != "crash_recovered" {
		t.Fatalf("a non-terminal from a previous life closes FAILED+marker, got state=%s marker=%q",
			crashed.State, crashed.RecoveryMarker)
	}
	if crashed.FinishedAt == nil {
		t.Fatal("recovery must stamp finished_at")
	}
	// NEVER re-executed and never re-finishable: it is terminal now.
	err = reopened.Finish(context.Background(), "act_1", action.StateSucceeded, time.Now().UTC())
	if !errors.Is(err, action.ErrInvalidTransition) {
		t.Fatalf("a recovered action is terminal, got %v", err)
	}
	// Terminals from the previous life are untouched.
	for id, want := range map[string]action.State{"act_2": action.StateDenied, "act_3": action.StateSucceeded} {
		rec, err := reopened.Get(context.Background(), id)
		if err != nil {
			t.Fatalf("Get(%s): %v", id, err)
		}
		if rec.State != want || rec.RecoveryMarker != "" {
			t.Fatalf("terminal %s must survive recovery untouched: state=%s marker=%q", id, rec.State, rec.RecoveryMarker)
		}
	}
}

func TestPrune_removesOldestTerminalsOnlyDownToTheCap(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	store.capRows = 5
	for i := 1; i <= 4; i++ { // live rows: NEVER prunable
		mustRecord(t, store, fmt.Sprintf("act_%d", i), action.StateAuthorized)
	}
	for i := 5; i <= 10; i++ { // terminals, oldest first
		mustRecord(t, store, fmt.Sprintf("act_%d", i), action.StateDenied)
	}
	removed, err := store.Prune(context.Background())
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if removed != 5 {
		t.Fatalf("prune removes exactly the excess (10-5), got %d", removed)
	}
	for i := 1; i <= 4; i++ { // every live row survives
		if _, err := store.Get(context.Background(), fmt.Sprintf("act_%d", i)); err != nil {
			t.Fatalf("live act_%d must survive pruning: %v", i, err)
		}
	}
	if _, err := store.Get(context.Background(), "act_10"); err != nil {
		t.Fatalf("the NEWEST terminal survives: %v", err)
	}
	for i := 5; i <= 9; i++ { // the oldest terminals are gone
		if _, err := store.Get(context.Background(), fmt.Sprintf("act_%d", i)); !errors.Is(err, ErrNotFound) {
			t.Fatalf("oldest terminal act_%d must be pruned, got %v", i, err)
		}
	}
}

func TestPrune_neverTouchesLiveRowsEvenBeyondTheCap(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	store.capRows = 3
	for i := 1; i <= 5; i++ {
		mustRecord(t, store, fmt.Sprintf("act_%d", i), action.StateAuthorized)
	}
	removed, err := store.Prune(context.Background())
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if removed != 0 {
		t.Fatalf("live rows are untouchable even beyond the cap, removed %d", removed)
	}
	if n, _ := store.Count(context.Background()); n != 5 {
		t.Fatalf("count = %d, want all 5 live rows", n)
	}
}

func TestOpen_prunesAtOpen(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "korvun.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for i := 1; i <= 10; i++ {
		mustRecord(t, store, fmt.Sprintf("act_%d", i), action.StateShadowed)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	reopened, err := openWithCap(path, 4)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	if n, _ := reopened.Count(context.Background()); n != 4 {
		t.Fatalf("Open must prune down to the cap, count = %d", n)
	}
}

func TestRecordAttempt_periodicPruneKeepsTheFileBounded(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	store.capRows = 6
	store.pruneEvery = 4
	for i := 1; i <= 40; i++ {
		mustRecord(t, store, fmt.Sprintf("act_%d", i), action.StateDenied)
	}
	n, err := store.Count(context.Background())
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n > store.capRows+store.pruneEvery {
		t.Fatalf("the file must stay bounded without any config: count=%d cap=%d every=%d",
			n, store.capRows, store.pruneEvery)
	}
}

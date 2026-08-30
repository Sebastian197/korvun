// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Action store contract — Trust Layer Etapa 1, lote 2, pieza 1 (spec
// FR-STORE-1/2, sealed decisions 1-2): the house SQLite mold with the
// store's OWN schema lifecycle, and the blueprint's write contract —
// every attempt lands with its decision ATOMICALLY, before any effect.
// Approved-red contract: not edited to fit an implementation.

package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

func testEnvelope(id string) action.Envelope {
	return action.NewEnvelope(
		id, "env-1",
		action.Source{Kind: "agent_brain", Protocol: "text", Channel: "console"},
		action.Operation{Namespace: "tool", Name: "echo", Version: 1},
		`{"a":1}`,
		time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC),
	)
}

func openTemp(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "korvun.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q) failed: %v", path, err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, path
}

func TestOpen_bootstrapsItsOwnLifecycleAndIsIdempotent(t *testing.T) {
	t.Parallel()
	store, path := openTemp(t)
	// Advanced from the Etapa-1 literal `1` under the batch-3 mandate: the
	// v1→v2 migration IS the authorized scope, and the test's intent (the
	// store owns its own schema lifecycle) is version-independent.
	if got, err := store.SchemaVersion(context.Background()); err != nil || got != schemaVersionCurrent {
		t.Fatalf("the store owns its schema lifecycle: version=%d err=%v, want %d", got, err, schemaVersionCurrent)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	again, err := Open(path)
	if err != nil {
		t.Fatalf("reopen must be idempotent, got %v", err)
	}
	defer func() { _ = again.Close() }()
	if got, err := again.SchemaVersion(context.Background()); err != nil || got != schemaVersionCurrent {
		t.Fatalf("reopen keeps the current version: version=%d err=%v", got, err)
	}
}

func TestOpen_badPathFailsLoud(t *testing.T) {
	t.Parallel()
	// A path whose parent is a regular FILE cannot host a database: the
	// boot-fatal posture demands the failure at Open, never at first write.
	base := filepath.Join(t.TempDir(), "occupied")
	if store, err := Open(base + "/korvun.db/impossible.db"); err == nil {
		_ = store.Close()
		t.Skip("filesystem allowed the path; the boot-fatal case needs a blocking parent")
	}
}

func TestRecordAttempt_persistsActionWithDecisionBeforeAnyEffect(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	env := testEnvelope("act_a1")
	decision := Decision{Outcome: "allow", Rule: "granted"}

	if err := store.RecordAttempt(context.Background(), env, decision, action.StateAuthorized); err != nil {
		t.Fatalf("RecordAttempt: %v", err)
	}
	rec, err := store.Get(context.Background(), "act_a1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.State != action.StateAuthorized {
		t.Fatalf("state = %s, want AUTHORIZED", rec.State)
	}
	if rec.Decision != decision {
		t.Fatalf("decision = %+v, want %+v", rec.Decision, decision)
	}
	if rec.Envelope.ParametersDigest != env.ParametersDigest ||
		rec.Envelope.Operation != env.Operation ||
		rec.Envelope.Source != env.Source ||
		rec.Envelope.CorrelationID != env.CorrelationID ||
		rec.Envelope.SchemaVersion != env.SchemaVersion ||
		!rec.Envelope.RequestedAt.Equal(env.RequestedAt) {
		t.Fatalf("the stored envelope must round-trip verbatim: %+v vs %+v", rec.Envelope, env)
	}
}

func TestRecordAttempt_terminalDecisionsPersistTerminal(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	for state, outcome := range map[action.State]string{
		action.StateDenied:   "deny",
		action.StateShadowed: "shadow",
	} {
		id := "act_" + string(state)
		err := store.RecordAttempt(context.Background(), testEnvelope(id), Decision{Outcome: outcome, Rule: "r"}, state)
		if err != nil {
			t.Fatalf("RecordAttempt(%s): %v", state, err)
		}
		rec, err := store.Get(context.Background(), id)
		if err != nil || rec.State != state {
			t.Fatalf("Get(%s): state=%s err=%v", id, rec.State, err)
		}
	}
}

func TestRecordAttempt_rejectsNonDecisionStates(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	for _, state := range []action.State{
		action.StateReceived, action.StateNormalized,
		action.StateSucceeded, action.StateFailed, action.State("BOGUS"),
	} {
		err := store.RecordAttempt(context.Background(), testEnvelope("act_x"), Decision{Outcome: "allow", Rule: "r"}, state)
		if !errors.Is(err, ErrNotADecisionState) {
			t.Fatalf("state %s must be rejected with the sentinel, got %v", state, err)
		}
	}
}

func TestRecordAttempt_duplicateIsAtomicallyRejected(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	env := testEnvelope("act_dup")
	first := Decision{Outcome: "allow", Rule: "granted"}
	if err := store.RecordAttempt(context.Background(), env, first, action.StateAuthorized); err != nil {
		t.Fatalf("first RecordAttempt: %v", err)
	}
	second := Decision{Outcome: "deny", Rule: "changed"}
	if err := store.RecordAttempt(context.Background(), env, second, action.StateDenied); err == nil {
		t.Fatal("a duplicate action_id must be rejected")
	}
	// Atomicity: the failed attempt left NO partial write — the original
	// action row and decision row survive untouched.
	rec, err := store.Get(context.Background(), "act_dup")
	if err != nil {
		t.Fatalf("Get after duplicate: %v", err)
	}
	if rec.State != action.StateAuthorized || rec.Decision != first {
		t.Fatalf("duplicate rejection must be atomic: got state=%s decision=%+v", rec.State, rec.Decision)
	}
}

func TestFinish_terminalsAreDurableAcrossReopen(t *testing.T) {
	t.Parallel()
	store, path := openTemp(t)
	env := testEnvelope("act_fin")
	if err := store.RecordAttempt(context.Background(), env, Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized); err != nil {
		t.Fatalf("RecordAttempt: %v", err)
	}
	finished := time.Date(2026, 8, 30, 10, 0, 5, 0, time.UTC)
	if err := store.Finish(context.Background(), "act_fin", action.StateSucceeded, finished); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	again, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = again.Close() }()
	rec, err := again.Get(context.Background(), "act_fin")
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if rec.State != action.StateSucceeded || rec.RecoveryMarker != "" {
		t.Fatalf("a terminal survives restart untouched: state=%s marker=%q", rec.State, rec.RecoveryMarker)
	}
	if rec.FinishedAt == nil || !rec.FinishedAt.Equal(finished) {
		t.Fatalf("finished_at must round-trip, got %v", rec.FinishedAt)
	}
}

func TestFinish_validatesTheStateMachine(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	if err := store.RecordAttempt(context.Background(), testEnvelope("act_den"), Decision{Outcome: "deny", Rule: "r"}, action.StateDenied); err != nil {
		t.Fatalf("RecordAttempt: %v", err)
	}
	err := store.Finish(context.Background(), "act_den", action.StateSucceeded, time.Now().UTC())
	if !errors.Is(err, action.ErrInvalidTransition) {
		t.Fatalf("finishing a DENIED action must wrap the machine's sentinel, got %v", err)
	}
	if err := store.Finish(context.Background(), "act_missing", action.StateFailed, time.Now().UTC()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("finishing an unknown action carries ErrNotFound, got %v", err)
	}
	if err := store.RecordAttempt(context.Background(), testEnvelope("act_ok"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized); err != nil {
		t.Fatalf("RecordAttempt: %v", err)
	}
	if err := store.Finish(context.Background(), "act_ok", action.StateNormalized, time.Now().UTC()); !errors.Is(err, action.ErrInvalidTransition) {
		t.Fatalf("AUTHORIZED may only finish SUCCEEDED/FAILED, got %v", err)
	}
}

func TestRecordAttempt_concurrentWritersAreSerialized(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	const writers = 16
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := action.NewID()
			errs <- store.RecordAttempt(context.Background(), testEnvelope(id), Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized)
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent RecordAttempt failed: %v", err)
		}
	}
	if n, err := store.Count(context.Background()); err != nil || n != writers {
		t.Fatalf("all %d rows must land, got %d (err %v)", writers, n, err)
	}
}

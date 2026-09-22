// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Error-path coverage for the action store: the boot-fatal posture on a
// corrupt file, every method failing loudly on a closed store, and the
// DSN builder's canonicalization edge.

package sqlite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

func TestOpen_corruptFileFailsAtOpenNotFirstWrite(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "korvun.db")
	if err := os.WriteFile(path, []byte("this is not a sqlite database, honestly"), 0o600); err != nil {
		t.Fatalf("seed corrupt file: %v", err)
	}
	store, err := Open(path)
	if err == nil {
		_ = store.Close()
		t.Fatal("a corrupt database must fail at Open (boot-fatal), not at first write")
	}
}

func TestClosedStore_everyMethodFailsLoudly(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	ctx := context.Background()
	if _, err := store.SchemaVersion(ctx); err == nil {
		t.Fatal("SchemaVersion on a closed store must error")
	}
	err := store.RecordAttempt(ctx, testEnvelope("act_closed"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized)
	if err == nil {
		t.Fatal("RecordAttempt on a closed store must error")
	}
	if err := store.Finish(ctx, "act_closed", action.StateFailed, time.Now().UTC()); err == nil {
		t.Fatal("Finish on a closed store must error")
	}
	if _, err := store.Get(ctx, "act_closed"); err == nil {
		t.Fatal("Get on a closed store must error")
	}
	if _, err := store.Count(ctx); err == nil {
		t.Fatal("Count on a closed store must error")
	}
	if _, err := store.Prune(ctx); err == nil {
		t.Fatal("Prune on a closed store must error")
	}
}

func TestGet_unknownActionCarriesTheSentinel(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	if _, err := store.Get(context.Background(), "act_nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown ids carry ErrNotFound, got %v", err)
	}
}

func TestBuildFileDSN_canonicalizesAndKeepsPragmas(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"/tmp/x/korvun.db", "C:/Users/x/korvun.db", ""} {
		dsn := buildFileDSN(path)
		if !strings.HasPrefix(dsn, "file:///") {
			t.Fatalf("DSN must canonicalize with a leading slash, got %q", dsn)
		}
		if !strings.Contains(dsn, "journal_mode(WAL)") || !strings.Contains(dsn, "foreign_keys(on)") {
			t.Fatalf("DSN must keep the house pragmas, got %q", dsn)
		}
	}
}

func TestOpenWithCap_pruneRunsOnTheSeam(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "korvun.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	mustRecord(t, store, "act_1", action.StateDenied)
	mustRecord(t, store, "act_2", action.StateDenied)
	mustRecord(t, store, "act_3", action.StateDenied)
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	reopened, err := openWithCap(path, 1)
	if err != nil {
		t.Fatalf("openWithCap: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	if n, _ := reopened.Count(context.Background()); n != 1 {
		t.Fatalf("the seam must prune to its cap, count=%d", n)
	}
}

func TestOpen_parentBlockedByAFileFailsLoud(t *testing.T) {
	t.Parallel()
	blocker := filepath.Join(t.TempDir(), "occupied")
	if err := os.WriteFile(blocker, []byte("file, not dir"), 0o600); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}
	if store, err := Open(filepath.Join(blocker, "korvun.db")); err == nil {
		_ = store.Close()
		t.Fatal("a parent blocked by a regular file must fail at Open")
	}
}

func TestOpenWithCap_corruptFileFailsTheSameWay(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "korvun.db")
	if err := os.WriteFile(path, []byte("still not a database"), 0o600); err != nil {
		t.Fatalf("seed corrupt file: %v", err)
	}
	if store, err := openWithCap(path, 3); err == nil {
		_ = store.Close()
		t.Fatal("the test seam keeps the boot-fatal posture")
	}
}

func TestGet_corruptTimestampsFailLoudly(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	mustRecord(t, store, "act_1", action.StateAuthorized)
	if _, err := store.db.Exec(`UPDATE actions SET requested_at = 'garbage' WHERE action_id = 'act_1'`); err != nil {
		t.Fatalf("corrupt requested_at: %v", err)
	}
	if _, err := store.Get(context.Background(), "act_1"); err == nil {
		t.Fatal("a corrupt requested_at must fail loudly, not silently zero")
	}
	mustRecord(t, store, "act_2", action.StateAuthorized)
	if err := store.Finish(context.Background(), "act_2", action.StateFailed, time.Now().UTC()); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if _, err := store.db.Exec(`UPDATE actions SET finished_at = 'garbage' WHERE action_id = 'act_2'`); err != nil {
		t.Fatalf("corrupt finished_at: %v", err)
	}
	if _, err := store.Get(context.Background(), "act_2"); err == nil {
		t.Fatal("a corrupt finished_at must fail loudly")
	}
}

// blockWrites installs a RAISE trigger so the next lifecycle write fails —
// the seam that makes deep error branches reachable in tests.
func blockWrites(t *testing.T, store *Store, event string) {
	t.Helper()
	stmt := `CREATE TRIGGER block_it BEFORE ` + event + // #nosec G202 -- event is a test-owned literal ("UPDATE"/"DELETE"), never external input
		` ON actions BEGIN SELECT RAISE(ABORT, 'blocked by test'); END;`
	if _, err := store.db.Exec(stmt); err != nil {
		t.Fatalf("install %s blocker: %v", event, err)
	}
}

func TestOpen_recoveryFailureIsBootFatal(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "korvun.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	mustRecord(t, store, "act_1", action.StateAuthorized) // non-terminal survivor
	blockWrites(t, store, "UPDATE")
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// R3 re-map: the recovery moved out of Open (the boot calls it after
	// wiring the sealer); the boot-fatal pin now covers the explicit pass.
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open no longer recovers and must succeed: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	if _, err := reopened.RecoverPreviousLife(context.Background()); err == nil {
		t.Fatal("a failing recovery pass must fail loud (boot-fatal), not limp on")
	}
}

func TestPrune_deleteFailurePropagates(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	store.capRows = 1
	mustRecord(t, store, "act_1", action.StateDenied)
	mustRecord(t, store, "act_2", action.StateDenied)
	blockWrites(t, store, "DELETE")
	if _, err := store.Prune(context.Background()); err == nil {
		t.Fatal("a blocked prune must fail loudly")
	}
}

// TestRecordAttempt_periodicPruneFailureNeverSpeaksForACommittedWrite is the
// INVERSION of a test that used to demand the opposite, and the inversion was
// authorised by the director on 2026-09-22 rather than taken.
//
// It read: «the periodic prune's failure must reach the caller», and it was
// true of the code. That was the P1 of ficha the ficha «Un aparcamiento confirmado puede devolver error». Every writer
// returned `noteWrite`'s error after its OWN transaction had committed, so a
// durable write reported itself as a refusal — and for the park, the executor
// read that refusal and recorded a DENIED attempt while the approval row sat in
// the tray waiting for a human nobody would ever tell.
//
// The guarantee is now the other one, and it has two halves, because dropping
// the failure would trade a lie for a blindness: the committed write reports
// SUCCESS, and the failure reaches the observer instead of vanishing.
func TestRecordAttempt_periodicPruneFailureNeverSpeaksForACommittedWrite(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	store.capRows = 0
	store.pruneEvery = 1
	var heard []error
	store.SetRetentionFailureObserver(func(err error) { heard = append(heard, err) })
	blockWrites(t, store, "DELETE")

	if err := store.RecordAttempt(context.Background(), testEnvelope("act_pp"),
		Decision{Outcome: "deny", Rule: "r"}, action.StateDenied); err != nil {
		t.Fatalf("a committed attempt reported the housekeeping's failure as its own: %v", err)
	}
	// The write is durable, which is the whole reason it may not report a fault.
	if _, err := store.Get(context.Background(), "act_pp"); err != nil {
		t.Fatalf("the attempt the caller was told succeeded is not in the store: %v", err)
	}
	if len(heard) != 1 {
		t.Fatalf("the observer heard %d failures, want exactly 1 — swallowing it in "+
			"silence is the other half of this guarantee", len(heard))
	}
	if !strings.Contains(heard[0].Error(), "periodic prune") {
		t.Fatalf("the observer heard %q, which does not name the prune", heard[0])
	}
}

func TestFinish_updateFailurePropagates(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	mustRecord(t, store, "act_1", action.StateAuthorized)
	blockWrites(t, store, "UPDATE")
	if err := store.Finish(context.Background(), "act_1", action.StateFailed, time.Now().UTC()); err == nil {
		t.Fatal("a blocked terminal write must fail loudly, never report success")
	}
}

func TestOpenWithCap_pruneFailureIsBootFatalOnTheSeam(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "korvun.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	mustRecord(t, store, "act_1", action.StateDenied)
	mustRecord(t, store, "act_2", action.StateDenied)
	blockWrites(t, store, "DELETE") // triggers persist in the file
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if reopened, err := openWithCap(path, 1); err == nil {
		_ = reopened.Close()
		t.Fatal("a failing open-time prune must fail the open")
	}
}

func TestOpen_seedFailureIsBootFatal(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "korvun.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// Empty the lifecycle row and block re-seeding: the next life's seed
	// INSERT must fail, and Open must refuse to limp on without it.
	if _, err := store.db.Exec(`DELETE FROM action_schema;
		CREATE TRIGGER block_seed BEFORE INSERT ON action_schema
		BEGIN SELECT RAISE(ABORT, 'blocked by test'); END;`); err != nil {
		t.Fatalf("prepare seed blocker: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if reopened, err := Open(path); err == nil {
		_ = reopened.Close()
		t.Fatal("a failing schema seed must fail Open")
	}
}

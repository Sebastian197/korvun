// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/conversation/sqlite"
)

// openTemp opens a store at a fresh temp path and registers its Close.
func openTemp(t *testing.T) *sqlite.SqliteStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "korvun.db")
	s, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestOpen_BootstrapsAndAccepts(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	const key = conversation.Key("telegram::1")

	got, err := s.Append(ctx, key, conversation.Turn{Role: conversation.RoleUser, Content: "hi"})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if got.Seq != 0 || got.Content != "hi" {
		t.Fatalf("Append returned %+v, want Seq 0 / Content hi", got)
	}
}

func TestOpen_ReopenExistingFileSucceeds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "korvun.db")
	s1, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open #1: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close #1: %v", err)
	}
	// CREATE TABLE IF NOT EXISTS must no-op on the second open.
	s2, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open #2 (existing file): %v", err)
	}
	_ = s2.Close()
}

func TestOpen_UnwritablePathIsAnError(t *testing.T) {
	// Make the parent a regular file so os.MkdirAll on it fails — the boot-fatal
	// path app relies on (ADR-0019 §4/§5).
	dir := t.TempDir()
	blocker := filepath.Join(dir, "afile")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, err := sqlite.Open(filepath.Join(blocker, "korvun.db"))
	if err == nil {
		t.Fatal("Open on an unwritable path returned nil error, want an error")
	}
}

func TestOpen_PathIsDirectoryFailsBootstrap(t *testing.T) {
	// Pointing at an existing directory: sql.Open is lazy and succeeds, but the
	// CREATE TABLE forces a connection that cannot open a directory as a DB file,
	// so Open fails at bootstrap (the boot-fatal path) and closes the handle.
	_, err := sqlite.Open(t.TempDir())
	if err == nil {
		t.Fatal("Open on a directory path returned nil error, want a bootstrap error")
	}
}

// TestOperationsAfterCloseError confirms the store surfaces errors (never panics
// or silently no-ops) once closed — the read/write paths' error returns.
func TestOperationsAfterCloseError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "korvun.db")
	s, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	ctx := context.Background()
	const key = conversation.Key("telegram::closed")

	if _, err := s.LoadRecent(ctx, key, 5); err == nil {
		t.Error("LoadRecent on a closed store returned nil error")
	}
	if _, err := s.Append(ctx, key, conversation.Turn{Role: conversation.RoleUser, Content: "x"}); err == nil {
		t.Error("Append on a closed store returned nil error")
	}
	if _, err := s.AppendTurns(ctx, key, conversation.Turn{Role: conversation.RoleUser, Content: "x"}); err == nil {
		t.Error("AppendTurns on a closed store returned nil error")
	}
}

func TestAppend_AssignsMonotonicSeq(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	const key = conversation.Key("telegram::1")
	for i := 0; i < 3; i++ {
		got, err := s.Append(ctx, key, conversation.Turn{Role: conversation.RoleUser, Content: "hi"})
		if err != nil {
			t.Fatalf("Append: %v", err)
		}
		if got.Seq != i {
			t.Errorf("Seq = %d, want %d", got.Seq, i)
		}
	}
}

func TestLoadRecent(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	const key = conversation.Key("telegram::1")
	for _, c := range []string{"a", "b", "c", "d"} {
		if _, err := s.Append(ctx, key, conversation.Turn{Role: conversation.RoleUser, Content: c}); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	t.Run("returns last n oldest-first", func(t *testing.T) {
		got, err := s.LoadRecent(ctx, key, 2)
		if err != nil {
			t.Fatalf("LoadRecent: %v", err)
		}
		if len(got) != 2 || got[0].Content != "c" || got[1].Content != "d" {
			t.Errorf("got %+v, want [c d]", got)
		}
	})

	t.Run("n larger than history returns all", func(t *testing.T) {
		got, err := s.LoadRecent(ctx, key, 100)
		if err != nil {
			t.Fatalf("LoadRecent: %v", err)
		}
		if len(got) != 4 {
			t.Errorf("len = %d, want 4", len(got))
		}
	})

	t.Run("n<=0 returns no turns", func(t *testing.T) {
		got, err := s.LoadRecent(ctx, key, 0)
		if err != nil {
			t.Fatalf("LoadRecent: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})

	t.Run("unknown key returns empty, no error", func(t *testing.T) {
		got, err := s.LoadRecent(ctx, conversation.Key("nope"), 5)
		if err != nil {
			t.Fatalf("LoadRecent: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})
}

func TestAppendTurns(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	const key = conversation.Key("telegram::pair")

	got, err := s.AppendTurns(ctx, key,
		conversation.Turn{Role: conversation.RoleUser, Content: "q"},
		conversation.Turn{Role: conversation.RoleAssistant, Content: "a"},
	)
	if err != nil {
		t.Fatalf("AppendTurns: %v", err)
	}
	if len(got) != 2 || got[0].Seq != 0 || got[1].Seq != 1 {
		t.Fatalf("returned turns = %+v, want two with Seq 0,1", got)
	}

	got2, err := s.AppendTurns(ctx, key,
		conversation.Turn{Role: conversation.RoleUser, Content: "q2"},
		conversation.Turn{Role: conversation.RoleAssistant, Content: "a2"},
	)
	if err != nil {
		t.Fatalf("AppendTurns: %v", err)
	}
	if got2[0].Seq != 2 || got2[1].Seq != 3 {
		t.Fatalf("second group Seqs = %d,%d, want 2,3", got2[0].Seq, got2[1].Seq)
	}

	t.Run("empty group is a no-op", func(t *testing.T) {
		out, err := s.AppendTurns(ctx, key)
		if err != nil || out != nil {
			t.Fatalf("AppendTurns() with no turns = (%v, %v), want (nil, nil)", out, err)
		}
	})
}

// TestAppendTurns_ErrorPathWritesNothing exercises the atomicity/error path that
// underwrites crash-consistency (ADR-0019 §3): if the group's transaction does
// not commit, it must leave ZERO rows — never a half-written pair. A pre-cancelled
// context fails BeginTx before any insert; the observable contract is the same as
// a crash mid-group: nothing persisted.
func TestAppendTurns_ErrorPathWritesNothing(t *testing.T) {
	s := openTemp(t)
	const key = conversation.Key("telegram::rollback")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	_, err := s.AppendTurns(ctx, key,
		conversation.Turn{Role: conversation.RoleUser, Content: "q"},
		conversation.Turn{Role: conversation.RoleAssistant, Content: "a"},
	)
	if err == nil {
		t.Fatal("AppendTurns with a cancelled context returned nil error, want an error")
	}
	got, err := s.LoadRecent(context.Background(), key, 10)
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("error path left %d rows, want 0 (partial group persisted)", len(got))
	}
}

// TestDurability_ReopenPersists is the property MemStore cannot have: turns
// written, the store closed, then REOPENED at the same path must still be there
// (ADR-0019: durable memory across "restarts").
func TestDurability_ReopenPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "korvun.db")
	ctx := context.Background()
	const key = conversation.Key("telegram::durable")

	s1, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open #1: %v", err)
	}
	if _, err := s1.AppendTurns(ctx, key,
		conversation.Turn{Role: conversation.RoleUser, Content: "remember me", Timestamp: time.Now()},
		conversation.Turn{Role: conversation.RoleAssistant, Content: "i will", Timestamp: time.Now()},
	); err != nil {
		t.Fatalf("AppendTurns: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close #1: %v", err)
	}

	s2, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open #2: %v", err)
	}
	defer func() { _ = s2.Close() }()

	got, err := s2.LoadRecent(ctx, key, 10)
	if err != nil {
		t.Fatalf("LoadRecent after reopen: %v", err)
	}
	if len(got) != 2 || got[0].Content != "remember me" || got[1].Content != "i will" {
		t.Fatalf("after reopen got %+v, want the two persisted turns", got)
	}
	// A new group continues the sequence from the persisted MAX(seq).
	cont, err := s2.Append(ctx, key, conversation.Turn{Role: conversation.RoleUser, Content: "again"})
	if err != nil {
		t.Fatalf("Append after reopen: %v", err)
	}
	if cont.Seq != 2 {
		t.Fatalf("Seq after reopen = %d, want 2 (continues persisted history)", cont.Seq)
	}
}

// TestTimestamp_ZeroRoundTrips guards the round-trip parity with MemStore: a
// zero Turn.Timestamp must read back as zero, NOT as the ~1754 garbage that
// UnixNano() of the year-1 zero value would produce.
func TestTimestamp_ZeroRoundTrips(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	const key = conversation.Key("telegram::zerots")
	if _, err := s.Append(ctx, key, conversation.Turn{Role: conversation.RoleUser, Content: "z"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, err := s.LoadRecent(ctx, key, 1)
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	if !got[0].Timestamp.IsZero() {
		t.Fatalf("zero Timestamp round-tripped to %v, want zero (IsZero)", got[0].Timestamp)
	}
}

// TestOpen_PathWithQueryCharIsRobust proves the DSN is built so a path
// containing a URI-significant character ('?') does not swallow the _pragma
// query: the store must function (WAL applied, writes persist across reopen) and
// the DB file must live at the intended path.
func TestOpen_PathWithQueryCharIsRobust(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("'?' is an illegal filename char on Windows (SQLITE_CANTOPEN); " +
			"DSN construction for '?' paths is covered portably by dsn_test.go")
	}
	dir := t.TempDir()
	weird := filepath.Join(dir, "weird?name.db")
	ctx := context.Background()
	const key = conversation.Key("telegram::weird")

	s1, err := sqlite.Open(weird)
	if err != nil {
		t.Fatalf("Open with '?' in path: %v", err)
	}
	if _, err := s1.Append(ctx, key, conversation.Turn{Role: conversation.RoleUser, Content: "ok"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(weird); err != nil {
		t.Fatalf("DB file not at intended path %q: %v (DSN mis-routed it)", weird, err)
	}
	// Reopen the same path and confirm the write persisted (proves WAL/path correct).
	s2, err := sqlite.Open(weird)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = s2.Close() }()
	got, err := s2.LoadRecent(ctx, key, 5)
	if err != nil {
		t.Fatalf("LoadRecent after reopen: %v", err)
	}
	if len(got) != 1 || got[0].Content != "ok" {
		t.Fatalf("after reopen got %+v, want the persisted turn", got)
	}
}

func TestTimestampRoundTrip(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	const key = conversation.Key("telegram::ts")
	// A timezone-bearing instant with sub-second precision; the store keeps UTC.
	ts := time.Date(2026, 6, 21, 15, 4, 5, 123456789, time.FixedZone("x", 2*3600))

	if _, err := s.Append(ctx, key, conversation.Turn{Role: conversation.RoleUser, Content: "t", Timestamp: ts}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, err := s.LoadRecent(ctx, key, 1)
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	if !got[0].Timestamp.Equal(ts) {
		t.Fatalf("Timestamp = %v, want the same instant as %v", got[0].Timestamp, ts)
	}
}

// TestConcurrentAppendSameKey is the load-bearing contract test (ADR-0018 §7),
// re-run against the durable store. With db.SetMaxOpenConns(1) the single writer
// serializes all writes, so it must pass cleanly with zero SQLITE_BUSY: exactly
// N turns, Seq the contiguous 0..N-1, positional, unique.
//
// Run with: go test -race -count=10 ./internal/conversation/sqlite/
func TestConcurrentAppendSameKey(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	const key = conversation.Key("telegram::race")
	const n = 100

	var wg sync.WaitGroup
	wg.Add(n)
	returnedSeq := make([]int, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			got, err := s.Append(ctx, key, conversation.Turn{Role: conversation.RoleUser, Content: "x"})
			if err != nil {
				t.Errorf("Append: %v", err)
				return
			}
			returnedSeq[i] = got.Seq
		}(i)
	}
	wg.Wait()

	turns, err := s.LoadRecent(ctx, key, n*2)
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	if len(turns) != n {
		t.Fatalf("got %d turns, want %d (lost writes under concurrency)", len(turns), n)
	}
	for i, tr := range turns {
		if tr.Seq != i {
			t.Fatalf("turns[%d].Seq = %d, want %d (Seq must equal history index)", i, tr.Seq, i)
		}
	}
	seen := make([]bool, n)
	for _, tr := range turns {
		if tr.Seq < 0 || tr.Seq >= n {
			t.Fatalf("Seq %d out of range [0,%d)", tr.Seq, n)
		}
		if seen[tr.Seq] {
			t.Fatalf("duplicate Seq %d", tr.Seq)
		}
		seen[tr.Seq] = true
	}
	for i, ok := range seen {
		if !ok {
			t.Fatalf("missing Seq %d (gap in sequence)", i)
		}
	}
	seenRet := make([]bool, n)
	for _, sq := range returnedSeq {
		if sq < 0 || sq >= n || seenRet[sq] {
			t.Fatalf("returned Seq %d is out of range or duplicated", sq)
		}
		seenRet[sq] = true
	}
}

// TestConcurrentAppendTurnsSameKey re-runs the F3 pair-contiguity proof against
// the durable store: N goroutines each AppendTurns(user, assistant) to the SAME
// key. Each pair must stay contiguous, ordered, and identity-matched.
//
// Run with: go test -race -count=10 ./internal/conversation/sqlite/
func TestConcurrentAppendTurnsSameKey(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	const key = conversation.Key("telegram::pairs-race")
	const n = 100

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			if _, err := s.AppendTurns(ctx, key,
				conversation.Turn{Role: conversation.RoleUser, Content: fmt.Sprintf("u%d", i)},
				conversation.Turn{Role: conversation.RoleAssistant, Content: fmt.Sprintf("a%d", i)},
			); err != nil {
				t.Errorf("AppendTurns: %v", err)
			}
		}(i)
	}
	wg.Wait()

	turns, err := s.LoadRecent(ctx, key, n*4)
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	if len(turns) != 2*n {
		t.Fatalf("got %d turns, want %d", len(turns), 2*n)
	}
	seenMsg := make(map[string]bool, n)
	for j := 0; j < n; j++ {
		u, a := turns[2*j], turns[2*j+1]
		if u.Role != conversation.RoleUser || a.Role != conversation.RoleAssistant {
			t.Fatalf("pair at %d split or reordered: roles %q,%q (want user,assistant)", 2*j, u.Role, a.Role)
		}
		if u.Seq != 2*j || a.Seq != 2*j+1 {
			t.Fatalf("pair at %d Seqs = %d,%d, want %d,%d", 2*j, u.Seq, a.Seq, 2*j, 2*j+1)
		}
		uid := strings.TrimPrefix(u.Content, "u")
		aid := strings.TrimPrefix(a.Content, "a")
		if uid != aid {
			t.Fatalf("pair at %d crossed messages: user %q with assistant %q", 2*j, u.Content, a.Content)
		}
		if seenMsg[uid] {
			t.Fatalf("message %q persisted twice", uid)
		}
		seenMsg[uid] = true
	}
	if len(seenMsg) != n {
		t.Fatalf("saw %d distinct messages, want %d", len(seenMsg), n)
	}
}

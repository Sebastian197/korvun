// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// SP1 unhappy paths: the sessionful surface must FAIL loudly (wrapped
// errors), never silently return empty data, when the database is gone —
// and the zero-timestamp sentinel must round-trip through every new read.
package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/conversation/sqlite"
)

func TestSessionSurface_ErrorsOnClosedStore(t *testing.T) {
	s, _ := openStore(t)
	mustAppend(t, s, conversation.Key("tg::1"), conversation.RoleUser, "hola", at(1))
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	ctx := context.Background()

	if _, err := s.NewSession(ctx, "tg::1"); err == nil {
		t.Fatal("NewSession on closed store: want error, got nil")
	}
	if _, err := s.ListConversations(ctx, 5); err == nil {
		t.Fatal("ListConversations on closed store: want error, got nil")
	}
	if _, err := s.ListSessions(ctx, "tg::1"); err == nil {
		t.Fatal("ListSessions on closed store: want error, got nil")
	}
	if _, err := s.LoadSession(ctx, "tg::1", 1); err == nil {
		t.Fatal("LoadSession on closed store: want error, got nil")
	}
}

func TestSessionReads_ZeroTimestampSentinelRoundTrips(t *testing.T) {
	// A zero Turn.Timestamp stores as the 0 sentinel and must come back as
	// the zero value through the NEW reads too (the LoadRecent guarantee,
	// extended).
	s, _ := openStore(t)
	ctx := context.Background()
	key := conversation.Key("tg::1")
	if _, err := s.Append(ctx, key, conversation.Turn{Role: conversation.RoleUser, Content: "sin reloj"}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	turns, err := s.LoadSession(ctx, key, 1)
	if err != nil || len(turns) != 1 {
		t.Fatalf("LoadSession = (%v, %v), want 1 turn", turns, err)
	}
	if !turns[0].Timestamp.IsZero() {
		t.Fatalf("zero timestamp came back as %v, want zero", turns[0].Timestamp)
	}

	sessions, err := s.ListSessions(ctx, key)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("ListSessions = (%v, %v), want 1 session", sessions, err)
	}
	if !sessions[0].First.IsZero() || !sessions[0].Last.IsZero() {
		t.Fatalf("zero-sentinel session times = %+v, want zero values", sessions[0])
	}

	convs, err := s.ListConversations(ctx, 5)
	if err != nil || len(convs) != 1 {
		t.Fatalf("ListConversations = (%v, %v), want 1 row", convs, err)
	}
	if !convs[0].LastActivity.IsZero() {
		t.Fatalf("zero-sentinel LastActivity = %v, want zero", convs[0].LastActivity)
	}
}

func TestListConversations_NonPositiveLimitReturnsNothing(t *testing.T) {
	s, _ := openStore(t)
	mustAppend(t, s, conversation.Key("tg::1"), conversation.RoleUser, "hola", at(1))
	for _, limit := range []int{0, -3} {
		got, err := s.ListConversations(context.Background(), limit)
		if err != nil || len(got) != 0 {
			t.Fatalf("ListConversations(%d) = (%v, %v), want (empty, nil)", limit, got, err)
		}
	}
}

// dropTable removes a table behind the store's back (a SECOND connection,
// test-only DDL) so the session queries fail mid-flight — the only practical
// way to reach their inner error wraps without a fault-injection driver.
func dropTable(t *testing.T, path, table string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("second connection: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`DROP TABLE ` + table); err != nil {
		t.Fatalf("drop %s: %v", table, err)
	}
}

func TestNewSession_SurfacesQueryFailures(t *testing.T) {
	// Active-session resolution fails when the sessions table is gone…
	s, path := openStore(t)
	mustAppend(t, s, conversation.Key("tg::1"), conversation.RoleUser, "hola", at(1))
	dropTable(t, path, "sessions")
	if _, err := s.NewSession(context.Background(), "tg::1"); err == nil {
		t.Fatal("NewSession without sessions table: want error, got nil")
	}

	// …and the empty-active count fails when the turns table is gone.
	s2, path2 := openStore(t)
	mustAppend(t, s2, conversation.Key("tg::1"), conversation.RoleUser, "hola", at(1))
	dropTable(t, path2, "turns")
	if _, err := s2.NewSession(context.Background(), "tg::1"); err == nil {
		t.Fatal("NewSession without turns table: want error, got nil")
	}
}

func TestOpen_FailsCleanlyWhenMigrationCannotRun(t *testing.T) {
	// A leftover turns_v1 table (e.g. a crash mid-migration on an ancient
	// copy) makes the RENAME collide: Open must fail with a wrapped
	// migration error — boot-fatal, never a silently half-migrated store.
	dir := t.TempDir()
	path := filepath.Join(dir, "korvun.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("fixture open: %v", err)
	}
	if _, err := db.Exec(v1Schema); err != nil {
		t.Fatalf("fixture schema: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE turns_v1 (x INTEGER)`); err != nil {
		t.Fatalf("fixture collision table: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("fixture close: %v", err)
	}

	if _, err := sqlite.Open(path); err == nil {
		t.Fatal("Open over unmigratable database: want error, got nil")
	} else if !strings.Contains(err.Error(), "migrate") {
		t.Fatalf("Open error = %v, want the migration wrap", err)
	}
}

func TestNewSession_EmptyActiveOnFreshKeyStaysIdempotentAcrossReopen(t *testing.T) {
	// The empty-active idempotence survives a restart: sessions are durable
	// rows, not process memory.
	s, path := openStore(t)
	ctx := context.Background()
	if id, err := s.NewSession(ctx, "tg::1"); err != nil || id != 1 {
		t.Fatalf("NewSession = (%d, %v), want (1, nil)", id, err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	s2, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = s2.Close() }()
	if id, err := s2.NewSession(ctx, "tg::1"); err != nil || id != 1 {
		t.Fatalf("NewSession after reopen = (%d, %v), want (1, nil) — empty session lost or stacked", id, err)
	}
}

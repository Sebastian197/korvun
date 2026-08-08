// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// SP1 red suite, sqlite side (operator-console spec 2026-08-08): the
// sessionful contract on the durable store PLUS the schema migration — a
// pre-migration fixture database (the 2026-08 v1 schema: turns keyed by
// (key, seq), no sessions) must come out with every turn intact, in order,
// as session 1 (active) of its key. AS-12.
package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/conversation/sqlite"
	_ "modernc.org/sqlite"
)

func at(sec int) time.Time { return time.Unix(int64(sec), 0).UTC() }

func openStore(t *testing.T) (*sqlite.SqliteStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "korvun.db")
	s, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

func mustAppend(t *testing.T, s *sqlite.SqliteStore, key conversation.Key, role conversation.Role, content string, ts time.Time) {
	t.Helper()
	if _, err := s.Append(context.Background(), key, conversation.Turn{Role: role, Content: content, Timestamp: ts}); err != nil {
		t.Fatalf("Append(%s, %q): %v", key, content, err)
	}
}

func contents(turns []conversation.Turn) []string {
	out := make([]string, len(turns))
	for i, tr := range turns {
		out[i] = tr.Content
	}
	return out
}

// --- The migration fixture (AS-12) -----------------------------------------

// v1Schema is the exact pre-session schema shipped 2026-08 (ADR-0019 era).
const v1Schema = `
CREATE TABLE IF NOT EXISTS turns (
    key     TEXT    NOT NULL,
    seq     INTEGER NOT NULL,
    role    TEXT    NOT NULL,
    content TEXT    NOT NULL,
    ts      INTEGER NOT NULL,
    PRIMARY KEY (key, seq)
) WITHOUT ROWID;`

func TestOpen_MigratesV1FixtureToSessionOne(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "korvun.db")

	// Build the fixture with the OLD schema, straight through database/sql —
	// no store code involved, so this is a faithful pre-upgrade database.
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("fixture open: %v", err)
	}
	if _, err := db.Exec(v1Schema); err != nil {
		t.Fatalf("fixture schema: %v", err)
	}
	fixture := []struct {
		key     string
		seq     int
		role    string
		content string
		ts      int64
	}{
		{"tg::1", 0, "user", "hola", at(1).UnixNano()},
		{"tg::1", 1, "assistant", "¿en qué te ayudo?", at(2).UnixNano()},
		{"tg::1", 2, "user", "en nada, gracias", at(3).UnixNano()},
		{"dc::9", 0, "user", "ping", at(5).UnixNano()},
	}
	for _, r := range fixture {
		if _, err := db.Exec(
			`INSERT INTO turns (key, seq, role, content, ts) VALUES (?, ?, ?, ?, ?)`,
			r.key, r.seq, r.role, r.content, r.ts,
		); err != nil {
			t.Fatalf("fixture insert %+v: %v", r, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("fixture close: %v", err)
	}

	// Open through the store: the migration must run here, once.
	s, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open over v1 fixture: %v", err)
	}
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	// Every turn intact, in order, as session 1.
	got, err := s.LoadSession(ctx, conversation.Key("tg::1"), 1)
	if err != nil {
		t.Fatalf("LoadSession(tg::1, 1): %v", err)
	}
	want := []string{"hola", "¿en qué te ayudo?", "en nada, gracias"}
	if len(got) != len(want) {
		t.Fatalf("migrated turns = %v, want %v (turn lost)", contents(got), want)
	}
	for i := range want {
		if got[i].Content != want[i] {
			t.Fatalf("migrated order broken at %d: %v, want %v", i, contents(got), want)
		}
	}
	if got[0].Role != conversation.RoleUser || got[1].Role != conversation.RoleAssistant {
		t.Fatalf("migrated roles broken: %+v", got)
	}

	// Session 1 is the ACTIVE session: LoadRecent still sees the history
	// (an upgrade must not cut anyone's context)…
	recent, err := s.LoadRecent(ctx, conversation.Key("tg::1"), 10)
	if err != nil {
		t.Fatalf("LoadRecent after migration: %v", err)
	}
	if len(recent) != 3 {
		t.Fatalf("LoadRecent after migration = %v, want the 3 migrated turns", contents(recent))
	}
	// …and appending continues the same session with the next seq.
	appended, err := s.Append(ctx, conversation.Key("tg::1"),
		conversation.Turn{Role: conversation.RoleUser, Content: "sigo aquí", Timestamp: at(9)})
	if err != nil {
		t.Fatalf("Append after migration: %v", err)
	}
	if appended.Seq != 3 {
		t.Fatalf("post-migration Seq = %d, want 3 (continues session 1)", appended.Seq)
	}

	// Both keys enumerate with one session each.
	convs, err := s.ListConversations(ctx, 10)
	if err != nil {
		t.Fatalf("ListConversations after migration: %v", err)
	}
	if len(convs) != 2 {
		t.Fatalf("ListConversations = %+v, want both fixture keys", convs)
	}
	for _, c := range convs {
		if c.ActiveSession != 1 || c.SessionCount != 1 {
			t.Fatalf("migrated conversation %+v, want active=1 count=1", c)
		}
	}

	// Reopening a MIGRATED database must be a clean no-op (idempotent).
	if err := s.Close(); err != nil {
		t.Fatalf("close before reopen: %v", err)
	}
	s2, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("reopen migrated db: %v", err)
	}
	defer func() { _ = s2.Close() }()
	again, err := s2.LoadSession(ctx, conversation.Key("tg::1"), 1)
	if err != nil || len(again) != 4 {
		t.Fatalf("after reopen LoadSession = (%v, %v), want the 4 turns", contents(again), err)
	}
}

// --- The sessionful contract on the durable store ---------------------------

func TestSqlite_LoadRecentScopedToActiveSession(t *testing.T) {
	s, _ := openStore(t)
	ctx := context.Background()
	key := conversation.Key("tg::1")
	mustAppend(t, s, key, conversation.RoleUser, "vieja", at(1))
	if _, err := s.NewSession(ctx, key); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	mustAppend(t, s, key, conversation.RoleUser, "nueva", at(2))

	got, err := s.LoadRecent(ctx, key, 10)
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	if len(got) != 1 || got[0].Content != "nueva" {
		t.Fatalf("LoadRecent across reset = %v, want [nueva] (old session leaked)", contents(got))
	}
}

func TestSqlite_NewSessionSemantics(t *testing.T) {
	s, _ := openStore(t)
	ctx := context.Background()
	key := conversation.Key("tg::1")

	id, err := s.NewSession(ctx, key)
	if err != nil || id != 1 {
		t.Fatalf("fresh key NewSession = (%d, %v), want (1, nil)", id, err)
	}
	// Idempotent while the active session is empty.
	again, err := s.NewSession(ctx, key)
	if err != nil || again != 1 {
		t.Fatalf("empty-active NewSession = (%d, %v), want (1, nil)", again, err)
	}
	mustAppend(t, s, key, conversation.RoleUser, "hola", at(1))
	next, err := s.NewSession(ctx, key)
	if err != nil || next != 2 {
		t.Fatalf("NewSession after turns = (%d, %v), want (2, nil)", next, err)
	}
}

func TestSqlite_SessionNavigation(t *testing.T) {
	s, _ := openStore(t)
	ctx := context.Background()
	key := conversation.Key("tg::1")
	mustAppend(t, s, key, conversation.RoleUser, "s1-a", at(1))
	mustAppend(t, s, key, conversation.RoleAssistant, "s1-b", at(2))
	if _, err := s.NewSession(ctx, key); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	mustAppend(t, s, key, conversation.RoleOperator, "s2-a", at(3))

	sessions, err := s.ListSessions(ctx, key)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 2 ||
		sessions[0].ID != 1 || sessions[0].TurnCount != 2 ||
		!sessions[0].First.Equal(at(1)) || !sessions[0].Last.Equal(at(2)) ||
		sessions[1].ID != 2 || sessions[1].TurnCount != 1 {
		t.Fatalf("ListSessions = %+v, want [{1,2,@1,@2},{2,1,…}]", sessions)
	}

	old, err := s.LoadSession(ctx, key, 1)
	if err != nil || len(old) != 2 || old[0].Content != "s1-a" || old[1].Content != "s1-b" {
		t.Fatalf("LoadSession(1) = (%v, %v), want [s1-a s1-b]", contents(old), err)
	}

	convs, err := s.ListConversations(ctx, 10)
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(convs) != 1 ||
		convs[0].ActiveSession != 2 || convs[0].SessionCount != 2 ||
		convs[0].LastRole != conversation.RoleOperator || !convs[0].LastActivity.Equal(at(3)) {
		t.Fatalf("ListConversations = %+v, want active=2 count=2 operator @3", convs)
	}
}

func TestSqlite_OperatorRoleRoundTrips(t *testing.T) {
	s, _ := openStore(t)
	key := conversation.Key("tg::1")
	mustAppend(t, s, key, conversation.RoleOperator, "soy humano", at(1))
	got, err := s.LoadRecent(context.Background(), key, 1)
	if err != nil || len(got) != 1 || got[0].Role != conversation.RoleOperator {
		t.Fatalf("operator turn = (%+v, %v), want role %q", got, err, conversation.RoleOperator)
	}
}

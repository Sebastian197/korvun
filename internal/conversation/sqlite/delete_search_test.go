// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Deletion + search on the durable store (FR-DEL-1 / FR-SEARCH / AS-13/14):
// the wipe is REAL ON DISK (row counts hit zero), the active session is
// protected, and the LIKE escaping makes %/_ match literally.
package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Sebastian197/korvun/internal/conversation"
	_ "modernc.org/sqlite"
)

func rowCounts(t *testing.T, path, key string) (turns, sessions int) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		t.Fatalf("ro open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := db.QueryRow(`SELECT COUNT(*) FROM turns WHERE key = ?`, key).Scan(&turns); err != nil {
		t.Fatalf("count turns: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE key = ?`, key).Scan(&sessions); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	return turns, sessions
}

func TestSqlite_DeleteConversation_RealOnDisk(t *testing.T) {
	s, path := openStore(t)
	ctx := context.Background()
	key := conversation.Key("tg::1")
	mustAppend(t, s, key, conversation.RoleUser, "s1", at(1))
	if _, err := s.NewSession(ctx, key); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	mustAppend(t, s, key, conversation.RoleOperator, "s2", at(2))
	mustAppend(t, s, conversation.Key("tg::other"), conversation.RoleUser, "queda", at(3))

	if err := s.DeleteConversation(ctx, key); err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}
	turns, sessions := rowCounts(t, path, string(key))
	if turns != 0 || sessions != 0 {
		t.Fatalf("disk rows after wipe: turns=%d sessions=%d, want 0/0 (really gone)", turns, sessions)
	}
	if oTurns, oSessions := rowCounts(t, path, "tg::other"); oTurns != 1 || oSessions != 1 {
		t.Fatalf("untouched key affected: turns=%d sessions=%d", oTurns, oSessions)
	}
	// Rebirth clean at session 1 (AS-13).
	mustAppend(t, s, key, conversation.RoleUser, "renacida", at(9))
	ss, _ := s.ListSessions(ctx, key)
	if len(ss) != 1 || ss[0].ID != 1 || ss[0].TurnCount != 1 {
		t.Fatalf("reborn = %+v, want clean session 1", ss)
	}
}

func TestSqlite_DeleteSession_ActiveProtected(t *testing.T) {
	s, path := openStore(t)
	ctx := context.Background()
	key := conversation.Key("tg::1")
	mustAppend(t, s, key, conversation.RoleUser, "s1", at(1))
	if _, err := s.NewSession(ctx, key); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	mustAppend(t, s, key, conversation.RoleUser, "s2", at(2))

	if err := s.DeleteSession(ctx, key, 2); !errors.Is(err, conversation.ErrActiveSession) {
		t.Fatalf("delete active err = %v, want ErrActiveSession", err)
	}
	if err := s.DeleteSession(ctx, key, 1); err != nil {
		t.Fatalf("DeleteSession(1): %v", err)
	}
	turns, sessions := rowCounts(t, path, string(key))
	if turns != 1 || sessions != 1 {
		t.Fatalf("after archive wipe: turns=%d sessions=%d, want 1/1", turns, sessions)
	}
	if err := s.DeleteSession(ctx, key, 9); err != nil {
		t.Fatalf("DeleteSession(unknown): %v", err)
	}
}

func TestSqlite_SearchTurns_EscapesLikeMetacharacters(t *testing.T) {
	s, _ := openStore(t)
	ctx := context.Background()
	key := conversation.Key("tg::1")
	mustAppend(t, s, key, conversation.RoleUser, "descuento del 100% garantizado", at(1))
	mustAppend(t, s, key, conversation.RoleUser, "un_guion_bajo", at(2))
	mustAppend(t, s, key, conversation.RoleUser, "cien por cien normal", at(3))

	hits, err := s.SearchTurns(ctx, "100%", 10)
	if err != nil {
		t.Fatalf("SearchTurns: %v", err)
	}
	if len(hits) != 1 || hits[0].Seq != 0 {
		t.Fatalf("%% not escaped: hits = %+v, want just the literal 100%% turn", hits)
	}
	hits, err = s.SearchTurns(ctx, "un_guion", 10)
	if err != nil || len(hits) != 1 || hits[0].Seq != 1 {
		t.Fatalf("_ not escaped: hits = (%+v, %v)", hits, err)
	}
	// Case-insensitive, newest first, addressable.
	hits, err = s.SearchTurns(ctx, "CIEN", 10)
	if err != nil || len(hits) != 1 || hits[0].Session != 1 || hits[0].Seq != 2 {
		t.Fatalf("case/addressing: hits = (%+v, %v)", hits, err)
	}
}

func TestSqlite_ListConversations_TurnCountTotal(t *testing.T) {
	s, _ := openStore(t)
	ctx := context.Background()
	key := conversation.Key("tg::1")
	mustAppend(t, s, key, conversation.RoleUser, "a", at(1))
	mustAppend(t, s, key, conversation.RoleAssistant, "b", at(2))
	if _, err := s.NewSession(ctx, key); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	mustAppend(t, s, key, conversation.RoleUser, "c", at(3))
	convs, err := s.ListConversations(ctx, 10)
	if err != nil || len(convs) != 1 {
		t.Fatalf("ListConversations = (%+v, %v)", convs, err)
	}
	if convs[0].TurnCount != 3 {
		t.Fatalf("TurnCount = %d, want 3 across sessions", convs[0].TurnCount)
	}
}

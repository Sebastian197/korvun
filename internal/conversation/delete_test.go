// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Deletion red suite (operator-console spec FR-DEL-1 / AS-13 / AS-14), on
// MemStore; the sqlite package runs the same contract plus the on-disk
// verification.
package conversation

import (
	"context"
	"errors"
	"testing"
	"time"
)

func seedTwoSessions(t *testing.T, s SessionStore, key Key) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.Append(ctx, key, Turn{Role: RoleUser, Content: "s1", Timestamp: time.Unix(1, 0)}); err != nil {
		t.Fatalf("seed s1: %v", err)
	}
	if _, err := s.NewSession(ctx, key); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if _, err := s.Append(ctx, key, Turn{Role: RoleOperator, Content: "s2", Timestamp: time.Unix(2, 0)}); err != nil {
		t.Fatalf("seed s2: %v", err)
	}
}

func TestDeleteConversation_WipesEverything(t *testing.T) {
	s := NewMemStore()
	key := Key("tg::1")
	other := Key("tg::other")
	seedTwoSessions(t, s, key)
	seedTwoSessions(t, s, other)
	ctx := context.Background()

	if err := s.DeleteConversation(ctx, key); err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}
	if sessions, _ := s.ListSessions(ctx, key); len(sessions) != 0 {
		t.Fatalf("sessions after wipe = %+v, want none", sessions)
	}
	if turns, _ := s.LoadRecent(ctx, key, 10); len(turns) != 0 {
		t.Fatalf("turns after wipe = %+v, want none", turns)
	}
	if convs, _ := s.ListConversations(ctx, 10); len(convs) != 1 || convs[0].Key != other {
		t.Fatalf("inbox after wipe = %+v, want only the untouched key", convs)
	}
	// Unknown key: a no-op, not an error.
	if err := s.DeleteConversation(ctx, Key("tg::ghost")); err != nil {
		t.Fatalf("DeleteConversation(unknown): %v", err)
	}

	// AS-13's rebirth: a new turn recreates the conversation clean at
	// session 1.
	if _, err := s.Append(ctx, key, Turn{Role: RoleUser, Content: "renacida", Timestamp: time.Unix(9, 0)}); err != nil {
		t.Fatalf("Append after wipe: %v", err)
	}
	sessions, _ := s.ListSessions(ctx, key)
	if len(sessions) != 1 || sessions[0].ID != 1 || sessions[0].TurnCount != 1 {
		t.Fatalf("reborn sessions = %+v, want a clean session 1 with 1 turn", sessions)
	}
}

func TestDeleteSession_ArchivedOnlyActiveProtected(t *testing.T) {
	s := NewMemStore()
	key := Key("tg::1")
	seedTwoSessions(t, s, key)
	ctx := context.Background()

	// The ACTIVE session (2) is protected.
	err := s.DeleteSession(ctx, key, 2)
	if !errors.Is(err, ErrActiveSession) {
		t.Fatalf("DeleteSession(active) err = %v, want ErrActiveSession", err)
	}
	if sessions, _ := s.ListSessions(ctx, key); len(sessions) != 2 {
		t.Fatalf("sessions after protected attempt = %+v, want both intact", sessions)
	}

	// The archived session (1) goes; 2 stays whole.
	if err := s.DeleteSession(ctx, key, 1); err != nil {
		t.Fatalf("DeleteSession(1): %v", err)
	}
	sessions, _ := s.ListSessions(ctx, key)
	if len(sessions) != 1 || sessions[0].ID != 2 || sessions[0].TurnCount != 1 {
		t.Fatalf("sessions after archive wipe = %+v, want only session 2", sessions)
	}
	if turns, _ := s.LoadSession(ctx, key, 1); len(turns) != 0 {
		t.Fatalf("archived turns survived the wipe: %+v", turns)
	}
	// Unknown session: a no-op.
	if err := s.DeleteSession(ctx, key, 9); err != nil {
		t.Fatalf("DeleteSession(unknown): %v", err)
	}
}

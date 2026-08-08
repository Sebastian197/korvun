// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// SP1 red suite (operator-console spec 2026-08-08, FR-SESS-1/2/6 +
// FR-STORE-1/2): the sessionful store contract, exercised on MemStore. The
// sqlite implementation runs the same behaviors in its own package test
// (plus the schema migration fixture).
package conversation

import (
	"context"
	"testing"
	"time"
)

func at(sec int) time.Time { return time.Unix(int64(sec), 0).UTC() }

func mustAppend(t *testing.T, s SessionStore, key Key, role Role, content string, ts time.Time) {
	t.Helper()
	if _, err := s.Append(context.Background(), key, Turn{Role: role, Content: content, Timestamp: ts}); err != nil {
		t.Fatalf("Append(%s, %q): %v", key, content, err)
	}
}

func contents(turns []Turn) []string {
	out := make([]string, len(turns))
	for i, tr := range turns {
		out[i] = tr.Content
	}
	return out
}

func TestNewSession_FirstKeyStartsAtOne(t *testing.T) {
	s := NewMemStore()
	id, err := s.NewSession(context.Background(), Key("tg::1"))
	if err != nil {
		t.Fatalf("NewSession on fresh key: %v", err)
	}
	if id != 1 {
		t.Fatalf("fresh key NewSession = %d, want 1", id)
	}
}

func TestNewSession_IncrementsAfterTurns(t *testing.T) {
	s := NewMemStore()
	key := Key("tg::1")
	mustAppend(t, s, key, RoleUser, "hola", at(1))
	id, err := s.NewSession(context.Background(), key)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if id != 2 {
		t.Fatalf("NewSession after turns = %d, want 2", id)
	}
}

func TestNewSession_IdempotentOnEmptyActive(t *testing.T) {
	s := NewMemStore()
	key := Key("tg::1")
	mustAppend(t, s, key, RoleUser, "hola", at(1))
	first, err := s.NewSession(context.Background(), key)
	if err != nil {
		t.Fatalf("first NewSession: %v", err)
	}
	again, err := s.NewSession(context.Background(), key)
	if err != nil {
		t.Fatalf("second NewSession: %v", err)
	}
	if again != first {
		t.Fatalf("NewSession on empty active stacked a session: got %d, want %d (idempotent)", again, first)
	}
}

func TestLoadRecent_ScopedToActiveSession(t *testing.T) {
	// FR-SESS-2: a reset is a hard context cut — LoadRecent never returns
	// turns from a previous session.
	s := NewMemStore()
	key := Key("tg::1")
	mustAppend(t, s, key, RoleUser, "vieja-1", at(1))
	mustAppend(t, s, key, RoleAssistant, "vieja-2", at(2))
	if _, err := s.NewSession(context.Background(), key); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	mustAppend(t, s, key, RoleUser, "nueva-1", at(3))

	got, err := s.LoadRecent(context.Background(), key, 10)
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	want := []string{"nueva-1"}
	if len(got) != len(want) || got[0].Content != want[0] {
		t.Fatalf("LoadRecent across reset = %v, want %v (old session leaked)", contents(got), want)
	}
}

func TestLoadRecent_EmptyActiveSessionReturnsNothing(t *testing.T) {
	s := NewMemStore()
	key := Key("tg::1")
	mustAppend(t, s, key, RoleUser, "vieja", at(1))
	if _, err := s.NewSession(context.Background(), key); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	got, err := s.LoadRecent(context.Background(), key, 10)
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("LoadRecent on fresh session = %v, want empty", contents(got))
	}
}

func TestLoadSession_OldSessionsRemainReadable(t *testing.T) {
	// FR-SESS-6: history is preserved and navigable.
	s := NewMemStore()
	key := Key("tg::1")
	mustAppend(t, s, key, RoleUser, "s1-a", at(1))
	mustAppend(t, s, key, RoleAssistant, "s1-b", at(2))
	if _, err := s.NewSession(context.Background(), key); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	mustAppend(t, s, key, RoleUser, "s2-a", at(3))

	old, err := s.LoadSession(context.Background(), key, 1)
	if err != nil {
		t.Fatalf("LoadSession(1): %v", err)
	}
	if want := []string{"s1-a", "s1-b"}; len(old) != 2 || old[0].Content != want[0] || old[1].Content != want[1] {
		t.Fatalf("LoadSession(1) = %v, want %v", contents(old), want)
	}
	active, err := s.LoadSession(context.Background(), key, 2)
	if err != nil {
		t.Fatalf("LoadSession(2): %v", err)
	}
	if len(active) != 1 || active[0].Content != "s2-a" {
		t.Fatalf("LoadSession(2) = %v, want [s2-a]", contents(active))
	}
	// Unknown session: empty, not an error.
	none, err := s.LoadSession(context.Background(), key, 9)
	if err != nil || len(none) != 0 {
		t.Fatalf("LoadSession(unknown) = (%v, %v), want (empty, nil)", contents(none), err)
	}
}

func TestListSessions_PerKeyOldestFirst(t *testing.T) {
	s := NewMemStore()
	key := Key("tg::1")
	mustAppend(t, s, key, RoleUser, "s1-a", at(1))
	mustAppend(t, s, key, RoleAssistant, "s1-b", at(2))
	if _, err := s.NewSession(context.Background(), key); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	mustAppend(t, s, key, RoleUser, "s2-a", at(5))

	got, err := s.ListSessions(context.Background(), key)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListSessions len = %d, want 2 (%+v)", len(got), got)
	}
	if got[0].ID != 1 || got[0].TurnCount != 2 || !got[0].First.Equal(at(1)) || !got[0].Last.Equal(at(2)) {
		t.Fatalf("session 1 info = %+v, want id=1 turns=2 first=%v last=%v", got[0], at(1), at(2))
	}
	if got[1].ID != 2 || got[1].TurnCount != 1 {
		t.Fatalf("session 2 info = %+v, want id=2 turns=1", got[1])
	}
	// Unknown key: empty, not an error.
	none, err := s.ListSessions(context.Background(), Key("tg::none"))
	if err != nil || len(none) != 0 {
		t.Fatalf("ListSessions(unknown) = (%v, %v), want (empty, nil)", none, err)
	}
}

func TestListConversations_ActiveCountsAndOrder(t *testing.T) {
	// FR-STORE-1: newest-activity first, active session id, session count,
	// last-turn role and timestamp.
	s := NewMemStore()
	older := Key("tg::older")
	newer := Key("dc::newer")
	mustAppend(t, s, older, RoleUser, "a", at(10))
	if _, err := s.NewSession(context.Background(), older); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	mustAppend(t, s, older, RoleAssistant, "b", at(20))
	mustAppend(t, s, newer, RoleOperator, "c", at(30))

	got, err := s.ListConversations(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListConversations len = %d, want 2 (%+v)", len(got), got)
	}
	if got[0].Key != newer || got[0].ActiveSession != 1 || got[0].SessionCount != 1 ||
		got[0].LastRole != RoleOperator || !got[0].LastActivity.Equal(at(30)) {
		t.Fatalf("row 0 = %+v, want newer key, active 1, count 1, operator @30", got[0])
	}
	if got[1].Key != older || got[1].ActiveSession != 2 || got[1].SessionCount != 2 ||
		got[1].LastRole != RoleAssistant || !got[1].LastActivity.Equal(at(20)) {
		t.Fatalf("row 1 = %+v, want older key, active 2, count 2, assistant @20", got[1])
	}

	// The limit bites and limit <= 0 returns nothing.
	one, err := s.ListConversations(context.Background(), 1)
	if err != nil || len(one) != 1 || one[0].Key != newer {
		t.Fatalf("ListConversations(1) = (%+v, %v), want just the newest", one, err)
	}
	zero, err := s.ListConversations(context.Background(), 0)
	if err != nil || len(zero) != 0 {
		t.Fatalf("ListConversations(0) = (%+v, %v), want (empty, nil)", zero, err)
	}
}

func TestOperatorRole_RoundTripsThroughStore(t *testing.T) {
	// FR-STORE-2: the operator role persists as-is and loads back.
	s := NewMemStore()
	key := Key("tg::1")
	mustAppend(t, s, key, RoleOperator, "soy humano", at(1))
	got, err := s.LoadRecent(context.Background(), key, 1)
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	if len(got) != 1 || got[0].Role != RoleOperator {
		t.Fatalf("operator turn = %+v, want role %q", got, RoleOperator)
	}
}

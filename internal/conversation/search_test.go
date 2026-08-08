// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Search + unread-anchor red suite (operator-console spec FR-SEARCH /
// FR-UNREAD store side), on MemStore; sqlite runs the same contract plus
// the LIKE-escaping cases.
package conversation

import (
	"context"
	"testing"
	"time"
)

func seedSearchable(t *testing.T, s SessionStore) {
	t.Helper()
	ctx := context.Background()
	key := Key("tg::1")
	for i, c := range []string{"la lavadora hace ruido", "prueba el centrifugado", "gracias, arreglado"} {
		if _, err := s.Append(ctx, key, Turn{Role: RoleUser, Content: c, Timestamp: time.Unix(int64(10+i), 0)}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	if _, err := s.NewSession(ctx, key); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if _, err := s.Append(ctx, key, Turn{Role: RoleOperator, Content: "¿la LAVADORA otra vez?", Timestamp: time.Unix(20, 0)}); err != nil {
		t.Fatalf("seed s2: %v", err)
	}
	if _, err := s.Append(ctx, Key("dc::2"), Turn{Role: RoleUser, Content: "otro tema", Timestamp: time.Unix(30, 0)}); err != nil {
		t.Fatalf("seed other: %v", err)
	}
}

func TestSearchTurns_CaseInsensitiveNewestFirstAddressable(t *testing.T) {
	s := NewMemStore()
	seedSearchable(t, s)
	hits, err := s.SearchTurns(context.Background(), "lavadora", 10)
	if err != nil {
		t.Fatalf("SearchTurns: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %d (%+v), want 2 across sessions", len(hits), hits)
	}
	// Newest first, addressable to the exact point.
	if hits[0].Session != 2 || hits[0].Role != RoleOperator || hits[0].Key != Key("tg::1") {
		t.Fatalf("hit 0 = %+v, want the session-2 operator turn", hits[0])
	}
	if hits[1].Session != 1 || hits[1].Seq != 0 {
		t.Fatalf("hit 1 = %+v, want session 1 seq 0", hits[1])
	}
	// The limit bites.
	one, _ := s.SearchTurns(context.Background(), "lavadora", 1)
	if len(one) != 1 || one[0].Session != 2 {
		t.Fatalf("limited hits = %+v, want just the newest", one)
	}
}

func TestSearchTurns_EmptyQueryOrLimitReturnsNothing(t *testing.T) {
	s := NewMemStore()
	seedSearchable(t, s)
	for _, q := range []string{"", "   "} {
		if hits, err := s.SearchTurns(context.Background(), q, 10); err != nil || len(hits) != 0 {
			t.Fatalf("SearchTurns(%q) = (%v, %v), want (empty, nil)", q, hits, err)
		}
	}
	if hits, err := s.SearchTurns(context.Background(), "lavadora", 0); err != nil || len(hits) != 0 {
		t.Fatalf("SearchTurns(limit 0) = (%v, %v), want (empty, nil)", hits, err)
	}
}

func TestListConversations_ReportsTotalTurnCount(t *testing.T) {
	// FR-UNREAD's anchor: the inbox row carries the TOTAL turn count across
	// sessions, so the shell's last-read arithmetic has a stable base.
	s := NewMemStore()
	seedSearchable(t, s)
	convs, err := s.ListConversations(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	for _, c := range convs {
		switch c.Key {
		case Key("tg::1"):
			if c.TurnCount != 4 {
				t.Fatalf("tg::1 TurnCount = %d, want 4 (3 in s1 + 1 in s2)", c.TurnCount)
			}
		case Key("dc::2"):
			if c.TurnCount != 1 {
				t.Fatalf("dc::2 TurnCount = %d, want 1", c.TurnCount)
			}
		}
	}
}

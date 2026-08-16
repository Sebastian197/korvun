// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// SP-A red suite, sqlite side (minimal-memory spec 2026-08-16, FR-STORE-A1):
// the LoadSessionTail contract on the durable store — same behaviors as the
// MemStore suite in internal/conversation (the SP1 molde).
package sqlite_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/conversation/sqlite"
)

// tailFixture seeds key "tg::1" with an ARCHIVED session 1 holding six turns
// t1..t6 and an active session 2 holding one turn.
func tailFixture(t *testing.T) (*sqlite.SqliteStore, conversation.Key) {
	t.Helper()
	s, _ := openStore(t)
	key := conversation.Key("tg::1")
	for i := 1; i <= 6; i++ {
		mustAppend(t, s, key, conversation.RoleUser, fmt.Sprintf("t%d", i), at(i))
	}
	if _, err := s.NewSession(context.Background(), key); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	mustAppend(t, s, key, conversation.RoleUser, "activo", at(10))
	return s, key
}

func TestLoadSessionTail_Contract(t *testing.T) {
	cases := []struct {
		name    string
		session int
		n       int
		want    []string
	}{
		{"last two oldest-first", 1, 2, []string{"t5", "t6"}},
		{"n equal to the session returns all", 1, 6, []string{"t1", "t2", "t3", "t4", "t5", "t6"}},
		{"n larger than the session returns all", 1, 99, []string{"t1", "t2", "t3", "t4", "t5", "t6"}},
		{"n zero returns no turns", 1, 0, nil},
		{"n negative returns no turns", 1, -3, nil},
		{"unknown session returns empty", 7, 4, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, key := tailFixture(t)
			got, err := s.LoadSessionTail(context.Background(), key, tc.session, tc.n)
			if err != nil {
				t.Fatalf("LoadSessionTail(%d, %d): %v", tc.session, tc.n, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("LoadSessionTail(%d, %d) = %v, want %v", tc.session, tc.n, contents(got), tc.want)
			}
			for i, want := range tc.want {
				if got[i].Content != want {
					t.Fatalf("LoadSessionTail(%d, %d)[%d] = %q, want %q (oldest-first among themselves)",
						tc.session, tc.n, i, got[i].Content, want)
				}
			}
		})
	}
}

func TestLoadSessionTail_UnknownKeyEmptyNoError(t *testing.T) {
	s, _ := openStore(t)
	got, err := s.LoadSessionTail(context.Background(), conversation.Key("tg::nadie"), 1, 4)
	if err != nil {
		t.Fatalf("LoadSessionTail on unknown key: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("LoadSessionTail on unknown key = %v, want empty", contents(got))
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// SP-A red suite (minimal-memory spec 2026-08-16, FR-STORE-A1): the
// LoadSessionTail contract, exercised on MemStore. The sqlite implementation
// runs the same behaviors in its own package test (the SP1 molde).
package conversation

import (
	"context"
	"fmt"
	"testing"
)

// tailFixture seeds key "tg::1" with an ARCHIVED session 1 holding six turns
// t1..t6 and an active session 2 holding one turn, so the tail reads must
// scope to the REQUESTED session, never the active one.
func tailFixture(t *testing.T) (SessionStore, Key) {
	t.Helper()
	s := NewMemStore()
	key := Key("tg::1")
	for i := 1; i <= 6; i++ {
		mustAppend(t, s, key, RoleUser, fmt.Sprintf("t%d", i), at(i))
	}
	if _, err := s.NewSession(context.Background(), key); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	mustAppend(t, s, key, RoleUser, "activo", at(10))
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
	s := NewMemStore()
	got, err := s.LoadSessionTail(context.Background(), Key("tg::nadie"), 1, 4)
	if err != nil {
		t.Fatalf("LoadSessionTail on unknown key: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("LoadSessionTail on unknown key = %v, want empty", contents(got))
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// SP-B red suite (minimal-memory spec 2026-08-16, FR-STORE-1): the notes
// domain on MemStore — the explicit scope enum, the atomic count cap, the
// incoherent-pair rejection, and THE single pure scope derivation. The
// sqlite implementation runs the same behaviors in its own package test
// (the SP1 molde) plus schema/durability/cascade.
package conversation

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestEffectiveNoteScope_Table(t *testing.T) {
	cases := []struct {
		name      string
		scope     NoteScope
		key       Key
		wantScope NoteScope
		wantKey   Key
		wantErr   bool
	}{
		{"conversation with key passes through", ScopeConversation, Key("tg::c"), ScopeConversation, Key("tg::c"), false},
		{"brain-global drops the key", ScopeBrainGlobal, Key("tg::c"), ScopeBrainGlobal, Key(""), false},
		{"brain-global with empty key", ScopeBrainGlobal, Key(""), ScopeBrainGlobal, Key(""), false},
		{"conversation with empty key errors", ScopeConversation, Key(""), 0, "", true},
		{"zero scope errors", 0, Key("tg::c"), 0, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotScope, gotKey, err := EffectiveNoteScope(tc.scope, tc.key)
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidNoteScope) {
					t.Fatalf("EffectiveNoteScope(%d, %q) err = %v, want ErrInvalidNoteScope", tc.scope, tc.key, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("EffectiveNoteScope(%d, %q): %v", tc.scope, tc.key, err)
			}
			if gotScope != tc.wantScope || gotKey != tc.wantKey {
				t.Fatalf("EffectiveNoteScope(%d, %q) = (%d, %q), want (%d, %q)",
					tc.scope, tc.key, gotScope, gotKey, tc.wantScope, tc.wantKey)
			}
		})
	}
}

func noteContents(notes []Note) []string {
	out := make([]string, len(notes))
	for i, n := range notes {
		out[i] = n.Content
	}
	return out
}

func TestAppendNote_StoresSequencedAndStamped(t *testing.T) {
	s := NewMemStore()
	key := Key("tg::c")
	for _, content := range []string{"primera", "segunda"} {
		if _, err := s.AppendNote(context.Background(), "brainA", ScopeConversation, key, content, 10); err != nil {
			t.Fatalf("AppendNote(%q): %v", content, err)
		}
	}
	notes, err := s.ListNotes(context.Background(), "brainA", ScopeConversation, key)
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if len(notes) != 2 || notes[0].Content != "primera" || notes[1].Content != "segunda" {
		t.Fatalf("ListNotes = %v, want [primera segunda] oldest-first", noteContents(notes))
	}
	if notes[0].Seq != 1 || notes[1].Seq != 2 {
		t.Fatalf("Seqs = %d,%d, want 1,2 (store-assigned, monotonic)", notes[0].Seq, notes[1].Seq)
	}
	for i, n := range notes {
		if n.Timestamp.IsZero() {
			t.Fatalf("note %d Timestamp is zero — the STORE stamps it (the NewSession precedent)", i)
		}
	}
}

// AS-B5 (store core): the count cap holds ATOMICALLY under concurrency —
// two goroutines racing on a box one below the cap can never exceed it; the
// loser gets the typed ErrNotesFull and nothing extra is stored.
func TestAppendNote_AtomicCapUnderConcurrency(t *testing.T) {
	s := NewMemStore()
	key := Key("tg::c")
	const maxNotes = 3
	for i := 0; i < maxNotes-1; i++ {
		if _, err := s.AppendNote(context.Background(), "b", ScopeConversation, key, "seed", maxNotes); err != nil {
			t.Fatalf("seed AppendNote: %v", err)
		}
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = s.AppendNote(context.Background(), "b", ScopeConversation, key, "race", maxNotes)
		}(i)
	}
	wg.Wait()

	notes, err := s.ListNotes(context.Background(), "b", ScopeConversation, key)
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if len(notes) > maxNotes {
		t.Fatalf("cap violated: %d notes stored, max_notes = %d", len(notes), maxNotes)
	}
	full := 0
	for _, e := range errs {
		if errors.Is(e, ErrNotesFull) {
			full++
		} else if e != nil {
			t.Fatalf("unexpected AppendNote error: %v", e)
		}
	}
	if full != 1 || len(notes) != maxNotes {
		t.Fatalf("winners/losers = stored %d, ErrNotesFull %d — want exactly the cap and ONE typed refusal", len(notes), full)
	}
}

// AS-B11 (store core): incoherent scope/key pairs are REJECTED with the
// typed error — an upstream derivation failure can never become a silent
// global write.
func TestAppendNote_RejectsIncoherentPairs(t *testing.T) {
	cases := []struct {
		name  string
		scope NoteScope
		key   Key
	}{
		{"conversation scope with empty key", ScopeConversation, Key("")},
		{"brain-global scope with a key", ScopeBrainGlobal, Key("tg::c")},
		{"zero scope", 0, Key("tg::c")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewMemStore()
			if _, err := s.AppendNote(context.Background(), "b", tc.scope, tc.key, "x", 10); !errors.Is(err, ErrInvalidNoteScope) {
				t.Fatalf("AppendNote err = %v, want ErrInvalidNoteScope", err)
			}
			global, err := s.ListNotes(context.Background(), "b", ScopeBrainGlobal, Key(""))
			if err != nil || len(global) != 0 {
				t.Fatalf("global notes after a rejected pair = (%v, %v), want empty (never a silent global write)", global, err)
			}
		})
	}
}

func TestClearNotes_LeavesEmptyAndUnknownIsNoop(t *testing.T) {
	s := NewMemStore()
	key := Key("tg::c")
	if _, err := s.AppendNote(context.Background(), "b", ScopeConversation, key, "x", 10); err != nil {
		t.Fatalf("AppendNote: %v", err)
	}
	if err := s.ClearNotes(context.Background(), "b", ScopeConversation, key); err != nil {
		t.Fatalf("ClearNotes: %v", err)
	}
	notes, err := s.ListNotes(context.Background(), "b", ScopeConversation, key)
	if err != nil || len(notes) != 0 {
		t.Fatalf("notes after clear = (%v, %v), want empty", noteContents(notes), err)
	}
	if err := s.ClearNotes(context.Background(), "b", ScopeConversation, Key("tg::nadie")); err != nil {
		t.Fatalf("ClearNotes on unknown scope = %v, want no-op nil", err)
	}
}

func TestListNotes_ScopeIsolation(t *testing.T) {
	s := NewMemStore()
	key := Key("tg::c")
	if _, err := s.AppendNote(context.Background(), "brainA", ScopeConversation, key, "de-A", 10); err != nil {
		t.Fatalf("AppendNote: %v", err)
	}
	for name, probe := range map[string]func() ([]Note, error){
		"other brain, same key": func() ([]Note, error) {
			return s.ListNotes(context.Background(), "brainB", ScopeConversation, key)
		},
		"same brain, other conversation": func() ([]Note, error) {
			return s.ListNotes(context.Background(), "brainA", ScopeConversation, Key("tg::otra"))
		},
		"same brain, global scope": func() ([]Note, error) {
			return s.ListNotes(context.Background(), "brainA", ScopeBrainGlobal, Key(""))
		},
	} {
		notes, err := probe()
		if err != nil || len(notes) != 0 {
			t.Fatalf("%s = (%v, %v), want empty (isolation)", name, noteContents(notes), err)
		}
	}
}

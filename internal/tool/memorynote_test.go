// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// SP-B red suite (minimal-memory spec 2026-08-16, FR-TOOL-1/2): the
// memory_note builtin — a leaf constructed over the app-composed writer
// closure, single-line normalization, the rune cap, the honest translation
// of the writer's failure modes, the single ParamTool field, and the zero
// house attrs that keep it valid on every locality (AS-B5/B6/B11 tool side).
package tool

import (
	"context"
	"strings"
	"sync"
	"testing"
)

// noteSpy records what the writer closure received.
type noteSpy struct {
	mu     sync.Mutex
	scopes []Scope
	notes  []string
	err    error
}

func (s *noteSpy) write(_ context.Context, scope Scope, note string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.scopes = append(s.scopes, scope)
	s.notes = append(s.notes, note)
	return nil
}

func (s *noteSpy) stored() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.notes...)
}

// The interface contracts: memory_note is a ScopedTool AND a ParamTool.
var (
	_ ScopedTool = (*MemoryNote)(nil)
	_ ParamTool  = (*MemoryNote)(nil)
)

func TestMemoryNote_BuiltinAttrsAreZero(t *testing.T) {
	attrs, ok := BuiltinAttrs("memory_note")
	if !ok {
		t.Fatalf("BuiltinAttrs(memory_note) unknown — the tool must enter the safe-toolset catalog")
	}
	if attrs != (Attrs{}) {
		t.Fatalf("memory_note house attrs = %+v, want Attrs{} (neither Sensitive nor Network — FR-TOOL-1)", attrs)
	}
}

func TestMemoryNote_ParamsSingleRequiredNote(t *testing.T) {
	m := NewMemoryNote((&noteSpy{}).write, 200)
	params := m.Params()
	if len(params) != 1 || params[0].Name != "note" || !params[0].Required {
		t.Fatalf("Params = %+v, want exactly one REQUIRED field \"note\"", params)
	}
	args, err := m.ArgsFromCall(map[string]any{"note": "hola"})
	if err != nil || args != "hola" {
		t.Fatalf("ArgsFromCall(note=hola) = (%q, %v), want (hola, nil)", args, err)
	}
	if _, err := m.ArgsFromCall(map[string]any{}); err == nil || !strings.Contains(err.Error(), "note") {
		t.Fatalf("ArgsFromCall without the field = %v, want an error naming \"note\"", err)
	}
}

// AS-B6: multi-line input stores single-lined; the observation confirms the
// store honestly.
func TestMemoryNote_SingleLineNormalization(t *testing.T) {
	spy := &noteSpy{}
	m := NewMemoryNote(spy.write, 200)
	obs, err := m.ExecuteScoped(context.Background(), Scope{Brain: "b", Conversation: "c"}, "línea una\nlínea dos\r\nlínea tres")
	if err != nil {
		t.Fatalf("ExecuteScoped: %v", err)
	}
	if obs == "" {
		t.Fatalf("empty observation for a stored note — the model needs the confirmation")
	}
	stored := spy.stored()
	if len(stored) != 1 {
		t.Fatalf("writer called %d times, want 1", len(stored))
	}
	if strings.ContainsAny(stored[0], "\n\r") {
		t.Fatalf("stored note is not single-line: %q", stored[0])
	}
	for _, frag := range []string{"línea una", "línea dos", "línea tres"} {
		if !strings.Contains(stored[0], frag) {
			t.Fatalf("normalization lost content %q: %q", frag, stored[0])
		}
	}
}

// AS-B6: over the rune cap the tool refuses NAMING the cap; nothing written.
func TestMemoryNote_RuneCapRefusalNamesCap(t *testing.T) {
	spy := &noteSpy{}
	m := NewMemoryNote(spy.write, 10)
	_, err := m.ExecuteScoped(context.Background(), Scope{Brain: "b", Conversation: "c"}, strings.Repeat("á", 11))
	if err == nil || !strings.Contains(err.Error(), "10") {
		t.Fatalf("over-cap err = %v, want a refusal naming the cap (10)", err)
	}
	if n := len(spy.stored()); n != 0 {
		t.Fatalf("writer called %d times on a refused note, want 0", n)
	}
}

// AS-B5 (tool side) + AS-B11 (tool side): the writer's typed failures
// translate into honest observations.
func TestMemoryNote_TranslatesWriterErrors(t *testing.T) {
	cases := []struct {
		name     string
		writeErr error
		wantFrag string
	}{
		{"note box full names the cap", ErrNoteBoxFull, "full"},
		{"scope derivation says notes need a conversation", ErrNoteNeedsConversation, "conversation"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spy := &noteSpy{err: tc.writeErr}
			m := NewMemoryNote(spy.write, 200)
			_, err := m.ExecuteScoped(context.Background(), Scope{Brain: "b", Conversation: "c"}, "nota")
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.wantFrag) {
				t.Fatalf("err = %v, want an observation containing %q", err, tc.wantFrag)
			}
			if n := len(spy.stored()); n != 0 {
				t.Fatalf("stored %d notes through a failing writer, want 0", n)
			}
		})
	}
}

// The scope reaches the writer verbatim — envelope facts only.
func TestMemoryNote_PassesScopeThrough(t *testing.T) {
	spy := &noteSpy{}
	m := NewMemoryNote(spy.write, 200)
	want := Scope{Brain: "brainA", Conversation: "tg::c"}
	if _, err := m.ExecuteScoped(context.Background(), want, "nota"); err != nil {
		t.Fatalf("ExecuteScoped: %v", err)
	}
	spy.mu.Lock()
	defer spy.mu.Unlock()
	if len(spy.scopes) != 1 || spy.scopes[0] != want {
		t.Fatalf("writer scope = %+v, want %+v verbatim", spy.scopes, want)
	}
}

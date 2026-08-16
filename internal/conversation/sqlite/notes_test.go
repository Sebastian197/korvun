// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// SP-B red suite, sqlite side (minimal-memory spec 2026-08-16,
// FR-STORE-1/2): the notes contract on the durable store — same behaviors
// as the MemStore suite in internal/conversation — plus what only sqlite
// can prove: durability across Close+Open (AS-B12), the DeleteConversation
// cascade that keeps FR-DEL-1 true while brain-global notes survive
// (AS-B12), and the schema riding createTableStmt as a composite-PK
// WITHOUT ROWID table.
package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/conversation/sqlite"
)

func noteContents(notes []conversation.Note) []string {
	out := make([]string, len(notes))
	for i, n := range notes {
		out[i] = n.Content
	}
	return out
}

func TestAppendNote_StoresSequencedAndStamped(t *testing.T) {
	s, _ := openStore(t)
	key := conversation.Key("tg::c")
	for _, content := range []string{"primera", "segunda"} {
		if _, err := s.AppendNote(context.Background(), "brainA", conversation.ScopeConversation, key, content, 10); err != nil {
			t.Fatalf("AppendNote(%q): %v", content, err)
		}
	}
	notes, err := s.ListNotes(context.Background(), "brainA", conversation.ScopeConversation, key)
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if len(notes) != 2 || notes[0].Content != "primera" || notes[1].Content != "segunda" {
		t.Fatalf("ListNotes = %v, want [primera segunda] oldest-first", noteContents(notes))
	}
	if notes[0].Seq != 1 || notes[1].Seq != 2 {
		t.Fatalf("Seqs = %d,%d, want 1,2", notes[0].Seq, notes[1].Seq)
	}
	if notes[0].Timestamp.IsZero() {
		t.Fatalf("Timestamp is zero — the STORE stamps it")
	}
}

// AS-B5 (durable store): the cap is atomic on the serialized writer too.
func TestAppendNote_AtomicCapUnderConcurrency(t *testing.T) {
	s, _ := openStore(t)
	key := conversation.Key("tg::c")
	const maxNotes = 3
	for i := 0; i < maxNotes-1; i++ {
		if _, err := s.AppendNote(context.Background(), "b", conversation.ScopeConversation, key, "seed", maxNotes); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = s.AppendNote(context.Background(), "b", conversation.ScopeConversation, key, "race", maxNotes)
		}(i)
	}
	wg.Wait()
	notes, err := s.ListNotes(context.Background(), "b", conversation.ScopeConversation, key)
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	full := 0
	for _, e := range errs {
		if errors.Is(e, conversation.ErrNotesFull) {
			full++
		} else if e != nil {
			t.Fatalf("unexpected AppendNote error: %v", e)
		}
	}
	if len(notes) != maxNotes || full != 1 {
		t.Fatalf("stored %d / ErrNotesFull %d — want exactly the cap (%d) and ONE typed refusal", len(notes), full, maxNotes)
	}
}

// AS-B11 (durable store): incoherent pairs rejected, nothing stored.
func TestAppendNote_RejectsIncoherentPairs(t *testing.T) {
	s, _ := openStore(t)
	if _, err := s.AppendNote(context.Background(), "b", conversation.ScopeConversation, conversation.Key(""), "x", 10); !errors.Is(err, conversation.ErrInvalidNoteScope) {
		t.Fatalf("conversation+empty key err = %v, want ErrInvalidNoteScope", err)
	}
	if _, err := s.AppendNote(context.Background(), "b", conversation.ScopeBrainGlobal, conversation.Key("tg::c"), "x", 10); !errors.Is(err, conversation.ErrInvalidNoteScope) {
		t.Fatalf("global+key err = %v, want ErrInvalidNoteScope", err)
	}
	global, err := s.ListNotes(context.Background(), "b", conversation.ScopeBrainGlobal, conversation.Key(""))
	if err != nil || len(global) != 0 {
		t.Fatalf("global notes after rejections = (%v, %v), want empty", noteContents(global), err)
	}
}

// AS-B12 (first half): notes survive Close + Open — durability.
func TestNotes_DurableAcrossReopen(t *testing.T) {
	s, path := openStore(t)
	key := conversation.Key("tg::c")
	if _, err := s.AppendNote(context.Background(), "b", conversation.ScopeConversation, key, "persistente", 10); err != nil {
		t.Fatalf("AppendNote: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	reopened, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("re-Open(%q): %v", path, err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	notes, err := reopened.ListNotes(context.Background(), "b", conversation.ScopeConversation, key)
	if err != nil {
		t.Fatalf("ListNotes after reopen: %v", err)
	}
	if len(notes) != 1 || notes[0].Content != "persistente" {
		t.Fatalf("notes after reopen = %v, want [persistente]", noteContents(notes))
	}
}

// AS-B12 (second half): DeleteConversation cascades the key's notes across
// brains (FR-DEL-1 "really gone" stays true) while brain-global notes are
// not the conversation's and survive — stated to the face.
func TestDeleteConversation_CascadesNotesButNotGlobal(t *testing.T) {
	s, _ := openStore(t)
	key := conversation.Key("tg::c")
	mustAppend(t, s, key, conversation.RoleUser, "hola", at(1))
	if _, err := s.AppendNote(context.Background(), "brainA", conversation.ScopeConversation, key, "de-la-conv", 10); err != nil {
		t.Fatalf("AppendNote conv: %v", err)
	}
	if _, err := s.AppendNote(context.Background(), "brainA", conversation.ScopeBrainGlobal, conversation.Key(""), "global", 10); err != nil {
		t.Fatalf("AppendNote global: %v", err)
	}
	if err := s.DeleteConversation(context.Background(), key); err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}
	conv, err := s.ListNotes(context.Background(), "brainA", conversation.ScopeConversation, key)
	if err != nil || len(conv) != 0 {
		t.Fatalf("conversation notes after delete = (%v, %v), want gone with the turns (FR-DEL-1)", noteContents(conv), err)
	}
	global, err := s.ListNotes(context.Background(), "brainA", conversation.ScopeBrainGlobal, conversation.Key(""))
	if err != nil || len(global) != 1 {
		t.Fatalf("global notes after delete = (%v, %v), want [global] surviving", noteContents(global), err)
	}
}

// The notes table rides createTableStmt: composite PK (brain, key, seq),
// WITHOUT ROWID — the sessions/turns house pattern (FR-STORE-2).
func TestNotesSchema_CompositePKWithoutRowid(t *testing.T) {
	_, path := openStore(t)
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var ddl string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='notes'`).Scan(&ddl); err != nil {
		t.Fatalf("the notes table is not in the schema (createTableStmt): %v", err)
	}
	if !strings.Contains(strings.ToUpper(ddl), "WITHOUT ROWID") {
		t.Fatalf("notes DDL lacks WITHOUT ROWID:\n%s", ddl)
	}
	rows, err := db.Query(`SELECT name, pk FROM pragma_table_info('notes') WHERE pk > 0 ORDER BY pk`)
	if err != nil {
		t.Fatalf("pragma_table_info: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var pkCols []string
	for rows.Next() {
		var name string
		var pk int
		if err := rows.Scan(&name, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		pkCols = append(pkCols, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	want := []string{"brain", "key", "seq"}
	if len(pkCols) != len(want) {
		t.Fatalf("notes PK columns = %v, want %v", pkCols, want)
	}
	for i := range want {
		if pkCols[i] != want[i] {
			t.Fatalf("notes PK columns = %v, want %v", pkCols, want)
		}
	}
}

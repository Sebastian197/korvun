// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// TANDA DECLARADA (SP-B encargo, punto 7 — GREEN on arrival, separate from
// the red suite): the error paths the EXISTING store methods carried
// uncovered (the LoadRecent/LoadSession inherited molde) — every method
// surfaces a wrapped error instead of silence when the database is gone.
// Adjudicated to lift the subpackage over the spec's 90% floor.
package sqlite_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/conversation/sqlite"
)

func TestClosedStore_EveryMethodFailsLoud(t *testing.T) {
	s, _ := openStore(t)
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	key := conversation.Key("tg::c")
	turn := conversation.Turn{Role: conversation.RoleUser, Content: "x", Timestamp: at(1)}

	cases := []struct {
		name string
		call func() error
	}{
		{"Append", func() error { _, err := s.Append(context.Background(), key, turn); return err }},
		{"AppendTurns", func() error { _, err := s.AppendTurns(context.Background(), key, turn); return err }},
		{"LoadRecent", func() error { _, err := s.LoadRecent(context.Background(), key, 5); return err }},
		{"LoadSession", func() error { _, err := s.LoadSession(context.Background(), key, 1); return err }},
		{"LoadSessionTail", func() error { _, err := s.LoadSessionTail(context.Background(), key, 1, 5); return err }},
		{"ListSessions", func() error { _, err := s.ListSessions(context.Background(), key); return err }},
		{"ListConversations", func() error { _, err := s.ListConversations(context.Background(), 5); return err }},
		{"SearchTurns", func() error { _, err := s.SearchTurns(context.Background(), "x", 5); return err }},
		{"NewSession", func() error { _, err := s.NewSession(context.Background(), key); return err }},
		{"DeleteConversation", func() error { return s.DeleteConversation(context.Background(), key) }},
		{"DeleteSession", func() error { return s.DeleteSession(context.Background(), key, 1) }},
		{"AppendNote", func() error {
			_, err := s.AppendNote(context.Background(), "b", conversation.ScopeConversation, key, "x", 10)
			return err
		}},
		{"ListNotes", func() error {
			_, err := s.ListNotes(context.Background(), "b", conversation.ScopeConversation, key)
			return err
		}},
		{"ClearNotes", func() error { return s.ClearNotes(context.Background(), "b", conversation.ScopeConversation, key) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil {
				t.Fatalf("%s on a closed store returned nil, want a wrapped error", tc.name)
			}
			if !strings.Contains(err.Error(), "sqlite") {
				t.Fatalf("%s error %q is not the package's wrapped shape", tc.name, err)
			}
		})
	}
}

// Open's boot-fatal error paths: an uncreatable parent directory and a
// corrupt database file both fail HERE, never on the first message
// (ADR-0019 §4/§5).
func TestOpen_ErrorPathsFailLoud(t *testing.T) {
	t.Run("parent is a file", func(t *testing.T) {
		parent := filepath.Join(t.TempDir(), "not-a-dir")
		if err := os.WriteFile(parent, []byte("x"), 0o600); err != nil {
			t.Fatalf("seed file: %v", err)
		}
		if _, err := sqlite.Open(filepath.Join(parent, "k.db")); err == nil {
			t.Fatalf("Open under a file parent returned nil, want a wrapped error")
		}
	})
	t.Run("corrupt database file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "k.db")
		if err := os.WriteFile(path, []byte("this is not a sqlite file at all"), 0o600); err != nil {
			t.Fatalf("seed corrupt file: %v", err)
		}
		if _, err := sqlite.Open(path); err == nil {
			t.Fatalf("Open on a corrupt file returned nil, want a wrapped error")
		}
	})
	t.Run("read-only database file", func(t *testing.T) {
		// A valid-but-empty sqlite file that cannot be written: the schema
		// bootstrap (CREATE TABLE IF NOT EXISTS) fails loud at Open, never
		// on the first message.
		path := filepath.Join(t.TempDir(), "k.db")
		if err := os.WriteFile(path, nil, 0o400); err != nil {
			t.Fatalf("seed read-only file: %v", err)
		}
		if _, err := sqlite.Open(path); err == nil {
			t.Fatalf("Open on a read-only file returned nil, want a wrapped error")
		}
	})
}

// badRow injects a raw row whose ts column holds TEXT (sqlite's type
// affinity stores it verbatim in the INTEGER column) so the read paths hit
// their scan-error branches — the inherited molde's uncovered lines.
func badRow(t *testing.T, path, stmt string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(stmt); err != nil {
		t.Fatalf("raw exec: %v", err)
	}
}

func TestScanErrors_SurfaceWrapped(t *testing.T) {
	s, path := openStore(t)
	mustAppend(t, s, conversation.Key("tg::c"), conversation.RoleUser, "xx", at(1))
	badRow(t, path, `INSERT INTO turns (key, session, seq, role, content, ts) VALUES ('tg::c', 1, 2, 'user', 'yy', 'not-a-number')`)
	badRow(t, path, `INSERT INTO notes (brain, key, seq, content, ts) VALUES ('b', 'tg::c', 1, 'x', 'not-a-number')`)

	cases := []struct {
		name string
		call func() error
	}{
		{"LoadRecent", func() error { _, err := s.LoadRecent(context.Background(), "tg::c", 5); return err }},
		{"LoadSession", func() error { _, err := s.LoadSession(context.Background(), "tg::c", 1); return err }},
		{"LoadSessionTail", func() error { _, err := s.LoadSessionTail(context.Background(), "tg::c", 1, 5); return err }},
		{"ListNotes", func() error {
			_, err := s.ListNotes(context.Background(), "b", conversation.ScopeConversation, "tg::c")
			return err
		}},
		{"SearchTurns", func() error { _, err := s.SearchTurns(context.Background(), "yy", 5); return err }},
		{"ListSessions", func() error { _, err := s.ListSessions(context.Background(), "tg::c"); return err }},
		{"ListConversations", func() error { _, err := s.ListConversations(context.Background(), 5); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil {
				t.Fatalf("%s over a NULL row returned nil, want a wrapped scan error", tc.name)
			}
			if !strings.Contains(err.Error(), "sqlite") {
				t.Fatalf("%s error %q is not the package's wrapped shape", tc.name, err)
			}
		})
	}
}

// A table dropped out from under a live store (raw SQL on the same file)
// surfaces each writer's IN-TRANSACTION error branch — begin succeeds, the
// statement fails, the wrapped error comes back.
func TestMissingTable_ErrorsSurfaceWrapped(t *testing.T) {
	s, path := openStore(t)
	key := conversation.Key("tg::c")
	mustAppend(t, s, key, conversation.RoleUser, "hola", at(1))
	for _, stmt := range []string{`DROP TABLE notes`, `DROP TABLE turns`, `DROP TABLE sessions`} {
		badRow(t, path, stmt)
	}
	turn := conversation.Turn{Role: conversation.RoleUser, Content: "x", Timestamp: at(2)}
	cases := []struct {
		name string
		call func() error
	}{
		{"AppendNote", func() error {
			_, err := s.AppendNote(context.Background(), "b", conversation.ScopeConversation, key, "x", 10)
			return err
		}},
		{"AppendTurns", func() error { _, err := s.AppendTurns(context.Background(), key, turn); return err }},
		{"NewSession", func() error { _, err := s.NewSession(context.Background(), key); return err }},
		{"DeleteConversation", func() error { return s.DeleteConversation(context.Background(), key) }},
		{"DeleteSession", func() error { return s.DeleteSession(context.Background(), key, 1) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil {
				t.Fatalf("%s with its table dropped returned nil, want a wrapped error", tc.name)
			}
			if !strings.Contains(err.Error(), "sqlite") {
				t.Fatalf("%s error %q is not the package's wrapped shape", tc.name, err)
			}
		})
	}
}

// AppendNote's insert branch: with the notes TABLE swapped for a read-only
// VIEW of the same shape, the count and seq probes succeed and the INSERT
// itself fails — the wrapped error names the step.
func TestAppendNote_InsertErrorSurfacesWrapped(t *testing.T) {
	s, path := openStore(t)
	badRow(t, path, `DROP TABLE notes`)
	badRow(t, path, `CREATE VIEW notes (brain, key, seq, content, ts) AS SELECT 'b', 'tg::c', 0, 'x', 0`)
	_, err := s.AppendNote(context.Background(), "b", conversation.ScopeConversation, conversation.Key("tg::c"), "x", 10)
	if err == nil {
		t.Fatalf("AppendNote into a view returned nil, want a wrapped insert error")
	}
	if !strings.Contains(err.Error(), "sqlite: AppendNote") {
		t.Fatalf("error %q is not the package's wrapped shape", err)
	}
}

// DeleteConversation's mid-transaction branches: with exactly ONE of its
// three tables dropped, the failure surfaces at that precise step.
func TestDeleteConversation_MidTransactionErrors(t *testing.T) {
	for _, table := range []string{"sessions", "notes"} {
		t.Run("dropped "+table, func(t *testing.T) {
			s, path := openStore(t)
			key := conversation.Key("tg::c")
			mustAppend(t, s, key, conversation.RoleUser, "hola", at(1))
			badRow(t, path, "DROP TABLE "+table)
			err := s.DeleteConversation(context.Background(), key)
			if err == nil {
				t.Fatalf("DeleteConversation with %s dropped returned nil, want a wrapped error", table)
			}
			if !strings.Contains(err.Error(), table) {
				t.Fatalf("error %q does not name the failing step (%s)", err, table)
			}
		})
	}
}

// The sqlite mirror of the MemStore ClearNotes contract (an SP-B RED
// omission, completed under this tanda's declared purpose): clear leaves
// the scope empty; an unknown scope is a no-op.
func TestClearNotes_LeavesEmptyAndUnknownIsNoop(t *testing.T) {
	s, _ := openStore(t)
	key := conversation.Key("tg::c")
	if _, err := s.AppendNote(context.Background(), "b", conversation.ScopeConversation, key, "x", 10); err != nil {
		t.Fatalf("AppendNote: %v", err)
	}
	if err := s.ClearNotes(context.Background(), "b", conversation.ScopeConversation, key); err != nil {
		t.Fatalf("ClearNotes: %v", err)
	}
	notes, err := s.ListNotes(context.Background(), "b", conversation.ScopeConversation, key)
	if err != nil || len(notes) != 0 {
		t.Fatalf("notes after clear = (%v, %v), want empty", noteContents(notes), err)
	}
	if err := s.ClearNotes(context.Background(), "b", conversation.ScopeConversation, conversation.Key("tg::nadie")); err != nil {
		t.Fatalf("ClearNotes on unknown scope = %v, want no-op nil", err)
	}
}

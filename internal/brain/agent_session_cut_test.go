// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/conversation/sqlite"
)

// The 2026-08-09 demo defect, reproduced at the level that matters: what the
// BRAIN actually loads into the model request after a session reset. A /new
// opens a fresh session (the router does that before dispatch); FR-SESS-2
// says a reset is a HARD context cut — so not one pre-reset turn may reach
// the model, no matter which store path the brain reads through.

// seedPoisonedHistory writes the demo's poisoned turns into the ACTIVE
// session, then opens a new session (what the /new trigger does).
func seedPoisonedHistory(t *testing.T, store conversation.SessionStore, key conversation.Key) {
	t.Helper()
	turns := []conversation.Turn{
		{Role: conversation.RoleUser, Content: "Lee el fichero nota.txt con la herramienta read_file y dime la palabra clave.", Timestamp: time.Unix(100, 0)},
		{Role: conversation.RoleAssistant, Content: `OBSERVACIÓN: La palabra clave en el archivo nota.txt es "seguridad".`, Timestamp: time.Unix(101, 0)},
	}
	for _, turn := range turns {
		if _, err := store.Append(context.Background(), key, turn); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	if _, err := store.NewSession(context.Background(), key); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
}

// requestCarriesPoison reports whether any message the model saw contains a
// pre-reset fragment.
func requestCarriesPoison(m *scriptedModel) (string, bool) {
	for _, msg := range m.lastReq.Messages {
		if strings.Contains(msg.Content, "seguridad") {
			return msg.Content, true
		}
	}
	return "", false
}

// AgentBrain: after a session reset, the model request must carry ZERO
// pre-reset turns.
func TestAgentBrain_sessionResetCutsLoadedContext(t *testing.T) {
	t.Parallel()
	store := conversation.NewMemStore()
	key := conversation.Key("console::op")
	seedPoisonedHistory(t, store, key)

	m := &scriptedModel{name: "m", replies: []string{"done"}}
	a := NewAgentBrain(m, builtinRegistry(),
		WithAgentLogger(quietLogger()),
		WithAgentConversationStore(store, 10))

	if _, err := a.Handle(context.Background(), inboundText("console", "op", "otra vez")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if leaked, bad := requestCarriesPoison(m); bad {
		t.Fatalf("pre-reset turn reached the model after /new:\n%q", leaked)
	}
}

// Same reproduction against the REAL sqlite store — the one the demo ran on.
func TestAgentBrain_sessionResetCutsLoadedContext_sqlite(t *testing.T) {
	t.Parallel()
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "cut.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	key := conversation.Key("console::op")
	seedPoisonedHistory(t, store, key)

	m := &scriptedModel{name: "m", replies: []string{"done"}}
	a := NewAgentBrain(m, builtinRegistry(),
		WithAgentLogger(quietLogger()),
		WithAgentConversationStore(store, 10))

	if _, err := a.Handle(context.Background(), inboundText("console", "op", "otra vez")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if leaked, bad := requestCarriesPoison(m); bad {
		t.Fatalf("pre-reset turn reached the model after /new (sqlite):\n%q", leaked)
	}
}

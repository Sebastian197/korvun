// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Deletion + search + unread-anchor API red (operator-console spec
// FR-DEL-2 / FR-SEARCH / FR-UNREAD, AS-13/14): bearer everywhere, the
// takeover gate released on conversation wipe, the active session answering
// 409, and the inbox row carrying turn_count.
package controlapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/conversation"
)

func TestConsole_DeleteConversationWipesAndReleasesTakeover(t *testing.T) {
	srv, store, op := consoleServer(t)
	seed(t, store, "tg::c", conversation.RoleUser, "hola", 10)
	if _, err := store.NewSession(context.Background(), "tg::c"); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	seed(t, store, "tg::c", conversation.RoleOperator, "voy", 20)
	op.TakeOver("tg::c")

	res := doReq(t, "DELETE", srv.URL+"/api/conversations/tg::c", consoleToken, "")
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete conversation = %d, want 204", res.StatusCode)
	}
	if sessions, _ := store.ListSessions(context.Background(), "tg::c"); len(sessions) != 0 {
		t.Fatalf("sessions survived the wipe: %+v", sessions)
	}
	if op.TakenOver("tg::c") {
		t.Fatal("takeover gate NOT released on wipe — a deleted conversation left a silenced ghost")
	}
	// Bad key: 400 without touching anything.
	res = doReq(t, "DELETE", srv.URL+"/api/conversations/sinseparador", consoleToken, "")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad key delete = %d, want 400", res.StatusCode)
	}
}

func TestConsole_DeleteSessionArchivedOnlyActive409(t *testing.T) {
	srv, store, _ := consoleServer(t)
	seed(t, store, "tg::c", conversation.RoleUser, "s1", 10)
	if _, err := store.NewSession(context.Background(), "tg::c"); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	seed(t, store, "tg::c", conversation.RoleUser, "s2", 20)

	// The ACTIVE session: honest 409, nothing deleted.
	res := doReq(t, "DELETE", srv.URL+"/api/conversations/tg::c/sessions/2", consoleToken, "")
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("delete active session = %d, want 409", res.StatusCode)
	}
	// The archived one goes.
	res = doReq(t, "DELETE", srv.URL+"/api/conversations/tg::c/sessions/1", consoleToken, "")
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete archived session = %d, want 204", res.StatusCode)
	}
	sessions, _ := store.ListSessions(context.Background(), "tg::c")
	if len(sessions) != 1 || sessions[0].ID != 2 {
		t.Fatalf("sessions after archive delete = %+v, want only session 2", sessions)
	}
}

func TestConsole_SearchEndpoint(t *testing.T) {
	srv, store, _ := consoleServer(t)
	if _, err := store.Append(context.Background(), "tg::c", conversation.Turn{
		Role: conversation.RoleUser, Content: "la lavadora hace ruido",
		Timestamp: time.Unix(10, 0).UTC(),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	res := doReq(t, "GET", srv.URL+"/api/search?q=lavadora", consoleToken, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("search = %d, want 200", res.StatusCode)
	}
	type hit struct {
		Key     string `json:"key"`
		Session int    `json:"session"`
		Seq     int    `json:"seq"`
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	hits := decode[[]hit](t, res)
	if len(hits) != 1 || hits[0].Key != "tg::c" || hits[0].Session != 1 || hits[0].Content != "la lavadora hace ruido" {
		t.Fatalf("hits = %+v, want the seeded turn, addressable", hits)
	}
	// Empty query: 400 (an unbounded scan is not a default).
	res = doReq(t, "GET", srv.URL+"/api/search?q=", consoleToken, "")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty q = %d, want 400", res.StatusCode)
	}
	// Bearer required.
	res = doReq(t, "GET", srv.URL+"/api/search?q=x", "", "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no bearer = %d, want 401", res.StatusCode)
	}
}

func TestConsole_InboxCarriesTurnCount(t *testing.T) {
	srv, store, _ := consoleServer(t)
	seed(t, store, "tg::c", conversation.RoleUser, "a", 10)
	seed(t, store, "tg::c", conversation.RoleAssistant, "b", 20)
	res := doReq(t, "GET", srv.URL+"/api/conversations", consoleToken, "")
	type row struct {
		Key       string `json:"key"`
		TurnCount int    `json:"turn_count"`
	}
	rows := decode[[]row](t, res)
	if len(rows) != 1 || rows[0].TurnCount != 2 {
		t.Fatalf("rows = %+v, want turn_count 2 (the unread anchor)", rows)
	}
}

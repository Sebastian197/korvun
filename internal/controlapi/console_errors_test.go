// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// SP3 unhappy paths: a failing store maps to honest 500s (never an empty
// 200), and malformed parameters are 400s that touch neither store nor
// router.
package controlapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Sebastian197/korvun/internal/controlapi"
	"github.com/Sebastian197/korvun/internal/conversation"
)

// failingStore implements conversation.SessionStore; every method errors.
type failingStore struct{}

var errStore = errors.New("store down")

func (failingStore) LoadRecent(context.Context, conversation.Key, int) ([]conversation.Turn, error) {
	return nil, errStore
}
func (failingStore) Append(context.Context, conversation.Key, conversation.Turn) (conversation.Turn, error) {
	return conversation.Turn{}, errStore
}
func (failingStore) AppendTurns(context.Context, conversation.Key, ...conversation.Turn) ([]conversation.Turn, error) {
	return nil, errStore
}
func (failingStore) NewSession(context.Context, conversation.Key) (int, error) { return 0, errStore }
func (failingStore) ListConversations(context.Context, int) ([]conversation.ConversationInfo, error) {
	return nil, errStore
}
func (failingStore) ListSessions(context.Context, conversation.Key) ([]conversation.SessionInfo, error) {
	return nil, errStore
}
func (failingStore) LoadSession(context.Context, conversation.Key, int) ([]conversation.Turn, error) {
	return nil, errStore
}

func failingServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	controlapi.RegisterConsole(mux, consoleToken, failingStore{}, newFakeOpRouter())
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestConsole_StoreFailuresAreHonest500s(t *testing.T) {
	srv := failingServer(t)
	cases := []struct{ method, path string }{
		{"GET", "/api/conversations"},
		{"GET", "/api/conversations/tg::c"},
		{"GET", "/api/conversations/tg::c/sessions"},
		{"GET", "/api/conversations/tg::c/sessions/1"},
		{"POST", "/api/conversations/tg::c/sessions"},
	}
	for _, tc := range cases {
		res := doReq(t, tc.method, srv.URL+tc.path, consoleToken, "")
		if res.StatusCode != http.StatusInternalServerError {
			t.Fatalf("%s %s = %d, want 500 (never an empty 200 over a broken store)",
				tc.method, tc.path, res.StatusCode)
		}
	}
}

func TestConsole_MalformedParamsAre400s(t *testing.T) {
	srv, _, _ := consoleServer(t)
	cases := []struct{ method, path string }{
		{"GET", "/api/conversations?limit=0"},
		{"GET", "/api/conversations?limit=nope"},
		{"GET", "/api/conversations/tg::c?n=-2"},
		{"GET", "/api/conversations/tg::c/sessions/0"},
		{"GET", "/api/conversations/tg::c/sessions/x"},
		{"GET", "/api/conversations/sinseparador"},
		{"POST", "/api/conversations/sinseparador/sessions"},
		{"POST", "/api/conversations/sinseparador/takeover"},
	}
	for _, tc := range cases {
		res := doReq(t, tc.method, srv.URL+tc.path, consoleToken, "")
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s %s = %d, want 400", tc.method, tc.path, res.StatusCode)
		}
	}
}

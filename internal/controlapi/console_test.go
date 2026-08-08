// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// SP3 red (operator-console spec, FR-API-1/1b/1c/2/3 + AS-1/2/3/4/8): the
// console endpoints — bearer on EVERY route (content leaves the process),
// inbox and session reads, operator reply through the router seam with
// honest error mapping, takeover/release and new-session mutations.
package controlapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/controlapi"
	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/router"
)

const consoleToken = "tok-console"

// fakeOpRouter records the operator-surface calls.
type fakeOpRouter struct {
	mu          sync.Mutex
	dispatched  []*envelope.Envelope
	dispatchErr error
	inbound     []*envelope.Envelope
	inboundErr  error
	taken       map[conversation.Key]bool
}

func newFakeOpRouter() *fakeOpRouter {
	return &fakeOpRouter{taken: make(map[conversation.Key]bool)}
}

func (f *fakeOpRouter) DispatchOutbound(_ context.Context, env *envelope.Envelope) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dispatchErr != nil {
		return f.dispatchErr
	}
	f.dispatched = append(f.dispatched, env)
	return nil
}
func (f *fakeOpRouter) DispatchInbound(_ context.Context, env *envelope.Envelope) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.inboundErr != nil {
		return f.inboundErr
	}
	f.inbound = append(f.inbound, env)
	return nil
}
func (f *fakeOpRouter) Inbound() []*envelope.Envelope {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*envelope.Envelope(nil), f.inbound...)
}
func (f *fakeOpRouter) TakeOver(k conversation.Key) {
	f.mu.Lock()
	f.taken[k] = true
	f.mu.Unlock()
}
func (f *fakeOpRouter) Release(k conversation.Key) {
	f.mu.Lock()
	delete(f.taken, k)
	f.mu.Unlock()
}
func (f *fakeOpRouter) TakenOver(k conversation.Key) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.taken[k]
}
func (f *fakeOpRouter) Dispatched() []*envelope.Envelope {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*envelope.Envelope(nil), f.dispatched...)
}

func consoleServer(t *testing.T) (*httptest.Server, *conversation.MemStore, *fakeOpRouter) {
	t.Helper()
	store := conversation.NewMemStore()
	op := newFakeOpRouter()
	mux := http.NewServeMux()
	controlapi.RegisterConsole(mux, consoleToken, store, op)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, store, op
}

func seed(t *testing.T, store *conversation.MemStore, key conversation.Key, role conversation.Role, content string, sec int) {
	t.Helper()
	if _, err := store.Append(context.Background(), key, conversation.Turn{
		Role: role, Content: content, Timestamp: time.Unix(int64(sec), 0).UTC(),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func doReq(t *testing.T, method, url, token, body string) *http.Response {
	t.Helper()
	var rd *strings.Reader
	if body == "" {
		rd = strings.NewReader("")
	} else {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rd)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

func decode[T any](t *testing.T, res *http.Response) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}

// --- AS-8: bearer on EVERY console route ------------------------------------

func TestConsole_EveryRouteRequiresBearer(t *testing.T) {
	srv, _, _ := consoleServer(t)
	routes := []struct{ method, path string }{
		{"GET", "/api/conversations"},
		{"GET", "/api/conversations/tg::c"},
		{"GET", "/api/conversations/tg::c/sessions"},
		{"GET", "/api/conversations/tg::c/sessions/1"},
		{"POST", "/api/conversations/tg::c/reply"},
		{"POST", "/api/conversations/tg::c/takeover"},
		{"POST", "/api/conversations/tg::c/release"},
		{"POST", "/api/conversations/tg::c/sessions"},
	}
	for _, rt := range routes {
		for _, token := range []string{"", "wrong"} {
			res := doReq(t, rt.method, srv.URL+rt.path, token, `{"text":"x"}`)
			if res.StatusCode != http.StatusUnauthorized {
				t.Fatalf("%s %s with token %q = %d, want 401", rt.method, rt.path, token, res.StatusCode)
			}
		}
	}
}

// --- AS-1/AS-2: inbox and session reads --------------------------------------

func TestConsole_InboxListsConversationsWithTakeoverState(t *testing.T) {
	srv, store, op := consoleServer(t)
	seed(t, store, "tg::a", conversation.RoleUser, "hola", 10)
	seed(t, store, "dc::b", conversation.RoleOperator, "aquí", 20)
	op.TakeOver("dc::b")

	res := doReq(t, "GET", srv.URL+"/api/conversations", consoleToken, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	type row struct {
		Key           string `json:"key"`
		ActiveSession int    `json:"active_session"`
		SessionCount  int    `json:"session_count"`
		LastActivity  string `json:"last_activity"`
		LastRole      string `json:"last_role"`
		TakenOver     bool   `json:"taken_over"`
	}
	rows := decode[[]row](t, res)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (%+v)", len(rows), rows)
	}
	if rows[0].Key != "dc::b" || !rows[0].TakenOver || rows[0].LastRole != "operator" {
		t.Fatalf("row 0 = %+v, want dc::b taken over, last role operator", rows[0])
	}
	if rows[1].Key != "tg::a" || rows[1].TakenOver || rows[1].ActiveSession != 1 {
		t.Fatalf("row 1 = %+v, want tg::a not taken, active 1", rows[1])
	}
}

func TestConsole_SessionNavigationReads(t *testing.T) {
	srv, store, _ := consoleServer(t)
	seed(t, store, "tg::c", conversation.RoleUser, "s1-a", 10)
	if _, err := store.NewSession(context.Background(), "tg::c"); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	seed(t, store, "tg::c", conversation.RoleAssistant, "s2-a", 20)

	// The session list of the key.
	res := doReq(t, "GET", srv.URL+"/api/conversations/tg::c/sessions", consoleToken, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("sessions status = %d, want 200", res.StatusCode)
	}
	type sess struct {
		ID        int `json:"id"`
		TurnCount int `json:"turn_count"`
	}
	sessions := decode[[]sess](t, res)
	if len(sessions) != 2 || sessions[0].ID != 1 || sessions[1].TurnCount != 1 {
		t.Fatalf("sessions = %+v, want [{1,1},{2,1}]", sessions)
	}

	// Any OLD session's turns remain readable, content included (FR-SESS-6).
	res = doReq(t, "GET", srv.URL+"/api/conversations/tg::c/sessions/1", consoleToken, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("session detail status = %d, want 200", res.StatusCode)
	}
	type turn struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	turns := decode[[]turn](t, res)
	if len(turns) != 1 || turns[0].Content != "s1-a" || turns[0].Role != "user" {
		t.Fatalf("session 1 turns = %+v, want [s1-a/user]", turns)
	}

	// The conversation detail = the ACTIVE session's recent turns.
	res = doReq(t, "GET", srv.URL+"/api/conversations/tg::c", consoleToken, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("detail status = %d, want 200", res.StatusCode)
	}
	turns = decode[[]turn](t, res)
	if len(turns) != 1 || turns[0].Content != "s2-a" {
		t.Fatalf("active detail = %+v, want the active session only", turns)
	}
}

// --- AS-3 (API half): the operator reply -------------------------------------

func TestConsole_ReplyDispatchesOperatorEnvelope(t *testing.T) {
	srv, store, op := consoleServer(t)
	seed(t, store, "tg::c", conversation.RoleUser, "hola", 10)

	res := doReq(t, "POST", srv.URL+"/api/conversations/tg::c/reply", consoleToken,
		`{"text":"aquí Chano"}`)
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (async funnel, AS-5 as amended)", res.StatusCode)
	}
	sent := op.Dispatched()
	if len(sent) != 1 {
		t.Fatalf("DispatchOutbound calls = %d, want 1", len(sent))
	}
	env := sent[0]
	if env.Channel != "tg" || env.Direction != envelope.Outbound {
		t.Fatalf("envelope misaddressed: %+v", env)
	}
	if env.Meta[conversation.MetaConversationID] != "c" {
		t.Fatalf("conversation identity lost: %+v (the operator turn must ALWAYS persist)", env.Meta)
	}
	if len(env.Parts) != 1 || env.Parts[0].Content != "aquí Chano" {
		t.Fatalf("text = %+v, want the operator text", env.Parts)
	}
}

func TestConsole_ReplyValidation(t *testing.T) {
	srv, _, op := consoleServer(t)
	cases := []struct {
		name, path, body string
	}{
		{"empty text", "/api/conversations/tg::c/reply", `{"text":""}`},
		{"whitespace text", "/api/conversations/tg::c/reply", `{"text":"   "}`},
		{"malformed body", "/api/conversations/tg::c/reply", `nope`},
		{"key without conversation id", "/api/conversations/tg::/reply", `{"text":"x"}`},
		{"key without channel", "/api/conversations/::c/reply", `{"text":"x"}`},
		{"key without separator", "/api/conversations/tgc/reply", `{"text":"x"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := doReq(t, "POST", srv.URL+tc.path, consoleToken, tc.body)
			if res.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", res.StatusCode)
			}
		})
	}
	if n := len(op.Dispatched()); n != 0 {
		t.Fatalf("invalid replies reached the router %d times, want 0", n)
	}
}

func TestConsole_ReplyErrorMappingIsHonest(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"unknown channel", router.ErrUnknownChannel, http.StatusConflict},
		{"saturated", router.ErrChannelSaturated, http.StatusServiceUnavailable},
		{"other", context.DeadlineExceeded, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _, op := consoleServer(t)
			op.dispatchErr = tc.err
			res := doReq(t, "POST", srv.URL+"/api/conversations/tg::c/reply", consoleToken,
				`{"text":"x"}`)
			if res.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d for %v", res.StatusCode, tc.want, tc.err)
			}
		})
	}
}

// --- FR-API-3 / FR-API-1c: takeover and new-session mutations -----------------

func TestConsole_TakeoverAndRelease(t *testing.T) {
	srv, _, op := consoleServer(t)
	res := doReq(t, "POST", srv.URL+"/api/conversations/tg::c/takeover", consoleToken, "")
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("takeover status = %d, want 204", res.StatusCode)
	}
	if !op.TakenOver("tg::c") {
		t.Fatal("takeover did not reach the router seam")
	}
	res = doReq(t, "POST", srv.URL+"/api/conversations/tg::c/release", consoleToken, "")
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("release status = %d, want 204", res.StatusCode)
	}
	if op.TakenOver("tg::c") {
		t.Fatal("release did not reach the router seam")
	}
}

func TestConsole_NewSessionEndpointNoAck(t *testing.T) {
	srv, store, op := consoleServer(t)
	seed(t, store, "tg::c", conversation.RoleUser, "hola", 10)

	res := doReq(t, "POST", srv.URL+"/api/conversations/tg::c/sessions", consoleToken, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("new-session status = %d, want 200", res.StatusCode)
	}
	out := decode[map[string]int](t, res)
	if out["session"] != 2 {
		t.Fatalf("session = %d, want 2", out["session"])
	}
	// FR-SESS-4: the console reset sends NO acknowledgement message.
	if n := len(op.Dispatched()); n != 0 {
		t.Fatalf("console reset dispatched %d envelopes, want 0", n)
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.15.1 block B — P2-1 at the wire. RED: written before any cure.
//
// An approval id is judged by its SHAPE at the door ("apr_" + 32 lowercase
// hex, what action.NewApprovalID mints). A malformed id names no request, so
// the route answers the registered `not_found` and the seam is never called:
// no store read, no decision act, nothing built from bytes nobody minted.
//
// Evidence level: the real mux and the real handlers over httptest, with a
// counting seam in place of the store. Proves what the route forwards; never
// what a store would have done.

package controlapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Sebastian197/korvun/internal/controlapi"
	"github.com/Sebastian197/korvun/internal/conversation"
)

// countingApprovals records every call that reaches the seam.
type countingApprovals struct{ calls atomic.Int64 }

func (c *countingApprovals) ListPending(context.Context) (controlapi.ApprovalList, error) {
	c.calls.Add(1)
	return controlapi.ApprovalList{}, nil
}

func (c *countingApprovals) Detail(context.Context, string) (controlapi.ApprovalDetail, error) {
	c.calls.Add(1)
	return controlapi.ApprovalDetail{}, controlapi.ErrApprovalNotFound
}

func (c *countingApprovals) Approve(context.Context, string, string) (controlapi.ApprovalOutcome, error) {
	c.calls.Add(1)
	return controlapi.ApprovalOutcome{}, controlapi.ErrApprovalNotFound
}

func (c *countingApprovals) Reject(context.Context, string, string) (controlapi.ApprovalOutcome, error) {
	c.calls.Add(1)
	return controlapi.ApprovalOutcome{}, controlapi.ErrApprovalNotFound
}

// TestV0151B_P2_1_aMalformedIDNeverReachesTheSeam sends every malformed id to
// the three routes that carry one. Each must answer 404 `not_found` with the
// registered message and leave the seam uncalled. Today every one of them is
// forwarded.
//
// Probing mutation (planned): drop the shape check from one route ⇒ the seam
// counts its calls and that route's rows redden.
// Evidence: the real mux and handlers over httptest with a counting seam,
// in-process.
func TestV0151B_P2_1_aMalformedIDNeverReachesTheSeam(t *testing.T) {
	hex := strings.Repeat("ab", 16)
	ids := map[string]string{
		"quote injected":  "apr_" + hex + `","x":"y`,
		"word joiner":     "apr_" + hex + "⁠",
		"bidi override":   "apr_‮" + hex,
		"C1 control":      "apr_" + hex + "",
		"upper-case stem": "APR_" + hex,
		"upper-case hex":  "apr_" + strings.ToUpper(hex),
		"short":           "apr_" + hex[:30],
		"action id":       "act_" + hex,
	}
	for name, id := range ids {
		for _, route := range []struct{ method, suffix, body string }{
			{http.MethodGet, "", ""},
			{http.MethodPost, "/approve", `{"digest":"sha256:` + strings.Repeat("0", 64) + `"}`},
			{http.MethodPost, "/reject", `{"comment":"x"}`},
		} {
			t.Run(route.method+route.suffix+"/"+name, func(t *testing.T) {
				seam := &countingApprovals{}
				mux := http.NewServeMux()
				controlapi.RegisterApprovals(mux, approvalsToken, seam)
				srv := httptest.NewServer(mux)
				defer srv.Close()
				srvURL := srv.URL

				req, err := http.NewRequest(route.method,
					srvURL+"/api/approvals/"+url.PathEscape(id)+route.suffix,
					strings.NewReader(route.body))
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				req.Header.Set("Authorization", "Bearer "+approvalsToken)
				req.Header.Set("Content-Type", "application/json")
				res, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatalf("do: %v", err)
				}
				defer func() { _ = res.Body.Close() }()
				var body struct {
					Error string `json:"error"`
				}
				_ = json.NewDecoder(res.Body).Decode(&body)

				if n := seam.calls.Load(); n != 0 {
					t.Fatalf("the seam was called %d time(s) with an id no mint produces", n)
				}
				if res.StatusCode != http.StatusNotFound || body.Error != string(controlapi.OutcomeNotFound) {
					t.Fatalf("status %d error %q, want 404 %q", res.StatusCode, body.Error, controlapi.OutcomeNotFound)
				}
			})
		}
	}
}

// peers are the RemoteAddr values the moulds place on a request: two
// non-loopback peers (documentation ranges, RFC 5737 / RFC 3849) and the two
// loopback controls.
var remotePeers = []string{"192.0.2.10:40000", "[2001:db8::10]:40000"}
var loopbackPeers = []string{"127.0.0.1:40000", "[::1]:40000"}

// serveAs runs ONE request through the real mux with the given peer address.
// httptest.NewRecorder + mux.ServeHTTP is the only way to place a peer the
// test does not own a socket for; the handler sees exactly what net/http would
// hand it in r.RemoteAddr.
func serveAs(mux http.Handler, method, path, body, remote string) (int, string) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = remote
	req.Header.Set("Authorization", "Bearer "+approvalsToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

// TestV0151B_P2_7_theSeamIsNeverCalledForARemotePeer is the oracle the socket
// mould cannot give: whether a route CALLED the seam. A handler that decides
// and then writes 403 passes any status-only mould; here the counting seam
// records every call, reads included, so «refuses before the seam» is observed,
// not inferred. Loopback peers are the controls: they must reach the seam.
//
// Evidence: the real mux and handlers, in-process; the peer address is set on
// the request by the test (not a socket).
// Probing mutation (planned): refuse after calling the seam ⇒ the remote rows
// red.
func TestV0151B_P2_7_theSeamIsNeverCalledForARemotePeer(t *testing.T) {
	id := "apr_" + strings.Repeat("ab", 16)
	routes := []struct{ method, path, body string }{
		{http.MethodGet, "/api/approvals", ""},
		{http.MethodGet, "/api/approvals/" + id, ""},
		{http.MethodPost, "/api/approvals/" + id + "/approve", `{"digest":"sha256:` + strings.Repeat("0", 64) + `"}`},
		{http.MethodPost, "/api/approvals/" + id + "/reject", `{"comment":"x"}`},
	}
	for _, r := range routes {
		for _, peer := range remotePeers {
			t.Run("remote "+peer+" "+r.method+" "+r.path, func(t *testing.T) {
				seam := &countingApprovals{}
				mux := http.NewServeMux()
				controlapi.RegisterApprovals(mux, approvalsToken, seam)
				status, body := serveAs(mux, r.method, r.path, r.body, peer)
				if n := seam.calls.Load(); n != 0 {
					t.Fatalf("the seam was called %d time(s) for a non-loopback peer", n)
				}
				if status != http.StatusForbidden || !strings.Contains(body, `"error":"loopback_only"`) {
					t.Fatalf("status %d body %s, want 403 loopback_only", status, body)
				}
			})
		}
		for _, peer := range loopbackPeers {
			t.Run("loopback "+peer+" "+r.method+" "+r.path, func(t *testing.T) {
				seam := &countingApprovals{}
				mux := http.NewServeMux()
				controlapi.RegisterApprovals(mux, approvalsToken, seam)
				_, _ = serveAs(mux, r.method, r.path, r.body, peer)
				if n := seam.calls.Load(); n != 1 {
					t.Fatalf("control: the seam was called %d time(s) for a loopback peer, want 1", n)
				}
			})
		}
	}
}

// TestV0151B_P2_7_sister_theConsoleMessageRefusesARemotePeer: the director put
// the console sister IN block B. POST /api/conversations/{key}/message
// dispatches a console-channel envelope that the core's provenance registry
// signs CredentialLoopbackInProcess, whoever the peer was. It answers 403
// `loopback_only` to a non-loopback peer and dispatches nothing; loopback
// peers are the controls.
//
// Evidence: the real mux and console handlers over an in-memory session store
// and a recording router, in-process; the peer address is set by the test.
// Probing mutation (planned): drop the peer check from the route ⇒ remote rows
// red.
func TestV0151B_P2_7_sister_theConsoleMessageRefusesARemotePeer(t *testing.T) {
	path := "/api/conversations/console::chat-1/message"
	for _, peer := range append(append([]string{}, remotePeers...), loopbackPeers...) {
		remote := peer == remotePeers[0] || peer == remotePeers[1]
		t.Run(peer, func(t *testing.T) {
			op := newFakeOpRouter()
			mux := http.NewServeMux()
			controlapi.RegisterConsole(mux, consoleToken, conversation.NewMemStore(), op)
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"text":"hola"}`))
			req.RemoteAddr = peer
			req.Header.Set("Authorization", "Bearer "+consoleToken)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if remote {
				if n := len(op.Inbound()); n != 0 {
					t.Fatalf("a non-loopback peer's message was dispatched (%d)", n)
				}
				if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"error":"loopback_only"`) {
					t.Fatalf("status %d body %s, want 403 loopback_only", rec.Code, rec.Body.String())
				}
				return
			}
			if n := len(op.Inbound()); n != 1 {
				t.Fatalf("control: a loopback peer's message dispatched %d time(s), want 1", n)
			}
		})
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The OTHER path that runs an irreversible tool — the eighth adversarial
// pass's P1-2.
//
// The approvals path is not the only one. When the gate does not park —
// approvals off, no ceiling on the brain, a class below the bar — this is the
// path, and it closed StateFailed for every tool error. A webhook_call whose
// POST the host had already received was recorded as a refusal, with this
// file's own comment saying «the tool refused before any effect».
//
// Evidence level, honest: in-process, the REAL webhook_call tool through the
// REAL runTool seam, against a real loopback server. Not a compiled binary.
package brain

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/tool"
)

// webhookHarness wires the REAL caged tool at `host` into the agent's tool
// registry, with a recorder that captures the state each attempt closes on.
func webhookHarness(t *testing.T, host string) (*AgentBrain, *resultFakeRecorder) {
	t.Helper()
	wc, err := tool.WebhookCall(tool.WebhookCallConfig{AllowHosts: []string{host}})
	if err != nil {
		t.Fatalf("wire the tool: %v", err)
	}
	journal := &[]string{}
	rec := &resultFakeRecorder{fakeRecorder: fakeRecorder{journal: journal}}
	a := NewAgentBrain(&scriptedModel{}, tool.Registry{"webhook_call": wc},
		WithAgentLogger(quietLogger()), WithActionRecorder(rec))
	return a, rec
}

// TestRunTool_aDeliveredPostNeverClosesFailed drives the plainest shape: the
// host reads the whole POST and hangs up.
//
// Probing mutation (executed, red, declared in the canto): restore
// `a.finishAction(ctx, actionID, action.StateFailed, "")` in runTool's error
// arms ⇒ this reddens.
func TestRunTool_aDeliveredPostNeverClosesFailed(t *testing.T) {
	t.Parallel()
	read := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		select {
		case read <- string(b):
		default:
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			return
		}
		_ = conn.Close()
	}))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	defer srv.Close()

	a, rec := webhookHarness(t, strings.TrimPrefix(srv.URL, "http://"))
	a.runTool(context.Background(), kernelEnv(), nil, laneText, "webhook_call", srv.URL+` {"event":"ping"}`)

	select {
	case got := <-read:
		if !strings.Contains(got, "ping") {
			t.Fatalf("the host did not receive the payload: %q", got)
		}
	default:
		t.Fatal("the POST never reached the server — this is not the post-delivery branch")
	}
	if len(rec.finishes) != 1 {
		t.Fatalf("the attempt must close exactly once: %v", rec.finishes)
	}
	if rec.finishes[0] == action.StateFailed {
		t.Fatal("the ledger closed FAILED over a POST the host had already read in full")
	}
	if rec.finishes[0] != action.StateOutcomeUnknown {
		t.Fatalf("state = %v, want OUTCOME_UNKNOWN", rec.finishes[0])
	}
}

// TestRunTool_aRefusedRedirectClosesUnknownAndStillAuditsAsDenied is the case
// the seventh pass's cure made worse by accident: ErrRedirectRefused wraps
// ErrCageViolation on purpose, so it arrives at the branch whose comment said
// «the tool refused before any effect» and whose close was FAILED.
//
// Both facts are true at once and the record carries both: the CAGE refused it
// — the audit rule is still "cage" — and the receiver answered (it read the
// POST before its 3xx, in this test), so the state is the honest unknown.
// (Wording corrected in v0.15.1 block A; assertions unchanged.)
//
// Probing mutation (executed, red, declared in the canto): judge the state by
// `breached` instead of by delivery ⇒ this reddens.
func TestRunTool_aRefusedRedirectClosesUnknownAndStillAuditsAsDenied(t *testing.T) {
	t.Parallel()
	read := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		select {
		case read <- string(b):
		default:
		}
		http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	wc, err := tool.WebhookCall(tool.WebhookCallConfig{AllowHosts: []string{host}})
	if err != nil {
		t.Fatalf("wire: %v", err)
	}
	pub := &spyPublisher{}
	journal := &[]string{}
	rec := &resultFakeRecorder{fakeRecorder: fakeRecorder{journal: journal}}
	a := NewAgentBrain(&scriptedModel{}, tool.Registry{"webhook_call": wc},
		WithAgentLogger(quietLogger()), WithActionRecorder(rec),
		WithAgentToolAudit(pub, "agent-1"))
	a.runTool(context.Background(), kernelEnv(), nil, laneText, "webhook_call", srv.URL+`/hook {"event":"fire"}`)

	select {
	case got := <-read:
		if !strings.Contains(got, "fire") {
			t.Fatalf("the host did not receive the payload: %q", got)
		}
	default:
		t.Fatal("the POST never reached the server")
	}
	if len(rec.finishes) != 1 || rec.finishes[0] != action.StateOutcomeUnknown {
		t.Fatalf("state = %v, want OUTCOME_UNKNOWN over a delivered POST", rec.finishes)
	}
	events := pub.snapshot()
	if len(events) != 1 || events[0].Rule != "cage" || events[0].Outcome != "denied" {
		t.Fatalf("the cage's rule is a separate fact and must survive: %+v", events)
	}
}

// TestRunTool_aDialThatNeverConnectsStillClosesFailed keeps the cure honest in
// the other direction: without it, closing every tool error as unknown would
// pass both tests above while telling an operator «we do not know» about a
// call that never left the machine.
//
// Probing mutation (executed, red, declared in the canto): close
// OUTCOME_UNKNOWN unconditionally ⇒ this reddens.
func TestRunTool_aDialThatNeverConnectsStillClosesFailed(t *testing.T) {
	t.Parallel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	a, rec := webhookHarness(t, addr)
	a.runTool(context.Background(), kernelEnv(), nil, laneText, "webhook_call", "http://"+addr+` {"event":"ping"}`)
	if len(rec.finishes) != 1 || rec.finishes[0] != action.StateFailed {
		t.Fatalf("state = %v, want FAILED — nothing reached the wire", rec.finishes)
	}
}

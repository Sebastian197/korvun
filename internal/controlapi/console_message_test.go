// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// FR-CONS-3 red: the direct-chat send — a USER envelope (never operator)
// into the full dispatch pipeline, console-channel keys only.
package controlapi_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/router"
)

func TestConsole_UserMessageEndpoint(t *testing.T) {
	srv, _, op := consoleServer(t)

	res := doReq(t, "POST", srv.URL+"/api/conversations/console::chat-1/message",
		consoleToken, `{"text":"hola korvun"}`)
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("message = %d, want 202 (async brain)", res.StatusCode)
	}
	in := op.Inbound()
	if len(in) != 1 {
		t.Fatalf("DispatchInbound calls = %d, want 1", len(in))
	}
	env := in[0]
	if env.Channel != "console" || env.Direction != envelope.Inbound {
		t.Fatalf("envelope = %+v, want inbound on console", env)
	}
	if env.Meta["conversation.id"] != "chat-1" {
		t.Fatalf("conversation identity lost: %+v", env.Meta)
	}
	if len(env.Parts) != 1 || env.Parts[0].Content != "hola korvun" {
		t.Fatalf("text = %+v", env.Parts)
	}
	// The human is USER here — the sender must never read as operator.
	if env.Sender.ID == "operator" {
		t.Fatalf("sender = %+v — the direct chat speaks as the user", env.Sender)
	}
}

func TestConsole_UserMessageValidation(t *testing.T) {
	srv, _, op := consoleServer(t)
	cases := []struct {
		name, path, body string
		want             int
	}{
		{"non-console channel", "/api/conversations/telegram::c/message", `{"text":"x"}`, http.StatusBadRequest},
		{"empty text", "/api/conversations/console::c/message", `{"text":" "}`, http.StatusBadRequest},
		{"bad key", "/api/conversations/console::/message", `{"text":"x"}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := doReq(t, "POST", srv.URL+tc.path, consoleToken, tc.body)
			if res.StatusCode != tc.want {
				t.Fatalf("%s = %d, want %d", tc.path, res.StatusCode, tc.want)
			}
		})
	}
	if n := len(op.Inbound()); n != 0 {
		t.Fatalf("invalid messages reached dispatch %d times", n)
	}
	// Bearer required.
	res := doReq(t, "POST", srv.URL+"/api/conversations/console::c/message", "", `{"text":"x"}`)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no bearer = %d, want 401", res.StatusCode)
	}
}

func TestConsole_UserMessageErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{
		{router.ErrNoRoute, http.StatusConflict},
		{router.ErrBrainSaturated, http.StatusServiceUnavailable},
		{context.DeadlineExceeded, http.StatusInternalServerError},
	} {
		srv, _, op := consoleServer(t)
		op.inboundErr = tc.err
		res := doReq(t, "POST", srv.URL+"/api/conversations/console::c/message",
			consoleToken, `{"text":"x"}`)
		if res.StatusCode != tc.want {
			t.Fatalf("err %v = %d, want %d", tc.err, res.StatusCode, tc.want)
		}
	}
}

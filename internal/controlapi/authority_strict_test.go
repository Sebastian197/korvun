// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package controlapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/controlapi"
	"github.com/Sebastian197/korvun/internal/conversation"
)

func TestConsole_StrictModeWithoutIssuerRejects(t *testing.T) {
	op := newFakeOpRouter()
	mux := http.NewServeMux()
	controlapi.RegisterStrictConsole(mux, consoleToken, conversation.NewMemStore(), op, nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost,
		"http://127.0.0.1/api/conversations/console::strict/message",
		strings.NewReader(`{"text":"must authenticate"}`))
	request.RemoteAddr = "127.0.0.1:41700"
	request.Header.Set("Authorization", "Bearer "+consoleToken)
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable ||
		!strings.Contains(recorder.Body.String(), "identity unavailable") {
		t.Fatalf("response = %d %q", recorder.Code, recorder.Body.String())
	}
	if got := len(op.Inbound()); got != 0 {
		t.Fatalf("strict issuer-less dispatches = %d", got)
	}
}

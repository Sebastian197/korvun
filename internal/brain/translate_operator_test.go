// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// SP1 red (operator-console spec 2026-08-08, FR-STORE-2): a history
// containing operator turns must reach the providers with the operator
// turns translated to the ASSISTANT role — the explicit switch arm Chano
// resolved on 2026-08-08 (clarification #2). Without it, toModelRole's
// default would silently present the operator's words as the USER's.
package brain

import (
	"testing"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/model"
)

func TestToModelRole_OperatorMapsToAssistant(t *testing.T) {
	if got := toModelRole(conversation.RoleOperator); got != model.RoleAssistant {
		t.Fatalf("toModelRole(operator) = %q, want %q", got, model.RoleAssistant)
	}
}

func TestRequestWithHistory_OperatorTurnArrivesAsAssistant(t *testing.T) {
	in := envelope.New("telegram", envelope.Inbound, envelope.Participant{ID: "u1"}).
		AddText("¿sigues ahí?")
	history := []conversation.Turn{
		{Role: conversation.RoleUser, Content: "hola"},
		{Role: conversation.RoleOperator, Content: "aquí Chano, te atiendo yo"},
	}
	req, ok := requestWithHistory(in, "", history)
	if !ok {
		t.Fatal("requestWithHistory reported nothing to ask")
	}
	if len(req.Messages) != 3 {
		t.Fatalf("messages = %d, want 3 (history ×2 + current)", len(req.Messages))
	}
	if req.Messages[1].Role != model.RoleAssistant {
		t.Fatalf("operator turn reached the provider as %q, want %q",
			req.Messages[1].Role, model.RoleAssistant)
	}
	if req.Messages[1].Content != "aquí Chano, te atiendo yo" {
		t.Fatalf("operator content mangled: %q", req.Messages[1].Content)
	}
}

// 2026-08-09 red (the autonomous round's gatekeeper-leak catch): system turns
// in the store are OPERATOR-FACING notices persisted by the console ack path
// ("New session started…", the /tools gatekeeper report). They are UI, not
// dialogue — replaying them as mid-conversation system messages handed the
// model the tool catalog in text form and it imitated the syntax instead of
// calling natively. History system turns must be SKIPPED entirely.
func TestRequestWithHistory_SystemTurnsAreSkipped(t *testing.T) {
	in := envelope.New("console", envelope.Inbound, envelope.Participant{ID: "u1"}).
		AddText("descarga esa página")
	history := []conversation.Turn{
		{Role: conversation.RoleUser, Content: "hola"},
		{Role: conversation.RoleAssistant, Content: "hola, dime"},
		{Role: conversation.RoleSystem, Content: "Gatekeeper — brain \"default\"\nTools:\n- http_fetch: allow [shield]"},
	}
	req, ok := requestWithHistory(in, "", history)
	if !ok {
		t.Fatal("requestWithHistory reported nothing to ask")
	}
	if len(req.Messages) != 3 {
		t.Fatalf("messages = %d, want 3 (2 dialogue turns + current; the system notice dropped)", len(req.Messages))
	}
	for i, m := range req.Messages {
		if m.Role == model.RoleSystem {
			t.Fatalf("message %d is a system turn leaked from history: %q", i, m.Content)
		}
	}
}

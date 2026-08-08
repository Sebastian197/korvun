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

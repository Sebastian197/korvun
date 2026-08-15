// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/Sebastian197/korvun/internal/bus"
	"github.com/Sebastian197/korvun/internal/model"
)

// RT-3 live sub-phase: the lane is picked by the ADAPTER's Go capability,
// so a model WITHOUT tool support (raw evidence: ollama 400 "does not
// support tools", deterministic) hit the native lane and every message
// degraded to the canned fallback. The contract now: on
// model.ErrToolsUnsupported the brain DEGRADES to the prompt-protocol text
// lane — answering the SAME message — flips a sticky process-lifetime flag,
// says so on the observable surfaces (structured warn + ONE audited
// metadata-only event), and never calls the native lane again.

// noToolsModel refuses the native lane like the real provider does, and
// answers the plain lane like the real model does.
type noToolsModel struct {
	nativeCalls atomic.Int64
	plainCalls  atomic.Int64
}

func (m *noToolsModel) Name() string { return "ollama" }
func (m *noToolsModel) Generate(context.Context, *model.Request) (*model.Response, error) {
	m.plainCalls.Add(1)
	return &model.Response{
		Message:   model.Message{Role: model.RoleAssistant, Content: "respuesta por el carril de texto"},
		Provider:  "ollama",
		ModelName: "gemma3:270m",
	}, nil
}
func (m *noToolsModel) GenerateWithTools(context.Context, *model.Request, []model.ToolSpec) (*model.Response, error) {
	m.nativeCalls.Add(1)
	return nil, fmt.Errorf("ollama: %w: status 400: does not support tools", model.ErrToolsUnsupported)
}

func TestNativeLane_toolsUnsupportedDegradesToTextLane(t *testing.T) {
	t.Parallel()
	m := &noToolsModel{}
	pub := &spyPublisher{}
	a := NewAgentBrain(m, spyRegistry(&spyTool{}),
		WithAgentLogger(quietLogger()), WithAgentToolAudit(pub, "agente-1"),
		WithAgentFallback("fallback enlatado"))

	// FIRST message: the capability refusal must degrade, not fail — the
	// user gets the text-lane answer for THIS message.
	out, err := a.Handle(context.Background(), inboundText("console", "c", "hola"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(out) == 0 || out[0].Parts[0].Content != "respuesta por el carril de texto" {
		t.Fatalf("reply = %+v, want the TEXT-lane answer, never the canned fallback", out)
	}

	// The degradation is OBSERVABLE and metadata-only: exactly one audited
	// event with finite labels naming the surface.
	events := pub.snapshot()
	if len(events) != 1 || events[0].Type != bus.ToolDenied ||
		events[0].Tool != "native_lane" || events[0].Rule != "tools_unsupported" {
		t.Fatalf("audit = %+v, want exactly one tool_denied tool=native_lane rule=tools_unsupported", events)
	}

	// STICKY: the second message goes straight to the text lane — the
	// native call count stays at 1 for the process lifetime.
	if _, err := a.Handle(context.Background(), inboundText("console", "c", "otra")); err != nil {
		t.Fatalf("second Handle: %v", err)
	}
	if got := m.nativeCalls.Load(); got != 1 {
		t.Fatalf("native calls = %d, want 1 (sticky degradation)", got)
	}
	if got := m.plainCalls.Load(); got < 2 {
		t.Fatalf("plain calls = %d, want >= 2 (both messages answered by the text lane)", got)
	}
	// And no duplicate degradation events on the second message.
	if got := len(pub.snapshot()); got != 1 {
		t.Fatalf("audit events after second Handle = %d, want still 1", got)
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/bus"
	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/model"
)

// B13 — protocol JSON never reaches a channel (spec
// 2026-08-29-b13-protocol-json-guard.md). The 2026-08-23 Telegram incident:
// llama3.2 answered `{"name":"avisos-caseros",…}` — a tool registered
// NOWHERE — as plain text; the text lane's parseReply only knows the
// "TOOL: name(args)" grammar, the native lane's rescueTextToolCall only
// rescues REGISTERED names, and both documented fail-opens let the raw
// protocol JSON reach the user with zero log trace. These tests pin the
// guard at Handle's common exit seam: whole-body tool-call-shaped JSON is
// replaced by the honest error, observably, without poisoning memory.

// leakJSON is the exact incident shape (AS-1).
const leakJSON = `{"name": "avisos-caseros", "parameters": {"aviso": "regar las plantas"}}`

// wantProtocolLeakReply pins the user-facing honest error VERBATIM (FR-B13-2,
// house register): the implementation's constant must match byte-for-byte.
const wantProtocolLeakReply = "Sorry, the model produced an internal tool request instead of an answer. Please try again."

// assertLeakBlocked asserts the outbound reply carries the honest error and
// no fragment of the protocol JSON (AS-1's Then).
func assertLeakBlocked(t *testing.T, got string) {
	t.Helper()
	if strings.Contains(got, "avisos-caseros") || strings.Contains(got, `"name"`) {
		t.Fatalf("protocol JSON reached the channel: %q", got)
	}
	if got != wantProtocolLeakReply {
		t.Fatalf("reply = %q, want the honest protocol-leak error %q", got, wantProtocolLeakReply)
	}
}

func TestHandle_blocksUnregisteredToolCallJSON_textLane(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	m := &scriptedModel{name: "m", replies: []string{leakJSON}}
	a := NewAgentBrain(m, builtinRegistry(),
		WithAgentLogger(slog.New(slog.NewTextHandler(&buf, nil))))

	out, err := a.Handle(context.Background(), inboundText("telegram", "c", "hola"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	assertLeakBlocked(t, latestText(out[0].Parts))
	// FR-B13-3: one structured WARN with the phantom tool name and channel.
	log := buf.String()
	if !strings.Contains(log, "WARN") || !strings.Contains(log, "avisos-caseros") || !strings.Contains(log, "telegram") {
		t.Fatalf("no WARN naming the phantom tool + channel in the log:\n%s", log)
	}
}

func TestHandle_blocksUnregisteredToolCallJSON_nativeLane(t *testing.T) {
	t.Parallel()
	spy := &spyTool{}
	m := &nativeScriptedModel{name: "n", replies: []model.Message{finalReply(leakJSON)}}
	a := NewAgentBrain(m, spyRegistry(spy), WithAgentLogger(quietLogger()))

	out, err := a.Handle(context.Background(), inboundText("telegram", "c", "hola"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	assertLeakBlocked(t, latestText(out[0].Parts))
	if spy.count() != 0 {
		t.Fatalf("the unregistered leak executed a tool %d times", spy.count())
	}
}

// TestHandle_guardShapeTable pins the conservative detector at the Handle
// seam (FR-B13-1/5): only a whole-body JSON object with "name" + one of
// "arguments"/"parameters"/"args" is blocked; everything else passes.
func TestHandle_guardShapeTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		label   string
		reply   string
		blocked bool
	}{
		{"parameters key", `{"name":"ghost","parameters":{"x":1}}`, true},
		{"arguments key", `{"name":"ghost","arguments":{"x":1}}`, true},
		{"args string key", `{"name":"ghost","args":"x"}`, true},
		{"fenced whole-body call (AS-5)", "```json\n{\"name\":\"ghost\",\"arguments\":{}}\n```", true},
		{"whitespace-padded call", "\n  {\"name\":\"ghost\",\"parameters\":{}}  \n", true},
		{"JSON amid prose (AS-3, the pinned edge)", "Claro, aquí tienes el ejemplo:\n{\"name\":\"ghost\",\"parameters\":{}}\nCámbialo a tu gusto.", false},
		{"plain JSON answer, no args key (AS-4)", `{"name": "Chano", "city": "Sevilla"}`, false},
		{"name not a string", `{"name": 3, "parameters": {}}`, false},
		{"JSON array", `[{"name":"ghost","parameters":{}}]`, false},
		{"ordinary prose", "hola, ¿en qué te ayudo?", false},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()
			m := &scriptedModel{name: "m", replies: []string{tc.reply}}
			a := NewAgentBrain(m, builtinRegistry(), WithAgentLogger(quietLogger()))
			out, err := a.Handle(context.Background(), inboundText("console", "c", "hola"))
			if err != nil {
				t.Fatalf("Handle: %v", err)
			}
			got := latestText(out[0].Parts)
			if tc.blocked {
				assertLeakBlocked(t, got)
			} else if got != tc.reply {
				t.Fatalf("legitimate reply mangled: got %q, want %q untouched", got, tc.reply)
			}
		})
	}
}

// TestHandle_guardedReplyNotPersisted — FR-B13-4: the leaked JSON must not
// enter conversation memory (nor the canned error as the assistant's turn).
func TestHandle_guardedReplyNotPersisted(t *testing.T) {
	t.Parallel()
	store := conversation.NewMemStore()
	m := &scriptedModel{name: "m", replies: []string{leakJSON}}
	a := NewAgentBrain(m, builtinRegistry(),
		WithAgentLogger(quietLogger()),
		WithAgentConversationStore(store, 4))

	env := inboundText("telegram", "c", "hola")
	env.Meta[conversation.MetaConversationID] = "conv-1"
	if _, err := a.Handle(context.Background(), env); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	key, err := conversation.KeyFromEnvelope(env)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	turns, err := store.LoadRecent(context.Background(), key, 10)
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	if len(turns) != 0 {
		t.Fatalf("guarded reply persisted %d turns, want 0: %+v", len(turns), turns)
	}
}

// TestHandle_guardAuditsWithFiniteLabels — FR-B13-3: one metadata-only
// audit event on the shared surfaces, FINITE labels only (the raw name is
// model-controlled and stays in the bounded local log).
func TestHandle_guardAuditsWithFiniteLabels(t *testing.T) {
	t.Parallel()
	pub := &spyPublisher{}
	m := &scriptedModel{name: "m", replies: []string{leakJSON}}
	a := NewAgentBrain(m, builtinRegistry(),
		WithAgentLogger(quietLogger()),
		WithAgentToolAudit(pub, "casa"))

	if _, err := a.Handle(context.Background(), inboundText("telegram", "c", "hola")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	events := pub.snapshot()
	found := false
	for _, ev := range events {
		if ev.Type == bus.ToolDenied && ev.Rule == "protocol_leak" {
			found = true
			if ev.Tool != "unknown" {
				t.Fatalf("audit Tool label = %q, want the finite \"unknown\" (raw name is model-controlled)", ev.Tool)
			}
			if ev.Brain != "casa" || ev.Channel != "telegram" {
				t.Fatalf("audit metadata incomplete: %+v", ev)
			}
		}
	}
	if !found {
		t.Fatalf("no ToolDenied/protocol_leak audit event published: %+v", events)
	}
}

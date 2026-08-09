// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package bus

import (
	"context"
	"testing"
	"time"
)

// The three governed-tool EventTypes (ADR-0041 §5, spec FR-AUD-1/2/4) and the
// additive metadata fields they ride on. METADATA-ONLY law: the Event carries
// tool/outcome/rule/latency — never tool args (there is deliberately no field
// for them, so the law holds by construction).

func TestToolEventTypes_String(t *testing.T) {
	t.Parallel()
	cases := []struct {
		typ  EventType
		want string
	}{
		{ToolUsed, "tool_used"},
		{ToolDenied, "tool_denied"},
		{ToolShadowed, "tool_shadowed"},
	}
	for _, tc := range cases {
		if got := tc.typ.String(); got != tc.want {
			t.Errorf("EventType(%d).String() = %q, want %q", int(tc.typ), got, tc.want)
		}
	}
}

// The additive tool-metadata fields round-trip through Publish/Subscribe
// untouched.
func TestToolEvent_metadataRoundTrips(t *testing.T) {
	t.Parallel()
	b := New()
	defer b.Close()

	got := make(chan Event, 1)
	unsub := b.Subscribe(ToolUsed, func(ev Event) { got <- ev })
	defer unsub()

	sent := Event{
		Type:    ToolUsed,
		Channel: "telegram",
		Brain:   "agent-1",
		Tool:    "http_fetch",
		Outcome: "ok",
		Rule:    "",
		Latency: 250 * time.Millisecond,
	}
	b.Publish(context.Background(), sent)

	select {
	case ev := <-got:
		if ev.Tool != "http_fetch" || ev.Outcome != "ok" || ev.Rule != "" || ev.Latency != 250*time.Millisecond {
			t.Fatalf("tool metadata mangled in transit: %+v", ev)
		}
		if ev.Brain != "agent-1" || ev.Channel != "telegram" {
			t.Fatalf("routing metadata mangled in transit: %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber never received the tool event")
	}
}

// Per-type isolation holds for the new types too: a ToolDenied subscriber
// never sees ToolUsed events (the same Subscribe contract the lifecycle
// types carry).
func TestToolEvent_perTypeIsolation(t *testing.T) {
	t.Parallel()
	b := New()
	defer b.Close()

	denied := make(chan Event, 1)
	unsub := b.Subscribe(ToolDenied, func(ev Event) { denied <- ev })
	defer unsub()

	b.Publish(context.Background(), Event{Type: ToolUsed, Tool: "calc", Outcome: "ok"})
	b.Publish(context.Background(), Event{Type: ToolDenied, Tool: "calc", Outcome: "denied", Rule: "channel"})

	select {
	case ev := <-denied:
		if ev.Type != ToolDenied || ev.Rule != "channel" {
			t.Fatalf("wrong event crossed the per-type wall: %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ToolDenied subscriber never received its event")
	}
	select {
	case ev := <-denied:
		t.Fatalf("ToolDenied subscriber received a second event: %+v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}

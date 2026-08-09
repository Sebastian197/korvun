// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/bus"
	"github.com/Sebastian197/korvun/internal/config"
)

// The gatekeeper report + the bounded in-memory ring behind /tools
// (FR-CHAT-1, mandate SP5.2). METADATA-ONLY law: the ring stores bus events
// (which carry no args by construction) and the report renders grants and
// event metadata — nothing content-derived.

func TestToolEventRing_boundedAndPerBrain(t *testing.T) {
	t.Parallel()
	ring := newToolEventRing(3, func() time.Time { return time.Unix(1000, 0).UTC() })

	for i := 0; i < 5; i++ {
		ring.record(bus.Event{Type: bus.ToolUsed, Brain: "a", Tool: "calc", Outcome: "ok"})
	}
	ring.record(bus.Event{Type: bus.ToolDenied, Brain: "b", Tool: "read_file", Outcome: "denied", Rule: "channel"})

	// Capacity 3: only the newest 3 entries survive; brain "a" kept 2 of
	// them after brain "b"'s entry displaced one.
	a := ring.lastFor("a", 10)
	if len(a) != 2 {
		t.Fatalf("lastFor(a) = %d entries, want 2 (bounded ring): %+v", len(a), a)
	}
	bEvents := ring.lastFor("b", 10)
	if len(bEvents) != 1 || bEvents[0].ev.Rule != "channel" {
		t.Fatalf("lastFor(b) = %+v, want the denied event with its rule", bEvents)
	}
	if none := ring.lastFor("c", 10); len(none) != 0 {
		t.Fatalf("lastFor(c) = %+v, want empty", none)
	}
}

func TestToolEventRing_subscribesToTheBus(t *testing.T) {
	t.Parallel()
	b := bus.New()
	defer b.Close()
	ring := newToolEventRing(8, time.Now)
	ring.subscribe(b)

	b.Publish(context.Background(), bus.Event{Type: bus.ToolUsed, Brain: "agent", Tool: "calc", Outcome: "ok"})
	b.Publish(context.Background(), bus.Event{Type: bus.ToolShadowed, Brain: "agent", Tool: "http_fetch", Outcome: "shadowed"})

	deadline := time.After(2 * time.Second)
	for {
		if len(ring.lastFor("agent", 10)) == 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("ring never received the bus events: %+v", ring.lastFor("agent", 10))
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestGatekeeperReport_rendersGrantsAndActivity(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	bc := governedAgentBrain(&config.AgentConfig{
		Tools: []string{"calc", "read_file", "http_fetch"},
		Governance: []config.ToolGrantConfig{
			{Tool: "calc", Mode: "allow"},
			{Tool: "read_file", Mode: "allow", Channels: []string{"console"}},
			{Tool: "http_fetch", Mode: "shadow"},
		},
		ReadFile:  &config.ReadFileToolConfig{Root: root},
		HTTPFetch: &config.HTTPFetchToolConfig{AllowHosts: []string{"127.0.0.1"}},
	})
	ring := newToolEventRing(8, func() time.Time { return time.Unix(1000, 0).UTC() })
	ring.record(bus.Event{Type: bus.ToolUsed, Brain: "agent", Tool: "calc", Outcome: "ok", Latency: 3 * time.Millisecond, Channel: "console"})
	ring.record(bus.Event{Type: bus.ToolDenied, Brain: "agent", Tool: "http_fetch", Outcome: "denied", Rule: "private_network_shield", Channel: "console"})

	report := gatekeeperReport(bc, ring)

	for _, want := range []string{
		`brain "agent"`,
		"private",
		"calc: allow",
		"read_file: allow",
		"channels: console",
		"sensitive", // read_file house default
		"http_fetch: shadow",
		"shield", // network tool on a private brain
		"tool_used calc ok",
		"tool_denied http_fetch denied private_network_shield",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q:\n%s", want, report)
		}
	}
}

// An ungoverned agent brain reports honestly: every listed tool allowed.
func TestGatekeeperReport_ungovernedIsHonest(t *testing.T) {
	t.Parallel()
	bc := governedAgentBrain(&config.AgentConfig{Tools: []string{"calc", "echo"}})
	ring := newToolEventRing(8, time.Now)

	report := gatekeeperReport(bc, ring)
	if !strings.Contains(report, "ungoverned") {
		t.Fatalf("ungoverned report must say so:\n%s", report)
	}
	if !strings.Contains(report, "calc") || !strings.Contains(report, "echo") {
		t.Fatalf("ungoverned report must still list the tools:\n%s", report)
	}
}

// A non-agent brain reports that it has no tools.
func TestGatekeeperReport_nonAgentBrain(t *testing.T) {
	t.Parallel()
	bc := ollamaBrain()
	ring := newToolEventRing(8, time.Now)

	report := gatekeeperReport(bc, ring)
	if !strings.Contains(report, "no tools") {
		t.Fatalf("non-agent report must say it has no tools:\n%s", report)
	}
}

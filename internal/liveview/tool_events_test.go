// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package liveview

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/bus"
	"github.com/Sebastian197/korvun/internal/envelope"
)

// readFrameWithin reads one SSE frame with a bounded wait, so a missing frame
// fails the test instead of hanging it (and the deferred Body.Close can run).
func readFrameWithin(t *testing.T, br interface{ ReadString(byte) (string, error) }, d time.Duration) string {
	t.Helper()
	got := make(chan string, 1)
	go func() {
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return // connection torn down — the select below reports the timeout
			}
			line = strings.TrimRight(line, "\r\n")
			if strings.HasPrefix(line, "data: ") {
				got <- strings.TrimPrefix(line, "data: ")
				return
			}
		}
	}()
	select {
	case frame := <-got:
		return frame
	case <-time.After(d):
		t.Fatalf("no SSE frame within %v", d)
		return ""
	}
}

// FR-AUD-1 pre-cut verification: an UNKNOWN EventType published on the bus
// must neither crash nor pollute an existing SSE consumer — the bus Subscribe
// is per-type, so a type nobody subscribed to simply does not arrive, and the
// stream stays alive for the types the consumer did subscribe to.
func TestSSE_unknownEventTypeIsTolerated(t *testing.T) {
	b := bus.New()
	defer b.Close()
	lv := New(b, WithNow(fixedNow))
	srv := newTestServer(t, lv)

	resp, br := connectSSE(t, srv)
	defer func() { _ = resp.Body.Close() }()

	// A type far outside the known set: nobody subscribes to it.
	b.Publish(context.Background(), bus.Event{Type: bus.EventType(99), Channel: "telegram"})
	// A known type published AFTER it must still arrive — the stream survived.
	b.Publish(context.Background(), bus.Event{Type: bus.MessageReceived, Channel: "telegram"})

	frame := readFrameWithin(t, br, 2*time.Second)
	if !strings.Contains(frame, `"type":"message_received"`) {
		t.Fatalf("stream did not survive an unknown event type; got frame %s", frame)
	}
	if strings.Contains(frame, "unknown") {
		t.Fatalf("unknown-type event leaked into the stream: %s", frame)
	}
}

// toolEnvelope builds an inbound envelope whose CONTENT is a sentinel that
// must never appear in any frame (the ADR-0024 §1 law extended to tool
// events: metadata only, never args, never content).
func toolEnvelope() *envelope.Envelope {
	return &envelope.Envelope{
		ID:        "env-tool-1",
		Channel:   "telegram",
		Direction: envelope.Inbound,
		Parts:     []envelope.Part{{Type: envelope.Text, Content: "SECRETPAYLOAD delete /etc/passwd"}},
	}
}

// The three tool EventTypes stream as metadata-only frames (spec FR-AUD-1/2/4):
// tool, outcome, rule, latency — and NEVER the envelope content nor anything
// args-derived.
func TestSSE_toolUsedFrameIsMetadataOnly(t *testing.T) {
	b := bus.New()
	defer b.Close()
	lv := New(b, WithNow(fixedNow))
	srv := newTestServer(t, lv)

	resp, br := connectSSE(t, srv)
	defer func() { _ = resp.Body.Close() }()

	b.Publish(context.Background(), bus.Event{
		Type:     bus.ToolUsed,
		Envelope: toolEnvelope(),
		Channel:  "telegram",
		Brain:    "agent-1",
		Tool:     "http_fetch",
		Outcome:  "ok",
		Latency:  1500 * time.Millisecond,
	})

	frame := readFrameWithin(t, br, 2*time.Second)
	for _, want := range []string{
		`"type":"tool_used"`,
		`"brain":"agent-1"`,
		`"channel":"telegram"`,
		`"tool":"http_fetch"`,
		`"outcome":"ok"`,
		`"latency_ms":1500`,
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("tool_used frame missing %s:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "SECRETPAYLOAD") || strings.Contains(frame, "passwd") {
		t.Fatalf("tool_used frame leaked message content:\n%s", frame)
	}
}

func TestSSE_toolDeniedFrameCarriesRule(t *testing.T) {
	b := bus.New()
	defer b.Close()
	lv := New(b, WithNow(fixedNow))
	srv := newTestServer(t, lv)

	resp, br := connectSSE(t, srv)
	defer func() { _ = resp.Body.Close() }()

	b.Publish(context.Background(), bus.Event{
		Type:     bus.ToolDenied,
		Envelope: toolEnvelope(),
		Channel:  "discord",
		Brain:    "agent-1",
		Tool:     "read_file",
		Outcome:  "denied",
		Rule:     "private_network_shield",
	})

	frame := readFrameWithin(t, br, 2*time.Second)
	for _, want := range []string{
		`"type":"tool_denied"`,
		`"tool":"read_file"`,
		`"outcome":"denied"`,
		`"rule":"private_network_shield"`,
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("tool_denied frame missing %s:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "SECRETPAYLOAD") {
		t.Fatalf("tool_denied frame leaked message content:\n%s", frame)
	}
}

func TestSSE_toolShadowedFrameStreams(t *testing.T) {
	b := bus.New()
	defer b.Close()
	lv := New(b, WithNow(fixedNow))
	srv := newTestServer(t, lv)

	resp, br := connectSSE(t, srv)
	defer func() { _ = resp.Body.Close() }()

	b.Publish(context.Background(), bus.Event{
		Type:     bus.ToolShadowed,
		Envelope: toolEnvelope(),
		Channel:  "console",
		Brain:    "agent-1",
		Tool:     "webhook_call",
		Outcome:  "shadowed",
	})

	frame := readFrameWithin(t, br, 2*time.Second)
	for _, want := range []string{
		`"type":"tool_shadowed"`,
		`"tool":"webhook_call"`,
		`"outcome":"shadowed"`,
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("tool_shadowed frame missing %s:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "SECRETPAYLOAD") {
		t.Fatalf("tool_shadowed frame leaked message content:\n%s", frame)
	}
}

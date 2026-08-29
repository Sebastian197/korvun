// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package router_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/bus"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/router"
)

// B9 — the direct-brain conversation-id contract (spec
// 2026-08-29-b9-brain-selector-new-chat.md). A console conversation id of
// the form "b:<pct-encoded brain>:<rest>" dispatches to the NAMED brain on
// the enabled channel only; a vanished brain falls back to the route
// default with exactly one honest AckBrainFallback notice per
// conversation; every other channel ignores the prefix entirely (the
// privacy invariant — a fabricated webhook conversation_id must not
// bypass its route).

// spyEvents records published bus events (the received-event label of AS-1).
type spyEvents struct {
	mu     sync.Mutex
	events []bus.Event
}

func (s *spyEvents) Publish(_ context.Context, ev bus.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
}

func (s *spyEvents) received() []bus.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []bus.Event
	for _, ev := range s.events {
		if ev.Type == bus.MessageReceived {
			out = append(out, ev)
		}
	}
	return out
}

// directBrainRouter wires the AS-1 shape: console routes to "local" (the
// default), a second brain "cloud" is registered but NOT routed.
func directBrainRouter(t *testing.T, opts ...router.Option) (*router.Router, *fakeChannel, *fakeBrain, *fakeBrain) {
	t.Helper()
	r := router.New(opts...)
	ch := newFakeChannel("console")
	local, cloud := newFakeBrain(), newFakeBrain()
	if err := r.RegisterChannel(ch); err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}
	if err := r.RegisterBrain("local", local); err != nil {
		t.Fatalf("RegisterBrain local: %v", err)
	}
	if err := r.RegisterBrain("cloud", cloud); err != nil {
		t.Fatalf("RegisterBrain cloud: %v", err)
	}
	if err := r.Route("console", "local"); err != nil {
		t.Fatalf("Route: %v", err)
	}
	return r, ch, local, cloud
}

func TestDirectBrain_prefixedIDDispatchesToChosenBrain(t *testing.T) {
	events := &spyEvents{}
	r, _, local, cloud := directBrainRouter(t,
		router.WithDirectBrainChannel("console"),
		router.WithEventPublisher(events))
	defer shutdown(t, r)

	// The case that birthed the piece: two brains, the id names the cloud one.
	if err := r.DispatchInbound(context.Background(), mkInbound("console", "b:cloud:chat-1", "hola")); err != nil {
		t.Fatalf("DispatchInbound (prefixed): %v", err)
	}
	eventually(t, time.Second, func() bool { return len(cloud.Handled()) == 1 },
		"the chosen cloud brain never received the message")
	if got := len(local.Handled()); got != 0 {
		t.Fatalf("route-default brain handled %d messages, want 0 (the id chose cloud)", got)
	}

	// An unprefixed id keeps today's route behavior byte-for-byte.
	if err := r.DispatchInbound(context.Background(), mkInbound("console", "chat-2", "hola")); err != nil {
		t.Fatalf("DispatchInbound (legacy): %v", err)
	}
	eventually(t, time.Second, func() bool { return len(local.Handled()) == 1 },
		"the route default never received the legacy-id message")

	// The received event names the EFFECTIVE brain.
	eventually(t, time.Second, func() bool { return len(events.received()) == 2 },
		"expected two MessageReceived events")
	if got := events.received()[0].Brain; got != "cloud" {
		t.Fatalf("received event brain = %q, want the chosen %q", got, "cloud")
	}
}

func TestDirectBrain_percentEncodedNameDecodes(t *testing.T) {
	r, _, _, cloud := directBrainRouter(t, router.WithDirectBrainChannel("console"))
	defer shutdown(t, r)
	// A brain named with a colon must round-trip through the encoding.
	weird := newFakeBrain()
	if err := r.RegisterBrain("mi:brain", weird); err != nil {
		t.Fatalf("RegisterBrain: %v", err)
	}
	if err := r.DispatchInbound(context.Background(), mkInbound("console", "b:mi%3Abrain:chat-1", "hola")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	eventually(t, time.Second, func() bool { return len(weird.Handled()) == 1 },
		"the percent-encoded brain name did not decode to its brain")
	if got := len(cloud.Handled()); got != 0 {
		t.Fatalf("cloud handled %d, want 0", got)
	}
}

func TestDirectBrain_vanishedBrainFallsBackWithOneNotice(t *testing.T) {
	r, ch, local, _ := directBrainRouter(t, router.WithDirectBrainChannel("console"))
	defer shutdown(t, r)

	// Two messages in the SAME conversation naming a ghost brain.
	for i := 0; i < 2; i++ {
		if err := r.DispatchInbound(context.Background(), mkInbound("console", "b:fantasma:chat-9", "hola")); err != nil {
			t.Fatalf("DispatchInbound #%d: %v", i, err)
		}
	}
	eventually(t, time.Second, func() bool { return len(local.Handled()) == 2 },
		"the route default did not handle the fallback messages")

	// Exactly ONE AckBrainFallback with the sealed Spanish text, addressed
	// to the conversation.
	eventually(t, time.Second, func() bool { return len(fallbackAcks(ch)) == 1 },
		"expected exactly one brain-fallback notice")
	consistently(t, 150*time.Millisecond, func() bool { return len(fallbackAcks(ch)) == 1 },
		"the fallback notice was re-sent within one conversation")
	ack := fallbackAcks(ch)[0]
	text := latestPartText(ack)
	if !strings.Contains(text, `El cerebro "fantasma" ya no existe`) ||
		!strings.Contains(text, "cerebro por defecto") {
		t.Fatalf("fallback notice text = %q, want the sealed Spanish notice", text)
	}
	if got := ack.Meta[router.MetaConversationID]; got != "b:fantasma:chat-9" {
		t.Fatalf("fallback notice conversation = %q, want the originating one", got)
	}

	// A DIFFERENT conversation with the same ghost gets its own notice.
	if err := r.DispatchInbound(context.Background(), mkInbound("console", "b:fantasma:chat-10", "hola")); err != nil {
		t.Fatalf("DispatchInbound (second conversation): %v", err)
	}
	eventually(t, time.Second, func() bool { return len(fallbackAcks(ch)) == 2 },
		"the second conversation never got its own notice")
}

func TestDirectBrain_disabledChannelIgnoresPrefix(t *testing.T) {
	// The privacy invariant (AS-3): withOUT the option — and with it on a
	// DIFFERENT channel — the prefix is inert: route default, no ack.
	for _, opts := range [][]router.Option{
		nil,
		{router.WithDirectBrainChannel("console")},
	} {
		r := router.New(opts...)
		ch := newFakeChannel("webhook")
		def, priv := newFakeBrain(), newFakeBrain()
		if err := r.RegisterChannel(ch); err != nil {
			t.Fatalf("RegisterChannel: %v", err)
		}
		_ = r.RegisterBrain("default", def)
		_ = r.RegisterBrain("privado", priv)
		if err := r.Route("webhook", "default"); err != nil {
			t.Fatalf("Route: %v", err)
		}
		if err := r.DispatchInbound(context.Background(), mkInbound("webhook", "b:privado:x", "hola")); err != nil {
			t.Fatalf("DispatchInbound: %v", err)
		}
		eventually(t, time.Second, func() bool { return len(def.Handled()) == 1 },
			"the route default did not handle the message")
		if got := len(priv.Handled()); got != 0 {
			t.Fatalf("PRIVACY: the un-enabled channel dispatched to the id-named brain (%d)", got)
		}
		consistently(t, 100*time.Millisecond, func() bool { return len(fallbackAcks(ch)) == 0 },
			"an un-enabled channel produced a fallback notice")
		shutdown(t, r)
	}
}

func fallbackAcks(ch *fakeChannel) []*envelope.Envelope {
	var out []*envelope.Envelope
	for _, e := range ch.Sent() {
		if e.Meta[envelope.MetaAck] == envelope.AckBrainFallback {
			out = append(out, e)
		}
	}
	return out
}

func latestPartText(e *envelope.Envelope) string {
	text := ""
	for _, p := range e.Parts {
		if p.Type == envelope.Text && p.Content != "" {
			text = p.Content
		}
	}
	return text
}

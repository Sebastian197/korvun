// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package router_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/bus"
	"github.com/Sebastian197/korvun/internal/router"
)

// recordingPublisher is a router.EventPublisher that records every published
// event, safe under the concurrent publishing the router does.
type recordingPublisher struct {
	mu     sync.Mutex
	events []bus.Event
}

func (p *recordingPublisher) Publish(_ context.Context, ev bus.Event) {
	p.mu.Lock()
	p.events = append(p.events, ev)
	p.mu.Unlock()
}

func (p *recordingPublisher) snapshot() []bus.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]bus.Event(nil), p.events...)
}

func (p *recordingPublisher) typesSeen() map[bus.EventType]bus.Event {
	out := make(map[bus.EventType]bus.Event)
	for _, ev := range p.snapshot() {
		out[ev.Type] = ev
	}
	return out
}

// eventuallyReplyDelivered polls until the fake channel has sent n replies, so the
// async dispatch -> handle -> deliver cycle has completed.
func eventuallyReplyDelivered(t *testing.T, ch *fakeChannel, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(ch.Sent()) >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("reply not delivered: have %d, want %d", len(ch.Sent()), n)
}

// TestEventHook_publishesReceivedThenSent proves the additive router hook
// (ADR-0023 §3): MessageReceived is published when the router accepts an inbound
// into the brain's queue, and ReplySent after the reply's Channel.Send succeeds.
func TestEventHook_publishesReceivedThenSent(t *testing.T) {
	pub := &recordingPublisher{}
	r := router.New(router.WithEventPublisher(pub))
	t.Cleanup(func() { shutdown(t, r) })

	ch := newFakeChannel("telegram")
	reply := mkOutbound("telegram", "c-1", "hi back")
	b := newFakeBrain(reply)

	if err := r.RegisterChannel(ch); err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}
	if err := r.RegisterBrain("brain-x", b); err != nil {
		t.Fatalf("RegisterBrain: %v", err)
	}
	if err := r.Route("telegram", "brain-x"); err != nil {
		t.Fatalf("Route: %v", err)
	}

	inbound := mkInbound("telegram", "c-1", "hi")
	if err := r.DispatchInbound(context.Background(), inbound); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}

	eventuallyReplyDelivered(t, ch, 1)

	seen := pub.typesSeen()
	recv, ok := seen[bus.MessageReceived]
	if !ok {
		t.Fatalf("no MessageReceived published; saw %+v", pub.snapshot())
	}
	if recv.Channel != "telegram" || recv.Brain != "brain-x" || recv.Envelope != inbound {
		t.Errorf("MessageReceived = %+v, want channel=telegram brain=brain-x envelope=inbound", recv)
	}
	sent, ok := seen[bus.ReplySent]
	if !ok {
		t.Fatalf("no ReplySent published; saw %+v", pub.snapshot())
	}
	if sent.Channel != "telegram" || sent.Envelope != reply {
		t.Errorf("ReplySent = %+v, want channel=telegram envelope=reply", sent)
	}
}

// TestEventHook_saturatedQueueIsNotReceived proves a saturated inbound queue
// yields NO MessageReceived (it is a drop, not a receive — ADR-0023 §3). The
// brain blocks so its single-slot queue fills; the second dispatch times out.
func TestEventHook_saturatedQueueIsNotReceived(t *testing.T) {
	pub := &recordingPublisher{}
	block := make(chan struct{})
	r := router.New(
		router.WithEventPublisher(pub),
		router.WithQueueCapacity(1),
		router.WithEnqueueTimeout(50*time.Millisecond),
	)
	t.Cleanup(func() { close(block); shutdown(t, r) })

	ch := newFakeChannel("telegram")
	b := &fakeBrain{releaseCh: block} // blocks every Handle until released
	if err := r.RegisterChannel(ch); err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}
	if err := r.RegisterBrain("brain-x", b); err != nil {
		t.Fatalf("RegisterBrain: %v", err)
	}
	if err := r.Route("telegram", "brain-x"); err != nil {
		t.Fatalf("Route: %v", err)
	}

	// 1st: taken by the (blocked) worker -> Received. 2nd: fills the 1-slot queue
	// -> Received. 3rd: queue full -> ErrBrainSaturated, NOT Received.
	_ = r.DispatchInbound(context.Background(), mkInbound("telegram", "c-1", "1"))
	_ = r.DispatchInbound(context.Background(), mkInbound("telegram", "c-1", "2"))
	err := r.DispatchInbound(context.Background(), mkInbound("telegram", "c-1", "3"))
	if err == nil {
		t.Fatal("3rd dispatch should have saturated, got nil")
	}

	received := 0
	for _, ev := range pub.snapshot() {
		if ev.Type == bus.MessageReceived {
			received++
		}
	}
	if received != 2 {
		t.Errorf("MessageReceived count = %d, want 2 (the saturated 3rd is a drop, not a receive)", received)
	}
}

// TestEventHook_realBusSubscriberReadsEnvelope_race wires the REAL bus and a
// subscriber that reads Envelope fields on its own goroutine while the router
// dispatches concurrently. Under -race it exercises the ADR-0023 "published
// Envelope is immutable" invariant: a producer-side mutation would surface as a
// read/write race here, which the fake-publisher tests cannot catch.
func TestEventHook_realBusSubscriberReadsEnvelope_race(t *testing.T) {
	eb := bus.New()
	defer eb.Close()

	r := router.New(router.WithEventPublisher(eb))
	t.Cleanup(func() { shutdown(t, r) })

	ch := newFakeChannel("telegram")
	b := newFakeBrain(mkOutbound("telegram", "c-1", "ok"))
	_ = r.RegisterChannel(ch)
	_ = r.RegisterBrain("brain-x", b)
	_ = r.Route("telegram", "brain-x")

	var seen atomic.Int64
	unsub := eb.Subscribe(bus.MessageReceived, func(ev bus.Event) {
		if ev.Envelope != nil { // read envelope fields concurrently with the router
			_ = ev.Envelope.ID
			_ = ev.Envelope.Channel
		}
		seen.Add(1)
	})
	defer unsub()

	for i := 0; i < 20; i++ {
		_ = r.DispatchInbound(context.Background(), mkInbound("telegram", "c-1", "hi"))
	}
	eventuallyReplyDelivered(t, ch, 1)
}

// TestEventHook_nilPublisher_dispatchStillWorks proves the default (no publisher)
// path is a zero-cost no-op: dispatch and reply work unchanged, no panic.
func TestEventHook_nilPublisher_dispatchStillWorks(t *testing.T) {
	r := router.New() // no WithEventPublisher
	t.Cleanup(func() { shutdown(t, r) })

	ch := newFakeChannel("telegram")
	b := newFakeBrain(mkOutbound("telegram", "c-1", "ok"))
	_ = r.RegisterChannel(ch)
	_ = r.RegisterBrain("brain-x", b)
	_ = r.Route("telegram", "brain-x")

	if err := r.DispatchInbound(context.Background(), mkInbound("telegram", "c-1", "hi")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	eventuallyReplyDelivered(t, ch, 1)
}

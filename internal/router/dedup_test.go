// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package router_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/bus"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/router"
)

// Audit finding R-1: the at-least-once channels (Telegram re-delivery,
// Discord resume replay, webhook sender retries) could hand the same event
// through the whole pipeline twice — double reply to the user, double turn
// persisted. This file pins the router-side contract: DispatchInbound keeps
// a bounded LRU+TTL window keyed by channel + Meta[provider.event_id]; a
// duplicate is a COUNTED drop (MessageDropped with ErrDuplicateEvent + the
// dedup counter), never an error to the caller; an event WITHOUT an id is
// never deduplicated (fail-open).

// capturingPublisher records every published bus event, concurrency-safe.
type capturingPublisher struct {
	mu     sync.Mutex
	events []bus.Event
}

func (p *capturingPublisher) Publish(_ context.Context, ev bus.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, ev)
}

func (p *capturingPublisher) byType(t bus.EventType) []bus.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []bus.Event
	for _, ev := range p.events {
		if ev.Type == t {
			out = append(out, ev)
		}
	}
	return out
}

// mkInboundWithEventID builds an inbound envelope carrying a provider event id.
func mkInboundWithEventID(channel, conv, text, eventID string) *envelope.Envelope {
	env := mkInbound(channel, conv, text)
	env.Meta[envelope.MetaProviderEventID] = eventID
	return env
}

// waitHandled polls until the brain has handled want envelopes or times out.
func waitHandled(t *testing.T, fb *fakeBrain, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(fb.Handled()) >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("brain handled %d envelopes, want %d", len(fb.Handled()), want)
}

func TestDedup_SameEventTwiceIsHandledOnce(t *testing.T) {
	pub := &capturingPublisher{}
	var counted []string
	var countMu sync.Mutex
	r := router.New(
		router.WithEventPublisher(pub),
		router.WithDedupCounter(func(channel string) {
			countMu.Lock()
			counted = append(counted, channel)
			countMu.Unlock()
		}),
	)
	t.Cleanup(func() { shutdown(t, r) })

	fb := newFakeBrain(mkOutbound("ch", "c", "respuesta"))
	fc := newFakeChannel("ch")
	_ = r.RegisterChannel(fc)
	_ = r.RegisterBrain("brain", fb)
	_ = r.Route("ch", "brain")

	first := mkInboundWithEventID("ch", "c", "hola", "42")
	dup := mkInboundWithEventID("ch", "c", "hola", "42")

	if err := r.DispatchInbound(context.Background(), first); err != nil {
		t.Fatalf("first dispatch: %v", err)
	}
	// The duplicate is a counted drop, NEVER an error to the caller.
	if err := r.DispatchInbound(context.Background(), dup); err != nil {
		t.Fatalf("duplicate dispatch returned %v; want nil (counted drop)", err)
	}

	waitHandled(t, fb, 1)
	time.Sleep(50 * time.Millisecond) // grace: a second Handle would land here
	if got := len(fb.Handled()); got != 1 {
		t.Fatalf("brain handled %d envelopes, want exactly 1", got)
	}
	// ONE reply reaches the user — the visible half of the R-1 defect.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && len(fc.Sent()) == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if got := len(fc.Sent()); got != 1 {
		t.Fatalf("channel sent %d replies, want exactly 1", got)
	}

	drops := pub.byType(bus.MessageDropped)
	if len(drops) != 1 {
		t.Fatalf("MessageDropped events = %d, want 1", len(drops))
	}
	if !errors.Is(drops[0].Err, router.ErrDuplicateEvent) {
		t.Errorf("drop Err = %v, want errors.Is(_, ErrDuplicateEvent)", drops[0].Err)
	}
	if drops[0].Channel != "ch" {
		t.Errorf("drop Channel = %q, want %q", drops[0].Channel, "ch")
	}
	if drops[0].Envelope == nil {
		t.Error("drop Envelope = nil, want the duplicate envelope")
	}

	countMu.Lock()
	defer countMu.Unlock()
	if len(counted) != 1 || counted[0] != "ch" {
		t.Errorf("dedup counter calls = %v, want exactly [ch]", counted)
	}
}

func TestDedup_NoEventIDMeansNoDedup(t *testing.T) {
	r := router.New()
	t.Cleanup(func() { shutdown(t, r) })

	fb := newFakeBrain()
	_ = r.RegisterChannel(newFakeChannel("ch"))
	_ = r.RegisterBrain("brain", fb)
	_ = r.Route("ch", "brain")

	// Same conversation, same content, NO provider event id: both must reach
	// the brain (fail-open — missing metadata never discards a message).
	if err := r.DispatchInbound(context.Background(), mkInbound("ch", "c", "x")); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := r.DispatchInbound(context.Background(), mkInbound("ch", "c", "x")); err != nil {
		t.Fatalf("second: %v", err)
	}
	waitHandled(t, fb, 2)
}

func TestDedup_DistinctChannelsDoNotCollide(t *testing.T) {
	r := router.New()
	t.Cleanup(func() { shutdown(t, r) })

	fbA, fbB := newFakeBrain(), newFakeBrain()
	_ = r.RegisterChannel(newFakeChannel("a"))
	_ = r.RegisterChannel(newFakeChannel("b"))
	_ = r.RegisterBrain("brainA", fbA)
	_ = r.RegisterBrain("brainB", fbB)
	_ = r.Route("a", "brainA")
	_ = r.Route("b", "brainB")

	// The SAME event id on two channels is two different events.
	if err := r.DispatchInbound(context.Background(), mkInboundWithEventID("a", "c", "x", "7")); err != nil {
		t.Fatalf("channel a: %v", err)
	}
	if err := r.DispatchInbound(context.Background(), mkInboundWithEventID("b", "c", "x", "7")); err != nil {
		t.Fatalf("channel b: %v", err)
	}
	waitHandled(t, fbA, 1)
	waitHandled(t, fbB, 1)
}

func TestDedup_TTLExpiryReadmitsTheID(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	var clockMu sync.Mutex
	clock := func() time.Time {
		clockMu.Lock()
		defer clockMu.Unlock()
		return now
	}
	advance := func(d time.Duration) {
		clockMu.Lock()
		now = now.Add(d)
		clockMu.Unlock()
	}

	r := router.New(router.WithClock(clock))
	t.Cleanup(func() { shutdown(t, r) })

	fb := newFakeBrain()
	_ = r.RegisterChannel(newFakeChannel("ch"))
	_ = r.RegisterBrain("brain", fb)
	_ = r.Route("ch", "brain")

	if err := r.DispatchInbound(context.Background(), mkInboundWithEventID("ch", "c", "x", "9")); err != nil {
		t.Fatalf("first: %v", err)
	}
	advance(router.DedupTTL + time.Second)
	// Past the TTL the id must be readmitted (the window is a bounded
	// memory, not a permanent ledger).
	if err := r.DispatchInbound(context.Background(), mkInboundWithEventID("ch", "c", "x", "9")); err != nil {
		t.Fatalf("post-TTL: %v", err)
	}
	waitHandled(t, fb, 2)
}

func TestDedup_CapacityEvictsOldest(t *testing.T) {
	r := router.New(
		router.WithQueueCapacity(router.DedupCapacity + 16),
	)
	t.Cleanup(func() { shutdown(t, r) })

	fb := newFakeBrain()
	_ = r.RegisterChannel(newFakeChannel("ch"))
	_ = r.RegisterBrain("brain", fb)
	_ = r.Route("ch", "brain")

	// Fill the window, then one more: the OLDEST id (0) must have been
	// evicted, so re-dispatching it is NOT a duplicate.
	for i := 0; i <= router.DedupCapacity; i++ {
		env := mkInboundWithEventID("ch", "c", "x", fmt.Sprintf("id-%d", i))
		if err := r.DispatchInbound(context.Background(), env); err != nil {
			t.Fatalf("dispatch %d: %v", i, err)
		}
	}
	if err := r.DispatchInbound(context.Background(), mkInboundWithEventID("ch", "c", "x", "id-0")); err != nil {
		t.Fatalf("re-dispatch of evicted id: %v", err)
	}
	waitHandled(t, fb, router.DedupCapacity+2)
}

func TestDedup_HouseConstants(t *testing.T) {
	if router.DedupCapacity != 4096 {
		t.Errorf("DedupCapacity = %d, want 4096 (house constant)", router.DedupCapacity)
	}
	if router.DedupTTL != 10*time.Minute {
		t.Errorf("DedupTTL = %v, want 10m (house constant)", router.DedupTTL)
	}
}

func TestDedup_FailedEnqueueDoesNotPoisonTheID(t *testing.T) {
	// Audit E-1 (Codex C-1): a delivery that the router could NOT accept
	// (saturated queue) must not leave its event id recorded — otherwise a
	// legitimate re-delivery within the TTL is dropped as a duplicate and
	// the message is lost for good.
	release := make(chan struct{})
	fb := newFakeBrain()
	fb.releaseCh = release

	r := router.New(
		router.WithQueueCapacity(1),
		router.WithEnqueueTimeout(30*time.Millisecond),
	)
	t.Cleanup(func() { shutdown(t, r) })
	_ = r.RegisterChannel(newFakeChannel("ch"))
	_ = r.RegisterBrain("brain", fb)
	_ = r.Route("ch", "brain")

	// Block the worker, fill the cap-1 queue, then saturate with the id
	// under test.
	if err := r.DispatchInbound(context.Background(), mkInboundWithEventID("ch", "c", "a", "blocker-1")); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := r.DispatchInbound(context.Background(), mkInboundWithEventID("ch", "c", "b", "blocker-2")); err != nil {
		t.Fatalf("second (fills the queue): %v", err)
	}
	err := r.DispatchInbound(context.Background(), mkInboundWithEventID("ch", "c", "x", "lost-42"))
	if !errors.Is(err, router.ErrBrainSaturated) {
		t.Fatalf("third dispatch err = %v, want ErrBrainSaturated", err)
	}

	// Unblock and drain, then RE-DELIVER the saturated id: it must be
	// accepted and handled, never dropped as a duplicate of its own failure.
	close(release)
	waitHandled(t, fb, 2)
	if err := r.DispatchInbound(context.Background(), mkInboundWithEventID("ch", "c", "x", "lost-42")); err != nil {
		t.Fatalf("re-delivery after failure: %v", err)
	}
	waitHandled(t, fb, 3)
}

func TestDedup_ConcurrentSameIDHandledExactlyOnce(t *testing.T) {
	// E-13 rider: the window's concurrency claim, pinned under -race.
	fb := newFakeBrain()
	r := router.New()
	t.Cleanup(func() { shutdown(t, r) })
	_ = r.RegisterChannel(newFakeChannel("ch"))
	_ = r.RegisterBrain("brain", fb)
	_ = r.Route("ch", "brain")

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.DispatchInbound(context.Background(), mkInboundWithEventID("ch", "c", "x", "same-77"))
		}()
	}
	wg.Wait()
	waitHandled(t, fb, 1)
	time.Sleep(50 * time.Millisecond)
	if got := len(fb.Handled()); got != 1 {
		t.Fatalf("brain handled %d, want exactly 1 under 16 concurrent duplicates", got)
	}
}

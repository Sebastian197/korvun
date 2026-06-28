// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package bus is Korvun's in-process event bus: a best-effort, non-blocking
// publish/subscribe tap over the message pipeline's lifecycle (ADR-0023,
// Stage 14 Phase 1a). It lives ALONGSIDE the router's point-to-point queues and
// never replaces them; the router publishes lifecycle facts to it, and read-only
// consumers (the Stage 14 SSE live-view, ADR-0024) subscribe.
//
// # Contract
//
//   - Publish is BEST-EFFORT and NON-BLOCKING. It never blocks the publisher (the
//     router's hot path), never errors, and never applies backpressure. Each
//     subscriber owns a bounded buffer; when that buffer is full (a slow
//     subscriber), the event is DROPPED for that subscriber and counted
//     (DroppedCount), never queued unboundedly and never awaited.
//   - Delivery is AT-MOST-ONCE. There is no persistence and no replay; a consumer
//     sees events from its Subscribe time forward, and a slow consumer sees gaps.
//   - Concurrency: Publish is safe under concurrent callers (the router's N brain
//     workers and channel pumps publish at once — the same discipline model.Model
//     and conversation.Store carry). Subscribe/unsubscribe are safe against
//     concurrent Publish.
//   - Panic-safety: a subscriber Handler that panics is recovered at the bus
//     boundary; it never crashes the bus, a publisher, or another subscriber.
//   - Teardown: unsubscribe (and Close) stop a subscriber's goroutine; no leak —
//     PROVIDED the Handler eventually returns. A Handler that blocks forever parks
//     its goroutine in user code; the bus cannot reclaim it until it returns.
//     Handlers must therefore be non-blocking or bounded (the SSE consumer drains
//     its own bounded buffer, ADR-0024).
//
// The Bus interface is the seam for a future natsBus (multi-process); NATS itself
// stays out of this build (the single-binary Pi promise). The ctx on Publish is
// part of that seam: a network-backed impl may consult it, but InMemoryBus does
// not — it is non-blocking by construction (the same way policy.PriorityReducer is
// free to ignore the ctx it is handed, ADR-0015).
package bus

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/Sebastian197/korvun/internal/envelope"
)

// EventType classifies a lifecycle fact about the message pipeline.
type EventType int

const (
	// MessageReceived: the router accepted an inbound Envelope into a brain's
	// queue (ownership transferred). A saturated queue yields MessageDropped
	// instead, never MessageReceived.
	MessageReceived EventType = iota + 1
	// ReplySent: a reply Envelope was delivered to its channel (Channel.Send
	// returned nil). Enqueue is not delivery; a queued reply can still drop.
	ReplySent
	// MessageDropped: a message or reply did not complete its path (inbound or
	// outbound saturation, or a failed Send).
	MessageDropped
	// HandleFailed: Brain.Handle failed to produce a reply.
	HandleFailed
)

// String renders the event type for logs, metrics labels, and SSE frames.
func (t EventType) String() string {
	switch t {
	case MessageReceived:
		return "message_received"
	case ReplySent:
		return "reply_sent"
	case MessageDropped:
		return "message_dropped"
	case HandleFailed:
		return "handle_failed"
	default:
		return "unknown"
	}
}

// Event is a typed lifecycle fact that WRAPS A REFERENCE to the Envelope plus
// metadata. It is NOT the Envelope itself and not a copy of the domain message.
//
// INVARIANT — a published Envelope is IMMUTABLE. The Envelope is a shared
// reference read by subscriber goroutines concurrently with the rest of the
// pipeline, so it must be frozen at publish time: the PRODUCER must not mutate an
// Envelope after publishing it, and SUBSCRIBERS must treat it as strictly
// read-only. This holds today by construction (MessageReceived publishes the
// inbound after dispatch freezes it; ReplySent publishes the reply only after
// Channel.Send returns, and brains build new output envelopes rather than mutating
// inputs) — but -race cannot police a future regression, so the invariant is
// stated here, not just assumed. Consumers that serialize the Envelope MUST emit
// only non-secret fields (never message content nor any secret/secret-reference),
// ADR-0024 §1.
type Event struct {
	Type     EventType
	Envelope *envelope.Envelope
	Channel  string
	Brain    string
	Err      error // set for MessageDropped / HandleFailed; nil otherwise
}

// Handler consumes one Event. It runs on the subscriber's own goroutine; a panic
// is recovered at the bus boundary.
type Handler func(Event)

// Bus is the publish/subscribe seam. The router depends only on a narrow
// publish-side view of it (see the router's EventPublisher); read-only consumers
// subscribe. The interface is the seam for a future natsBus.
type Bus interface {
	// Publish hands ev to every current subscriber of ev.Type, best-effort and
	// non-blocking. ctx is accepted for the natsBus seam; InMemoryBus does not
	// consult it.
	Publish(ctx context.Context, ev Event)
	// Subscribe registers h for events of type t and returns an idempotent
	// unsubscribe func that stops delivery and the subscriber's goroutine.
	// unsubscribe is NOT synchronous with handler quiescence: events already
	// buffered may still be delivered and an in-flight handler runs to completion
	// AFTER unsubscribe returns. A consumer that tears down external state on
	// unsubscribe (e.g. an HTTP ResponseWriter) must tolerate one more handler
	// firing for already-buffered events.
	Subscribe(t EventType, h Handler) (unsubscribe func())
}

// DefaultSubscriberBuffer is the per-subscriber buffer depth. A subscriber that
// falls this far behind starts dropping (counted), never blocking the publisher.
const DefaultSubscriberBuffer = 64

// Option configures New.
type Option func(*InMemoryBus)

// WithSubscriberBuffer sets the per-subscriber buffer depth (default
// DefaultSubscriberBuffer). A non-positive n is ignored.
func WithSubscriberBuffer(n int) Option {
	return func(b *InMemoryBus) {
		if n > 0 {
			b.buffer = n
		}
	}
}

// InMemoryBus is the in-process Bus implementation. Construct with New; the zero
// value is not usable.
type InMemoryBus struct {
	buffer int

	mu     sync.RWMutex
	subs   map[EventType]map[*subscription]struct{}
	closed bool

	dropped atomic.Uint64
}

// subscription is one subscriber: a buffered channel drained by a dedicated
// goroutine, plus a once-guarded close so unsubscribe and Close cannot double-close.
type subscription struct {
	ch        chan Event
	closeOnce sync.Once
}

func (s *subscription) closeCh() { s.closeOnce.Do(func() { close(s.ch) }) }

// New constructs an empty InMemoryBus.
func New(opts ...Option) *InMemoryBus {
	b := &InMemoryBus{
		buffer: DefaultSubscriberBuffer,
		subs:   make(map[EventType]map[*subscription]struct{}),
	}
	for _, o := range opts {
		o(b)
	}
	return b
}

// Publish delivers ev to every current subscriber of ev.Type without blocking.
// It holds the read lock for the duration of the non-blocking sends so a
// concurrent unsubscribe (which removes the subscriber under the write lock
// BEFORE closing its channel) can never make Publish send on a closed channel:
// removal-from-map happens-before close, and Publish only sends to subscribers it
// finds in the map under the read lock.
func (b *InMemoryBus) Publish(_ context.Context, ev Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for s := range b.subs[ev.Type] {
		select {
		case s.ch <- ev:
		default:
			b.dropped.Add(1) // slow subscriber: drop, never backpressure the hot path
		}
	}
}

// Subscribe registers h for events of type t. It starts a goroutine that drains
// the subscriber's buffer and invokes h (recovering panics), and returns an
// idempotent unsubscribe that removes the subscriber and stops its goroutine. A
// Subscribe after Close is a no-op returning a no-op unsubscribe.
func (b *InMemoryBus) Subscribe(t EventType, h Handler) (unsubscribe func()) {
	s := &subscription{ch: make(chan Event, b.buffer)}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return func() {}
	}
	set := b.subs[t]
	if set == nil {
		set = make(map[*subscription]struct{})
		b.subs[t] = set
	}
	set[s] = struct{}{}
	b.mu.Unlock()

	go func() {
		for ev := range s.ch {
			invoke(h, ev)
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			if set := b.subs[t]; set != nil {
				delete(set, s)
			}
			b.mu.Unlock()
			s.closeCh() // after removal from the map: Publish can no longer find s
		})
	}
}

// Close stops every subscriber goroutine and makes further Publish a no-op. It is
// idempotent and safe to call concurrently with Publish/Subscribe.
func (b *InMemoryBus) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	drained := make([]*subscription, 0)
	for _, set := range b.subs {
		for s := range set {
			drained = append(drained, s)
		}
	}
	b.subs = make(map[EventType]map[*subscription]struct{})
	b.mu.Unlock()

	for _, s := range drained {
		s.closeCh() // outside the lock; the drain goroutine exits on the closed channel
	}
}

// DroppedCount returns the cumulative number of events dropped because a
// subscriber's buffer was full. It is the bus's saturation signal, mirroring
// telegram.DroppedCount (ADR-0020); the app exposes it as a pull metric.
func (b *InMemoryBus) DroppedCount() uint64 { return b.dropped.Load() }

// invoke calls h, recovering a panic at the bus boundary so a faulty subscriber
// never crashes the bus, a publisher, or another subscriber.
func invoke(h Handler, ev Event) {
	defer func() { _ = recover() }()
	h(ev)
}

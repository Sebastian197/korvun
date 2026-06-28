// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package bus

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// waitForGoroutines polls until the goroutine count drops to within `slack` of
// baseline, or fails. Used to prove subscriptions do not leak goroutines.
func waitForGoroutines(t *testing.T, baseline, slack int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= baseline+slack {
			return
		}
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("goroutines did not settle: have %d, want <= %d", runtime.NumGoroutine(), baseline+slack)
}

// TestPublish_deliversToSubscriber: a subscriber of a type receives a published
// event of that type.
func TestPublish_deliversToSubscriber(t *testing.T) {
	b := New()
	defer b.Close()

	got := make(chan Event, 1)
	unsub := b.Subscribe(MessageReceived, func(ev Event) { got <- ev })
	defer unsub()

	b.Publish(context.Background(), Event{Type: MessageReceived, Channel: "telegram"})

	select {
	case ev := <-got:
		if ev.Type != MessageReceived || ev.Channel != "telegram" {
			t.Errorf("got %+v, want MessageReceived/telegram", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("event not delivered")
	}
}

// TestSubscribe_typeFilter: a subscriber only receives its own EventType.
func TestSubscribe_typeFilter(t *testing.T) {
	b := New()
	defer b.Close()

	var sent atomic.Int64
	unsub := b.Subscribe(ReplySent, func(Event) { sent.Add(1) })
	defer unsub()

	b.Publish(context.Background(), Event{Type: MessageReceived}) // other type
	b.Publish(context.Background(), Event{Type: ReplySent})       // matching
	b.Publish(context.Background(), Event{Type: HandleFailed})    // other type

	time.Sleep(50 * time.Millisecond)
	if n := sent.Load(); n != 1 {
		t.Errorf("ReplySent subscriber saw %d events, want exactly 1 (type filter)", n)
	}
}

// TestUnsubscribe_stopsDeliveryAndExitsGoroutine: after unsubscribe, no further
// delivery, and the subscriber goroutine exits (no leak).
func TestUnsubscribe_stopsDeliveryAndExitsGoroutine(t *testing.T) {
	baseline := runtime.NumGoroutine()
	b := New()

	var count atomic.Int64
	unsub := b.Subscribe(MessageReceived, func(Event) { count.Add(1) })
	b.Publish(context.Background(), Event{Type: MessageReceived})
	time.Sleep(50 * time.Millisecond)

	unsub()
	b.Publish(context.Background(), Event{Type: MessageReceived}) // after unsubscribe
	time.Sleep(50 * time.Millisecond)

	if n := count.Load(); n != 1 {
		t.Errorf("delivered %d, want 1 (nothing after unsubscribe)", n)
	}
	b.Close()
	waitForGoroutines(t, baseline, 1)
}

// TestPublish_noSubscribers_isNoop: publishing with no subscribers neither blocks
// nor panics.
func TestPublish_noSubscribers_isNoop(t *testing.T) {
	b := New()
	defer b.Close()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			b.Publish(context.Background(), Event{Type: MessageReceived})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("publish with no subscribers blocked")
	}
}

// TestPublish_nonBlocking_slowSubscriberDrops is THE load-bearing test (the one
// that justifies the concurrency /review): with a deliberately-slow subscriber and
// CONCURRENT publishers, (a) Publish never blocks the hot path, (b) the slow
// subscriber drops with a counter instead of applying backpressure, (c) no
// goroutine/subscription leak.
func TestPublish_nonBlocking_slowSubscriberDrops(t *testing.T) {
	baseline := runtime.NumGoroutine()
	const buf = 4
	b := New(WithSubscriberBuffer(buf))

	release := make(chan struct{})
	var handled atomic.Int64
	_ = b.Subscribe(MessageReceived, func(Event) {
		<-release // block until released: the buffer fills, further publishes drop
		handled.Add(1)
	}) // teardown via Close below

	// Concurrent publishers (simulating brainWorkers>1 publishing at once).
	const workers, perWorker = 8, 50
	var wg sync.WaitGroup
	start := time.Now()
	publishDone := make(chan struct{})
	go func() {
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < perWorker; i++ {
					b.Publish(context.Background(), Event{Type: MessageReceived})
				}
			}()
		}
		wg.Wait()
		close(publishDone)
	}()

	// (a) Publish must never block, even though the only subscriber is stuck.
	select {
	case <-publishDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a slow subscriber (backpressure on the hot path)")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("publishing took %v, want fast (non-blocking)", elapsed)
	}

	// (b) The slow subscriber dropped events with a counter, not backpressure.
	total := uint64(workers * perWorker)
	if d := b.DroppedCount(); d == 0 {
		t.Errorf("DroppedCount = 0, want > 0 (slow subscriber must drop, not block)")
	} else if d > total {
		t.Errorf("DroppedCount = %d, want <= %d", d, total)
	}

	close(release) // let the handler drain
	// (c) No leak after Close.
	b.Close()
	waitForGoroutines(t, baseline, 1)
}

// TestHandler_panic_doesNotCrashBus: a subscriber handler that panics must not
// crash the bus or the publisher, and other subscribers keep working.
func TestHandler_panic_doesNotCrashBus(t *testing.T) {
	b := New()
	defer b.Close()

	unsubPanic := b.Subscribe(MessageReceived, func(Event) { panic("boom") })
	defer unsubPanic()

	var ok atomic.Int64
	unsubOK := b.Subscribe(MessageReceived, func(Event) { ok.Add(1) })
	defer unsubOK()

	// Publish several: the panicking handler must not take down the bus, and the
	// healthy subscriber must keep receiving.
	for i := 0; i < 5; i++ {
		b.Publish(context.Background(), Event{Type: MessageReceived})
	}
	time.Sleep(100 * time.Millisecond)
	if n := ok.Load(); n == 0 {
		t.Error("healthy subscriber received nothing; a panicking peer crashed the bus")
	}
}

// TestEventType_String covers every label plus the unknown fallback.
func TestEventType_String(t *testing.T) {
	cases := map[EventType]string{
		MessageReceived: "message_received",
		ReplySent:       "reply_sent",
		MessageDropped:  "message_dropped",
		HandleFailed:    "handle_failed",
		EventType(0):    "unknown",
		EventType(99):   "unknown",
	}
	for et, want := range cases {
		if got := et.String(); got != want {
			t.Errorf("EventType(%d).String() = %q, want %q", int(et), got, want)
		}
	}
}

// TestClose_idempotent: a second Close is a safe no-op.
func TestClose_idempotent(t *testing.T) {
	b := New()
	b.Close()
	b.Close() // must not panic or block
}

// TestSubscribe_afterClose_isNoop: subscribing on a closed bus returns a no-op
// unsubscribe, starts no goroutine, and delivers nothing.
func TestSubscribe_afterClose_isNoop(t *testing.T) {
	baseline := runtime.NumGoroutine()
	b := New()
	b.Close()

	var got atomic.Int64
	unsub := b.Subscribe(MessageReceived, func(Event) { got.Add(1) })
	b.Publish(context.Background(), Event{Type: MessageReceived})
	time.Sleep(20 * time.Millisecond)
	unsub() // must be safe

	if got.Load() != 0 {
		t.Error("a subscriber on a closed bus received an event")
	}
	waitForGoroutines(t, baseline, 1)
}

// TestWithSubscriberBuffer_ignoresNonPositive: a non-positive buffer keeps the
// default rather than producing an unbuffered (deadlock-prone) channel.
func TestWithSubscriberBuffer_ignoresNonPositive(t *testing.T) {
	b := New(WithSubscriberBuffer(0), WithSubscriberBuffer(-5))
	if b.buffer != DefaultSubscriberBuffer {
		t.Errorf("buffer = %d, want default %d (non-positive ignored)", b.buffer, DefaultSubscriberBuffer)
	}
}

// TestPublish_concurrentSubscribeUnsubscribe is the -race stress: publishers,
// subscribers, and unsubscribers all racing. Run with -race -count to flush
// out registration/publish races.
func TestPublish_concurrentSubscribeUnsubscribe(t *testing.T) {
	b := New()
	defer b.Close()

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Publishers.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					b.Publish(context.Background(), Event{Type: MessageReceived})
				}
			}
		}()
	}
	// Churning subscribers.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					unsub := b.Subscribe(MessageReceived, func(Event) {})
					unsub()
				}
			}
		}()
	}

	time.Sleep(200 * time.Millisecond)
	close(stop)
	wg.Wait()
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package router_test

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/channel"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/router"
)

// The inbound pump (ADR-0017 §2) is the inbound mirror of the outbound worker:
// RegisterChannel starts a goroutine that drains Channel.Receive() and hands
// each Envelope to DispatchInbound, so the router owns BOTH directions. These
// tests anchor its concurrency contract — the exact class as the Phase 2E.8
// race (a channel + a goroutine that drains it + a shutdown that closes it), so
// they are written to be run under -race -count=N.

// bufferedFakeChannel returns a fakeChannel whose inbound stream is buffered,
// so a test can preload Envelopes and then close the stream (simulating
// Channel.Stop's clean "no more updates" signal, ADR-0008) without a concurrent
// writer racing the close.
func bufferedFakeChannel(name string, capacity int) *fakeChannel {
	return &fakeChannel{
		name:    name,
		inbound: make(chan *envelope.Envelope, capacity),
	}
}

// TestPump_DeliversInboundToBrain is the core wiring assertion: an Envelope
// written to Channel.Receive() reaches the routed brain WITHOUT any direct
// DispatchInbound call. Before the pump exists, the brain never sees it.
func TestPump_DeliversInboundToBrain(t *testing.T) {
	r := router.New()
	ch := newFakeChannel("telegram") // unbuffered: send blocks until the pump reads
	b := newFakeBrain()
	if err := r.RegisterChannel(ch); err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}
	if err := r.RegisterBrain("b1", b); err != nil {
		t.Fatalf("RegisterBrain: %v", err)
	}
	if err := r.Route("telegram", "b1"); err != nil {
		t.Fatalf("Route: %v", err)
	}
	defer shutdown(t, r)

	ch.inbound <- mkInbound("telegram", "1000", "hola")
	eventually(t, time.Second, func() bool { return len(b.Handled()) == 1 },
		"pump did not deliver the inbound Envelope to the brain")
}

// TestPump_DrainsBufferedThenShutdown exercises the ADR-0008 sequence:
// Channel.Stop closes the inbound stream, the pump drains the buffered backlog
// to the brain and exits via ok==false, and only THEN does router.Shutdown run
// and complete cleanly. This is the fence between the pump and Shutdown.
func TestPump_DrainsBufferedThenShutdown(t *testing.T) {
	r := router.New()
	ch := bufferedFakeChannel("telegram", 8)
	b := newFakeBrain()
	if err := r.RegisterChannel(ch); err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}
	if err := r.RegisterBrain("b1", b); err != nil {
		t.Fatalf("RegisterBrain: %v", err)
	}
	if err := r.Route("telegram", "b1"); err != nil {
		t.Fatalf("Route: %v", err)
	}

	const n = 5
	for i := 0; i < n; i++ {
		ch.inbound <- mkInbound("telegram", "1000", "x")
	}
	close(ch.inbound) // Channel.Stop: no more updates

	eventually(t, time.Second, func() bool { return len(b.Handled()) == n },
		"pump did not drain the buffered backlog before exiting")

	shutdown(t, r) // ADR-0008: channel.Stop BEFORE router.Shutdown
}

// TestPump_ConcurrentShutdownAndChannelClose is the load-bearing test: the
// Phase 2E.8 scenario. The pump is blocked on Receive while, concurrently, the
// channel closes its inbound stream (Channel.Stop) AND the router shuts down
// (context cancel). Both pump exits — ok==false and ctx.Done() — must be safe:
// no panic, no send on a closed channel, Shutdown returns within its bound. The
// inner loop multiplies interleavings so a single -race run has a real chance to
// catch a misordered fence; run with -count for more.
func TestPump_ConcurrentShutdownAndChannelClose(t *testing.T) {
	const iterations = 100
	for i := 0; i < iterations; i++ {
		r := router.New()
		ch := newFakeChannel("telegram") // unbuffered: pump blocks in Receive
		b := newFakeBrain()
		if err := r.RegisterChannel(ch); err != nil {
			t.Fatalf("iter %d RegisterChannel: %v", i, err)
		}
		if err := r.RegisterBrain("b1", b); err != nil {
			t.Fatalf("iter %d RegisterBrain: %v", i, err)
		}
		if err := r.Route("telegram", "b1"); err != nil {
			t.Fatalf("iter %d Route: %v", i, err)
		}

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			close(ch.inbound) // Channel.Stop closes the inbound stream
		}()
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := r.Shutdown(ctx); err != nil {
				t.Errorf("iter %d Shutdown: %v", i, err)
			}
		}()
		wg.Wait()
	}
}

// TestPump_ConcurrentRegisterAndShutdown stresses the load-bearing invariant
// that channelWg.Add(2) stays serialized against Shutdown's flag set, so a
// RegisterChannel racing a Shutdown can never trigger a WaitGroup
// "Add called concurrently with Wait" panic. Register must return either nil
// (it won the race; both workers start and Shutdown joins them) or ErrShutdown
// (Shutdown won; nothing started) — never panic. Run under -race -count for
// interleaving coverage; the inner loop multiplies interleavings per run.
func TestPump_ConcurrentRegisterAndShutdown(t *testing.T) {
	const iterations = 200
	for i := 0; i < iterations; i++ {
		r := router.New()
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			err := r.RegisterChannel(newFakeChannel("telegram"))
			if err != nil && !errors.Is(err, router.ErrShutdown) {
				t.Errorf("iter %d RegisterChannel: %v, want nil or ErrShutdown", i, err)
			}
		}()
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := r.Shutdown(ctx); err != nil {
				t.Errorf("iter %d Shutdown: %v", i, err)
			}
		}()
		wg.Wait()
		// Shutdown's Wait joined whatever Register started; a second Shutdown
		// is idempotent and must not hang or panic.
		shutdown(t, r)
	}
}

// TestPump_NoGoroutineLeak confirms the pump goroutine does not outlive
// Shutdown: the goroutine count returns to its pre-registration baseline once
// the router is shut down (the same shape as the fan-out's no-leak test).
func TestPump_NoGoroutineLeak(t *testing.T) {
	runtime.GC()
	start := runtime.NumGoroutine()

	r := router.New()
	ch := newFakeChannel("telegram")
	b := newFakeBrain()
	if err := r.RegisterChannel(ch); err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}
	if err := r.RegisterBrain("b1", b); err != nil {
		t.Fatalf("RegisterBrain: %v", err)
	}
	if err := r.Route("telegram", "b1"); err != nil {
		t.Fatalf("Route: %v", err)
	}

	ch.inbound <- mkInbound("telegram", "1000", "hi") // exercise the pump
	eventually(t, time.Second, func() bool { return len(b.Handled()) == 1 },
		"brain did not handle before leak check")

	shutdown(t, r)

	eventually(t, time.Second, func() bool {
		runtime.GC()
		return runtime.NumGoroutine() <= start
	}, "goroutines leaked past Shutdown (pump did not exit)")
}

// TestPump_LogAndContinue_BadEnvelope confirms a single bad Envelope is
// log-and-continue, never fatal: the pump surfaces it to the error hook
// (ErrKindInboundDispatch) and stays alive to process the next, good Envelope.
func TestPump_LogAndContinue_BadEnvelope(t *testing.T) {
	var mu sync.Mutex
	var got []router.RouterError
	r := router.New(router.WithErrorHandler(func(e router.RouterError) {
		mu.Lock()
		got = append(got, e)
		mu.Unlock()
	}))
	ch := newFakeChannel("telegram")
	b := newFakeBrain()
	if err := r.RegisterChannel(ch); err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}
	if err := r.RegisterBrain("b1", b); err != nil {
		t.Fatalf("RegisterBrain: %v", err)
	}
	if err := r.Route("telegram", "b1"); err != nil {
		t.Fatalf("Route: %v", err)
	}
	defer shutdown(t, r)

	// Bad: an inbound Envelope with no conversation id → DispatchInbound rejects
	// it with ErrNoConversationID. The pump must not crash.
	bad := envelope.New("telegram", envelope.Inbound, envelope.Participant{ID: "u"})
	bad.AddText("no conv id")
	ch.inbound <- bad

	// Good: follows immediately and must still reach the brain.
	ch.inbound <- mkInbound("telegram", "1000", "ok")

	eventually(t, time.Second, func() bool { return len(b.Handled()) == 1 },
		"pump did not survive a bad Envelope to deliver the next one")

	eventually(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) == 1
	}, "error hook did not receive the bad-Envelope dispatch failure")

	mu.Lock()
	defer mu.Unlock()
	if got[0].Kind != router.ErrKindInboundDispatch {
		t.Errorf("Kind = %v, want ErrKindInboundDispatch", got[0].Kind)
	}
	if !errors.Is(got[0].Err, router.ErrNoConversationID) {
		t.Errorf("Err = %v, want wrapped ErrNoConversationID", got[0].Err)
	}
	if got[0].Channel != "telegram" {
		t.Errorf("Channel = %q, want telegram", got[0].Channel)
	}
}

// TestPump_ReceiveError_FailsRegistration confirms a channel whose Receive
// returns an error fails registration loudly (ErrChannelReceive), rather than
// starting a pump over a nil stream.
func TestPump_ReceiveError_FailsRegistration(t *testing.T) {
	r := router.New()
	t.Cleanup(func() { shutdown(t, r) })

	ch := &receiveErrChannel{name: "telegram"}
	err := r.RegisterChannel(ch)
	if !errors.Is(err, router.ErrChannelReceive) {
		t.Errorf("err = %v, want ErrChannelReceive", err)
	}
}

// receiveErrChannel is a channel.Channel whose Receive always errors, used to
// prove RegisterChannel fails loudly instead of pumping over a nil stream.
type receiveErrChannel struct{ name string }

func (c *receiveErrChannel) Name() string               { return c.name }
func (c *receiveErrChannel) Manifest() channel.Manifest { return channel.Manifest{Text: true} }
func (c *receiveErrChannel) Send(context.Context, *envelope.Envelope) error {
	return nil
}
func (c *receiveErrChannel) Receive(context.Context) (<-chan *envelope.Envelope, error) {
	return nil, errors.New("receive boom")
}

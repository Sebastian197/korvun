// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package router_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/router"
)

// ============================================================================
// Phase 3.2 — concurrency, resilience, error hook.
// All tests run under -race when invoked via `go test -race ./...`.
// ============================================================================

// ---------- Configurable brain workers ------------------------------------

func TestBrainWorkers_NConcurrentHandle(t *testing.T) {
	const n = 4
	started := make(chan struct{}, n)
	release := make(chan struct{})

	b := &fakeBrain{
		releaseCh: release,
		onHandle: func(_ context.Context, _ *envelope.Envelope) {
			started <- struct{}{}
		},
	}

	r := router.New(router.WithBrainWorkers(n))
	t.Cleanup(func() {
		close(release)
		shutdown(t, r)
	})

	_ = r.RegisterChannel(newFakeChannel("ch"))
	_ = r.RegisterBrain("brain", b)
	_ = r.Route("ch", "brain")

	for i := 0; i < n; i++ {
		if err := r.DispatchInbound(context.Background(), mkInbound("ch", "c", "x")); err != nil {
			t.Fatalf("dispatch %d: %v", i, err)
		}
	}

	// n Handle invocations must all start before any of them is released.
	for i := 0; i < n; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("only %d/%d concurrent handlers started", i, n)
		}
	}
}

func TestBrainWorkers_DefaultIsOne(t *testing.T) {
	// The 3.1 contract: with no WithBrainWorkers, a single worker per
	// brain serialises Handle calls.
	release := make(chan struct{})
	started := make(chan struct{}, 2)

	b := &fakeBrain{
		releaseCh: release,
		onHandle: func(_ context.Context, _ *envelope.Envelope) {
			started <- struct{}{}
		},
	}

	r := router.New()
	t.Cleanup(func() {
		close(release)
		shutdown(t, r)
	})

	_ = r.RegisterChannel(newFakeChannel("ch"))
	_ = r.RegisterBrain("brain", b)
	_ = r.Route("ch", "brain")

	_ = r.DispatchInbound(context.Background(), mkInbound("ch", "c", "x"))
	_ = r.DispatchInbound(context.Background(), mkInbound("ch", "c", "x"))

	// Only the first Handle should start; the second sits in the queue.
	select {
	case <-started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("first handler did not start")
	}
	select {
	case <-started:
		t.Fatal("second handler started concurrently with default workers=1")
	case <-time.After(100 * time.Millisecond):
		// expected: second handler is queued behind the first.
	}
}

// ---------- Brain handler timeout ------------------------------------------

func TestBrainHandlerTimeout_CutsSlowHandle(t *testing.T) {
	hook := make(chan router.RouterError, 4)
	r := router.New(
		router.WithBrainHandlerTimeout(40*time.Millisecond),
		router.WithErrorHandler(func(re router.RouterError) {
			select {
			case hook <- re:
			default:
			}
		}),
	)
	t.Cleanup(func() { shutdown(t, r) })

	_ = r.RegisterChannel(newFakeChannel("ch"))

	// releaseCh is never closed; the handler only escapes via ctx.Done().
	never := make(chan struct{})
	b := &fakeBrain{releaseCh: never}
	_ = r.RegisterBrain("brain", b)
	_ = r.Route("ch", "brain")

	_ = r.DispatchInbound(context.Background(), mkInbound("ch", "c", "x"))

	select {
	case re := <-hook:
		if re.Kind != router.ErrKindHandle {
			t.Errorf("Kind = %v, want ErrKindHandle", re.Kind)
		}
		if !errors.Is(re.Err, context.DeadlineExceeded) {
			t.Errorf("Err = %v, want DeadlineExceeded", re.Err)
		}
		if re.Brain != "brain" {
			t.Errorf("Brain = %q, want %q", re.Brain, "brain")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("no error hook within 500ms")
	}
}

// ---------- Error hook -----------------------------------------------------

func TestErrorHook_HandleError(t *testing.T) {
	hook := make(chan router.RouterError, 4)
	r := router.New(router.WithErrorHandler(func(re router.RouterError) {
		select {
		case hook <- re:
		default:
		}
	}))
	t.Cleanup(func() { shutdown(t, r) })

	_ = r.RegisterChannel(newFakeChannel("ch"))
	b := &fakeBrain{handleErr: errors.New("brain boom")}
	_ = r.RegisterBrain("brain", b)
	_ = r.Route("ch", "brain")

	_ = r.DispatchInbound(context.Background(), mkInbound("ch", "c", "x"))

	select {
	case re := <-hook:
		if re.Kind != router.ErrKindHandle {
			t.Errorf("Kind = %v, want ErrKindHandle", re.Kind)
		}
		if re.Brain != "brain" {
			t.Errorf("Brain = %q, want %q", re.Brain, "brain")
		}
		if re.Err == nil || re.Err.Error() != "brain boom" {
			t.Errorf("Err = %v, want 'brain boom'", re.Err)
		}
		if re.Envelope == nil {
			t.Error("Envelope is nil")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("no error hook")
	}
}

func TestErrorHook_SendError(t *testing.T) {
	hook := make(chan router.RouterError, 4)
	r := router.New(router.WithErrorHandler(func(re router.RouterError) {
		select {
		case hook <- re:
		default:
		}
	}))
	t.Cleanup(func() { shutdown(t, r) })

	ch := newFakeChannel("ch")
	ch.sendErr = errors.New("send boom")
	_ = r.RegisterChannel(ch)

	b := newFakeBrain(mkOutbound("ch", "c", "y"))
	_ = r.RegisterBrain("brain", b)
	_ = r.Route("ch", "brain")

	_ = r.DispatchInbound(context.Background(), mkInbound("ch", "c", "x"))

	select {
	case re := <-hook:
		if re.Kind != router.ErrKindSend {
			t.Errorf("Kind = %v, want ErrKindSend", re.Kind)
		}
		if re.Channel != "ch" {
			t.Errorf("Channel = %q, want %q", re.Channel, "ch")
		}
		if re.Err == nil || re.Err.Error() != "send boom" {
			t.Errorf("Err = %v, want 'send boom'", re.Err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("no error hook")
	}
}

func TestErrorHook_NotSet_ErrorsSwallowedSafely(t *testing.T) {
	// Without a hook, errors must be swallowed silently and the worker
	// must remain available for the next envelope.
	r := router.New()
	t.Cleanup(func() { shutdown(t, r) })

	ch := newFakeChannel("ch")
	_ = r.RegisterChannel(ch)

	first := true
	b := &fakeBrain{
		replies: []*envelope.Envelope{mkOutbound("ch", "c", "ok")},
		onHandle: func(_ context.Context, _ *envelope.Envelope) {
			// Fail the first time, then succeed.
			if first {
				first = false
			}
		},
	}
	// First call: simulate failure via handleErr swap mid-run.
	b.handleErr = errors.New("boom")
	_ = r.RegisterBrain("brain", b)
	_ = r.Route("ch", "brain")

	if err := r.DispatchInbound(context.Background(), mkInbound("ch", "c", "first")); err != nil {
		t.Fatal(err)
	}
	// Give the worker a moment to consume the first envelope.
	time.Sleep(20 * time.Millisecond)
	// Reset the failure for the next call.
	b.mu.Lock()
	b.handleErr = nil
	b.mu.Unlock()

	if err := r.DispatchInbound(context.Background(), mkInbound("ch", "c", "second")); err != nil {
		t.Fatal(err)
	}

	eventually(t, 500*time.Millisecond, func() bool { return len(ch.Sent()) == 1 }, "second envelope reply should be sent")
}

// ---------- Outbound queue per channel -------------------------------------

func TestOutboundQueue_BrainWorkerNotBlockedBySlowSend(t *testing.T) {
	// With the outbound queue, the brain worker should free up as soon
	// as it has enqueued the reply, even if Send is still in flight.
	r := router.New()
	t.Cleanup(func() { shutdown(t, r) })

	ch := newFakeChannel("ch")
	ch.sendDelay = 250 * time.Millisecond
	_ = r.RegisterChannel(ch)

	released := make(chan struct{})
	b := &fakeBrain{
		replies: []*envelope.Envelope{mkOutbound("ch", "c", "r")},
		onHandle: func(_ context.Context, _ *envelope.Envelope) {
			released <- struct{}{}
		},
	}
	_ = r.RegisterBrain("brain", b)
	_ = r.Route("ch", "brain")

	_ = r.DispatchInbound(context.Background(), mkInbound("ch", "c", "first"))
	<-released // first handler completed (well, started).

	// Now dispatch a second; the brain worker must accept it well before
	// the first Send completes (~250 ms).
	start := time.Now()
	_ = r.DispatchInbound(context.Background(), mkInbound("ch", "c", "second"))
	select {
	case <-released:
		if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
			t.Errorf("second handler took %v, expected <100ms (brain worker blocked on Send?)", elapsed)
		}
	case <-time.After(time.Second):
		t.Fatal("second handler never ran")
	}
}

func TestOutboundQueue_SaturatedHooksError(t *testing.T) {
	hook := make(chan router.RouterError, 8)
	r := router.New(
		router.WithOutboundQueueCapacity(1),
		router.WithOutboundEnqueueTimeout(30*time.Millisecond),
		router.WithSendTimeout(time.Second),
		router.WithErrorHandler(func(re router.RouterError) {
			select {
			case hook <- re:
			default:
			}
		}),
	)
	t.Cleanup(func() { shutdown(t, r) })

	ch := newFakeChannel("ch")
	ch.sendDelay = 500 * time.Millisecond
	_ = r.RegisterChannel(ch)

	// Five replies in one Handle: first goes to the channel worker, second
	// fills the cap=1 outbound queue, third+ must hit the outbound
	// enqueue timeout and surface ErrKindOutboundSaturated.
	replies := make([]*envelope.Envelope, 5)
	for i := range replies {
		replies[i] = mkOutbound("ch", "c", "r")
	}
	_ = r.RegisterBrain("brain", newFakeBrain(replies...))
	_ = r.Route("ch", "brain")

	_ = r.DispatchInbound(context.Background(), mkInbound("ch", "c", "x"))

	select {
	case re := <-hook:
		if re.Kind != router.ErrKindOutboundSaturated {
			t.Errorf("Kind = %v, want ErrKindOutboundSaturated", re.Kind)
		}
		if !errors.Is(re.Err, router.ErrChannelSaturated) {
			t.Errorf("Err = %v, want ErrChannelSaturated", re.Err)
		}
		if re.Channel != "ch" {
			t.Errorf("Channel = %q, want %q", re.Channel, "ch")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("no saturation hook within 500ms")
	}
}

// ---------- Isolation between brains ---------------------------------------

func TestBrainIsolation_SlowBrainDoesNotBlockFastBrain(t *testing.T) {
	r := router.New()
	t.Cleanup(func() { shutdown(t, r) })

	slowCh := newFakeChannel("slow-ch")
	fastCh := newFakeChannel("fast-ch")
	_ = r.RegisterChannel(slowCh)
	_ = r.RegisterChannel(fastCh)

	block := make(chan struct{})
	bSlow := &fakeBrain{releaseCh: block}
	bFast := newFakeBrain()
	_ = r.RegisterBrain("slow", bSlow)
	_ = r.RegisterBrain("fast", bFast)
	_ = r.Route("slow-ch", "slow")
	_ = r.Route("fast-ch", "fast")

	t.Cleanup(func() { close(block) })

	// Block the slow brain on its very first envelope.
	if err := r.DispatchInbound(context.Background(), mkInbound("slow-ch", "c", "x")); err != nil {
		t.Fatalf("dispatch slow: %v", err)
	}

	// While the slow brain is stuck, the fast brain must keep processing.
	const fastCount = 5
	for i := 0; i < fastCount; i++ {
		if err := r.DispatchInbound(context.Background(), mkInbound("fast-ch", "c", "x")); err != nil {
			t.Fatalf("dispatch fast %d: %v", i, err)
		}
	}

	eventually(t, 500*time.Millisecond, func() bool {
		return len(bFast.Handled()) == fastCount
	}, "fast brain should process its queue while slow brain is stuck")

	if got := len(bSlow.Handled()); got != 0 {
		t.Errorf("slow brain should have handled 0 envelopes (blocked), got %d", got)
	}
}

// ---------- Isolation between channel outbound queues ----------------------

func TestChannelOutboundIsolation_SlowChannelDoesNotBlockFastChannel(t *testing.T) {
	r := router.New(router.WithSendTimeout(time.Second))
	t.Cleanup(func() { shutdown(t, r) })

	slowCh := newFakeChannel("slow")
	slowCh.sendDelay = 400 * time.Millisecond
	fastCh := newFakeChannel("fast")
	_ = r.RegisterChannel(slowCh)
	_ = r.RegisterChannel(fastCh)

	bSlow := newFakeBrain(mkOutbound("slow", "c", "x"))
	bFast := newFakeBrain(mkOutbound("fast", "c", "x"))
	_ = r.RegisterBrain("brain-slow", bSlow)
	_ = r.RegisterBrain("brain-fast", bFast)
	_ = r.Route("slow", "brain-slow")
	_ = r.Route("fast", "brain-fast")

	if err := r.DispatchInbound(context.Background(), mkInbound("slow", "c", "x")); err != nil {
		t.Fatalf("dispatch slow: %v", err)
	}
	if err := r.DispatchInbound(context.Background(), mkInbound("fast", "c", "x")); err != nil {
		t.Fatalf("dispatch fast: %v", err)
	}

	eventually(t, 150*time.Millisecond, func() bool { return len(fastCh.Sent()) == 1 }, "fast channel should send 1 reply within 150ms while slow channel is busy")
	if got := len(slowCh.Sent()); got != 0 {
		t.Errorf("slow channel should not have sent yet (sendDelay 400ms), got %d", got)
	}
}

// ---------- Concurrent dispatch under -race --------------------------------

func TestConcurrentDispatch_NoRaces(t *testing.T) {
	r := router.New(
		router.WithBrainWorkers(4),
		router.WithQueueCapacity(1024),
	)
	t.Cleanup(func() { shutdown(t, r) })

	ch := newFakeChannel("ch")
	_ = r.RegisterChannel(ch)

	var handled atomic.Int64
	b := &fakeBrain{
		onHandle: func(_ context.Context, _ *envelope.Envelope) {
			handled.Add(1)
		},
	}
	_ = r.RegisterBrain("brain", b)
	_ = r.Route("ch", "brain")

	const total = 200
	const goroutines = 20
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < total/goroutines; j++ {
				if err := r.DispatchInbound(context.Background(), mkInbound("ch", "c", "x")); err != nil {
					t.Errorf("dispatch: %v", err)
				}
			}
		}()
	}
	wg.Wait()

	eventually(t, 2*time.Second, func() bool { return handled.Load() == total }, "all 200 envelopes should be processed")
}

// ---------- Shutdown under load --------------------------------------------

func TestShutdown_WhileInFlight_CtxBounded(t *testing.T) {
	r := router.New()

	_ = r.RegisterChannel(newFakeChannel("ch"))
	// Brain handler blocks forever unless ctx cancels it.
	never := make(chan struct{})
	b := &fakeBrain{releaseCh: never}
	_ = r.RegisterBrain("brain", b)
	_ = r.Route("ch", "brain")

	_ = r.DispatchInbound(context.Background(), mkInbound("ch", "c", "x"))
	// Let the handler enter its select{}.
	time.Sleep(20 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown returned err = %v, want nil (handler should release on ctx cancel)", err)
	}
}

func TestShutdown_SuppressesShutdownTimeErrors(t *testing.T) {
	// Errors caused by Shutdown cancelling the router context must not
	// reach the hook — they are an artefact of shutting down, not a
	// real failure.
	hookN := atomic.Int64{}
	r := router.New(router.WithErrorHandler(func(_ router.RouterError) {
		hookN.Add(1)
	}))

	_ = r.RegisterChannel(newFakeChannel("ch"))
	never := make(chan struct{})
	b := &fakeBrain{releaseCh: never}
	_ = r.RegisterBrain("brain", b)
	_ = r.Route("ch", "brain")

	_ = r.DispatchInbound(context.Background(), mkInbound("ch", "c", "x"))
	time.Sleep(20 * time.Millisecond)

	shutdown(t, r)
	// Give any straggler error any chance to fire.
	time.Sleep(50 * time.Millisecond)

	if got := hookN.Load(); got != 0 {
		t.Errorf("error hook fired %d times during shutdown, want 0", got)
	}
}

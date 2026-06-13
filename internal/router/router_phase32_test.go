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

// failOnceBrain returns an error on the first Handle call and the
// configured replies on every subsequent one. Race-safe across
// goroutines via atomic counter — used by TestErrorHook_NotSet_*.
type failOnceBrain struct {
	calls   atomic.Int64
	replies []*envelope.Envelope
}

func (b *failOnceBrain) Handle(_ context.Context, _ *envelope.Envelope) ([]*envelope.Envelope, error) {
	if b.calls.Add(1) == 1 {
		return nil, errors.New("boom")
	}
	return b.replies, nil
}

func TestErrorHook_NotSet_ErrorsSwallowedSafely(t *testing.T) {
	// Without a hook, errors must be swallowed silently and the worker
	// must remain available for the next envelope.
	r := router.New()
	t.Cleanup(func() { shutdown(t, r) })

	ch := newFakeChannel("ch")
	_ = r.RegisterChannel(ch)

	b := &failOnceBrain{replies: []*envelope.Envelope{mkOutbound("ch", "c", "ok")}}
	_ = r.RegisterBrain("brain", b)
	_ = r.Route("ch", "brain")

	if err := r.DispatchInbound(context.Background(), mkInbound("ch", "c", "first")); err != nil {
		t.Fatal(err)
	}
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

// ---------- Branch coverage ------------------------------------------------
// Short tests covering the "disabled" branch of each timeout knob, the
// option clamps, and the RouterError formatting helpers.

func TestOptions_ClampNonPositiveValuesToOne(t *testing.T) {
	// Each clamping option must accept 0 or negative and behave as if 1
	// were passed. The only observable effect is that a brain with the
	// clamped value still works: e.g. WithBrainWorkers(0) starts at
	// least one worker, so dispatch eventually reaches the brain.
	r := router.New(
		router.WithQueueCapacity(0),
		router.WithBrainWorkers(0),
		router.WithOutboundQueueCapacity(0),
	)
	t.Cleanup(func() { shutdown(t, r) })

	ch := newFakeChannel("ch")
	_ = r.RegisterChannel(ch)
	b := newFakeBrain(mkOutbound("ch", "c", "x"))
	_ = r.RegisterBrain("brain", b)
	_ = r.Route("ch", "brain")

	if err := r.DispatchInbound(context.Background(), mkInbound("ch", "c", "x")); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	eventually(t, 500*time.Millisecond, func() bool { return len(ch.Sent()) == 1 }, "reply should land even with clamped knobs")
}

func TestDispatchInbound_ZeroEnqueueTimeout_BlocksUntilCtx(t *testing.T) {
	// WithEnqueueTimeout(0) disables the enqueue timeout: the call
	// blocks until either the envelope is enqueued or ctx is cancelled.
	r := router.New(
		router.WithQueueCapacity(1),
		router.WithEnqueueTimeout(0),
	)

	_ = r.RegisterChannel(newFakeChannel("ch"))
	block := make(chan struct{})
	b := &fakeBrain{releaseCh: block}
	_ = r.RegisterBrain("brain", b)
	_ = r.Route("ch", "brain")

	// Fill worker + queue (1 in worker, 1 in queue) so the next push
	// blocks indefinitely.
	if err := r.DispatchInbound(context.Background(), mkInbound("ch", "c", "x")); err != nil {
		t.Fatal(err)
	}
	if err := r.DispatchInbound(context.Background(), mkInbound("ch", "c", "x")); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- r.DispatchInbound(ctx, mkInbound("ch", "c", "x"))
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("dispatch did not return after ctx cancel")
	}

	close(block)
	shutdown(t, r)
}

func TestBrainHandlerTimeout_DisabledWhenZero(t *testing.T) {
	// WithBrainHandlerTimeout(0) disables the per-call timeout: the
	// handler is called with the router context directly.
	r := router.New(router.WithBrainHandlerTimeout(0))
	t.Cleanup(func() { shutdown(t, r) })

	_ = r.RegisterChannel(newFakeChannel("ch"))
	b := newFakeBrain()
	_ = r.RegisterBrain("brain", b)
	_ = r.Route("ch", "brain")

	if err := r.DispatchInbound(context.Background(), mkInbound("ch", "c", "x")); err != nil {
		t.Fatal(err)
	}
	eventually(t, 200*time.Millisecond, func() bool { return len(b.Handled()) == 1 }, "handler should run with disabled timeout")
}

func TestSendTimeout_DisabledWhenZero(t *testing.T) {
	// WithSendTimeout(0) disables the send deadline: Channel.Send is
	// invoked with the router context directly.
	r := router.New(router.WithSendTimeout(0))
	t.Cleanup(func() { shutdown(t, r) })

	ch := newFakeChannel("ch")
	_ = r.RegisterChannel(ch)
	b := newFakeBrain(mkOutbound("ch", "c", "x"))
	_ = r.RegisterBrain("brain", b)
	_ = r.Route("ch", "brain")

	if err := r.DispatchInbound(context.Background(), mkInbound("ch", "c", "x")); err != nil {
		t.Fatal(err)
	}
	eventually(t, 200*time.Millisecond, func() bool { return len(ch.Sent()) == 1 }, "reply should land with disabled send timeout")
}

func TestOutboundEnqueueTimeout_DisabledWhenZero(t *testing.T) {
	// WithOutboundEnqueueTimeout(0) disables the timeout: a saturated
	// outbound queue makes the brain worker wait until either a slot
	// frees up or the router context is cancelled. Test the happy path
	// (a slot eventually frees).
	r := router.New(
		router.WithOutboundQueueCapacity(1),
		router.WithOutboundEnqueueTimeout(0),
	)
	t.Cleanup(func() { shutdown(t, r) })

	ch := newFakeChannel("ch")
	_ = r.RegisterChannel(ch)

	// One reply: fits comfortably in cap=1.
	b := newFakeBrain(mkOutbound("ch", "c", "x"))
	_ = r.RegisterBrain("brain", b)
	_ = r.Route("ch", "brain")

	_ = r.DispatchInbound(context.Background(), mkInbound("ch", "c", "x"))
	eventually(t, 200*time.Millisecond, func() bool { return len(ch.Sent()) == 1 }, "reply should land with disabled outbound enqueue timeout")
}

func TestRouterError_FormattingAndUnwrap(t *testing.T) {
	tests := []struct {
		name string
		re   router.RouterError
		want string
	}{
		{
			"handle with brain",
			router.RouterError{Kind: router.ErrKindHandle, Brain: "b1", Err: errors.New("boom")},
			"router/handle: boom (brain=b1)",
		},
		{
			"send with channel",
			router.RouterError{Kind: router.ErrKindSend, Channel: "c1", Err: errors.New("nope")},
			"router/send: nope (channel=c1)",
		},
		{
			"outbound saturated wraps sentinel",
			router.RouterError{Kind: router.ErrKindOutboundSaturated, Channel: "c1", Err: router.ErrChannelSaturated},
			"router/outbound_saturated: " + router.ErrChannelSaturated.Error() + " (channel=c1)",
		},
		{
			"both subjects",
			router.RouterError{Kind: router.ErrKindHandle, Brain: "b", Channel: "c", Err: errors.New("e")},
			"router/handle: e (brain=b channel=c)",
		},
		{
			"no underlying err",
			router.RouterError{Kind: router.ErrKindHandle, Brain: "b"},
			"router/handle (brain=b)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.re.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("unwrap returns inner", func(t *testing.T) {
		re := router.RouterError{Err: router.ErrChannelSaturated}
		if !errors.Is(re, router.ErrChannelSaturated) {
			t.Error("errors.Is(re, ErrChannelSaturated) = false, want true (via Unwrap)")
		}
	})

	t.Run("kind string", func(t *testing.T) {
		cases := map[router.ErrorKind]string{
			router.ErrKindHandle:            "handle",
			router.ErrKindSend:              "send",
			router.ErrKindOutboundSaturated: "outbound_saturated",
			router.ErrorKind(99):            "unknown(99)",
		}
		for k, want := range cases {
			if got := k.String(); got != want {
				t.Errorf("ErrorKind(%d).String() = %q, want %q", int(k), got, want)
			}
		}
	})
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

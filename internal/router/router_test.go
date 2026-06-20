// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package router_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/channel"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/router"
)

// ---------- test doubles ---------------------------------------------------

// fakeChannel implements channel.Channel for in-process testing without
// any real transport.
type fakeChannel struct {
	name string

	sentMu sync.Mutex
	sent   []*envelope.Envelope

	sendErr   error
	sendDelay time.Duration

	inbound chan *envelope.Envelope
}

func newFakeChannel(name string) *fakeChannel {
	return &fakeChannel{
		name:    name,
		inbound: make(chan *envelope.Envelope),
	}
}

func (f *fakeChannel) Name() string               { return f.name }
func (f *fakeChannel) Manifest() channel.Manifest { return channel.Manifest{Text: true} }
func (f *fakeChannel) Receive(_ context.Context) (<-chan *envelope.Envelope, error) {
	return f.inbound, nil
}

func (f *fakeChannel) Send(ctx context.Context, env *envelope.Envelope) error {
	if f.sendDelay > 0 {
		select {
		case <-time.After(f.sendDelay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f.sentMu.Lock()
	f.sent = append(f.sent, env)
	f.sentMu.Unlock()
	return f.sendErr
}

func (f *fakeChannel) Sent() []*envelope.Envelope {
	f.sentMu.Lock()
	defer f.sentMu.Unlock()
	return append([]*envelope.Envelope(nil), f.sent...)
}

// fakeBrain implements brain.Brain with controllable behaviour.
type fakeBrain struct {
	mu        sync.Mutex
	handled   []*envelope.Envelope
	replies   []*envelope.Envelope
	handleErr error

	// releaseCh, if non-nil, blocks each Handle call until either the
	// channel is closed/receives or ctx is cancelled.
	releaseCh chan struct{}

	// onHandle, if non-nil, runs at the very start of every Handle
	// call, before any blocking. Used to observe concurrency.
	onHandle func(ctx context.Context, env *envelope.Envelope)
}

func newFakeBrain(replies ...*envelope.Envelope) *fakeBrain {
	return &fakeBrain{replies: replies}
}

func (f *fakeBrain) Handle(ctx context.Context, env *envelope.Envelope) ([]*envelope.Envelope, error) {
	if f.onHandle != nil {
		f.onHandle(ctx, env)
	}
	if f.releaseCh != nil {
		select {
		case <-f.releaseCh:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	f.mu.Lock()
	f.handled = append(f.handled, env)
	f.mu.Unlock()
	return f.replies, f.handleErr
}

func (f *fakeBrain) Handled() []*envelope.Envelope {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*envelope.Envelope(nil), f.handled...)
}

// ---------- helpers ---------------------------------------------------------

func mkInbound(channelName, convID, text string) *envelope.Envelope {
	e := envelope.New(channelName, envelope.Inbound, envelope.Participant{ID: "u-1"})
	e.AddText(text)
	e.Meta[router.MetaConversationID] = convID
	return e
}

func mkOutbound(channelName, convID, text string) *envelope.Envelope {
	e := envelope.New(channelName, envelope.Outbound, envelope.Participant{ID: "bot"})
	e.AddText(text)
	e.Meta[router.MetaConversationID] = convID
	return e
}

// shutdown helper that always uses a bounded ctx so the test cannot hang.
func shutdown(t *testing.T, r *router.Router) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

// eventually polls cond until true or the deadline is reached.
func eventually(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("eventually timed out after %v: %s", timeout, msg)
}

// consistently asserts cond holds for the whole duration (the negative of
// eventually): used to verify that nothing happens (e.g. no reply is sent).
func consistently(t *testing.T, dur time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(dur)
	for time.Now().Before(deadline) {
		if !cond() {
			t.Fatalf("consistently failed: %s", msg)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// A Brain that returns an empty/nil reply slice — the Orchestrator's no-text
// short-circuit (ADR-0014 §5) — must not make the router send anything or
// panic. Anchors the router-side contract the Brain relies on.
func TestHandle_EmptyReplies_NothingSent(t *testing.T) {
	r := router.New()
	ch := newFakeChannel("telegram")
	b := newFakeBrain() // no replies → Handle returns (nil, nil)
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

	if err := r.DispatchInbound(context.Background(), mkInbound("telegram", "1000", "hi")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	eventually(t, time.Second, func() bool { return len(b.Handled()) == 1 },
		"brain did not handle the inbound")
	consistently(t, 100*time.Millisecond, func() bool { return len(ch.Sent()) == 0 },
		"router sent something for an empty reply slice")
}

// ---------- DispatchInbound validation -------------------------------------

func TestDispatchInbound_validation(t *testing.T) {
	tests := []struct {
		name string
		env  func() *envelope.Envelope
		want error
	}{
		{
			name: "nil envelope",
			env:  func() *envelope.Envelope { return nil },
			want: router.ErrNilEnvelope,
		},
		{
			name: "outbound direction",
			env: func() *envelope.Envelope {
				e := envelope.New("ch", envelope.Outbound, envelope.Participant{ID: "bot"})
				e.AddText("x")
				e.Meta[router.MetaConversationID] = "c"
				return e
			},
			want: router.ErrNotInbound,
		},
		{
			name: "missing conversation id",
			env: func() *envelope.Envelope {
				e := envelope.New("ch", envelope.Inbound, envelope.Participant{ID: "u"})
				e.AddText("x")
				return e
			},
			want: router.ErrNoConversationID,
		},
		{
			name: "empty conversation id",
			env: func() *envelope.Envelope {
				e := envelope.New("ch", envelope.Inbound, envelope.Participant{ID: "u"})
				e.AddText("x")
				e.Meta[router.MetaConversationID] = ""
				return e
			},
			want: router.ErrNoConversationID,
		},
		{
			name: "unknown channel",
			env:  func() *envelope.Envelope { return mkInbound("nope", "c", "x") },
			want: router.ErrUnknownChannel,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := router.New()
			t.Cleanup(func() { shutdown(t, r) })
			err := r.DispatchInbound(context.Background(), tt.env())
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestDispatchInbound_NoRoute(t *testing.T) {
	r := router.New()
	t.Cleanup(func() { shutdown(t, r) })

	ch := newFakeChannel("telegram")
	if err := r.RegisterChannel(ch); err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}

	err := r.DispatchInbound(context.Background(), mkInbound("telegram", "c", "x"))
	if !errors.Is(err, router.ErrNoRoute) {
		t.Errorf("err = %v, want ErrNoRoute", err)
	}
}

// ---------- Registration validation ---------------------------------------

func TestRegister_validation(t *testing.T) {
	t.Run("nil channel", func(t *testing.T) {
		r := router.New()
		t.Cleanup(func() { shutdown(t, r) })
		if err := r.RegisterChannel(nil); !errors.Is(err, router.ErrNilChannel) {
			t.Errorf("err = %v, want ErrNilChannel", err)
		}
	})
	t.Run("empty channel name", func(t *testing.T) {
		r := router.New()
		t.Cleanup(func() { shutdown(t, r) })
		if err := r.RegisterChannel(newFakeChannel("")); !errors.Is(err, router.ErrEmptyChannelName) {
			t.Errorf("err = %v, want ErrEmptyChannelName", err)
		}
	})
	t.Run("nil brain", func(t *testing.T) {
		r := router.New()
		t.Cleanup(func() { shutdown(t, r) })
		if err := r.RegisterBrain("x", nil); !errors.Is(err, router.ErrNilBrain) {
			t.Errorf("err = %v, want ErrNilBrain", err)
		}
	})
	t.Run("empty brain name", func(t *testing.T) {
		r := router.New()
		t.Cleanup(func() { shutdown(t, r) })
		if err := r.RegisterBrain("", newFakeBrain()); !errors.Is(err, router.ErrEmptyBrainName) {
			t.Errorf("err = %v, want ErrEmptyBrainName", err)
		}
	})
	t.Run("route unknown channel", func(t *testing.T) {
		r := router.New()
		t.Cleanup(func() { shutdown(t, r) })
		_ = r.RegisterBrain("b", newFakeBrain())
		if err := r.Route("nope", "b"); !errors.Is(err, router.ErrUnknownChannel) {
			t.Errorf("err = %v, want ErrUnknownChannel", err)
		}
	})
	t.Run("route unknown brain", func(t *testing.T) {
		r := router.New()
		t.Cleanup(func() { shutdown(t, r) })
		_ = r.RegisterChannel(newFakeChannel("c"))
		if err := r.Route("c", "nope"); !errors.Is(err, router.ErrUnknownBrain) {
			t.Errorf("err = %v, want ErrUnknownBrain", err)
		}
	})
}

// ---------- Routing core ---------------------------------------------------

func TestRouting_DispatchInboundReachesCorrectBrain(t *testing.T) {
	r := router.New()
	t.Cleanup(func() { shutdown(t, r) })

	ch := newFakeChannel("telegram")
	if err := r.RegisterChannel(ch); err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}
	b1 := newFakeBrain()
	b2 := newFakeBrain()
	if err := r.RegisterBrain("brain-1", b1); err != nil {
		t.Fatalf("RegisterBrain b1: %v", err)
	}
	if err := r.RegisterBrain("brain-2", b2); err != nil {
		t.Fatalf("RegisterBrain b2: %v", err)
	}
	if err := r.Route("telegram", "brain-2"); err != nil {
		t.Fatalf("Route: %v", err)
	}

	if err := r.DispatchInbound(context.Background(), mkInbound("telegram", "1000", "hola")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}

	eventually(t, 500*time.Millisecond, func() bool {
		return len(b2.Handled()) == 1
	}, "brain-2 should have received 1 envelope")

	if got := len(b1.Handled()); got != 0 {
		t.Errorf("brain-1 should not have received envelopes, got %d", got)
	}
}

func TestRouting_ReplyGoesBackToOriginatingChannel(t *testing.T) {
	r := router.New()
	t.Cleanup(func() { shutdown(t, r) })

	ch := newFakeChannel("telegram")
	_ = r.RegisterChannel(ch)

	reply := mkOutbound("telegram", "1000", "respuesta")
	b := newFakeBrain(reply)
	_ = r.RegisterBrain("brain", b)
	_ = r.Route("telegram", "brain")

	if err := r.DispatchInbound(context.Background(), mkInbound("telegram", "1000", "hola")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}

	eventually(t, 500*time.Millisecond, func() bool { return len(ch.Sent()) == 1 }, "channel.Send should have been called once")
	sent := ch.Sent()
	if sent[0].Parts[0].Content != "respuesta" {
		t.Errorf("sent content = %q, want %q", sent[0].Parts[0].Content, "respuesta")
	}
}

func TestRouting_MultipleRepliesFannedOut(t *testing.T) {
	r := router.New()
	t.Cleanup(func() { shutdown(t, r) })

	ch := newFakeChannel("telegram")
	_ = r.RegisterChannel(ch)

	b := newFakeBrain(
		mkOutbound("telegram", "1000", "uno"),
		mkOutbound("telegram", "1000", "dos"),
		mkOutbound("telegram", "1000", "tres"),
	)
	_ = r.RegisterBrain("brain", b)
	_ = r.Route("telegram", "brain")

	_ = r.DispatchInbound(context.Background(), mkInbound("telegram", "1000", "x"))
	eventually(t, 500*time.Millisecond, func() bool { return len(ch.Sent()) == 3 }, "channel should have received 3 outbound envelopes")
}

func TestRouting_RouteOverwritesPrevious(t *testing.T) {
	r := router.New()
	t.Cleanup(func() { shutdown(t, r) })

	ch := newFakeChannel("telegram")
	_ = r.RegisterChannel(ch)
	a := newFakeBrain()
	b := newFakeBrain()
	_ = r.RegisterBrain("a", a)
	_ = r.RegisterBrain("b", b)
	if err := r.Route("telegram", "a"); err != nil {
		t.Fatal(err)
	}
	if err := r.Route("telegram", "b"); err != nil {
		t.Fatal(err)
	}

	_ = r.DispatchInbound(context.Background(), mkInbound("telegram", "c", "x"))
	eventually(t, 500*time.Millisecond, func() bool { return len(b.Handled()) == 1 }, "brain b should receive after overwrite")
	if got := len(a.Handled()); got != 0 {
		t.Errorf("brain a should not receive after overwrite, got %d", got)
	}
}

// ---------- Backpressure ---------------------------------------------------

func TestBackpressure_SaturatedReturnsErrBrainSaturated(t *testing.T) {
	capacity := 2
	r := router.New(
		router.WithQueueCapacity(capacity),
		router.WithEnqueueTimeout(30*time.Millisecond),
	)

	ch := newFakeChannel("telegram")
	_ = r.RegisterChannel(ch)

	block := make(chan struct{})
	b := &fakeBrain{releaseCh: block}
	_ = r.RegisterBrain("brain", b)
	_ = r.Route("telegram", "brain")

	// Block the single worker on its first dequeue, then fill the queue
	// up to capacity. The next call must hit the enqueue timeout.
	for i := 0; i < capacity+1; i++ {
		if err := r.DispatchInbound(context.Background(), mkInbound("telegram", "c", "x")); err != nil {
			t.Fatalf("DispatchInbound #%d: %v", i, err)
		}
	}
	err := r.DispatchInbound(context.Background(), mkInbound("telegram", "c", "x"))
	if !errors.Is(err, router.ErrBrainSaturated) {
		t.Errorf("err = %v, want ErrBrainSaturated", err)
	}

	close(block)
	shutdown(t, r)
}

func TestBackpressure_ContextCancelledWhileWaiting(t *testing.T) {
	r := router.New(
		router.WithQueueCapacity(1),
		router.WithEnqueueTimeout(time.Second),
	)

	ch := newFakeChannel("telegram")
	_ = r.RegisterChannel(ch)

	block := make(chan struct{})
	b := &fakeBrain{releaseCh: block}
	_ = r.RegisterBrain("brain", b)
	_ = r.Route("telegram", "brain")

	// Saturate worker + queue.
	if err := r.DispatchInbound(context.Background(), mkInbound("telegram", "c", "x")); err != nil {
		t.Fatal(err)
	}
	if err := r.DispatchInbound(context.Background(), mkInbound("telegram", "c", "x")); err != nil {
		t.Fatal(err)
	}

	// The next call should block on the enqueue timeout; cancel its ctx
	// and verify the call returns immediately with ctx.Err().
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- r.DispatchInbound(ctx, mkInbound("telegram", "c", "x"))
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("DispatchInbound did not return after ctx cancel")
	}

	close(block)
	shutdown(t, r)
}

// ---------- Outbound timeout ----------------------------------------------

func TestSendTimeout_AppliesToSlowChannel(t *testing.T) {
	r := router.New(router.WithSendTimeout(15 * time.Millisecond))
	t.Cleanup(func() { shutdown(t, r) })

	ch := newFakeChannel("telegram")
	ch.sendDelay = 500 * time.Millisecond
	_ = r.RegisterChannel(ch)

	b := newFakeBrain(mkOutbound("telegram", "1000", "resp"))
	_ = r.RegisterBrain("brain", b)
	_ = r.Route("telegram", "brain")

	_ = r.DispatchInbound(context.Background(), mkInbound("telegram", "1000", "x"))

	// Wait well past the send timeout but well below sendDelay. The Send
	// call must have been cancelled by the deadline, so the channel
	// records nothing.
	time.Sleep(150 * time.Millisecond)
	if got := len(ch.Sent()); got != 0 {
		t.Errorf("ch.Sent len = %d, want 0 (Send should have been cancelled by timeout)", got)
	}
}

// ---------- Lifecycle ------------------------------------------------------

func TestShutdown_StopsAndRejectsFurtherDispatch(t *testing.T) {
	r := router.New()
	ch := newFakeChannel("telegram")
	_ = r.RegisterChannel(ch)
	b := newFakeBrain()
	_ = r.RegisterBrain("brain", b)
	_ = r.Route("telegram", "brain")

	shutdown(t, r)

	if err := r.DispatchInbound(context.Background(), mkInbound("telegram", "c", "x")); !errors.Is(err, router.ErrShutdown) {
		t.Errorf("after shutdown err = %v, want ErrShutdown", err)
	}
}

func TestShutdown_Idempotent(t *testing.T) {
	r := router.New()
	shutdown(t, r)
	// Second shutdown must also succeed without panicking.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Shutdown(ctx); err != nil {
		t.Errorf("second Shutdown err = %v, want nil", err)
	}
}

func TestShutdown_RejectsRegistrationAfterShutdown(t *testing.T) {
	r := router.New()
	shutdown(t, r)

	if err := r.RegisterChannel(newFakeChannel("c")); !errors.Is(err, router.ErrShutdown) {
		t.Errorf("RegisterChannel err = %v, want ErrShutdown", err)
	}
	if err := r.RegisterBrain("b", newFakeBrain()); !errors.Is(err, router.ErrShutdown) {
		t.Errorf("RegisterBrain err = %v, want ErrShutdown", err)
	}
	if err := r.Route("c", "b"); !errors.Is(err, router.ErrShutdown) {
		t.Errorf("Route err = %v, want ErrShutdown", err)
	}
}

// ---------- Conversation key ----------------------------------------------

func TestConversationKey(t *testing.T) {
	tests := []struct {
		name string
		env  *envelope.Envelope
		want string
	}{
		{"nil", nil, ""},
		{
			"no meta",
			func() *envelope.Envelope {
				return envelope.New("telegram", envelope.Inbound, envelope.Participant{ID: "u"})
			}(),
			"",
		},
		{
			"empty value",
			func() *envelope.Envelope {
				e := envelope.New("telegram", envelope.Inbound, envelope.Participant{ID: "u"})
				e.Meta[router.MetaConversationID] = ""
				return e
			}(),
			"",
		},
		{
			"populated",
			mkInbound("telegram", "1000", "x"),
			"telegram::1000",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := router.ConversationKey(tt.env); got != tt.want {
				t.Errorf("ConversationKey = %q, want %q", got, tt.want)
			}
		})
	}
}

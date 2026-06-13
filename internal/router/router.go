// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Sebastian197/korvun/internal/brain"
	"github.com/Sebastian197/korvun/internal/channel"
	"github.com/Sebastian197/korvun/internal/envelope"
)

// Option configures a Router at construction time.
type Option func(*Router)

// WithQueueCapacity overrides the per-brain queue capacity. Values
// less than 1 are clamped to 1.
func WithQueueCapacity(n int) Option {
	return func(r *Router) {
		if n < 1 {
			n = 1
		}
		r.queueCapacity = n
	}
}

// WithEnqueueTimeout overrides the DispatchInbound enqueue timeout.
// Values less than or equal to zero disable the timeout (the call
// only returns on enqueue or ctx cancellation).
func WithEnqueueTimeout(d time.Duration) Option {
	return func(r *Router) { r.enqueueTimeout = d }
}

// WithSendTimeout overrides the per-call timeout applied to every
// Channel.Send invocation for outbound replies.
func WithSendTimeout(d time.Duration) Option {
	return func(r *Router) { r.sendTimeout = d }
}

// Router is Korvun's in-process message router.
type Router struct {
	mu sync.RWMutex

	channels map[string]channel.Channel
	brains   map[string]*brainWorker
	routes   map[string]string // channel name -> brain name

	queueCapacity  int
	enqueueTimeout time.Duration
	sendTimeout    time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	shutdownOnce sync.Once
	shutdown     bool
}

// brainWorker pairs a registered brain with its inbound queue.
type brainWorker struct {
	name  string
	brain brain.Brain
	queue chan *envelope.Envelope
}

// New constructs a Router with the given options. Defaults are pinned
// by ADR-0003: queue capacity 64, enqueue timeout 250 ms, send
// timeout 5 s.
func New(opts ...Option) *Router {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Router{
		channels:       make(map[string]channel.Channel),
		brains:         make(map[string]*brainWorker),
		routes:         make(map[string]string),
		queueCapacity:  DefaultQueueCapacity,
		enqueueTimeout: DefaultEnqueueTimeout,
		sendTimeout:    DefaultSendTimeout,
		ctx:            ctx,
		cancel:         cancel,
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// RegisterChannel makes a channel available to the router. The
// channel's Name() is used as its registry key and must be non-empty.
func (r *Router) RegisterChannel(ch channel.Channel) error {
	if ch == nil {
		return ErrNilChannel
	}
	name := ch.Name()
	if name == "" {
		return ErrEmptyChannelName
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.shutdown {
		return ErrShutdown
	}
	r.channels[name] = ch
	return nil
}

// RegisterBrain attaches a brain to the router under the given name
// and starts its single background worker. The worker stops cleanly
// during Shutdown.
func (r *Router) RegisterBrain(name string, b brain.Brain) error {
	if b == nil {
		return ErrNilBrain
	}
	if name == "" {
		return ErrEmptyBrainName
	}
	r.mu.Lock()
	if r.shutdown {
		r.mu.Unlock()
		return ErrShutdown
	}
	bw := &brainWorker{
		name:  name,
		brain: b,
		queue: make(chan *envelope.Envelope, r.queueCapacity),
	}
	r.brains[name] = bw
	r.wg.Add(1)
	r.mu.Unlock()

	go r.runBrainWorker(bw)
	return nil
}

// Route binds an inbound channel to a target brain. Both names must
// already be registered. A subsequent call for the same channel
// overwrites the prior route.
func (r *Router) Route(channelName, brainName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.shutdown {
		return ErrShutdown
	}
	if _, ok := r.channels[channelName]; !ok {
		return fmt.Errorf("%w: %q", ErrUnknownChannel, channelName)
	}
	if _, ok := r.brains[brainName]; !ok {
		return fmt.Errorf("%w: %q", ErrUnknownBrain, brainName)
	}
	r.routes[channelName] = brainName
	return nil
}

// DispatchInbound enqueues an inbound Envelope for the brain routed
// from its channel. It enforces conversation correlation, routing
// table integrity, and bounded-queue backpressure per ADR-0003.
//
// Concretely, DispatchInbound either pushes the envelope onto the
// target brain's queue, returns ctx.Err() if the caller cancels, or
// returns ErrBrainSaturated if the queue stays full for the configured
// enqueue timeout. The call never blocks beyond that deadline.
func (r *Router) DispatchInbound(ctx context.Context, env *envelope.Envelope) error {
	if env == nil {
		return ErrNilEnvelope
	}
	if env.Direction != envelope.Inbound {
		return ErrNotInbound
	}
	if v := env.Meta[MetaConversationID]; v == "" {
		return ErrNoConversationID
	}

	r.mu.RLock()
	if r.shutdown {
		r.mu.RUnlock()
		return ErrShutdown
	}
	if _, ok := r.channels[env.Channel]; !ok {
		r.mu.RUnlock()
		return fmt.Errorf("%w: %q", ErrUnknownChannel, env.Channel)
	}
	brainName, ok := r.routes[env.Channel]
	if !ok {
		r.mu.RUnlock()
		return fmt.Errorf("%w: channel %q", ErrNoRoute, env.Channel)
	}
	bw, ok := r.brains[brainName]
	if !ok {
		r.mu.RUnlock()
		return fmt.Errorf("%w: %q", ErrUnknownBrain, brainName)
	}
	r.mu.RUnlock()

	if r.enqueueTimeout <= 0 {
		select {
		case bw.queue <- env:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	timer := time.NewTimer(r.enqueueTimeout)
	defer timer.Stop()
	select {
	case bw.queue <- env:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ErrBrainSaturated
	}
}

// Shutdown stops every brain worker and waits for in-flight handlers
// to return. It is safe to call concurrently and multiple times: only
// the first invocation does any work; subsequent ones simply wait for
// the in-progress shutdown to finish.
//
// The provided ctx bounds how long Shutdown blocks waiting for
// workers; if ctx is cancelled first, Shutdown returns ctx.Err() while
// the workers still drain in the background.
func (r *Router) Shutdown(ctx context.Context) error {
	r.shutdownOnce.Do(func() {
		r.mu.Lock()
		r.shutdown = true
		r.cancel()
		for _, bw := range r.brains {
			close(bw.queue)
		}
		r.mu.Unlock()
	})
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ConversationKey returns the routing key for an envelope, joining the
// channel name and the conversation id with "::". It is the canonical
// way to address a conversation in the router. Returns "" if the
// envelope is nil or the conversation id is absent or empty.
func ConversationKey(env *envelope.Envelope) string {
	if env == nil {
		return ""
	}
	v := env.Meta[MetaConversationID]
	if v == "" {
		return ""
	}
	return env.Channel + "::" + v
}

// runBrainWorker is the single goroutine attached to each registered
// brain (Phase 3.1: one worker per brain). It drains the brain's queue,
// calls Handle, and dispatches every reply through the originating
// channel under the configured send timeout.
func (r *Router) runBrainWorker(bw *brainWorker) {
	defer r.wg.Done()
	for env := range bw.queue {
		r.handleAndReply(bw.brain, env)
	}
}

// handleAndReply runs Brain.Handle on env and forwards each returned
// envelope to env.Channel via Channel.Send under a context bound by
// sendTimeout. Errors are intentionally swallowed in Phase 3.1; an
// error-reporting hook arrives in Phase 3.2.
func (r *Router) handleAndReply(b brain.Brain, env *envelope.Envelope) {
	out, err := b.Handle(r.ctx, env)
	if err != nil || len(out) == 0 {
		return
	}
	r.mu.RLock()
	ch, ok := r.channels[env.Channel]
	r.mu.RUnlock()
	if !ok {
		return
	}
	for _, reply := range out {
		sendCtx, cancel := context.WithTimeout(r.ctx, r.sendTimeout)
		_ = ch.Send(sendCtx, reply)
		cancel()
	}
}

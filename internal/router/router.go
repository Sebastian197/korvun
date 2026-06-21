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

// Router wires inbound Envelopes from channels to brains and outbound
// replies back to channels. Phase 3.2 adds configurable worker pools,
// per-call brain handler timeouts, a per-channel outbound queue, and
// an asynchronous error hook (RouterError).
type Router struct {
	mu sync.RWMutex

	channels map[string]*channelWorker
	brains   map[string]*brainWorker
	routes   map[string]string // channel name -> brain name

	// Phase 3.1 knobs.
	queueCapacity  int
	enqueueTimeout time.Duration
	sendTimeout    time.Duration

	// Phase 3.2 knobs.
	brainWorkers           int
	brainHandlerTimeout    time.Duration
	outboundQueueCapacity  int
	outboundEnqueueTimeout time.Duration
	errorHandler           func(RouterError)

	ctx    context.Context
	cancel context.CancelFunc

	brainWg   sync.WaitGroup
	channelWg sync.WaitGroup

	shutdownOnce sync.Once
	shutdown     bool
}

// brainWorker pairs a registered brain with its bounded inbound queue.
type brainWorker struct {
	name  string
	brain brain.Brain
	queue chan *envelope.Envelope
}

// channelWorker pairs a registered channel with its bounded outbound
// queue. Replies the brain produces enter this queue; a dedicated
// goroutine drains it and invokes Channel.Send.
type channelWorker struct {
	name    string
	channel channel.Channel
	queue   chan *envelope.Envelope
}

// New constructs a Router with the given options. All knobs default
// to the values pinned by ADR-0003 (DefaultQueueCapacity,
// DefaultEnqueueTimeout, DefaultSendTimeout, DefaultBrainWorkers,
// DefaultBrainHandlerTimeout, DefaultOutboundQueueCapacity,
// DefaultOutboundEnqueueTimeout). No error hook is set by default;
// without one, asynchronous errors are silently dropped (compatible
// with Phase 3.1 behaviour).
func New(opts ...Option) *Router {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Router{
		channels:               make(map[string]*channelWorker),
		brains:                 make(map[string]*brainWorker),
		routes:                 make(map[string]string),
		queueCapacity:          DefaultQueueCapacity,
		enqueueTimeout:         DefaultEnqueueTimeout,
		sendTimeout:            DefaultSendTimeout,
		brainWorkers:           DefaultBrainWorkers,
		brainHandlerTimeout:    DefaultBrainHandlerTimeout,
		outboundQueueCapacity:  DefaultOutboundQueueCapacity,
		outboundEnqueueTimeout: DefaultOutboundEnqueueTimeout,
		ctx:                    ctx,
		cancel:                 cancel,
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// RegisterChannel makes a channel available to the router and starts
// BOTH of its workers: the outbound worker that drains replies to
// Channel.Send, and the inbound pump that drains Channel.Receive() into
// DispatchInbound (ADR-0017 §2). The router thus owns inbound and
// outbound symmetrically. The channel's Name() is used as its registry
// key and must be non-empty. If Channel.Receive fails, registration
// fails with ErrChannelReceive and no worker is started.
func (r *Router) RegisterChannel(ch channel.Channel) error {
	if ch == nil {
		return ErrNilChannel
	}
	name := ch.Name()
	if name == "" {
		return ErrEmptyChannelName
	}

	// Obtain the inbound stream before committing any state, so a Receive
	// failure aborts registration cleanly. r.ctx is the router's lifetime
	// context; a well-behaved channel may use it, though Stop closing the
	// stream (ADR-0008) is the primary drain signal the pump relies on.
	inbound, err := ch.Receive(r.ctx)
	if err != nil {
		return fmt.Errorf("%w: channel %q: %w", ErrChannelReceive, name, err)
	}

	r.mu.Lock()
	if r.shutdown {
		r.mu.Unlock()
		return ErrShutdown
	}
	cw := &channelWorker{
		name:    name,
		channel: ch,
		queue:   make(chan *envelope.Envelope, r.outboundQueueCapacity),
	}
	r.channels[name] = cw
	// Two goroutines under one WaitGroup: the outbound worker and the
	// inbound pump. Shutdown waits on channelWg, so both join before it
	// returns.
	//
	// INVARIANT (load-bearing, do not move): this Add MUST stay inside the
	// r.mu section that also performs the r.shutdown re-check above, and
	// Shutdown sets r.shutdown under the SAME mutex before it cancels and
	// Waits. That ordering is the only thing preventing a WaitGroup
	// "Add called concurrently with Wait" panic: any Add that runs has
	// already observed shutdown==false under the lock, so it happens-before
	// Shutdown releases the lock, hence before Shutdown's cancel + Wait.
	// Moving this Add outside the lock reopens that panic.
	r.channelWg.Add(2)
	r.mu.Unlock()

	go r.runChannelWorker(cw)
	go r.runChannelPump(cw, inbound)
	return nil
}

// RegisterBrain attaches a brain to the router under the given name
// and starts its configured number of worker goroutines (see
// WithBrainWorkers; default 1). All workers consume the same bounded
// inbound queue; concurrency between them is the router's only
// concurrency-control knob for a brain.
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
	workers := r.brainWorkers
	r.brainWg.Add(workers)
	r.mu.Unlock()

	for i := 0; i < workers; i++ {
		go r.runBrainWorker(bw)
	}
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
// table integrity, and bounded-queue backpressure per ADR-0003. The
// call never blocks beyond the configured enqueue timeout.
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

// Shutdown stops every brain worker and channel worker and waits for
// them to return. It is idempotent and safe for concurrent use. The
// supplied ctx bounds how long Shutdown blocks: if ctx is cancelled
// first, Shutdown returns ctx.Err() while workers continue draining
// in the background.
func (r *Router) Shutdown(ctx context.Context) error {
	r.shutdownOnce.Do(func() {
		r.mu.Lock()
		r.shutdown = true
		r.mu.Unlock()
		r.cancel()
	})
	done := make(chan struct{})
	go func() {
		r.brainWg.Wait()
		r.channelWg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ConversationKey returns the routing key for an envelope, joining
// the channel name and the conversation id with "::". Returns "" if
// the envelope is nil or its conversation id is absent or empty.
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

// ---------- internal workers ----------------------------------------------

// runBrainWorker drains its brain's inbound queue, invoking
// handleAndReply for each envelope. The worker exits on either queue
// close (Shutdown closed it) or router context cancellation.
func (r *Router) runBrainWorker(bw *brainWorker) {
	defer r.brainWg.Done()
	for {
		select {
		case env, ok := <-bw.queue:
			if !ok {
				return
			}
			r.handleAndReply(bw.name, bw.brain, env)
		case <-r.ctx.Done():
			return
		}
	}
}

// runChannelWorker drains its channel's outbound queue, invoking
// deliver for each reply.
func (r *Router) runChannelWorker(cw *channelWorker) {
	defer r.channelWg.Done()
	for {
		select {
		case env, ok := <-cw.queue:
			if !ok {
				return
			}
			r.deliver(cw, env)
		case <-r.ctx.Done():
			return
		}
	}
}

// runChannelPump drains the channel's inbound Envelope stream and hands
// each Envelope to DispatchInbound. It is the inbound mirror of
// runChannelWorker — RegisterChannel starts both — so the router owns
// inbound and outbound symmetrically (ADR-0017 §2).
//
// Exit is dual, matching runChannelWorker:
//
//   - The inbound channel is closed (Channel.Stop's clean "no more
//     updates" signal, ADR-0008): the receive yields ok==false and the
//     pump returns, after draining whatever was already buffered. The
//     pump only ever READS this channel, so a close can never make it
//     send on a closed channel.
//   - The router context is cancelled (Shutdown): the pump returns at
//     once.
//
// Per the ADR-0008 ordering (channel.Stop BEFORE router.Shutdown), the
// usual path is the first: Stop closes the stream, the pump drains and
// exits, then Shutdown joins it via channelWg. If Shutdown races ahead,
// the second path fires; either way the pump joins before Shutdown
// returns, with no goroutine left behind.
func (r *Router) runChannelPump(cw *channelWorker, inbound <-chan *envelope.Envelope) {
	defer r.channelWg.Done()
	for {
		select {
		case env, ok := <-inbound:
			if !ok {
				return
			}
			r.dispatchFromPump(cw.name, env)
		case <-r.ctx.Done():
			return
		}
	}
}

// dispatchFromPump enqueues one received Envelope via DispatchInbound.
// A failure is log-and-continue: a single malformed Envelope (or a
// transiently saturated brain) must never crash the long-running
// process. The failure is surfaced to the async error hook as
// ErrKindInboundDispatch; errors that are an artefact of the router's
// own Shutdown are suppressed by notifyError (its ctx-cancellation
// guard), exactly as on the outbound path.
func (r *Router) dispatchFromPump(channelName string, env *envelope.Envelope) {
	if err := r.DispatchInbound(r.ctx, env); err != nil {
		r.notifyError(RouterError{
			Kind:     ErrKindInboundDispatch,
			Channel:  channelName,
			Envelope: env,
			Err:      err,
		})
	}
}

// handleAndReply runs Brain.Handle under a bounded context (per
// WithBrainHandlerTimeout) and, on success, enqueues every reply on
// its target channel's outbound queue. Errors from Handle are routed
// to the error hook.
func (r *Router) handleAndReply(brainName string, b brain.Brain, env *envelope.Envelope) {
	var (
		out []*envelope.Envelope
		err error
	)
	if r.brainHandlerTimeout > 0 {
		ctx, cancel := context.WithTimeout(r.ctx, r.brainHandlerTimeout)
		out, err = b.Handle(ctx, env)
		cancel()
	} else {
		out, err = b.Handle(r.ctx, env)
	}
	if err != nil {
		r.notifyError(RouterError{
			Kind:     ErrKindHandle,
			Brain:    brainName,
			Envelope: env,
			Err:      err,
		})
		return
	}
	for _, reply := range out {
		r.sendReply(reply)
	}
}

// sendReply enqueues a single reply onto the originating channel's
// outbound queue. If the queue is saturated within
// outboundEnqueueTimeout, the reply is dropped and the error hook
// receives ErrKindOutboundSaturated wrapping ErrChannelSaturated.
func (r *Router) sendReply(env *envelope.Envelope) {
	r.mu.RLock()
	if r.shutdown {
		r.mu.RUnlock()
		return
	}
	cw, ok := r.channels[env.Channel]
	r.mu.RUnlock()
	if !ok {
		return
	}

	if r.outboundEnqueueTimeout <= 0 {
		select {
		case cw.queue <- env:
			return
		case <-r.ctx.Done():
			return
		}
	}
	timer := time.NewTimer(r.outboundEnqueueTimeout)
	defer timer.Stop()
	select {
	case cw.queue <- env:
		return
	case <-timer.C:
		r.notifyError(RouterError{
			Kind:     ErrKindOutboundSaturated,
			Channel:  cw.name,
			Envelope: env,
			Err:      ErrChannelSaturated,
		})
	case <-r.ctx.Done():
		return
	}
}

// deliver invokes Channel.Send under a context bounded by sendTimeout.
// Errors from Send are routed to the error hook.
func (r *Router) deliver(cw *channelWorker, env *envelope.Envelope) {
	var (
		sendCtx context.Context
		cancel  context.CancelFunc
	)
	if r.sendTimeout > 0 {
		sendCtx, cancel = context.WithTimeout(r.ctx, r.sendTimeout)
	} else {
		sendCtx, cancel = context.WithCancel(r.ctx)
	}
	err := cw.channel.Send(sendCtx, env)
	cancel()
	if err != nil {
		r.notifyError(RouterError{
			Kind:     ErrKindSend,
			Channel:  cw.name,
			Envelope: env,
			Err:      err,
		})
	}
}

// notifyError invokes the configured error hook, if any. Errors
// produced by the router's own internal context cancellation (i.e.
// during Shutdown) are suppressed: they are an artefact of shutting
// down, not a real failure to surface to the operator.
func (r *Router) notifyError(re RouterError) {
	select {
	case <-r.ctx.Done():
		return
	default:
	}
	if r.errorHandler != nil {
		r.errorHandler(re)
	}
}

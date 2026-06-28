// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package router

import "time"

// Option configures a Router at construction time. Options are
// declarative; a Router built from the same option list always
// behaves identically.
type Option func(*Router)

// WithQueueCapacity overrides the per-brain inbound queue capacity.
// Values less than 1 are clamped to 1.
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
// Channel.Send invocation.
func WithSendTimeout(d time.Duration) Option {
	return func(r *Router) { r.sendTimeout = d }
}

// WithBrainWorkers sets the number of concurrent worker goroutines
// each registered brain runs. Values less than 1 are clamped to 1.
// Default is DefaultBrainWorkers (1) which preserves the Phase 3.1
// serial-per-brain semantics.
func WithBrainWorkers(n int) Option {
	return func(r *Router) {
		if n < 1 {
			n = 1
		}
		r.brainWorkers = n
	}
}

// WithBrainHandlerTimeout overrides the per-call ctx timeout passed to
// Brain.Handle. Values less than or equal to zero disable the timeout
// (the handler context is the router's own background context).
func WithBrainHandlerTimeout(d time.Duration) Option {
	return func(r *Router) { r.brainHandlerTimeout = d }
}

// WithOutboundQueueCapacity overrides the per-channel outbound queue
// capacity. Values less than 1 are clamped to 1.
func WithOutboundQueueCapacity(n int) Option {
	return func(r *Router) {
		if n < 1 {
			n = 1
		}
		r.outboundQueueCapacity = n
	}
}

// WithOutboundEnqueueTimeout overrides the timeout a brain worker
// waits while pushing a reply onto a saturated channel outbound
// queue before reporting ErrKindOutboundSaturated. Values less than
// or equal to zero disable the timeout (push waits indefinitely or
// until the router context is cancelled).
func WithOutboundEnqueueTimeout(d time.Duration) Option {
	return func(r *Router) { r.outboundEnqueueTimeout = d }
}

// WithErrorHandler sets the asynchronous error hook. The hook is
// invoked from a worker goroutine; it must be safe for concurrent
// use and should not block (it stalls the worker that called it).
// Errors caused by Shutdown cancelling the router context are
// suppressed and never reach the hook.
func WithErrorHandler(h func(RouterError)) Option {
	return func(r *Router) { r.errorHandler = h }
}

// WithEventPublisher sets the optional lifecycle-event sink (ADR-0023). The
// router publishes MessageReceived (on a successful inbound enqueue in
// DispatchInbound) and ReplySent (after a successful Channel.Send in deliver) to
// it. Publishing is best-effort and MUST be non-blocking — the bus drops on a
// slow subscriber rather than backpressuring the hot path. nil (the default)
// disables publishing at zero cost. MessageDropped / HandleFailed are NOT
// published here; they ride the existing WithErrorHandler funnel app-side.
func WithEventPublisher(p EventPublisher) Option {
	return func(r *Router) { r.eventPublisher = p }
}

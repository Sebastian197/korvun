// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package router wires inbound Envelopes from channels to brains and
// outbound replies back to channels.
//
// The router owns a routing table (channel name → brain name), a bounded
// queue per brain, and a single worker goroutine per brain that dequeues
// envelopes, invokes Brain.Handle, and dispatches each reply through the
// originating channel's Send method.
//
// Design and tuning knobs are pinned by ADR-0003:
//
//   - Conversations are correlated via env.Meta["conversation.id"],
//     populated by the channel adapter at envelope-construction time.
//   - Backpressure uses a per-brain bounded queue plus a short enqueue
//     timeout that surfaces saturation as ErrBrainSaturated instead of
//     unbounded buffering or silent message loss.
//
// Phase 3.1 ships one worker per registered brain. Phase 3.2 will make
// the worker count and the per-call brain handler timeout configurable,
// and will introduce a per-channel outbound queue.
package router

import "time"

// Meta key for the conversation correlation field every inbound
// Envelope must carry. Channels populate it from their native
// conversation identifier (Telegram chat ID, webhook conversation
// field, ...) so the router stays unaware of channel-specific keys.
const MetaConversationID = "conversation.id"

// Tuning defaults pinned in ADR-0003. They are deliberately
// conservative; Phase 3.1's job was to make the wiring correct and
// the contract explicit; Phase 3.2 makes the rest configurable and
// adds the error hook + per-channel outbound queue.
const (
	// DefaultQueueCapacity is the buffered size of each per-brain
	// inbound queue. (Phase 3.1.)
	DefaultQueueCapacity = 64

	// DefaultEnqueueTimeout caps how long DispatchInbound waits to
	// push an envelope into a saturated brain queue before returning
	// ErrBrainSaturated. (Phase 3.1.)
	DefaultEnqueueTimeout = 250 * time.Millisecond

	// DefaultSendTimeout caps how long a single Channel.Send call may
	// take. (Phase 3.1.)
	DefaultSendTimeout = 5 * time.Second

	// DefaultBrainWorkers is the number of concurrent goroutines
	// draining the per-brain inbound queue. Default 1 preserves the
	// serial-per-brain semantics from Phase 3.1. (Configurable in
	// Phase 3.2 via WithBrainWorkers.)
	DefaultBrainWorkers = 1

	// DefaultBrainHandlerTimeout caps how long a single Brain.Handle
	// invocation may take. Past this point the handler's context is
	// cancelled and an ErrKindHandle event reaches the error hook.
	// (Phase 3.2; WithBrainHandlerTimeout overrides.)
	DefaultBrainHandlerTimeout = 5 * time.Second

	// DefaultOutboundQueueCapacity is the buffered size of each
	// per-channel outbound queue holding replies waiting for
	// Channel.Send. (Phase 3.2; WithOutboundQueueCapacity overrides.)
	DefaultOutboundQueueCapacity = 64

	// DefaultOutboundEnqueueTimeout caps how long a brain worker waits
	// to push a reply onto a saturated channel outbound queue before
	// surfacing ErrKindOutboundSaturated to the error hook (and
	// dropping that reply). Symmetric to DefaultEnqueueTimeout for
	// inbound. (Phase 3.2; WithOutboundEnqueueTimeout overrides.)
	DefaultOutboundEnqueueTimeout = 250 * time.Millisecond
)

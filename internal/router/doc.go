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
// conservative; Phase 3.1's job is to make the wiring correct and the
// contract explicit, not to tune for any particular workload.
const (
	// DefaultQueueCapacity is the buffered size of each per-brain
	// inbound queue.
	DefaultQueueCapacity = 64

	// DefaultEnqueueTimeout caps how long DispatchInbound waits to
	// push an envelope into a saturated brain queue before returning
	// ErrBrainSaturated.
	DefaultEnqueueTimeout = 250 * time.Millisecond

	// DefaultSendTimeout caps how long a single Channel.Send call may
	// take. It is applied to every reply dispatch, regardless of the
	// channel implementation, so a slow transport cannot indefinitely
	// occupy a brain worker.
	DefaultSendTimeout = 5 * time.Second
)

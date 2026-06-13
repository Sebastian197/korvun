// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package router

import "errors"

// Sentinel errors returned by the router. Callers should match these
// with errors.Is rather than string comparison.
var (
	// ErrNilEnvelope is returned when DispatchInbound is called with a
	// nil Envelope.
	ErrNilEnvelope = errors.New("router: envelope is nil")

	// ErrNotInbound is returned when DispatchInbound receives an
	// Envelope whose Direction is not Inbound.
	ErrNotInbound = errors.New("router: envelope direction is not inbound")

	// ErrNoConversationID is returned when an inbound Envelope is
	// missing the canonical Meta["conversation.id"] entry, or carries
	// it empty. This is the router-side enforcement of the
	// correlation convention defined in ADR-0003.
	ErrNoConversationID = errors.New(`router: envelope is missing Meta["conversation.id"]`)

	// ErrNilChannel is returned when RegisterChannel is called with a
	// nil channel.Channel.
	ErrNilChannel = errors.New("router: channel is nil")

	// ErrEmptyChannelName is returned when a channel reports an empty
	// Name(); a channel must be uniquely addressable.
	ErrEmptyChannelName = errors.New("router: channel name is empty")

	// ErrUnknownChannel is returned when Route or DispatchInbound
	// reference a channel name that has not been registered.
	ErrUnknownChannel = errors.New("router: unknown channel")

	// ErrNilBrain is returned when RegisterBrain is called with a nil
	// brain.Brain.
	ErrNilBrain = errors.New("router: brain is nil")

	// ErrEmptyBrainName is returned when RegisterBrain is called with
	// an empty name.
	ErrEmptyBrainName = errors.New("router: brain name is empty")

	// ErrUnknownBrain is returned when Route or DispatchInbound
	// reference a brain name that has not been registered.
	ErrUnknownBrain = errors.New("router: unknown brain")

	// ErrNoRoute is returned when DispatchInbound finds no routing
	// entry for the envelope's channel.
	ErrNoRoute = errors.New("router: no route registered for channel")

	// ErrBrainSaturated is returned when DispatchInbound cannot enqueue
	// the envelope within the configured enqueue timeout because the
	// target brain's queue is full. Per ADR-0003 this is the explicit
	// backpressure signal callers act on; the router never drops
	// silently.
	ErrBrainSaturated = errors.New("router: brain queue saturated")

	// ErrShutdown is returned by every public operation invoked after
	// Shutdown.
	ErrShutdown = errors.New("router: shut down")
)

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"errors"
	"fmt"

	"github.com/Sebastian197/korvun/internal/envelope"
)

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

	// ErrChannelSaturated is the underlying error wrapped inside a
	// RouterError of kind ErrKindOutboundSaturated. Per ADR-0003,
	// outbound saturation surfaces as an explicit event to the error
	// hook rather than as silent message loss.
	ErrChannelSaturated = errors.New("router: channel outbound queue saturated")
)

// ErrorKind classifies the failure mode an asynchronous router event
// represents. Returned to the error hook configured by
// WithErrorHandler.
type ErrorKind int

// Error kinds delivered to the WithErrorHandler hook.
const (
	// ErrKindHandle indicates a Brain.Handle invocation returned an
	// error (including context deadline exceeded if the per-call
	// brain handler timeout fired).
	ErrKindHandle ErrorKind = iota + 1

	// ErrKindSend indicates a Channel.Send invocation returned an
	// error (including context deadline exceeded if the send timeout
	// fired).
	ErrKindSend

	// ErrKindOutboundSaturated indicates a reply could not be enqueued
	// on a channel's outbound queue within the outbound enqueue
	// timeout; the reply was dropped. The wrapped error is
	// ErrChannelSaturated.
	ErrKindOutboundSaturated
)

// String returns the short human-readable name of the error kind.
func (k ErrorKind) String() string {
	switch k {
	case ErrKindHandle:
		return "handle"
	case ErrKindSend:
		return "send"
	case ErrKindOutboundSaturated:
		return "outbound_saturated"
	default:
		return fmt.Sprintf("unknown(%d)", int(k))
	}
}

// RouterError is the structured event passed to the error hook
// configured via WithErrorHandler. The router never returns these to
// the caller of DispatchInbound — they are asynchronous failures
// surfaced from a brain worker or a channel worker.
//
// Brain is populated when Kind is ErrKindHandle; Channel is populated
// when Kind is ErrKindSend or ErrKindOutboundSaturated. Envelope is
// the envelope being processed when the failure occurred. Err is the
// underlying cause.
type RouterError struct {
	Kind     ErrorKind
	Brain    string
	Channel  string
	Envelope *envelope.Envelope
	Err      error
}

// Error implements the error interface. The format is "router/<kind>:
// <err>" with optional " (brain=… channel=…)" suffix.
func (e RouterError) Error() string {
	subject := ""
	switch {
	case e.Brain != "" && e.Channel != "":
		subject = fmt.Sprintf(" (brain=%s channel=%s)", e.Brain, e.Channel)
	case e.Brain != "":
		subject = fmt.Sprintf(" (brain=%s)", e.Brain)
	case e.Channel != "":
		subject = fmt.Sprintf(" (channel=%s)", e.Channel)
	}
	if e.Err != nil {
		return fmt.Sprintf("router/%s: %s%s", e.Kind, e.Err.Error(), subject)
	}
	return fmt.Sprintf("router/%s%s", e.Kind, subject)
}

// Unwrap returns the underlying error so errors.Is / errors.As can
// match against ErrChannelSaturated and friends.
func (e RouterError) Unwrap() error { return e.Err }

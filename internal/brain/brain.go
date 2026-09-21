// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package brain declares the minimal contract a reasoning engine must
// implement to participate in Korvun's router.
//
// This package is deliberately minimal: it is a forward slice of the
// full Stage 7 design exposing only the surface the router needs to
// dispatch envelopes. Stateful behaviour, model configuration,
// telemetry hooks, and policy attachment all belong to Stage 7 and
// are intentionally absent from this interface.
package brain

import (
	"context"

	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/identity"
)

// Brain handles a single inbound Envelope and returns zero or more
// outbound Envelopes destined for the originating channel.
//
// Implementations may be stateful. The router does not protect Handle
// against concurrent invocation; in Phase 3.1 one worker per registered
// brain guarantees serial calls, but Phase 3.2 may change that.
type Brain interface {
	Handle(ctx context.Context, env *envelope.Envelope) ([]*envelope.Envelope, error)
}

// AuthenticatedBrain receives the ingress capability carried by the router's
// queued work item. A router uses this extension whenever the envelope came
// through an authenticated adapter door.
type AuthenticatedBrain interface {
	HandleAuthenticated(ctx context.Context, env *envelope.Envelope, ingress identity.AuthenticatedIngress) ([]*envelope.Envelope, error)
}

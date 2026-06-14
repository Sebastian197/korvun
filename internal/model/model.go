// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"context"
	"fmt"
)

// Role classifies a Message by its author in the LLM-style
// conversation. The zero value is intentionally invalid so an
// uninitialised Message never silently masquerades as one of the
// real roles.
type Role int

// Recognised roles. Numeric order is irrelevant to providers (they
// see the lowercase string from Role.String), so additions are
// safe at any position.
const (
	RoleSystem Role = iota + 1
	RoleUser
	RoleAssistant
)

// String returns the canonical lowercase name used by every LLM
// provider Korvun integrates with ("system" / "user" / "assistant").
// Unrecognised roles surface as "unknown(<n>)" so a misconfigured
// caller is loud rather than silent.
func (r Role) String() string {
	switch r {
	case RoleSystem:
		return "system"
	case RoleUser:
		return "user"
	case RoleAssistant:
		return "assistant"
	default:
		return fmt.Sprintf("unknown(%d)", int(r))
	}
}

// Message is one turn of the conversation handed to a Model. Role
// labels who authored it; Content is the plain text payload.
// Phase 4.1 does not model multimodal content (images, tool calls,
// vision) — those grow as additive fields or sibling Part types
// when a real consumer needs them.
type Message struct {
	Role    Role
	Content string
}

// Request is the input to Model.Generate. Model names the
// provider-side model identifier (e.g. "llama3.2" for Ollama,
// "gpt-4o" for OpenAI); Messages is the ordered conversation, with
// system prompts first by convention. The Model adapter does not
// enforce ordering — that is a Brain concern.
type Request struct {
	Model    string
	Messages []Message
}

// Response is what Model.Generate returns. Message is the assistant
// turn (Role == RoleAssistant on success); Provider and ModelName
// label the source so a fan-out result (Phase 4.3) keeps its
// attribution without a side channel.
type Response struct {
	Message   Message
	Provider  string
	ModelName string
}

// Model is the contract every reasoning provider implements.
// Generate produces an assistant Message from the given
// conversation; implementations MUST propagate ctx to the
// underlying HTTP request so cancellation works end-to-end.
//
// Streaming is intentionally not on this interface — providers
// that support it additionally satisfy StreamingModel (declared
// in a later phase) so non-streaming callers do not pay any
// streaming cognitive cost. See ADR-0009 §2.
type Model interface {
	// Generate produces the assistant reply for the given request.
	Generate(ctx context.Context, req *Request) (*Response, error)
	// Name returns the canonical provider name (e.g. "ollama"),
	// used for error wrapping and (later) by the Registry.
	Name() string
}

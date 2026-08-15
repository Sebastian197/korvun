// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"errors"
	"fmt"
	"time"
)

// Sentinel errors every Model adapter returns at the upstream
// validation seam. Callers match with errors.Is rather than
// string comparison.
var (
	// ErrNilRequest is returned by Generate when called with a
	// nil *Request. Adapter wrappers may also surface this.
	ErrNilRequest = errors.New("model: request is nil")

	// ErrEmptyModel is returned when Request.Model is the empty
	// string. Every provider needs a concrete model identifier.
	ErrEmptyModel = errors.New("model: request model name is empty")

	// ErrEmptyMessages is returned when Request.Messages is empty.
	// A Model with no conversation has nothing to answer.
	ErrEmptyMessages = errors.New("model: request has no messages")

	// ErrInvalidRole is returned when a Message carries a Role
	// value outside the recognised RoleSystem / RoleUser /
	// RoleAssistant range.
	ErrInvalidRole = errors.New("model: message has invalid role")

	// ErrEmptyContent is returned when a Message has Role set but
	// Content empty. An empty turn carries no information.
	ErrEmptyContent = errors.New("model: message content is empty")

	// ErrProviderUnavailable is returned by adapter implementations
	// when the underlying transport fails to deliver the request
	// (network error, server down, etc.). Wraps the underlying
	// cause for errors.Is / errors.As inspection.
	ErrProviderUnavailable = errors.New("model: provider unavailable")

	// ErrProviderResponse is returned by adapter implementations
	// when the underlying provider responded but the payload could
	// not be parsed or the status code was non-2xx. Wraps the
	// underlying cause.
	ErrProviderResponse = errors.New("model: provider returned a bad response")

	// ErrAuthInvalid is returned by adapter implementations when the
	// underlying provider rejected the call for missing or invalid
	// credentials (typically HTTP 401 or 403 on cloud APIs). Distinct
	// from ErrProviderUnavailable because a retry will not help — the
	// operator must fix the credentials before the call can succeed.
	// (ADR-0010 §1 / §4.)
	ErrAuthInvalid = errors.New("model: provider authentication failed")

	// ErrToolsUnsupported is returned by a ToolCallingModel adapter when the
	// underlying MODEL refuses the tools protocol itself — verified raw
	// against ollama 0.30.8 + gemma3:270m (2026-08-15, deterministic 3/3):
	// HTTP 400 {"error":"...does not support tools"}. Distinct from
	// ErrProviderResponse because the caller can DEGRADE: the same model
	// answers the plain chat lane, so the agent falls back to the
	// prompt-protocol lane instead of failing the message (RT-3).
	ErrToolsUnsupported = errors.New("model: model does not support native tool calling")

	// ErrRateLimited is returned by adapter implementations when the
	// underlying provider rejected the call for exceeding a quota
	// (typically HTTP 429 on cloud APIs). Recoverable by waiting; a
	// fan-out / retry policy upstream of the adapter (Phase 4.3) can
	// inspect *RateLimitError via errors.As to recover the optional
	// RetryAfter hint. (ADR-0010 §1 / §4.)
	ErrRateLimited = errors.New("model: provider rate-limited the caller")
)

// RateLimitError is the concrete error type returned when a provider
// signals a rate-limit hit. Provider identifies the source (handy
// when a fan-out sees several errors from different providers);
// RetryAfter is the suggested wait, zero when the provider did not
// advise one (e.g. HTTP 429 without a retry-after header).
//
// Wraps ErrRateLimited so errors.Is(err, ErrRateLimited) keeps
// working without callers having to know the concrete type;
// errors.As(err, &rle) recovers the metadata. Same pattern as
// net.OpError, os.PathError, etc.
type RateLimitError struct {
	Provider   string
	RetryAfter time.Duration
}

// Error implements the error interface. Includes the provider and,
// when present, the retry-after hint, so a log line is self-
// describing without forcing the caller to type-assert.
func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("%s: %s rate-limited, retry after %s",
			ErrRateLimited.Error(), e.Provider, e.RetryAfter)
	}
	return fmt.Sprintf("%s: %s rate-limited", ErrRateLimited.Error(), e.Provider)
}

// Unwrap returns the wrapped ErrRateLimited sentinel so
// errors.Is(err, ErrRateLimited) succeeds on a *RateLimitError.
func (e *RateLimitError) Unwrap() error { return ErrRateLimited }

// ValidateRequest checks the universal upstream invariants every
// adapter expects: non-nil request, non-empty Model, at least one
// Message, each Message with a recognised Role and non-empty
// Content. Adapters call this first thing inside Generate so
// validation errors look the same across providers.
func ValidateRequest(req *Request) error {
	if req == nil {
		return ErrNilRequest
	}
	if req.Model == "" {
		return ErrEmptyModel
	}
	if len(req.Messages) == 0 {
		return ErrEmptyMessages
	}
	for _, m := range req.Messages {
		// Per-role field invariants (estreno E-10): the tool-calling fields
		// are role-bound — only the assistant emits ToolCalls, only a tool
		// turn carries ToolName. Ambiguous shapes would serialize to the
		// provider on the native lane while old adapters ignore them (role
		// confusion, provider-dependent failures).
		switch m.Role {
		case RoleSystem, RoleUser:
			if len(m.ToolCalls) > 0 || m.ToolName != "" {
				return fmt.Errorf("%w: %s turn carries tool-calling fields", ErrInvalidRole, m.Role)
			}
		case RoleAssistant:
			if m.ToolName != "" {
				return fmt.Errorf("%w: assistant turn carries a tool result attribution", ErrInvalidRole)
			}
		case RoleTool:
			// A native-lane tool-result turn (ADR-0042 §2): valid only with
			// its tool attribution; content is the tool's output — and it
			// never emits calls of its own.
			if m.ToolName == "" || len(m.ToolCalls) > 0 {
				return ErrInvalidRole
			}
		default:
			return ErrInvalidRole
		}
		if m.Content == "" && !(m.Role == RoleAssistant && len(m.ToolCalls) > 0) {
			// An assistant turn carrying tool_calls is words-free by design
			// (ADR-0042 §3); everything else still requires content.
			return ErrEmptyContent
		}
	}
	return nil
}

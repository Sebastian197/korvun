// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package model

import "errors"

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
)

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
		switch m.Role {
		case RoleSystem, RoleUser, RoleAssistant:
		default:
			return ErrInvalidRole
		}
		if m.Content == "" {
			return ErrEmptyContent
		}
	}
	return nil
}

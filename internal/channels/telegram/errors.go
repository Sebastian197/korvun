// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import "errors"

// Sentinel errors returned by the Telegram adapter. Callers should match
// with errors.Is rather than string comparison.
var (
	// ErrNoMessage is returned when the inbound Update has no Message field
	// (or the Update itself is nil). Stage 2.3 only covers regular Messages;
	// other update kinds (edits, callback queries, ...) are out of scope.
	ErrNoMessage = errors.New("telegram: update has no message")

	// ErrUnsupportedContent is returned when the inbound Message has no
	// content the adapter can translate (e.g. empty text, missing From,
	// non-text media in this phase).
	ErrUnsupportedContent = errors.New("telegram: unsupported message content")

	// ErrNilEnvelope is returned when OutboundToSendMessage is called with
	// a nil Envelope.
	ErrNilEnvelope = errors.New("telegram: envelope is nil")

	// ErrNotOutbound is returned when an outbound conversion is attempted
	// on an Envelope whose Direction is not Outbound.
	ErrNotOutbound = errors.New("telegram: envelope direction is not outbound")

	// ErrWrongChannel is returned when an Envelope intended for a different
	// channel is passed to the Telegram adapter.
	ErrWrongChannel = errors.New("telegram: envelope channel is not telegram")

	// ErrMissingChatID is returned when an outbound Envelope does not carry
	// the telegram.chat_id meta entry, or carries it empty.
	ErrMissingChatID = errors.New("telegram: envelope is missing telegram.chat_id meta")

	// ErrInvalidChatID is returned when the telegram.chat_id meta is present
	// but does not parse as int64.
	ErrInvalidChatID = errors.New("telegram: telegram.chat_id is not a valid int64")

	// ErrNoTextPart is returned when an outbound Envelope does not contain
	// any non-empty text part. Phase 2.3 only sends text messages.
	ErrNoTextPart = errors.New("telegram: envelope has no text part")
)

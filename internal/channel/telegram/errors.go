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
	// any non-empty text part. Phase 2.3 only sends text messages;
	// OutboundParams (Phase 2E.2) prefers ErrNoPartsToSend for the
	// multi-modal path.
	ErrNoTextPart = errors.New("telegram: envelope has no text part")

	// ErrNoPartsToSend is returned by OutboundParams when an outbound
	// Envelope has neither a non-empty text part nor a media part.
	// (Phase 2E.2.)
	ErrNoPartsToSend = errors.New("telegram: envelope has no parts to send")

	// ErrTooManyMediaParts is returned by OutboundParams when an
	// outbound Envelope carries more than one media part. Telegram's
	// per-message send methods address one media item at a time;
	// media groups require a different Send method and are out of
	// scope for Phase 2E.2.
	ErrTooManyMediaParts = errors.New("telegram: envelope has more than one media part")

	// ErrMissingMediaSource is returned by OutboundParams when a
	// media part has an empty Source (no Telegram file_id or URL to
	// reference).
	ErrMissingMediaSource = errors.New("telegram: media part has empty source")

	// ErrInvalidLocation is returned by OutboundParams when a Location
	// part's Content cannot be decoded as the canonical {lat, lon} JSON
	// shape fixed by ADR-0004. (Phase 2E.3.)
	ErrInvalidLocation = errors.New("telegram: location part has invalid content")

	// ErrMissingCallbackQueryID is returned by OutboundParams when an
	// OpCallbackAck Envelope does not carry telegram.callback_query_id
	// in Meta (or carries it empty). The ack is addressed by that ID,
	// so its absence makes the envelope unsendable. (Phase 2E.4,
	// migrated to Operation routing in Phase 2E.6.)
	ErrMissingCallbackQueryID = errors.New("telegram: envelope is missing telegram.callback_query_id meta")

	// ErrMissingTargetMessageID is returned by OutboundParams when an
	// edit/delete Operation Envelope does not carry
	// telegram.message_id in Meta (absent, empty, or not parseable as
	// int). The target message of an edit/delete is addressed by that
	// ID, so its absence makes the envelope unsendable. (Phase 2E.6.)
	ErrMissingTargetMessageID = errors.New("telegram: envelope is missing or has invalid telegram.message_id meta")

	// ErrMissingToken is returned by New when no Bot API token was
	// supplied via WithToken. (Phase 2E.8.)
	ErrMissingToken = errors.New("telegram: missing bot token")

	// ErrInvalidMode is returned by New when WithMode was not used
	// (zero Mode) or when the value is not a recognised Mode. (Phase 2E.8.)
	ErrInvalidMode = errors.New("telegram: invalid or missing transport mode")

	// ErrMissingWebhookURL is returned by New when Mode is
	// ModeWebhook and no public URL was supplied via WithWebhookURL.
	// (Phase 2E.8.)
	ErrMissingWebhookURL = errors.New("telegram: webhook mode requires a public URL")

	// ErrMissingListenAddr is returned by New when Mode is
	// ModeWebhook and no listen address was supplied via
	// WithListenAddr. (Phase 2E.8.)
	ErrMissingListenAddr = errors.New("telegram: webhook mode requires a listen address")

	// ErrMissingSecretToken is returned by New when Mode is
	// ModeWebhook and no secret token was supplied via
	// WithSecretToken. The secret token is the only authentication
	// between Telegram and Korvun; running webhook mode without it
	// is not a supported deployment. (Phase 2E.8.)
	ErrMissingSecretToken = errors.New("telegram: webhook mode requires a secret token")

	// ErrMissingTLSConfig is returned by New when Mode is
	// ModeWebhook and neither WithTLS nor WithReverseProxyTermination
	// was supplied. (Phase 2E.8.)
	ErrMissingTLSConfig = errors.New("telegram: webhook mode requires WithTLS or WithReverseProxyTermination")

	// ErrInvalidInboundCapacity is returned by New when
	// WithInboundCapacity was set to a non-positive value. (Phase 2E.8.)
	ErrInvalidInboundCapacity = errors.New("telegram: inbound capacity must be positive")

	// ErrInvalidEnqueueTimeout is returned by New when
	// WithEnqueueTimeout was set to a non-positive value. (Phase 2E.8.)
	ErrInvalidEnqueueTimeout = errors.New("telegram: enqueue timeout must be positive")

	// ErrAlreadyStarted is returned by Adapter.Start when the
	// adapter has already been started. (Phase 2E.8.)
	ErrAlreadyStarted = errors.New("telegram: adapter already started")

	// ErrNotStarted is returned by Adapter.Send when the adapter
	// has not been started yet, or has been stopped. (Phase 2E.8.)
	ErrNotStarted = errors.New("telegram: adapter not started")

	// ErrUnknownOutboundKind is returned by Send when the Outbound
	// classification returns an OutboundKind value the dispatcher
	// does not recognise. Defensive — every existing kind has a
	// case. (Phase 2E.8.)
	ErrUnknownOutboundKind = errors.New("telegram: unknown outbound kind")
)

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package discord

import "errors"

// Exported errors — construction, the honest REST stub, and the terminal causes the
// reconnect supervisor can surface to the caller (a fatal, non-recoverable close or a
// missing secret).
var (
	// ErrMissingTokenEnv is returned by New when no token_env name is configured
	// (WithTokenEnv was not given). It is a structural error, independent of the
	// environment.
	ErrMissingTokenEnv = errors.New("discord: token_env is required (name of the env var holding the bot token)")

	// ErrMissingToken is returned by New when the named environment variable is
	// unset or empty, and is ALSO a fatal terminal cause if the token disappears
	// before a (re)connect. Callers wrap it with the env-var NAME only — the token
	// VALUE is never placed in an error or a log (ADR-0010).
	ErrMissingToken = errors.New("discord: bot token env var is not set")

	// ErrInvalidMode is returned by New for an unsupported transport mode. Discord
	// receives only over the Gateway, so ModeGateway is the sole valid mode.
	ErrInvalidMode = errors.New("discord: invalid or missing transport mode (supported: gateway)")

	// ErrMissingChannelID is returned by Send when the outbound Envelope carries no
	// destination channel id (conversation.id Meta key) — nowhere to deliver the reply.
	ErrMissingChannelID = errors.New("discord: outbound envelope has no channel id (conversation.id)")

	// ErrInvalidChannelID is returned by Send when the destination channel id is not a
	// numeric Discord snowflake — rejected at the edge so it cannot be injected into
	// the request URL.
	ErrInvalidChannelID = errors.New("discord: outbound channel id is not a valid snowflake")

	// ErrEmptyMessage is returned by Send when the outbound Envelope has no non-blank
	// text to deliver — refused rather than posting an empty message.
	ErrEmptyMessage = errors.New("discord: outbound envelope has no text content to send")

	// ErrSendUnauthorized wraps a REST 401 (the bot token is invalid). Named because a
	// retry will not help until the operator fixes the token.
	ErrSendUnauthorized = errors.New("discord: REST unauthorized (401): check the bot token")

	// ErrSendForbidden wraps a REST 403 (the bot lacks permission in the channel).
	ErrSendForbidden = errors.New("discord: REST forbidden (403): the bot lacks permission in this channel")

	// ErrChannelNotFound wraps a REST 404 (the target channel does not exist / the bot
	// cannot see it).
	ErrChannelNotFound = errors.New("discord: REST channel not found (404)")

	// ErrAlreadyReceiving is returned by a second Receive call on an Adapter whose
	// Gateway supervisor is already running. One Adapter drives one supervisor.
	ErrAlreadyReceiving = errors.New("discord: Receive was already called; one Adapter drives one Gateway supervisor")

	// ErrGatewayFatalClose is the terminal cause when the Gateway closes with a
	// non-recoverable close code (authentication failed, invalid/disallowed intents,
	// invalid shard/API version). The supervisor does NOT reconnect on these — it
	// closes the inbound channel and stops (retrying a bad token/intent forever would
	// be an infinite failure loop).
	ErrGatewayFatalClose = errors.New("discord: gateway closed with a fatal, non-recoverable code")
)

// Internal session-end signals. The supervisor classifies each into resume /
// re-identify / fatal / clean-stop; they are never surfaced to the caller raw.
var (
	// errUnexpectedFirstFrame — the first frame was not Hello (op 10); the handshake
	// is aborted and the supervisor reconnects.
	errUnexpectedFirstFrame = errors.New("discord: unexpected first gateway frame (expected Hello op 10)")

	// errZombie — a heartbeat was sent and never ACKed before the next was due; the
	// connection is dead. The supervisor reconnects (resume).
	errZombie = errors.New("discord: gateway connection is a zombie (no heartbeat ACK within the interval)")

	// errReconnectOp7 — the Gateway asked the client to reconnect (op 7). Resume.
	errReconnectOp7 = errors.New("discord: gateway requested a reconnect (op 7)")

	// errInvalidSessionResumable — Invalid Session (op 9) with d=true. Resume.
	errInvalidSessionResumable = errors.New("discord: gateway invalid session, resumable (op 9 d=true)")

	// errInvalidSessionFresh — Invalid Session (op 9) with d=false. Re-Identify.
	errInvalidSessionFresh = errors.New("discord: gateway invalid session, not resumable (op 9 d=false)")
)

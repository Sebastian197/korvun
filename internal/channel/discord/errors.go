// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package discord

import "errors"

var (
	// ErrMissingTokenEnv is returned by New when no token_env name is configured
	// (WithTokenEnv was not given). It is a structural error, independent of the
	// environment.
	ErrMissingTokenEnv = errors.New("discord: token_env is required (name of the env var holding the bot token)")

	// ErrMissingToken is returned by New when the named environment variable is
	// unset or empty. Callers wrap it with the env-var NAME only — the token VALUE
	// is never placed in an error or a log (ADR-0010).
	ErrMissingToken = errors.New("discord: bot token env var is not set")

	// ErrInvalidMode is returned by New for an unsupported transport mode. Discord
	// receives only over the Gateway, so ModeGateway is the sole valid mode.
	ErrInvalidMode = errors.New("discord: invalid or missing transport mode (supported: gateway)")

	// ErrSendNotImplemented is the honest sub-phase-1 stub for the REST send path
	// (createMessage with rate-limit handling lands in SP5).
	ErrSendNotImplemented = errors.New("discord: REST send is not implemented yet (sub-phase 5)")

	// ErrUnexpectedFirstFrame is returned when the first Gateway frame is not the
	// expected Hello (op 10), or its heartbeat_interval is non-positive. It aborts
	// the handshake rather than proceeding against an unknown protocol state.
	ErrUnexpectedFirstFrame = errors.New("discord: unexpected first gateway frame (expected Hello op 10)")

	// ErrZombieConnection is the terminal error when the Gateway stops acknowledging
	// heartbeats: a heartbeat was sent and no ACK (op 11) arrived before the next
	// beat was due, so the connection is dead. SP3 tears the connection down and
	// surfaces this; SP4 turns it into an automatic resume/reconnect.
	ErrZombieConnection = errors.New("discord: gateway connection is a zombie (no heartbeat ACK within the interval)")

	// ErrGatewayReconnect is the terminal error when the Gateway asks the client to
	// reconnect (op 7). SP3 closes with this named error; SP4 will resume the session.
	ErrGatewayReconnect = errors.New("discord: gateway requested a reconnect (op 7)")

	// ErrGatewayInvalidSession is the terminal error when the Gateway invalidates the
	// session (op 9). SP3 closes with this named error; SP4 will decide whether to
	// resume or re-identify (re-reading op 9's resumable flag then).
	ErrGatewayInvalidSession = errors.New("discord: gateway invalidated the session (op 9)")

	// ErrAlreadyReceiving is returned by a second Receive call on an Adapter whose
	// Gateway session is already running. One Adapter drives one session (SP4 adds
	// resume/reconnect on top); a concurrent second Receive would race the inbound
	// channel, so it fails loudly instead.
	ErrAlreadyReceiving = errors.New("discord: Receive was already called; one Adapter drives one Gateway session")
)

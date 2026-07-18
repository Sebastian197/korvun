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

	// ErrReceiveNotImplemented is the honest sub-phase-1 stub for the Gateway
	// receive path (the identify/heartbeat/dispatch state machine lands in SP3, and
	// resume/reconnect in SP4). Receive returns it rather than a dead channel.
	ErrReceiveNotImplemented = errors.New("discord: Gateway receive is not implemented yet (sub-phase 3)")

	// ErrSendNotImplemented is the honest sub-phase-1 stub for the REST send path
	// (createMessage with rate-limit handling lands in SP5).
	ErrSendNotImplemented = errors.New("discord: REST send is not implemented yet (sub-phase 5)")
)

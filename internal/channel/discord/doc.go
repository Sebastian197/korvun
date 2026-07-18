// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package discord is the Discord channel adapter (Piece 4, ADR-0033). It receives
// messages over the Discord Gateway — a WebSocket, so it depends on
// github.com/coder/websocket (ADR-0034) — and sends replies over the REST API,
// behind the channel.Channel seam that Telegram and Webhook already validate.
//
// Sub-phase 1 lands the dependency, the config surface (type "discord" / mode
// "gateway", validated in internal/config), and this skeleton. The constructor
// resolves the bot token SOLELY from the environment variable it is named after
// (ADR-0010: a missing name or an unset var is a loud, named error, and the token
// VALUE never appears in a log or error). Name/Manifest/Mode/DroppedCount are real;
// the not-yet-built paths (Receive over the Gateway, Send over REST) return
// explicit, honest errors, never silent no-ops.
//
// Later sub-phases fill this in: inbound MESSAGE_CREATE -> Envelope mapping (SP2),
// the Gateway lifecycle (identify/heartbeat/dispatch, SP3), resume/reconnect (SP4),
// REST send with rate-limit handling (SP5), and wiring into internal/app (SP6).
package discord

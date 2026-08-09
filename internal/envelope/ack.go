// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package envelope

// MetaAck marks a router-generated acknowledgement envelope (e.g. the
// session-reset ack). Channels that persist their own outbound — the
// console channel — use it to record acks as SYSTEM turns without ever
// double-persisting brain replies (operator-console spec FR-CONS-1).
const MetaAck = "korvun.ack"

// AckSessionReset is the MetaAck value for the session-reset ack.
const AckSessionReset = "session-reset"

// AckToolsReport is the MetaAck value for the /tools gatekeeper report
// (ADR-0041, FR-CHAT-1): a system response the console persists as a SYSTEM
// turn like any ack.
const AckToolsReport = "tools-report"

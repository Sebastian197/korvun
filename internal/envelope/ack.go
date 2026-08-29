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

// AckBrainFallback is the MetaAck value for the direct-brain fallback
// notice (B9): a conversation id asked for a brain that no longer exists,
// the message was handled by the route default, and the conversation is
// told so honestly (spec FR-B9-2). Self-persisting channels record it as
// a SYSTEM turn like every ack.
const AckBrainFallback = "brain-fallback"

// AckToolsReport is the MetaAck value for the /tools gatekeeper report
// (ADR-0041, FR-CHAT-1): a system response the console persists as a SYSTEM
// turn like any ack.
const AckToolsReport = "tools-report"

// AckRecall is the MetaAck value for the /recall acknowledgement
// (minimal-memory spec FR-RECALL-1, ADR-0043): a system response the
// console persists as a SYSTEM turn like any ack.
const AckRecall = "session-recall"

// AckNotesReport is the MetaAck value for the /notes report and its acks
// (minimal-memory spec FR-AUD-1, ADR-0043 §7 — the /tools molde):
// sanctioned conversation content the console persists as a SYSTEM turn.
const AckNotesReport = "notes-report"

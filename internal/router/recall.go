// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
)

// This file is the /recall command (minimal-memory spec FR-RECALL-1,
// ADR-0043 §2): a channel-agnostic first-token system command that imports
// the tail of the newest archived session WITH dialogue into an EMPTY active
// session as ONE quoted context block — deliberate, provenance-visible and
// non-duplicating by construction. Zero model involvement; the fixed replies
// ride the MetaAck molde (self-persisting channels record them as SYSTEM
// turns; the router itself persists nothing but the block).

// RecallTailWindow is C — how many tail turns one bounded LoadSessionTail
// probe reads per session (spec FR-RECALL-1: C = 32).
const RecallTailWindow = 32

// RecallScanSessions is S — how many archived sessions, newest→oldest, one
// /recall may probe for dialogue before honestly giving up (spec: S = 5).
const RecallScanSessions = 5

// RecallBlockRunes caps the rendered quoted block; oldest lines drop first
// and the header names the truncation (spec: 4000).
const RecallBlockRunes = 4000

// RecallBlockHeaderFormat renders the quoted block's first line: the
// provenance header naming the source session (fmt verb: session id).
const RecallBlockHeaderFormat = "[Recalled from session %d — quoted context, not new messages]"

// RecallBlockHeaderTruncatedFormat is the header when the block hit
// RecallBlockRunes and oldest lines were dropped (fmt verb: session id).
const RecallBlockHeaderTruncatedFormat = "[Recalled from session %d — quoted context, not new messages — truncated: oldest lines dropped]"

// RecallAckFormat is the fixed acknowledgement naming how many turns were
// imported and from which session (fmt verbs: count, session id).
const RecallAckFormat = "Recalled %d turns from session %d."

// RecallNothingReply is the fixed honest reply when no archived session
// within the scan bound holds dialogue.
const RecallNothingReply = "Nothing to recall — no previous session with dialogue."

// RecallRefusalReply is the fixed refusal when the active session already
// holds a non-system turn; it names /new as the way to start clean.
const RecallRefusalReply = "The active session already has messages — /recall works only on an empty session. Start one with /new."

// RecallUsageReply is the fixed reply for a malformed argument (k <= 0,
// non-numeric, or trailing text).
const RecallUsageReply = "Usage: /recall [n] — import the last n turns of the previous session into an empty one."

// RecallErrorReply is the fixed honest reply when the conversation store
// fails mid-command; never silence, never the model.
const RecallErrorReply = "Recall failed — the conversation store returned an error. Nothing was imported."

// maybeHandleRecall applies the /recall first-token command (FR-RECALL-1).
// It reports whether the envelope was fully handled here. Callers guarantee
// r.sessionStore is non-nil; the command is active only with RecallMax > 0
// (0 = disabled: the token falls through like any unknown command).
func (r *Router) maybeHandleRecall(ctx context.Context, key conversation.Key, env *envelope.Envelope, brainName string) bool {
	if r.sessionPolicy.RecallMax <= 0 {
		return false
	}
	first, rest, _ := strings.Cut(strings.TrimSpace(latestEnvelopeText(env.Parts)), " ")
	if first != "/recall" {
		return false
	}
	// The handled inbound is announced like every accepted message (the
	// estreno E-15 lesson, the /tools molde): without it the live view shows
	// the reply materializing with no corresponding received event.
	r.publishReceived(env, brainName)

	// Grammar: bare = the configured max; a single positive integer clamps
	// to it; anything else is the fixed usage reply, zero writes.
	k := r.sessionPolicy.RecallMax
	if rest = strings.TrimSpace(rest); rest != "" {
		fields := strings.Fields(rest)
		v, err := strconv.Atoi(fields[0])
		if len(fields) != 1 || err != nil || v <= 0 {
			r.recallReply(env, RecallUsageReply)
			return true
		}
		if v < k {
			k = v
		}
	}

	sessions, err := r.sessionStore.ListSessions(ctx, key)
	if err != nil {
		r.recallError(env, err)
		return true
	}

	// Precondition (P3): only into an EMPTY active session — no non-system
	// turn. Checked boundedly: one tail window plus the TurnCount, never a
	// whole session; a pathological all-system active larger than the
	// window is refused honestly rather than scanned unboundedly.
	if len(sessions) > 0 {
		active := sessions[len(sessions)-1]
		tail, err := r.sessionStore.LoadSessionTail(ctx, key, active.ID, RecallTailWindow)
		if err != nil {
			r.recallError(env, err)
			return true
		}
		if len(nonSystemTurns(tail)) > 0 || active.TurnCount > RecallTailWindow {
			r.recallReply(env, RecallRefusalReply)
			return true
		}
	}

	// Source: the newest ARCHIVED session with dialogue, scanning
	// newest→oldest across at most RecallScanSessions bounded probes;
	// ack-only sessions are skipped (the reset-ack persists into the NEW
	// session on the console, so bare-ack archives exist).
	var srcID int
	var dialogue []conversation.Turn
	archived := sessions
	if len(archived) > 0 {
		archived = archived[:len(archived)-1]
	}
	for i, probes := len(archived)-1, 0; i >= 0 && probes < RecallScanSessions; i, probes = i-1, probes+1 {
		tail, err := r.sessionStore.LoadSessionTail(ctx, key, archived[i].ID, RecallTailWindow)
		if err != nil {
			r.recallError(env, err)
			return true
		}
		if d := nonSystemTurns(tail); len(d) > 0 {
			srcID, dialogue = archived[i].ID, d
			break
		}
	}
	if srcID == 0 {
		r.recallReply(env, RecallNothingReply)
		return true
	}

	// Import shape: the last k non-system turns rendered into ONE RoleUser
	// quoted block. Operator lines render as Assistant, exactly as live
	// history replays them (translate.toModelRole).
	if len(dialogue) > k {
		dialogue = dialogue[len(dialogue)-k:]
	}
	lines := make([]string, 0, len(dialogue))
	for _, t := range dialogue {
		label := "User"
		if t.Role == conversation.RoleAssistant || t.Role == conversation.RoleOperator {
			label = "Assistant"
		}
		lines = append(lines, label+": "+t.Content)
	}
	block := composeRecallBlock(fmt.Sprintf(RecallBlockHeaderFormat, srcID), lines)
	if utf8.RuneCountInString(block) > RecallBlockRunes {
		header := fmt.Sprintf(RecallBlockHeaderTruncatedFormat, srcID)
		for len(lines) > 0 && utf8.RuneCountInString(composeRecallBlock(header, lines)) > RecallBlockRunes {
			lines = lines[1:] // oldest lines drop first
		}
		block = composeRecallBlock(header, lines)
	}

	if _, err := r.sessionStore.AppendTurns(ctx, key, conversation.Turn{
		Role:      conversation.RoleUser,
		Content:   block,
		Timestamp: r.clock(),
	}); err != nil {
		r.recallError(env, err)
		return true
	}

	// The ack names the REAL imported count (the lines actually quoted).
	r.recallReply(env, fmt.Sprintf(RecallAckFormat, len(lines), srcID))
	return true
}

// composeRecallBlock joins the provenance header and the quoted lines.
func composeRecallBlock(header string, lines []string) string {
	if len(lines) == 0 {
		return header
	}
	return header + "\n" + strings.Join(lines, "\n")
}

// nonSystemTurns filters a tail down to its dialogue (user, assistant,
// operator) — system acks are UI, not dialogue.
func nonSystemTurns(turns []conversation.Turn) []conversation.Turn {
	var out []conversation.Turn
	for _, t := range turns {
		if t.Role != conversation.RoleSystem {
			out = append(out, t)
		}
	}
	return out
}

// recallReply sends one fixed /recall reply through the normal outbound
// funnel, addressed back via the inbound's echoed Meta and marked with the
// MetaAck molde (self-persisting channels record it as a SYSTEM turn;
// network channels ignore the mark — FR-AUD-1).
func (r *Router) recallReply(env *envelope.Envelope, text string) {
	out := envelope.New(env.Channel, envelope.Outbound, korvun).AddText(text)
	for k, v := range env.Meta {
		out.Meta[k] = v
	}
	out.Meta[envelope.MetaAck] = envelope.AckRecall
	r.sendReply(out)
}

// recallError reports a mid-command store failure: the structured session
// error (%w-wrapped) plus the fixed honest reply — never silence, never the
// model (AS-A7).
func (r *Router) recallError(env *envelope.Envelope, err error) {
	r.notifyError(RouterError{Kind: ErrKindSession, Envelope: env, Err: fmt.Errorf("recall: %w", err)})
	r.recallReply(env, RecallErrorReply)
}

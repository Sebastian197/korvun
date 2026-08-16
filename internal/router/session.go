// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Session dispatch, the takeover gate, and the public outbound entry
// (operator-console spec 2026-08-08, SP2: FR-SESS-3/5, FR-TAKE-1,
// FR-OUT-1). All of it rides the EXISTING dispatch path and outbound
// funnel — no timers, no background goroutines: expiry is evaluated lazily
// at the next inbound with an injected clock, and operator replies share
// the brain replies' queue, events, and saturation semantics.

package router

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
)

// SessionResetAck is the FIXED acknowledgement a bare session trigger
// ("/new" or "/reset" alone) sends back through the normal outbound funnel —
// zero model involvement (FR-SESS-3). The copy is a spec-review item.
const SessionResetAck = "New session — previous context cleared."

// ErrNotOutbound is returned when DispatchOutbound receives an envelope
// whose Direction is not Outbound.
var ErrNotOutbound = errors.New("router: envelope direction is not outbound")

// korvun is the Sender stamped on the bare-trigger acknowledgement (the same
// shape the brains stamp on replies; delivery addressing rides the echoed
// Meta, not Sender).
var korvun = envelope.Participant{ID: "korvun", Name: "Korvun"}

// SessionPolicy configures session dispatch (FR-SESS-3/5). The zero value
// disables everything: no triggers, no automatic expiry — upgrading changes
// nothing until configured (the spec's `none` default).
type SessionPolicy struct {
	// Triggers are the exact-match reset commands (first token,
	// case-sensitive), e.g. ["/new", "/reset"]. Empty disables manual
	// channel resets.
	Triggers []string

	// Daily, when true, expires the active session at the FIRST inbound
	// after the DailyHour:DailyMin boundary in the clock's location
	// (lazy — no timers).
	Daily     bool
	DailyHour int
	DailyMin  int

	// IdleMin, when > 0, expires the active session at the first inbound
	// arriving more than IdleMin minutes after the session's last activity.
	IdleMin int

	// RecallMax, when > 0, enables the /recall command and caps how many
	// archived turns one recall may import (minimal-memory spec
	// FR-RECALL-1/FR-CFG-A1; 0 disables — the token falls through like any
	// unknown command).
	RecallMax int
}

// WithSessionStore wires the sessionful conversation store into the dispatch
// path. nil (the default) disables session dispatch, takeover persistence,
// and operator-turn persistence — the router behaves exactly as before.
func WithSessionStore(s conversation.SessionStore) Option {
	return func(r *Router) { r.sessionStore = s }
}

// WithSessionPolicy sets the session dispatch policy (FR-SESS-3/5).
func WithSessionPolicy(p SessionPolicy) Option {
	return func(r *Router) { r.sessionPolicy = p }
}

// WithClock injects the time source used by session expiry and turn
// timestamps (AS-11: the tests travel in time with it). nil keeps time.Now.
func WithClock(now func() time.Time) Option {
	return func(r *Router) {
		if now != nil {
			r.clock = now
		}
	}
}

// TakeOver silences the brain for key's conversation (FR-TAKE-1): inbound
// messages are persisted as user turns but never dispatched to the brain
// until Release. The gate is per conversation and spans sessions — a session
// reset does not lift it. State is in-memory on purpose: a restart fails
// open to the brain, never to silence (spec folded decision).
func (r *Router) TakeOver(key conversation.Key) {
	r.takeoverMu.Lock()
	r.takeover[key] = struct{}{}
	r.takeoverMu.Unlock()
}

// Release lifts the takeover gate for key. Idempotent.
func (r *Router) Release(key conversation.Key) {
	r.takeoverMu.Lock()
	delete(r.takeover, key)
	r.takeoverMu.Unlock()
}

// TakenOver reports whether key's conversation is currently taken over.
func (r *Router) TakenOver(key conversation.Key) bool {
	r.takeoverMu.RLock()
	_, ok := r.takeover[key]
	r.takeoverMu.RUnlock()
	return ok
}

// DispatchOutbound is the public outbound entry (FR-OUT-1): the operator's
// reply enters the SAME per-channel outbound funnel as brain replies (same
// queue, same saturation semantics, same delivery events) — zero model
// involvement by construction. Persist-then-send (spec folded decision): the
// operator turn is recorded in the conversation's active session BEFORE the
// enqueue, so the brain regains full context after a takeover; a turn that
// cannot be recorded does not go out.
func (r *Router) DispatchOutbound(ctx context.Context, env *envelope.Envelope) error {
	if env == nil {
		return ErrNilEnvelope
	}
	if env.Direction != envelope.Outbound {
		return ErrNotOutbound
	}
	r.mu.RLock()
	if r.shutdown {
		r.mu.RUnlock()
		return ErrShutdown
	}
	cw, ok := r.channels[env.Channel]
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownChannel, env.Channel)
	}

	if r.sessionStore != nil {
		if key, err := conversation.KeyFromEnvelope(env); err == nil {
			if text := latestEnvelopeText(env.Parts); text != "" {
				if _, err := r.sessionStore.Append(ctx, key, conversation.Turn{
					Role:      conversation.RoleOperator,
					Content:   text,
					Timestamp: r.clock(),
				}); err != nil {
					return fmt.Errorf("router: persist operator turn %q: %w", key, err)
				}
			}
		}
	}

	if r.outboundEnqueueTimeout <= 0 {
		select {
		case cw.queue <- env:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-r.ctx.Done():
			return ErrShutdown
		}
	}
	timer := time.NewTimer(r.outboundEnqueueTimeout)
	defer timer.Stop()
	select {
	case cw.queue <- env:
		return nil
	case <-timer.C:
		return fmt.Errorf("%w: %q", ErrChannelSaturated, env.Channel)
	case <-ctx.Done():
		return ctx.Err()
	case <-r.ctx.Done():
		return ErrShutdown
	}
}

// sessionPreDispatch runs the session pipeline for one inbound envelope,
// AFTER routing validation and BEFORE the brain enqueue: lazy expiry, then
// trigger handling, then the takeover gate. It returns the envelope to
// dispatch (possibly rewritten to a trigger's remainder) and whether the
// message was fully handled here (trigger ack, or silenced by takeover).
func (r *Router) sessionPreDispatch(ctx context.Context, env *envelope.Envelope, brainName string) (*envelope.Envelope, bool) {
	key, err := conversation.KeyFromEnvelope(env)
	if err != nil {
		return env, false // no conversation identity: session logic cannot apply
	}

	if r.sessionStore != nil {
		r.maybeExpireSession(ctx, key, env)
		out, handled := r.maybeHandleTrigger(ctx, key, env)
		if handled {
			return nil, true
		}
		if out != nil {
			env = out
		}
		// /recall runs AFTER the lazy expiry and the triggers by
		// construction (FR-RECALL-1): a /recall following an idle/daily cut
		// recovers from the just-archived session — the primary case.
		if r.maybeHandleRecall(ctx, key, env, brainName) {
			return nil, true
		}
	}

	// /notes rides beside /recall (FR-RECALL-2) but needs no session store:
	// the app-composed closures own the notes; an unconfigured brain lets
	// the token fall through to the model.
	if r.maybeHandleNotes(ctx, key, env, brainName) {
		return nil, true
	}

	// The /tools gatekeeper report (FR-CHAT-1) answers BEFORE the takeover
	// gate: it is a system command, so it works whether or not a human has
	// silenced the brain.
	if r.maybeHandleToolsCommand(env, brainName) {
		return nil, true
	}

	if r.TakenOver(key) {
		// The human is the brain: persist the user's words so nothing is
		// lost (FR-TAKE-1), still announce the message on the bus (the
		// console's change signal), and skip the brain entirely.
		if r.sessionStore != nil {
			// TranscriptText: a media message under takeover persists its
			// honest markers too (FR-ATTACH) — the human at the console
			// must see WHAT arrived, never a mute void.
			if text := envelope.TranscriptText(env.Parts); text != "" {
				if _, err := r.sessionStore.Append(ctx, key, conversation.Turn{
					Role:      conversation.RoleUser,
					Content:   text,
					Timestamp: r.clock(),
				}); err != nil {
					r.notifyError(RouterError{Kind: ErrKindSession, Envelope: env, Err: err})
				}
			}
		}
		r.publishReceived(env, brainName)
		return nil, true
	}

	return env, false
}

// maybeExpireSession applies the lazy daily/idle expiry (FR-SESS-5):
// evaluated ONLY here, at an inbound, against the injected clock — no
// timers. First rule to fire wins; NewSession's empty-active idempotence
// guarantees a combined firing opens exactly one session.
func (r *Router) maybeExpireSession(ctx context.Context, key conversation.Key, env *envelope.Envelope) {
	p := r.sessionPolicy
	if !p.Daily && p.IdleMin <= 0 {
		return
	}
	sessions, err := r.sessionStore.ListSessions(ctx, key)
	if err != nil {
		r.notifyError(RouterError{Kind: ErrKindSession, Envelope: env, Err: err})
		return
	}
	if len(sessions) == 0 {
		return
	}
	last := sessions[len(sessions)-1].Last
	if last.IsZero() {
		return // the active session is empty — nothing to cut
	}
	now := r.clock()
	expired := p.IdleMin > 0 && now.Sub(last) > time.Duration(p.IdleMin)*time.Minute
	if !expired && p.Daily {
		boundary := time.Date(now.Year(), now.Month(), now.Day(), p.DailyHour, p.DailyMin, 0, 0, now.Location())
		if boundary.After(now) {
			boundary = boundary.AddDate(0, 0, -1)
		}
		expired = last.Before(boundary)
	}
	if !expired {
		return
	}
	if _, err := r.sessionStore.NewSession(ctx, key); err != nil {
		r.notifyError(RouterError{Kind: ErrKindSession, Envelope: env, Err: err})
	}
}

// maybeHandleTrigger applies the manual channel reset (FR-SESS-3):
// exact-match on the FIRST token, case-sensitive, configured set. On a
// match it opens a new session BEFORE dispatch; the remainder (if any)
// becomes the message the brain sees, in a rewritten envelope — the trigger
// text itself is never persisted and never reaches the brain. A bare
// trigger sends the fixed acknowledgement through the normal outbound
// funnel and ends the dispatch. Store failures fail OPEN: the original
// message continues to the brain untouched.
func (r *Router) maybeHandleTrigger(ctx context.Context, key conversation.Key, env *envelope.Envelope) (*envelope.Envelope, bool) {
	if len(r.sessionPolicy.Triggers) == 0 {
		return nil, false
	}
	text := latestEnvelopeText(env.Parts)
	if text == "" {
		return nil, false
	}
	trimmed := strings.TrimSpace(text)
	first, rest, _ := strings.Cut(trimmed, " ")
	if !slices.Contains(r.sessionPolicy.Triggers, first) {
		return nil, false
	}
	if _, err := r.sessionStore.NewSession(ctx, key); err != nil {
		r.notifyError(RouterError{Kind: ErrKindSession, Envelope: env, Err: err})
		return nil, false
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		r.sendReply(triggerAck(env))
		return nil, true
	}
	return rewriteEnvelopeText(env, rest), false
}

// maybeHandleToolsCommand answers the /tools first-token command on the
// configured tools channel (ADR-0041 FR-CHAT-1): a SYSTEM response carrying
// the gatekeeper report for the conversation's routed brain, through the
// normal outbound funnel, zero brain involvement. Any trailing text after
// the token is ignored — the report is the whole answer. nil reporter or a
// different channel/token = not handled.
func (r *Router) maybeHandleToolsCommand(env *envelope.Envelope, brainName string) bool {
	if r.toolsReport == nil || env.Channel != r.toolsChannel {
		return false
	}
	first, _, _ := strings.Cut(strings.TrimSpace(latestEnvelopeText(env.Parts)), " ")
	if first != "/tools" {
		return false
	}
	out := envelope.New(env.Channel, envelope.Outbound, korvun).AddText(r.toolsReport(brainName))
	for k, v := range env.Meta {
		out.Meta[k] = v
	}
	out.Meta[envelope.MetaAck] = envelope.AckToolsReport
	// The handled inbound is announced like every accepted message (estreno
	// E-15): without this, the live view showed the REPLY materializing with
	// no corresponding received event.
	r.publishReceived(env, brainName)
	r.sendReply(out)
	return true
}

// triggerAck builds the fixed, model-free acknowledgement for a bare
// trigger, addressed back through the inbound's echoed Meta (the same
// delivery contract brain replies use).
func triggerAck(in *envelope.Envelope) *envelope.Envelope {
	out := envelope.New(in.Channel, envelope.Outbound, korvun).AddText(SessionResetAck)
	for k, v := range in.Meta {
		out.Meta[k] = v
	}
	// Marked as an ack (FR-CONS-1): self-persisting channels (console)
	// record marked envelopes as SYSTEM turns; network channels ignore it.
	out.Meta[envelope.MetaAck] = envelope.AckSessionReset
	return out
}

// rewriteEnvelopeText returns a copy of env whose Parts carry only the given
// text. A COPY on purpose: the original inbound may already be visible to
// other components, and published envelopes are immutable by invariant.
func rewriteEnvelopeText(env *envelope.Envelope, text string) *envelope.Envelope {
	out := *env
	out.Parts = []envelope.Part{{Type: envelope.Text, Content: text}}
	return &out
}

// latestEnvelopeText returns the Content of the last Text Part with
// non-whitespace content, or "" — the router-side twin of the brains'
// latestText (each side stays dependency-free of the other).
func latestEnvelopeText(parts []envelope.Part) string {
	text := ""
	for _, p := range parts {
		if p.Type == envelope.Text && strings.TrimSpace(p.Content) != "" {
			text = p.Content
		}
	}
	return text
}

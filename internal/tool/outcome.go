// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The ONE classifier of what a tool run closes on (director's class cure,
// 2026-09-16). Both execution paths — the approved one in internal/app and
// the brain's own in internal/brain — call this and nothing else.
//
// It lives here because the classes it reads are the tools' own: past its Do
// call, webhook_call wraps exactly one of ErrEffectDelivered (the receiver
// answered, not with a usable success), ErrDeliveryUnknown (a connection was
// obtained, no answer) and ErrNotSent (no connection was obtained), and this
// package owns those sentinels. Before Do — a scheme or allow-list refusal, a
// body that is not JSON — it wraps none, and such an error, like any tool's
// that wraps none, is classified by the error alone. Two copies of the rule
// is the defect this replaces: the v0.15.0 train cured the paths one at a time
// and they disagreed about a bare deadline until the second cure caught up.
package tool

import (
	"context"
	"errors"

	"github.com/Sebastian197/korvun/internal/action"
)

// CloseStateAfterRun names the state an attempt closes on, given the error its
// tool returned. One table, checked in this order (v0.15.1 block A):
//
//   - nil: the tool answered. SUCCEEDED.
//   - ErrEffectDelivered: the receiver answered and the answer is not a
//     usable success (a status outside 2xx, a refused redirect, an unreadable
//     or oversized body). The effect may have happened. OUTCOME_UNKNOWN.
//   - ErrDeliveryUnknown: a connection was obtained and no response was read.
//     Whether anything left is not known. OUTCOME_UNKNOWN.
//   - ErrNotSent: no connection was ever obtained, so no request byte left —
//     even when the run ended on a deadline or a cancellation. FAILED.
//   - a context that ended (deadline or cancellation) and carries none of the
//     three classes:
//     a tool that does not observe the wire may have produced its effect.
//     OUTCOME_UNKNOWN.
//   - anything else: a tool that refused before it acted — a host off the
//     allow-list, a malformed body. That is a DECIDED outcome, and FAILED is
//     the honest word for it.
func CloseStateAfterRun(err error) action.State {
	switch {
	case err == nil:
		return action.StateSucceeded
	case errors.Is(err, ErrEffectDelivered),
		errors.Is(err, ErrDeliveryUnknown):
		return action.StateOutcomeUnknown
	case errors.Is(err, ErrNotSent):
		return action.StateFailed
	case errors.Is(err, context.DeadlineExceeded),
		errors.Is(err, context.Canceled):
		return action.StateOutcomeUnknown
	default:
		return action.StateFailed
	}
}

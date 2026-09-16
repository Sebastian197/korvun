// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The ONE classifier of what a tool run closes on (director's class cure,
// 2026-09-16). Both execution paths — the approved one in internal/app and
// the brain's own in internal/brain — call this and nothing else.
//
// It lives here because the frontier it reads is the tools' own: the caged
// tools wrap ErrEffectDelivered past the point where the request reached the
// wire, and this package owns that sentinel. Two copies of the rule is the
// defect this replaces: the v0.15.0 train cured the paths one at a time and
// they disagreed about a bare deadline until the second cure caught up.
package tool

import (
	"context"
	"errors"

	"github.com/Sebastian197/korvun/internal/action"
)

// CloseStateAfterRun names the state an attempt closes on, given the error its
// tool returned.
//
//   - nil: the tool answered. SUCCEEDED.
//   - ErrEffectDelivered: the request reached the wire and its answer was not
//     read. The effect may have happened; nobody can say it did not.
//   - a context that ended (deadline or cancellation): the tool may have
//     produced its effect and then lost its context. Same uncertainty.
//   - anything else: the tool refused before anything left — a host off the
//     allow-list, a shield refusal at the dial, a malformed body, a failed
//     dial. That is a DECIDED outcome, and FAILED is the honest word for it.
//
// What it does NOT decide: whether a context that ended did so BEFORE the
// request was written. That case closes unknown here too, and it is filed for
// v0.15.1 on both paths alike rather than guessed at.
func CloseStateAfterRun(err error) action.State {
	switch {
	case err == nil:
		return action.StateSucceeded
	case errors.Is(err, ErrEffectDelivered),
		errors.Is(err, context.DeadlineExceeded),
		errors.Is(err, context.Canceled):
		return action.StateOutcomeUnknown
	default:
		return action.StateFailed
	}
}

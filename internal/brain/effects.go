// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The effect engine's adapter half (Trust Layer Etapa 3, lote 2, spec
// FR-ENV-1 + FR-REG-2/3): the boot wires a classifier — in production,
// tool.BuiltinEffects — and every recorded envelope wakes its REAL class
// from it. The classifier's input is the OPERATION NAME alone (§9.7:
// never parameters, never model text — pinned by the trap test). A nil
// classifier keeps the pre-stage behavior byte-for-byte (the house seam
// mold: ActionRecorder, ActionIdentity, now this).
package brain

import (
	"context"
	"fmt"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// EffectClassifier classifies one operation by NAME from the declared
// registry. The signature is the §9.7 law made compile-time: there is
// nowhere to pass parameters or model text.
type EffectClassifier func(name string) (action.EffectDescriptor, bool)

// WithEffectClassifier wires the effect engine into the brain.
func WithEffectClassifier(classify EffectClassifier) AgentOption {
	return func(a *AgentBrain) { a.effects = classify }
}

// classifyEffect resolves an operation's effect for the envelope:
// (class, true) when the registry declares it; ("", false) when the
// classifier exists but does not know the name — the gate's
// effect_undeclared wall fires on that; (placeholder, true) when no
// classifier is wired (pre-stage behavior, the honest E1 placeholder).
func (a *AgentBrain) classifyEffect(name string) (string, bool) {
	if a.effects == nil {
		return "unclassified", true
	}
	descriptor, declared := a.effects(name)
	if !declared {
		return "", false
	}
	return string(descriptor.Class), true
}

// effectGateRule is the effect tier of the action gate (Etapa 3, spec
// FR-CEIL-3 + FR-REQ, sealed NC-2(b)), a PURE function with DETERMINISTIC
// precedence (§13.3 subset) — the first applicable denial wins:
//
//  1. effect_ceiling — the class exceeds the bounded authority's ceiling
//     (unknown classes rank above critical and never fit).
//  2. approval_unavailable — under BOUNDED authority (a ceiling present;
//     never the root's standing authority), write_irreversible and
//     critical demand human approval, and no approval workflow exists
//     until E5: the honest no that E5 will turn into an approval card.
//  3. prepare_unavailable — whatever demands a prepare phase dies while
//     no connector supports one (the demand signal stays structurally
//     unreachable until the descriptor's prepare field wakes in E5/E6;
//     the wall exists NOW, authority-independent).
//
// "" means the effect tier lets the attempt proceed.
func effectGateRule(class action.EffectClass, ceiling action.EffectClass, requiresPrepare bool) string {
	if ceiling != "" {
		if class.Rank() > ceiling.Rank() {
			return "effect_ceiling"
		}
		if class == action.EffectWriteIrreversible || class == action.EffectCritical {
			return "approval_unavailable"
		}
	}
	if requiresPrepare {
		return "prepare_unavailable"
	}
	return ""
}

// approvalRequester is the OPTIONAL extension of the recorder seam
// (Etapa 5, spec FR-GATE, sealed NC-2): a recorder that can turn the
// gate's honest approval_unavailable into a full request — the action
// parked PENDING_APPROVAL with its sealed preview and its canonical
// parameters, born whole in one transaction. The app wires it ONLY
// when approvals.enabled; without it the E3 denial stands
// byte-for-byte forever (the sacred pin). Interface-assertion mold.
type approvalRequester interface {
	RequestApproval(ctx context.Context, env action.Envelope, rule string, rawParams string) (string, error)
}

// pendingApprovalObservation is the honest observation the model
// receives when an action parks for human approval: unmistakably
// PENDING — not an error, not a denial, not a success — naming the
// request so the operator's decision can be referenced later. The
// conversation never blocks waiting for a human (FR-GATE-2).
func pendingApprovalObservation(name, approvalID string, expiresAt time.Time) string {
	expiry := ""
	if !expiresAt.IsZero() {
		expiry = " The request expires at " + expiresAt.UTC().Format(time.RFC3339) + "."
	}
	return fmt.Sprintf("PENDING APPROVAL: the tool %s was NOT executed yet. This action requires "+
		"human approval and is parked as request %s awaiting the operator's decision.%s "+
		"Tell the user plainly that the action awaits approval — do NOT retry it, do NOT "+
		"offer to do it manually, and do NOT treat this as a failure.", name, approvalID, expiry)
}

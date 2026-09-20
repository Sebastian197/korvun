// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// This file keeps the brain-facing effect classifier and approval observation
// seams. The executor consumes the classifier by operation name only; no
// parameters or model text cross that boundary.
package brain

import (
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

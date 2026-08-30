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

import "github.com/Sebastian197/korvun/internal/action"

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

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// This file keeps the brain-facing identity configuration and recorder alias.
// The executor consumes both to bind provenance before durable admission.
package brain

import (
	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/action/executor"
)

// ActionIdentity is the boot-wired identity context: the config-pinned
// provenance registry, the intent today's flows record under (the root),
// and the derived grant that EXPLAINS this brain's governed allows
// ("" when the brain carries no config governance).
type ActionIdentity struct {
	// Registry maps configured channel names to their provenance.
	Registry action.ProvenanceRegistry
	// IntentID is the intent recorded on every attempt.
	IntentID string
	// GrantID is the derived grant referenced on rule "granted".
	GrantID string
	// EffectCeiling bounds the effect class this identity's authority may
	// reach (Etapa 3, FR-CEIL-3): "" = no ceiling — production's derived
	// grants carry none today, which keeps the exterior byte-for-byte.
	EffectCeiling action.EffectClass
}

// IdentifiedRecorder is the OPTIONAL identity-aware extension of the
// ActionRecorder seam (FR-EVID-2 live): the attempt, its decision AND
// its evidence land through one call — the store commits them in one
// transaction. Consumer-side, like the base seam; the app's adapter
// implements both.
type IdentifiedRecorder = executor.IdentifiedRecorder

// WithActionIdentity wires the identity context into the brain.
func WithActionIdentity(id ActionIdentity) AgentOption {
	return func(a *AgentBrain) { a.identity = &id }
}

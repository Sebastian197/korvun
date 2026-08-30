// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The identity-aware half of the Action Kernel adapter (Trust Layer
// Etapa 2, lote 4, spec FR-ENV-1 + FR-PRIN-3/4): the app wires the
// provenance registry, the root intent and the brain's derived grant at
// boot; every hot-path attempt then fills its identity refs and mints
// its per-attempt evidence — kernel-side, adapters changing ZERO lines.
// A nil identity (or a recorder without the identified seam) keeps the
// Etapa-1 path byte-for-byte.
package brain

import (
	"context"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/envelope"
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
}

// IdentifiedRecorder is the OPTIONAL identity-aware extension of the
// ActionRecorder seam (FR-EVID-2 live): the attempt, its decision AND
// its evidence land through one call — the store commits them in one
// transaction. Consumer-side, like the base seam; the app's adapter
// implements both.
type IdentifiedRecorder interface {
	RecordAttemptIdentified(ctx context.Context, env action.Envelope, outcome, rule string, state action.State, evidence action.IdentityEvidence) error
}

// WithActionIdentity wires the identity context into the brain.
func WithActionIdentity(id ActionIdentity) AgentOption {
	return func(a *AgentBrain) { a.identity = &id }
}

// identifiedRecorder returns the identity-aware seam when BOTH halves
// are wired: an identity context and a recorder that implements it.
func (a *AgentBrain) identifiedRecorder() (IdentifiedRecorder, bool) {
	if a.identity == nil {
		return nil, false
	}
	ir, ok := a.actions.(IdentifiedRecorder)
	return ir, ok
}

// identify resolves one attempt's identity from AUTHENTICATED provenance
// and fills the envelope's identity refs: the acting principal is the
// BRAIN (agent_brain answering to the operator, §14.2); the channel's
// resolved evidence carries the sender strictly as its subject; the
// derived grant is referenced only when "granted" names config authority
// (an ungoverned allow acts directly under the root's standing
// authority; a denial has no explaining grant). Reports false — without
// inventing any principal — when the channel is absent from the
// registry.
func (a *AgentBrain) identify(env *envelope.Envelope, e *action.Envelope, rule string) (action.IdentityEvidence, bool) {
	_, evidence, err := action.ResolvePrincipal(a.identity.Registry, env.Channel, env.Sender.ID, a.now())
	if err != nil {
		return action.IdentityEvidence{}, false
	}
	brainPrincipal := action.BrainPrincipal(a.name)
	e.Principal = action.PrincipalRef{
		PrincipalID:        brainPrincipal.PrincipalID,
		EvidenceID:         evidence.EvidenceID,
		ResponsibleHumanID: brainPrincipal.ResponsibleHumanID,
	}
	e.IntentID = a.identity.IntentID
	if rule == "granted" && a.identity.GrantID != "" {
		e.AuthorityRefs = []string{a.identity.GrantID}
	}
	return evidence, true
}

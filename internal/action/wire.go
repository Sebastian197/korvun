// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Wiring domain (Trust Layer Etapa 2, lote 4, spec FR-ENV-1 + FR-MIG-1 +
// sealed decision 2): the identity references the envelope wakes, the
// deterministic root intent the app materializes at boot, and the
// config-derived grants that EXPLAIN today's governed allows without
// judging them — config stays the single source of truth and SelectTools
// stays THE judge (wrap, never replace).
package action

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// RootIntentID is the root intent's identity (sealed decision 1: the
// single operator's standing authority, auto-materialized at boot).
const RootIntentID = "int_root"

// PrincipalRef is the envelope's identity reference (spec FR-ENV-1): WHO
// acted, under WHICH evidence, answering to WHOM — references only,
// never authorization inputs.
type PrincipalRef struct {
	// PrincipalID names the acting principal.
	PrincipalID string
	// EvidenceID names the identity evidence recorded with the attempt.
	EvidenceID string
	// ResponsibleHumanID names the human behind an agent principal.
	ResponsibleHumanID string
}

// NewIntentID generates a fresh operator-intent identity ("int_" + 16
// random bytes hex, the NewID mold).
func NewIntentID() string {
	b := make([]byte, 16)
	// crypto/rand.Read is documented (Go ≥1.24) to always succeed.
	_, _ = rand.Read(b)
	return "int_" + hex.EncodeToString(b)
}

// NewGrantID generates a fresh operator-grant identity ("grant_" + 16
// random bytes hex).
func NewGrantID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "grant_" + hex.EncodeToString(b)
}

// RootIntent returns the root intent contract: the operator's standing
// authority made explicit. TIMELESS (zero ValidFrom/ExpiresAt) and
// unlimited (FR-BUD-2), so its digest is deterministic across every
// install and every boot — idempotent materialization needs no clock.
func RootIntent() IntentContract {
	return IntentContract{
		IntentID:          RootIntentID,
		SchemaVersion:     1,
		OwnerPrincipalID:  OperatorPrincipal().PrincipalID,
		Purpose:           "operate this Korvun instance under the operator's standing authority",
		AllowedOperations: []string{"*"},
		AllowedResources:  []string{"*"},
		Status:            LifecycleActive,
		Version:           1,
	}
}

// DeriveConfigGrant derives the IN-MEMORY grant that explains one brain's
// configured authority under the root intent (FR-MIG-1): subject = the
// brain principal, issuer = the operator, operations = the brain's
// allowed tools, resources = the carried channel restrictions (or "*").
// The id is deterministic across boots (`grant_cfg_<digest>`, AS-7):
// derived from the grant TERMS, so the same config always derives the
// same id and a config edit derives a new one. Depth 0: config grants do
// not delegate in E2. Derived, never persisted — config remains the
// single source of truth (FR-MIG-2).
func DeriveConfigGrant(brainName string, operations, resources []string) AuthorityGrant {
	g := AuthorityGrant{
		IntentID:           RootIntentID,
		IssuerPrincipalID:  OperatorPrincipal().PrincipalID,
		SubjectPrincipalID: BrainPrincipal(brainName).PrincipalID,
		Operations:         operations,
		ResourceScope:      resources,
		Status:             LifecycleActive,
	}
	// The digest covers the terms with an empty GrantID; the id is then
	// minted FROM it, keeping the id ↔ terms bond circular-free.
	digest := strings.TrimPrefix(g.Digest(), "sha256:")
	g.GrantID = "grant_cfg_" + digest[:16]
	return g
}

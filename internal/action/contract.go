// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Intent Contract and Authority Grant foundations (Trust Layer Etapa 2,
// lote 1, spec FR-INT-1/FR-AUTH-1): the domain structs and their shared
// fail-closed lifecycle. Persistence arrives in lote 3; the attenuation
// validator (§14.3) is lote 2's piece. RESERVED and documented until
// their stages: `signature` (E4 — receipts own signing), `effect_ceiling`
// and `data_scope` (E3 — storing an unenforceable ceiling would be
// governance theater), `tenant_id` (E10).
package action

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

// LifecycleStatus is the shared contract lifecycle (§10.3/§10.4):
// DRAFT → ACTIVE → EXPIRED | REVOKED, fail-closed like every kernel
// machine.
type LifecycleStatus string

const (
	// LifecycleDraft is a contract not yet in force.
	LifecycleDraft LifecycleStatus = "DRAFT"
	// LifecycleActive is the only status that authorizes anything.
	LifecycleActive LifecycleStatus = "ACTIVE"
	// LifecycleExpired is terminal: the validity window closed.
	LifecycleExpired LifecycleStatus = "EXPIRED"
	// LifecycleRevoked is terminal: a human withdrew the contract.
	LifecycleRevoked LifecycleStatus = "REVOKED"
)

// ErrInvalidLifecycleTransition is the sentinel every rejected lifecycle
// edge wraps.
var ErrInvalidLifecycleTransition = errors.New("action: invalid lifecycle transition")

// lifecycleTransitions is the COMPLETE table; anything absent is invalid,
// so terminal exits, unknown states and self-loops fail closed by
// construction (the Etapa-1 machine mold).
var lifecycleTransitions = map[LifecycleStatus]map[LifecycleStatus]bool{
	LifecycleDraft:  {LifecycleActive: true},
	LifecycleActive: {LifecycleExpired: true, LifecycleRevoked: true},
}

// LifecycleTransition validates one lifecycle edge: nil for the table,
// the wrapped sentinel for everything else.
func LifecycleTransition(from, to LifecycleStatus) error {
	if lifecycleTransitions[from][to] {
		return nil
	}
	return fmt.Errorf("%w: %s -> %s", ErrInvalidLifecycleTransition, from, to)
}

// Terminal reports whether s admits no further transitions.
func (s LifecycleStatus) Terminal() bool {
	switch s {
	case LifecycleExpired, LifecycleRevoked:
		return true
	default:
		return false
	}
}

// Budgets are the v1 quantitative limits of an intent or grant
// (FR-BUD-1). The zero value means UNLIMITED — the root intent's standing
// authority carries no budget (FR-BUD-2); limits bite only when set.
type Budgets struct {
	// MaxActions caps total recorded AUTHORIZED actions under the
	// contract. Zero = unlimited.
	MaxActions int
	// MaxActionsPerOperation optionally caps per operation name.
	MaxActionsPerOperation map[string]int
}

// IntentContract is the §10.3 v1 subset: the authorized outcome and its
// limits. Resources are COARSE in v1 (operation-level names or "*");
// fine-grained resources arrive with the Effect Engine (E3).
type IntentContract struct {
	// IntentID is the contract identity ("int_" namespace).
	IntentID string
	// SchemaVersion pins the contract schema (1).
	SchemaVersion int
	// OwnerPrincipalID is the human owner (the operator in E2).
	OwnerPrincipalID string
	// Purpose states the authorized outcome in words.
	Purpose string
	// AllowedOperations is the operation SET the intent covers.
	AllowedOperations []string
	// AllowedResources is the coarse resource SET ("*" in v1 flows).
	AllowedResources []string
	// Budgets are the quantitative limits (zero value = unlimited).
	Budgets Budgets
	// ValidFrom / ExpiresAt bound the window; zero ExpiresAt = no expiry.
	ValidFrom time.Time
	ExpiresAt time.Time
	// Status is the runtime lifecycle — NOT part of the contract digest.
	Status LifecycleStatus
	// Version increments on contract amendment.
	Version int
}

// Digest returns the deterministic contract digest: the CONTRACT TERMS
// (identity, owner, purpose, sorted operation/resource sets, budgets,
// window, version) through the fuzzed canonicalizer — runtime Status is
// deliberately excluded: revocation closes a contract's lifecycle, it
// does not rewrite its identity.
func (c IntentContract) Digest() string {
	return HashCanonical(contractTerms(map[string]any{
		"intent_id":      c.IntentID,
		"schema_version": c.SchemaVersion,
		"owner":          c.OwnerPrincipalID,
		"purpose":        c.Purpose,
		"operations":     sortedSet(c.AllowedOperations),
		"resources":      sortedSet(c.AllowedResources),
		"budgets":        budgetTerms(c.Budgets),
		"valid_from":     timeTerm(c.ValidFrom),
		"expires_at":     timeTerm(c.ExpiresAt),
		"version":        c.Version,
	}))
}

// AuthorityGrant is the §10.4 v1 subset: limited authority inside an
// intent, delegable only by attenuation (the lote-2 validator).
type AuthorityGrant struct {
	// GrantID is the grant identity ("grant_" namespace).
	GrantID string
	// IntentID ties the grant to its intent.
	IntentID string
	// IssuerPrincipalID granted; SubjectPrincipalID received.
	IssuerPrincipalID  string
	SubjectPrincipalID string
	// ParentGrantID names the delegation parent ("" = issued from the
	// intent itself).
	ParentGrantID string
	// Operations / ResourceScope are the granted SETS.
	Operations    []string
	ResourceScope []string
	// Budgets are the grant's quantitative limits (zero = unlimited).
	Budgets Budgets
	// ValidFrom / ExpiresAt bound the grant window.
	ValidFrom time.Time
	ExpiresAt time.Time
	// DelegationDepthRemaining bounds further delegation (§14.3: a child
	// must have strictly less).
	DelegationDepthRemaining int
	// Status is the runtime lifecycle — NOT part of the grant digest.
	Status LifecycleStatus
}

// Digest returns the deterministic grant digest — same discipline as the
// contract digest: terms only, runtime Status excluded.
func (g AuthorityGrant) Digest() string {
	return HashCanonical(contractTerms(map[string]any{
		"grant_id":   g.GrantID,
		"intent_id":  g.IntentID,
		"issuer":     g.IssuerPrincipalID,
		"subject":    g.SubjectPrincipalID,
		"parent":     g.ParentGrantID,
		"operations": sortedSet(g.Operations),
		"resources":  sortedSet(g.ResourceScope),
		"budgets":    budgetTerms(g.Budgets),
		"valid_from": timeTerm(g.ValidFrom),
		"expires_at": timeTerm(g.ExpiresAt),
		"depth":      g.DelegationDepthRemaining,
	}))
}

// sortedSet renders a string slice as a SET: sorted, so serialization
// order never changes a digest.
func sortedSet(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

// budgetTerms renders budgets deterministically (maps marshal with
// sorted keys via encoding/json).
func budgetTerms(b Budgets) map[string]any {
	return map[string]any{
		"max_actions":   b.MaxActions,
		"per_operation": b.MaxActionsPerOperation,
	}
}

// timeTerm renders a time as its RFC3339Nano UTC form ("" for zero).
func timeTerm(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// contractTerms marshals a terms map to the canonical raw form the
// digest consumes. Marshal of string-keyed maps is key-sorted, and
// HashCanonical re-canonicalizes defensively.
func contractTerms(terms map[string]any) string {
	raw, err := json.Marshal(terms)
	if err != nil {
		// Unreachable for the plain types above; kept for honesty.
		return "unmarshalable"
	}
	return string(raw)
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The attenuation validator (Trust Layer Etapa 2, lote 2, spec FR-AUTH-2,
// §14.3): a delegation edge is valid ONLY if the child grant is a subset
// of its parent in EVERY dimension. Pure and deterministic — no clock, no
// store, no I/O — so the property tests and the fuzzer can drive it
// against an independent oracle. Per-operation budget attenuation is
// deliberately deferred to the budget-enforcement piece, where its
// semantics relative to the child's operation set are defined; v1
// attenuates MaxActions.
package action

import (
	"errors"
	"fmt"
)

// ErrAttenuationViolated is the sentinel every rejected delegation wraps;
// the wrapping error NAMES the widened dimension.
var ErrAttenuationViolated = errors.New("action: delegation widens authority")

// widens builds the named rejection for one dimension.
func widens(dimension, detail string) error {
	return fmt.Errorf("%w: %s (%s)", ErrAttenuationViolated, dimension, detail)
}

// coveredBy reports whether one child item is covered by a parent SET:
// an exact member covers itself, and a parent wildcard "*" covers
// everything — including a child "*", which therefore requires a parent
// "*" to be valid.
func coveredBy(item string, parentSet []string) bool {
	for _, s := range parentSet {
		if s == item || s == "*" {
			return true
		}
	}
	return false
}

// ValidateAttenuation validates one delegation edge (§14.3): nil when the
// child attenuates its parent in every dimension, the wrapped sentinel
// naming the FIRST widened dimension otherwise. Linkage is part of the
// edge: the child must live in the parent's intent, name the parent as
// its delegation parent, and be issued BY the parent's subject.
func ValidateAttenuation(parent, child AuthorityGrant) error {
	if child.IntentID != parent.IntentID {
		return widens("intent", "child leaves the parent's intent")
	}
	if child.ParentGrantID != parent.GrantID {
		return widens("parent", "child does not name this grant as its delegation parent")
	}
	if child.IssuerPrincipalID != parent.SubjectPrincipalID {
		return widens("issuer", "only the parent's subject may issue the child")
	}
	if parent.DelegationDepthRemaining <= 0 {
		return widens("depth", "parent has no delegation depth remaining")
	}
	if child.DelegationDepthRemaining >= parent.DelegationDepthRemaining {
		return widens("depth", "child depth must strictly decrease")
	}
	for _, op := range child.Operations {
		if !coveredBy(op, parent.Operations) {
			return widens("operations", "operation "+op+" is not covered by the parent")
		}
	}
	for _, r := range child.ResourceScope {
		if !coveredBy(r, parent.ResourceScope) {
			return widens("resources", "resource "+r+" is not covered by the parent")
		}
	}
	// Budgets: zero = UNLIMITED, so under a limited parent a zero child
	// would widen — the child must carry a limit of its own, within it.
	if parent.Budgets.MaxActions != 0 &&
		(child.Budgets.MaxActions == 0 || child.Budgets.MaxActions > parent.Budgets.MaxActions) {
		return widens("budget", "child max_actions must be limited within the parent's")
	}
	// Expiry: zero = NEVER EXPIRES, same widening shape as budgets.
	if !parent.ExpiresAt.IsZero() &&
		(child.ExpiresAt.IsZero() || child.ExpiresAt.After(parent.ExpiresAt)) {
		return widens("expiry", "child must expire no later than the parent")
	}
	if child.ValidFrom.Before(parent.ValidFrom) {
		return widens("window", "child validity must not start before the parent's")
	}
	return nil
}

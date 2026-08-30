// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Validity validators (Trust Layer Etapa 2, lote 2, spec FR-INT-3 and
// FR-AUTH-3): expired and revoked authority fails CLOSED with the sealed
// stable rules. Pure functions over an INJECTED instant — no wall clock
// in the domain — so expiry is testable to the millisecond and the
// wiring (lote 4) evaluates every request at ONE consistent `at`.
package action

import "time"

// The sealed stable rules (spec FR-ENV-3) plus authority_inactive, the
// honest addendum: a grant that is not yet in force (DRAFT, unknown
// status, or before its window) is neither expired nor revoked, and
// labeling it either would lie to the audit trail.
const (
	// RuleIntentInactive denies under an intent not currently in force.
	RuleIntentInactive = "intent_inactive"
	// RuleIntentExpired denies under an intent past its window.
	RuleIntentExpired = "intent_expired"
	// RuleAuthorityExpired denies under a grant past its window.
	RuleAuthorityExpired = "authority_expired"
	// RuleAuthorityRevoked denies under a grant a human withdrew.
	RuleAuthorityRevoked = "authority_revoked"
	// RuleAuthorityInactive denies under a grant not yet in force.
	RuleAuthorityInactive = "authority_inactive"
	// RuleBudgetExhausted denies when a budget limit is reached.
	RuleBudgetExhausted = "budget_exhausted"
	// RulePrincipalDisabled denies for a disabled principal.
	RulePrincipalDisabled = "principal_disabled"
)

// ValidateIntentAt reports whether an intent authorizes at the injected
// instant: "" when it does, the stable finite rule when it does not.
// Fail-closed: only LifecycleActive can authorize, the window is
// half-open ([ValidFrom, ExpiresAt)), and the CLOCK beats a stale ACTIVE
// status — a zero ExpiresAt means no expiry.
func ValidateIntentAt(c IntentContract, at time.Time) string {
	if c.Status != LifecycleActive {
		return RuleIntentInactive
	}
	if at.Before(c.ValidFrom) {
		return RuleIntentInactive
	}
	if !c.ExpiresAt.IsZero() && !at.Before(c.ExpiresAt) {
		return RuleIntentExpired
	}
	return ""
}

// ValidateGrantAt reports whether a grant authorizes at the injected
// instant — same clock discipline as intents, with the grant rules.
// Revocation OUTRANKS expiry: the human's withdrawal is the record even
// outside the window. DRAFT and unknown statuses are inactive, not
// expired or revoked.
func ValidateGrantAt(g AuthorityGrant, at time.Time) string {
	switch g.Status {
	case LifecycleRevoked:
		return RuleAuthorityRevoked
	case LifecycleExpired:
		return RuleAuthorityExpired
	case LifecycleActive:
		// In force; the window decides below.
	default:
		return RuleAuthorityInactive
	}
	if at.Before(g.ValidFrom) {
		return RuleAuthorityInactive
	}
	if !g.ExpiresAt.IsZero() && !at.Before(g.ExpiresAt) {
		return RuleAuthorityExpired
	}
	return ""
}

// ValidatePrincipalAt reports whether a principal may act at the injected
// instant: "" when enabled, principal_disabled from the disable instant
// on (a zero DisabledAt means never disabled).
func ValidatePrincipalAt(p Principal, at time.Time) string {
	if !p.DisabledAt.IsZero() && !at.Before(p.DisabledAt) {
		return RulePrincipalDisabled
	}
	return ""
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Validity contract — Etapa 2, lote 2, pieza 2 (spec FR-INT-3/FR-AUTH-3):
// expired and revoked authority fails CLOSED with the sealed stable rules
// (intent_inactive, intent_expired, authority_expired, authority_revoked,
// principal_disabled — plus authority_inactive, the honest addendum for a
// grant that is not yet in force). The clock is INJECTED: expiry is
// testable to the millisecond. Approved-red contract.

package action

import (
	"testing"
	"time"
)

var valBase = time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

func activeIntent() IntentContract {
	c := testIntent()
	c.ValidFrom = valBase
	c.ExpiresAt = valBase.Add(time.Hour)
	return c
}

func activeGrant() AuthorityGrant {
	g := parentGrant()
	g.ValidFrom = valBase
	g.ExpiresAt = valBase.Add(time.Hour)
	return g
}

func TestValidateIntentAt_tableWithMillisecondEdges(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(*IntentContract)
		at     time.Time
		want   string
	}{
		{"active inside window", nil, valBase.Add(time.Minute), ""},
		{"one millisecond before expiry", nil, valBase.Add(time.Hour - time.Millisecond), ""},
		{"the expiry instant itself", nil, valBase.Add(time.Hour), RuleIntentExpired},
		{"one millisecond after expiry", nil, valBase.Add(time.Hour + time.Millisecond), RuleIntentExpired},
		{"the valid_from instant is in force", nil, valBase, ""},
		{"one millisecond before valid_from", nil, valBase.Add(-time.Millisecond), RuleIntentInactive},
		{"no expiry never expires", func(c *IntentContract) { c.ExpiresAt = time.Time{} },
			valBase.Add(1000000 * time.Hour), ""},
		{"draft is inactive", func(c *IntentContract) { c.Status = LifecycleDraft },
			valBase.Add(time.Minute), RuleIntentInactive},
		{"revoked is inactive", func(c *IntentContract) { c.Status = LifecycleRevoked },
			valBase.Add(time.Minute), RuleIntentInactive},
		{"expired status is inactive", func(c *IntentContract) { c.Status = LifecycleExpired },
			valBase.Add(time.Minute), RuleIntentInactive},
		{"unknown status fails closed", func(c *IntentContract) { c.Status = LifecycleStatus("BOGUS") },
			valBase.Add(time.Minute), RuleIntentInactive},
		{"clock beats a stale ACTIVE status", nil, valBase.Add(2 * time.Hour), RuleIntentExpired},
	}
	for _, tc := range cases {
		intent := activeIntent()
		if tc.mutate != nil {
			tc.mutate(&intent)
		}
		if got := ValidateIntentAt(intent, tc.at); got != tc.want {
			t.Fatalf("%s: rule = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestValidateGrantAt_tableWithMillisecondEdges(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(*AuthorityGrant)
		at     time.Time
		want   string
	}{
		{"active inside window", nil, valBase.Add(time.Minute), ""},
		{"one millisecond before expiry", nil, valBase.Add(time.Hour - time.Millisecond), ""},
		{"the expiry instant itself", nil, valBase.Add(time.Hour), RuleAuthorityExpired},
		{"revoked", func(g *AuthorityGrant) { g.Status = LifecycleRevoked },
			valBase.Add(time.Minute), RuleAuthorityRevoked},
		{"expired status", func(g *AuthorityGrant) { g.Status = LifecycleExpired },
			valBase.Add(time.Minute), RuleAuthorityExpired},
		{"draft is not in force", func(g *AuthorityGrant) { g.Status = LifecycleDraft },
			valBase.Add(time.Minute), RuleAuthorityInactive},
		{"unknown status fails closed", func(g *AuthorityGrant) { g.Status = LifecycleStatus("BOGUS") },
			valBase.Add(time.Minute), RuleAuthorityInactive},
		{"before valid_from", nil, valBase.Add(-time.Millisecond), RuleAuthorityInactive},
		{"no expiry never expires", func(g *AuthorityGrant) { g.ExpiresAt = time.Time{} },
			valBase.Add(1000000 * time.Hour), ""},
		{"clock beats a stale ACTIVE status", nil, valBase.Add(2 * time.Hour), RuleAuthorityExpired},
		// Revocation OUTRANKS expiry: a revoked grant reports revocation
		// even outside its window — the human's withdrawal is the record.
		{"revoked outranks expired", func(g *AuthorityGrant) { g.Status = LifecycleRevoked },
			valBase.Add(2 * time.Hour), RuleAuthorityRevoked},
	}
	for _, tc := range cases {
		grant := activeGrant()
		if tc.mutate != nil {
			tc.mutate(&grant)
		}
		if got := ValidateGrantAt(grant, tc.at); got != tc.want {
			t.Fatalf("%s: rule = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestValidatePrincipalAt_disabledFailsClosed(t *testing.T) {
	t.Parallel()
	p := OperatorPrincipal()
	if got := ValidatePrincipalAt(p, valBase); got != "" {
		t.Fatalf("an enabled principal authorizes, got %q", got)
	}
	p.DisabledAt = valBase
	if got := ValidatePrincipalAt(p, valBase); got != RulePrincipalDisabled {
		t.Fatalf("the disable instant itself already denies, got %q", got)
	}
	if got := ValidatePrincipalAt(p, valBase.Add(-time.Millisecond)); got != "" {
		t.Fatalf("one millisecond before the disable the principal still authorizes, got %q", got)
	}
}

func TestValidityRules_areTheSealedFiniteSet(t *testing.T) {
	t.Parallel()
	want := map[string]string{
		RuleIntentInactive:    "intent_inactive",
		RuleIntentExpired:     "intent_expired",
		RuleAuthorityExpired:  "authority_expired",
		RuleAuthorityRevoked:  "authority_revoked",
		RuleAuthorityInactive: "authority_inactive",
		RuleBudgetExhausted:   "budget_exhausted",
		RulePrincipalDisabled: "principal_disabled",
	}
	for constant, literal := range want {
		if constant != literal {
			t.Fatalf("rule constant %q must equal its sealed literal %q", constant, literal)
		}
	}
}

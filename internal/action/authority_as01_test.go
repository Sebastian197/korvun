// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package action

import (
	"errors"
	"testing"
)

// TestAuthority_EveryDimensionRejectsWidening (AS-AUTH-01) widens ONE dimension
// of an otherwise valid child per row and demands the attenuation error that
// NAMES that dimension — never merely "an error", because a widening caught by
// the wrong check would mean the right one is dead.
//
// The table has one row per dimension NormalizeAuthorityDelegation judges,
// `parent` included: a child that names another parent is the widening that
// detaches every other comparison from the grant it should be compared with.
//
// Evidence level: unit, in-process, pure domain — no store. The door that
// persists a child is proved to CALL this normalizer by
// TestAuthority_DelegateDoorInvokesFullAttenuation.
// Probing mutations executed: ONE PER ROW — each dimension's own check
// neutralized alone, its row run, its red captured; twenty-one mutations,
// twenty-one reds, recorded in the phase's mutations.txt.
func TestAuthority_EveryDimensionRejectsWidening(t *testing.T) {
	parent, intent := authorityTestParent(), authorityTestIntent()
	tests := []struct {
		dimension string
		mutate    func(*AuthorityGrantV2)
	}{
		{"profile", func(c *AuthorityGrantV2) { c.ProfileID = "other" }},
		{"intent", func(c *AuthorityGrantV2) { c.IntentID = "other" }},
		{"intent_version", func(c *AuthorityGrantV2) { c.IntentVersion++ }},
		{"intent_digest", func(c *AuthorityGrantV2) { c.IntentDigest = "sha256:other" }},
		{"parent", func(c *AuthorityGrantV2) { c.ParentGrantID = "grant_some_other_parent" }},
		{"issuer", func(c *AuthorityGrantV2) { c.IssuerPrincipalID = "attacker" }},
		{"operations", func(c *AuthorityGrantV2) { c.Operations = append(c.Operations, OperationRef{"tool", "read_file", 1}) }},
		{"channels", func(c *AuthorityGrantV2) { c.Channels = append(c.Channels, "discord") }},
		{"allowed_resources", func(c *AuthorityGrantV2) {
			c.AllowedResources = []ResourceRef{{Kind: "url", ID: "https://elsewhere.example/"}}
		}},
		{"denied_resources", func(c *AuthorityGrantV2) { c.DeniedResources = nil }},
		{"allowed_data", func(c *AuthorityGrantV2) { c.AllowedData = append(c.AllowedData, "secret") }},
		{"denied_data", func(c *AuthorityGrantV2) { c.DeniedData = nil }},
		{"destinations", func(c *AuthorityGrantV2) { c.OutputDestinations = append(c.OutputDestinations, "elsewhere.example") }},
		{"effect_classes", func(c *AuthorityGrantV2) { c.EffectClasses = append(c.EffectClasses, EffectCritical) }},
		{"effect_ceiling", func(c *AuthorityGrantV2) { c.EffectCeiling = EffectCritical }},
		{"budget_total_remaining", func(c *AuthorityGrantV2) { c.Budget.Total = authorityTestInt64(7) }},
		{"budget_operation_remaining", func(c *AuthorityGrantV2) { c.Budget.PerOperation["tool/webhook_call@1"] = 3 }},
		{"valid_from", func(c *AuthorityGrantV2) { c.ValidFrom = parent.ValidFrom.Add(-1) }},
		{"expires_at", func(c *AuthorityGrantV2) { c.ExpiresAt = parent.ExpiresAt.Add(1) }},
		{"depth", func(c *AuthorityGrantV2) { c.DelegationDepthRemaining = parent.DelegationDepthRemaining }},
		{"approval", func(c *AuthorityGrantV2) { c.Approval.Required = false }},
	}
	for _, tc := range tests {
		t.Run(tc.dimension, func(t *testing.T) {
			child := authorityTestChild()
			tc.mutate(&child)
			_, err := NormalizeAuthorityDelegation(parent, child, intent, authorityTestRemaining(), authorityTestMatchers())
			var atten *AttenuationError
			if !errors.Is(err, ErrAttenuationViolated) || !errors.As(err, &atten) || atten.Dimension != tc.dimension {
				t.Fatalf("error = %#v, want attenuation dimension %q", err, tc.dimension)
			}
		})
	}
}

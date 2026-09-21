// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package action

import (
	"net/url"
	"path"
	"slices"
	"strings"
	"testing"
)

// TestAuthority_PropertySubsetAgainstFiniteModel (AS-AUTH-02) enumerates small
// universes EXHAUSTIVELY and demands that the production normalizer never
// disagrees with an oracle written apart from it. The oracle calls none of
// production's subset, matching or RANKING code — it carries its own effect
// ladder — because an oracle that shares the judged code shares its bugs.
//
// Three families of rows exist because their mutations SURVIVED the first
// version of this mould, and a mutation that survives is the finding:
// an ABSENT child total under a finite parent balance, an ABSENT child effect
// ceiling under a parent that has one, and a prohibition that only the INTENT
// declares. In each, "absent" must mean nothing was granted — never unlimited.
//
// Evidence level: unit, in-process, pure domain — no store.
// Probing mutations executed, each alone: subset replaced by overlap; inherited
// denied resources omitted; inherited denied data omitted; the intent's own
// denials dropped from the inherited set; an absent total read as unlimited;
// an absent effect ceiling read as unlimited. Six mutations, six reds.
func TestAuthority_PropertySubsetAgainstFiniteModel(t *testing.T) {
	parent, intent, remaining := authorityTestParent(), authorityTestIntent(), authorityTestRemaining()
	cases := []struct {
		name   string
		mutate func(*AuthorityGrantV2)
	}{
		{"baseline", func(*AuthorityGrantV2) {}},
		{"profile", func(c *AuthorityGrantV2) { c.ProfileID = "other" }},
		{"intent", func(c *AuthorityGrantV2) { c.IntentID = "other" }},
		{"intent version", func(c *AuthorityGrantV2) { c.IntentVersion++ }},
		{"intent digest", func(c *AuthorityGrantV2) { c.IntentDigest = "sha256:other" }},
		{"parent", func(c *AuthorityGrantV2) { c.ParentGrantID = "other" }},
		{"issuer", func(c *AuthorityGrantV2) { c.IssuerPrincipalID = "other" }},
		{"operation", func(c *AuthorityGrantV2) {
			c.Operations = append(c.Operations, OperationRef{Namespace: "tool", Name: "other", Version: 1})
		}},
		{"channel", func(c *AuthorityGrantV2) { c.Channels = append(c.Channels, "discord") }},
		{"allowed resource", func(c *AuthorityGrantV2) {
			c.AllowedResources = []ResourceRef{{Kind: "url", ID: "https://elsewhere.example/"}}
		}},
		{"denied resource", func(c *AuthorityGrantV2) { c.DeniedResources = nil }},
		{"allowed data", func(c *AuthorityGrantV2) { c.AllowedData = append(c.AllowedData, "secret") }},
		{"denied data", func(c *AuthorityGrantV2) { c.DeniedData = nil }},
		{"destination", func(c *AuthorityGrantV2) { c.OutputDestinations = append(c.OutputDestinations, "other.example") }},
		{"effect class", func(c *AuthorityGrantV2) { c.EffectClasses = append(c.EffectClasses, EffectCritical) }},
		{"effect ceiling", func(c *AuthorityGrantV2) { c.EffectCeiling = EffectCritical }},
		{"total budget", func(c *AuthorityGrantV2) { c.Budget.Total = authorityTestInt64(7) }},
		{"operation budget", func(c *AuthorityGrantV2) { c.Budget.PerOperation["tool/webhook_call@1"] = 3 }},
		{"valid from", func(c *AuthorityGrantV2) { c.ValidFrom = parent.ValidFrom.Add(-1) }},
		{"expires at", func(c *AuthorityGrantV2) { c.ExpiresAt = parent.ExpiresAt.Add(1) }},
		{"depth", func(c *AuthorityGrantV2) { c.DelegationDepthRemaining = parent.DelegationDepthRemaining }},
		{"approval", func(c *AuthorityGrantV2) { c.Approval.Required = false }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			child := authorityTestChild()
			tc.mutate(&child)
			want := independentAuthorityOracle(parent, child, intent, remaining)
			_, err := NormalizeAuthorityDelegation(parent, child, intent, remaining,
				ResourceMatchers{"url": independentURLContains})
			if got := err == nil; got != want {
				t.Fatalf("accepted=%t independent oracle=%t error=%v", got, want, err)
			}
		})
	}

	universe := []string{"console", "telegram", "discord"}
	for parentMask := 0; parentMask < 8; parentMask++ {
		for childMask := 0; childMask < 8; childMask++ {
			p, c := authorityTestParent(), authorityTestChild()
			p.Channels, c.Channels = finiteStrings(universe, parentMask), finiteStrings(universe, childMask)
			want := independentAuthorityOracle(p, c, intent, remaining)
			_, err := NormalizeAuthorityDelegation(p, c, intent, remaining,
				ResourceMatchers{"url": independentURLContains})
			if got := err == nil; got != want {
				t.Fatalf("channel masks parent=%03b child=%03b accepted=%t oracle=%t error=%v",
					parentMask, childMask, got, want, err)
			}
		}
	}

	operationUniverse := []OperationRef{
		{Namespace: "tool", Name: "webhook_call", Version: 1},
		{Namespace: "tool", Name: "read_file", Version: 1},
	}
	for parentMask := 1; parentMask < 4; parentMask++ {
		for childMask := 1; childMask < 4; childMask++ {
			p, c, i := authorityTestParent(), authorityTestChild(), authorityTestIntent()
			i.Operations = finiteOperations(operationUniverse, 3)
			p.Operations = finiteOperations(operationUniverse, parentMask)
			p.IntentDigest = i.Digest()
			c.Operations = finiteOperations(operationUniverse, childMask)
			c.IntentDigest = i.Digest()
			assertIndependentAuthority(t, "operation masks", p, c, i, remaining)
		}
	}

	stringDimensions := []struct {
		name   string
		apply  func(*AuthorityGrantV2, *AuthorityGrantV2, *IntentContractV2, []string, []string)
		values []string
	}{
		{"data", func(p, c *AuthorityGrantV2, i *IntentContractV2, pv, cv []string) {
			i.DataScope, p.AllowedData, c.AllowedData = []string{"public", "invoice"}, pv, cv
		}, []string{"public", "invoice"}},
		{"destinations", func(p, c *AuthorityGrantV2, i *IntentContractV2, pv, cv []string) {
			i.OutputDestinations = []string{"a.example", "b.example"}
			p.OutputDestinations, c.OutputDestinations = pv, cv
		}, []string{"a.example", "b.example"}},
	}
	for _, dimension := range stringDimensions {
		for parentMask := 1; parentMask < 4; parentMask++ {
			for childMask := 1; childMask < 4; childMask++ {
				p, c, i := authorityTestParent(), authorityTestChild(), authorityTestIntent()
				dimension.apply(&p, &c, &i, finiteStrings(dimension.values, parentMask),
					finiteStrings(dimension.values, childMask))
				p.IntentDigest, c.IntentDigest = i.Digest(), i.Digest()
				assertIndependentAuthority(t, dimension.name+" masks", p, c, i, remaining)
			}
		}
	}

	resourceUniverse := []ResourceRef{
		{Kind: "url", ID: "https://pay.example/api/a"},
		{Kind: "url", ID: "https://pay.example/api/b"},
	}
	for parentMask := 1; parentMask < 4; parentMask++ {
		for childMask := 1; childMask < 4; childMask++ {
			p, c, i := authorityTestParent(), authorityTestChild(), authorityTestIntent()
			i.AllowedResources = []ResourceRef{{Kind: "url", ID: "https://pay.example/api/"}}
			i.DeniedResources, p.DeniedResources, c.DeniedResources = nil, nil, nil
			p.AllowedResources = finiteResources(resourceUniverse, parentMask)
			c.AllowedResources = finiteResources(resourceUniverse, childMask)
			p.IntentDigest, c.IntentDigest = i.Digest(), i.Digest()
			assertIndependentAuthority(t, "resource masks", p, c, i, remaining)
		}
	}

	effectUniverse := []EffectClass{EffectWriteReversible, EffectWriteCompensatable}
	for parentMask := 1; parentMask < 4; parentMask++ {
		for childMask := 1; childMask < 4; childMask++ {
			p, c, i := authorityTestParent(), authorityTestChild(), authorityTestIntent()
			i.EffectClasses = append([]EffectClass(nil), effectUniverse...)
			p.EffectClasses = finiteEffects(effectUniverse, parentMask)
			c.EffectClasses = finiteEffects(effectUniverse, childMask)
			p.EffectCeiling, c.EffectCeiling = EffectWriteCompensatable, EffectWriteCompensatable
			p.IntentDigest, c.IntentDigest = i.Digest(), i.Digest()
			assertIndependentAuthority(t, "effect masks", p, c, i, remaining)
		}
	}

	// ABSENT IS NOT UNLIMITED — total budget. Every pairing of a parent balance
	// that is unlimited or finite with a child total that is absent, inside, or
	// above it. The row that matters is (finite, absent): a child that omits
	// its total under a finite parent must be refused, not read as boundless.
	for _, parentRemaining := range []*int64{nil, authorityTestInt64(5)} {
		for _, childTotal := range []*int64{nil, authorityTestInt64(3), authorityTestInt64(7)} {
			p, c, i := authorityTestParent(), authorityTestChild(), authorityTestIntent()
			c.Budget.Total = childTotal
			r := AuthorityBudgetRemaining{Total: parentRemaining, PerOperation: remaining.PerOperation}
			assertIndependentAuthority(t, "absent-or-finite total budget", p, c, i, r)
		}
	}

	// ABSENT IS NOT UNLIMITED — effect ceiling. Every pairing over the whole
	// ladder plus "absent" on both sides.
	ceilings := []EffectClass{"", EffectPure, EffectReadExternal, EffectWriteReversible,
		EffectWriteCompensatable, EffectWriteIrreversible, EffectCritical}
	for _, parentCeiling := range ceilings {
		for _, childCeiling := range ceilings {
			p, c, i := authorityTestParent(), authorityTestChild(), authorityTestIntent()
			p.EffectCeiling, c.EffectCeiling = parentCeiling, childCeiling
			assertIndependentAuthority(t, "absent-or-ranked effect ceiling", p, c, i, remaining)
		}
	}

	// A PROHIBITION ONLY THE INTENT DECLARES. The parent does not repeat it, so
	// a normalizer that inherits denials from the parent alone lets the child
	// shed it. Each subset of {intent denial, parent denial} against each
	// subset the child keeps.
	intentDenial := ResourceRef{Kind: "url", ID: "https://pay.example/api/forbidden-by-intent"}
	parentDenial := ResourceRef{Kind: "url", ID: "https://pay.example/api/forbidden-by-parent"}
	for inheritedMask := 0; inheritedMask < 4; inheritedMask++ {
		for keptMask := 0; keptMask < 4; keptMask++ {
			p, c, i := authorityTestParent(), authorityTestChild(), authorityTestIntent()
			i.DeniedResources, p.DeniedResources, c.DeniedResources = nil, nil, nil
			if inheritedMask&1 != 0 {
				i.DeniedResources = []ResourceRef{intentDenial}
			}
			if inheritedMask&2 != 0 {
				p.DeniedResources = []ResourceRef{parentDenial}
			}
			c.DeniedResources = finiteResources([]ResourceRef{intentDenial, parentDenial}, keptMask)
			p.IntentDigest, c.IntentDigest = i.Digest(), i.Digest()
			assertIndependentAuthority(t, "denial declared by intent or parent", p, c, i, remaining)
		}
	}

	chainUniverse := []string{"console", "telegram"}
	for rootMask := 1; rootMask < 4; rootMask++ {
		for childMask := 1; childMask < 4; childMask++ {
			for leafMask := 1; leafMask < 4; leafMask++ {
				root, child, intent := authorityTestParent(), authorityTestChild(), authorityTestIntent()
				root.Channels, child.Channels = finiteStrings(chainUniverse, rootMask),
					finiteStrings(chainUniverse, childMask)
				leaf := child
				leaf.GrantID, leaf.ParentGrantID = "grant_leaf", child.GrantID
				leaf.ParentGrantVersion, leaf.IssuerPrincipalID = child.Version, child.SubjectPrincipalID
				leaf.Channels = finiteStrings(chainUniverse, leafMask)
				leaf.DelegationDepthRemaining--
				first := independentAuthorityOracle(root, child, intent, remaining)
				_, firstErr := NormalizeAuthorityDelegation(root, child, intent, remaining,
					ResourceMatchers{"url": independentURLContains})
				second := independentAuthorityOracle(child, leaf, intent, remaining)
				_, secondErr := NormalizeAuthorityDelegation(child, leaf, intent, remaining,
					ResourceMatchers{"url": independentURLContains})
				if (firstErr == nil) != first || (secondErr == nil) != second {
					t.Fatalf("chain masks root=%02b child=%02b leaf=%02b oracle=%t/%t errors=%v/%v",
						rootMask, childMask, leafMask, first, second, firstErr, secondErr)
				}
			}
		}
	}
}

func assertIndependentAuthority(t *testing.T, label string, parent, child AuthorityGrantV2,
	intent IntentContractV2, remaining AuthorityBudgetRemaining) {
	t.Helper()
	want := independentAuthorityOracle(parent, child, intent, remaining)
	_, err := NormalizeAuthorityDelegation(parent, child, intent, remaining,
		ResourceMatchers{"url": independentURLContains})
	if (err == nil) != want {
		t.Fatalf("%s accepted=%t oracle=%t error=%v", label, err == nil, want, err)
	}
}

func finiteOperations(universe []OperationRef, mask int) []OperationRef {
	var values []OperationRef
	for i, value := range universe {
		if mask&(1<<i) != 0 {
			values = append(values, value)
		}
	}
	return values
}

func finiteResources(universe []ResourceRef, mask int) []ResourceRef {
	var values []ResourceRef
	for i, value := range universe {
		if mask&(1<<i) != 0 {
			values = append(values, value)
		}
	}
	return values
}

func finiteEffects(universe []EffectClass, mask int) []EffectClass {
	var values []EffectClass
	for i, value := range universe {
		if mask&(1<<i) != 0 {
			values = append(values, value)
		}
	}
	return values
}

func finiteStrings(universe []string, mask int) []string {
	var values []string
	for i, value := range universe {
		if mask&(1<<i) != 0 {
			values = append(values, value)
		}
	}
	return values
}

func independentAuthorityOracle(parent, child AuthorityGrantV2, intent IntentContractV2,
	remaining AuthorityBudgetRemaining) bool {
	if len(parent.Operations) == 0 || len(parent.Channels) == 0 ||
		len(parent.EffectClasses) == 0 || len(child.Operations) == 0 ||
		len(child.Channels) == 0 || len(child.EffectClasses) == 0 {
		return false
	}
	if child.ProfileID != parent.ProfileID || child.ProfileID != intent.ProfileID ||
		child.IntentID != parent.IntentID || child.IntentID != intent.IntentID ||
		child.IntentVersion != parent.IntentVersion || child.IntentVersion != intent.Version ||
		child.IntentDigest != parent.IntentDigest || child.IntentDigest != intent.Digest() ||
		child.ParentGrantID != parent.GrantID || child.ParentGrantVersion != parent.Version ||
		child.IssuerPrincipalID != parent.SubjectPrincipalID {
		return false
	}
	operationSet := func(children, parents []OperationRef) bool {
		for _, child := range children {
			if !slices.Contains(parents, child) {
				return false
			}
		}
		return true
	}
	stringSet := func(children, parents []string) bool {
		for _, child := range children {
			if !slices.Contains(parents, child) && !slices.Contains(parents, "*") {
				return false
			}
		}
		return true
	}
	resourceSet := func(children, parents []ResourceRef) bool {
		for _, child := range children {
			covered := false
			for _, parent := range parents {
				if child.Kind == parent.Kind && child.Kind == "url" &&
					independentURLContains(parent.ID, child.ID) {
					covered = true
				}
			}
			if !covered {
				return false
			}
		}
		return true
	}
	if !operationSet(child.Operations, parent.Operations) ||
		!operationSet(child.Operations, intent.Operations) ||
		!stringSet(child.Channels, parent.Channels) ||
		!resourceSet(child.AllowedResources, parent.AllowedResources) ||
		!resourceSet(child.AllowedResources, intent.AllowedResources) ||
		!resourceSet(append(parent.DeniedResources, intent.DeniedResources...), child.DeniedResources) ||
		!stringSet(child.AllowedData, parent.AllowedData) ||
		!stringSet(child.AllowedData, intent.DataScope) ||
		!stringSet(parent.DeniedData, child.DeniedData) ||
		!stringSet(child.OutputDestinations, parent.OutputDestinations) ||
		!stringSet(child.OutputDestinations, intent.OutputDestinations) ||
		!independentEffectsSubset(child.EffectClasses, parent.EffectClasses) ||
		!independentEffectsSubset(child.EffectClasses, intent.EffectClasses) {
		return false
	}
	if parent.EffectCeiling != "" &&
		(child.EffectCeiling == "" || independentEffectRank(child.EffectCeiling) > independentEffectRank(parent.EffectCeiling)) {
		return false
	}
	if remaining.Total != nil &&
		(child.Budget.Total == nil || *child.Budget.Total > *remaining.Total) {
		return false
	}
	for operation, maximum := range remaining.PerOperation {
		if childMaximum, ok := child.Budget.PerOperation[operation]; ok && childMaximum > maximum {
			return false
		}
	}
	return !child.ValidFrom.Before(parent.ValidFrom) && !child.ValidFrom.Before(intent.ValidFrom) &&
		!child.ExpiresAt.After(parent.ExpiresAt) && !child.ExpiresAt.After(intent.ExpiresAt) &&
		parent.DelegationDepthRemaining > 0 && child.DelegationDepthRemaining >= 0 &&
		child.DelegationDepthRemaining < parent.DelegationDepthRemaining &&
		child.DelegationDepthRemaining <= intent.MaxDelegationDepth &&
		(!parent.Approval.Required && !intent.Approval.Required || child.Approval.Required)
}

// independentEffectRank is the oracle's OWN ladder, spelled with string
// literals rather than production's constants or its Rank method: the spec
// forbids the oracle from calling production ranking, and a ladder copied from
// the judged code would inherit a mis-ordered rung silently.
func independentEffectRank(class EffectClass) int {
	switch string(class) {
	case "pure":
		return 0
	case "read_external":
		return 1
	case "write_reversible":
		return 2
	case "write_compensatable":
		return 3
	case "write_irreversible":
		return 4
	case "critical":
		return 5
	default:
		return 1 << 20
	}
}

func independentEffectsSubset(children, parents []EffectClass) bool {
	for _, child := range children {
		if !slices.Contains(parents, child) {
			return false
		}
	}
	return true
}

func independentURLContains(parent, child string) bool {
	p, err := url.Parse(parent)
	if err != nil {
		return false
	}
	c, err := url.Parse(child)
	if err != nil || !strings.EqualFold(p.Scheme, c.Scheme) ||
		!strings.EqualFold(p.Host, c.Host) {
		return false
	}
	base := strings.TrimSuffix(path.Clean(p.Path), "/")
	want := path.Clean(c.Path)
	return want == base || strings.HasPrefix(want, base+"/")
}

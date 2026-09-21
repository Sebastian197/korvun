// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package action

import "time"

func authorityTestInt64(v int64) *int64 { return &v }

func authorityTestIntent() IntentContractV2 {
	return IntentContractV2{
		IntentID: "int_pay", SchemaVersion: 2, Version: 3, ProfileID: "profile_a",
		OwnerPrincipalID: "principal_operator", Purpose: "Pay approved suppliers",
		Operations:       []OperationRef{{Namespace: "tool", Name: "webhook_call", Version: 1}},
		AllowedResources: []ResourceRef{{Kind: "url", ID: "https://pay.example/api/"}},
		DeniedResources:  []ResourceRef{{Kind: "url", ID: "https://pay.example/api/admin"}},
		DataScope:        []string{"supplier_invoice", "public"}, OutputDestinations: []string{"pay.example"},
		EffectClasses: []EffectClass{EffectWriteReversible, EffectWriteCompensatable},
		Budget:        IntentBudgetV2{Total: authorityTestInt64(10), PerOperation: map[string]int64{"tool/webhook_call@1": 4}},
		ValidFrom:     time.Date(2026, 9, 21, 8, 0, 0, 0, time.UTC), ExpiresAt: time.Date(2026, 9, 22, 8, 0, 0, 0, time.UTC),
		Approval: ApprovalRequirementV2{Required: true}, MaxDelegationDepth: 4,
	}
}

func authorityTestParent() AuthorityGrantV2 {
	i := authorityTestIntent()
	return AuthorityGrantV2{
		GrantID: "grant_parent", SchemaVersion: 2, Version: 1, ProfileID: i.ProfileID,
		IntentID: i.IntentID, IntentVersion: i.Version, IntentDigest: i.Digest(),
		IssuerPrincipalID: i.OwnerPrincipalID, SubjectPrincipalID: "principal_agent",
		Operations: append([]OperationRef(nil), i.Operations...), Channels: []string{"console", "telegram"},
		AllowedResources: append([]ResourceRef(nil), i.AllowedResources...),
		DeniedResources:  append([]ResourceRef(nil), i.DeniedResources...),
		AllowedData:      []string{"supplier_invoice", "public"}, DeniedData: []string{"secret"},
		OutputDestinations: []string{"pay.example"}, EffectClasses: append([]EffectClass(nil), i.EffectClasses...),
		EffectCeiling: EffectWriteCompensatable,
		Budget:        IntentBudgetV2{Total: authorityTestInt64(8), PerOperation: map[string]int64{"tool/webhook_call@1": 3}},
		ValidFrom:     i.ValidFrom.Add(time.Hour), ExpiresAt: i.ExpiresAt.Add(-time.Hour),
		DelegationDepthRemaining: 3, Approval: ApprovalRequirementV2{Required: true}, Status: LifecycleActive,
	}
}

func authorityTestChild() AuthorityGrantV2 {
	p := authorityTestParent()
	return AuthorityGrantV2{
		GrantID: "grant_child", SchemaVersion: 2, Version: 1, ProfileID: p.ProfileID,
		IntentID: p.IntentID, IntentVersion: p.IntentVersion, IntentDigest: p.IntentDigest,
		IssuerPrincipalID: p.SubjectPrincipalID, SubjectPrincipalID: "principal_worker",
		ParentGrantID: p.GrantID, ParentGrantVersion: p.Version,
		Operations: append([]OperationRef(nil), p.Operations...), Channels: []string{"telegram"},
		AllowedResources: []ResourceRef{{Kind: "url", ID: "https://pay.example/api/invoices"}},
		DeniedResources:  append([]ResourceRef(nil), p.DeniedResources...),
		AllowedData:      []string{"supplier_invoice"}, DeniedData: []string{"secret", "credential"},
		OutputDestinations: append([]string(nil), p.OutputDestinations...),
		EffectClasses:      []EffectClass{EffectWriteReversible}, EffectCeiling: EffectWriteReversible,
		Budget:    IntentBudgetV2{Total: authorityTestInt64(2), PerOperation: map[string]int64{"tool/webhook_call@1": 2}},
		ValidFrom: p.ValidFrom.Add(time.Minute), ExpiresAt: p.ExpiresAt.Add(-time.Minute),
		DelegationDepthRemaining: 2, Approval: ApprovalRequirementV2{Required: true}, Status: LifecycleActive,
	}
}

func authorityTestRemaining() AuthorityBudgetRemaining {
	return AuthorityBudgetRemaining{Total: authorityTestInt64(6), PerOperation: map[string]int64{"tool/webhook_call@1": 2}}
}

func authorityTestMatchers() ResourceMatchers {
	return ResourceMatchers{"url": func(parent, child string) bool { return len(child) >= len(parent) && child[:len(parent)] == parent }}
}

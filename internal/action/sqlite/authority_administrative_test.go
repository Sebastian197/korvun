// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/identity"
)

func TestAuthority_AdministrativeActsKeepOperatorSeparateFromGrantIssuer(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 8)
	ctx := context.Background()
	adminRoot := f.root
	adminRoot.GrantID = "grant_admin_root"
	adminRoot.IssuerPrincipalID = "principal_external_issuer"
	adminRoot.SubjectPrincipalID = "principal_admin_subject"
	reason := "operator recovers an externally issued grant"
	issueAct := authorityActorAct(t, f.store, f.resolver, f.issuer, "issue",
		CanonicalAdminAuthorityGrant(adminRoot, reason), f.now.Add(time.Second))
	if err := f.store.AdminIssueAuthority(ctx, adminRoot, issueAct, reason, f.now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	assertAdministrativeEvent(t, f.store, adminRoot.GrantID, adminRoot.IssuerPrincipalID,
		"principal_responsible", reason)

	child := adminRoot
	child.GrantID = "grant_admin_child"
	child.IssuerPrincipalID = adminRoot.SubjectPrincipalID
	child.SubjectPrincipalID = "principal_admin_leaf"
	child.ParentGrantID, child.ParentGrantVersion = adminRoot.GrantID, adminRoot.Version
	child.DelegationDepthRemaining--
	childMaximum := int64(3)
	child.Budget = action.IntentBudgetV2{
		Total: &childMaximum, PerOperation: map[string]int64{"tool/probe@1": childMaximum},
	}
	delegateReason := "operator delegates after incident review"
	delegateAct := authorityActorAct(t, f.store, f.resolver, f.issuer, "delegate",
		CanonicalAdminAuthorityGrant(child, delegateReason), f.now.Add(2*time.Second))
	if err := f.store.AdminDelegateAuthority(ctx, child, delegateAct, delegateReason, f.now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	assertAdministrativeEvent(t, f.store, child.GrantID, child.IssuerPrincipalID,
		"principal_responsible", delegateReason)

	revokeReason := "operator withdraws compromised delegation"
	revokeAct := authorityActorAct(t, f.store, f.resolver, f.issuer, "revoke",
		CanonicalAuthorityRevoke(child.GrantID, revokeReason), f.now.Add(3*time.Second))
	if err := f.store.AdminRevokeAuthority(ctx, child.GrantID, revokeAct, revokeReason, f.now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	var status string
	var administrative int
	if err := f.store.db.QueryRow(`SELECT status FROM grant_heads WHERE grant_id=?`, child.GrantID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(action.LifecycleRevoked) {
		t.Fatalf("status = %q", status)
	}
	if err := f.store.db.QueryRow(`SELECT administrative FROM grant_events WHERE grant_id=? ORDER BY revision DESC LIMIT 1`, child.GrantID).Scan(&administrative); err != nil {
		t.Fatal(err)
	}
	if administrative != 1 {
		t.Fatalf("administrative = %d", administrative)
	}
	if err := f.store.AdminRevokeAuthority(ctx, child.GrantID, revokeAct, revokeReason, f.now.Add(4*time.Second)); !errors.Is(err, identity.ErrIdentityBindingMismatch) {
		t.Fatalf("reused operator act error = %v", err)
	}

	if err := f.store.AdminIssueAuthority(ctx, adminRoot, "", "", f.now); !errors.Is(err, action.ErrAuthorityMalformed) {
		t.Fatalf("blank issue reason error = %v", err)
	}
	if err := f.store.AdminDelegateAuthority(ctx, child, "", "", f.now); !errors.Is(err, action.ErrAuthorityMalformed) {
		t.Fatalf("blank delegation reason error = %v", err)
	}
	if err := f.store.AdminRevokeAuthority(ctx, child.GrantID, "", "", f.now); !errors.Is(err, action.ErrAuthorityMalformed) {
		t.Fatalf("blank revocation reason error = %v", err)
	}
}

func assertAdministrativeEvent(t *testing.T, store *Store, grantID, wantIssuer, wantActor, wantReason string) {
	t.Helper()
	var issuer, actor, reason string
	var administrative int
	if err := store.db.QueryRow(`SELECT v.issuer_principal_id,e.actor_principal_id,e.administrative,e.reason
		FROM grant_versions v JOIN grant_events e ON e.grant_id=v.grant_id AND e.grant_version=v.version
		WHERE v.grant_id=? ORDER BY e.revision LIMIT 1`, grantID).
		Scan(&issuer, &actor, &administrative, &reason); err != nil {
		t.Fatal(err)
	}
	if issuer != wantIssuer || actor != wantActor || administrative != 1 || reason != wantReason {
		t.Fatalf("event = issuer %q actor %q admin %d reason %q", issuer, actor, administrative, reason)
	}
}

func TestAuthority_AdministrativeOperatorLeadsVisibleChain(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 3)
	root := f.root
	root.GrantID = "grant_admin_visible"
	root.IssuerPrincipalID = "principal_external_issuer"
	reason := "operator adopts external issuer terms"
	issueAt := f.now.Add(time.Second)
	act := authorityActorAct(t, f.store, f.resolver, f.issuer, "issue",
		CanonicalAdminAuthorityGrant(root, reason), issueAt)
	if err := f.store.AdminIssueAuthority(context.Background(), root, act, reason, issueAt); err != nil {
		t.Fatal(err)
	}
	if err := f.store.PutExecutionBinding(context.Background(), action.ExecutionBinding{
		BindingID: "binding_admin_visible", ActorPrincipalID: root.SubjectPrincipalID,
		Channel: "webhook", ConversationID: "admin-visible",
		IntentID: f.intent.IntentID, IntentVersion: f.intent.Version,
		IntentDigest: f.intent.Digest(), GrantID: root.GrantID,
		GrantVersion: root.Version, GrantDigest: root.Digest(), Revision: 1,
		Status: action.BindingActive,
	}); err != nil {
		t.Fatal(err)
	}
	const requestID = "request-admin-visible"
	ingress, err := f.issuer.Issue(requestID, "authenticated-subject")
	if err != nil {
		t.Fatal(err)
	}
	parked, err := f.store.ParkAuthorization(context.Background(), AuthorityPendingRequest{
		ActorPrincipalID: root.SubjectPrincipalID, CorrelationID: requestID,
		SourceProtocol: "native", Channel: "webhook", ConversationID: "admin-visible",
		Operation: action.Operation{Namespace: "tool", Name: "probe", Version: 1},
		Arguments: `{}`, EffectClass: action.EffectWriteReversible, At: f.now.Add(2 * time.Second),
		ApprovalContext: action.ApprovalContext{
			ToolCage: "probe", Descriptor: action.EffectDescriptor{Class: action.EffectWriteReversible},
			HasDescriptor: true, LawVersion: 1, LawDigest: "sha256:authority-law",
			TTL: time.Minute,
		},
		ResolveEvidence: func(actionID string) (identity.Evidence, error) {
			return f.resolver.Resolve(ingress, identity.ResolveRequest{
				ActionID: actionID, RequestID: requestID, Channel: "webhook", Brain: "alpha",
			})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := f.store.ApprovalDetail(context.Background(), parked.ApprovalID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"principal_responsible", "principal_external_issuer", "principal_brain_alpha"}
	if detail.Authority == nil || !sameStringSlice(detail.Authority.PrincipalChain, want) {
		t.Fatalf("visible chain = %#v, want %#v", detail.Authority, want)
	}
}

func TestAuthority_LegacyImportIsExplicitAndCarriesConsumedBaseline(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 8)
	ctx := context.Background()
	validFrom := f.now.Add(-time.Minute)
	expiresAt := f.now.Add(time.Hour)
	legacyIntent := action.IntentContract{
		IntentID: "int_legacy_import", SchemaVersion: 1,
		OwnerPrincipalID: "principal_external_issuer", Purpose: "run imported probe",
		AllowedOperations: []string{"probe"}, AllowedResources: []string{"cage-a"},
		Budgets:   action.Budgets{MaxActions: 5, MaxActionsPerOperation: map[string]int{"probe": 3}},
		ValidFrom: validFrom, ExpiresAt: expiresAt, Status: action.LifecycleActive, Version: 1,
	}
	if err := f.store.CreateIntent(ctx, legacyIntent); err != nil {
		t.Fatal(err)
	}
	total := int64(5)
	v2Intent := action.IntentContractV2{
		IntentID: legacyIntent.IntentID, SchemaVersion: 2, Version: 1, ProfileID: f.intent.ProfileID,
		OwnerPrincipalID: legacyIntent.OwnerPrincipalID, Purpose: legacyIntent.Purpose,
		Operations:       []action.OperationRef{{Namespace: "legacy", Name: "probe", Version: 1}},
		AllowedResources: []action.ResourceRef{{Kind: "legacy", ID: "cage-a"}},
		EffectClasses:    []action.EffectClass{action.EffectWriteReversible},
		Budget: action.IntentBudgetV2{Total: &total,
			PerOperation: map[string]int64{"legacy/probe@1": 3}},
		ValidFrom: validFrom, ExpiresAt: expiresAt, MaxDelegationDepth: 2,
	}
	if err := f.store.CreateIntentV2(ctx, v2Intent, v2Intent.OwnerPrincipalID, f.now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := f.store.ActivateIntentV2(ctx, v2Intent.IntentID, v2Intent.Version,
		v2Intent.OwnerPrincipalID, f.now); err != nil {
		t.Fatal(err)
	}
	legacy := action.AuthorityGrant{
		GrantID: "grant_legacy_import", IntentID: legacyIntent.IntentID,
		IssuerPrincipalID: legacyIntent.OwnerPrincipalID, SubjectPrincipalID: "principal_legacy_agent",
		Operations: []string{"probe"}, ResourceScope: []string{"cage-a"},
		Budgets:   action.Budgets{MaxActions: 5, MaxActionsPerOperation: map[string]int{"probe": 3}},
		ValidFrom: validFrom, ExpiresAt: expiresAt, DelegationDepthRemaining: 2,
		EffectCeiling: action.EffectWriteReversible, Status: action.LifecycleActive,
	}
	if err := f.store.CreateGrant(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if rule, err := f.store.ConsumeBudget(ctx, legacy.GrantID, legacy.Budgets, "probe"); err != nil || rule != "" {
			t.Fatalf("legacy consume = %q, %v", rule, err)
		}
	}
	grant := action.AuthorityGrantV2{
		GrantID: legacy.GrantID, SchemaVersion: 2, Version: 1, ProfileID: v2Intent.ProfileID,
		IntentID: v2Intent.IntentID, IntentVersion: v2Intent.Version, IntentDigest: v2Intent.Digest(),
		IssuerPrincipalID: legacy.IssuerPrincipalID, SubjectPrincipalID: legacy.SubjectPrincipalID,
		Operations: append([]action.OperationRef(nil), v2Intent.Operations...), Channels: []string{"webhook"},
		AllowedResources: append([]action.ResourceRef(nil), v2Intent.AllowedResources...),
		EffectClasses:    append([]action.EffectClass(nil), v2Intent.EffectClasses...),
		EffectCeiling:    legacy.EffectCeiling,
		Budget: action.IntentBudgetV2{Total: &total,
			PerOperation: map[string]int64{"legacy/probe@1": 3}},
		ValidFrom: validFrom, ExpiresAt: expiresAt, DelegationDepthRemaining: 2,
		Status: action.LifecycleActive,
	}
	reason := "adopt reviewed v1 authority without erasing its spend"
	act := authorityActorAct(t, f.store, f.resolver, f.issuer, "import",
		CanonicalLegacyAuthorityImport(legacy.GrantID, grant, reason), f.now.Add(time.Second))
	if err := f.store.ImportLegacyAuthority(ctx, legacy.GrantID, grant, act, reason, f.now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	account := budgetAccountID(grant.ProfileID, "grant", grant.GrantID)
	for operation, want := range map[string]int64{"*": 2, "legacy/probe@1": 2} {
		var spent int64
		if err := f.store.db.QueryRow(`SELECT spent FROM budget_counters WHERE account_id=? AND operation_key=?`, account, operation).Scan(&spent); err != nil {
			t.Fatal(err)
		}
		if spent != want {
			t.Fatalf("%s spent = %d, want %d", operation, spent, want)
		}
	}
	var baseline int64
	if err := f.store.db.QueryRow(`SELECT baseline_spent FROM legacy_authority_imports WHERE grant_id=?`, legacy.GrantID).Scan(&baseline); err != nil {
		t.Fatal(err)
	}
	if baseline != 2 {
		t.Fatalf("baseline = %d", baseline)
	}
	if err := f.store.ImportLegacyAuthority(ctx, legacy.GrantID, grant, act, reason, f.now.Add(2*time.Second)); !errors.Is(err, identity.ErrIdentityBindingMismatch) {
		t.Fatalf("reused import act error = %v", err)
	}
}

func TestAuthority_LegacyImportValidatorRejectsEveryMismatch(t *testing.T) {
	validFrom := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	expiresAt := validFrom.Add(time.Hour)
	maximum := int64(5)
	legacy := action.AuthorityGrant{
		GrantID: "grant_legacy", IntentID: "int_legacy", IssuerPrincipalID: "principal_owner",
		SubjectPrincipalID: "principal_agent", Operations: []string{"probe"}, ResourceScope: []string{"cage-a"},
		Budgets:   action.Budgets{MaxActions: 5, MaxActionsPerOperation: map[string]int{"probe": 3}},
		ValidFrom: validFrom, ExpiresAt: expiresAt, DelegationDepthRemaining: 2,
		EffectCeiling: action.EffectWriteReversible,
	}
	intent := action.IntentContractV2{IntentID: legacy.IntentID, ProfileID: "profile_a"}
	grant := action.AuthorityGrantV2{
		IntentID: legacy.IntentID, ProfileID: intent.ProfileID,
		IssuerPrincipalID: legacy.IssuerPrincipalID, SubjectPrincipalID: legacy.SubjectPrincipalID,
		Operations: []action.OperationRef{{Namespace: "legacy", Name: "probe", Version: 1}},
		Channels:   []string{"console"}, AllowedResources: []action.ResourceRef{{Kind: "legacy", ID: "cage-a"}},
		EffectClasses: []action.EffectClass{action.EffectWriteReversible}, EffectCeiling: legacy.EffectCeiling,
		Budget:    action.IntentBudgetV2{Total: &maximum, PerOperation: map[string]int64{"legacy/probe@1": 3}},
		ValidFrom: validFrom, ExpiresAt: expiresAt, DelegationDepthRemaining: 2,
	}
	if err := validateLegacyImport(legacy, grant, intent); err != nil {
		t.Fatal(err)
	}
	cases := []func(*action.AuthorityGrantV2){
		func(g *action.AuthorityGrantV2) { g.SubjectPrincipalID = "other" },
		func(g *action.AuthorityGrantV2) { g.Operations = nil },
		func(g *action.AuthorityGrantV2) { g.AllowedResources = nil },
		func(g *action.AuthorityGrantV2) {
			g.DeniedResources = []action.ResourceRef{{Kind: "legacy", ID: "deny"}}
		},
		func(g *action.AuthorityGrantV2) { g.Channels = nil },
		func(g *action.AuthorityGrantV2) { g.EffectClasses = nil },
		func(g *action.AuthorityGrantV2) { g.Budget.Total = nil },
		func(g *action.AuthorityGrantV2) { g.Budget.PerOperation = nil },
		func(g *action.AuthorityGrantV2) { g.Budget.PerOperation["legacy/probe@1"] = 2 },
		func(g *action.AuthorityGrantV2) { g.ValidFrom = g.ValidFrom.Add(time.Second) },
	}
	for i, mutate := range cases {
		bad := grant
		bad.Budget.PerOperation = map[string]int64{"legacy/probe@1": 3}
		mutate(&bad)
		if !errors.Is(validateLegacyImport(legacy, bad, intent), action.ErrAttenuationViolated) {
			t.Fatalf("mismatch %d accepted", i)
		}
	}
	unlimitedLegacy := legacy
	unlimitedLegacy.Budgets.MaxActions = 0
	unlimited := grant
	unlimited.Budget.Total = nil
	if err := validateLegacyImport(unlimitedLegacy, unlimited, intent); err != nil {
		t.Fatal(err)
	}
	unlimited.Budget.Total = &maximum
	if !errors.Is(validateLegacyImport(unlimitedLegacy, unlimited, intent), action.ErrAttenuationViolated) {
		t.Fatal("unlimited legacy imported as finite")
	}
}

func TestAuthority_LegacyImportMayAttachOnlyAsAValidAttenuatedChild(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 8)
	ctx := context.Background()
	validFrom := f.now.Add(-time.Minute)
	expiresAt := f.now.Add(time.Hour)
	maximum := int64(4)
	intent := action.IntentContractV2{
		IntentID: "int_legacy_child", SchemaVersion: 2, Version: 1, ProfileID: f.intent.ProfileID,
		OwnerPrincipalID: f.root.SubjectPrincipalID, Purpose: "import a reviewed child",
		Operations:       []action.OperationRef{{Namespace: "legacy", Name: "probe", Version: 1}},
		AllowedResources: []action.ResourceRef{{Kind: "legacy", ID: "cage-a"}},
		EffectClasses:    []action.EffectClass{action.EffectWriteReversible},
		Budget:           action.IntentBudgetV2{Total: &maximum, PerOperation: map[string]int64{"legacy/probe@1": 4}},
		ValidFrom:        validFrom, ExpiresAt: expiresAt, MaxDelegationDepth: 3,
	}
	if err := f.store.CreateIntentV2(ctx, intent, intent.OwnerPrincipalID, f.now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := f.store.ActivateIntentV2(ctx, intent.IntentID, intent.Version, intent.OwnerPrincipalID, f.now); err != nil {
		t.Fatal(err)
	}
	parent := action.AuthorityGrantV2{
		GrantID: "grant_legacy_parent_v2", SchemaVersion: 2, Version: 1, ProfileID: intent.ProfileID,
		IntentID: intent.IntentID, IntentVersion: intent.Version, IntentDigest: intent.Digest(),
		IssuerPrincipalID: intent.OwnerPrincipalID, SubjectPrincipalID: "principal_legacy_parent",
		Operations: append([]action.OperationRef(nil), intent.Operations...), Channels: []string{"webhook"},
		AllowedResources: append([]action.ResourceRef(nil), intent.AllowedResources...),
		EffectClasses:    append([]action.EffectClass(nil), intent.EffectClasses...),
		EffectCeiling:    action.EffectWriteReversible,
		Budget:           action.IntentBudgetV2{Total: &maximum, PerOperation: map[string]int64{"legacy/probe@1": 4}},
		ValidFrom:        validFrom, ExpiresAt: expiresAt, DelegationDepthRemaining: 3,
		Status: action.LifecycleActive,
	}
	issueAct := authorityActorAct(t, f.store, f.resolver, f.issuer, "issue", parent.CanonicalBytes(), f.now.Add(time.Second))
	if err := f.store.IssueAuthority(ctx, parent, issueAct, f.now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	legacyIntent := action.IntentContract{
		IntentID: intent.IntentID, SchemaVersion: 1, OwnerPrincipalID: parent.SubjectPrincipalID,
		Purpose: intent.Purpose, AllowedOperations: []string{"probe"}, AllowedResources: []string{"cage-a"},
		Budgets:   action.Budgets{MaxActions: 2, MaxActionsPerOperation: map[string]int{"probe": 2}},
		ValidFrom: validFrom.Add(time.Second), ExpiresAt: expiresAt.Add(-time.Second),
		Status: action.LifecycleActive, Version: 1,
	}
	if err := f.store.CreateIntent(ctx, legacyIntent); err != nil {
		t.Fatal(err)
	}
	legacy := action.AuthorityGrant{
		GrantID: "grant_legacy_child", IntentID: intent.IntentID,
		IssuerPrincipalID: parent.SubjectPrincipalID, SubjectPrincipalID: "principal_legacy_leaf",
		Operations: []string{"probe"}, ResourceScope: []string{"cage-a"},
		Budgets:   action.Budgets{MaxActions: 2, MaxActionsPerOperation: map[string]int{"probe": 2}},
		ValidFrom: legacyIntent.ValidFrom, ExpiresAt: legacyIntent.ExpiresAt,
		DelegationDepthRemaining: 2, EffectCeiling: action.EffectWriteReversible,
		Status: action.LifecycleActive,
	}
	if err := f.store.CreateGrant(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	childMaximum := int64(2)
	child := action.AuthorityGrantV2{
		GrantID: legacy.GrantID, SchemaVersion: 2, Version: 1, ProfileID: intent.ProfileID,
		IntentID: intent.IntentID, IntentVersion: intent.Version, IntentDigest: intent.Digest(),
		IssuerPrincipalID: legacy.IssuerPrincipalID, SubjectPrincipalID: legacy.SubjectPrincipalID,
		ParentGrantID: parent.GrantID, ParentGrantVersion: parent.Version,
		Operations: append([]action.OperationRef(nil), intent.Operations...), Channels: []string{"webhook"},
		AllowedResources: append([]action.ResourceRef(nil), intent.AllowedResources...),
		EffectClasses:    append([]action.EffectClass(nil), intent.EffectClasses...),
		EffectCeiling:    action.EffectWriteReversible,
		Budget:           action.IntentBudgetV2{Total: &childMaximum, PerOperation: map[string]int64{"legacy/probe@1": 2}},
		ValidFrom:        legacy.ValidFrom, ExpiresAt: legacy.ExpiresAt, DelegationDepthRemaining: 2,
		Status: action.LifecycleActive,
	}
	reason := "attach the reviewed legacy child below its current parent"
	act := authorityActorAct(t, f.store, f.resolver, f.issuer, "import",
		CanonicalLegacyAuthorityImport(legacy.GrantID, child, reason), f.now.Add(2*time.Second))
	if err := f.store.ImportLegacyAuthority(ctx, legacy.GrantID, child, act, reason, f.now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	var parentID string
	if err := f.store.db.QueryRow(`SELECT parent_grant_id FROM grant_versions WHERE grant_id=?`, child.GrantID).Scan(&parentID); err != nil {
		t.Fatal(err)
	}
	if parentID != parent.GrantID {
		t.Fatalf("parent = %q", parentID)
	}
}

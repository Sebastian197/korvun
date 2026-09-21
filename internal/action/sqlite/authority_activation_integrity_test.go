// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

func TestAuthority_ActivationAdoptsLegacyApprovalsAndChainsStrictBirths(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 4)
	ctx := context.Background()
	legacy := createLegacyApprovalBeforeAuthority(t, f, "act_legacy_before_authority")
	manifest, err := f.store.AuthorityActivationManifest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reason := "adopt the exact legacy approval manifest"
	act := authorityActorAct(t, f.store, f.resolver, f.issuer, "activate",
		CanonicalAuthorityActivation(f.intent.ProfileID, manifest, reason), f.now.Add(time.Second))
	root, err := f.store.ActivateAuthority(ctx, f.intent.ProfileID, act, reason, f.now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if root == "" {
		t.Fatal("activation returned an empty root")
	}
	if err := f.store.RequireAuthorityActivation(ctx, "", root); err != nil {
		t.Fatal(err)
	}
	if f.store.ActivatedAuthorityProfile() != f.intent.ProfileID {
		t.Fatalf("active profile = %q", f.store.ActivatedAuthorityProfile())
	}
	var strict, sequence int
	if err := f.store.db.QueryRow(`SELECT strict_required,sequence FROM approval_birth_events WHERE approval_id=?`, legacy.ApprovalID).
		Scan(&strict, &sequence); err != nil {
		t.Fatal(err)
	}
	if strict != 0 || sequence != 1 {
		t.Fatalf("legacy birth = strict %d sequence %d", strict, sequence)
	}
	parked := parkStrictAuthorityFixture(t, f)
	if parked.ApprovalID == legacy.ApprovalID {
		t.Fatal("strict approval reused a legacy id")
	}
	if err := f.store.RequireAuthorityActivation(ctx, f.intent.ProfileID, root); err != nil {
		t.Fatal(err)
	}
	if err := f.store.RequireAuthorityActivation(ctx, f.intent.ProfileID, "sha256:wrong"); !errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
		t.Fatalf("wrong pin error = %v", err)
	}
	var strictBirths int
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM approval_birth_events WHERE strict_required=1 AND approval_id=?`, parked.ApprovalID).Scan(&strictBirths); err != nil {
		t.Fatal(err)
	}
	if strictBirths != 1 {
		t.Fatalf("strict births = %d", strictBirths)
	}
}

func createLegacyApprovalBeforeAuthority(t *testing.T, f authoritySQLiteFixture, actionID string) action.Approval {
	t.Helper()
	env, evidence := identityAttempt(t, f.resolver, f.issuer, actionID, f.now)
	env.Effect = action.Effect{Class: string(action.EffectWriteReversible)}
	const params = `{"legacy":true}`
	env.ParametersDigest = action.Digest(env.Operation, params)
	bound, err := action.NewBoundApprovalRequest(env, params, action.ApprovalContext{
		IntentPurpose: "legacy approval adopted at activation", ToolCage: "probe",
		Descriptor:    action.EffectDescriptor{Class: action.EffectWriteReversible},
		HasDescriptor: true, LawVersion: 1, LawDigest: "sha256:legacy-law",
		Rule: "require_approval", Now: f.now, TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.CreateApprovalRequestAuthenticated(context.Background(), bound, evidence); err != nil {
		t.Fatal(err)
	}
	return bound.Approval()
}

func TestAuthority_ConfigGenerationsAreSignedReplayableAndClosed(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 4)
	ctx := context.Background()
	activateAuthorityFixture(t, f)
	clause := configClauseFixture(f, "read_file", []string{"console"})
	generation, err := f.store.SyncConfigAuthorityClauses(ctx, f.intent.ProfileID,
		f.root.SubjectPrincipalID, []action.ConfigAuthorityClause{clause}, f.now.Add(2*time.Second))
	if err != nil || generation != 1 {
		t.Fatalf("first generation = %d, %v", generation, err)
	}
	replayed, err := f.store.SyncConfigAuthorityClauses(ctx, f.intent.ProfileID,
		f.root.SubjectPrincipalID, []action.ConfigAuthorityClause{clause}, f.now.Add(3*time.Second))
	if err != nil || replayed != generation {
		t.Fatalf("replay generation = %d, %v", replayed, err)
	}
	changed := configClauseFixture(f, "read_file", []string{"telegram"})
	second, err := f.store.SyncConfigAuthorityClauses(ctx, f.intent.ProfileID,
		f.root.SubjectPrincipalID, []action.ConfigAuthorityClause{changed}, f.now.Add(4*time.Second))
	if err != nil || second != 2 {
		t.Fatalf("second generation = %d, %v", second, err)
	}

	duplicateTool := configClauseFixture(f, "read_file", []string{"webhook"})
	if _, err := f.store.SyncConfigAuthorityClauses(ctx, f.intent.ProfileID,
		f.root.SubjectPrincipalID, []action.ConfigAuthorityClause{changed, duplicateTool}, f.now.Add(5*time.Second)); !errors.Is(err, action.ErrAuthorityMalformed) {
		t.Fatalf("duplicate tool error = %v", err)
	}
	foreign := changed
	foreign.ProfileID = "profile_other"
	if _, err := f.store.SyncConfigAuthorityClauses(ctx, f.intent.ProfileID,
		f.root.SubjectPrincipalID, []action.ConfigAuthorityClause{foreign}, f.now.Add(5*time.Second)); !errors.Is(err, action.ErrAuthorityMalformed) {
		t.Fatalf("foreign clause error = %v", err)
	}
	if _, err := f.store.SyncConfigAuthorityClauses(ctx, "profile_other",
		f.root.SubjectPrincipalID, nil, f.now.Add(5*time.Second)); !errors.Is(err, action.ErrAuthorityEvidenceCorrupt) {
		t.Fatalf("foreign profile error = %v", err)
	}

	if _, err := f.store.db.Exec(`UPDATE config_authority_heads SET canonical_head=canonical_head||X'20'
		WHERE profile_id=? AND brain_principal_id=?`, f.intent.ProfileID, f.root.SubjectPrincipalID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.SyncConfigAuthorityClauses(ctx, f.intent.ProfileID,
		f.root.SubjectPrincipalID, []action.ConfigAuthorityClause{changed}, f.now.Add(6*time.Second)); !errors.Is(err, action.ErrAuthorityEvidenceCorrupt) {
		t.Fatalf("tampered head error = %v", err)
	}
}

func activateAuthorityFixture(t *testing.T, f authoritySQLiteFixture) string {
	t.Helper()
	manifest, err := f.store.AuthorityActivationManifest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	reason := "enable strict authority fixture"
	act := authorityActorAct(t, f.store, f.resolver, f.issuer, "activate",
		CanonicalAuthorityActivation(f.intent.ProfileID, manifest, reason), f.now.Add(time.Second))
	root, err := f.store.ActivateAuthority(context.Background(), f.intent.ProfileID,
		act, reason, f.now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.RequireAuthorityActivation(context.Background(), f.intent.ProfileID, root); err != nil {
		t.Fatal(err)
	}
	return root
}

func configClauseFixture(f authoritySQLiteFixture, tool string, channels []string) action.ConfigAuthorityClause {
	clause := action.ConfigAuthorityClause{
		SchemaVersion: 1, ProfileID: f.intent.ProfileID, BrainPrincipal: f.root.SubjectPrincipalID,
		ToolName: tool, Channels: channels, CageDigest: action.HashCanonical("cage:" + tool),
	}
	clause.ClauseID = "cfg_" + strings.TrimPrefix(clause.Digest(), "sha256:")
	return clause
}

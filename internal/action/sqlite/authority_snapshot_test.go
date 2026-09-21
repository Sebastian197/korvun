// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

func authorityApprovalFixture(t *testing.T, id string) (authoritySQLiteFixture, action.Approval, action.AuthorizationSnapshotV1) {
	t.Helper()
	f := newAuthoritySQLiteFixture(t, 5)
	env, evidence := identityAttempt(t, f.resolver, f.issuer, id, f.now)
	env.IntentID = f.intent.IntentID
	env.Operation = action.Operation{Namespace: "tool", Name: "probe", Version: 1}
	env.Effect = action.Effect{Class: string(action.EffectWriteReversible)}
	const params = `{"resource":"fixture"}`
	env.ParametersDigest = action.Digest(env.Operation, params)
	bound, err := action.NewBoundApprovalRequest(env, params, action.ApprovalContext{
		IntentPurpose: f.intent.Purpose,
		GrantID:       f.root.GrantID,
		GrantDepth:    1,
		CostLine:      "maximum 5 starts",
		ToolCage:      "probe",
		Descriptor: action.EffectDescriptor{
			Class: action.EffectWriteReversible,
		},
		HasDescriptor: true,
		LawVersion:    1,
		LawDigest:     "sha256:authority-law",
		Rule:          "require_approval",
		Now:           f.now,
		TTL:           time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	remaining := int64(5)
	snapshot := action.AuthorizationSnapshotV1{
		Kind:                 action.AuthorizationSnapshotPending,
		ActionID:             env.ActionID,
		ApprovalID:           bound.Approval().ApprovalID,
		RequesterPrincipalID: evidence.RequesterPrincipalID,
		ActorPrincipalID:     evidence.ActorPrincipalID,
		IntentID:             f.intent.IntentID,
		IntentVersion:        f.intent.Version,
		IntentDigest:         f.intent.Digest(),
		IntentPurpose:        f.intent.Purpose,
		PrincipalChain:       []string{f.root.IssuerPrincipalID, f.root.SubjectPrincipalID},
		BudgetKind:           action.AuthorizationBudgetFinite,
		BudgetRemaining:      &remaining,
		RecordedAt:           f.now,
	}
	if err := f.store.CreateAuthorizedApprovalRequest(context.Background(), bound, evidence, snapshot); err != nil {
		t.Fatal(err)
	}
	return f, bound.Approval(), snapshot
}

func TestApprovalDetail_AuthoritySnapshotSignatureIsRequired(t *testing.T) {
	f, approval, _ := authorityApprovalFixture(t, "act_authority_snapshot_signature")
	if _, err := f.store.db.Exec(`UPDATE authorization_snapshots
		SET intent_purpose='forged purpose', canonical_context=replace(canonical_context,'Run the authority probe','forged purpose'),
		    authorization_digest='sha256:coherent-looking-but-unsigned'
		WHERE action_id=?`, approval.ActionID); err != nil {
		t.Fatal(err)
	}
	_, err := f.store.ApprovalDetail(context.Background(), approval.ApprovalID)
	if !errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
		t.Fatalf("detail error = %v, want ErrAuthorizationSnapshotCorrupt", err)
	}
}

func TestApprovalDetail_MissingRequiredSnapshotIsCorrupt(t *testing.T) {
	f, approval, _ := authorityApprovalFixture(t, "act_authority_snapshot_missing")
	if _, err := f.store.db.Exec(`DELETE FROM authorization_snapshots WHERE action_id=?`, approval.ActionID); err != nil {
		t.Fatal(err)
	}
	_, err := f.store.ApprovalDetail(context.Background(), approval.ApprovalID)
	if !errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
		t.Fatalf("detail error = %v, want ErrAuthorizationSnapshotCorrupt", err)
	}
}

func TestApprovalDetail_AuthoritySnapshotRoundTripsExactDisplayFacts(t *testing.T) {
	f, approval, want := authorityApprovalFixture(t, "act_authority_snapshot_roundtrip")
	got, err := f.store.ApprovalDetail(context.Background(), approval.ApprovalID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Authority == nil {
		t.Fatal("authority snapshot is absent")
	}
	normalized := *got.Authority
	if normalized.IdentityEvidenceDigest == "" {
		t.Fatal("authority snapshot does not bind signed identity evidence")
	}
	normalized.IdentityEvidenceDigest = ""
	if string(normalized.CanonicalBytes()) != string(want.CanonicalBytes()) {
		t.Fatalf("authority snapshot = %#v, want %#v", got.Authority, want)
	}
}

func TestApprovalDetail_ActivatedLedgerPreventsLegacyDowngrade(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 5)
	reason := "activate strict approval evidence"
	manifest, err := f.store.AuthorityActivationManifest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	act := authorityActorAct(t, f.store, f.resolver, f.issuer, "activate",
		CanonicalAuthorityActivation(f.intent.ProfileID, manifest, reason), f.now)
	digest, err := f.store.ActivateAuthority(context.Background(), f.intent.ProfileID,
		act, reason, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.RequireAuthorityActivation(context.Background(),
		f.intent.ProfileID, digest); err != nil {
		t.Fatal(err)
	}
	parked := parkStrictAuthorityFixture(t, f)
	if _, err := f.store.db.Exec(`UPDATE approvals SET authority_snapshot_required=0
		WHERE approval_id=?`, parked.ApprovalID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.db.Exec(`DELETE FROM authorization_snapshots WHERE action_id=?`,
		parked.ActionID); err != nil {
		t.Fatal(err)
	}
	_, err = f.store.ApprovalDetail(context.Background(), parked.ApprovalID)
	if !errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
		t.Fatalf("downgraded detail error = %v", err)
	}
}

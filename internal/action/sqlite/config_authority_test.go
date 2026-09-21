// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
)

func TestAuthority_ConfigHeadRemovalInvalidatesFutureStart(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 3)
	ctx := context.Background()
	manifest, err := f.store.AuthorityActivationManifest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reason := "enable strict authority"
	act := authorityActorAct(t, f.store, f.resolver, f.issuer, "activate",
		CanonicalAuthorityActivation(f.intent.ProfileID, manifest, reason), f.now.Add(5))
	root, err := f.store.ActivateAuthority(ctx, f.intent.ProfileID, act, reason, f.now.Add(5))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.RequireAuthorityActivation(ctx, f.intent.ProfileID, root); err != nil {
		t.Fatal(err)
	}
	clause := action.ConfigAuthorityClause{
		SchemaVersion: 1, ProfileID: f.intent.ProfileID,
		BrainPrincipal: f.root.SubjectPrincipalID, ToolName: "probe",
		Channels: []string{"webhook"}, CageDigest: action.HashCanonical("cage-a"),
	}
	clause.ClauseID = "cfg_" + strings.TrimPrefix(clause.Digest(), "sha256:")
	if _, err := f.store.SyncConfigAuthorityClauses(ctx, f.intent.ProfileID,
		f.root.SubjectPrincipalID, []action.ConfigAuthorityClause{clause}, f.now.Add(6)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.db.Exec(`UPDATE execution_bindings SET grant_id=NULL,grant_version=NULL,grant_digest=NULL WHERE binding_id='binding_authority_root'`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.StartAuthorization(ctx, authorityStartRequest(f, "config-ok", f.now.Add(7))); err != nil {
		t.Fatalf("config start: %v", err)
	}
	if _, err := f.store.SyncConfigAuthorityClauses(ctx, f.intent.ProfileID,
		f.root.SubjectPrincipalID, nil, f.now.Add(8)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.StartAuthorization(ctx, authorityStartRequest(f, "config-removed", f.now.Add(9))); !errors.Is(err, ErrAuthorityMissing) {
		t.Fatalf("removed clause error = %v", err)
	}
}

func TestAuthority_ActivationRootIsPinnedAndNotRecreated(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 2)
	ctx := context.Background()
	manifest, err := f.store.AuthorityActivationManifest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reason := "pin strict birth history"
	act := authorityActorAct(t, f.store, f.resolver, f.issuer, "activate",
		CanonicalAuthorityActivation(f.intent.ProfileID, manifest, reason), f.now.Add(10))
	digest, err := f.store.ActivateAuthority(ctx, f.intent.ProfileID, act, reason, f.now.Add(10))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.RequireAuthorityActivation(ctx, f.intent.ProfileID, digest); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.db.Exec(`DELETE FROM approval_birth_events WHERE profile_id=?`, f.intent.ProfileID); err != nil {
		t.Fatal(err)
	}
	if err := f.store.RequireAuthorityActivation(ctx, f.intent.ProfileID, digest); !errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
		t.Fatalf("deleted root error = %v", err)
	}
	if _, err := f.store.ActivateAuthority(ctx, f.intent.ProfileID, act, reason, f.now.Add(11)); !errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
		t.Fatalf("reactivation error = %v", err)
	}
}

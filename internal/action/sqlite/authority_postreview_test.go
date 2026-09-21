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

func TestAuthority_ImmediateStartRequiresFreshAuthenticatedEvidence(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 2)
	req := authorityStartRequest(f, "", f.now)
	req.ResolveEvidence = nil
	if _, err := f.store.StartAuthorization(context.Background(), req); !errors.Is(err, identity.ErrIdentityEvidenceMissing) {
		t.Fatalf("start error = %v, want authenticated evidence", err)
	}
	if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM authorization_starts`); n != 0 {
		t.Fatalf("unauthenticated starts = %d", n)
	}
}

func TestAuthority_MutationDoorRejectsLegacyIdentitySnapshot(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 2)
	grant := f.root
	grant.GrantID = "grant_legacy_actor"
	grant.IssuerPrincipalID = "principal_external_issuer"
	params := CanonicalAdminAuthorityGrant(grant, "legacy actor attack")
	env := action.NewEnvelope("act_legacy_authority_actor", "legacy-authority-actor",
		action.Source{Kind: "operator", Protocol: "cli", Channel: "cli"},
		action.Operation{Namespace: "authority", Name: "issue", Version: 1}, string(params), f.now)
	ident := testIdentity()
	ident.PrincipalID = "principal_brain_alpha"
	if err := f.store.RecordAttemptIdentified(context.Background(), env,
		Decision{Outcome: "allow", Rule: "operator"}, action.StateAuthorized, ident); err != nil {
		t.Fatal(err)
	}
	err := f.store.AdminIssueAuthority(context.Background(), grant, env.ActionID,
		"legacy actor attack", f.now.Add(time.Second))
	if !errors.Is(err, identity.ErrIdentityEvidenceMissing) {
		t.Fatalf("legacy actor error = %v, want v2 identity refusal", err)
	}
	if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM grant_versions WHERE grant_id=?`, grant.GrantID); n != 0 {
		t.Fatalf("legacy actor wrote %d grant rows", n)
	}
}

func TestAuthority_SignedDebitTailRejectsCoherentProjectionRewrite(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 2)
	started, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.db.Exec(`UPDATE budget_debits SET cumulative_spent=0 WHERE action_id=?`, started.ActionID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.db.Exec(`UPDATE budget_counters SET spent=0`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now.Add(time.Second))); !errors.Is(err, ErrBudgetEvidenceCorrupt) {
		t.Fatalf("rewritten signed tail error = %v", err)
	}
}

func TestAuthority_ImmediateStartCannotClaimArbitraryApproval(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 2)
	approval := boundPark(t, f.store, "act_unrelated_approval")
	req := authorityStartRequest(f, "", f.now)
	req.ApprovalID = approval.ApprovalID
	if _, err := f.store.StartAuthorization(context.Background(), req); !errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
		t.Fatalf("arbitrary approval claim error = %v", err)
	}
	if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM authorization_starts`); n != 0 {
		t.Fatalf("arbitrary approval created %d starts", n)
	}
	params, err := f.store.ApprovalParams(context.Background(), approval.ApprovalID)
	if err != nil || len(params) == 0 {
		t.Fatalf("approval params = %q, %v", params, err)
	}
}

func TestAuthority_StartProofRejectsUnsignedProjectionRewrite(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 2)
	started, err := f.store.StartAuthorization(context.Background(),
		authorityStartRequest(f, "", f.now))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.db.Exec(`UPDATE authorization_starts
		SET generation=999,debit_set_digest='sha256:forged' WHERE action_id=?`,
		started.ActionID); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Recover(context.Background(), f.now.Add(time.Second)); !errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
		t.Fatalf("rewritten start proof error = %v", err)
	}
}

func TestAuthority_ConfirmedFailedStartDoesNotRefund(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 1)
	started, err := f.store.StartAuthorization(context.Background(),
		authorityStartRequest(f, "", f.now))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.Finish(context.Background(), started.ActionID,
		action.StateFailed, f.now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.StartAuthorization(context.Background(),
		authorityStartRequest(f, "", f.now.Add(2*time.Second))); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("start after confirmed failure = %v", err)
	}
}

func TestAuthority_DelegateRejectsRewrittenCounter(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 10)
	if _, err := f.store.StartAuthorization(context.Background(),
		authorityStartRequest(f, "", f.now)); err != nil {
		t.Fatal(err)
	}
	account := budgetAccountID(f.root.ProfileID, "grant", f.root.GrantID)
	if _, err := f.store.db.Exec(`UPDATE budget_counters SET spent=0
		WHERE account_id=? AND operation_key='*'`, account); err != nil {
		t.Fatal(err)
	}
	child := f.root
	child.GrantID = "grant_counter_rewrite_child"
	child.ParentGrantID, child.ParentGrantVersion = f.root.GrantID, f.root.Version
	child.IssuerPrincipalID = f.root.SubjectPrincipalID
	child.SubjectPrincipalID = "principal_worker"
	child.DelegationDepthRemaining = f.root.DelegationDepthRemaining - 1
	act := authorityActorAct(t, f.store, f.resolver, f.issuer,
		"delegate", child.CanonicalBytes(), f.now.Add(time.Second))
	err := f.store.DelegateAuthority(context.Background(), child, act, f.now.Add(2*time.Second))
	if !errors.Is(err, ErrBudgetEvidenceCorrupt) {
		t.Fatalf("error = %v", err)
	}
	if n := authorityScalar(t, f.store,
		`SELECT COUNT(*) FROM grant_versions WHERE grant_id=?`, child.GrantID); n != 0 {
		t.Fatalf("child rows = %d", n)
	}
}

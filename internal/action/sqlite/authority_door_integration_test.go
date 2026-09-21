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

func TestAuthority_DelegateDoorInvokesFullAttenuation(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 5)
	child := authorityChildForDoor(f, "grant_widened_door")
	child.Operations = append(child.Operations,
		action.OperationRef{Namespace: "tool", Name: "outside", Version: 1})
	act := authorityActorAct(t, f.store, f.resolver, f.issuer,
		"delegate", child.CanonicalBytes(), f.now.Add(time.Second))
	err := f.store.DelegateAuthority(context.Background(), child, act, f.now.Add(2*time.Second))
	var attenuation *action.AttenuationError
	if !errors.As(err, &attenuation) || attenuation.Dimension != "operations" {
		t.Fatalf("error = %#v", err)
	}
	if n := authorityScalar(t, f.store,
		`SELECT COUNT(*) FROM grant_versions WHERE grant_id=?`, child.GrantID); n != 0 {
		t.Fatalf("child rows = %d", n)
	}
}

func TestAuthority_StartDoorRejectsSignedInvalidChain(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 5)
	child := authorityChildForDoor(f, "grant_signed_invalid_door")
	child.Operations = append(child.Operations,
		action.OperationRef{Namespace: "tool", Name: "outside", Version: 1})
	tx, err := f.store.beginAuthorityWrite(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.insertGrantTx(context.Background(), tx, child,
		"act_test_invalid_chain", f.root.SubjectPrincipalID, false, "", f.now); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := f.store.ensureBudgetAccountTx(context.Background(), tx, child.ProfileID,
		"grant", child.GrantID, child.Budget, f.now); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	putAuthorityBinding(t, f, child, "invalid-chain")
	_, err = f.store.StartAuthorization(context.Background(),
		authorityStartRequest(f, "invalid-chain", f.now))
	var widened *action.AttenuationError
	if !errors.As(err, &widened) || widened.Dimension != "operations" {
		t.Fatalf("signed invalid chain: error = %v, want the attenuation dimension %q", err, "operations")
	}
}

func TestAuthority_StartDoorBindsActualResourceArguments(t *testing.T) {
	op := action.OperationRef{Namespace: "tool", Name: "read_file", Version: 1}
	f := newAuthoritySQLiteFixtureForTerms(t, 5, op,
		[]action.ResourceRef{{Kind: "path", ID: "/cage"}})
	child := authorityChildForDoor(f, "grant_scoped_door")
	child.AllowedResources = []action.ResourceRef{{Kind: "path", ID: "/cage/a"}}
	act := authorityActorAct(t, f.store, f.resolver, f.issuer,
		"delegate", child.CanonicalBytes(), f.now.Add(time.Second))
	if err := f.store.DelegateAuthority(context.Background(), child, act,
		f.now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	putAuthorityBinding(t, f, child, "resource-scope")
	req := authorityStartRequest(f, "resource-scope", f.now.Add(3*time.Second))
	req.Operation = action.Operation(op)
	req.Arguments = `{"path":"/cage/a/../b/secret.txt"}`
	if _, err := f.store.StartAuthorization(context.Background(), req); !errors.Is(err, action.ErrResourceOutOfScope) {
		t.Fatalf("error = %v", err)
	}
}

func authorityChildForDoor(f authoritySQLiteFixture, id string) action.AuthorityGrantV2 {
	child := f.root
	child.GrantID = id
	child.ParentGrantID, child.ParentGrantVersion = f.root.GrantID, f.root.Version
	child.IssuerPrincipalID = f.root.SubjectPrincipalID
	child.SubjectPrincipalID = f.root.SubjectPrincipalID
	child.DelegationDepthRemaining = f.root.DelegationDepthRemaining - 1
	return child
}

func putAuthorityBinding(t *testing.T, f authoritySQLiteFixture,
	grant action.AuthorityGrantV2, conversation string) {
	t.Helper()
	binding := action.ExecutionBinding{
		BindingID: "binding_" + grant.GrantID, ActorPrincipalID: grant.SubjectPrincipalID,
		Channel: "webhook", ConversationID: conversation, IntentID: f.intent.IntentID,
		IntentVersion: f.intent.Version, IntentDigest: f.intent.Digest(),
		GrantID: grant.GrantID, GrantVersion: grant.Version, GrantDigest: grant.Digest(),
		Revision: 1, Status: action.BindingActive,
	}
	if err := f.store.PutExecutionBinding(context.Background(), binding); err != nil {
		t.Fatal(err)
	}
}

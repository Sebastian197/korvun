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

// TestAuthority_StartDoorBindsActualResourceArguments drives AS-AUTH-16 through
// the START DOOR with the argument strings the real read_file tool is handed —
// a bare path, the tool's own grammar — under a leaf scoped to one directory of
// its parent's. What the door binds is derived from those arguments by the
// store-side analyzer, never declared by the caller.
//
// The third row is the other half of the same finding (the adversary's pass
// over this phase, F2): an analyzer that IS registered and cannot resolve what
// it was given refuses the start even when NO term restricts anything. It used
// to be swallowed into «no analyzer» and the start went ahead with nothing
// bound.
//
// Evidence level: in-process, one real SQLite store, through StartAuthorization.
// Probing mutations executed, each alone: (1) the door skips the actual-use
// check — red on the outside row with «error = <nil>», «budget debits after the
// refusal = 6, want 0» and «durable starts = 2», and on the unresolved row;
// (2) a registered analyzer's failure is treated as «no analyzer» again — red
// on the unresolved row with «error = <nil>» and «budget debits = 4, want 0».
func TestAuthority_StartDoorBindsActualResourceArguments(t *testing.T) {
	op := action.OperationRef{Namespace: "tool", Name: "read_file", Version: 1}

	t.Run("a leaf scoped to one directory", func(t *testing.T) {
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

		outside := authorityStartRequest(f, "resource-scope", f.now.Add(3*time.Second))
		outside.Operation = action.Operation(op)
		outside.Arguments = "/cage/a/../b/secret.txt"
		if _, err := f.store.StartAuthorization(context.Background(), outside); !errors.Is(err, action.ErrResourceOutOfScope) {
			t.Errorf("a path that leaves the leaf's directory: error = %v, want %v", err, action.ErrResourceOutOfScope)
		}
		if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM budget_debits`); n != 0 {
			t.Errorf("budget debits after the refusal = %d, want 0", n)
		}

		inside := authorityStartRequest(f, "resource-scope", f.now.Add(4*time.Second))
		inside.Operation = action.Operation(op)
		inside.Arguments = "  /cage/a/report.txt  "
		if _, err := f.store.StartAuthorization(context.Background(), inside); err != nil {
			t.Errorf("a path inside the leaf's directory: %v", err)
		}
		if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM authorization_starts`); n != 1 {
			t.Errorf("durable starts = %d, want the one inside the scope", n)
		}
	})

	t.Run("a registered analyzer that cannot resolve refuses even with no restriction", func(t *testing.T) {
		f := newAuthoritySQLiteFixtureForOperation(t, 5, op)
		req := authorityStartRequest(f, "", f.now.Add(time.Second))
		req.Operation = action.Operation(op)
		// Relative: the tool would join it to a jail root this layer does not know.
		req.Arguments = "notes/today.txt"
		if _, err := f.store.StartAuthorization(context.Background(), req); !errors.Is(err, action.ErrAuthorityUseUnresolved) {
			t.Errorf("error = %v, want %v", err, action.ErrAuthorityUseUnresolved)
		}
		if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM budget_debits`); n != 0 {
			t.Errorf("budget debits = %d, want 0", n)
		}
	})
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

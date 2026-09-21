// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/identity"
)

type authoritySQLiteFixture struct {
	store    *Store
	resolver *identity.Resolver
	issuer   *identity.Issuer
	private  ed25519.PrivateKey
	now      time.Time
	intent   action.IntentContractV2
	root     action.AuthorityGrantV2
}

func newAuthoritySQLiteFixture(t *testing.T, maximum int64) authoritySQLiteFixture {
	return newAuthoritySQLiteFixtureForOperation(t, maximum,
		action.OperationRef{Namespace: "tool", Name: "probe", Version: 1})
}

func newAuthoritySQLiteFixtureForOperation(t *testing.T, maximum int64,
	operation action.OperationRef) authoritySQLiteFixture {
	return newAuthoritySQLiteFixtureForTerms(t, maximum, operation, nil)
}

func newAuthoritySQLiteFixtureForTerms(t *testing.T, maximum int64,
	operation action.OperationRef, allowedResources []action.ResourceRef) authoritySQLiteFixture {
	t.Helper()
	store, resolver, issuer, privateKey, now := identityStoreFixture(t)
	store.SetIntentV2Signer(
		func(c action.IntentContractV2) action.SignedIntentContractV2 {
			return action.SignIntentContractV2(privateKey, c)
		},
		func(e action.IntentEventV1) action.SignedIntentEventV1 {
			return action.SignIntentEventV1(privateKey, e)
		},
	)
	store.SetAuthoritySigner(func(domain string, canonical []byte) action.AuthoritySignature {
		return action.SignAuthorityBytes(privateKey, domain, canonical)
	})
	intent := action.IntentContractV2{
		IntentID: "int_authority", SchemaVersion: 2, Version: 1, ProfileID: "profile_a",
		OwnerPrincipalID: "principal_brain_alpha", Purpose: "Run the authority probe",
		Operations:       []action.OperationRef{operation},
		AllowedResources: append([]action.ResourceRef(nil), allowedResources...),
		EffectClasses:    []action.EffectClass{action.EffectWriteReversible},
		Budget: action.IntentBudgetV2{Total: &maximum,
			PerOperation: map[string]int64{fmt.Sprintf("%s/%s@%d", operation.Namespace, operation.Name, operation.Version): maximum}},
		ValidFrom: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour), MaxDelegationDepth: 4,
	}
	if err := store.CreateIntentV2(context.Background(), intent, intent.OwnerPrincipalID, now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateIntentV2(context.Background(), intent.IntentID, intent.Version, intent.OwnerPrincipalID, now); err != nil {
		t.Fatal(err)
	}
	root := action.AuthorityGrantV2{
		GrantID: "grant_root", SchemaVersion: 2, Version: 1, ProfileID: intent.ProfileID,
		IntentID: intent.IntentID, IntentVersion: intent.Version, IntentDigest: intent.Digest(),
		IssuerPrincipalID: intent.OwnerPrincipalID, SubjectPrincipalID: "principal_brain_alpha",
		Operations: append([]action.OperationRef(nil), intent.Operations...), Channels: []string{"webhook"},
		AllowedResources: append([]action.ResourceRef(nil), allowedResources...),
		EffectClasses:    append([]action.EffectClass(nil), intent.EffectClasses...), EffectCeiling: action.EffectWriteReversible,
		Budget: action.IntentBudgetV2{Total: &maximum,
			PerOperation: map[string]int64{fmt.Sprintf("%s/%s@%d", operation.Namespace, operation.Name, operation.Version): maximum}},
		ValidFrom: intent.ValidFrom, ExpiresAt: intent.ExpiresAt, DelegationDepthRemaining: 4,
		Status: action.LifecycleActive,
	}
	act := authorityActorAct(t, store, resolver, issuer, "issue", root.CanonicalBytes(), now)
	if err := store.IssueAuthority(context.Background(), root, act, now); err != nil {
		t.Fatal(err)
	}
	binding := action.ExecutionBinding{
		BindingID: "binding_authority_root", ActorPrincipalID: root.SubjectPrincipalID,
		Channel: "webhook", IntentID: intent.IntentID, IntentVersion: intent.Version,
		IntentDigest: intent.Digest(), GrantID: root.GrantID, GrantVersion: root.Version,
		GrantDigest: root.Digest(), Revision: 1, Status: action.BindingActive,
	}
	if err := store.PutExecutionBinding(context.Background(), binding); err != nil {
		t.Fatal(err)
	}
	return authoritySQLiteFixture{store, resolver, issuer, privateKey, now, intent, root}
}

func authorityActorAct(t *testing.T, store *Store, resolver *identity.Resolver, issuer *identity.Issuer, verb string, params []byte, at time.Time) string {
	t.Helper()
	id := fmt.Sprintf("act_authority_%s_%d", verb, at.UnixNano())
	env, evidence := identityAttempt(t, resolver, issuer, id, at)
	env.Operation = action.Operation{Namespace: "authority", Name: verb, Version: 1}
	env.ParametersDigest = action.Digest(env.Operation, string(params))
	env.Effect = action.Effect{Class: string(action.EffectWriteReversible)}
	if err := store.RecordAttemptAuthenticated(context.Background(), env, Decision{Outcome: "allow", Rule: "administrative"}, action.StateAuthorized, evidence); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f authoritySQLiteFixture) delegate(t *testing.T, id, subject string, maximum int64, parent action.AuthorityGrantV2) action.AuthorityGrantV2 {
	t.Helper()
	child := parent
	child.GrantID, child.Version = id, 1
	child.IssuerPrincipalID, child.SubjectPrincipalID = parent.SubjectPrincipalID, subject
	child.ParentGrantID, child.ParentGrantVersion = parent.GrantID, parent.Version
	child.DelegationDepthRemaining = parent.DelegationDepthRemaining - 1
	child.Budget = action.IntentBudgetV2{Total: &maximum, PerOperation: map[string]int64{"tool/probe@1": maximum}}
	act := authorityActorAct(t, f.store, f.resolver, f.issuer, "delegate", child.CanonicalBytes(), f.now.Add(time.Duration(len(id))*time.Nanosecond))
	if err := f.store.DelegateAuthority(context.Background(), child, act, f.now); err != nil {
		t.Fatal(err)
	}
	return child
}

var authorityStartRequestSequence atomic.Uint64

func authorityStartRequest(f authoritySQLiteFixture, conversation string, at time.Time) AuthorityStartRequest {
	requestID := fmt.Sprintf("request-authority-start-%d", authorityStartRequestSequence.Add(1))
	ingress, issueErr := f.issuer.Issue(requestID, "authenticated-subject")
	return AuthorityStartRequest{
		ActorPrincipalID: "principal_brain_alpha", Channel: "webhook", ConversationID: conversation,
		Operation: action.Operation{Namespace: "tool", Name: "probe", Version: 1}, Arguments: `{}`,
		EffectClass: action.EffectWriteReversible, At: at,
		CorrelationID: requestID, ResolveEvidence: func(actionID string) (identity.Evidence, error) {
			if issueErr != nil {
				return identity.Evidence{}, issueErr
			}
			return f.resolver.Resolve(ingress, identity.ResolveRequest{
				ActionID: actionID, RequestID: requestID, Channel: "webhook", Brain: "alpha",
			})
		},
	}
}

func authorityScalar(t *testing.T, store *Store, query string, args ...any) int {
	t.Helper()
	var n int
	if err := store.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

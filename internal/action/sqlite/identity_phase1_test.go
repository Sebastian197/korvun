// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionexecutor "github.com/Sebastian197/korvun/internal/action/executor"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/identity"
	"github.com/Sebastian197/korvun/internal/tool"
)

func identityStoreFixture(t *testing.T) (*Store, *identity.Resolver, *identity.Issuer, ed25519.PrivateKey, time.Time) {
	t.Helper()
	now := time.Date(2026, 9, 21, 16, 0, 0, 0, time.UTC)
	store, err := Open(filepath.Join(t.TempDir(), "identity.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyID := action.SigningKeyID(pub)
	if err := store.PutSigningKey(context.Background(), keyID, hex.EncodeToString(pub), now); err != nil {
		t.Fatal(err)
	}
	store.SetReceiptSealer(func(r action.Receipt) action.Receipt { return action.SignReceipt(priv, r) })
	store.SetIdentitySigners(
		func(e identity.Evidence) identity.SignedEvidence { return identity.SignEvidence(priv, e) },
		func(e identity.PrincipalEvent) identity.SignedPrincipalEvent {
			return identity.SignPrincipalEvent(priv, e)
		},
	)
	store.identityNow = func() time.Time { return now }
	registry := identityRegistryFixture()
	if err := store.RegisterIdentity(context.Background(), registry, now); err != nil {
		t.Fatalf("RegisterIdentity: %v", err)
	}
	resolver, err := identity.NewResolver(registry, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := resolver.NewIssuer(identity.IssuerConfig{
		BindingID: "binding_webhook", Method: "bearer",
		CredentialClass: "shared_secret", TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, resolver, issuer, priv, now
}

func approvedIdentityFixture(t *testing.T, actionID string) (*Store, action.Approval, identity.Evidence, ed25519.PrivateKey, time.Time) {
	t.Helper()
	store, resolver, issuer, privateKey, now := identityStoreFixture(t)
	env, evidence := identityAttempt(t, resolver, issuer, actionID, now)
	env.Effect = action.Effect{Class: string(action.EffectWriteIrreversible)}
	const rawParams = `{"arg":"value"}`
	env.ParametersDigest = action.Digest(env.Operation, rawParams)
	bound, err := action.NewBoundApprovalRequest(env, rawParams, action.ApprovalContext{
		IntentPurpose: "identity claim", ToolCage: "identity test",
		Descriptor:    action.EffectDescriptor{Class: action.EffectWriteIrreversible},
		HasDescriptor: true, LawVersion: 1, LawDigest: "sha256:law",
		Rule: "require_approval", Now: now, TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateApprovalRequestAuthenticated(context.Background(), bound, evidence); err != nil {
		t.Fatal(err)
	}
	approval := bound.Approval()
	decisionEnv, attemptIdentity := operatorDecisionEnv("approve", approval.ApprovalID)
	if _, err := store.DecideApprovalUnderLaw(context.Background(), approval.ApprovalID,
		action.DecisionApproved, now.Add(time.Second), decisionEnv, attemptIdentity, "",
		PolicyPin{Version: 1, Digest: "sha256:law"}); err != nil {
		t.Fatal(err)
	}
	return store, approval, evidence, privateKey, now
}

func TestIngress_CannotTransplantEvidence(t *testing.T) {
	tests := []struct {
		name   string
		want   error
		mutate func(*testing.T, *Store, action.Approval, identity.Evidence, time.Time)
	}{
		{"canonical", identity.ErrIdentityEvidenceCorrupt, func(t *testing.T, store *Store, a action.Approval, _ identity.Evidence, _ time.Time) {
			externalIdentityExec(t, store, `UPDATE actions SET identity_canonical_evidence=identity_canonical_evidence||X'20' WHERE action_id=?`, a.ActionID)
		}},
		{"digest", identity.ErrIdentityEvidenceCorrupt, func(t *testing.T, store *Store, a action.Approval, _ identity.Evidence, _ time.Time) {
			externalIdentityExec(t, store, `UPDATE actions SET identity_evidence_digest='sha256:00' WHERE action_id=?`, a.ActionID)
		}},
		{"key", identity.ErrIdentityEvidenceCorrupt, func(t *testing.T, store *Store, a action.Approval, _ identity.Evidence, _ time.Time) {
			externalIdentityExec(t, store, `UPDATE actions SET identity_signing_key_id='ed25519:unknown' WHERE action_id=?`, a.ActionID)
		}},
		{"signature", identity.ErrIdentityEvidenceCorrupt, func(t *testing.T, store *Store, a action.Approval, _ identity.Evidence, _ time.Time) {
			externalIdentityExec(t, store, `UPDATE actions SET identity_signature='00' WHERE action_id=?`, a.ActionID)
		}},
		{"era", identity.ErrIdentityEvidenceCorrupt, func(t *testing.T, store *Store, a action.Approval, _ identity.Evidence, _ time.Time) {
			externalIdentityExec(t, store, `UPDATE actions SET identity_version=0 WHERE action_id=?`, a.ActionID)
		}},
		{"full evidence missing", identity.ErrIdentityEvidenceCorrupt, func(t *testing.T, store *Store, a action.Approval, _ identity.Evidence, _ time.Time) {
			externalIdentityExec(t, store, `DELETE FROM identity_evidence_v2 WHERE action_id=?`, a.ActionID)
		}},
		{"expired", identity.ErrIdentityEvidenceExpired, func(_ *testing.T, store *Store, _ action.Approval, e identity.Evidence, _ time.Time) {
			store.identityNow = func() time.Time { return e.ExpiresAt }
		}},
		{"requester disabled", identity.ErrPrincipalDisabled, func(t *testing.T, store *Store, _ action.Approval, e identity.Evidence, now time.Time) {
			if err := store.DisablePrincipal(context.Background(), e.RequesterPrincipalID, now.Add(2*time.Second)); err != nil {
				t.Fatal(err)
			}
			store.identityNow = func() time.Time { return now.Add(3 * time.Second) }
		}},
		{"binding revoked", identity.ErrIdentityBindingMismatch, func(t *testing.T, store *Store, _ action.Approval, _ identity.Evidence, _ time.Time) {
			externalIdentityExec(t, store, `UPDATE principal_bindings SET status='revoked' WHERE binding_id='binding_webhook'`)
		}},
		{"binding verified subject changed", identity.ErrIdentityBindingMismatch, func(t *testing.T, store *Store, _ action.Approval, _ identity.Evidence, _ time.Time) {
			externalIdentityExec(t, store, `UPDATE principal_bindings SET verified_subject='other_authenticated_subject' WHERE binding_id='binding_webhook'`)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, approval, evidence, _, now := approvedIdentityFixture(t, "act_claim_"+strings.ReplaceAll(tt.name, " ", "_"))
			tt.mutate(t, store, approval, evidence, now)
			params, _, err := store.ClaimApprovalParamsUnderDigest(context.Background(),
				approval.ApprovalID, &PolicyPin{Version: 1, Digest: "sha256:law"},
				approval.ActionDigest, nil)
			if !errors.Is(err, tt.want) {
				t.Fatalf("claim error = %v, want %v", err, tt.want)
			}
			if params != nil {
				t.Fatalf("refused claim returned params %q", params)
			}
			raw, readErr := store.ApprovalParams(context.Background(), approval.ApprovalID)
			if readErr != nil || len(raw) == 0 {
				t.Fatalf("refused claim consumed params: %q, %v", raw, readErr)
			}
		})
	}

	t.Run("historical key and single connection", func(t *testing.T) {
		store, approval, _, _, now := approvedIdentityFixture(t, "act_claim_historical_key")
		publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.RotateSigningKey(context.Background(), action.SigningKeyID(publicKey),
			hex.EncodeToString(publicKey), now.Add(2*time.Second)); err != nil {
			t.Fatal(err)
		}
		store.SetIdentitySigners(
			func(e identity.Evidence) identity.SignedEvidence { return identity.SignEvidence(privateKey, e) },
			func(e identity.PrincipalEvent) identity.SignedPrincipalEvent {
				return identity.SignPrincipalEvent(privateKey, e)
			},
		)
		store.db.SetMaxOpenConns(1)
		params, _, err := store.ClaimApprovalParamsUnderDigest(context.Background(),
			approval.ApprovalID, &PolicyPin{Version: 1, Digest: "sha256:law"},
			approval.ActionDigest, nil)
		if err != nil || len(params) == 0 {
			t.Fatalf("historical K1 claim = %q, %v", params, err)
		}
	})
}

func identityRegistryFixture() identity.Registry {
	return identity.Registry{
		Principals: []identity.Principal{
			{ID: "principal_webhook", Kind: identity.PrincipalExternalSystem},
			{ID: "principal_brain_alpha", Kind: identity.PrincipalWorkload},
			{ID: "principal_responsible", Kind: identity.PrincipalHuman},
		},
		Bindings: []identity.Binding{{
			ID: "binding_webhook", Provider: "webhook", Channel: "webhook",
			CredentialRef: "WEBHOOK_SECRET", SubjectNamespace: "payload.sender_id",
			VerifiedSubject: "shared_webhook_credential",
			PrincipalID:     "principal_webhook", Generation: 1, Status: identity.BindingActive,
		}},
		Workloads: []identity.Workload{{
			Brain: "alpha", PrincipalID: "principal_brain_alpha",
			ResponsiblePrincipalID: "principal_responsible",
		}},
	}
}

func identityAttempt(t *testing.T, resolver *identity.Resolver, issuer *identity.Issuer, actionID string, now time.Time) (action.Envelope, identity.Evidence) {
	t.Helper()
	requestID := "request-" + actionID
	ingress, err := issuer.Issue(requestID, "forged-operator")
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := resolver.Resolve(ingress, identity.ResolveRequest{
		ActionID: actionID, RequestID: requestID, Channel: "webhook", Brain: "alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	env := action.NewEnvelope(actionID, requestID,
		action.Source{Kind: "agent_brain", Protocol: "text", Channel: "webhook"},
		action.Operation{Namespace: "tool", Name: "probe", Version: 1}, "{}", now)
	env.Principal = action.PrincipalRef{
		PrincipalID:        evidence.ActorPrincipalID,
		ResponsibleHumanID: evidence.ResponsiblePrincipalID,
		EvidenceID:         evidence.EvidenceID,
	}
	return env, evidence
}

func TestIdentity_DisableWinsBeforeStartCommit(t *testing.T) {
	tests := []struct {
		name   string
		want   error
		mutate func(*testing.T, *Store, identity.Evidence, time.Time)
	}{
		{"requester disabled", identity.ErrPrincipalDisabled, func(t *testing.T, store *Store, e identity.Evidence, now time.Time) {
			if err := store.DisablePrincipal(context.Background(), e.RequesterPrincipalID, now.Add(time.Second)); err != nil {
				t.Fatal(err)
			}
		}},
		{"actor disabled", identity.ErrPrincipalDisabled, func(t *testing.T, store *Store, e identity.Evidence, now time.Time) {
			if err := store.DisablePrincipal(context.Background(), e.ActorPrincipalID, now.Add(time.Second)); err != nil {
				t.Fatal(err)
			}
		}},
		{"responsible disabled", identity.ErrPrincipalDisabled, func(t *testing.T, store *Store, e identity.Evidence, now time.Time) {
			if err := store.DisablePrincipal(context.Background(), e.ResponsiblePrincipalID, now.Add(time.Second)); err != nil {
				t.Fatal(err)
			}
		}},
		{"binding revoked", identity.ErrIdentityBindingMismatch, func(t *testing.T, store *Store, _ identity.Evidence, _ time.Time) {
			externalIdentityExec(t, store, `UPDATE principal_bindings SET status='revoked' WHERE binding_id='binding_webhook'`)
		}},
		{"binding generation advanced", identity.ErrIdentityBindingMismatch, func(t *testing.T, store *Store, _ identity.Evidence, _ time.Time) {
			externalIdentityExec(t, store, `UPDATE principal_bindings SET generation=2 WHERE binding_id='binding_webhook'`)
		}},
		{"binding verified subject changed", identity.ErrIdentityBindingMismatch, func(t *testing.T, store *Store, _ identity.Evidence, _ time.Time) {
			externalIdentityExec(t, store, `UPDATE principal_bindings SET verified_subject='other_authenticated_subject' WHERE binding_id='binding_webhook'`)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, resolver, issuer, _, now := identityStoreFixture(t)
			store.identityNow = func() time.Time { return now.Add(2 * time.Second) }
			env, evidence := identityAttempt(t, resolver, issuer, "act_"+strings.ReplaceAll(tt.name, " ", "_"), now)
			tt.mutate(t, store, evidence, now)
			err := store.RecordAttemptAuthenticated(context.Background(), env,
				Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized, evidence)
			if !errors.Is(err, tt.want) {
				t.Fatalf("RecordAttemptAuthenticated error = %v, want %v", err, tt.want)
			}
			if _, err := store.Get(context.Background(), env.ActionID); !errors.Is(err, ErrNotFound) {
				t.Fatalf("Get after refusal = %v, want ErrNotFound", err)
			}
		})
	}

	t.Run("signed disable defeats projection rewind", func(t *testing.T) {
		store, resolver, issuer, _, now := identityStoreFixture(t)
		store.identityNow = func() time.Time { return now.Add(2 * time.Second) }
		env, evidence := identityAttempt(t, resolver, issuer, "act_rewind", now)
		if err := store.DisablePrincipal(context.Background(), evidence.RequesterPrincipalID, now.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		externalIdentityExec(t, store,
			`UPDATE principals SET disabled_at=NULL,revision=1 WHERE principal_id=?`,
			evidence.RequesterPrincipalID)
		if err := store.RegisterIdentity(context.Background(), identityRegistryFixture(), now.Add(2*time.Second)); !errors.Is(err, identity.ErrPrincipalEvidenceCorrupt) {
			t.Fatalf("boot verification = %v, want ErrPrincipalEvidenceCorrupt", err)
		}
		err := store.RecordAttemptAuthenticated(context.Background(), env,
			Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized, evidence)
		if !errors.Is(err, identity.ErrPrincipalEvidenceCorrupt) {
			t.Fatalf("start verification = %v, want ErrPrincipalEvidenceCorrupt", err)
		}
	})

	t.Run("principal event corruption fails boot and start", func(t *testing.T) {
		attacks := []struct {
			name   string
			mutate func(*testing.T, *Store, identity.Evidence, ed25519.PrivateKey)
		}{
			{"coherent canonical rewrite with stale signature", func(t *testing.T, store *Store, e identity.Evidence, privateKey ed25519.PrivateKey) {
				var eventID string
				if err := store.db.QueryRow(`SELECT event_id FROM principal_events
					WHERE principal_id=? AND revision=2`, e.RequesterPrincipalID).Scan(&eventID); err != nil {
					t.Fatal(err)
				}
				externalIdentityExec(t, store, `DELETE FROM principal_events WHERE principal_id=? AND revision=1`, e.RequesterPrincipalID)
				principal := identity.Principal{ID: e.RequesterPrincipalID,
					Kind: identity.PrincipalExternalSystem, CreatedAt: phase1CreatedAt(t, store, e.RequesterPrincipalID), Revision: 1}
				event := identity.PrincipalEvent{EventID: eventID, Principal: principal,
					Revision: 1, Kind: "created", OccurredAt: principal.CreatedAt}
				resigned := identity.SignPrincipalEvent(privateKey, event)
				externalIdentityExec(t, store, `UPDATE principal_events
					SET revision=1,kind='created',occurred_at=?,canonical_event=?,digest=?
					WHERE event_id=?`, event.OccurredAt.Format(time.RFC3339Nano),
					resigned.Canonical, resigned.Digest, eventID)
				externalIdentityExec(t, store, `UPDATE principals SET disabled_at=NULL,revision=1 WHERE principal_id=?`, e.RequesterPrincipalID)
			}},
			{"signature", func(t *testing.T, store *Store, e identity.Evidence, _ ed25519.PrivateKey) {
				externalIdentityExec(t, store, `UPDATE principal_events SET signature='00'
					WHERE principal_id=? AND revision=2`, e.RequesterPrincipalID)
			}},
			{"key", func(t *testing.T, store *Store, e identity.Evidence, _ ed25519.PrivateKey) {
				publicKey, _, err := ed25519.GenerateKey(rand.Reader)
				if err != nil {
					t.Fatal(err)
				}
				keyID := action.SigningKeyID(publicKey)
				if err := store.RotateSigningKey(context.Background(), keyID,
					hex.EncodeToString(publicKey), time.Now().UTC()); err != nil {
					t.Fatal(err)
				}
				externalIdentityExec(t, store, `UPDATE principal_events SET signing_key_id=?
					WHERE principal_id=? AND revision=2`, keyID, e.RequesterPrincipalID)
			}},
			{"latest event missing", func(t *testing.T, store *Store, e identity.Evidence, _ ed25519.PrivateKey) {
				externalIdentityExec(t, store, `DELETE FROM principal_events
					WHERE principal_id=? AND revision=2`, e.RequesterPrincipalID)
			}},
		}
		for _, attack := range attacks {
			t.Run(attack.name, func(t *testing.T) {
				store, resolver, issuer, privateKey, now := identityStoreFixture(t)
				env, evidence := identityAttempt(t, resolver, issuer,
					"act_event_"+strings.ReplaceAll(attack.name, " ", "_"), now)
				if err := store.DisablePrincipal(context.Background(),
					evidence.RequesterPrincipalID, now.Add(time.Second)); err != nil {
					t.Fatal(err)
				}
				attack.mutate(t, store, evidence, privateKey)
				if err := store.RegisterIdentity(context.Background(), identityRegistryFixture(),
					now.Add(2*time.Second)); !errors.Is(err, identity.ErrPrincipalEvidenceCorrupt) {
					t.Fatalf("boot error = %v, want ErrPrincipalEvidenceCorrupt", err)
				}
				err := store.RecordAttemptAuthenticated(context.Background(), env,
					Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized, evidence)
				if !errors.Is(err, identity.ErrPrincipalEvidenceCorrupt) {
					t.Fatalf("start error = %v, want ErrPrincipalEvidenceCorrupt", err)
				}
			})
		}

	})

	t.Run("historical principal event key remains valid", func(t *testing.T) {
		store, resolver, issuer, _, now := identityStoreFixture(t)
		publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.RotateSigningKey(context.Background(), action.SigningKeyID(publicKey),
			hex.EncodeToString(publicKey), now.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		store.SetIdentitySigners(
			func(e identity.Evidence) identity.SignedEvidence { return identity.SignEvidence(privateKey, e) },
			func(e identity.PrincipalEvent) identity.SignedPrincipalEvent {
				return identity.SignPrincipalEvent(privateKey, e)
			},
		)
		if err := store.RegisterIdentity(context.Background(), identityRegistryFixture(), now.Add(2*time.Second)); err != nil {
			t.Fatalf("boot rejected historical principal event: %v", err)
		}
		env, evidence := identityAttempt(t, resolver, issuer, "act_historical_event", now)
		if err := store.RecordAttemptAuthenticated(context.Background(), env,
			Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized, evidence); err != nil {
			t.Fatalf("start rejected historical principal event: %v", err)
		}
	})
}

func phase1CreatedAt(t *testing.T, store *Store, principalID string) time.Time {
	t.Helper()
	var raw string
	if err := store.db.QueryRow(`SELECT created_at FROM principals WHERE principal_id=?`, principalID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	at, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func externalIdentityExec(t *testing.T, store *Store, query string, args ...any) {
	t.Helper()
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(store.path)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func TestIdentity_EvidenceAndActionCommitTogether(t *testing.T) {
	store, resolver, issuer, _, now := identityStoreFixture(t)
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(store.path)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TRIGGER identity_abort BEFORE INSERT ON identity_evidence_v2
		BEGIN SELECT RAISE(ABORT, 'identity evidence refused'); END`); err != nil {
		t.Fatal(err)
	}
	env, evidence := identityAttempt(t, resolver, issuer, "act_atomic", now)
	if err := store.RecordAttemptAuthenticated(context.Background(), env,
		Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized, evidence); err == nil {
		t.Fatal("RecordAttemptAuthenticated succeeded through abort trigger")
	}
	for _, table := range []string{"actions", "action_decisions", "identity_evidence_v2"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE action_id = ?`, env.ActionID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s rows = %d, want 0 after rollback", table, count)
		}
	}

	t.Run("approval birth", func(t *testing.T) {
		env, evidence := identityAttempt(t, resolver, issuer, "act_atomic_approval", now)
		env.Effect = action.Effect{Class: string(action.EffectWriteIrreversible)}
		bound, err := action.NewBoundApprovalRequest(env, `{}`, action.ApprovalContext{
			IntentPurpose: "identity atomicity", ToolCage: "test cage",
			Descriptor:    action.EffectDescriptor{Class: action.EffectWriteIrreversible},
			HasDescriptor: true, LawVersion: 1, LawDigest: "sha256:law",
			Rule: "require_approval", Now: now, TTL: time.Minute,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.CreateApprovalRequestAuthenticated(context.Background(), bound, evidence); err == nil {
			t.Fatal("approval birth succeeded through evidence abort trigger")
		}
		for _, table := range []string{"actions", "action_decisions", "identity_evidence_v2", "approvals"} {
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE action_id=?`, env.ActionID).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("%s rows = %d after approval rollback", table, count)
			}
		}
	})

	t.Run("mutating evidence signer", func(t *testing.T) {
		fresh, freshResolver, freshIssuer, priv, freshNow := identityStoreFixture(t)
		fresh.SetIdentitySigners(func(e identity.Evidence) identity.SignedEvidence {
			signed := identity.SignEvidence(priv, e)
			signed.Evidence.SubjectClaim = "changed-by-signer"
			return signed
		}, func(e identity.PrincipalEvent) identity.SignedPrincipalEvent {
			return identity.SignPrincipalEvent(priv, e)
		})
		env, evidence := identityAttempt(t, freshResolver, freshIssuer, "act_mutating_signer", freshNow)
		err := fresh.RecordAttemptAuthenticated(context.Background(), env,
			Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized, evidence)
		if !errors.Is(err, identity.ErrSignedObjectMutated) {
			t.Fatalf("mutating signer error = %v", err)
		}
		if _, err := fresh.Get(context.Background(), env.ActionID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("mutating signer left action: %v", err)
		}
	})

	t.Run("retired signer", func(t *testing.T) {
		fresh, freshResolver, freshIssuer, _, freshNow := identityStoreFixture(t)
		pub2, _, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		if err := fresh.RotateSigningKey(context.Background(), action.SigningKeyID(pub2),
			hex.EncodeToString(pub2), freshNow.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		env, evidence := identityAttempt(t, freshResolver, freshIssuer, "act_retired_signer", freshNow)
		err = fresh.RecordAttemptAuthenticated(context.Background(), env,
			Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized, evidence)
		if !errors.Is(err, identity.ErrSigningKeyRetired) {
			t.Fatalf("retired signer error = %v", err)
		}
	})

	t.Run("principal event transactions", func(t *testing.T) {
		t.Run("birth trigger", func(t *testing.T) {
			fresh, _, _, _ := emptyIdentityStoreFixture(t)
			externalIdentityExec(t, fresh, `CREATE TRIGGER principal_birth_abort
				BEFORE INSERT ON principal_events BEGIN SELECT RAISE(ABORT,'event refused'); END`)
			if err := fresh.RegisterIdentity(context.Background(), identityRegistryFixture(), now); err == nil {
				t.Fatal("principal birth succeeded through event abort trigger")
			}
			assertIdentityTableCount(t, fresh, "principals", 0)
			assertIdentityTableCount(t, fresh, "principal_events", 0)
			assertIdentityTableCount(t, fresh, "principal_bindings", 0)
		})

		t.Run("disable trigger", func(t *testing.T) {
			fresh, _, _, _, freshNow := identityStoreFixture(t)
			externalIdentityExec(t, fresh, `CREATE TRIGGER principal_disable_abort
				BEFORE INSERT ON principal_events WHEN NEW.kind='disabled'
				BEGIN SELECT RAISE(ABORT,'disable event refused'); END`)
			if err := fresh.DisablePrincipal(context.Background(), "principal_webhook", freshNow.Add(time.Second)); err == nil {
				t.Fatal("principal disable succeeded through event abort trigger")
			}
			var disabled sql.NullString
			var revision int
			if err := fresh.db.QueryRow(`SELECT disabled_at,revision FROM principals
				WHERE principal_id='principal_webhook'`).Scan(&disabled, &revision); err != nil {
				t.Fatal(err)
			}
			if disabled.Valid || revision != 1 {
				t.Fatalf("disable projection survived rollback: disabled=%v revision=%d", disabled, revision)
			}
			var events int
			if err := fresh.db.QueryRow(`SELECT COUNT(*) FROM principal_events
				WHERE principal_id='principal_webhook'`).Scan(&events); err != nil {
				t.Fatal(err)
			}
			if events != 1 {
				t.Fatalf("principal events = %d, want original birth only", events)
			}
		})

		t.Run("mutating principal signer", func(t *testing.T) {
			fresh, _, privateKey, freshNow := emptyIdentityStoreFixture(t)
			fresh.SetIdentitySigners(
				func(e identity.Evidence) identity.SignedEvidence { return identity.SignEvidence(privateKey, e) },
				func(e identity.PrincipalEvent) identity.SignedPrincipalEvent {
					signed := identity.SignPrincipalEvent(privateKey, e)
					signed.Event.Kind = "disabled"
					return signed
				},
			)
			if err := fresh.RegisterIdentity(context.Background(), identityRegistryFixture(), freshNow); !errors.Is(err, identity.ErrSignedObjectMutated) {
				t.Fatalf("mutating principal signer = %v", err)
			}
			assertIdentityTableCount(t, fresh, "principals", 0)
			assertIdentityTableCount(t, fresh, "principal_events", 0)
		})

		t.Run("retired principal signer", func(t *testing.T) {
			fresh, _, _, freshNow := emptyIdentityStoreFixture(t)
			publicKey, _, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			if err := fresh.RotateSigningKey(context.Background(), action.SigningKeyID(publicKey),
				hex.EncodeToString(publicKey), freshNow.Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			if err := fresh.RegisterIdentity(context.Background(), identityRegistryFixture(), freshNow.Add(2*time.Second)); !errors.Is(err, identity.ErrSigningKeyRetired) {
				t.Fatalf("retired principal signer = %v", err)
			}
			assertIdentityTableCount(t, fresh, "principals", 0)
			assertIdentityTableCount(t, fresh, "principal_events", 0)
		})
	})
}

func emptyIdentityStoreFixture(t *testing.T) (*Store, ed25519.PublicKey, ed25519.PrivateKey, time.Time) {
	t.Helper()
	now := time.Date(2026, 9, 21, 16, 0, 0, 0, time.UTC)
	store, err := Open(filepath.Join(t.TempDir(), "empty-identity.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutSigningKey(context.Background(), action.SigningKeyID(publicKey),
		hex.EncodeToString(publicKey), now); err != nil {
		t.Fatal(err)
	}
	store.SetIdentitySigners(
		func(e identity.Evidence) identity.SignedEvidence { return identity.SignEvidence(privateKey, e) },
		func(e identity.PrincipalEvent) identity.SignedPrincipalEvent {
			return identity.SignPrincipalEvent(privateKey, e)
		},
	)
	store.identityNow = func() time.Time { return now }
	return store, publicKey, privateKey, now
}

func assertIdentityTableCount(t *testing.T, store *Store, table string, want int) {
	t.Helper()
	var got int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s rows = %d, want %d", table, got, want)
	}
}

func TestIdentity_MigrationDoesNotUpgradeHistoricalClaims(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v13.db")
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(createStmt); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO action_schema(version) VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	for version := 1; version < 13; version++ {
		if err := migrateStep(db, migrations[version], migrationCopies[version], version); err != nil {
			t.Fatalf("migrate to %d: %v", version+1, err)
		}
	}
	legacyAt := time.Date(2026, 9, 19, 1, 2, 3, 0, time.UTC).Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO actions(action_id,schema_version,correlation_id,
		source_kind,source_protocol,source_channel,op_namespace,op_name,op_version,
		parameters_digest,effect_class,state,requested_at,principal_id)
		VALUES('act_legacy',1,'operator-looking','agent_brain','text','webhook',
		'tool','probe',1,'sha256:legacy','pure','DENIED',?,'principal_operator')`, legacyAt); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open migrated store: %v", err)
	}
	defer func() { _ = store.Close() }()
	if got, err := store.SchemaVersion(context.Background()); err != nil || got != 15 {
		t.Fatalf("SchemaVersion = %d, %v; want 15", got, err)
	}
	check, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = check.Close() }()
	var era int
	var canonical []byte
	if err := check.QueryRow(`SELECT identity_version, identity_canonical_evidence
		FROM actions WHERE action_id='act_legacy'`).Scan(&era, &canonical); err != nil {
		t.Fatal(err)
	}
	if era != 0 || len(canonical) != 0 {
		t.Fatalf("legacy identity snapshot = era %d bytes %d, want zero/empty", era, len(canonical))
	}
	for _, table := range []string{
		"principals", "principal_bindings", "identity_evidence_v2", "principal_events",
	} {
		var count int
		if err := check.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s historical rows = %d, want 0", table, count)
		}
	}

	t.Run("migration rollback leaves no partial v14 shape", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "interrupted-v13.db")
		db := openSchema13Fixture(t, path)
		defer func() { _ = db.Close() }()
		if _, err := db.Exec(`CREATE TRIGGER stop_identity_migration
			BEFORE UPDATE OF version ON action_schema WHEN NEW.version=14
			BEGIN SELECT RAISE(ABORT,'stop v14'); END`); err != nil {
			t.Fatal(err)
		}
		if err := migrateStep(db, migrations[13], migrationCopies[13], 13); err == nil {
			t.Fatal("interrupted migration unexpectedly committed")
		}
		var version int
		if err := db.QueryRow(`SELECT version FROM action_schema`).Scan(&version); err != nil || version != 13 {
			t.Fatalf("schema after interrupted migration = %d, %v; want 13", version, err)
		}
		for _, table := range []string{"principals", "principal_bindings", "principal_events", "identity_evidence_v2"} {
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("partial migration left table %s", table)
			}
		}
		if hasSQLiteColumn(t, db, "actions", "identity_version") ||
			hasSQLiteColumn(t, db, "receipts", "identity_status") {
			t.Fatal("interrupted migration left v14 columns")
		}
	})

	t.Run("all evidence foreign keys reject orphans", func(t *testing.T) {
		attacks := []struct {
			column string
			value  string
		}{
			{"action_id", "act_missing"},
			{"requester_principal_id", "principal_missing"},
			{"actor_principal_id", "principal_missing"},
			{"responsible_principal_id", "principal_missing"},
			{"binding_id", "binding_missing"},
			{"signing_key_id", "ed25519:missing"},
		}
		for _, attack := range attacks {
			t.Run(attack.column, func(t *testing.T) {
				store, resolver, issuer, _, now := identityStoreFixture(t)
				env, evidence := identityAttempt(t, resolver, issuer, "act_fk_"+attack.column, now)
				if err := store.RecordAttemptAuthenticated(context.Background(), env,
					Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized, evidence); err != nil {
					t.Fatal(err)
				}
				db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(store.path)))
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = db.Close() }()
				query := `UPDATE identity_evidence_v2 SET ` + attack.column + `=? WHERE action_id=?` // #nosec G202 -- column comes from the closed test table above
				if _, err := db.Exec(query, attack.value, env.ActionID); err == nil {
					t.Fatalf("orphan %s update succeeded", attack.column)
				}
			})
		}

		t.Run("retired K1 snapshot verifies under K2 recovery", func(t *testing.T) {
			store, resolver, issuer, _, now := identityStoreFixture(t)
			env, evidence := identityAttempt(t, resolver, issuer, "act_recover_retired_k1", now)
			if err := store.RecordAttemptAuthenticated(context.Background(), env,
				Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized, evidence); err != nil {
				t.Fatal(err)
			}
			publicK2, privateK2, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.RotateSigningKey(context.Background(), action.SigningKeyID(publicK2),
				hex.EncodeToString(publicK2), now.Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			store.SetReceiptSealer(func(receipt action.Receipt) action.Receipt {
				return action.SignReceipt(privateK2, receipt)
			})
			externalIdentityExec(t, store, `DELETE FROM identity_evidence_v2 WHERE action_id=?`, env.ActionID)
			if _, err := store.RecoverPreviousLife(context.Background()); err != nil {
				t.Fatal(err)
			}
			assertVerifiedV3Receipt(t, store, env.ActionID, publicK2, evidence)
		})
	})

	t.Run("boot registration is idempotent and conflict preserving", func(t *testing.T) {
		store, _, _, _, now := identityStoreFixture(t)
		var eventsBefore int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM principal_events`).Scan(&eventsBefore); err != nil {
			t.Fatal(err)
		}
		if err := store.RegisterIdentity(context.Background(), identityRegistryFixture(), now.Add(time.Second)); err != nil {
			t.Fatalf("equivalent second boot: %v", err)
		}
		var eventsAfter int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM principal_events`).Scan(&eventsAfter); err != nil {
			t.Fatal(err)
		}
		if eventsAfter != eventsBefore {
			t.Fatalf("equivalent boot appended events: before=%d after=%d", eventsBefore, eventsAfter)
		}
		conflict := identityRegistryFixture()
		conflict.Bindings[0].VerifiedSubject = "different_authenticated_subject"
		if err := store.RegisterIdentity(context.Background(), conflict, now.Add(2*time.Second)); err == nil {
			t.Fatal("conflicting boot overwrote a binding")
		}
		var verified string
		if err := store.db.QueryRow(`SELECT verified_subject FROM principal_bindings
			WHERE binding_id='binding_webhook'`).Scan(&verified); err != nil {
			t.Fatal(err)
		}
		if verified != "shared_webhook_credential" {
			t.Fatalf("conflict changed verified subject to %q", verified)
		}
	})

	t.Run("v2 identity terminal receipts are v3", func(t *testing.T) {
		cases := []struct {
			name    string
			state   action.State
			outcome string
			rule    string
		}{
			{"deny", action.StateDenied, "deny", "policy_denied"},
			{"shadow", action.StateShadowed, "shadow", "shadow"},
			{"unknown tool", action.StateDenied, "deny", "unknown_tool"},
			{"undeclared effect", action.StateDenied, "deny", "undeclared_effect"},
		}
		for _, tt := range cases {
			t.Run(tt.name, func(t *testing.T) {
				store, resolver, issuer, privateKey, now := identityStoreFixture(t)
				env, evidence := identityAttempt(t, resolver, issuer,
					"act_terminal_"+strings.ReplaceAll(tt.name, " ", "_"), now)
				if err := store.RecordAttemptAuthenticated(context.Background(), env,
					Decision{Outcome: tt.outcome, Rule: tt.rule}, tt.state, evidence); err != nil {
					t.Fatal(err)
				}
				assertVerifiedV3Receipt(t, store, env.ActionID,
					privateKey.Public().(ed25519.PublicKey), evidence)
			})
		}

		t.Run("finish", func(t *testing.T) {
			store, resolver, issuer, privateKey, now := identityStoreFixture(t)
			env, evidence := identityAttempt(t, resolver, issuer, "act_terminal_finish", now)
			if err := store.RecordAttemptAuthenticated(context.Background(), env,
				Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized, evidence); err != nil {
				t.Fatal(err)
			}
			if err := store.Finish(context.Background(), env.ActionID,
				action.StateSucceeded, now.Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			assertVerifiedV3Receipt(t, store, env.ActionID,
				privateKey.Public().(ed25519.PublicKey), evidence)
		})

		t.Run("rejection", func(t *testing.T) {
			store, resolver, issuer, privateKey, now := identityStoreFixture(t)
			env, evidence := identityAttempt(t, resolver, issuer, "act_terminal_rejection", now)
			env.Effect = action.Effect{Class: string(action.EffectWriteIrreversible)}
			bound, err := action.NewBoundApprovalRequest(env, `{}`, action.ApprovalContext{
				IntentPurpose: "receipt rejection", ToolCage: "identity test",
				Descriptor:    action.EffectDescriptor{Class: action.EffectWriteIrreversible},
				HasDescriptor: true, LawVersion: 1, LawDigest: "sha256:law",
				Rule: "require_approval", Now: now, TTL: time.Minute,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := store.CreateApprovalRequestAuthenticated(context.Background(), bound, evidence); err != nil {
				t.Fatal(err)
			}
			approval := bound.Approval()
			decisionEnv, attemptIdentity := operatorDecisionEnv("reject", approval.ApprovalID)
			if _, err := store.DecideApprovalUnderLaw(context.Background(), approval.ApprovalID,
				action.DecisionRejected, now.Add(time.Second), decisionEnv, attemptIdentity, "",
				PolicyPin{Version: 1, Digest: "sha256:law"}); err != nil {
				t.Fatal(err)
			}
			assertVerifiedV3Receipt(t, store, env.ActionID,
				privateKey.Public().(ed25519.PublicKey), evidence)
		})
	})

	t.Run("recovery uses signed birth snapshot", func(t *testing.T) {
		t.Run("full evidence missing", func(t *testing.T) {
			store, resolver, issuer, privateKey, now := identityStoreFixture(t)
			env, evidence := identityAttempt(t, resolver, issuer, "act_recover_missing_full", now)
			if err := store.RecordAttemptAuthenticated(context.Background(), env,
				Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized, evidence); err != nil {
				t.Fatal(err)
			}
			externalIdentityExec(t, store, `DELETE FROM identity_evidence_v2 WHERE action_id=?`, env.ActionID)
			if _, err := store.RecoverPreviousLife(context.Background()); err != nil {
				t.Fatal(err)
			}
			assertVerifiedV3Receipt(t, store, env.ActionID,
				privateKey.Public().(ed25519.PublicKey), evidence)
			if _, err := store.RecoverPreviousLife(context.Background()); err != nil {
				t.Fatal(err)
			}
			receipts, err := store.ReceiptsByAction(context.Background(), env.ActionID)
			if err != nil || len(receipts) != 1 {
				t.Fatalf("recovery receipts = %d, %v; want exactly one", len(receipts), err)
			}
		})

		attacks := []struct {
			name   string
			column string
			value  any
		}{
			{"canonical", "identity_canonical_evidence", []byte(`{"broken":true}`)},
			{"digest", "identity_evidence_digest", "sha256:00"},
			{"key", "identity_signing_key_id", "ed25519:unknown"},
			{"signature", "identity_signature", "00"},
			{"era", "identity_version", 7},
			{"inverse downgrade", "identity_version", 0},
		}
		for _, attack := range attacks {
			t.Run(attack.name, func(t *testing.T) {
				store, resolver, issuer, privateKey, now := identityStoreFixture(t)
				env, evidence := identityAttempt(t, resolver, issuer,
					"act_recover_"+strings.ReplaceAll(attack.name, " ", "_"), now)
				if err := store.RecordAttemptAuthenticated(context.Background(), env,
					Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized, evidence); err != nil {
					t.Fatal(err)
				}
				externalIdentityExec(t, store, `UPDATE actions SET `+attack.column+`=? WHERE action_id=?`, attack.value, env.ActionID)
				snapshot := readIdentitySnapshot(t, store, env.ActionID)
				if _, err := store.RecoverPreviousLife(context.Background()); err != nil {
					t.Fatal(err)
				}
				receipts, err := store.ReceiptsByAction(context.Background(), env.ActionID)
				if err != nil || len(receipts) != 1 {
					t.Fatalf("corrupt recovery receipts = %+v, %v", receipts, err)
				}
				r := receipts[0]
				if r.SchemaVersion != 3 || r.IdentityStatus != "corrupt" ||
					r.IdentityEvidenceDigest != "" || r.RequesterPrincipalID != "" ||
					r.ActorPrincipalID != "" || r.ResponsiblePrincipalID != "" ||
					r.IdentitySnapshotDigest != identity.SnapshotDigest(snapshot) {
					t.Fatalf("corrupt recovery attribution = %+v", r)
				}
				if err := action.VerifyReceiptSignature(privateKey.Public().(ed25519.PublicKey), r); err != nil {
					t.Fatalf("corrupt v3 receipt signature: %v", err)
				}
			})
		}
	})

	t.Run("coordinator crash processes recover once", func(t *testing.T) {
		for _, mode := range []string{"immediate", "approved"} {
			t.Run(mode, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), mode+".db")
				marker := filepath.Join(t.TempDir(), mode+".dispatched")
				publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
				if err != nil {
					t.Fatal(err)
				}
				// #nosec G204 -- the crash mold deliberately re-executes this test binary.
				cmd := exec.Command(os.Args[0], "-test.run=^TestIdentityCrashProcess$")
				cmd.Env = append(os.Environ(),
					"KORVUN_IDENTITY_CRASH_MODE="+mode,
					"KORVUN_IDENTITY_CRASH_DB="+path,
					"KORVUN_IDENTITY_CRASH_MARKER="+marker,
					"KORVUN_IDENTITY_CRASH_KEY="+hex.EncodeToString(privateKey),
				)
				err = cmd.Run()
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() != 23 {
					t.Fatalf("crash child error = %v, want exit 23", err)
				}
				// #nosec G304 -- marker is a test-owned file under t.TempDir.
				if raw, err := os.ReadFile(marker); err != nil || string(raw) != "dispatched" {
					t.Fatalf("dispatch marker = %q, %v", raw, err)
				}
				store, err := Open(path)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = store.Close() }()
				store.SetReceiptSealer(func(receipt action.Receipt) action.Receipt {
					return action.SignReceipt(privateKey, receipt)
				})
				store.SetIdentitySigners(
					func(e identity.Evidence) identity.SignedEvidence { return identity.SignEvidence(privateKey, e) },
					func(e identity.PrincipalEvent) identity.SignedPrincipalEvent {
						return identity.SignPrincipalEvent(privateKey, e)
					},
				)
				var actionID string
				if err := store.db.QueryRow(`SELECT action_id FROM actions`).Scan(&actionID); err != nil {
					t.Fatal(err)
				}
				if mode == "approved" {
					var params string
					if err := store.db.QueryRow(`SELECT canonical_params FROM approvals WHERE action_id=?`, actionID).Scan(&params); err != nil {
						t.Fatal(err)
					}
					if params != "" {
						t.Fatalf("approved crash retained claimed params %q", params)
					}
				}
				snapshot := readIdentitySnapshot(t, store, actionID)
				evidence, err := identity.ParseCanonicalEvidence(snapshot.Canonical)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := store.RecoverPreviousLife(context.Background()); err != nil {
					t.Fatal(err)
				}
				record, err := store.Get(context.Background(), actionID)
				if err != nil || record.State != action.StateOutcomeUnknown {
					t.Fatalf("recovered state = %v, %v", record.State, err)
				}
				assertVerifiedV3Receipt(t, store, actionID, publicKey, evidence)
				if _, err := store.RecoverPreviousLife(context.Background()); err != nil {
					t.Fatal(err)
				}
				receipts, err := store.ReceiptsByAction(context.Background(), actionID)
				if err != nil || len(receipts) != 1 {
					t.Fatalf("repeat recovery receipts = %d, %v", len(receipts), err)
				}
			})
		}
	})

	t.Run("receipt attribution survives action pruning and reopen", func(t *testing.T) {
		store, resolver, issuer, privateKey, now := identityStoreFixture(t)
		env, evidence := identityAttempt(t, resolver, issuer, "act_pruned_identity", now)
		if err := store.RecordAttemptAuthenticated(context.Background(), env,
			Decision{Outcome: "deny", Rule: "policy_denied"}, action.StateDenied, evidence); err != nil {
			t.Fatal(err)
		}
		store.capRows = 0
		if removed, err := store.Prune(context.Background()); err != nil || removed != 1 {
			t.Fatalf("Prune = %d, %v; want 1", removed, err)
		}
		if _, err := store.Get(context.Background(), env.ActionID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("pruned action read = %v", err)
		}
		assertVerifiedV3Receipt(t, store, env.ActionID,
			privateKey.Public().(ed25519.PublicKey), evidence)
		reopened, err := Open(store.path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = reopened.Close() }()
		assertVerifiedV3Receipt(t, reopened, env.ActionID,
			privateKey.Public().(ed25519.PublicKey), evidence)
	})
}

type identityCrashTool struct {
	marker string
}

func (*identityCrashTool) Name() string        { return "identity_crash" }
func (*identityCrashTool) Description() string { return "identity crash probe" }
func (c *identityCrashTool) Execute(context.Context, string) (string, error) {
	if err := os.WriteFile(c.marker, []byte("dispatched"), 0o600); err != nil {
		os.Exit(22)
	}
	os.Exit(23)
	return "", nil
}

type identityCrashRecorder struct {
	store *Store
}

func (r identityCrashRecorder) RecordAttempt(ctx context.Context, env action.Envelope,
	outcome, rule string, state action.State,
) error {
	return r.store.RecordAttempt(ctx, env, Decision{Outcome: outcome, Rule: rule}, state)
}

func (r identityCrashRecorder) RecordAttemptAuthenticated(ctx context.Context,
	env action.Envelope, outcome, rule string, state action.State, evidence identity.Evidence,
) error {
	return r.store.RecordAttemptAuthenticated(ctx, env,
		Decision{Outcome: outcome, Rule: rule}, state, evidence)
}

func (r identityCrashRecorder) Finish(ctx context.Context, actionID string,
	state action.State, at time.Time,
) error {
	return r.store.Finish(ctx, actionID, state, at)
}

type identityApprovedStore struct {
	store  *Store
	digest string
}

func (s identityApprovedStore) ReadApproval(ctx context.Context, approvalID string) (action.Approval, error) {
	approval, _, err := s.store.GetApproval(ctx, approvalID)
	return approval, err
}

func (s identityApprovedStore) ReadActionState(ctx context.Context, actionID string) (action.State, error) {
	record, err := s.store.Get(ctx, actionID)
	return record.State, err
}

func (s identityApprovedStore) Claim(ctx context.Context, approvalID string,
	seen *action.Approval,
) ([]byte, action.Operation, error) {
	return s.store.ClaimApprovalParamsUnderDigest(ctx, approvalID,
		&PolicyPin{Version: 1, Digest: "sha256:law"}, s.digest, seen)
}

func (s identityApprovedStore) Close(ctx context.Context, actionID string,
	state action.State, at time.Time, digest string,
) error {
	return s.store.FinishWithResult(ctx, actionID, state, at, digest)
}

func (s identityApprovedStore) ReadReceipt(ctx context.Context, approvalID string) (string, error) {
	approval, _, err := s.store.GetApproval(ctx, approvalID)
	return approval.DecisionReceiptID, err
}

func TestIdentityCrashProcess(t *testing.T) {
	mode := os.Getenv("KORVUN_IDENTITY_CRASH_MODE")
	if mode == "" {
		t.Skip("crash-process helper")
	}
	privateRaw, err := hex.DecodeString(os.Getenv("KORVUN_IDENTITY_CRASH_KEY"))
	if err != nil || len(privateRaw) != ed25519.PrivateKeySize {
		t.Fatalf("private key fixture: %v", err)
	}
	privateKey := ed25519.PrivateKey(privateRaw)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	now := time.Date(2026, 9, 21, 20, 0, 0, 0, time.UTC)
	store, err := Open(os.Getenv("KORVUN_IDENTITY_CRASH_DB"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutSigningKey(context.Background(), action.SigningKeyID(publicKey),
		hex.EncodeToString(publicKey), now); err != nil {
		t.Fatal(err)
	}
	store.SetReceiptSealer(func(receipt action.Receipt) action.Receipt {
		return action.SignReceipt(privateKey, receipt)
	})
	store.SetIdentitySigners(
		func(e identity.Evidence) identity.SignedEvidence { return identity.SignEvidence(privateKey, e) },
		func(e identity.PrincipalEvent) identity.SignedPrincipalEvent {
			return identity.SignPrincipalEvent(privateKey, e)
		},
	)
	store.identityNow = func() time.Time { return now }
	registry := identityRegistryFixture()
	if err := store.RegisterIdentity(context.Background(), registry, now); err != nil {
		t.Fatal(err)
	}
	resolver, err := identity.NewResolver(registry, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := resolver.NewIssuer(identity.IssuerConfig{
		BindingID: "binding_webhook", Method: "bearer",
		CredentialClass: "shared_secret", TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	crash := &identityCrashTool{marker: os.Getenv("KORVUN_IDENTITY_CRASH_MARKER")}
	registryTools := tool.Registry{"identity_crash": crash}
	if mode == "immediate" {
		exec := actionexecutor.NewCoordinator(registryTools, time.Second,
			func() time.Time { return now }, actionexecutor.CoordinatorConfig{
				BrainName: "alpha", Recorder: identityCrashRecorder{store: store},
				PrincipalResolver: resolver,
			})
		in := envelope.New("webhook", envelope.Inbound, envelope.Participant{ID: "subject"})
		in.ID = "request-crash-immediate"
		ingress, err := issuer.Issue(in.ID, in.Sender.ID)
		if err != nil {
			t.Fatal(err)
		}
		_, plan, err := exec.SelectTools(context.Background(), in.Channel)
		if err != nil {
			t.Fatal(err)
		}
		req, err := exec.Prepare(actionexecutor.Submission{
			Inbound: in, Ingress: ingress, Lane: "text", Name: "identity_crash",
			Arguments: `{}`, Plan: plan,
		})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = exec.Submit(context.Background(), req)
		t.Fatal("immediate crash tool returned")
	}
	if mode != "approved" {
		t.Fatalf("unknown crash mode %q", mode)
	}
	env, evidence := identityAttempt(t, resolver, issuer, "act_crash_approved", now)
	env.Operation = action.Operation{Namespace: "tool", Name: "identity_crash", Version: 1}
	env.Effect = action.Effect{Class: string(action.EffectWriteIrreversible)}
	env.ParametersDigest = action.Digest(env.Operation, `{}`)
	bound, err := action.NewBoundApprovalRequest(env, `{}`, action.ApprovalContext{
		IntentPurpose: "approved crash", ToolCage: "identity crash test",
		Descriptor:    action.EffectDescriptor{Class: action.EffectWriteIrreversible},
		HasDescriptor: true, LawVersion: 1, LawDigest: "sha256:law",
		Rule: "require_approval", Now: now, TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateApprovalRequestAuthenticated(context.Background(), bound, evidence); err != nil {
		t.Fatal(err)
	}
	approval := bound.Approval()
	decisionEnv, attemptIdentity := operatorDecisionEnv("approve", approval.ApprovalID)
	if _, err := store.DecideApprovalUnderLaw(context.Background(), approval.ApprovalID,
		action.DecisionApproved, now.Add(time.Second), decisionEnv, attemptIdentity, "",
		PolicyPin{Version: 1, Digest: "sha256:law"}); err != nil {
		t.Fatal(err)
	}
	exec := actionexecutor.New(registryTools, time.Second, func() time.Time { return now.Add(2 * time.Second) })
	_, _ = exec.ResumeApproved(context.Background(), identityApprovedStore{
		store: store, digest: approval.ActionDigest,
	}, approval.ApprovalID)
	t.Fatal("approved crash tool returned")
}

func openSchema13Fixture(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(createStmt); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO action_schema(version) VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	for version := 1; version < 13; version++ {
		if err := migrateStep(db, migrations[version], migrationCopies[version], version); err != nil {
			t.Fatalf("migrate to %d: %v", version+1, err)
		}
	}
	return db
}

func hasSQLiteColumn(t *testing.T, db *sql.DB, table, want string) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		if name == want {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return false
}

func assertVerifiedV3Receipt(t *testing.T, store *Store, actionID string,
	publicKey ed25519.PublicKey, evidence identity.Evidence,
) {
	t.Helper()
	receipts, err := store.ReceiptsByAction(context.Background(), actionID)
	if err != nil || len(receipts) != 1 {
		t.Fatalf("ReceiptsByAction(%s) = %+v, %v", actionID, receipts, err)
	}
	receipt := receipts[0]
	if receipt.SchemaVersion != 3 || receipt.IdentityStatus != "verified" ||
		receipt.RequesterPrincipalID != evidence.RequesterPrincipalID ||
		receipt.ActorPrincipalID != evidence.ActorPrincipalID ||
		receipt.ResponsiblePrincipalID != evidence.ResponsiblePrincipalID ||
		receipt.IdentityEvidenceDigest == "" || receipt.IdentitySnapshotDigest == "" {
		t.Fatalf("verified v3 receipt = %+v", receipt)
	}
	if err := action.VerifyReceiptSignature(publicKey, receipt); err != nil {
		t.Fatalf("VerifyReceiptSignature: %v", err)
	}
	parsed, err := action.ParseCanonicalReceipt(action.CanonicalReceipt(receipt))
	if err != nil || parsed != receiptWithoutSeal(receipt) {
		t.Fatalf("v3 canonical round trip = %+v, %v; want %+v", parsed, err, receiptWithoutSeal(receipt))
	}
}

func receiptWithoutSeal(receipt action.Receipt) action.Receipt {
	receipt.ReceiptHash = ""
	receipt.SigningKeyID = ""
	receipt.Signature = ""
	return receipt
}

func readIdentitySnapshot(t *testing.T, store *Store, actionID string) identity.BirthSnapshot {
	t.Helper()
	var snapshot identity.BirthSnapshot
	if err := store.db.QueryRow(`SELECT identity_version,identity_evidence_digest,
		identity_canonical_evidence,identity_signing_key_id,identity_signature
		FROM actions WHERE action_id=?`, actionID).Scan(&snapshot.Version, &snapshot.Digest,
		&snapshot.Canonical, &snapshot.SigningKeyID, &snapshot.Signature); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

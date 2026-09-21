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
	"github.com/Sebastian197/korvun/internal/identity"
)

func TestAuthority_StartAndParkRejectIncompleteInputsWithoutWrites(t *testing.T) {
	boom := errors.New("resolver failed")
	t.Run("start actor derives from signed evidence", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		req := authorityStartRequest(f, "", f.now)
		req.ActorPrincipalID = ""
		started, err := f.store.StartAuthorization(context.Background(), req)
		if err != nil || started.Evidence == nil ||
			started.Evidence.ActorPrincipalID != "principal_brain_alpha" {
			t.Fatalf("start = %#v, error = %v", started, err)
		}
	})
	t.Run("start resolver error", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		req := authorityStartRequest(f, "", f.now)
		req.ResolveEvidence = func(string) (identity.Evidence, error) { return identity.Evidence{}, boom }
		if _, err := f.store.StartAuthorization(context.Background(), req); !errors.Is(err, boom) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("start actor mismatch", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		req := authorityStartRequest(f, "", f.now)
		requestID := "request-start-mismatch"
		ingress, err := f.issuer.Issue(requestID, "claimed")
		if err != nil {
			t.Fatal(err)
		}
		req.ResolveEvidence = func(actionID string) (identity.Evidence, error) {
			return f.resolver.Resolve(ingress, identity.ResolveRequest{ActionID: actionID, RequestID: requestID, Channel: "webhook", Brain: "alpha"})
		}
		req.ActorPrincipalID = "principal_other"
		if _, err := f.store.StartAuthorization(context.Background(), req); !errors.Is(err, ErrIssuerMismatch) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("start missing binding", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		req := authorityStartRequest(f, "", f.now)
		req.Channel = "missing"
		if _, err := f.store.StartAuthorization(context.Background(), req); !errors.Is(err, ErrAuthorityMissing) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("start operation outside intent", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		req := authorityStartRequest(f, "", f.now)
		req.Operation.Name = "other"
		if _, err := f.store.StartAuthorization(context.Background(), req); !errors.Is(err, action.ErrAttenuationViolated) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("start claim names no approval", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		req := authorityStartRequest(f, "", f.now)
		req.ApprovalID = "apr3_missing"
		if _, err := f.store.StartAuthorization(context.Background(), req); !errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
			t.Fatalf("error = %v", err)
		}
		if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM authorization_starts`); n != 0 {
			t.Fatalf("rolled-back starts = %d", n)
		}
	})
	t.Run("park missing resolver", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		if _, err := f.store.ParkAuthorization(context.Background(), AuthorityPendingRequest{}); !errors.Is(err, identity.ErrIdentityEvidenceMissing) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("park resolver error", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		if _, err := f.store.ParkAuthorization(context.Background(), AuthorityPendingRequest{
			ResolveEvidence: func(string) (identity.Evidence, error) { return identity.Evidence{}, boom },
		}); !errors.Is(err, boom) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("park actor mismatch", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		requestID := "request-park-mismatch"
		ingress, err := f.issuer.Issue(requestID, "claimed")
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.store.ParkAuthorization(context.Background(), AuthorityPendingRequest{
			ActorPrincipalID: "principal_other", CorrelationID: requestID,
			ResolveEvidence: func(actionID string) (identity.Evidence, error) {
				return f.resolver.Resolve(ingress, identity.ResolveRequest{ActionID: actionID, RequestID: requestID, Channel: "webhook", Brain: "alpha"})
			},
		})
		if !errors.Is(err, ErrIssuerMismatch) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("park exhausted", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 1)
		if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now)); err != nil {
			t.Fatal(err)
		}
		if _, err := parkStrictAuthorityFixtureErr(f); !errors.Is(err, ErrBudgetExhausted) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("park oversized arguments", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		requestID := "request-park-large"
		ingress, err := f.issuer.Issue(requestID, "claimed")
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.store.ParkAuthorization(context.Background(), AuthorityPendingRequest{
			ActorPrincipalID: f.root.SubjectPrincipalID, CorrelationID: requestID,
			Channel: "webhook", Operation: action.Operation{Namespace: "tool", Name: "probe", Version: 1},
			Arguments: strings.Repeat("x", maxApprovalParamsBytes+1), EffectClass: action.EffectWriteReversible,
			At: f.now, ApprovalContext: action.ApprovalContext{
				ToolCage: "probe", Descriptor: action.EffectDescriptor{Class: action.EffectWriteReversible},
				HasDescriptor: true, LawVersion: 1, LawDigest: "sha256:authority-law", TTL: time.Minute,
			},
			ResolveEvidence: func(actionID string) (identity.Evidence, error) {
				return f.resolver.Resolve(ingress, identity.ResolveRequest{ActionID: actionID, RequestID: requestID, Channel: "webhook", Brain: "alpha"})
			},
		})
		if err == nil || !strings.Contains(err.Error(), "params exceed") {
			t.Fatalf("error = %v", err)
		}
	})
}

func parkStrictAuthorityFixtureErr(f authoritySQLiteFixture) (AuthorityPendingResult, error) {
	const requestID = "request-strict-pending-error"
	ingress, err := f.issuer.Issue(requestID, "verified-subject")
	if err != nil {
		return AuthorityPendingResult{}, err
	}
	return f.store.ParkAuthorization(context.Background(), AuthorityPendingRequest{
		ActorPrincipalID: f.root.SubjectPrincipalID, CorrelationID: requestID,
		SourceProtocol: "native", Channel: "webhook",
		Operation: action.Operation{Namespace: "tool", Name: "probe", Version: 1}, Arguments: `{}`,
		EffectClass: action.EffectWriteReversible, At: f.now,
		ApprovalContext: action.ApprovalContext{
			ToolCage: "probe", Descriptor: action.EffectDescriptor{Class: action.EffectWriteReversible},
			HasDescriptor: true, LawVersion: 1, LawDigest: "sha256:authority-law", TTL: time.Minute,
		},
		ResolveEvidence: func(actionID string) (identity.Evidence, error) {
			return f.resolver.Resolve(ingress, identity.ResolveRequest{ActionID: actionID, RequestID: requestID, Channel: "webhook", Brain: "alpha"})
		},
	})
}

func TestAuthority_BudgetLedgerFailureBoundaries(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 3)
	ctx := context.Background()
	tx, err := f.store.beginAuthorityWrite(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()

	unlimitedAccount := budgetAccountID(f.intent.ProfileID, "test", "per-op-only")
	if err := f.store.ensureBudgetAccountTx(ctx, tx, f.intent.ProfileID, "test", "per-op-only",
		action.IntentBudgetV2{PerOperation: map[string]int64{"tool/probe@1": 1}}, f.now); err != nil {
		t.Fatal(err)
	}
	remaining, err := f.store.debitAccountTx(ctx, tx, "act_budget_one", unlimitedAccount, "tool/probe@1", f.now)
	if err != nil || remaining == nil || *remaining != 1 {
		t.Fatalf("per-op first debit = %v, %v", remaining, err)
	}
	if _, err := f.store.debitAccountTx(ctx, tx, "act_budget_two", unlimitedAccount, "tool/probe@1", f.now); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("per-op exhaustion = %v", err)
	}

	total := int64(1)
	totalAccount := budgetAccountID(f.intent.ProfileID, "test", "total-only")
	if err := f.store.ensureBudgetAccountTx(ctx, tx, f.intent.ProfileID, "test", "total-only",
		action.IntentBudgetV2{Total: &total}, f.now); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.debitAccountTx(ctx, tx, "act_total_one", totalAccount, "tool/probe@1", f.now); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.debitAccountTx(ctx, tx, "act_total_two", totalAccount, "tool/probe@1", f.now); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("total exhaustion = %v", err)
	}

	badAccount := budgetAccountID(f.intent.ProfileID, "test", "bad-json")
	if _, err := tx.Exec(`INSERT INTO budget_accounts(account_id,profile_id,scope_kind,stable_scope_id,max_total,per_operation,created_at)
		VALUES(?,?,?,?,?,?,?)`, badAccount, f.intent.ProfileID, "test", "bad-json", nil, "{", f.now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.debitAccountTx(ctx, tx, "act_bad_json", badAccount, "tool/probe@1", f.now); !errors.Is(err, ErrBudgetEvidenceCorrupt) {
		t.Fatalf("bad budget json = %v", err)
	}
	if err := f.store.verifyDebitTailTx(ctx, tx, "missing", "*", 1, 0, "tail"); !errors.Is(err, ErrBudgetEvidenceCorrupt) {
		t.Fatalf("impossible zero sequence = %v", err)
	}
	if err := f.store.verifyDebitTailTx(ctx, tx, "missing", "*", 1, 1, "tail"); !errors.Is(err, ErrBudgetEvidenceCorrupt) {
		t.Fatalf("missing debit tail = %v", err)
	}
	if err := f.store.verifyDebitTailTx(ctx, tx, unlimitedAccount, "tool/probe@1", 2, 1, "wrong"); !errors.Is(err, ErrBudgetEvidenceCorrupt) {
		t.Fatalf("mismatched debit tail = %v", err)
	}
}

func TestAuthority_SignerAndSnapshotFailureBoundaries(t *testing.T) {
	t.Run("missing signer", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		f.store.SetAuthoritySigner(nil)
		if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now)); err == nil || !strings.Contains(err.Error(), "signer unavailable") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("mutating signer", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		f.store.SetAuthoritySigner(func(domain string, canonical []byte) action.AuthoritySignature {
			return action.SignAuthorityBytes(f.private, domain+".wrong", canonical)
		})
		if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now)); err == nil {
			t.Fatal("invalid authority signature was accepted")
		}
	})
	t.Run("snapshot identity mismatch", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		tx, err := f.store.beginAuthorityWrite(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback() }()
		remaining := int64(1)
		snapshot := action.AuthorizationSnapshotV1{
			Kind: action.AuthorizationSnapshotStart, ActionID: "act_snapshot",
			RequesterPrincipalID: "requester", ActorPrincipalID: "actor", IdentityEvidenceDigest: "sha256:a",
			IntentID: "intent", IntentVersion: 1, IntentDigest: "sha256:intent", IntentPurpose: "purpose",
			PrincipalChain: []string{"actor"}, BudgetKind: action.AuthorizationBudgetFinite,
			BudgetRemaining: &remaining, RecordedAt: f.now,
		}
		if err := f.store.insertAuthorizationSnapshotTx(context.Background(), tx, snapshot, "sha256:b"); !errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestAuthority_MutationDoorsRejectMalformedActorsAndBudgets(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 5)
	ctx := context.Background()
	malformed := f.root
	malformed.GrantID = ""
	if err := f.store.IssueAuthority(ctx, malformed, "", f.now); !errors.Is(err, action.ErrAuthorityMalformed) {
		t.Fatalf("malformed issue = %v", err)
	}
	wrongIssuer := f.root
	wrongIssuer.GrantID = "grant_wrong_issuer"
	wrongIssuer.IssuerPrincipalID = "principal_forged"
	wrongIssueAct := authorityActorAct(t, f.store, f.resolver, f.issuer, "issue", wrongIssuer.CanonicalBytes(), f.now.Add(time.Second))
	if err := f.store.IssueAuthority(ctx, wrongIssuer, wrongIssueAct, f.now.Add(time.Second)); !errors.Is(err, ErrIssuerMismatch) {
		t.Fatalf("wrong issue actor = %v", err)
	}

	tooMuch := f.root
	tooMuch.GrantID = "grant_too_much"
	tooMuch.IssuerPrincipalID, tooMuch.SubjectPrincipalID = f.root.SubjectPrincipalID, "principal_child"
	tooMuch.ParentGrantID, tooMuch.ParentGrantVersion = f.root.GrantID, f.root.Version
	tooMuch.DelegationDepthRemaining--
	maximum := int64(6)
	tooMuch.Budget = action.IntentBudgetV2{Total: &maximum, PerOperation: map[string]int64{"tool/probe@1": maximum}}
	delegateAct := authorityActorAct(t, f.store, f.resolver, f.issuer, "delegate", tooMuch.CanonicalBytes(), f.now.Add(2*time.Second))
	if err := f.store.DelegateAuthority(ctx, tooMuch, delegateAct, f.now.Add(2*time.Second)); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("excess delegation = %v", err)
	}
	wrongDelegate := tooMuch
	wrongDelegate.GrantID = "grant_wrong_delegate"
	wrongDelegate.IssuerPrincipalID = "principal_forged"
	maximum = 2
	wrongDelegate.Budget = action.IntentBudgetV2{Total: &maximum, PerOperation: map[string]int64{"tool/probe@1": maximum}}
	wrongDelegateAct := authorityActorAct(t, f.store, f.resolver, f.issuer, "delegate", wrongDelegate.CanonicalBytes(), f.now.Add(3*time.Second))
	if err := f.store.DelegateAuthority(ctx, wrongDelegate, wrongDelegateAct, f.now.Add(3*time.Second)); !errors.Is(err, ErrIssuerMismatch) {
		t.Fatalf("wrong delegate actor = %v", err)
	}

	adminRoot := f.root
	adminRoot.GrantID = "grant_revoke_actor"
	adminRoot.IssuerPrincipalID = "principal_external"
	adminRoot.SubjectPrincipalID = "principal_external_subject"
	reason := "recover external grant"
	adminAct := authorityActorAct(t, f.store, f.resolver, f.issuer, "issue",
		CanonicalAdminAuthorityGrant(adminRoot, reason), f.now.Add(4*time.Second))
	if err := f.store.AdminIssueAuthority(ctx, adminRoot, adminAct, reason, f.now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	revokeReason := "ordinary actor must not impersonate issuer"
	revokeAct := authorityActorAct(t, f.store, f.resolver, f.issuer, "revoke",
		CanonicalAuthorityRevoke(adminRoot.GrantID, revokeReason), f.now.Add(5*time.Second))
	if err := f.store.RevokeAuthority(ctx, adminRoot.GrantID, revokeAct, revokeReason, f.now.Add(5*time.Second)); !errors.Is(err, ErrIssuerMismatch) {
		t.Fatalf("wrong revoke actor = %v", err)
	}
	adminRevoke := authorityActorAct(t, f.store, f.resolver, f.issuer, "revoke",
		CanonicalAuthorityRevoke(adminRoot.GrantID, revokeReason), f.now.Add(6*time.Second))
	if err := f.store.AdminRevokeAuthority(ctx, adminRoot.GrantID, adminRevoke, revokeReason, f.now.Add(6*time.Second)); err != nil {
		t.Fatal(err)
	}
	secondRevoke := authorityActorAct(t, f.store, f.resolver, f.issuer, "revoke",
		CanonicalAuthorityRevoke(adminRoot.GrantID, revokeReason), f.now.Add(7*time.Second))
	if err := f.store.AdminRevokeAuthority(ctx, adminRoot.GrantID, secondRevoke, revokeReason, f.now.Add(7*time.Second)); !errors.Is(err, ErrAuthorityRevoked) {
		t.Fatalf("second revoke = %v", err)
	}
}

func TestAuthority_ActiveIntentAndRemainingBalanceFailureStates(t *testing.T) {
	t.Run("draft intent", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		draft := f.intent
		draft.IntentID = "int_draft_authority"
		draft.Version = 1
		if err := f.store.CreateIntentV2(context.Background(), draft, draft.OwnerPrincipalID, f.now); err != nil {
			t.Fatal(err)
		}
		tx, err := f.store.db.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback() }()
		if _, err := f.store.activeIntentTx(context.Background(), tx, draft.IntentID, draft.Version, draft.Digest(), f.now); !errors.Is(err, action.ErrIntentInactive) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("intent window", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		tx, err := f.store.db.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback() }()
		if _, err := f.store.activeIntentTx(context.Background(), tx, f.intent.IntentID, f.intent.Version,
			f.intent.Digest(), f.intent.ExpiresAt); !errors.Is(err, action.ErrIntentExpired) {
			t.Fatalf("error = %v", err)
		}
	})
	for name, mutate := range map[string]func(*authoritySQLiteFixture){
		"negative total": func(f *authoritySQLiteFixture) {
			account := budgetAccountID(f.intent.ProfileID, "grant", f.root.GrantID)
			if _, err := f.store.db.Exec(`UPDATE budget_accounts SET max_total=0 WHERE account_id=?`, account); err != nil {
				t.Fatal(err)
			}
		},
		"negative operation": func(f *authoritySQLiteFixture) {
			account := budgetAccountID(f.intent.ProfileID, "grant", f.root.GrantID)
			if _, err := f.store.db.Exec(`UPDATE budget_accounts SET per_operation='{"tool/probe@1":0}' WHERE account_id=?`, account); err != nil {
				t.Fatal(err)
			}
		},
		"malformed operation map": func(f *authoritySQLiteFixture) {
			account := budgetAccountID(f.intent.ProfileID, "grant", f.root.GrantID)
			if _, err := f.store.db.Exec(`UPDATE budget_accounts SET per_operation='{' WHERE account_id=?`, account); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newAuthoritySQLiteFixture(t, 2)
			if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now)); err != nil {
				t.Fatal(err)
			}
			mutate(&f)
			tx, err := f.store.db.BeginTx(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback() }()
			account := budgetAccountID(f.intent.ProfileID, "grant", f.root.GrantID)
			if _, err := f.store.remainingBeforeAccountTx(context.Background(), tx, account, "tool/probe@1"); !errors.Is(err, ErrBudgetEvidenceCorrupt) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

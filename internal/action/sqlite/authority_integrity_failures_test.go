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

func TestAuthority_ApprovedResumeRejectsMovedEvidence(t *testing.T) {
	t.Run("pending decision", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		parked := parkStrictAuthorityFixture(t, f)
		approval, _, err := f.store.GetApproval(context.Background(), parked.ApprovalID)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.store.StartApprovedAuthorization(context.Background(), parked.ApprovalID,
			PolicyPin{Version: 1, Digest: "sha256:authority-law"}, approval.ActionDigest, f.now.Add(time.Second))
		if !errors.Is(err, ErrApprovalNoLongerApproved) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("changed law", func(t *testing.T) {
		f, approval := approvedAuthorityResumeFixture(t)
		_, err := f.store.StartApprovedAuthorization(context.Background(), approval.ApprovalID,
			PolicyPin{Version: 2, Digest: "sha256:other-law"}, approval.ActionDigest, f.now.Add(2*time.Second))
		if !errors.Is(err, ErrApprovalInvalidated) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("empty parameters", func(t *testing.T) {
		f, approval := approvedAuthorityResumeFixture(t)
		if _, err := f.store.db.Exec(`UPDATE approvals SET canonical_params='' WHERE approval_id=?`, approval.ApprovalID); err != nil {
			t.Fatal(err)
		}
		_, err := f.store.StartApprovedAuthorization(context.Background(), approval.ApprovalID,
			PolicyPin{Version: 1, Digest: "sha256:authority-law"}, approval.ActionDigest, f.now.Add(2*time.Second))
		if !errors.Is(err, ErrApprovalParamsEmpty) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("corrupt preview", func(t *testing.T) {
		f, approval := approvedAuthorityResumeFixture(t)
		if _, err := f.store.db.Exec(`UPDATE approvals SET canonical_preview='{}' WHERE approval_id=?`, approval.ApprovalID); err != nil {
			t.Fatal(err)
		}
		_, err := f.store.StartApprovedAuthorization(context.Background(), approval.ApprovalID,
			PolicyPin{Version: 1, Digest: "sha256:authority-law"}, approval.ActionDigest, f.now.Add(2*time.Second))
		if !errors.Is(err, ErrApprovalEvidenceCorrupt) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("missing snapshot", func(t *testing.T) {
		f, approval := approvedAuthorityResumeFixture(t)
		if _, err := f.store.db.Exec(`DELETE FROM authorization_snapshots WHERE action_id=?`, approval.ActionID); err != nil {
			t.Fatal(err)
		}
		_, err := f.store.StartApprovedAuthorization(context.Background(), approval.ApprovalID,
			PolicyPin{Version: 1, Digest: "sha256:authority-law"}, approval.ActionDigest, f.now.Add(2*time.Second))
		if !errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
			t.Fatalf("error = %v", err)
		}
	})
	// The ingress capability's expiry belongs to the PARK, and this row is that
	// guarantee at the door that owns it: a request whose capability is already
	// dead is not parked at all, by name, and nothing of it is born. What the
	// strict RESUME does with the two instants — freshness at the park,
	// liveness at the start — is
	// TestAuthority_ApprovedResumeJudgesFreshnessAtTheParkAndLivenessAtTheStart.
	// Until the director's adjudication of 2026-09-22 this row lived on the
	// resume and demanded the opposite; the sentence it used to make is in the
	// canto.
	// Probing mutation executed: the park judges expiry at the evidence's own
	// observation instant — red with «error = <nil>» and «parked actions = 1,
	// want 0».
	t.Run("a capability already dead is not parked", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		const requestID = "request-strict-pending-dead-capability"
		ingress, err := f.issuer.Issue(requestID, "verified-subject")
		if err != nil {
			t.Fatal(err)
		}
		// The fixture's capability lives one minute; this park is two later.
		_, err = f.store.ParkAuthorization(context.Background(), AuthorityPendingRequest{
			ActorPrincipalID: f.root.SubjectPrincipalID, CorrelationID: requestID,
			SourceProtocol: "native", Channel: "webhook",
			Operation: action.Operation{Namespace: "tool", Name: "probe", Version: 1}, Arguments: `{}`,
			EffectClass: action.EffectWriteReversible, At: f.now.Add(2 * time.Minute),
			ApprovalContext: action.ApprovalContext{
				ToolCage: "probe", Descriptor: action.EffectDescriptor{Class: action.EffectWriteReversible},
				HasDescriptor: true, LawVersion: 1, LawDigest: "sha256:authority-law", TTL: time.Minute,
			},
			ResolveEvidence: func(actionID string) (identity.Evidence, error) {
				return f.resolver.Resolve(ingress, identity.ResolveRequest{
					ActionID: actionID, RequestID: requestID, Channel: "webhook", Brain: "alpha",
				})
			},
		})
		if !errors.Is(err, identity.ErrIdentityEvidenceExpired) {
			t.Errorf("error = %v, want %v", err, identity.ErrIdentityEvidenceExpired)
		}
		for what, query := range map[string]string{
			"parked actions":   `SELECT COUNT(*) FROM actions WHERE action_id LIKE 'act3_%'`,
			"approvals":        `SELECT COUNT(*) FROM approvals`,
			"signed snapshots": `SELECT COUNT(*) FROM authorization_snapshots`,
		} {
			if n := authorityScalar(t, f.store, query); n != 0 {
				t.Errorf("%s = %d, want 0: a refused park left something born", what, n)
			}
		}
	})
	t.Run("action state moved", func(t *testing.T) {
		f, approval := approvedAuthorityResumeFixture(t)
		if _, err := f.store.db.Exec(`UPDATE actions SET state=? WHERE action_id=?`, string(action.StateAuthorized), approval.ActionID); err != nil {
			t.Fatal(err)
		}
		_, err := f.store.StartApprovedAuthorization(context.Background(), approval.ApprovalID,
			PolicyPin{Version: 1, Digest: "sha256:authority-law"}, approval.ActionDigest, f.now.Add(2*time.Second))
		if !errors.Is(err, ErrApprovalNoLongerApproved) {
			t.Fatalf("error = %v", err)
		}
	})
}

func approvedAuthorityResumeFixture(t *testing.T) (authoritySQLiteFixture, action.Approval) {
	t.Helper()
	f := newAuthoritySQLiteFixture(t, 2)
	parked := parkStrictAuthorityFixture(t, f)
	return f, approveStrictAuthorityFixture(t, f, parked)
}

func TestAuthority_StartRejectsMovedIntentBindingAndGrantEvidence(t *testing.T) {
	t.Run("revoked intent", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		if err := f.store.RevokeIntentV2(context.Background(), f.intent.IntentID,
			f.intent.OwnerPrincipalID, f.now.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now.Add(2*time.Second))); !errors.Is(err, action.ErrIntentRevoked) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("expired intent", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		if err := f.store.ExpireIntentV2(context.Background(), f.intent.IntentID,
			f.intent.OwnerPrincipalID, f.now.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now.Add(2*time.Second))); !errors.Is(err, action.ErrIntentExpired) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("intent digest", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		if _, err := f.store.db.Exec(`UPDATE execution_bindings SET intent_digest='sha256:bad'`); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now)); !errors.Is(err, action.ErrIntentEvidenceCorrupt) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("partial grant binding", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		if _, err := f.store.db.Exec(`UPDATE execution_bindings SET grant_digest=NULL`); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now)); !errors.Is(err, action.ErrAuthorityEvidenceCorrupt) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("grant binding digest", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		if _, err := f.store.db.Exec(`UPDATE execution_bindings SET grant_digest='sha256:bad'`); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now)); !errors.Is(err, action.ErrAuthorityEvidenceCorrupt) {
			t.Fatalf("error = %v", err)
		}
	})
	for name, mutation := range map[string]string{
		"grant terms":   `UPDATE grant_versions SET canonical_terms=X'7B7D' WHERE grant_id='grant_root'`,
		"grant seal":    `UPDATE grant_versions SET signature='00' WHERE grant_id='grant_root'`,
		"event time":    `UPDATE grant_events SET occurred_at='bad' WHERE grant_id='grant_root'`,
		"event seal":    `UPDATE grant_events SET signature='00' WHERE grant_id='grant_root'`,
		"head digest":   `UPDATE grant_heads SET last_event_digest='sha256:bad' WHERE grant_id='grant_root'`,
		"head revision": `UPDATE grant_heads SET revision=revision+1 WHERE grant_id='grant_root'`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newAuthoritySQLiteFixture(t, 2)
			if _, err := f.store.db.Exec(mutation); err != nil {
				t.Fatal(err)
			}
			if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now)); !errors.Is(err, action.ErrAuthorityEvidenceCorrupt) {
				t.Fatalf("corrupt grant evidence: error = %v, want %v", err, action.ErrAuthorityEvidenceCorrupt)
			}
		})
	}
}

func TestAuthority_ActivationAndConfigTamperingFailClosed(t *testing.T) {
	activationMutations := map[string]string{
		"head seal":      `UPDATE approval_birth_heads SET signature='00'`,
		"head sequence":  `UPDATE approval_birth_heads SET sequence=sequence+1`,
		"event seal":     `UPDATE approval_birth_events SET signature='00' WHERE sequence=0`,
		"event previous": `UPDATE approval_birth_events SET previous_event_digest='sha256:bad' WHERE sequence=0`,
		"activation id":  `UPDATE approval_birth_events SET approval_id='activation:wrong' WHERE sequence=0`,
	}
	for name, mutation := range activationMutations {
		t.Run("activation "+name, func(t *testing.T) {
			f := newAuthoritySQLiteFixture(t, 2)
			root := activateAuthorityFixture(t, f)
			if _, err := f.store.db.Exec(mutation); err != nil {
				t.Fatal(err)
			}
			if err := f.store.RequireAuthorityActivation(context.Background(), f.intent.ProfileID, root); !errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	t.Run("activation unknown automatic profile", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		if err := f.store.RequireAuthorityActivation(context.Background(), "", "sha256:missing"); !errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
			t.Fatalf("error = %v", err)
		}
	})

	configMutations := map[string]string{
		"head seal":        `UPDATE config_authority_heads SET signature='00'`,
		"head set":         `UPDATE config_authority_heads SET clause_set_digest='sha256:bad'`,
		"clause json":      `UPDATE config_authority_snapshots SET canonical_clause=X'7B7D'`,
		"clause seal":      `UPDATE config_authority_snapshots SET signature='00'`,
		"missing snapshot": `DELETE FROM config_authority_snapshots`,
	}
	for name, mutation := range configMutations {
		t.Run("config "+name, func(t *testing.T) {
			f := newAuthoritySQLiteFixture(t, 2)
			activateAuthorityFixture(t, f)
			clause := configClauseFixture(f, "probe", []string{"webhook"})
			if _, err := f.store.SyncConfigAuthorityClauses(context.Background(), f.intent.ProfileID,
				f.root.SubjectPrincipalID, []action.ConfigAuthorityClause{clause}, f.now.Add(2*time.Second)); err != nil {
				t.Fatal(err)
			}
			if _, err := f.store.db.Exec(mutation); err != nil {
				t.Fatal(err)
			}
			if _, err := f.store.SyncConfigAuthorityClauses(context.Background(), f.intent.ProfileID,
				f.root.SubjectPrincipalID, []action.ConfigAuthorityClause{clause}, f.now.Add(3*time.Second)); !errors.Is(err, action.ErrAuthorityEvidenceCorrupt) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

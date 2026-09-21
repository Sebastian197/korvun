// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/identity"
)

func TestAuthority_ActivationAndImportInputBoundaries(t *testing.T) {
	t.Run("activation shape", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		if _, err := f.store.ActivateAuthority(context.Background(), "", "", "", f.now); !errors.Is(err, action.ErrAuthorityMalformed) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("activation actor binding", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		manifest, err := f.store.AuthorityActivationManifest(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		act := authorityActorAct(t, f.store, f.resolver, f.issuer, "activate", []byte("wrong"), f.now)
		if _, err := f.store.ActivateAuthority(context.Background(), f.intent.ProfileID, act,
			"bind exact manifest", f.now); !errors.Is(err, identity.ErrIdentityBindingMismatch) {
			t.Fatalf("manifest %s error = %v", manifest, err)
		}
	})
	t.Run("activation one shot", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		activateAuthorityFixture(t, f)
		if _, err := f.store.ActivateAuthority(context.Background(), f.intent.ProfileID, "unused",
			"second activation", f.now.Add(time.Second)); !errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("strict row cannot be adopted as legacy", func(t *testing.T) {
		f, _ := authorityApprovalFixture(t)
		if _, err := f.store.AuthorityActivationManifest(context.Background()); !errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("import shape", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		if err := f.store.ImportLegacyAuthority(context.Background(), "", f.root, "", "", f.now); !errors.Is(err, action.ErrAuthorityMalformed) {
			t.Fatalf("error = %v", err)
		}
		bad := f.root
		bad.Version = 0
		if err := f.store.ImportLegacyAuthority(context.Background(), "legacy", bad, "", "reason", f.now); !errors.Is(err, action.ErrAuthorityMalformed) {
			t.Fatalf("malformed grant error = %v", err)
		}
	})
	t.Run("missing legacy grant", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		grant := f.root
		grant.GrantID = "grant_missing_legacy"
		reason := "missing legacy evidence must fail"
		act := authorityActorAct(t, f.store, f.resolver, f.issuer, "import",
			CanonicalLegacyAuthorityImport("legacy_missing", grant, reason), f.now)
		if err := f.store.ImportLegacyAuthority(context.Background(), "legacy_missing", grant, act, reason, f.now); !errors.Is(err, ErrAuthorityMissing) {
			t.Fatalf("import over a legacy grant that is not there: error = %v, want %v", err, ErrAuthorityMissing)
		}
	})
	t.Run("inactive legacy grant", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		legacyIntent := draftIntent(f.intent.IntentID)
		legacyIntent.Status = action.LifecycleActive
		if err := f.store.CreateIntent(context.Background(), legacyIntent); err != nil {
			t.Fatal(err)
		}
		legacy := rootGrant(f.root.GrantID, legacyIntent.IntentID)
		legacy.Status = action.LifecycleDraft
		if err := f.store.CreateGrant(context.Background(), legacy); err != nil {
			t.Fatal(err)
		}
		reason := "draft legacy grant cannot be imported"
		act := authorityActorAct(t, f.store, f.resolver, f.issuer, "import",
			CanonicalLegacyAuthorityImport(legacy.GrantID, f.root, reason), f.now)
		if err := f.store.ImportLegacyAuthority(context.Background(), legacy.GrantID, f.root, act, reason, f.now); !errors.Is(err, ErrAuthorityInactive) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestAuthority_SignerAndClosedStoreBoundaries(t *testing.T) {
	t.Run("unknown signer key", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		f.store.SetAuthoritySigner(func(string, []byte) action.AuthoritySignature {
			return action.AuthoritySignature{Digest: "sha256:unknown", SigningKeyID: "ed25519:unknown", Signature: "00"}
		})
		if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now)); !errors.Is(err, ErrNotFound) {
			t.Fatalf("a seal under a key the store does not hold: error = %v, want %v", err, ErrNotFound)
		}
	})
	t.Run("retired signer key", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		if _, err := f.store.db.Exec(`UPDATE signing_keys SET retired_at=? WHERE retired_at IS NULL`, f.now.Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now)); !errors.Is(err, action.ErrSigningKeyRetired) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("closed store doors", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		if err := f.store.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.AuthorityActivationManifest(context.Background()); err == nil || !strings.Contains(err.Error(), "sql: database is closed") {
			t.Fatalf("manifest on a closed store: error = %v, want the driver's «sql: database is closed»", err)
		}
		if err := f.store.IssueAuthority(context.Background(), f.root, "act", f.now); err == nil || !strings.Contains(err.Error(), "sql: database is closed") {
			t.Fatalf("issue on a closed store: error = %v, want the driver's «sql: database is closed»", err)
		}
		if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now)); err == nil || !strings.Contains(err.Error(), "sql: database is closed") {
			t.Fatalf("start on a closed store: error = %v, want the driver's «sql: database is closed»", err)
		}
		if _, err := f.store.ParkAuthorization(context.Background(), AuthorityPendingRequest{
			ResolveEvidence: func(string) (identity.Evidence, error) { return identity.Evidence{}, nil },
		}); err == nil || !strings.Contains(err.Error(), "sql: database is closed") {
			t.Fatalf("park on a closed store: error = %v, want the driver's «sql: database is closed»", err)
		}
	})
}

func TestAuthority_ConfigAndApprovedLegacyBoundaries(t *testing.T) {
	t.Run("config requires activation", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		if _, err := f.store.SyncConfigAuthorityClauses(context.Background(), f.intent.ProfileID,
			f.root.SubjectPrincipalID, nil, f.now); !errors.Is(err, action.ErrAuthorityEvidenceCorrupt) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("config missing head", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		activateAuthorityFixture(t, f)
		if _, err := f.store.db.Exec(`UPDATE execution_bindings SET grant_id=NULL,grant_version=NULL,grant_digest=NULL`); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now.Add(2*time.Second))); !errors.Is(err, ErrAuthorityMissing) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("legacy approval has no strict snapshot", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		approval := createLegacyApprovalBeforeAuthority(t, f, "act_legacy_resume")
		env, identified := operatorDecisionEnv("approve", approval.ApprovalID)
		if _, err := f.store.DecideApprovalUnderLaw(context.Background(), approval.ApprovalID,
			action.DecisionApproved, f.now.Add(time.Second), env, identified, "",
			PolicyPin{Version: 1, Digest: "sha256:legacy-law"}); err != nil {
			t.Fatal(err)
		}
		_, err := f.store.StartApprovedAuthorization(context.Background(), approval.ApprovalID,
			PolicyPin{Version: 1, Digest: "sha256:legacy-law"}, approval.ActionDigest, f.now.Add(2*time.Second))
		if !errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("remaining account missing", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		tx, err := f.store.db.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback() }()
		if _, err := f.store.remainingBeforeAccountTx(context.Background(), tx, "missing", "tool/probe@1"); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("remaining balance of an account that is not there: error = %v, want %v", err, sql.ErrNoRows)
		}
	})
}

func TestAuthority_InternalSigningAndDoorFailuresRollBack(t *testing.T) {
	t.Run("write lock invariant", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		if _, err := f.store.db.Exec(`UPDATE authority_write_lock SET revision=-1 WHERE singleton=1`); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now)); !errors.Is(err, action.ErrAuthorityEvidenceCorrupt) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("empty administrative actor", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		tx, err := f.store.db.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback() }()
		if _, err := f.store.validateAuthorityActorTx(context.Background(), tx, "", "issue", nil, false, f.now); !errors.Is(err, identity.ErrIdentityEvidenceMissing) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("issue actor binding", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		grant := f.root
		grant.GrantID = "grant_wrong_issue_act"
		at := f.now.Add(99 * time.Nanosecond)
		act := authorityActorAct(t, f.store, f.resolver, f.issuer, "issue", []byte("wrong"), at)
		if err := f.store.IssueAuthority(context.Background(), grant, act, at); !errors.Is(err, identity.ErrIdentityBindingMismatch) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("malformed delegation", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		child := f.root
		child.Version = 0
		if err := f.store.DelegateAuthority(context.Background(), child, "", f.now); !errors.Is(err, action.ErrAuthorityMalformed) {
			t.Fatalf("error = %v", err)
		}
	})
	// Three doors pointed at an authority row that is not there — the grant to
	// revoke, the parent to delegate under, the legacy grant to import (the
	// import row lives in TestAuthority_ActivationAndImportInputBoundaries) —
	// answered with the driver's bare sql.ErrNoRows. They name it now.
	// Probing mutation executed: authorityAbsent returns the error it was given
	// — red on all three rows with «error = sql: no rows in result set».
	t.Run("missing revocation target", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		if err := f.store.RevokeAuthority(context.Background(), "grant_missing", "", "reason", f.now); !errors.Is(err, ErrAuthorityMissing) {
			t.Fatalf("revoke of a grant that is not there: error = %v, want %v", err, ErrAuthorityMissing)
		}
	})
	t.Run("missing delegation parent", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		child := f.root
		child.GrantID = "grant_orphan"
		child.ParentGrantID, child.ParentGrantVersion = "grant_parent_missing", 1
		child.DelegationDepthRemaining = f.root.DelegationDepthRemaining - 1
		act := authorityActorAct(t, f.store, f.resolver, f.issuer, "delegate", child.CanonicalBytes(), f.now)
		if err := f.store.DelegateAuthority(context.Background(), child, act, f.now); !errors.Is(err, ErrAuthorityMissing) {
			t.Fatalf("delegation under a parent that is not there: error = %v, want %v", err, ErrAuthorityMissing)
		}
		if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM grant_heads WHERE grant_id='grant_orphan'`); n != 0 {
			t.Fatalf("orphan grants born = %d, want 0", n)
		}
	})
	t.Run("import actor binding", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		act := authorityActorAct(t, f.store, f.resolver, f.issuer, "import", []byte("wrong"), f.now)
		if err := f.store.ImportLegacyAuthority(context.Background(), "legacy_missing", f.root,
			act, "exact actor binding", f.now); !errors.Is(err, identity.ErrIdentityBindingMismatch) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("activation root signer", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		manifest, err := f.store.AuthorityActivationManifest(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		reason := "root signer must exist"
		act := authorityActorAct(t, f.store, f.resolver, f.issuer, "activate",
			CanonicalAuthorityActivation(f.intent.ProfileID, manifest, reason), f.now)
		f.store.SetAuthoritySigner(nil)
		if _, err := f.store.ActivateAuthority(context.Background(), f.intent.ProfileID, act, reason, f.now); !errors.Is(err, ErrAuthoritySignerUnavailable) {
			t.Fatalf("activation with no signer: error = %v, want %v", err, ErrAuthoritySignerUnavailable)
		}
	})
	t.Run("activation legacy event signer", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		createLegacyApprovalBeforeAuthority(t, f, "act_legacy_signer_failure")
		manifest, err := f.store.AuthorityActivationManifest(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		reason := "every adopted event must be signed"
		act := authorityActorAct(t, f.store, f.resolver, f.issuer, "activate",
			CanonicalAuthorityActivation(f.intent.ProfileID, manifest, reason), f.now)
		calls := 0
		f.store.SetAuthoritySigner(func(domain string, canonical []byte) action.AuthoritySignature {
			calls++
			if calls == 2 {
				return action.SignAuthorityBytes(f.private, domain+".wrong", canonical)
			}
			return action.SignAuthorityBytes(f.private, domain, canonical)
		})
		if _, err := f.store.ActivateAuthority(context.Background(), f.intent.ProfileID, act, reason, f.now); !errors.Is(err, action.ErrAuthorityEvidenceCorrupt) {
			t.Fatalf("error = %v, want %v", err, action.ErrAuthorityEvidenceCorrupt)
		}
	})
	t.Run("activation head signer", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		manifest, err := f.store.AuthorityActivationManifest(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		reason := "head must be signed"
		act := authorityActorAct(t, f.store, f.resolver, f.issuer, "activate",
			CanonicalAuthorityActivation(f.intent.ProfileID, manifest, reason), f.now)
		calls := 0
		f.store.SetAuthoritySigner(func(domain string, canonical []byte) action.AuthoritySignature {
			calls++
			if calls == 2 {
				return action.SignAuthorityBytes(f.private, domain+".wrong", canonical)
			}
			return action.SignAuthorityBytes(f.private, domain, canonical)
		})
		if _, err := f.store.ActivateAuthority(context.Background(), f.intent.ProfileID, act, reason, f.now); !errors.Is(err, action.ErrAuthorityEvidenceCorrupt) {
			t.Fatalf("error = %v, want %v", err, action.ErrAuthorityEvidenceCorrupt)
		}
	})
	for _, withClause := range []bool{false, true} {
		name := "config head signer"
		if withClause {
			name = "config clause signer"
		}
		t.Run(name, func(t *testing.T) {
			f := newAuthoritySQLiteFixture(t, 2)
			activateAuthorityFixture(t, f)
			var clauses []action.ConfigAuthorityClause
			if withClause {
				clauses = []action.ConfigAuthorityClause{configClauseFixture(f, "probe", []string{"webhook"})}
			}
			f.store.SetAuthoritySigner(nil)
			if _, err := f.store.SyncConfigAuthorityClauses(context.Background(), f.intent.ProfileID,
				f.root.SubjectPrincipalID, clauses, f.now.Add(time.Second)); !errors.Is(err, ErrAuthoritySignerUnavailable) {
				t.Fatalf("config sync with no signer: error = %v, want %v", err, ErrAuthoritySignerUnavailable)
			}
		})
	}
}

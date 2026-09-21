// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/identity"
)

// TestIdentity_ClaimJudgesTheStoredRow attacks the claim-time binding by
// rewriting, from a SECOND REAL CONNECTION, the two `actions` columns the
// evidence must agree with. Before the cure both comparisons judged a value
// recomputed out of the evidence itself, so a rewritten correlation id or
// source channel claimed CLEAN with the signature still verifying — the
// signature covers the evidence, and nothing covered the row.
//
// The forbidden outcome is a successful claim: the approval's parameters must
// stay unconsumed and readable afterwards.
//
// Evidence level: MULTIPLE REAL SQLITE CONNECTIONS (externalIdentityExec opens
// its own connection to the same file and commits before the claim begins).
// Probing mutation executed: build the claim envelope from `evidence.RequestID`
// and `evidence.Provider` again — both rows go green, which is the finding this
// mould exists to make impossible.
func TestIdentity_ClaimJudgesTheStoredRow(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"correlation id rewritten", `UPDATE actions SET correlation_id='request-OTHER' WHERE action_id=?`},
		{"source channel rewritten", `UPDATE actions SET source_channel='discord' WHERE action_id=?`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actionID := "act_stored_" + tt.name[:6]
			store, approval, _, _, _ := approvedIdentityFixture(t, actionID)
			externalIdentityExec(t, store, tt.query, approval.ActionID)

			params, _, err := store.ClaimApprovalParamsUnderDigest(context.Background(),
				approval.ApprovalID, &PolicyPin{Version: 1, Digest: "sha256:law"},
				approval.ActionDigest, nil)
			if !errors.Is(err, identity.ErrIdentityBindingMismatch) {
				t.Fatalf("claim error = %v, want %v", err, identity.ErrIdentityBindingMismatch)
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
}

// TestIdentity_ClaimPrecedenceIsPinned forces TWO faults at once and demands
// the exact sentinel the implemented order produces. The paper declared an
// order no test pinned, and the order it declared was not the one the code
// runs; a combined fault is the only way to tell them apart, because either
// fault alone returns its own error and proves nothing about precedence.
//
// The pinned order, verified here: evidence corruption, then expiry, then
// binding mismatch, then principal-history corruption, then a disabled
// principal, then an inactive or moved binding.
//
// Evidence level: MULTIPLE REAL SQLITE CONNECTIONS.
// Probing mutation executed: move the expiry check after the principal loop.
// It is judged in TWO places and the claim path reaches the one in
// validateActionIdentityTx FIRST, so moving only the one inside
// validateResolvedEvidenceTx changes nothing and the mould stays green — the
// mutation was mis-aimed, not the mould blind. Moving BOTH reddens "expired
// beats history corrupt" with «claim error = identity: principal evidence
// corrupt, want identity: authenticated ingress expired».
func TestIdentity_ClaimPrecedenceIsPinned(t *testing.T) {
	tests := []struct {
		name   string
		want   error
		mutate func(*testing.T, *Store, action.Approval, identity.Evidence, time.Time)
	}{
		{"corrupt beats expired", identity.ErrIdentityEvidenceCorrupt,
			func(t *testing.T, store *Store, a action.Approval, e identity.Evidence, _ time.Time) {
				externalIdentityExec(t, store, `UPDATE actions SET identity_signature='00' WHERE action_id=?`, a.ActionID)
				store.identityNow = func() time.Time { return e.ExpiresAt }
			}},
		{"expired beats history corrupt", identity.ErrIdentityEvidenceExpired,
			func(t *testing.T, store *Store, _ action.Approval, e identity.Evidence, _ time.Time) {
				externalIdentityExec(t, store, `DELETE FROM principal_events WHERE principal_id=?`, e.RequesterPrincipalID)
				store.identityNow = func() time.Time { return e.ExpiresAt }
			}},
		{"binding mismatch beats history corrupt", identity.ErrIdentityBindingMismatch,
			func(t *testing.T, store *Store, a action.Approval, e identity.Evidence, _ time.Time) {
				externalIdentityExec(t, store, `DELETE FROM principal_events WHERE principal_id=?`, e.RequesterPrincipalID)
				externalIdentityExec(t, store, `UPDATE actions SET correlation_id='request-OTHER' WHERE action_id=?`, a.ActionID)
			}},
		{"history corrupt beats disabled", identity.ErrPrincipalEvidenceCorrupt,
			func(t *testing.T, store *Store, _ action.Approval, e identity.Evidence, now time.Time) {
				if err := store.DisablePrincipal(context.Background(), e.RequesterPrincipalID, now.Add(2*time.Second)); err != nil {
					t.Fatal(err)
				}
				externalIdentityExec(t, store, `DELETE FROM principal_events WHERE principal_id=?`, e.RequesterPrincipalID)
				store.identityNow = func() time.Time { return now.Add(3 * time.Second) }
			}},
		{"disabled beats binding revoked", identity.ErrPrincipalDisabled,
			func(t *testing.T, store *Store, _ action.Approval, e identity.Evidence, now time.Time) {
				if err := store.DisablePrincipal(context.Background(), e.RequesterPrincipalID, now.Add(2*time.Second)); err != nil {
					t.Fatal(err)
				}
				externalIdentityExec(t, store, `UPDATE principal_bindings SET status='revoked' WHERE binding_id='binding_webhook'`)
				store.identityNow = func() time.Time { return now.Add(3 * time.Second) }
			}},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, approval, evidence, _, now := approvedIdentityFixture(t, "act_prec_"+string(rune('a'+i)))
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
		})
	}
}

// TestIdentity_CloseAfterTheEffectNeverRefuses attacks the close that runs
// AFTER a physical effect has already happened. Corrupting the birth snapshot
// from a second connection made FinishWithResult return a brand-new error
// class, which stranded the executed action in its pre-terminal state and left
// the effect with no terminal receipt — a failure class the paper explicitly
// says this phase does not add to the close path.
//
// The demanded outcome is not leniency: the close SUCCEEDS, the action reaches
// its terminal state, and the receipt is born v3 carrying identity_status
// "corrupt" with EMPTY attribution. No principal is ever invented for a
// snapshot that does not verify.
//
// Evidence level: MULTIPLE REAL SQLITE CONNECTIONS.
// Probing mutation executed: pass allowCorruptIdentity=false at the
// FinishWithResult call site — the close fails with "evidence corrupt" and the
// action stays AUTHORIZED, reddening this mould.
func TestIdentity_CloseAfterTheEffectNeverRefuses(t *testing.T) {
	const actionID = "act_close_after_effect"
	store, resolver, issuer, _, now := identityStoreFixture(t)
	env, evidence := identityAttempt(t, resolver, issuer, actionID, now)
	if err := store.RecordAttemptAuthenticated(context.Background(), env,
		Decision{Outcome: "allow", Rule: "identity_close"}, action.StateAuthorized, evidence); err != nil {
		t.Fatalf("RecordAttemptAuthenticated: %v", err)
	}

	// The effect has run. Only now does the snapshot go bad underneath it.
	externalIdentityExec(t, store, `UPDATE actions SET identity_signature='00' WHERE action_id=?`, actionID)

	if err := store.FinishWithResult(context.Background(), actionID,
		action.StateSucceeded, now.Add(time.Second), "sha256:result"); err != nil {
		t.Fatalf("FinishWithResult after the effect ran = %v, want nil", err)
	}

	var state string
	if err := store.db.QueryRow(`SELECT state FROM actions WHERE action_id=?`, actionID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if action.State(state) != action.StateSucceeded {
		t.Fatalf("state after the close = %q, want %q", state, action.StateSucceeded)
	}
	receipts, err := store.ReceiptsByAction(context.Background(), actionID)
	if err != nil || len(receipts) != 1 {
		t.Fatalf("ReceiptsByAction = %+v, %v", receipts, err)
	}
	receipt := receipts[0]
	if receipt.SchemaVersion != 3 || receipt.IdentityStatus != "corrupt" {
		t.Fatalf("receipt = v%d/%q, want v3/corrupt", receipt.SchemaVersion, receipt.IdentityStatus)
	}
	if receipt.RequesterPrincipalID != "" || receipt.ActorPrincipalID != "" ||
		receipt.ResponsiblePrincipalID != "" || receipt.IdentityEvidenceDigest != "" {
		t.Fatalf("corrupt receipt invented attribution: %+v", receipt)
	}
	if receipt.IdentitySnapshotDigest == "" {
		t.Fatal("corrupt receipt carries no snapshot digest: the corruption itself is unrecorded")
	}
}

// TestIdentity_RegistrationToleratesReferenceDrift attacks boot survival. A
// binding's CredentialRef is the NAME of an environment variable and a
// principal's DisplayName is decoration by its own godoc; comparing either
// against the stored row made a routine `token_env` rename permanently
// boot-fatal on an existing store, with no documented remedy and no way back.
//
// The same mould demands the opposite for an identity-bearing field: a changed
// CHANNEL is still refused, unoverwritten, with its named message. Without that
// second half the first half would just be a weakening.
//
// Evidence level: in-process, one store reopened twice (the real registration
// path, not a helper).
// Probing mutation executed: restore `stored != configured` and drop the Kind
// split — "renamed credential reference" reddens.
func TestIdentity_RegistrationToleratesReferenceDrift(t *testing.T) {
	newStore := func(t *testing.T) (*Store, time.Time) {
		t.Helper()
		store, _, _, _, now := identityStoreFixture(t)
		return store, now
	}

	t.Run("renamed credential reference", func(t *testing.T) {
		store, now := newStore(t)
		renamed := identityRegistryFixture()
		renamed.Bindings[0].CredentialRef = "WEBHOOK_SECRET_V2"
		renamed.Principals[0].DisplayName = "renamed in the config"
		if err := store.RegisterIdentity(context.Background(), renamed, now); err != nil {
			t.Fatalf("re-registration after a rename = %v, want nil", err)
		}
		var stored string
		if err := store.db.QueryRow(
			`SELECT credential_ref FROM principal_bindings WHERE binding_id='binding_webhook'`).
			Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if stored != "WEBHOOK_SECRET" {
			t.Fatalf("stored credential_ref = %q; the tolerated drift OVERWROTE the row", stored)
		}
	})

	t.Run("changed channel is still refused", func(t *testing.T) {
		store, now := newStore(t)
		moved := identityRegistryFixture()
		moved.Bindings[0].Channel = "discord"
		err := store.RegisterIdentity(context.Background(), moved, now)
		if err == nil {
			t.Fatal("a binding moved to another channel registered cleanly")
		}
		var stored string
		if qErr := store.db.QueryRow(
			`SELECT channel FROM principal_bindings WHERE binding_id='binding_webhook'`).
			Scan(&stored); qErr != nil {
			t.Fatal(qErr)
		}
		if stored != "webhook" {
			t.Fatalf("stored channel = %q; the refused registration overwrote the row", stored)
		}
	})

	t.Run("changed principal kind is still refused", func(t *testing.T) {
		store, now := newStore(t)
		moved := identityRegistryFixture()
		moved.Principals[0].Kind = identity.PrincipalHuman
		if err := store.RegisterIdentity(context.Background(), moved, now); err == nil {
			t.Fatal("a shared credential re-declared as a human registered cleanly")
		}
	})
}

// TestIdentity_ReadOnlyStoreAnswersWhatTimeItIs attacks the constructor
// asymmetry: OpenReadOnly built a Store with no identity clock while open()
// set one, so the first identity-bearing call through a read-only store was a
// nil-pointer PANIC rather than any error. A panic is not a failure class; it
// is the absence of one.
//
// Evidence level: in-process, over a real store file opened read-only.
// Probing mutation executed: drop identityNow from the OpenReadOnly literal —
// the call panics and the mould reddens.
func TestIdentity_ReadOnlyStoreAnswersWhatTimeItIs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "readonly.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	readOnly, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = readOnly.Close() })

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("a read-only store panicked instead of erroring: %v", r)
		}
	}()
	if readOnly.identityNow == nil {
		t.Fatal("OpenReadOnly left identityNow nil: the next identity call is a panic, not an error")
	}
	if got := readOnly.identityNow(); got.IsZero() {
		t.Fatal("a read-only store's clock reads the zero time")
	}
}

// TestIdentity_LegacyEvidenceViewNeverInventsACredential attacks the v1 view of
// a v2 evidence row. CredentialType is a FINITE enum; the fallback ended in a
// default that cast the stored class STRING straight into it, so an unknown
// provider produced a credential kind that is not a member of the enum and that
// nothing downstream could reject. A guard by provider TEXT with an inventing
// default is worse than no guard: it answers confidently and wrongly.
//
// Evidence level: MULTIPLE REAL SQLITE CONNECTIONS (the provider is rewritten
// from a second connection, committed, before the read).
// Probing mutation executed: restore `default: return action.CredentialType(class)`
// — the unknown-provider row comes back as a manufactured credential kind with
// no error and this mould reddens.
func TestIdentity_LegacyEvidenceViewNeverInventsACredential(t *testing.T) {
	const actionID = "act_legacy_view"
	store, resolver, issuer, _, now := identityStoreFixture(t)
	env, evidence := identityAttempt(t, resolver, issuer, actionID, now)
	if err := store.RecordAttemptAuthenticated(context.Background(), env,
		Decision{Outcome: "allow", Rule: "identity_view"}, action.StateAuthorized, evidence); err != nil {
		t.Fatalf("RecordAttemptAuthenticated: %v", err)
	}

	known, err := store.GetEvidence(context.Background(), actionID)
	if err != nil {
		t.Fatalf("GetEvidence over a v2 row = %v", err)
	}
	if known.Credential != action.CredentialInboundBearer {
		t.Fatalf("webhook v2 evidence mapped to %q, want %q",
			known.Credential, action.CredentialInboundBearer)
	}

	externalIdentityExec(t, store, `UPDATE identity_evidence_v2 SET provider='mystery' WHERE action_id=?`, actionID)
	invented, err := store.GetEvidence(context.Background(), actionID)
	if err == nil {
		t.Fatalf("an unknown provider produced credential %q instead of a refusal", invented.Credential)
	}
	if invented.Credential != "" {
		t.Fatalf("a refused read still returned a credential kind: %q", invented.Credential)
	}
}

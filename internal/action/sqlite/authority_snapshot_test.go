// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// authorityApprovalFixture parks one strict request through ParkAuthorization —
// the door production parks through — over a fixture whose intent allows five
// starts. An earlier shape of this fixture was born through a store door with
// no production caller, fed a snapshot the TEST had written; every detail mould
// then read back the test's own strings (the adversary's pass over this phase,
// F4). That door is gone. What these moulds read now is what the store wrote.
func authorityApprovalFixture(t *testing.T) (authoritySQLiteFixture, AuthorityPendingResult) {
	t.Helper()
	f := newAuthoritySQLiteFixture(t, 5)
	return f, parkStrictAuthorityFixture(t, f)
}

// TestApprovalDetail_AuthoritySnapshotSignatureIsRequired rewrites a parked
// snapshot COHERENTLY — the purpose column, the same purpose inside the
// canonical bytes, and a fresh digest — the way a writer with the database but
// not the profile key would. Column-against-bytes agreement cannot see it; only
// the signature can.
//
// Evidence level: in-process, one real SQLite store, the row rewritten with SQL.
// Probing mutation executed: accept the snapshot without verifying its
// signature — red with «detail error = <nil>».
func TestApprovalDetail_AuthoritySnapshotSignatureIsRequired(t *testing.T) {
	f, parked := authorityApprovalFixture(t)
	if _, err := f.store.db.Exec(`UPDATE authorization_snapshots
		SET intent_purpose='forged purpose', canonical_context=replace(canonical_context,'Run the authority probe','forged purpose'),
		    authorization_digest='sha256:coherent-looking-but-unsigned'
		WHERE action_id=?`, parked.ActionID); err != nil {
		t.Fatal(err)
	}
	_, err := f.store.ApprovalDetail(context.Background(), parked.ApprovalID)
	if !errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
		t.Errorf("detail error = %v, want %v", err, ErrAuthorizationSnapshotCorrupt)
	}
}

// TestApprovalDetail_MissingRequiredSnapshotIsCorrupt deletes the snapshot of a
// row that still carries the strict marker. Absent evidence under a marker that
// demands it is corruption, never «a legacy approval with no authority».
//
// Evidence level: in-process, one real SQLite store, the row deleted with SQL.
// Probing mutation executed: answer an absent required snapshot with «no
// authority» instead of the refusal — red with «detail error = <nil>».
func TestApprovalDetail_MissingRequiredSnapshotIsCorrupt(t *testing.T) {
	f, parked := authorityApprovalFixture(t)
	if _, err := f.store.db.Exec(`DELETE FROM authorization_snapshots WHERE action_id=?`, parked.ActionID); err != nil {
		t.Fatal(err)
	}
	_, err := f.store.ApprovalDetail(context.Background(), parked.ApprovalID)
	if !errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
		t.Errorf("detail error = %v, want %v", err, ErrAuthorizationSnapshotCorrupt)
	}
}

// TestApprovalDetail_ShowsTheParkedSnapshotNotTheLiveBudget attacks FR-UI-03:
// the detail shows what was TRUE WHEN THE REQUEST WAS PARKED, and says so; it
// never shows the live state of the budget as if it were the parked one. The
// request is parked with five starts remaining; two ordinary strict starts then
// spend the same intent and the same grant; the detail must still say five,
// under the facts the store itself wrote at the park — requester, actor,
// intent, purpose, chain. Without the spending in between, live and parked are
// the same number and no mould can tell them apart (the adversary's pass over
// this phase, F4c, its probe P-I and its mutation M4).
//
// Evidence level: in-process, one real SQLite store; the park and the two
// starts go through the production doors.
// Probing mutation executed: the adversary's M4 — the detail overwrites the
// verified snapshot's remainder with the live remainder of the intent account —
// red with «budget kind = "finite", remaining = 3, want the parked 5».
func TestApprovalDetail_ShowsTheParkedSnapshotNotTheLiveBudget(t *testing.T) {
	f, parked := authorityApprovalFixture(t)
	for i := 1; i <= 2; i++ {
		if _, err := f.store.StartAuthorization(context.Background(),
			authorityStartRequest(f, "", f.now.Add(time.Duration(i)*time.Second))); err != nil {
			t.Fatalf("live start %d after the park: %v", i, err)
		}
	}
	if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM authorization_starts`); n != 2 {
		t.Fatalf("live starts committed = %d, want 2: nothing was spent, this mould would prove nothing", n)
	}
	got, err := f.store.ApprovalDetail(context.Background(), parked.ApprovalID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Authority == nil {
		t.Fatal("authority snapshot is absent")
	}
	a := got.Authority
	if a.BudgetKind != action.AuthorizationBudgetFinite || a.BudgetRemaining == nil || *a.BudgetRemaining != 5 {
		remaining := "absent"
		if a.BudgetRemaining != nil {
			remaining = fmt.Sprint(*a.BudgetRemaining)
		}
		t.Errorf("budget kind = %q, remaining = %s, want the parked 5", a.BudgetKind, remaining)
	}
	if a.Kind != action.AuthorizationSnapshotPending || a.ActionID != parked.ActionID || a.ApprovalID != parked.ApprovalID {
		t.Errorf("snapshot = %q for %q / %q, want the pending one of %q / %q",
			a.Kind, a.ActionID, a.ApprovalID, parked.ActionID, parked.ApprovalID)
	}
	if a.RequesterPrincipalID != parked.Evidence.RequesterPrincipalID || a.ActorPrincipalID != parked.Evidence.ActorPrincipalID {
		t.Errorf("requester / actor = %q / %q, want the parked evidence's %q / %q", a.RequesterPrincipalID,
			a.ActorPrincipalID, parked.Evidence.RequesterPrincipalID, parked.Evidence.ActorPrincipalID)
	}
	if a.IdentityEvidenceDigest == "" {
		t.Error("the snapshot does not bind the signed identity evidence")
	}
	if a.IntentID != f.intent.IntentID || a.IntentVersion != f.intent.Version ||
		a.IntentDigest != f.intent.Digest() || a.IntentPurpose != f.intent.Purpose {
		t.Errorf("intent = %q v%d %q %q, want the fixture's", a.IntentID, a.IntentVersion, a.IntentDigest, a.IntentPurpose)
	}
	if want := []string{f.root.SubjectPrincipalID}; !sameStringSlice(a.PrincipalChain, want) {
		t.Errorf("principal chain = %v, want %v", a.PrincipalChain, want)
	}
}

func TestApprovalDetail_ActivatedLedgerPreventsLegacyDowngrade(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 5)
	reason := "activate strict approval evidence"
	manifest, err := f.store.AuthorityActivationManifest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	act := authorityActorAct(t, f.store, f.resolver, f.issuer, "activate",
		CanonicalAuthorityActivation(f.intent.ProfileID, manifest, reason), f.now)
	digest, err := f.store.ActivateAuthority(context.Background(), f.intent.ProfileID,
		act, reason, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.RequireAuthorityActivation(context.Background(),
		f.intent.ProfileID, digest); err != nil {
		t.Fatal(err)
	}
	parked := parkStrictAuthorityFixture(t, f)

	// RE-APUNTADO 2026-09-23. This mould used to downgrade the row by setting
	// `authority_snapshot_required` to 0 and deleting the snapshot — which is
	// now ALSO refused by the id/marker pair check in `strictMarkerTx`, with the
	// SAME sentinel. The adversary captured the consequence: with that fixture
	// the activation-ledger guard could be deleted outright and this mould still
	// passed, so it no longer tested what its name says.
	//
	// The row is left COHERENT — `apr3_` id, marker 1, snapshot present — and
	// the attack moves to the birth ledger the activation guard replays. Only
	// that guard can refuse this state, so only its removal can redden here.
	if _, err := f.store.db.Exec(`DELETE FROM approval_birth_events WHERE approval_id=?`,
		parked.ApprovalID); err != nil {
		t.Fatal(err)
	}
	_, err = f.store.ApprovalDetail(context.Background(), parked.ApprovalID)
	if !errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
		t.Fatalf("a ledger missing this approval's birth event was served: %v", err)
	}
}

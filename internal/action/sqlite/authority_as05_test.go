// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// TestAuthority_DelegateAndRevokeSerialize (AS-AUTH-05) attacks the window
// between what a delegation KNOWS about its parent and what is true when the
// child is written. A sequential test — revoke, then call delegate — cannot
// see that window: a door that read the parent BEFORE asking for write
// ownership would pass it too, because in a sequential test the revocation is
// already there whenever the read happens.
//
// So the delegation is put IN FLIGHT first and parked at the last instant
// before it requests write ownership. While it is parked:
//
//  1. the parent is read on the delegating pool and is ACTIVE — the exact fact
//     a pre-transaction read would have captured, which proves the window is
//     real rather than assumed;
//  2. a SECOND real pool revokes the parent, and the revocation is CONFIRMED
//     committed by a THIRD, independent connection;
//  3. only then is the delegation released.
//
// The demanded outcome is exact: ErrAuthorityRevoked, and nothing of the child
// anywhere — no terms, no head, no lifecycle event, no budget account — and the
// one-shot delegate act still unconsumed, because the transaction that would
// have consumed it rolled back.
//
// Evidence level: MULTIPLE REAL SQLITE CONNECTIONS — two independent store
// pools plus a third observer connection on the same file — with a real
// barrier; in-process, no child OS process.
// Probing mutation executed: read the parent chain BEFORE beginAuthorityWrite
// and reuse it instead of re-reading inside the writer ("restore the
// pre-transaction parent read", the spec's own mutation). The stale ACTIVE
// parent then authorizes the child and this mould reddens with «error =
// <nil>».
func TestAuthority_DelegateAndRevokeSerialize(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 10)
	revoker, err := Open(f.store.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = revoker.Close() })
	revoker.authoritySigner = f.store.authoritySigner
	observer, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(f.store.path)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = observer.Close() })

	child := f.root
	child.GrantID, child.ParentGrantID, child.ParentGrantVersion = "grant_after_revoke", f.root.GrantID, f.root.Version
	child.IssuerPrincipalID, child.SubjectPrincipalID = f.root.SubjectPrincipalID, "principal_worker"
	child.DelegationDepthRemaining = f.root.DelegationDepthRemaining - 1
	delegateAct := authorityActorAct(t, f.store, f.resolver, f.issuer,
		"delegate", child.CanonicalBytes(), f.now.Add(time.Nanosecond))
	revokeAct := authorityActorAct(t, f.store, f.resolver, f.issuer,
		"revoke", CanonicalAuthorityRevoke(f.root.GrantID, "stop"), f.now.Add(2*time.Nanosecond))

	parked, release := make(chan struct{}), make(chan struct{})
	f.store.authorityBeforeWriter = func() {
		close(parked)
		<-release
	}
	delegated := make(chan error, 1)
	go func() {
		delegated <- f.store.DelegateAuthority(context.Background(), child, delegateAct, f.now.Add(4*time.Nanosecond))
	}()
	select {
	case <-parked:
	case err := <-delegated:
		t.Fatalf("delegation finished before reaching the writer barrier: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("delegation never reached the writer barrier")
	}
	// From here the barrier must always be lifted, or a failing assert would
	// leak a goroutine parked forever.
	released := false
	liftBarrier := func() {
		if !released {
			released = true
			close(release)
		}
	}
	t.Cleanup(liftBarrier)

	// 1. The prior read: with the delegation already in flight, its own pool
	// still sees an ACTIVE parent.
	var before string
	if err := f.store.db.QueryRow(`SELECT status FROM grant_heads WHERE grant_id=?`, f.root.GrantID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before != string(action.LifecycleActive) {
		t.Fatalf("parent status at the barrier = %q, want ACTIVE: the window this mould attacks does not exist", before)
	}

	// 2. A second real pool revokes, and a third connection confirms the commit.
	if err := revoker.RevokeAuthority(context.Background(), f.root.GrantID, revokeAct, "stop", f.now.Add(3*time.Nanosecond)); err != nil {
		t.Fatalf("revocation on the second pool: %v", err)
	}
	var after string
	if err := observer.QueryRow(`SELECT status FROM grant_heads WHERE grant_id=?`, f.root.GrantID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != string(action.LifecycleRevoked) {
		t.Fatalf("parent status seen by the observer = %q, want REVOKED: the revocation is not confirmed committed", after)
	}

	// 3. Only now does the delegation continue.
	liftBarrier()
	select {
	case err := <-delegated:
		// Errorf, not Fatalf: under the probing mutation the forbidden state
		// itself — the child born — must be OBSERVED by the row probes below,
		// not merely inferred from a nil error.
		if !errors.Is(err, ErrAuthorityRevoked) {
			t.Errorf("error = %v, want %v", err, ErrAuthorityRevoked)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("delegation never returned after the barrier was lifted")
	}

	for _, probe := range []struct{ what, query, arg string }{
		{"child terms", `SELECT COUNT(*) FROM grant_versions WHERE grant_id=?`, child.GrantID},
		{"child head", `SELECT COUNT(*) FROM grant_heads WHERE grant_id=?`, child.GrantID},
		{"child lifecycle events", `SELECT COUNT(*) FROM grant_events WHERE grant_id=?`, child.GrantID},
		{"child budget account", `SELECT COUNT(*) FROM budget_accounts WHERE account_id=?`,
			budgetAccountID(child.ProfileID, "grant", child.GrantID)},
		{"consumed delegate act", `SELECT COUNT(*) FROM grant_events WHERE actor_action_id=?`, delegateAct},
	} {
		var n int
		if err := observer.QueryRow(probe.query, probe.arg).Scan(&n); err != nil {
			t.Fatalf("%s: %v", probe.what, err)
		}
		if n != 0 {
			t.Errorf("%s = %d rows, want 0: the refused delegation left something behind", probe.what, n)
		}
	}
}

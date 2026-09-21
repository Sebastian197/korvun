// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// authorityBarrier parks ONE protected door of store at the last instant before
// it asks SQLite for write ownership. It is one-shot: the first door through is
// parked, every later call on the same store passes, so a mould can keep using
// the store after the barrier has done its job.
type authorityBarrier struct {
	parked  chan struct{}
	release chan struct{}
	lift    sync.Once
}

func armAuthorityBarrier(t *testing.T, store *Store) *authorityBarrier {
	t.Helper()
	b := &authorityBarrier{parked: make(chan struct{}), release: make(chan struct{})}
	var first sync.Once
	store.authorityBeforeWriter = func() {
		first.Do(func() {
			close(b.parked)
			<-b.release
		})
	}
	// A failing assert must never leave a goroutine parked forever.
	t.Cleanup(b.Lift)
	return b
}

// Lift lets the parked door continue. Safe to call more than once.
func (b *authorityBarrier) Lift() { b.lift.Do(func() { close(b.release) }) }

// AwaitParked blocks until the door is parked, failing if it finishes first.
func (b *authorityBarrier) AwaitParked(t *testing.T, finished <-chan error) {
	t.Helper()
	select {
	case <-b.parked:
	case err := <-finished:
		t.Fatalf("the door finished before reaching the writer barrier: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the door never reached the writer barrier")
	}
}

// authoritySecondPool opens an independent pool on f's file, able to sign.
func authoritySecondPool(t *testing.T, f authoritySQLiteFixture) *Store {
	t.Helper()
	second, err := Open(f.store.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	second.authoritySigner = f.store.authoritySigner
	second.SetIntentV2Signer(
		func(c action.IntentContractV2) action.SignedIntentContractV2 {
			return action.SignIntentContractV2(f.private, c)
		},
		func(e action.IntentEventV1) action.SignedIntentEventV1 {
			return action.SignIntentEventV1(f.private, e)
		},
	)
	return second
}

// authorityObserver is a THIRD connection that belongs to neither pool: what it
// reads is what any outsider would read, which is what "committed" means.
func authorityObserver(t *testing.T, f authoritySQLiteFixture) *sql.DB {
	t.Helper()
	observer, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(f.store.path)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = observer.Close() })
	return observer
}

func authorityObserved(t *testing.T, observer *sql.DB, query string, args ...any) string {
	t.Helper()
	var value string
	if err := observer.QueryRow(query, args...).Scan(&value); err != nil {
		t.Fatalf("observer %q: %v", query, err)
	}
	return value
}

// TestAuthority_IssueReadsIntentInsideWriter is AS-AUTH-05's sister at the ISSUE
// door: the stale fact there is the INTENT. A root issuance is put in flight and
// parked before it takes write ownership; its own pool still reads the intent
// ACTIVE; a second real pool revokes the intent and a third connection confirms
// the commit; only then does the issuance continue.
//
// Demanded: action.ErrIntentRevoked, and nothing of the grant anywhere, the
// one-shot issue act included.
//
// Evidence level: MULTIPLE REAL SQLITE CONNECTIONS (two store pools and a third
// observer) with a real barrier; in-process.
// Probing mutation executed: read the intent in a throwaway transaction BEFORE
// beginAuthorityWrite and reuse it — the stale ACTIVE intent then issues the
// grant, and this mould reddens with «error = <nil>» and the grant's rows
// observed.
func TestAuthority_IssueReadsIntentInsideWriter(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 10)
	second, observer := authoritySecondPool(t, f), authorityObserver(t, f)

	grant := f.root
	grant.GrantID = "grant_issued_across_a_revocation"
	issueAct := authorityActorAct(t, f.store, f.resolver, f.issuer, "issue", grant.CanonicalBytes(), f.now.Add(7*time.Nanosecond))

	barrier := armAuthorityBarrier(t, f.store)
	issued := make(chan error, 1)
	go func() {
		issued <- f.store.IssueAuthority(context.Background(), grant, issueAct, f.now.Add(time.Second))
	}()
	barrier.AwaitParked(t, issued)

	var before string
	if err := f.store.db.QueryRow(`SELECT status FROM intent_heads WHERE intent_id=?`, f.intent.IntentID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before != string(action.LifecycleActive) {
		t.Fatalf("intent status at the barrier = %q, want ACTIVE: the window this mould attacks does not exist", before)
	}
	if err := second.RevokeIntentV2(context.Background(), f.intent.IntentID, f.intent.OwnerPrincipalID, f.now.Add(500*time.Millisecond)); err != nil {
		t.Fatalf("intent revocation on the second pool: %v", err)
	}
	if after := authorityObserved(t, observer, `SELECT status FROM intent_heads WHERE intent_id=?`, f.intent.IntentID); after != string(action.LifecycleRevoked) {
		t.Fatalf("intent status seen by the observer = %q, want REVOKED", after)
	}

	barrier.Lift()
	select {
	case err := <-issued:
		if !errors.Is(err, action.ErrIntentRevoked) {
			t.Errorf("error = %v, want %v", err, action.ErrIntentRevoked)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("issuance never returned after the barrier was lifted")
	}
	for what, query := range map[string]string{
		"grant terms":        `SELECT COUNT(*) FROM grant_versions WHERE grant_id='grant_issued_across_a_revocation'`,
		"grant head":         `SELECT COUNT(*) FROM grant_heads WHERE grant_id='grant_issued_across_a_revocation'`,
		"grant events":       `SELECT COUNT(*) FROM grant_events WHERE grant_id='grant_issued_across_a_revocation'`,
		"consumed issue act": `SELECT COUNT(*) FROM grant_events WHERE actor_action_id='` + issueAct + `'`,
	} {
		if n := authorityObserved(t, observer, query); n != "0" {
			t.Errorf("%s = %s rows, want 0: the refused issuance left something behind", what, n)
		}
	}
}

// TestAuthority_StartAndRevokeFollowCommitOrder proves revocation A in BOTH
// orders, each with a real barrier — because one order alone proves nothing: a
// store that ALWAYS refused would pass "revoke first", and one that NEVER looked
// would pass "start first".
//
// Evidence level: MULTIPLE REAL SQLITE CONNECTIONS (two store pools and a third
// observer) with real barriers; in-process.
// Probing mutations executed, each alone: (revoke first) judge the chain's
// lifecycle from a read taken BEFORE write ownership — the stale ACTIVE leaf
// then starts, red with «error = <nil>» and a durable start observed; (start
// first) re-judge the chain AFTER the commit and refuse on what it finds — the
// committed capability is then withdrawn, red with «error = authority revoked».
func TestAuthority_StartAndRevokeFollowCommitOrder(t *testing.T) {
	t.Run("the revocation commits first: the start is refused and nothing is spent", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 10)
		second, observer := authoritySecondPool(t, f), authorityObserver(t, f)
		revokeAct := authorityActorAct(t, f.store, f.resolver, f.issuer, "revoke",
			CanonicalAuthorityRevoke(f.root.GrantID, "stop"), f.now.Add(3*time.Nanosecond))

		barrier := armAuthorityBarrier(t, f.store)
		started := make(chan error, 1)
		go func() {
			_, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now.Add(time.Second)))
			started <- err
		}()
		barrier.AwaitParked(t, started)

		if before := authorityObserved(t, observer, `SELECT status FROM grant_heads WHERE grant_id=?`, f.root.GrantID); before != string(action.LifecycleActive) {
			t.Fatalf("leaf status at the barrier = %q, want ACTIVE", before)
		}
		if err := second.RevokeAuthority(context.Background(), f.root.GrantID, revokeAct, "stop", f.now.Add(500*time.Millisecond)); err != nil {
			t.Fatalf("revocation on the second pool: %v", err)
		}
		if after := authorityObserved(t, observer, `SELECT status FROM grant_heads WHERE grant_id=?`, f.root.GrantID); after != string(action.LifecycleRevoked) {
			t.Fatalf("leaf status seen by the observer = %q, want REVOKED", after)
		}

		barrier.Lift()
		select {
		case err := <-started:
			if !errors.Is(err, ErrAuthorityRevoked) {
				t.Errorf("error = %v, want %v", err, ErrAuthorityRevoked)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("start never returned after the barrier was lifted")
		}
		for what, query := range map[string]string{
			"durable starts": `SELECT COUNT(*) FROM authorization_starts`,
			"budget debits":  `SELECT COUNT(*) FROM budget_debits`,
			"strict actions": `SELECT COUNT(*) FROM actions WHERE action_id LIKE 'act3_%'`,
		} {
			if n := authorityObserved(t, observer, query); n != "0" {
				t.Errorf("%s = %s, want 0: a start ordered after the revocation left something behind", what, n)
			}
		}
	})

	t.Run("the start commits first: its capability stands, and the NEXT start is refused", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 10)
		second, observer := authoritySecondPool(t, f), authorityObserver(t, f)
		revokeAct := authorityActorAct(t, f.store, f.resolver, f.issuer, "revoke",
			CanonicalAuthorityRevoke(f.root.GrantID, "stop"), f.now.Add(3*time.Nanosecond))

		// The start is parked AFTER its commit and BEFORE it returns — the only
		// window in which a later revocation could tempt a store into
		// withdrawing a capability it has already made durable.
		committed, release := make(chan struct{}), make(chan struct{})
		var lift sync.Once
		liftBarrier := func() { lift.Do(func() { close(release) }) }
		t.Cleanup(liftBarrier)
		request := authorityStartRequest(f, "", f.now.Add(time.Second))
		request.Probe = func(probe AuthorityStartProbe) error {
			if probe == AuthorityProbeAfterCommitBeforeReturn {
				close(committed)
				<-release
			}
			return nil
		}
		type outcome struct {
			result AuthorityStartResult
			err    error
		}
		started := make(chan outcome, 1)
		go func() {
			result, err := f.store.StartAuthorization(context.Background(), request)
			started <- outcome{result, err}
		}()
		select {
		case <-committed:
		case o := <-started:
			t.Fatalf("start returned before its post-commit probe: %v", o.err)
		case <-time.After(10 * time.Second):
			t.Fatal("start never reached its post-commit probe")
		}
		if n := authorityObserved(t, observer, `SELECT COUNT(*) FROM authorization_starts`); n != "1" {
			t.Fatalf("durable starts seen by the observer at the probe = %s, want 1: the start is not confirmed committed", n)
		}
		if err := second.RevokeAuthority(context.Background(), f.root.GrantID, revokeAct, "stop", f.now.Add(2*time.Second)); err != nil {
			t.Fatalf("revocation on the second pool: %v", err)
		}
		if after := authorityObserved(t, observer, `SELECT status FROM grant_heads WHERE grant_id=?`, f.root.GrantID); after != string(action.LifecycleRevoked) {
			t.Fatalf("leaf status seen by the observer = %q, want REVOKED", after)
		}

		liftBarrier()
		var first outcome
		select {
		case first = <-started:
		case <-time.After(30 * time.Second):
			t.Fatal("start never returned after the barrier was lifted")
		}
		if first.err != nil || first.result.ActionID == "" {
			t.Fatalf("committed start = %+v, %v; want its capability: a later revocation cannot retract a committed start", first.result, first.err)
		}
		if state := authorityObserved(t, observer, `SELECT state FROM actions WHERE action_id=?`, first.result.ActionID); state != string(action.StateAuthorized) {
			t.Errorf("committed action state = %q, want %q", state, action.StateAuthorized)
		}
		if n := authorityObserved(t, observer, `SELECT COUNT(*) FROM budget_debits WHERE action_id=?`, first.result.ActionID); n == "0" {
			t.Error("the committed start kept no debit")
		}
		// And the revocation is real: the very next start is refused.
		if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now.Add(3*time.Second))); !errors.Is(err, ErrAuthorityRevoked) {
			t.Errorf("next start error = %v, want %v", err, ErrAuthorityRevoked)
		}
		if n := authorityObserved(t, observer, `SELECT COUNT(*) FROM authorization_starts`); n != "1" {
			t.Errorf("durable starts after the refused one = %s, want 1", n)
		}
	})
}

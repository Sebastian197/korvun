// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

// AS07's harness: the fixture of TestAuthority_ConcurrentStartsShareAncestorBudget
// (a shared ancestor grant with a budget of 12, two sibling grants under it, two
// real connections to one ledger) and the test-side driver that conducts one start
// until it resolves. The driver is where the TEST — never the door — retries a start
// the door answered with ErrLedgerBusy: the declared cure of 2026-09-29. The door is
// unchanged; ledger_busy stays its answer.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/identity"
)

const (
	as07Maximum     = 12              // the shared ancestor grant's budget
	as07Starts      = 4 * as07Maximum // the concurrent starts
	as07MaxAttempts = 20              // the test's cap on attempts per start
)

// as07Outcome is how one start ended once driven.
type as07Outcome string

const (
	as07Committed as07Outcome = "committed"
	as07Exhausted as07Outcome = "exhausted"
	as07Busy      as07Outcome = "busy"
	as07Other     as07Outcome = "other"
)

// as07Result is one driven start: its outcome, the attempts it took, how many of
// them the door answered busy, and the last error.
type as07Result struct {
	outcome  as07Outcome
	attempts int
	busy     int
	err      error
}

// errAS07StillBusy is the class of as07Check's verdict on a start that was
// still answered ErrLedgerBusy after the test's attempts: a mould tells it apart
// by errors.Is, never by the text, which the door's own sentinel also carries.
var errAS07StillBusy = errors.New("still ledger_busy after the test's attempts")

type as07Fixture struct {
	f      authoritySQLiteFixture
	second *Store
	common action.AuthorityGrantV2
}

// newAS07Fixture builds the grant chain (root → common ancestor → two siblings,
// the ancestor and each sibling with a budget of 12), binds each sibling to its
// conversation, and opens a second real connection to the same ledger.
func newAS07Fixture(t *testing.T) as07Fixture {
	t.Helper()
	f := newAuthoritySQLiteFixture(t, as07Maximum*2)
	common := f.delegate(t, "grant_common_ancestor", f.root.SubjectPrincipalID, as07Maximum, f.root)
	left := f.delegate(t, "grant_sibling_left", f.root.SubjectPrincipalID, as07Maximum, common)
	right := f.delegate(t, "grant_sibling_right", f.root.SubjectPrincipalID, as07Maximum, common)
	for _, binding := range []action.ExecutionBinding{
		{BindingID: "binding_sibling_left", ActorPrincipalID: f.root.SubjectPrincipalID,
			Channel: "webhook", ConversationID: "left", IntentID: f.intent.IntentID,
			IntentVersion: f.intent.Version, IntentDigest: f.intent.Digest(), GrantID: left.GrantID,
			GrantVersion: left.Version, GrantDigest: left.Digest(), Revision: 1, Status: action.BindingActive},
		{BindingID: "binding_sibling_right", ActorPrincipalID: f.root.SubjectPrincipalID,
			Channel: "webhook", ConversationID: "right", IntentID: f.intent.IntentID,
			IntentVersion: f.intent.Version, IntentDigest: f.intent.Digest(), GrantID: right.GrantID,
			GrantVersion: right.Version, GrantDigest: right.Digest(), Revision: 1, Status: action.BindingActive},
	} {
		if err := f.store.PutExecutionBinding(context.Background(), binding); err != nil {
			t.Fatal(err)
		}
	}
	second, err := openFull(f.store.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	second.authoritySigner = f.store.authoritySigner
	second.SetIdentitySigners(
		func(e identity.Evidence) identity.SignedEvidence { return identity.SignEvidence(f.private, e) },
		func(e identity.PrincipalEvent) identity.SignedPrincipalEvent {
			return identity.SignPrincipalEvent(f.private, e)
		},
	)
	return as07Fixture{f: f, second: second, common: common}
}

// start is ONE real attempt of start i at the door: even starts on the fixture's
// own connection and the conversation «left», odd ones on the second connection
// and «right».
func (x as07Fixture) start(i int) error {
	store, conversation := x.f.store, "left"
	if i%2 == 1 {
		store, conversation = x.second, "right"
	}
	_, err := store.StartAuthorization(context.Background(), authorityStartRequest(x.f, conversation, x.f.now))
	return err
}

// as07Drive conducts one start until it resolves: nil is committed and
// ErrBudgetExhausted is exhausted. An ErrLedgerBusy (errors.Is) is the door's
// honest «not now», so the start is attempted again, at most maxAttempts times in
// all; past that it ends busy, a class of its own. Any other error ends the start
// at once, as itself.
func as07Drive(maxAttempts int, attempt func() error) as07Result {
	r := as07Result{}
	for r.attempts < maxAttempts {
		r.attempts++
		r.err = attempt()
		switch {
		case r.err == nil:
			r.outcome = as07Committed
			return r
		case errors.Is(r.err, ErrBudgetExhausted):
			r.outcome = as07Exhausted
			return r
		case errors.Is(r.err, ErrLedgerBusy):
			r.busy++
		default:
			r.outcome = as07Other
			return r
		}
	}
	r.outcome = as07Busy
	return r
}

// runAll drives the starts concurrently. A start already in pre keeps that result
// and is not run again; attemptFor, when it returns a function for a start, replaces
// that start's attempt (a mould's lying dependency).
func (x as07Fixture) runAll(pre map[int]as07Result, attemptFor func(i int) func() error) []as07Result {
	results := make([]as07Result, as07Starts)
	var wg sync.WaitGroup
	for i := 0; i < as07Starts; i++ {
		if r, ok := pre[i]; ok {
			results[i] = r
			continue
		}
		attempt := func() error { return x.start(i) }
		if attemptFor != nil {
			if replaced := attemptFor(i); replaced != nil {
				attempt = replaced
			}
		}
		wg.Add(1)
		go func(i int, attempt func() error) {
			defer wg.Done()
			results[i] = as07Drive(as07MaxAttempts, attempt)
		}(i, attempt)
	}
	wg.Wait()
	return results
}

// as07Check judges the driven starts. A start that ended busy, or with any other
// error, is named FIRST — a busy never passes as the exhaustion it displaced —
// and only then are the counts compared, exactly. The verdict on a failing start
// wraps its error, and only a start still busy after the test's attempts also
// carries errAS07StillBusy.
func as07Check(results []as07Result) error {
	committed, exhausted := 0, 0
	for i, r := range results {
		switch r.outcome {
		case as07Committed:
			committed++
		case as07Exhausted:
			exhausted++
		case as07Busy:
			return fmt.Errorf("start %d: %w (%d): %w", i, errAS07StillBusy, r.attempts, r.err)
		default:
			return fmt.Errorf("start %d: %w", i, r.err)
		}
	}
	if committed != as07Maximum || exhausted != 3*as07Maximum {
		return fmt.Errorf("committed=%d exhausted=%d, want %d and %d", committed, exhausted, as07Maximum, 3*as07Maximum)
	}
	return nil
}

// busyRetries adds up the attempts the door answered busy.
func busyRetries(results []as07Result) int {
	n := 0
	for _, r := range results {
		n += r.busy
	}
	return n
}

// sharedAccount reads the shared ancestor's total counter from the ledger itself:
// the account's max_total and what has been spent against it.
func (x as07Fixture) sharedAccount(t *testing.T) (maxTotal, spent int64) {
	t.Helper()
	account := budgetAccountID(x.common.ProfileID, "grant", x.common.GrantID)
	if err := x.f.store.db.QueryRow(`SELECT max_total FROM budget_accounts WHERE account_id=?`, account).Scan(&maxTotal); err != nil {
		t.Fatalf("the shared account's max_total: %v", err)
	}
	if err := x.f.store.db.QueryRow(`SELECT spent FROM budget_counters WHERE account_id=? AND operation_key='*'`, account).Scan(&spent); err != nil {
		t.Fatalf("the shared account's total counter: %v", err)
	}
	return maxTotal, spent
}

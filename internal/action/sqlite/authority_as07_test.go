// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// AS07 (v0.16.1) · concurrent starts share an ancestor's budget: 48 starts on two
// real connections to one ledger, under a shared ancestor grant of 12, end with
// exactly 12 committed and 36 exhausted, and the ledger's shared account never
// goes below zero.
//
// Declared cure (2026-09-29): each start is DRIVEN until it resolves — the test,
// never the door, retries a start the door answered with ErrLedgerBusy, with at
// most as07MaxAttempts attempts in all (as07Drive). The door is unchanged:
// ledger_busy stays its honest answer. The test fell three times on that answer
// before the cure, each time with committed=12 exhausted=35: in the coverage pass
// of the local make quality of 2026-09-27 (20.67 s, start 0), and twice in
// master's Windows CI of 2026-09-29, run 36520026213 (11.84 s, start 26; then
// its rerun, 14.22 s, start 1). The busy retries are logged — go test shows them
// with -v, or when the test fails — and never asserted.
//
// PROBING MUTATIONS: the shared ancestor's account left undebited — alone, or
// with every account but the start's own sibling — → committed=24 → reddens. The
// exhaustion check removed on every account → the store's next guard
// (remainingBeforeAccountTx) stops the run at 13 committed, the other 35 starts
// failing as «budget evidence corrupt» → reddens by that name; with the count
// check neutralised as well, the ledger assertion alone reddens on spent=13 of
// max_total=12. A start that commits but is reported busy once — a debit the
// count never sees — with the count check neutralised → the ledger assertion
// reddens on spent=12 with 11 committed.
//
// Evidence level: real connections in process (two stores on one real file).
func TestAuthority_ConcurrentStartsShareAncestorBudget(t *testing.T) {
	x := newAS07Fixture(t)
	results := x.runAll(nil, nil)
	t.Logf("busy retries across the %d starts: %d", as07Starts, busyRetries(results))
	if err := as07Check(results); err != nil {
		t.Fatal(err)
	}
	committed := 0
	for _, r := range results {
		if r.outcome == as07Committed {
			committed++
		}
	}
	maxTotal, spent := x.sharedAccount(t)
	if spent != int64(committed) || spent != as07Maximum || maxTotal-spent < 0 {
		t.Fatalf("the shared account in the ledger: spent=%d of max_total=%d with %d starts committed, want spent=%d, one debit per committed start, never below zero",
			spent, maxTotal, committed, as07Maximum)
	}
}

// Mould 1 · the failure, reproduced: a REAL busy — a third connection holds BEGIN
// IMMEDIATE past the busy timeout while start 26 (the start master's Windows CI
// saw busy on its first run) makes its first attempt — does not bring the test
// down. The lock is let go the moment that attempt returns; the test retries the
// start, and the run still ends 12/36 with the ledger's shared account at 12.
//
// PROBING MUTATIONS: as07Drive without its retry → start 26 ends busy → reddens,
// naming ledger_busy; a busy counted as exhausted or as other → reddens on start
// 26's outcome, and counted as committed → on the count (13 committed); the raw
// lock never taken → reddens on the first attempt; the release's ROLLBACK
// failing → reddens by that name.
//
// Evidence level: three real connections in process; the busy is the door's own,
// after the busy timeout.
func TestAuthority_AS07_aRealBusyOnOneStartIsRetriedByTheTest(t *testing.T) {
	const n = 26
	x := newAS07Fixture(t)
	ctx := context.Background()
	conn, err := rawConn(t, x.f.store).Conn(ctx)
	if err != nil {
		t.Fatalf("raw conn: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		t.Fatalf("raw BEGIN IMMEDIATE: %v", err)
	}
	held := true
	release := func() {
		if held {
			held = false
			if _, err := conn.ExecContext(ctx, `ROLLBACK`); err != nil {
				t.Errorf("release the raw BEGIN IMMEDIATE: %v", err)
			}
			_ = conn.Close()
		}
	}
	t.Cleanup(release)
	var first error
	attempts := 0
	driven := as07Drive(as07MaxAttempts, func() error {
		attempts++
		err := x.start(n)
		if attempts == 1 {
			first = err
			release()
		}
		return err
	})
	if !errors.Is(first, ErrLedgerBusy) {
		t.Fatalf("start %d's first attempt behind the held lock = %v, want the door's ErrLedgerBusy", n, first)
	}
	// Start n runs alone, before the others, with the budget whole: once retried
	// it commits. A busy counted as anything else — exhausted, above all — reddens
	// this mould too, not only mould 2.
	if driven.outcome != as07Committed {
		t.Fatalf("start %d after its retry = %s (%d attempts): %v, want committed", n, driven.outcome, driven.attempts, driven.err)
	}
	results := x.runAll(map[int]as07Result{n: driven}, nil)
	t.Logf("busy retries across the %d starts: %d", as07Starts, busyRetries(results))
	if err := as07Check(results); err != nil {
		t.Fatal(err)
	}
	if maxTotal, spent := x.sharedAccount(t); spent != as07Maximum || maxTotal-spent < 0 {
		t.Fatalf("the shared account in the ledger: spent=%d of max_total=%d, want spent=%d", spent, maxTotal, as07Maximum)
	}
}

// Mould 2 · the adjacent attack: the LAST start answered busy on every attempt,
// past the cap. A cure that counted busy as exhausted would land exactly on 12/36
// here — the busy took the place of an exhaustion that was due — so the verdict
// must name ledger_busy, the driver must have made exactly as07MaxAttempts
// attempts, and the other 47 starts must still count 12 committed and 35
// exhausted.
//
// PROBING MUTATIONS: busy past the cap counted as exhausted → reddens; the cap off
// by one → reddens on the attempts; the checker's busy verdict folded into its
// «any other error» one → reddens on the class.
//
// Evidence level: the test's own driver over a lying attempt for the last start (a
// synthetic ErrLedgerBusy, wrapped the way the door wraps it); the other 47 go to
// the real store on two real connections.
func TestAuthority_AS07_busyPastTheCapOnTheLastStartIsNamedBusy(t *testing.T) {
	last := as07Starts - 1
	x := newAS07Fixture(t)
	lying := fmt.Errorf("%w: %w", ErrAuthorityStoreBusy,
		fmt.Errorf("%w: database is locked (5) (SQLITE_BUSY)", ErrLedgerBusy))
	calls := 0
	results := x.runAll(nil, func(i int) func() error {
		if i != last {
			return nil
		}
		return func() error {
			calls++
			return lying
		}
	})
	r := results[last]
	if r.outcome != as07Busy || r.attempts != as07MaxAttempts || calls != as07MaxAttempts {
		t.Fatalf("the last start = %s after %d attempts (%d calls), want busy after exactly %d",
			r.outcome, r.attempts, calls, as07MaxAttempts)
	}
	err := as07Check(results)
	if !errors.Is(err, errAS07StillBusy) || strings.Contains(err.Error(), "exhausted=") {
		t.Fatalf("the verdict = %v, want its busy class (errAS07StillBusy) and not a count of exhausted", err)
	}
	committed, exhausted := 0, 0
	for i, r := range results {
		switch {
		case i == last:
		case r.outcome == as07Committed:
			committed++
		case r.outcome == as07Exhausted:
			exhausted++
		default:
			t.Fatalf("start %d = %s: %v", i, r.outcome, r.err)
		}
	}
	if committed != as07Maximum || exhausted != 3*as07Maximum-1 {
		t.Fatalf("the other %d starts: committed=%d exhausted=%d, want %d and %d",
			as07Starts-1, committed, exhausted, as07Maximum, 3*as07Maximum-1)
	}
}

// Mould 3 · the driver retries ErrLedgerBusy and nothing else. ErrLedgerBusy
// comes from the door in two chains — inside the store's busy class when BEGIN
// is busy, bare when a statement inside the transaction is — and in both the
// start is attempted again, then resolves. The door's other refusals end the
// start at once, as themselves: its busy class without ledger_busy, as the rows
// build it (a context already over when the start begins, a raw busy the door's
// own retry window gave up on, an interrupted statement), a corrupt budget, a
// plain error.
//
// PROBING MUTATIONS: every error retried → reddens; the retry keyed on
// ErrAuthorityStoreBusy instead of ErrLedgerBusy → reddens on the bare row and
// on the three rows of that class without ledger_busy; the retry requiring both
// classes → reddens on the bare row alone.
//
// Evidence level: unit, in process; the test's own driver over lying attempts.
func TestAuthority_AS07_theDriverRetriesOnlyLedgerBusy(t *testing.T) {
	ledgerBusy := fmt.Errorf("%w: %w", ErrAuthorityStoreBusy,
		fmt.Errorf("%w: database is locked (5) (SQLITE_BUSY)", ErrLedgerBusy))
	bareBusy := fmt.Errorf("%w: database is locked (5) (SQLITE_BUSY)", ErrLedgerBusy)
	for _, c := range []struct {
		name     string
		answers  []error // the door's answer to each attempt; the last one repeats
		outcome  as07Outcome
		attempts int
		busy     int
	}{
		{"a busy BEGIN, then committed", []error{ledgerBusy, nil}, as07Committed, 2, 1},
		{"a busy BEGIN, then exhausted", []error{ledgerBusy, ErrBudgetExhausted}, as07Exhausted, 2, 1},
		{"a busy statement, bare, then committed", []error{bareBusy, nil}, as07Committed, 2, 1},
		{"exhausted at once", []error{ErrBudgetExhausted}, as07Exhausted, 1, 0},
		{"the store busy from a context already over", []error{fmt.Errorf("%w: %w", ErrAuthorityStoreBusy, context.DeadlineExceeded)}, as07Other, 1, 0},
		{"the store busy from a raw busy its own retry window gave up on", []error{fmt.Errorf("%w: %w", ErrAuthorityStoreBusy, errors.New("database is locked (5) (SQLITE_BUSY)"))}, as07Other, 1, 0},
		{"the store busy from an interrupted statement", []error{fmt.Errorf("%w: %w", ErrAuthorityStoreBusy, errors.New("interrupted (9) (SQLITE_INTERRUPT)"))}, as07Other, 1, 0},
		{"a corrupt budget", []error{ErrBudgetEvidenceCorrupt}, as07Other, 1, 0},
		{"a plain error", []error{errors.New("disk I/O error")}, as07Other, 1, 0},
	} {
		calls := 0
		r := as07Drive(as07MaxAttempts, func() error {
			answer := c.answers[min(calls, len(c.answers)-1)]
			calls++
			return answer
		})
		if r.outcome != c.outcome || r.attempts != c.attempts || r.busy != c.busy || calls != c.attempts {
			t.Errorf("%s: %s after %d attempts (%d busy, %d calls), want %s after %d (%d busy)",
				c.name, r.outcome, r.attempts, r.busy, calls, c.outcome, c.attempts, c.busy)
		}
	}
}

// Mould 4 · the checker names each class. A start still busy after the test's
// attempts is judged with errAS07StillBusy and the door's error; any other
// failing start with its own error and never that class; once every start
// resolved, the counts alone, never as busy.
//
// PROBING MUTATIONS: the busy class put on every failing start → reddens on
// the other row; the busy verdict folded into «any other error» → reddens on
// the busy row; the counts comparison removed → reddens on the counts row.
//
// Evidence level: unit, in process; the test's own checker over built results.
func TestAuthority_AS07_theCheckerNamesEachClass(t *testing.T) {
	resolved := func() []as07Result {
		results := make([]as07Result, as07Starts)
		for i := range results {
			results[i].outcome = as07Exhausted
			if i < as07Maximum {
				results[i].outcome = as07Committed
			}
		}
		return results
	}
	busy := resolved()
	busy[5] = as07Result{outcome: as07Busy, attempts: as07MaxAttempts, busy: as07MaxAttempts,
		err: fmt.Errorf("%w: database is locked (5) (SQLITE_BUSY)", ErrLedgerBusy)}
	other := resolved()
	other[5] = as07Result{outcome: as07Other, attempts: 1, err: ErrBudgetEvidenceCorrupt}
	counts := resolved()
	counts[as07Maximum].outcome = as07Committed // 13 committed, 35 exhausted
	for _, c := range []struct {
		name    string
		results []as07Result
		want    error // a class the verdict must carry; nil for none
		busy    bool  // whether it must carry errAS07StillBusy
		fails   bool
	}{
		{"every start resolved, 12 and 36", resolved(), nil, false, false},
		{"a start still busy", busy, ErrLedgerBusy, true, true},
		{"a start that failed otherwise", other, ErrBudgetEvidenceCorrupt, false, true},
		{"every start resolved, 13 and 35", counts, nil, false, true},
	} {
		err := as07Check(c.results)
		if (err != nil) != c.fails || errors.Is(err, errAS07StillBusy) != c.busy ||
			(c.want != nil && !errors.Is(err, c.want)) {
			t.Errorf("%s: the verdict = %v, want failing=%t, busy class=%t, carrying %v",
				c.name, err, c.fails, c.busy, c.want)
		}
	}
}

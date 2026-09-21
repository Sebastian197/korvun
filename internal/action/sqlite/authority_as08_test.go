// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// TestAuthority_BusyIsNotBudgetExhaustion (AS-AUTH-08) holds the authority
// write lock from a REAL second connection and asks the door for a start. A
// store someone else is writing to is «busy» — never «budget exhausted», which
// would tell the caller its authority is spent — and nothing is debited.
//
// What this mould observes, said exactly (the adversary's pass over this phase,
// F8): the door gives up on ITS OWN 80 ms context deadline, so the class it
// sees arrives through the context branch of the classifier. It does not wait
// out the driver's busy timeout, and it would pass with the driver's own busy
// TEXT unclassified. That branch is pinned by
// TestAuthority_TheDriversOwnBusyIsClassifiedBusy.
//
// Evidence level: multiple real SQLite connections; one real external writer.
// Probing mutation executed: a busy store is reported as budget exhaustion —
// red with «error = action/sqlite: authority budget exhausted: context deadline
// exceeded».
func TestAuthority_BusyIsNotBudgetExhaustion(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 2)
	hand, err := sql.Open("sqlite", f.store.path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(0)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = hand.Close() }()
	tx, err := hand.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`UPDATE authority_write_lock SET revision=revision+1 WHERE singleton=1`); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	_, err = f.store.StartAuthorization(ctx, authorityStartRequest(f, "", f.now))
	if !errors.Is(err, ErrAuthorityStoreBusy) || errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("error = %v", err)
	}
	if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM budget_debits`); n != 0 {
		t.Fatalf("debits = %d", n)
	}
}

// TestAuthority_TheDriversOwnBusyIsClassifiedBusy hands the classifier the
// error THE PINNED DRIVER ITSELF returns for a locked database — obtained here
// from a real second writer with no busy timeout — not a string somebody typed.
// The only mould of that branch fed it a hand-written «database is locked», so
// a driver that worded its busy differently, or a classifier that stopped
// recognizing it, would have gone unseen.
//
// Evidence level: multiple real SQLite connections; the error value comes from
// the driver, and the classifier is called directly — NOT through a door: the
// door retries a busy store for thirty seconds before it gives up, and AS-AUTH-08
// above covers the door under a deadline.
// Probing mutation executed: the adversary's M2 — drop the driver-text
// classification and keep the context errors — red with «the driver's own busy
// was not classified busy».
func TestAuthority_TheDriversOwnBusyIsClassifiedBusy(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 2)
	open := func() *sql.DB {
		db, err := sql.Open("sqlite", f.store.path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(0)")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		return db
	}
	const lock = `UPDATE authority_write_lock SET revision=revision+1 WHERE singleton=1`
	holder, err := open().Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback() }()
	if _, err := holder.Exec(lock); err != nil {
		t.Fatal(err)
	}
	_, driverErr := open().Exec(lock)
	if driverErr == nil {
		t.Fatal("the second writer was not refused: there is no driver error to classify")
	}
	if errors.Is(driverErr, context.DeadlineExceeded) || errors.Is(driverErr, context.Canceled) {
		t.Fatalf("the refusal is a context error (%v), not the driver's busy: this mould would prove nothing", driverErr)
	}
	mapped := mapAuthorityStoreError(driverErr)
	if !errors.Is(mapped, ErrAuthorityStoreBusy) {
		t.Errorf("the driver's own busy was not classified busy: driver said %q, classifier answered %v", driverErr, mapped)
	}
	if !errors.Is(mapped, driverErr) {
		t.Errorf("the classified error dropped its cause: %v does not wrap %v", mapped, driverErr)
	}
}

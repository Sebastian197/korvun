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

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestAuthority_EverySignedLedgerRowIsHeldByItsSignature attacks the four
// signed ledgers of this phase the way a writer with the database and WITHOUT
// the profile key would: every column left coherent, only the signature no
// longer the key's. Column-against-bytes agreement cannot see that; the
// signature check is the only belt, and until this mould no test made it the
// one that had to answer. Each row names its refusal and proves the refused
// door consumed nothing.
//
// The ledgers, and the door that reads them: the last debit of an account and
// the account's signed counter head (the next start); a start proof and an
// approval-birth event (the strict boot's verification, through
// RequireAuthorityActivation).
//
// Evidence level: in-process, one real SQLite store; the rows are rewritten
// with SQL through the store's own connection.
// Probing mutations executed, each alone, one per row: the verifier accepts the
// debit's / the counter head's / the start proof's / the birth event's
// signature without checking it — red on that row with «error = <nil>».
func TestAuthority_EverySignedLedgerRowIsHeldByItsSignature(t *testing.T) {
	const forged = "00"
	committedStart := func(t *testing.T, f authoritySQLiteFixture) {
		t.Helper()
		if _, err := f.store.StartAuthorization(context.Background(),
			authorityStartRequest(f, "", f.now.Add(2*time.Second))); err != nil {
			t.Fatalf("the committed start whose evidence is attacked: %v", err)
		}
	}
	nextStartRefused := func(t *testing.T, f authoritySQLiteFixture, want error) {
		t.Helper()
		debits := authorityScalar(t, f.store, `SELECT COUNT(*) FROM budget_debits`)
		starts := authorityScalar(t, f.store, `SELECT COUNT(*) FROM authorization_starts`)
		_, err := f.store.StartAuthorization(context.Background(),
			authorityStartRequest(f, "", f.now.Add(3*time.Second)))
		if !errors.Is(err, want) {
			t.Errorf("the next start: error = %v, want %v", err, want)
		}
		if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM budget_debits`); n != debits {
			t.Errorf("budget debits after the refusal = %d, want the %d there were", n, debits)
		}
		if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM authorization_starts`); n != starts {
			t.Errorf("durable starts after the refusal = %d, want the %d there were", n, starts)
		}
	}

	t.Run("the last debit of an account", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 3)
		committedStart(t, f)
		if _, err := f.store.db.Exec(`UPDATE budget_debits SET signature=?`, forged); err != nil {
			t.Fatal(err)
		}
		nextStartRefused(t, f, ErrBudgetEvidenceCorrupt)
	})

	t.Run("the signed counter head of an account", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 3)
		committedStart(t, f)
		if _, err := f.store.db.Exec(`UPDATE budget_counters SET signature=?`, forged); err != nil {
			t.Fatal(err)
		}
		nextStartRefused(t, f, ErrBudgetEvidenceCorrupt)
	})

	t.Run("a start proof", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 3)
		activation := activateAuthorityFixture(t, f)
		committedStart(t, f)
		if _, err := f.store.db.Exec(`UPDATE authorization_starts SET signature=?`, forged); err != nil {
			t.Fatal(err)
		}
		if err := f.store.RequireAuthorityActivation(context.Background(), f.intent.ProfileID, activation); !errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
			t.Errorf("the strict boot's verification: error = %v, want %v", err, ErrAuthorizationSnapshotCorrupt)
		}
	})

	t.Run("an approval-birth event", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 3)
		activation := activateAuthorityFixture(t, f)
		parkStrictAuthorityFixture(t, f)
		if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM approval_birth_events WHERE sequence>0`); n != 1 {
			t.Fatalf("birth events after one strict park = %d, want 1: there is nothing to attack", n)
		}
		if _, err := f.store.db.Exec(`UPDATE approval_birth_events SET signature=? WHERE sequence>0`, forged); err != nil {
			t.Fatal(err)
		}
		if err := f.store.RequireAuthorityActivation(context.Background(), f.intent.ProfileID, activation); !errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
			t.Errorf("the strict boot's verification: error = %v, want %v", err, ErrAuthorizationSnapshotCorrupt)
		}
		nextStartRefused(t, f, ErrAuthorizationSnapshotCorrupt)
	})
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/identity"
)

// TestAuthority_RepeatedActionIDCannotSpendOrStartTwice (AS-AUTH-10): one
// action id is one start, one set of debits — whoever presents it again, and
// however close together.
//
// Evidence level: MULTIPLE REAL SQLITE CONNECTIONS for the race row — two
// independent pools minting the SAME id and starting at once; in-process for
// the sequential row.
// Probing mutation executed: answer an existing start with success instead of
// ErrActionAlreadyStarted — red on BOTH rows: «second = <nil>» in sequence, and
// «started=8 refused=0» in the race.
func TestAuthority_RepeatedActionIDCannotSpendOrStartTwice(t *testing.T) {
	const repeated = "act3_1_repeated"
	t.Run("the same id presented again, in sequence", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 5)
		f.store.authorityNewActionID = func(int64) string { return repeated }
		if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now)); err != nil {
			t.Fatal(err)
		}
		debits := authorityScalar(t, f.store, `SELECT COUNT(*) FROM budget_debits`)
		if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now)); !errors.Is(err, ErrActionAlreadyStarted) {
			t.Errorf("second = %v, want %v", err, ErrActionAlreadyStarted)
		}
		if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM authorization_starts WHERE action_id=?`, repeated); n != 1 {
			t.Errorf("starts = %d, want 1", n)
		}
		if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM budget_debits`); n != debits {
			t.Errorf("debits moved from %d to %d: the repeated id spent again", debits, n)
		}
	})

	t.Run("the same id raced from two real pools", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 5)
		second, err := Open(f.store.path)
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
		for _, store := range []*Store{f.store, second} {
			store.authorityNewActionID = func(int64) string { return repeated }
		}
		const callers = 8
		var wg sync.WaitGroup
		results := make(chan error, callers)
		gate := make(chan struct{})
		for i := 0; i < callers; i++ {
			store := f.store
			if i%2 == 1 {
				store = second
			}
			request := authorityStartRequest(f, "", f.now)
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-gate
				_, err := store.StartAuthorization(context.Background(), request)
				results <- err
			}()
		}
		close(gate)
		wg.Wait()
		close(results)
		started, refused := 0, 0
		for err := range results {
			switch {
			case err == nil:
				started++
			case errors.Is(err, ErrActionAlreadyStarted):
				refused++
			default:
				t.Errorf("a racing caller got %v, want a start or %v", err, ErrActionAlreadyStarted)
			}
		}
		if started != 1 || refused != callers-1 {
			t.Errorf("started=%d refused=%d, want exactly 1 and %d", started, refused, callers-1)
		}
		if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM authorization_starts WHERE action_id=?`, repeated); n != 1 {
			t.Errorf("starts = %d, want 1", n)
		}
		// One start under this fixture spends the intent and the root, total
		// and per-operation: four debit rows, and not one more.
		if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM budget_debits WHERE action_id=?`, repeated); n != 4 {
			t.Errorf("debits for the repeated id = %d, want the 4 of one start", n)
		}
		if record, err := f.store.Get(context.Background(), repeated); err != nil || record.State != action.StateAuthorized {
			t.Errorf("the winning action = %v, %v", record.State, err)
		}
	})
}

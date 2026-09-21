// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/identity"
)

// TestAuthority_TheActivatingOperatorIsJudgedAtTheActivation executes a
// prediction the adversary's pass over this phase left unexecuted (F9): the
// ledger verification re-judged the human who activated the profile as ENABLED
// on every later start. Disabling that human afterwards — an ordinary
// administrative act — then made every start and the strict boot answer
// «authorization snapshot corrupt» about a ledger nobody had touched: a
// profile bricked, under a false name. Confirmed by execution before the cure.
//
// What legitimises an activation is who its actor was WHEN it was signed. So a
// disable dated AFTER the activation changes nothing the ledger says, and a
// disable dated at or before it means the recorded actor could not have
// activated anything: that ledger is refused.
//
// No production door disables a principal today; the store door exists since
// phase 1 and this is the day it gets a caller.
//
// Evidence level: in-process, one real SQLite store; both disables go through
// the store's own signed DisablePrincipal door, not through SQL.
// Probing mutations executed, each alone: (1) the verifier demands the operator
// enabled NOW, as it did — red on the first row with «error = action/sqlite:
// authorization snapshot corrupt»; (2) the verifier ignores the operator's
// disable entirely — red on the second row with «error = <nil>».
func TestAuthority_TheActivatingOperatorIsJudgedAtTheActivation(t *testing.T) {
	const operator = "principal_responsible"

	t.Run("disabled after the activation: the ledger still stands", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 3)
		activation := activateAuthorityFixture(t, f) // signed at f.now + 1s
		if err := f.store.DisablePrincipal(context.Background(), operator, f.now.Add(30*time.Second)); err != nil {
			t.Fatalf("disable the operator: %v", err)
		}
		if err := f.store.RequireAuthorityActivation(context.Background(), f.intent.ProfileID, activation); err != nil {
			t.Errorf("the strict boot's verification after a LATER disable: error = %v, want none", err)
		}
		// In this fixture the same human answers for the brain, so the brain's
		// next start IS refused — by the name of what happened, a disabled
		// principal, and not as a corrupt ledger.
		_, err := f.store.StartAuthorization(context.Background(),
			authorityStartRequest(f, "", f.now.Add(40*time.Second)))
		if !errors.Is(err, identity.ErrPrincipalDisabled) {
			t.Errorf("the next start of a brain that human answers for: error = %v, want %v", err, identity.ErrPrincipalDisabled)
		}
	})

	t.Run("disabled at or before the activation: the ledger's actor could not have acted", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 3)
		activation := activateAuthorityFixture(t, f) // signed at f.now + 1s
		// A signed disable DATED before the activation it would have prevented.
		if err := f.store.DisablePrincipal(context.Background(), operator, f.now); err != nil {
			t.Fatalf("disable the operator: %v", err)
		}
		if err := f.store.RequireAuthorityActivation(context.Background(), f.intent.ProfileID, activation); !errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
			t.Errorf("the strict boot's verification: error = %v, want %v", err, ErrAuthorizationSnapshotCorrupt)
		}
	})
}

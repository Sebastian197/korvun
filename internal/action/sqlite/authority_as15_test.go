// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"testing"
)

func TestAuthority_CounterTamperingDoesNotRestoreBudget(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 2)
	if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.db.Exec(`UPDATE budget_counters SET spent=0`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now)); !errors.Is(err, ErrBudgetEvidenceCorrupt) {
		t.Fatalf("error = %v", err)
	}
}

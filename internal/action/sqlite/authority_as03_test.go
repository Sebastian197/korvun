// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"testing"
)

func TestAuthority_DelegateUsesRemainingBudget(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 10)
	for i := 0; i < 8; i++ {
		if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now)); err != nil {
			t.Fatal(err)
		}
	}
	child := f.root
	child.GrantID, child.ParentGrantID, child.ParentGrantVersion = "grant_too_large", f.root.GrantID, f.root.Version
	child.IssuerPrincipalID, child.SubjectPrincipalID = f.root.SubjectPrincipalID, "principal_worker"
	child.DelegationDepthRemaining = 3
	three := int64(3)
	child.Budget.Total = &three
	act := authorityActorAct(t, f.store, f.resolver, f.issuer, "delegate", child.CanonicalBytes(), f.now.Add(20))
	err := f.store.DelegateAuthority(context.Background(), child, act, f.now)
	if !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("error = %v, want budget remaining refusal", err)
	}
	if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM grant_versions WHERE grant_id=?`, child.GrantID); n != 0 {
		t.Fatalf("child rows = %d", n)
	}
}

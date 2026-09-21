// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"testing"
)

func TestAuthority_IssuerComesFromAuthenticatedActor(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 10)
	child := f.root
	child.GrantID, child.ParentGrantID, child.ParentGrantVersion = "grant_forged", f.root.GrantID, 1
	child.IssuerPrincipalID, child.SubjectPrincipalID, child.DelegationDepthRemaining = "principal_responsible", "principal_worker", 3
	act := authorityActorAct(t, f.store, f.resolver, f.issuer, "delegate", child.CanonicalBytes(), f.now.Add(3))
	if err := f.store.DelegateAuthority(context.Background(), child, act, f.now); !errors.Is(err, ErrIssuerMismatch) {
		t.Fatalf("error = %v", err)
	}
	if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM grant_versions WHERE grant_id=?`, child.GrantID); n != 0 {
		t.Fatalf("child rows = %d", n)
	}
}

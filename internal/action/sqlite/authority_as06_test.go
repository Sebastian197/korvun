// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
)

func TestAuthority_AncestorRevocationStopsLeaf(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 10)
	child := f.delegate(t, "grant_child", "principal_brain_alpha", 8, f.root)
	leaf := f.delegate(t, "grant_leaf", "principal_brain_alpha", 6, child)
	if _, err := f.store.db.Exec(`UPDATE execution_bindings SET grant_id=?,grant_version=?,grant_digest=? WHERE binding_id='binding_authority_root'`, leaf.GrantID, leaf.Version, leaf.Digest()); err != nil {
		t.Fatal(err)
	}
	act := authorityActorAct(t, f.store, f.resolver, f.issuer, "revoke", CanonicalAuthorityRevoke(f.root.GrantID, "stop ancestor"), f.now.Add(30))
	if err := f.store.RevokeAuthority(context.Background(), f.root.GrantID, act, "stop ancestor", f.now.Add(31)); err != nil {
		t.Fatal(err)
	}
	before := authorityScalar(t, f.store, `SELECT COUNT(*) FROM budget_debits`)
	_, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now.Add(32)))
	if !errors.Is(err, ErrAuthorityRevoked) {
		t.Fatalf("error = %v", err)
	}
	if after := authorityScalar(t, f.store, `SELECT COUNT(*) FROM budget_debits`); after != before {
		t.Fatalf("debits %d -> %d", before, after)
	}
	_ = action.StateAuthorized
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/identity"
)

func TestAuthority_EvidenceSurvivesActionPrune(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 2)
	activation := activateAuthorityFixture(t, f)
	req := authorityStartRequest(f, "prune-proof", f.now)
	req.CorrelationID = "request-prune-proof"
	ingress, err := f.issuer.Issue(req.CorrelationID, "forged-operator")
	if err != nil {
		t.Fatal(err)
	}
	req.ResolveEvidence = func(actionID string) (identity.Evidence, error) {
		return f.resolver.Resolve(ingress, identity.ResolveRequest{
			ActionID: actionID, RequestID: req.CorrelationID, Channel: "webhook", Brain: "alpha",
		})
	}
	started, err := f.store.StartAuthorization(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.Finish(context.Background(), started.ActionID, action.StateFailed, f.now.Add(1)); err != nil {
		t.Fatal(err)
	}
	f.store.capRows = 0
	removed, err := f.store.Prune(context.Background())
	if err != nil || removed != 1 {
		t.Fatalf("prune = %d, %v", removed, err)
	}
	if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM actions WHERE action_id=?`, started.ActionID); n != 0 {
		t.Fatalf("action rows = %d", n)
	}
	for table, want := range map[string]int{
		"authorization_starts":    1,
		"authorization_snapshots": 1,
		"budget_debits":           4,
	} {
		if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM `+table+` WHERE action_id=?`, started.ActionID); n != want {
			t.Fatalf("%s rows = %d, want %d", table, n, want)
		}
	}
	if err := authorityStrictBootRecover(f, activation); err != nil {
		t.Fatalf("verify surviving start proof: %v", err)
	}
}

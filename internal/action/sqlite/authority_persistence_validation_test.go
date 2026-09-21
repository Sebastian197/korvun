// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

func TestAuthority_RootValidationRejectsEveryWideningDimension(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 5)
	base := f.root
	base.AllowedResources = nil
	base.DeniedResources = nil
	base.AllowedData = nil
	base.OutputDestinations = nil
	cases := []struct {
		dimension string
		mutate    func(*action.AuthorityGrantV2)
	}{
		{"parent", func(g *action.AuthorityGrantV2) { g.ParentGrantID, g.ParentGrantVersion = "parent", 1 }},
		{"intent", func(g *action.AuthorityGrantV2) { g.IntentID = "other" }},
		{"operations", func(g *action.AuthorityGrantV2) {
			g.Operations = append(g.Operations, action.OperationRef{Namespace: "tool", Name: "other", Version: 1})
		}},
		{"operations", func(g *action.AuthorityGrantV2) { g.EffectClasses = []action.EffectClass{action.EffectCritical} }},
		{"allowed_resources", func(g *action.AuthorityGrantV2) {
			g.AllowedResources = []action.ResourceRef{{Kind: "path", ID: "/outside"}}
		}},
		{"denied_resources", func(g *action.AuthorityGrantV2) {
			g.DeniedResources = nil
		}},
		{"allowed_data", func(g *action.AuthorityGrantV2) { g.AllowedData = []string{"secret"} }},
		{"destinations", func(g *action.AuthorityGrantV2) { g.OutputDestinations = []string{"outside"} }},
		{"validity", func(g *action.AuthorityGrantV2) { g.ExpiresAt = f.intent.ExpiresAt.Add(time.Second) }},
		{"budget_total_remaining", func(g *action.AuthorityGrantV2) { g.Budget.Total = nil }},
		{"budget_operation_remaining", func(g *action.AuthorityGrantV2) { g.Budget.PerOperation = nil }},
		{"budget_operation_remaining", func(g *action.AuthorityGrantV2) { g.Budget.PerOperation["tool/probe@1"] = 6 }},
		{"approval", func(g *action.AuthorityGrantV2) { g.Approval.Required = false }},
	}
	intent := f.intent
	intent.AllowedResources = []action.ResourceRef{{Kind: "path", ID: "/cage"}}
	intent.DeniedResources = []action.ResourceRef{{Kind: "path", ID: "/cage/secret"}}
	intent.DataScope = []string{"public"}
	intent.OutputDestinations = []string{"example.com"}
	intent.Approval.Required = true
	base.IntentDigest = intent.Digest()
	base.AllowedResources = []action.ResourceRef{{Kind: "path", ID: "/cage/a"}}
	base.DeniedResources = append([]action.ResourceRef(nil), intent.DeniedResources...)
	base.AllowedData = []string{"public"}
	base.OutputDestinations = []string{"example.com"}
	base.Approval.Required = true
	for i, tc := range cases {
		t.Run(tc.dimension+string(rune('a'+i)), func(t *testing.T) {
			grant := base
			grant.Budget.PerOperation = map[string]int64{"tool/probe@1": 5}
			tc.mutate(&grant)
			_, err := normalizeRootAuthority(grant, intent)
			var attenuation *action.AttenuationError
			if !errors.As(err, &attenuation) || attenuation.Dimension != tc.dimension {
				t.Fatalf("error = %#v, want %q", err, tc.dimension)
			}
		})
	}
	valid := base
	valid.Budget.PerOperation = map[string]int64{"tool/probe@1": 5}
	if _, err := normalizeRootAuthority(valid, intent); err != nil {
		t.Fatal(err)
	}
}

func TestAuthority_PersistenceHelperBoundaries(t *testing.T) {
	if !resourceSupersetSQLite(
		[]action.ResourceRef{{Kind: "path", ID: "a"}, {Kind: "path", ID: "b"}},
		[]action.ResourceRef{{Kind: "path", ID: "b"}}) {
		t.Fatal("resource superset rejected")
	}
	if resourceSupersetSQLite([]action.ResourceRef{{Kind: "path", ID: "a"}},
		[]action.ResourceRef{{Kind: "path", ID: "b"}}) {
		t.Fatal("resource widening accepted")
	}
	if !stringSubsetSQLite([]string{"console"}, []string{"*"}) ||
		stringSubsetSQLite([]string{"telegram"}, []string{"console"}) {
		t.Fatal("string subset boundary failed")
	}
	if !sameStringSlice([]string{"a", "b"}, []string{"a", "b"}) ||
		sameStringSlice([]string{"a"}, []string{"a", "b"}) ||
		sameStringSlice([]string{"a", "b"}, []string{"b", "a"}) {
		t.Fatal("ordered slice comparison failed")
	}
	if !sameAuthorityBytes([]byte("same"), []byte("same")) || sameAuthorityBytes([]byte("a"), []byte("b")) {
		t.Fatal("authority byte comparison failed")
	}
	if containsAuthorityEffect([]action.EffectClass{action.EffectPure}, action.EffectCritical) {
		t.Fatal("absent effect was reported present")
	}
	if mapAuthorityStoreError(nil) != nil {
		t.Fatal("nil store error changed")
	}
	for _, err := range []error{context.DeadlineExceeded, context.Canceled, errors.New("database is locked"), errors.New("interrupted")} {
		if !errors.Is(mapAuthorityStoreError(err), ErrAuthorityStoreBusy) {
			t.Fatalf("store error %v was not busy", err)
		}
	}
	original := errors.New("disk failed")
	if mapAuthorityStoreError(original) != original {
		t.Fatal("unrelated store error changed")
	}
}

func TestAuthority_LegacyZeroBaselineAndNegativeGrantBalanceFailClosed(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 1)
	if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now)); err != nil {
		t.Fatal(err)
	}
	tx, err := f.store.beginAuthorityWrite(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	account := budgetAccountID(f.root.ProfileID, "grant", f.root.GrantID)
	if err := f.store.insertLegacyBaselineTx(context.Background(), tx, "act_zero", account, "*", 0, f.now); err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	grant := f.root
	grant.Budget.Total = &zero
	if _, err := f.store.remainingForGrantTx(context.Background(), tx, grant); !errors.Is(err, ErrBudgetEvidenceCorrupt) {
		t.Fatalf("negative remaining error = %v", err)
	}
}

func TestAuthority_ActualUseFailsClosedOnlyWhenTermsNeedResolution(t *testing.T) {
	openIntent := action.IntentContractV2{}
	if err := validateAuthorityActualUse(openIntent, nil, "unknown_tool", `{}`); err != nil {
		t.Fatalf("unrestricted unknown operation = %v", err)
	}
	restricted := openIntent
	restricted.DataScope = []string{"payload"}
	if err := validateAuthorityActualUse(restricted, nil, "unknown_tool", `{}`); !errors.Is(err, action.ErrAuthorityUseUnresolved) {
		t.Fatalf("restricted unknown operation = %v", err)
	}
	pathIntent := action.IntentContractV2{
		AllowedResources: []action.ResourceRef{{Kind: "path", ID: "/cage"}},
	}
	if err := validateAuthorityActualUse(pathIntent, nil, "read_file", "/cage/file"); err != nil {
		t.Fatal(err)
	}
	if err := validateAuthorityActualUse(pathIntent, nil, "read_file", "/outside/file"); !errors.Is(err, action.ErrResourceOutOfScope) {
		t.Fatalf("outside path error = %v", err)
	}
	grant := action.AuthorityGrantV2{AllowedResources: []action.ResourceRef{{Kind: "path", ID: "/cage/sub"}}}
	chain := []storedGrantV2{{signed: action.SignedAuthorityGrantV2{Grant: grant}}}
	if err := validateAuthorityActualUse(pathIntent, chain, "read_file", "/cage/file"); !errors.Is(err, action.ErrResourceOutOfScope) {
		t.Fatalf("grant narrowing error = %v", err)
	}
}

func TestAuthority_BindingAndChainFailureStates(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 3)
	ctx := context.Background()
	tx, err := f.store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := bindingAuthorityTx(ctx, tx, "missing", "webhook", ""); !errors.Is(err, ErrAuthorityMissing) {
		t.Fatalf("missing binding error = %v", err)
	}
	if _, err := f.store.authorityChainTx(ctx, tx, "", 1, f.now); !errors.Is(err, action.ErrAuthorityEvidenceCorrupt) {
		t.Fatalf("empty chain error = %v", err)
	}
	if _, err := f.store.authorityChainTx(ctx, tx, f.root.GrantID, f.root.Version, f.root.ExpiresAt); !errors.Is(err, ErrAuthorityExpired) {
		t.Fatalf("expired chain error = %v", err)
	}
	if _, err := f.store.authorityChainTx(ctx, tx, "grant_missing", 1, f.now); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("chain whose FIRST link is not there: error = %v, want %v (an absent first link is «missing» to its callers)", err, sql.ErrNoRows)
	}
}

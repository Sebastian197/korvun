// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Listing surfaces for the operator's act — Etapa 2, lote 5 (additive):
// ListIntents for `intent list`, ListByOperation for locating receipts.
// Approved-red contract.

package sqlite

import (
	"context"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
)

func TestListIntents_returnsAllOrderedById(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	for _, id := range []string{"int_b", "int_a", "int_c"} {
		intent := draftIntent(id)
		if err := store.CreateIntent(ctx, intent); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	got, err := store.ListIntents(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 intents, got %d", len(got))
	}
	if got[0].IntentID != "int_a" || got[1].IntentID != "int_b" || got[2].IntentID != "int_c" {
		t.Fatalf("deterministic id order, got %v %v %v", got[0].IntentID, got[1].IntentID, got[2].IntentID)
	}
	if got[0].Digest() != draftIntent("int_a").Digest() {
		t.Fatal("listed contracts round-trip whole")
	}
	empty, err := openTemp2(t)
	if err != nil {
		t.Fatalf("open empty: %v", err)
	}
	defer func() { _ = empty.Close() }()
	none, err := empty.ListIntents(ctx)
	if err != nil || len(none) != 0 {
		t.Fatalf("an empty store lists nothing: %v %v", none, err)
	}
}

// openTemp2 opens a second isolated store (openTemp's mold without the
// shared path return).
func openTemp2(t *testing.T) (*Store, error) {
	t.Helper()
	return Open(t.TempDir() + "/korvun.db")
}

func TestListByOperation_filtersExactly(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	envA := testEnvelope("act_lb1")
	envA.Operation = action.Operation{Namespace: "intent", Name: "create", Version: 1}
	envB := testEnvelope("act_lb2")
	envB.Operation = action.Operation{Namespace: "intent", Name: "activate", Version: 1}
	envC := testEnvelope("act_lb3")
	envC.Operation = action.Operation{Namespace: "tool", Name: "create", Version: 1}
	for _, env := range []action.Envelope{envA, envB, envC} {
		if err := store.RecordAttempt(ctx, env, Decision{Outcome: "allow", Rule: "granted"},
			action.StateAuthorized); err != nil {
			t.Fatalf("record %s: %v", env.ActionID, err)
		}
	}
	got, err := store.ListByOperation(ctx, "intent", "create")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].Envelope.ActionID != "act_lb1" {
		t.Fatalf("exact namespace+name filter, got %+v", got)
	}
	none, err := store.ListByOperation(ctx, "grant", "issue")
	if err != nil || len(none) != 0 {
		t.Fatalf("no matches lists nothing: %v %v", none, err)
	}
}

func TestList_deepErrorBranches(t *testing.T) {
	t.Parallel()
	// A closed store fails loud on both listing surfaces.
	closed, _ := openTemp(t)
	if err := closed.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := closed.ListIntents(context.Background()); err == nil {
		t.Fatal("ListIntents over a closed store must fail loud")
	}
	if _, err := closed.ListByOperation(context.Background(), "intent", "create"); err == nil {
		t.Fatal("ListByOperation over a closed store must fail loud")
	}
	// A corrupt stored row propagates its parse failure through the list.
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	if err := store.CreateIntent(ctx, draftIntent("int_corrupt")); err != nil {
		t.Fatalf("create: %v", err)
	}
	corruptCell(t, store, "intents", "operations", "intent_id", "int_corrupt", "{not json")
	if _, err := store.ListIntents(ctx); err == nil {
		t.Fatal("a corrupt intent row must fail the listing loud")
	}
	env := testEnvelope("act_corrupt")
	env.Operation = action.Operation{Namespace: "intent", Name: "create", Version: 1}
	if err := store.RecordAttempt(ctx, env, Decision{Outcome: "allow", Rule: "granted"},
		action.StateAuthorized); err != nil {
		t.Fatalf("record: %v", err)
	}
	corruptCell(t, store, "actions", "principal_id", "action_id", "act_corrupt", "principal_operator")
	corruptCell(t, store, "actions", "authority_refs", "action_id", "act_corrupt", "{not json")
	if _, err := store.ListByOperation(ctx, "intent", "create"); err == nil {
		t.Fatal("a corrupt action row must fail the listing loud")
	}
}

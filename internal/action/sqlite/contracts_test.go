// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Intent/grant persistence contract — Etapa 2, lote 3, pieza 3: create/
// activate/revoke intents, issue/delegate/revoke grants, every lifecycle
// edge validated from the STORED state (the Etapa-1 Finish mold) — and
// the golden frontier: DELEGATE calls ValidateAttenuation BEFORE
// persisting, so a widening child NEVER touches the disk. Approved-red
// contract.

package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

var contractBase = time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)

func draftIntent(id string) action.IntentContract {
	return action.IntentContract{
		IntentID:          id,
		SchemaVersion:     1,
		OwnerPrincipalID:  "principal_operator",
		Purpose:           "operate this Korvun instance",
		AllowedOperations: []string{"calc", "time", "read_file"},
		AllowedResources:  []string{"*"},
		Budgets:           action.Budgets{MaxActions: 100, MaxActionsPerOperation: map[string]int{"calc": 10}},
		ValidFrom:         contractBase,
		ExpiresAt:         contractBase.Add(48 * time.Hour),
		Status:            action.LifecycleDraft,
		Version:           1,
	}
}

func rootGrant(id, intentID string) action.AuthorityGrant {
	return action.AuthorityGrant{
		GrantID:                  id,
		IntentID:                 intentID,
		IssuerPrincipalID:        "principal_operator",
		SubjectPrincipalID:       "principal_brain_a",
		Operations:               []string{"calc", "time"},
		ResourceScope:            []string{"*"},
		Budgets:                  action.Budgets{MaxActions: 50},
		ValidFrom:                contractBase,
		ExpiresAt:                contractBase.Add(24 * time.Hour),
		DelegationDepthRemaining: 2,
		Status:                   action.LifecycleActive,
	}
}

func childGrant(id string, parent action.AuthorityGrant) action.AuthorityGrant {
	return action.AuthorityGrant{
		GrantID:                  id,
		IntentID:                 parent.IntentID,
		IssuerPrincipalID:        parent.SubjectPrincipalID,
		SubjectPrincipalID:       "principal_ch_hooks",
		ParentGrantID:            parent.GrantID,
		Operations:               []string{"calc"},
		ResourceScope:            []string{"*"},
		Budgets:                  action.Budgets{MaxActions: 5},
		ValidFrom:                parent.ValidFrom.Add(time.Hour),
		ExpiresAt:                parent.ExpiresAt.Add(-time.Hour),
		DelegationDepthRemaining: 1,
		Status:                   action.LifecycleActive,
	}
}

func TestIntent_roundTripAndLifecycleFromStoredState(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	intent := draftIntent("int_1")
	if err := store.CreateIntent(ctx, intent); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := store.GetIntent(ctx, "int_1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Digest() != intent.Digest() {
		t.Fatalf("the contract must round-trip with its digest intact:\ngot  %+v\nwant %+v", got, intent)
	}
	if got.Status != action.LifecycleDraft {
		t.Fatalf("status = %s", got.Status)
	}
	// Lifecycle edges are validated from the STORED state.
	if err := store.TransitionIntent(ctx, "int_1", action.LifecycleActive); err != nil {
		t.Fatalf("draft -> active: %v", err)
	}
	if err := store.TransitionIntent(ctx, "int_1", action.LifecycleDraft); !errors.Is(err, action.ErrInvalidLifecycleTransition) {
		t.Fatalf("active -> draft must be rejected from the stored state, got %v", err)
	}
	if err := store.TransitionIntent(ctx, "int_1", action.LifecycleRevoked); err != nil {
		t.Fatalf("active -> revoked: %v", err)
	}
	if err := store.TransitionIntent(ctx, "int_1", action.LifecycleActive); !errors.Is(err, action.ErrInvalidLifecycleTransition) {
		t.Fatalf("revoked is terminal in the store too, got %v", err)
	}
	final, err := store.GetIntent(ctx, "int_1")
	if err != nil {
		t.Fatalf("get final: %v", err)
	}
	if final.Status != action.LifecycleRevoked {
		t.Fatalf("stored status must track transitions, got %s", final.Status)
	}
}

func TestIntent_createRejectsTerminalStatus(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	dead := draftIntent("int_dead")
	dead.Status = action.LifecycleRevoked
	if err := store.CreateIntent(context.Background(), dead); err == nil {
		t.Fatal("a contract cannot be born terminal")
	}
}

func TestIntent_unknownFailsClosed(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	if _, err := store.GetIntent(context.Background(), "int_ghost"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown intent: %v", err)
	}
	if err := store.TransitionIntent(context.Background(), "int_ghost", action.LifecycleActive); !errors.Is(err, ErrNotFound) {
		t.Fatalf("transitioning a ghost: %v", err)
	}
}

func TestGrant_issueAndRoundTrip(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	if err := store.CreateIntent(ctx, draftIntent("int_g")); err != nil {
		t.Fatalf("create intent: %v", err)
	}
	grant := rootGrant("grant_1", "int_g")
	if err := store.CreateGrant(ctx, grant); err != nil {
		t.Fatalf("issue grant: %v", err)
	}
	got, err := store.GetGrant(ctx, "grant_1")
	if err != nil {
		t.Fatalf("get grant: %v", err)
	}
	if got.Digest() != grant.Digest() {
		t.Fatalf("the grant must round-trip with its digest intact:\ngot  %+v\nwant %+v", got, grant)
	}
}

func TestGrant_issueRequiresExistingIntent(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	if err := store.CreateGrant(context.Background(), rootGrant("grant_x", "int_missing")); err == nil {
		t.Fatal("a grant cannot exist outside a stored intent")
	}
}

// TestDelegate_wideningNeverTouchesTheDisk is the golden frontier: the
// pure §14.3 validator turned into the database's wall. A child that
// widens its parent in ANY dimension is rejected BEFORE persisting.
func TestDelegate_wideningNeverTouchesTheDisk(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	if err := store.CreateIntent(ctx, draftIntent("int_d")); err != nil {
		t.Fatalf("create intent: %v", err)
	}
	parent := rootGrant("grant_p", "int_d")
	if err := store.CreateGrant(ctx, parent); err != nil {
		t.Fatalf("issue parent: %v", err)
	}
	widening := childGrant("grant_wide", parent)
	widening.Budgets.MaxActions = 500 // wider than the parent's 50
	err := store.DelegateGrant(ctx, widening)
	if !errors.Is(err, action.ErrAttenuationViolated) {
		t.Fatalf("the wall must reject a widening with the attenuation sentinel, got %v", err)
	}
	if _, err := store.GetGrant(ctx, "grant_wide"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a widening child must NEVER touch the disk, got %v", err)
	}
}

func TestDelegate_validChildPersists(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	if err := store.CreateIntent(ctx, draftIntent("int_v")); err != nil {
		t.Fatalf("create intent: %v", err)
	}
	parent := rootGrant("grant_pv", "int_v")
	if err := store.CreateGrant(ctx, parent); err != nil {
		t.Fatalf("issue parent: %v", err)
	}
	child := childGrant("grant_cv", parent)
	if err := store.DelegateGrant(ctx, child); err != nil {
		t.Fatalf("a strict subset delegation must persist: %v", err)
	}
	got, err := store.GetGrant(ctx, "grant_cv")
	if err != nil {
		t.Fatalf("get child: %v", err)
	}
	if got.ParentGrantID != "grant_pv" || got.Digest() != child.Digest() {
		t.Fatalf("delegated child corrupted: %+v", got)
	}
}

func TestDelegate_parentMustBeStoredAndDelegable(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	if err := store.CreateIntent(ctx, draftIntent("int_p")); err != nil {
		t.Fatalf("create intent: %v", err)
	}
	parent := rootGrant("grant_pp", "int_p")
	if err := store.CreateGrant(ctx, parent); err != nil {
		t.Fatalf("issue parent: %v", err)
	}
	// A child naming NO parent is not a delegation.
	orphan := childGrant("grant_orphan", parent)
	orphan.ParentGrantID = ""
	if err := store.DelegateGrant(ctx, orphan); err == nil {
		t.Fatal("delegation demands a named stored parent")
	}
	// A ghost parent fails closed.
	ghost := childGrant("grant_ghost", parent)
	ghost.ParentGrantID = "grant_never"
	if err := store.DelegateGrant(ctx, ghost); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a missing parent fails closed, got %v", err)
	}
	// A REVOKED stored parent cannot delegate — the STORED status decides,
	// not whatever the caller's copy of the parent claims.
	if err := store.TransitionGrant(ctx, "grant_pp", action.LifecycleRevoked); err != nil {
		t.Fatalf("revoke parent: %v", err)
	}
	late := childGrant("grant_late", parent)
	if err := store.DelegateGrant(ctx, late); !errors.Is(err, ErrParentNotDelegable) {
		t.Fatalf("a revoked stored parent must refuse to delegate, got %v", err)
	}
	if _, err := store.GetGrant(ctx, "grant_late"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("nothing lands under a revoked parent, got %v", err)
	}
}

func TestGrant_lifecycleFromStoredState(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	if err := store.CreateIntent(ctx, draftIntent("int_l")); err != nil {
		t.Fatalf("create intent: %v", err)
	}
	if err := store.CreateGrant(ctx, rootGrant("grant_l", "int_l")); err != nil {
		t.Fatalf("issue: %v", err)
	}
	if err := store.TransitionGrant(ctx, "grant_l", action.LifecycleRevoked); err != nil {
		t.Fatalf("active -> revoked: %v", err)
	}
	if err := store.TransitionGrant(ctx, "grant_l", action.LifecycleActive); !errors.Is(err, action.ErrInvalidLifecycleTransition) {
		t.Fatalf("revoked is terminal from the stored state, got %v", err)
	}
	if err := store.TransitionGrant(ctx, "grant_ghost2", action.LifecycleRevoked); !errors.Is(err, ErrNotFound) {
		t.Fatalf("transitioning a ghost grant: %v", err)
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Deep error branches of the v2 surfaces (Etapa 2, lote 3): closed-store
// sweep, corrupt stored rows, blocked writes — the Etapa-1 discipline of
// making every error path reachable instead of decorative.

package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// TestClosedStore_everyV2SurfaceFailsLoud sweeps the new methods over a
// CLOSED store: infrastructure failure must surface, never a silent zero.
func TestClosedStore_everyV2SurfaceFailsLoud(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	ctx := context.Background()
	calls := map[string]func() error{
		"CreateIntent": func() error { return store.CreateIntent(ctx, draftIntent("int_x")) },
		"GetIntent":    func() error { _, err := store.GetIntent(ctx, "int_x"); return err },
		"TransitionIntent": func() error {
			return store.TransitionIntent(ctx, "int_x", action.LifecycleActive)
		},
		"CreateGrant": func() error { return store.CreateGrant(ctx, rootGrant("g", "int_x")) },
		"GetGrant":    func() error { _, err := store.GetGrant(ctx, "g"); return err },
		"TransitionGrant": func() error {
			return store.TransitionGrant(ctx, "g", action.LifecycleRevoked)
		},
		"DelegateGrant": func() error {
			child := childGrant("c", rootGrant("g", "int_x"))
			return store.DelegateGrant(ctx, child)
		},
		"ConsumeBudget": func() error {
			_, err := store.ConsumeBudget(ctx, "s", action.Budgets{}, "op")
			return err
		},
		"SpentBudget": func() error { _, err := store.SpentBudget(ctx, "s"); return err },
		"RecordAttemptIdentified": func() error {
			return store.RecordAttemptIdentified(ctx, testEnvelope("a"),
				Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized, testIdentity())
		},
		"GetEvidence": func() error { _, err := store.GetEvidence(ctx, "a"); return err },
	}
	for name, call := range calls {
		if err := call(); err == nil {
			t.Fatalf("%s over a closed store must fail loud", name)
		}
	}
}

// corruptCell overwrites one cell of a stored row with raw garbage.
func corruptCell(t *testing.T, store *Store, table, column, idColumn, id, garbage string) {
	t.Helper()
	stmt := `UPDATE ` + table + ` SET ` + column + ` = ? WHERE ` + idColumn + ` = ?` // #nosec G202 -- test-owned literals
	if _, err := store.db.Exec(stmt, garbage, id); err != nil {
		t.Fatalf("corrupt %s.%s: %v", table, column, err)
	}
}

func TestGetIntent_corruptCellsFailLoud(t *testing.T) {
	t.Parallel()
	cases := []struct{ column, garbage, want string }{
		{"operations", "{not json", "parse set"},
		{"per_operation", "{not json", "per-operation"},
		{"valid_from", "not-a-time", "valid_from"},
		{"expires_at", "not-a-time", "expires_at"},
	}
	for _, tc := range cases {
		store, _ := openTemp(t)
		if err := store.CreateIntent(context.Background(), draftIntent("int_c")); err != nil {
			t.Fatalf("create: %v", err)
		}
		corruptCell(t, store, "intents", tc.column, "intent_id", "int_c", tc.garbage)
		_, err := store.GetIntent(context.Background(), "int_c")
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("corrupt %s must fail loud naming the cell, got %v", tc.column, err)
		}
		_ = store.Close()
	}
}

func TestGetGrant_corruptCellsFailLoud(t *testing.T) {
	t.Parallel()
	cases := []struct{ column, garbage, want string }{
		{"operations", "{not json", "parse set"},
		{"resources", "{not json", "parse set"},
		{"per_operation", "{not json", "per-operation"},
		{"valid_from", "not-a-time", "valid_from"},
		{"expires_at", "not-a-time", "expires_at"},
	}
	for _, tc := range cases {
		store, _ := openTemp(t)
		ctx := context.Background()
		if err := store.CreateIntent(ctx, draftIntent("int_g")); err != nil {
			t.Fatalf("create intent: %v", err)
		}
		if err := store.CreateGrant(ctx, rootGrant("g_c", "int_g")); err != nil {
			t.Fatalf("create grant: %v", err)
		}
		corruptCell(t, store, "grants", tc.column, "grant_id", "g_c", tc.garbage)
		_, err := store.GetGrant(ctx, "g_c")
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("corrupt %s must fail loud naming the cell, got %v", tc.column, err)
		}
		_ = store.Close()
	}
}

func TestGetEvidence_corruptIssuedAtFailsLoud(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	if err := store.RecordAttemptIdentified(ctx, testEnvelope("act_ce"),
		Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized, testIdentity()); err != nil {
		t.Fatalf("record: %v", err)
	}
	corruptCell(t, store, "evidence", "issued_at", "action_id", "act_ce", "garbage")
	if _, err := store.GetEvidence(ctx, "act_ce"); err == nil || !strings.Contains(err.Error(), "issued_at") {
		t.Fatalf("corrupt issued_at must fail loud, got %v", err)
	}
}

func TestGet_corruptAuthorityRefsFailsLoud(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	if err := store.RecordAttemptIdentified(ctx, testEnvelope("act_cr"),
		Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized, testIdentity()); err != nil {
		t.Fatalf("record: %v", err)
	}
	corruptCell(t, store, "actions", "authority_refs", "action_id", "act_cr", "{not json")
	if _, err := store.Get(ctx, "act_cr"); err == nil || !strings.Contains(err.Error(), "authority_refs") {
		t.Fatalf("corrupt authority_refs must fail loud, got %v", err)
	}
}

func TestTransition_blockedUpdateSurfacesTheError(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	if err := store.CreateIntent(ctx, draftIntent("int_b")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := store.db.Exec(
		`CREATE TRIGGER block_intent_update BEFORE UPDATE ON intents
		 BEGIN SELECT RAISE(ABORT, 'blocked by test'); END;`,
	); err != nil {
		t.Fatalf("install blocker: %v", err)
	}
	if err := store.TransitionIntent(ctx, "int_b", action.LifecycleActive); err == nil {
		t.Fatal("a blocked lifecycle update must surface the error")
	}
	// The stored status must be untouched by the failed transition.
	got, err := store.GetIntent(ctx, "int_b")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != action.LifecycleDraft {
		t.Fatalf("a failed transition must change nothing, got %s", got.Status)
	}
}

func TestRecordIdentified_duplicateActionFailsAtomically(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	ident := testIdentity()
	if err := store.RecordAttemptIdentified(ctx, testEnvelope("act_dup"),
		Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized, ident); err != nil {
		t.Fatalf("first record: %v", err)
	}
	ident2 := ident
	ident2.Evidence.EvidenceID = "evd_1111111111111111"
	if err := store.RecordAttemptIdentified(ctx, testEnvelope("act_dup"),
		Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized, ident2); err == nil {
		t.Fatal("a duplicate action_id must fail")
	}
	// The original evidence row is intact — no partial overwrite.
	ev, err := store.GetEvidence(ctx, "act_dup")
	if err != nil {
		t.Fatalf("get evidence: %v", err)
	}
	if ev.EvidenceID != ident.Evidence.EvidenceID {
		t.Fatalf("the losing duplicate must leave no trace, got %q", ev.EvidenceID)
	}
}

func TestCreateIntent_duplicateFailsLoud(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	if err := store.CreateIntent(ctx, draftIntent("int_dd")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.CreateIntent(ctx, draftIntent("int_dd")); err == nil {
		t.Fatal("a duplicate intent id must fail")
	}
	grant := rootGrant("g_dd", "int_dd")
	if err := store.CreateGrant(ctx, grant); err != nil {
		t.Fatalf("create grant: %v", err)
	}
	if err := store.CreateGrant(ctx, grant); err == nil {
		t.Fatal("a duplicate grant id must fail")
	}
}

// TestTimeHelpers_zeroAndValueRoundTrip pins the NULL-time convention.
func TestTimeHelpers_zeroAndValueRoundTrip(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	// A no-expiry intent stores NULL and reads back the zero time.
	forever := draftIntent("int_forever")
	forever.ExpiresAt = time.Time{}
	if err := store.CreateIntent(ctx, forever); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := store.GetIntent(ctx, "int_forever")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.ExpiresAt.IsZero() {
		t.Fatalf("no-expiry must round-trip as the zero time, got %v", got.ExpiresAt)
	}
	if got.Digest() != forever.Digest() {
		t.Fatal("the no-expiry contract digest must survive the round-trip")
	}
}

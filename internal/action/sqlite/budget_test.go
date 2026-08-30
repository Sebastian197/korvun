// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Durable budget contract — Etapa 2, lote 3, pieza 4 (spec FR-BUD): the
// batch-2 ledger discipline against the REAL store. Consumption is
// check-and-increment inside one transaction, so the limit is never
// exceeded under the concurrent hammer, denials consume nothing, a crash
// mid-consume counts nothing, and spent consumption SURVIVES restarts.
// Approved-red contract.

package sqlite

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
)

func TestConsumeBudget_concurrentHammerNeverExceedsTheLimit(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	const limit = 50
	limits := action.Budgets{MaxActions: limit}
	var granted, denied, infra atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 4*limit; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rule, err := store.ConsumeBudget(context.Background(), "int_h", limits, "calc")
			switch {
			case err != nil:
				infra.Add(1)
			case rule == "":
				granted.Add(1)
			case rule == action.RuleBudgetExhausted:
				denied.Add(1)
			}
		}()
	}
	wg.Wait()
	if infra.Load() != 0 {
		t.Fatalf("%d infrastructure errors under the hammer", infra.Load())
	}
	if granted.Load() != limit {
		t.Fatalf("EXACTLY the limit must be granted: got %d of %d", granted.Load(), limit)
	}
	if denied.Load() != 3*limit {
		t.Fatalf("denials = %d, want %d", denied.Load(), 3*limit)
	}
	spent, err := store.SpentBudget(context.Background(), "int_h")
	if err != nil {
		t.Fatalf("spent: %v", err)
	}
	if spent != limit {
		t.Fatalf("denials must not consume: spent = %d, want %d", spent, limit)
	}
}

func TestConsumeBudget_survivesRestart(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "korvun.db")
	limits := action.Budgets{MaxActions: 5}
	store, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	for i := 0; i < 3; i++ {
		if rule, err := store.ConsumeBudget(context.Background(), "int_r", limits, "calc"); err != nil || rule != "" {
			t.Fatalf("consume %d: rule=%q err=%v", i, rule, err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// The next life REMEMBERS: 3 of 5 already spent.
	again, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = again.Close() }()
	spent, err := again.SpentBudget(context.Background(), "int_r")
	if err != nil {
		t.Fatalf("spent after restart: %v", err)
	}
	if spent != 3 {
		t.Fatalf("consumption must survive restarts: spent = %d, want 3", spent)
	}
	for i := 0; i < 2; i++ {
		if rule, err := again.ConsumeBudget(context.Background(), "int_r", limits, "calc"); err != nil || rule != "" {
			t.Fatalf("post-restart consume %d: rule=%q err=%v", i, rule, err)
		}
	}
	rule, err := again.ConsumeBudget(context.Background(), "int_r", limits, "calc")
	if err != nil {
		t.Fatalf("final consume: %v", err)
	}
	if rule != action.RuleBudgetExhausted {
		t.Fatalf("the durable limit must bite across lives, got %q", rule)
	}
}

func TestConsumeBudget_perOperationAndUnlimited(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	limits := action.Budgets{
		MaxActions:             10,
		MaxActionsPerOperation: map[string]int{"calc": 1},
	}
	if rule, err := store.ConsumeBudget(ctx, "int_po", limits, "calc"); err != nil || rule != "" {
		t.Fatalf("first calc: rule=%q err=%v", rule, err)
	}
	rule, err := store.ConsumeBudget(ctx, "int_po", limits, "calc")
	if err != nil {
		t.Fatalf("second calc: %v", err)
	}
	if rule != action.RuleBudgetExhausted {
		t.Fatalf("per-operation cap must bite, got %q", rule)
	}
	spent, err := store.SpentBudget(ctx, "int_po")
	if err != nil {
		t.Fatalf("spent: %v", err)
	}
	if spent != 1 {
		t.Fatalf("a per-op denial must not consume the total: spent = %d", spent)
	}
	// The remaining total room stays usable by other operations.
	for i := 0; i < 9; i++ {
		if rule, err := store.ConsumeBudget(ctx, "int_po", limits, "time"); err != nil || rule != "" {
			t.Fatalf("total room must remain (%d): rule=%q err=%v", i, rule, err)
		}
	}
	// Zero limits stay UNLIMITED, per the domain semantics.
	for i := 0; i < 100; i++ {
		if rule, err := store.ConsumeBudget(ctx, "int_free", action.Budgets{}, "x"); err != nil || rule != "" {
			t.Fatalf("unlimited scope (%d): rule=%q err=%v", i, rule, err)
		}
	}
}

// TestConsumeBudget_crashMidConsumeConsumesNothing: an aborted consume
// transaction counts NOTHING — the durable counter cannot drift.
func TestConsumeBudget_crashMidConsumeConsumesNothing(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	limits := action.Budgets{MaxActions: 10}
	if rule, err := store.ConsumeBudget(ctx, "int_c", limits, "calc"); err != nil || rule != "" {
		t.Fatalf("seed consume: rule=%q err=%v", rule, err)
	}
	if _, err := store.db.Exec(
		`CREATE TRIGGER crash_consume BEFORE UPDATE ON budget_spent
		 BEGIN SELECT RAISE(ABORT, 'crash mid-consume'); END;`,
	); err != nil {
		t.Fatalf("install crash trigger: %v", err)
	}
	if _, err := store.ConsumeBudget(ctx, "int_c", limits, "calc"); err == nil {
		t.Fatal("a blocked consume must surface the infrastructure error")
	}
	if _, err := store.db.Exec(`DROP TRIGGER crash_consume`); err != nil {
		t.Fatalf("drop crash trigger: %v", err)
	}
	spent, err := store.SpentBudget(ctx, "int_c")
	if err != nil {
		t.Fatalf("spent: %v", err)
	}
	if spent != 1 {
		t.Fatalf("an aborted consume must count NOTHING: spent = %d, want 1", spent)
	}
	if rule, err := store.ConsumeBudget(ctx, "int_c", limits, "calc"); err != nil || rule != "" {
		t.Fatalf("the next consume proceeds cleanly: rule=%q err=%v", rule, err)
	}
}

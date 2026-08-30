// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Budget ledger contract — Etapa 2, lote 2, pieza 3 (spec FR-BUD-1): v1
// budgets with SERIALIZED conditional consumption. The mandatory test is
// the concurrent hammer under -race: with limit N and 4N concurrent
// consumers, EXACTLY N succeed — the limit is never exceeded, denials
// never consume. Approved-red contract.

package action

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestBudgetLedger_concurrentHammerNeverExceedsTheLimit(t *testing.T) {
	t.Parallel()
	const limit = 100
	ledger := NewBudgetLedger(Budgets{MaxActions: limit})
	var granted, denied atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 4*limit; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rule := ledger.Consume("calc"); rule == "" {
				granted.Add(1)
			} else if rule == RuleBudgetExhausted {
				denied.Add(1)
			}
		}()
	}
	wg.Wait()
	if granted.Load() != limit {
		t.Fatalf("EXACTLY the limit must be granted: got %d of limit %d", granted.Load(), limit)
	}
	if denied.Load() != 3*limit {
		t.Fatalf("every consumer past the limit is denied with the rule: got %d denials", denied.Load())
	}
	if spent := ledger.Spent(); spent != limit {
		t.Fatalf("denials must not consume: spent = %d, want %d", spent, limit)
	}
	// Exhausted stays exhausted.
	if rule := ledger.Consume("calc"); rule != RuleBudgetExhausted {
		t.Fatalf("an exhausted ledger keeps denying, got %q", rule)
	}
}

func TestBudgetLedger_zeroMeansUnlimited(t *testing.T) {
	t.Parallel()
	ledger := NewBudgetLedger(Budgets{})
	for i := 0; i < 500; i++ {
		if rule := ledger.Consume("anything"); rule != "" {
			t.Fatalf("an unlimited ledger never denies, got %q at %d", rule, i)
		}
	}
	if spent := ledger.Spent(); spent != 500 {
		t.Fatalf("consumption is still recorded: spent = %d", spent)
	}
}

func TestBudgetLedger_perOperationHammerIsIndependentlyBounded(t *testing.T) {
	t.Parallel()
	const opLimit = 25
	ledger := NewBudgetLedger(Budgets{
		MaxActionsPerOperation: map[string]int{"calc": opLimit},
	})
	var calcGranted, timeGranted atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 4*opLimit; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ledger.Consume("calc") == "" {
				calcGranted.Add(1)
			}
			if ledger.Consume("time") == "" {
				timeGranted.Add(1)
			}
		}()
	}
	wg.Wait()
	if calcGranted.Load() != opLimit {
		t.Fatalf("capped operation: got %d of limit %d", calcGranted.Load(), opLimit)
	}
	if timeGranted.Load() != 4*opLimit {
		t.Fatalf("an uncapped operation under a per-op-only budget is unlimited, got %d", timeGranted.Load())
	}
}

func TestBudgetLedger_totalCapBitesEvenUnderPerOperationRoom(t *testing.T) {
	t.Parallel()
	ledger := NewBudgetLedger(Budgets{
		MaxActions:             3,
		MaxActionsPerOperation: map[string]int{"calc": 10},
	})
	for i := 0; i < 3; i++ {
		if rule := ledger.Consume("calc"); rule != "" {
			t.Fatalf("within both caps, got %q at %d", rule, i)
		}
	}
	if rule := ledger.Consume("calc"); rule != RuleBudgetExhausted {
		t.Fatalf("the total cap bites first, got %q", rule)
	}
}

func TestBudgetLedger_deniedPerOperationDoesNotConsumeTheTotal(t *testing.T) {
	t.Parallel()
	ledger := NewBudgetLedger(Budgets{
		MaxActions:             10,
		MaxActionsPerOperation: map[string]int{"calc": 1},
	})
	if rule := ledger.Consume("calc"); rule != "" {
		t.Fatalf("first calc passes, got %q", rule)
	}
	if rule := ledger.Consume("calc"); rule != RuleBudgetExhausted {
		t.Fatalf("second calc is per-op denied, got %q", rule)
	}
	if spent := ledger.Spent(); spent != 1 {
		t.Fatalf("a per-op denial must not consume the total: spent = %d", spent)
	}
	// The remaining total room stays usable by other operations.
	for i := 0; i < 9; i++ {
		if rule := ledger.Consume("time"); rule != "" {
			t.Fatalf("total room must remain: got %q at %d", rule, i)
		}
	}
}

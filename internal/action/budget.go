// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Budget ledger (Trust Layer Etapa 2, lote 2, spec FR-BUD-1): v1 budget
// enforcement with SERIALIZED conditional consumption — one mutex owns
// check-and-increment, so the limit is never exceeded no matter how many
// consumers hammer it concurrently, and a denial never consumes. The
// zero Budgets value is UNLIMITED (FR-BUD-2: the root intent's standing
// authority carries no budget); limits bite only when set.
package action

import "sync"

// BudgetLedger tracks consumption against one contract's Budgets. Safe
// for concurrent use; persistence of consumption arrives in lote 3 —
// this is the in-memory serialization point the store will feed.
type BudgetLedger struct {
	mu     sync.Mutex
	limits Budgets
	total  int
	perOp  map[string]int
}

// NewBudgetLedger builds a ledger over one contract's budgets.
func NewBudgetLedger(b Budgets) *BudgetLedger {
	return &BudgetLedger{limits: b, perOp: make(map[string]int)}
}

// Consume atomically checks-and-increments one action against the
// budgets: "" when granted, RuleBudgetExhausted when either the total
// cap or the operation's cap is reached. Denials consume NOTHING —
// remaining room stays usable by other operations.
func (l *BudgetLedger) Consume(operation string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.limits.MaxActions != 0 && l.total >= l.limits.MaxActions {
		return RuleBudgetExhausted
	}
	if opCap, capped := l.limits.MaxActionsPerOperation[operation]; capped && opCap != 0 &&
		l.perOp[operation] >= opCap {
		return RuleBudgetExhausted
	}
	l.total++
	l.perOp[operation]++
	return ""
}

// Spent reports total granted consumption so far.
func (l *BudgetLedger) Spent() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.total
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Durable budgets (Trust Layer Etapa 2, lote 3, pieza 4, spec FR-BUD):
// the batch-2 ledger discipline moved into the store. One transaction
// owns check-and-increment over the budget_spent table — the single
// serialized writer makes concurrent consumers queue on it, so the limit
// is never exceeded, a denial writes nothing, and an aborted transaction
// counts nothing. Consumption is durable: restarts remember every grant.
// Growth is bounded by contracts × operation names — operator scale.
package sqlite

import (
	"context"
	"fmt"

	"github.com/Sebastian197/korvun/internal/action"
)

// budgetTotalKey is the operation key of a scope's TOTAL counter.
const budgetTotalKey = ""

// ConsumeBudget atomically checks-and-increments one action against a
// contract's limits: ("", nil) when granted, (RuleBudgetExhausted, nil)
// when the total or per-operation cap is reached, and a non-nil error
// only for infrastructure failure — in which case nothing was counted.
// Zero limits are UNLIMITED (the domain's semantics), but consumption is
// still recorded so later, tighter contracts see honest history.
func (s *Store) ConsumeBudget(ctx context.Context, scopeID string, limits action.Budgets, operation string) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("action/sqlite: begin consume: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var total, perOp int
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(CASE WHEN operation = ? THEN spent END), 0),
		        COALESCE(SUM(CASE WHEN operation = ? THEN spent END), 0)
		   FROM budget_spent WHERE scope_id = ?`,
		budgetTotalKey, operation, scopeID,
	).Scan(&total, &perOp); err != nil {
		return "", fmt.Errorf("action/sqlite: read budget of %q: %w", scopeID, err)
	}
	if limits.MaxActions != 0 && total >= limits.MaxActions {
		return action.RuleBudgetExhausted, nil
	}
	if opCap, capped := limits.MaxActionsPerOperation[operation]; capped && opCap != 0 && perOp >= opCap {
		return action.RuleBudgetExhausted, nil
	}
	// An empty operation name would collide with the total's key and
	// double-count; it consumes the total only.
	keys := []string{budgetTotalKey}
	if operation != budgetTotalKey {
		keys = append(keys, operation)
	}
	for _, key := range keys {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO budget_spent (scope_id, operation, spent) VALUES (?, ?, 1)
			 ON CONFLICT (scope_id, operation) DO UPDATE SET spent = spent + 1`,
			scopeID, key,
		); err != nil {
			return "", fmt.Errorf("action/sqlite: consume budget of %q: %w", scopeID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("action/sqlite: commit consume of %q: %w", scopeID, err)
	}
	return "", nil
}

// SpentBudget reports a scope's total granted consumption (0 for a scope
// that never consumed).
func (s *Store) SpentBudget(ctx context.Context, scopeID string) (int, error) {
	var spent int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(spent), 0) FROM budget_spent
		  WHERE scope_id = ? AND operation = ?`,
		scopeID, budgetTotalKey,
	).Scan(&spent); err != nil {
		return 0, fmt.Errorf("action/sqlite: spent budget of %q: %w", scopeID, err)
	}
	return spent, nil
}

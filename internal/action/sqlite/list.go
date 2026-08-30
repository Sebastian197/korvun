// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Listing surfaces (Trust Layer Etapa 2, lote 5): ListIntents backs
// `korvun intent list`; ListByOperation locates the operator's receipts.
// Both are reads over operator-scale tables (tens of contracts) or the
// capped actions table — bounded by construction.
package sqlite

import (
	"context"
	"fmt"

	"github.com/Sebastian197/korvun/internal/action"
)

// ListIntents returns every stored intent contract, ordered by id for
// deterministic output.
func (s *Store) ListIntents(ctx context.Context) ([]action.IntentContract, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT intent_id FROM intents ORDER BY intent_id ASC`)
	if err != nil {
		return nil, fmt.Errorf("action/sqlite: list intents: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("action/sqlite: scan intent id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("action/sqlite: iterate intents: %w", err)
	}
	// Row-at-a-time via GetIntent keeps ONE scan/parse path (operator
	// scale: tens of rows, never a hot path).
	out := make([]action.IntentContract, 0, len(ids))
	for _, id := range ids {
		c, err := s.GetIntent(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// ListByOperation returns the stored records whose operation matches
// namespace and name exactly, oldest first.
func (s *Store) ListByOperation(ctx context.Context, namespace, name string) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT action_id FROM actions
		  WHERE op_namespace = ? AND op_name = ?
		  ORDER BY requested_at ASC, action_id ASC`, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("action/sqlite: list by operation: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("action/sqlite: scan action id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("action/sqlite: iterate actions: %w", err)
	}
	out := make([]Record, 0, len(ids))
	for _, id := range ids {
		rec, err := s.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

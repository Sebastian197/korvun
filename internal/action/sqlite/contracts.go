// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Intent/grant persistence (Trust Layer Etapa 2, lote 3, pieza 3):
// create/activate/revoke intents, issue/delegate/revoke grants. Every
// lifecycle edge is validated from the STORED state (the Etapa-1 Finish
// mold), and delegation runs the pure §14.3 attenuation validator
// BEFORE persisting — a widening child never touches the disk. Growth
// is operator-scale by design (a human creates contracts deliberately:
// tens, not millions), so these tables carry no retention cap.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// ErrParentNotDelegable reports a delegation from a stored parent that is
// not ACTIVE: only in-force authority can be passed on (fail-closed; the
// STORED status decides, never the caller's copy).
var ErrParentNotDelegable = errors.New("action/sqlite: parent grant not delegable")

// jsonSet marshals a string set for a TEXT column.
func jsonSet(values []string) (string, error) {
	raw, err := json.Marshal(values)
	if err != nil {
		// Unreachable for string slices; kept for honesty.
		return "", fmt.Errorf("action/sqlite: marshal set: %w", err)
	}
	return string(raw), nil
}

// parseSet unmarshals a TEXT column back into a string set ("" = nil).
func parseSet(raw string) ([]string, error) {
	if raw == "" || raw == "null" {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("action/sqlite: parse set: %w", err)
	}
	return out, nil
}

// jsonPerOp marshals the per-operation budget map ("" for empty).
func jsonPerOp(m map[string]int) (string, error) {
	if len(m) == 0 {
		return "", nil
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("action/sqlite: marshal per-operation budgets: %w", err)
	}
	return string(raw), nil
}

// parsePerOp unmarshals the per-operation budget map ("" = nil).
func parsePerOp(raw string) (map[string]int, error) {
	if raw == "" {
		return nil, nil
	}
	var out map[string]int
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("action/sqlite: parse per-operation budgets: %w", err)
	}
	return out, nil
}

// timeOrNull renders a time column (zero time = NULL, no expiry).
func timeOrNull(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// parseNullTime reads a nullable time column back.
func parseNullTime(raw sql.NullString) (time.Time, error) {
	if !raw.Valid || raw.String == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, raw.String)
}

// CreateIntent stores a new intent contract. A contract cannot be born
// terminal — only DRAFT or ACTIVE may enter.
func (s *Store) CreateIntent(ctx context.Context, c action.IntentContract) error {
	if c.Status.Terminal() {
		return fmt.Errorf("action/sqlite: intent %q cannot be created terminal (%s)", c.IntentID, c.Status)
	}
	ops, err := jsonSet(c.AllowedOperations)
	if err != nil {
		return err
	}
	res, err := jsonSet(c.AllowedResources)
	if err != nil {
		return err
	}
	perOp, err := jsonPerOp(c.Budgets.MaxActionsPerOperation)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO intents (intent_id, schema_version, owner_principal_id,
		    purpose, operations, resources, max_actions, per_operation,
		    valid_from, expires_at, status, version, digest)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.IntentID, c.SchemaVersion, c.OwnerPrincipalID,
		c.Purpose, ops, res, c.Budgets.MaxActions, perOp,
		c.ValidFrom.UTC().Format(time.RFC3339Nano), timeOrNull(c.ExpiresAt),
		string(c.Status), c.Version, c.Digest(),
	); err != nil {
		return fmt.Errorf("action/sqlite: create intent %q: %w", c.IntentID, err)
	}
	return nil
}

// GetIntent returns one stored intent contract.
func (s *Store) GetIntent(ctx context.Context, intentID string) (action.IntentContract, error) {
	var (
		c          action.IntentContract
		ops        string
		res        string
		maxActions int
		perOp      sql.NullString
		validFrom  string
		expiresAt  sql.NullString
		status     string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT intent_id, schema_version, owner_principal_id, purpose,
		        operations, resources, max_actions, per_operation,
		        valid_from, expires_at, status, version
		   FROM intents WHERE intent_id = ?`, intentID,
	).Scan(&c.IntentID, &c.SchemaVersion, &c.OwnerPrincipalID, &c.Purpose,
		&ops, &res, &maxActions, &perOp, &validFrom, &expiresAt, &status, &c.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return action.IntentContract{}, fmt.Errorf("%w: intent %q", ErrNotFound, intentID)
	}
	if err != nil {
		return action.IntentContract{}, fmt.Errorf("action/sqlite: get intent %q: %w", intentID, err)
	}
	if c.AllowedOperations, err = parseSet(ops); err != nil {
		return action.IntentContract{}, err
	}
	if c.AllowedResources, err = parseSet(res); err != nil {
		return action.IntentContract{}, err
	}
	c.Budgets.MaxActions = maxActions
	if c.Budgets.MaxActionsPerOperation, err = parsePerOp(perOp.String); err != nil {
		return action.IntentContract{}, err
	}
	if c.ValidFrom, err = time.Parse(time.RFC3339Nano, validFrom); err != nil {
		return action.IntentContract{}, fmt.Errorf("action/sqlite: parse valid_from of intent %q: %w", intentID, err)
	}
	if c.ExpiresAt, err = parseNullTime(expiresAt); err != nil {
		return action.IntentContract{}, fmt.Errorf("action/sqlite: parse expires_at of intent %q: %w", intentID, err)
	}
	c.Status = action.LifecycleStatus(status)
	return c, nil
}

// TransitionIntent moves one intent along the lifecycle, validating the
// edge from the STORED status (the Finish mold).
func (s *Store) TransitionIntent(ctx context.Context, intentID string, to action.LifecycleStatus) error {
	return s.transitionContract(ctx, "intents", "intent_id", intentID, to)
}

// TransitionGrant moves one grant along the lifecycle, validating the
// edge from the STORED status.
func (s *Store) TransitionGrant(ctx context.Context, grantID string, to action.LifecycleStatus) error {
	return s.transitionContract(ctx, "grants", "grant_id", grantID, to)
}

// transitionContract is the shared stored-state lifecycle mold. The table
// and column names are compile-time literals from the two wrappers above,
// never external input.
func (s *Store) transitionContract(ctx context.Context, table, idColumn, id string, to action.LifecycleStatus) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("action/sqlite: begin transition: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var current string
	query := `SELECT status FROM ` + table + ` WHERE ` + idColumn + ` = ?` // #nosec G202 -- table/column are compile-time literals from the wrappers
	err = tx.QueryRowContext(ctx, query, id).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %s %q", ErrNotFound, table, id)
	}
	if err != nil {
		return fmt.Errorf("action/sqlite: read status of %s %q: %w", table, id, err)
	}
	if err := action.LifecycleTransition(action.LifecycleStatus(current), to); err != nil {
		return fmt.Errorf("action/sqlite: transition %s %q: %w", table, id, err)
	}
	update := `UPDATE ` + table + ` SET status = ? WHERE ` + idColumn + ` = ?` // #nosec G202 -- same compile-time literals
	if _, err := tx.ExecContext(ctx, update, string(to), id); err != nil {
		return fmt.Errorf("action/sqlite: update status of %s %q: %w", table, id, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("action/sqlite: commit transition of %s %q: %w", table, id, err)
	}
	return nil
}

// CreateGrant issues a grant directly under a stored intent (the foreign
// key enforces the intent's existence). Terminal births are rejected.
func (s *Store) CreateGrant(ctx context.Context, g action.AuthorityGrant) error {
	if g.Status.Terminal() {
		return fmt.Errorf("action/sqlite: grant %q cannot be created terminal (%s)", g.GrantID, g.Status)
	}
	ops, err := jsonSet(g.Operations)
	if err != nil {
		return err
	}
	res, err := jsonSet(g.ResourceScope)
	if err != nil {
		return err
	}
	perOp, err := jsonPerOp(g.Budgets.MaxActionsPerOperation)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO grants (grant_id, intent_id, issuer_principal_id,
		    subject_principal_id, parent_grant_id, operations, resources,
		    max_actions, per_operation, valid_from, expires_at, status,
		    depth_remaining, effect_ceiling, digest)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		g.GrantID, g.IntentID, g.IssuerPrincipalID,
		g.SubjectPrincipalID, g.ParentGrantID, ops, res,
		g.Budgets.MaxActions, perOp,
		g.ValidFrom.UTC().Format(time.RFC3339Nano), timeOrNull(g.ExpiresAt),
		string(g.Status), g.DelegationDepthRemaining, string(g.EffectCeiling), g.Digest(),
	); err != nil {
		return fmt.Errorf("action/sqlite: create grant %q: %w", g.GrantID, err)
	}
	return nil
}

// GetGrant returns one stored grant.
func (s *Store) GetGrant(ctx context.Context, grantID string) (action.AuthorityGrant, error) {
	var (
		g          action.AuthorityGrant
		ops        string
		res        string
		maxActions int
		perOp      sql.NullString
		validFrom  string
		expiresAt  sql.NullString
		status     string
		ceiling    string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT grant_id, intent_id, issuer_principal_id, subject_principal_id,
		        parent_grant_id, operations, resources, max_actions,
		        per_operation, valid_from, expires_at, status, depth_remaining,
		        effect_ceiling
		   FROM grants WHERE grant_id = ?`, grantID,
	).Scan(&g.GrantID, &g.IntentID, &g.IssuerPrincipalID, &g.SubjectPrincipalID,
		&g.ParentGrantID, &ops, &res, &maxActions, &perOp,
		&validFrom, &expiresAt, &status, &g.DelegationDepthRemaining, &ceiling)
	if errors.Is(err, sql.ErrNoRows) {
		return action.AuthorityGrant{}, fmt.Errorf("%w: grant %q", ErrNotFound, grantID)
	}
	if err != nil {
		return action.AuthorityGrant{}, fmt.Errorf("action/sqlite: get grant %q: %w", grantID, err)
	}
	if g.Operations, err = parseSet(ops); err != nil {
		return action.AuthorityGrant{}, err
	}
	if g.ResourceScope, err = parseSet(res); err != nil {
		return action.AuthorityGrant{}, err
	}
	g.Budgets.MaxActions = maxActions
	if g.Budgets.MaxActionsPerOperation, err = parsePerOp(perOp.String); err != nil {
		return action.AuthorityGrant{}, err
	}
	if g.ValidFrom, err = time.Parse(time.RFC3339Nano, validFrom); err != nil {
		return action.AuthorityGrant{}, fmt.Errorf("action/sqlite: parse valid_from of grant %q: %w", grantID, err)
	}
	if g.ExpiresAt, err = parseNullTime(expiresAt); err != nil {
		return action.AuthorityGrant{}, fmt.Errorf("action/sqlite: parse expires_at of grant %q: %w", grantID, err)
	}
	g.Status = action.LifecycleStatus(status)
	g.EffectCeiling = action.EffectClass(ceiling)
	return g, nil
}

// DelegateGrant persists a delegated child grant — the golden frontier:
// the STORED parent is loaded, must be ACTIVE (fail-closed), and the pure
// §14.3 attenuation validator runs BEFORE any write. A child that widens
// its parent in any dimension never touches the disk.
func (s *Store) DelegateGrant(ctx context.Context, child action.AuthorityGrant) error {
	if child.ParentGrantID == "" {
		return fmt.Errorf("action/sqlite: delegation of %q demands a named stored parent", child.GrantID)
	}
	parent, err := s.GetGrant(ctx, child.ParentGrantID)
	if err != nil {
		return err
	}
	if parent.Status != action.LifecycleActive {
		return fmt.Errorf("%w: %q is %s", ErrParentNotDelegable, parent.GrantID, parent.Status)
	}
	if err := action.ValidateAttenuation(parent, child); err != nil {
		return fmt.Errorf("action/sqlite: delegate %q: %w", child.GrantID, err)
	}
	return s.CreateGrant(ctx, child)
}

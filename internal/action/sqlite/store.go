// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package sqlite persists the Action Kernel's attempts and decisions
// (Trust Layer Etapa 1, spec FR-STORE, sealed 2026-08-30). It follows the
// house SQLite mold (WAL DSN, single serialized writer, boot-fatal Open,
// idempotent bootstrap) and — sealed decision 1 — SHARES the database
// file with the conversation store while owning its OWN schema lifecycle:
// the `action_schema` version table below never mixes with the
// conversation migrations, and vice versa. The store opens its own
// connection pool on the shared file; WAL plus the busy timeout make the
// two single-writer pools coexist safely.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/Sebastian197/korvun/internal/action"

	_ "modernc.org/sqlite" // the pure-Go driver the house already ships
)

// Decision is the persisted authorization outcome of one attempt: the
// gate's verdict and the rule that produced it (finite grammar, no free
// text from the model).
type Decision struct {
	// Outcome is the gate verdict: "allow", "deny" or "shadow".
	Outcome string
	// Rule names the deciding dimension (the ADR-0041 audit grammar).
	Rule string
}

// Record is one stored action with its decision and lifecycle facts.
type Record struct {
	// Envelope is the ActionEnvelope v1 as persisted (round-trips verbatim).
	Envelope action.Envelope
	// State is the action's current state.
	State action.State
	// Decision is the persisted gate verdict.
	Decision Decision
	// RecoveryMarker is "" normally and "crash_recovered" when a previous
	// life's non-terminal action was closed by the recovery pass.
	RecoveryMarker string
	// FinishedAt is nil while the action is not terminal.
	FinishedAt *time.Time
}

// ErrNotFound reports an unknown action_id.
var ErrNotFound = errors.New("action/sqlite: action not found")

// ErrNotADecisionState reports a RecordAttempt with a state that is not a
// decision outcome (only DENIED, SHADOWED and AUTHORIZED enter the store).
var ErrNotADecisionState = errors.New("action/sqlite: not a decision state")

// dsnQuery mirrors the house connection pragmas (the conversation store's
// mold): WAL for multi-pool robustness on the SHARED file, busy_timeout as
// the cross-pool safety net, foreign_keys on because the decision row
// references its action row.
const dsnQuery = "_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)"

// createStmt is the store's v1 schema. CREATE TABLE IF NOT EXISTS keeps
// the bootstrap idempotent; `action_schema` is this store's OWN lifecycle
// marker, deliberately separate from every conversation table.
const createStmt = `
CREATE TABLE IF NOT EXISTS action_schema (
    version INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS actions (
    action_id         TEXT    NOT NULL PRIMARY KEY,
    schema_version    INTEGER NOT NULL,
    correlation_id    TEXT    NOT NULL,
    source_kind       TEXT    NOT NULL,
    source_protocol   TEXT    NOT NULL,
    source_channel    TEXT    NOT NULL,
    op_namespace      TEXT    NOT NULL,
    op_name           TEXT    NOT NULL,
    op_version        INTEGER NOT NULL,
    parameters_digest TEXT    NOT NULL,
    effect_class      TEXT    NOT NULL,
    state             TEXT    NOT NULL,
    recovery_marker   TEXT    NOT NULL DEFAULT '',
    requested_at      TEXT    NOT NULL,
    finished_at       TEXT
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS actions_by_correlation ON actions(correlation_id);
CREATE INDEX IF NOT EXISTS actions_by_requested ON actions(requested_at);
CREATE TABLE IF NOT EXISTS action_decisions (
    action_id  TEXT NOT NULL PRIMARY KEY REFERENCES actions(action_id) ON DELETE CASCADE,
    outcome    TEXT NOT NULL,
    rule       TEXT NOT NULL,
    decided_at TEXT NOT NULL
) WITHOUT ROWID;`

// buildFileDSN is the house DSN builder (conversation store mold): url.URL
// percent-encodes the path so URI-significant characters cannot swallow
// the pragmas; a leading '/' canonicalizes Windows drive paths.
func buildFileDSN(slashed string) string {
	if len(slashed) == 0 || slashed[0] != '/' {
		slashed = "/" + slashed
	}
	return (&url.URL{Scheme: "file", Path: slashed, RawQuery: dsnQuery}).String()
}

// Store persists actions and decisions. All access flows through one
// serialized connection (the house single-writer discipline), so method
// calls are safe from concurrent brain workers.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the action store at path — normally the SAME
// file the conversation store uses (sealed decision 1). It creates the
// parent directory, applies the single-writer pool, bootstraps this
// store's own schema and lifecycle row, and pings, so a bad path or a
// corrupt file fails HERE (the boot-fatal posture), never on the first
// recorded action.
func Open(path string) (*Store, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("action/sqlite: resolve path %q: %w", path, err)
	}
	if dir := filepath.Dir(abs); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("action/sqlite: create data dir %q: %w", dir, err)
		}
	}
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(abs)))
	if err != nil {
		return nil, fmt.Errorf("action/sqlite: open %q: %w", abs, err)
	}
	// Single serialized writer, the load-bearing house discipline: one
	// connection serializes whole transactions, so the write patterns
	// below are race-free without SQLITE_BUSY between our own callers.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(createStmt); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("action/sqlite: bootstrap schema in %q: %w", abs, err)
	}
	if _, err := db.Exec(
		`INSERT INTO action_schema (version) SELECT 1 WHERE NOT EXISTS (SELECT 1 FROM action_schema)`,
	); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("action/sqlite: seed schema version in %q: %w", abs, err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("action/sqlite: ping %q: %w", abs, err)
	}
	return &Store{db: db}, nil
}

// Close releases the store's connection pool.
func (s *Store) Close() error {
	return s.db.Close()
}

// SchemaVersion reports the store's OWN schema lifecycle version.
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	var v int
	if err := s.db.QueryRowContext(ctx, `SELECT version FROM action_schema`).Scan(&v); err != nil {
		return 0, fmt.Errorf("action/sqlite: read schema version: %w", err)
	}
	return v, nil
}

// RecordAttempt persists one attempt WITH its decision in a single
// transaction — the blueprint contract: every attempt produces an
// explainable decision and a durable record BEFORE any effect. Only the
// three decision outcomes of the machine (DENIED, SHADOWED, AUTHORIZED)
// may enter; anything else wraps ErrNotADecisionState. A duplicate
// action_id fails atomically: the transaction leaves no partial write.
func (s *Store) RecordAttempt(ctx context.Context, env action.Envelope, d Decision, state action.State) error {
	switch state {
	case action.StateDenied, action.StateShadowed, action.StateAuthorized:
	default:
		return fmt.Errorf("%w: %s", ErrNotADecisionState, state)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("action/sqlite: begin record: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO actions (action_id, schema_version, correlation_id,
		    source_kind, source_protocol, source_channel,
		    op_namespace, op_name, op_version,
		    parameters_digest, effect_class, state, requested_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		env.ActionID, env.SchemaVersion, env.CorrelationID,
		env.Source.Kind, env.Source.Protocol, env.Source.Channel,
		env.Operation.Namespace, env.Operation.Name, env.Operation.Version,
		env.ParametersDigest, env.Effect.Class, string(state),
		env.RequestedAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("action/sqlite: insert action %q: %w", env.ActionID, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO action_decisions (action_id, outcome, rule, decided_at) VALUES (?, ?, ?, ?)`,
		env.ActionID, d.Outcome, d.Rule, env.RequestedAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("action/sqlite: insert decision %q: %w", env.ActionID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("action/sqlite: commit record %q: %w", env.ActionID, err)
	}
	return nil
}

// Finish closes an action into a terminal state, validating the machine's
// edge from the CURRENT stored state (only AUTHORIZED has terminal exits
// in Etapa 1). The terminal write commits before returning — restart
// durability is the AS-6 contract.
func (s *Store) Finish(ctx context.Context, actionID string, to action.State, finishedAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("action/sqlite: begin finish: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var current string
	err = tx.QueryRowContext(ctx, `SELECT state FROM actions WHERE action_id = ?`, actionID).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %q", ErrNotFound, actionID)
	}
	if err != nil {
		return fmt.Errorf("action/sqlite: read state %q: %w", actionID, err)
	}
	if err := action.Transition(action.State(current), to); err != nil {
		return fmt.Errorf("action/sqlite: finish %q: %w", actionID, err)
	}
	if !to.Terminal() {
		return fmt.Errorf("action/sqlite: finish %q: %w", actionID,
			action.Transition(action.StateAuthorized, to))
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE actions SET state = ?, finished_at = ? WHERE action_id = ?`,
		string(to), finishedAt.UTC().Format(time.RFC3339Nano), actionID,
	); err != nil {
		return fmt.Errorf("action/sqlite: finish %q: %w", actionID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("action/sqlite: commit finish %q: %w", actionID, err)
	}
	return nil
}

// Get returns one stored record, envelope round-tripped verbatim.
func (s *Store) Get(ctx context.Context, actionID string) (Record, error) {
	var (
		rec         Record
		state       string
		requestedAt string
		finishedAt  sql.NullString
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT a.action_id, a.schema_version, a.correlation_id,
		        a.source_kind, a.source_protocol, a.source_channel,
		        a.op_namespace, a.op_name, a.op_version,
		        a.parameters_digest, a.effect_class, a.state,
		        a.recovery_marker, a.requested_at, a.finished_at,
		        d.outcome, d.rule
		   FROM actions a JOIN action_decisions d ON d.action_id = a.action_id
		  WHERE a.action_id = ?`, actionID,
	).Scan(
		&rec.Envelope.ActionID, &rec.Envelope.SchemaVersion, &rec.Envelope.CorrelationID,
		&rec.Envelope.Source.Kind, &rec.Envelope.Source.Protocol, &rec.Envelope.Source.Channel,
		&rec.Envelope.Operation.Namespace, &rec.Envelope.Operation.Name, &rec.Envelope.Operation.Version,
		&rec.Envelope.ParametersDigest, &rec.Envelope.Effect.Class, &state,
		&rec.RecoveryMarker, &requestedAt, &finishedAt,
		&rec.Decision.Outcome, &rec.Decision.Rule,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, fmt.Errorf("%w: %q", ErrNotFound, actionID)
	}
	if err != nil {
		return Record{}, fmt.Errorf("action/sqlite: get %q: %w", actionID, err)
	}
	rec.State = action.State(state)
	at, err := time.Parse(time.RFC3339Nano, requestedAt)
	if err != nil {
		return Record{}, fmt.Errorf("action/sqlite: parse requested_at of %q: %w", actionID, err)
	}
	rec.Envelope.RequestedAt = at
	if finishedAt.Valid {
		f, err := time.Parse(time.RFC3339Nano, finishedAt.String)
		if err != nil {
			return Record{}, fmt.Errorf("action/sqlite: parse finished_at of %q: %w", actionID, err)
		}
		rec.FinishedAt = &f
	}
	return rec, nil
}

// Count returns the number of stored actions.
func (s *Store) Count(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM actions`).Scan(&n); err != nil {
		return 0, fmt.Errorf("action/sqlite: count: %w", err)
	}
	return n, nil
}

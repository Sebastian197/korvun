// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Signing-key persistence (Trust Layer Etapa 4, lote 2, spec FR-KEY):
// the ledger's ink registry. Retired keys are KEPT FOREVER — the domain
// API offers no delete, and every rotation closes the old key's
// validity window in the SAME transaction that opens the new one. At
// most one key is active. Growth is rotation-scale: a handful of rows
// per profile lifetime, the reason written here.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SigningKey is one row of the ink registry. A zero RetiredAt means the
// key is ACTIVE; a closed window means retired — kept for verification
// of historical receipts forever.
type SigningKey struct {
	KeyID     string
	PublicKey string
	CreatedAt time.Time
	RetiredAt time.Time
}

// PutSigningKey registers the FIRST key of a profile. While any key is
// active it refuses — rotation is the only path forward (the invariant:
// at most one active key, enforced here and by RotateSigningKey's
// transaction).
func (s *Store) PutSigningKey(ctx context.Context, keyID, publicKey string, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("action/sqlite: begin put signing key: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var active int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM signing_keys WHERE retired_at IS NULL`).Scan(&active); err != nil {
		return fmt.Errorf("action/sqlite: count active keys: %w", err)
	}
	if active > 0 {
		return fmt.Errorf("action/sqlite: an active signing key exists; rotate instead of putting %q", keyID)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO signing_keys (key_id, public_key, created_at, retired_at) VALUES (?, ?, ?, NULL)`,
		keyID, publicKey, at.UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("action/sqlite: put signing key %q: %w", keyID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("action/sqlite: commit put signing key %q: %w", keyID, err)
	}
	return nil
}

// RotateSigningKey retires the active key and registers the new one in
// ONE transaction: the old key's validity window closes at the same
// instant the new one opens — historical receipts keep verifying
// against the retired public key forever.
func (s *Store) RotateSigningKey(ctx context.Context, keyID, publicKey string, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("action/sqlite: begin rotate: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stamp := at.UTC().Format(time.RFC3339Nano)
	res, err := tx.ExecContext(ctx,
		`UPDATE signing_keys SET retired_at = ? WHERE retired_at IS NULL`, stamp)
	if err != nil {
		return fmt.Errorf("action/sqlite: retire active key: %w", err)
	}
	retired, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("action/sqlite: retire rows affected: %w", err)
	}
	if retired == 0 {
		return errors.New("action/sqlite: no active signing key to rotate")
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO signing_keys (key_id, public_key, created_at, retired_at) VALUES (?, ?, ?, NULL)`,
		keyID, publicKey, stamp,
	); err != nil {
		return fmt.Errorf("action/sqlite: register rotated key %q: %w", keyID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("action/sqlite: commit rotation to %q: %w", keyID, err)
	}
	return nil
}

// ActiveSigningKey returns the single active key (ErrNotFound when the
// keystore is empty or fully retired).
func (s *Store) ActiveSigningKey(ctx context.Context) (SigningKey, error) {
	return s.scanSigningKey(s.db.QueryRowContext(ctx,
		`SELECT key_id, public_key, created_at, retired_at FROM signing_keys
		  WHERE retired_at IS NULL`), "active signing key")
}

// GetSigningKey returns one key by id — retired keys included, forever.
func (s *Store) GetSigningKey(ctx context.Context, keyID string) (SigningKey, error) {
	return s.scanSigningKey(s.db.QueryRowContext(ctx,
		`SELECT key_id, public_key, created_at, retired_at FROM signing_keys
		  WHERE key_id = ?`, keyID), keyID)
}

// ListSigningKeys returns the whole ink registry, oldest first.
func (s *Store) ListSigningKeys(ctx context.Context) ([]SigningKey, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT key_id, public_key, created_at, retired_at FROM signing_keys
		  ORDER BY created_at ASC, key_id ASC`)
	if err != nil {
		return nil, fmt.Errorf("action/sqlite: list signing keys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []SigningKey
	for rows.Next() {
		key, err := scanSigningKeyRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("action/sqlite: iterate signing keys: %w", err)
	}
	return out, nil
}

// scanSigningKey adapts one QueryRow result.
func (s *Store) scanSigningKey(row *sql.Row, what string) (SigningKey, error) {
	key, err := scanSigningKeyRow(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return SigningKey{}, fmt.Errorf("%w: %s", ErrNotFound, what)
	}
	return key, err
}

// scanSigningKeyRow shares the scan across row and rows.
func scanSigningKeyRow(scan func(dest ...any) error) (SigningKey, error) {
	var (
		key       SigningKey
		createdAt string
		retiredAt sql.NullString
	)
	if err := scan(&key.KeyID, &key.PublicKey, &createdAt, &retiredAt); err != nil {
		return SigningKey{}, err
	}
	var err error
	if key.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return SigningKey{}, fmt.Errorf("action/sqlite: parse key created_at: %w", err)
	}
	if key.RetiredAt, err = parseNullTime(retiredAt); err != nil {
		return SigningKey{}, fmt.Errorf("action/sqlite: parse key retired_at: %w", err)
	}
	return key, nil
}

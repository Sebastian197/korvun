// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package sqlite is the durable implementation of the conversation.Store seam
// (ADR-0019, Stage 9 ADR-B), backed by SQLite through the pure-Go
// modernc.org/sqlite driver (no cgo — decisive for the Pi/ARM cross-compile).
//
// It is a subpackage so the conversation package stays a pure leaf: this package
// imports conversation + database/sql + the driver; conversation imports neither
// database/sql nor this package (ADR-0019 §1, mirroring internal/model/{ollama,
// groq}). The driver is registered under the "sqlite" database/sql name by the
// blank import below.
//
// Concurrency (ADR-0019 §3): a single serialized writer (db.SetMaxOpenConns(1)),
// so SQLITE_BUSY and write-write deadlock are structurally impossible. WAL and
// busy_timeout are set for robustness against checkpoints and external readers,
// but the serialization guarantee comes from the one-connection pool, not from
// busy_timeout. AppendTurns wraps its read-max-then-insert in one transaction so
// a group is atomic AND crash-consistent (a crash mid-group commits the whole
// pair or none).
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/Sebastian197/korvun/internal/conversation"

	_ "modernc.org/sqlite" // registers the pure-Go "sqlite" database/sql driver
)

// Compile-time assertion: SqliteStore is a Store.
var _ conversation.Store = (*SqliteStore)(nil)

// dsnQuery configures every connection: WAL for reader/checkpoint robustness,
// busy_timeout as a safety net, foreign_keys on for future-proofing.
// Serialization itself is enforced by SetMaxOpenConns(1), not by busy_timeout.
// It is the RawQuery of a file: URL (no leading '?') so the driver applies it as
// PRAGMAs on each connection.
const dsnQuery = "_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)"

const createTableStmt = `
CREATE TABLE IF NOT EXISTS turns (
    key     TEXT    NOT NULL,
    seq     INTEGER NOT NULL,
    role    TEXT    NOT NULL,
    content TEXT    NOT NULL,
    ts      INTEGER NOT NULL,
    PRIMARY KEY (key, seq)
) WITHOUT ROWID;`

// buildFileDSN builds the SQLite "file:" DSN from a forward-slashed absolute
// path (the result of filepath.ToSlash). dsnQuery is emitted verbatim as the
// query so the _pragma settings survive a path containing URI-significant
// characters.
//
// A Windows drive-letter path is forward-slashed but rootless ("C:/Users/x"):
// without a leading slash, url.URL renders it as "file://C:/..." and the SQLite
// driver reads "C:" as the URI authority ("invalid uri authority"). Prepending
// '/' yields the canonical "file:///C:/Users/x" — empty authority, drive letter
// in the path — which SQLite resolves correctly on Windows. Unix paths already
// start with '/', so this is a no-op for them and leaves their DSN unchanged.
func buildFileDSN(slashed string) string {
	if len(slashed) == 0 || slashed[0] != '/' {
		slashed = "/" + slashed
	}
	return (&url.URL{Scheme: "file", Path: slashed, RawQuery: dsnQuery}).String()
}

// SqliteStore persists conversation turns in a single SQLite database file. It
// implements conversation.Store and is closable.
type SqliteStore struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path and returns a ready store.
// It creates the parent directory if needed, applies the single-writer pool,
// bootstraps the schema (CREATE TABLE IF NOT EXISTS), and pings the database, so
// a bad path / corrupt or unwritable file fails HERE (the boot-fatal path app
// relies on, ADR-0019 §4/§5), not on the first message.
func Open(path string) (*SqliteStore, error) {
	// Resolve to an absolute path and build the DSN with net/url so a path
	// containing URI-significant characters (?, #, &) cannot corrupt the _pragma
	// query: naive "file:"+path+query concatenation lets a '?' in the path swallow
	// the pragmas (silently dropping WAL and mis-placing the file). url.URL
	// percent-encodes the path while emitting RawQuery verbatim.
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("sqlite: resolve path %q: %w", path, err)
	}
	if dir := filepath.Dir(abs); dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("sqlite: create data dir %q: %w", dir, err)
		}
	}
	// filepath.ToSlash makes the path separator '/' on every OS (a no-op on Unix;
	// on Windows it turns C:\Users\x into C:/Users/x) so buildFileDSN sees the
	// same shape everywhere — letting the Windows drive-letter case be tested
	// from a Unix host (see dsn_test.go).
	dsn := buildFileDSN(filepath.ToSlash(abs))

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %q: %w", abs, err)
	}
	// Single serialized writer: all access goes through one connection, so
	// SQLITE_BUSY and write-write deadlock cannot occur. LOAD-BEARING: it MUST
	// stay 1 — AppendTurns' SELECT MAX(seq)+1-then-INSERT is race-free only because
	// the single connection serializes whole transactions; raising this
	// reintroduces a read-max race and (key,seq) PK collisions (ADR-0019 §3).
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(createTableStmt); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite: bootstrap schema in %q: %w", abs, err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite: ping %q: %w", abs, err)
	}
	return &SqliteStore{db: db}, nil
}

// LoadRecent returns up to the last n turns for key, oldest first. n <= 0 returns
// no turns; an unknown key returns an empty slice; neither is an error
// (conversation.Store contract). Each call reads fresh rows, so the returned
// slice never aliases stored state.
func (s *SqliteStore) LoadRecent(ctx context.Context, key conversation.Key, n int) ([]conversation.Turn, error) {
	if n <= 0 {
		return nil, nil
	}
	// Take the last n by seq DESC, then reverse to oldest-first.
	rows, err := s.db.QueryContext(ctx,
		`SELECT role, content, ts, seq FROM turns WHERE key = ? ORDER BY seq DESC LIMIT ?`,
		string(key), n)
	if err != nil {
		return nil, fmt.Errorf("sqlite: LoadRecent %q: %w", key, err)
	}
	defer func() { _ = rows.Close() }()

	var desc []conversation.Turn
	for rows.Next() {
		var (
			role, content string
			tsNanos       int64
			seq           int
		)
		if err := rows.Scan(&role, &content, &tsNanos, &seq); err != nil {
			return nil, fmt.Errorf("sqlite: LoadRecent scan %q: %w", key, err)
		}
		// ts == 0 is the sentinel for a zero-value Timestamp (see AppendTurns), so
		// a zero Turn.Timestamp round-trips as zero rather than as time.Unix(0,0)
		// (1970), matching MemStore's value-preserving behavior.
		var ts time.Time
		if tsNanos != 0 {
			ts = time.Unix(0, tsNanos).UTC()
		}
		desc = append(desc, conversation.Turn{
			Role:      conversation.Role(role),
			Content:   content,
			Timestamp: ts,
			Seq:       seq,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: LoadRecent rows %q: %w", key, err)
	}

	// Reverse in place to oldest-first.
	for i, j := 0, len(desc)-1; i < j; i, j = i+1, j-1 {
		desc[i], desc[j] = desc[j], desc[i]
	}
	return desc, nil
}

// Append atomically adds one turn to key and returns it with its store-assigned
// Seq filled in. It delegates to AppendTurns so the Seq logic lives in one place.
func (s *SqliteStore) Append(ctx context.Context, key conversation.Key, turn conversation.Turn) (conversation.Turn, error) {
	out, err := s.AppendTurns(ctx, key, turn)
	if err != nil {
		return conversation.Turn{}, err
	}
	return out[0], nil
}

// AppendTurns atomically appends a group of turns to key under a single
// transaction, assigning consecutive Seq values (the next indices in the key's
// history) and returning them Seq-filled. The single transaction gives both
// group atomicity and crash-consistency: a crash mid-group commits the whole
// group or none of it (ADR-0019 §3, closing what ADR-0018 §5 deferred). An empty
// group is a no-op returning (nil, nil).
func (s *SqliteStore) AppendTurns(ctx context.Context, key conversation.Key, turns ...conversation.Turn) ([]conversation.Turn, error) {
	if len(turns) == 0 {
		return nil, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("sqlite: AppendTurns begin %q: %w", key, err)
	}
	// Rollback is a no-op once Commit succeeds; on any error path it discards the
	// partial group (crash-consistency).
	defer func() { _ = tx.Rollback() }()

	// Next seq for this key. COALESCE(...,0) handles the empty-history case.
	// Running inside the (serialized) transaction means no other writer can read
	// the same MAX before this group inserts.
	var base int
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq)+1, 0) FROM turns WHERE key = ?`, string(key)).Scan(&base); err != nil {
		return nil, fmt.Errorf("sqlite: AppendTurns next-seq %q: %w", key, err)
	}

	out := make([]conversation.Turn, len(turns))
	for i, turn := range turns {
		seq := base + i
		turn.Seq = seq
		// A zero Timestamp stores as 0 (not UnixNano(), which overflows for the
		// year-1 zero value and would read back as ~1754, corrupting the value and
		// any ts-ordered query). 0 is the sentinel LoadRecent maps back to zero.
		var ns int64
		if !turn.Timestamp.IsZero() {
			ns = turn.Timestamp.UTC().UnixNano()
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO turns(key, seq, role, content, ts) VALUES (?, ?, ?, ?, ?)`,
			string(key), seq, string(turn.Role), turn.Content, ns,
		); err != nil {
			return nil, fmt.Errorf("sqlite: AppendTurns insert %q seq %d: %w", key, seq, err)
		}
		out[i] = turn
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("sqlite: AppendTurns commit %q: %w", key, err)
	}
	return out, nil
}

// Close closes the underlying database. The app closes the store LAST (after
// router.Shutdown) so a Close cannot race a still-running AppendTurns into a
// closed DB (ADR-0019 §6). NOTE: closing last prevents that race; it does not by
// itself guarantee the final in-flight turn was committed, because the router
// cancels its context on shutdown (see the durability note in app.Shutdown).
func (s *SqliteStore) Close() error {
	return s.db.Close()
}

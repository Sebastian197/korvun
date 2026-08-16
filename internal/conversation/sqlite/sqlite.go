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
	"strings"
	"time"

	"github.com/Sebastian197/korvun/internal/conversation"

	_ "modernc.org/sqlite" // registers the pure-Go "sqlite" database/sql driver
)

// Compile-time assertions: *SqliteStore satisfies the Store seam and its
// sessionful superset (operator-console spec SP1).
var (
	_ conversation.Store        = (*SqliteStore)(nil)
	_ conversation.SessionStore = (*SqliteStore)(nil)
)

// tsToTime maps a stored ts back to time.Time, honoring the 0 sentinel for
// the zero value (see AppendTurns).
func tsToTime(ns int64) time.Time {
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns).UTC()
}

// NewSession opens a fresh session for key and returns its id
// (conversation.SessionStore). Idempotent on an empty active session; a key
// with no history returns 1. The single transaction on the serialized
// connection makes resolve-then-insert race-free (the ADR-0019 §3 discipline).
func (s *SqliteStore) NewSession(ctx context.Context, key conversation.Key) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("sqlite: NewSession begin %q: %w", key, err)
	}
	defer func() { _ = tx.Rollback() }()

	var active int
	if err := tx.QueryRowContext(ctx, activeSessionQuery, string(key)).Scan(&active); err != nil {
		return 0, fmt.Errorf("sqlite: NewSession active %q: %w", key, err)
	}
	if active > 0 {
		var turnsInActive int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM turns WHERE key = ? AND session = ?`,
			string(key), active).Scan(&turnsInActive); err != nil {
			return 0, fmt.Errorf("sqlite: NewSession count %q: %w", key, err)
		}
		if turnsInActive == 0 {
			// Idempotent: never stack empty sessions.
			return active, tx.Commit()
		}
	}
	next := active + 1
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO sessions(key, id, created_ts) VALUES (?, ?, ?)`,
		string(key), next, time.Now().UTC().UnixNano()); err != nil {
		return 0, fmt.Errorf("sqlite: NewSession insert %q: %w", key, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("sqlite: NewSession commit %q: %w", key, err)
	}
	return next, nil
}

// ListConversations lists up to limit conversations, most recent activity
// first (conversation.SessionStore). LastActivity/LastRole come from the
// key's most recent turn across sessions; ties order by key for determinism.
func (s *SqliteStore) ListConversations(ctx context.Context, limit int) ([]conversation.ConversationInfo, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.key,
		       MAX(s.id),
		       COUNT(s.id),
		       (SELECT COUNT(*) FROM turns tc WHERE tc.key = s.key),
		       COALESCE((SELECT MAX(t.ts) FROM turns t WHERE t.key = s.key), 0),
		       COALESCE((SELECT t2.role FROM turns t2 WHERE t2.key = s.key
		                 ORDER BY t2.session DESC, t2.seq DESC LIMIT 1), '')
		FROM sessions s
		GROUP BY s.key
		ORDER BY 4 DESC, s.key ASC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("sqlite: ListConversations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []conversation.ConversationInfo
	for rows.Next() {
		var (
			key, role string
			active    int
			count     int
			turnTotal int
			ns        int64
		)
		if err := rows.Scan(&key, &active, &count, &turnTotal, &ns, &role); err != nil {
			return nil, fmt.Errorf("sqlite: ListConversations scan: %w", err)
		}
		out = append(out, conversation.ConversationInfo{
			Key:           conversation.Key(key),
			ActiveSession: active,
			SessionCount:  count,
			TurnCount:     turnTotal,
			LastActivity:  tsToTime(ns),
			LastRole:      conversation.Role(role),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: ListConversations rows: %w", err)
	}
	return out, nil
}

// ListSessions lists every session of key, oldest first
// (conversation.SessionStore). An unknown key returns an empty slice.
func (s *SqliteStore) ListSessions(ctx context.Context, key conversation.Key) ([]conversation.SessionInfo, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id,
		       COUNT(t.seq),
		       COALESCE(MIN(t.ts), 0),
		       COALESCE(MAX(t.ts), 0)
		FROM sessions s
		LEFT JOIN turns t ON t.key = s.key AND t.session = s.id
		WHERE s.key = ?
		GROUP BY s.id
		ORDER BY s.id ASC`, string(key))
	if err != nil {
		return nil, fmt.Errorf("sqlite: ListSessions %q: %w", key, err)
	}
	defer func() { _ = rows.Close() }()

	var out []conversation.SessionInfo
	for rows.Next() {
		var (
			id, count   int
			first, last int64
		)
		if err := rows.Scan(&id, &count, &first, &last); err != nil {
			return nil, fmt.Errorf("sqlite: ListSessions scan %q: %w", key, err)
		}
		out = append(out, conversation.SessionInfo{
			ID:        id,
			TurnCount: count,
			First:     tsToTime(first),
			Last:      tsToTime(last),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: ListSessions rows %q: %w", key, err)
	}
	return out, nil
}

// LoadSession returns ALL turns of the given session of key, oldest first
// (conversation.SessionStore). An unknown key or session returns an empty
// slice, not an error.
func (s *SqliteStore) LoadSession(ctx context.Context, key conversation.Key, session int) ([]conversation.Turn, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT role, content, ts, seq FROM turns
		 WHERE key = ? AND session = ? ORDER BY seq ASC`,
		string(key), session)
	if err != nil {
		return nil, fmt.Errorf("sqlite: LoadSession %q/%d: %w", key, session, err)
	}
	defer func() { _ = rows.Close() }()

	var out []conversation.Turn
	for rows.Next() {
		var (
			role, content string
			ns            int64
			seq           int
		)
		if err := rows.Scan(&role, &content, &ns, &seq); err != nil {
			return nil, fmt.Errorf("sqlite: LoadSession scan %q/%d: %w", key, session, err)
		}
		out = append(out, conversation.Turn{
			Role:      conversation.Role(role),
			Content:   content,
			Timestamp: tsToTime(ns),
			Seq:       seq,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: LoadSession rows %q/%d: %w", key, session, err)
	}
	return out, nil
}

// dsnQuery configures every connection: WAL for reader/checkpoint robustness,
// busy_timeout as a safety net, foreign_keys on for future-proofing.
// Serialization itself is enforced by SetMaxOpenConns(1), not by busy_timeout.
// It is the RawQuery of a file: URL (no leading '?') so the driver applies it as
// PRAGMAs on each connection.
const dsnQuery = "_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)"

// The v2 schema (operator-console spec SP1, FR-STORE-1): turns are scoped by
// session — turn identity is (key, session, seq), seq restarting per session
// — and the sessions table is the source of truth for which sessions exist
// (an EMPTY session has no turns rows, so it can only live here). The ACTIVE
// session of a key is its highest id.
const createTableStmt = `
CREATE TABLE IF NOT EXISTS sessions (
    key        TEXT    NOT NULL,
    id         INTEGER NOT NULL,
    created_ts INTEGER NOT NULL,
    PRIMARY KEY (key, id)
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS turns (
    key     TEXT    NOT NULL,
    session INTEGER NOT NULL,
    seq     INTEGER NOT NULL,
    role    TEXT    NOT NULL,
    content TEXT    NOT NULL,
    ts      INTEGER NOT NULL,
    PRIMARY KEY (key, session, seq)
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS notes (
    brain   TEXT    NOT NULL,
    key     TEXT    NOT NULL,
    seq     INTEGER NOT NULL,
    content TEXT    NOT NULL,
    ts      INTEGER NOT NULL,
    PRIMARY KEY (brain, key, seq)
) WITHOUT ROWID;`

// migrateV1Stmt upgrades a pre-session database (the 2026-08 v1 schema:
// turns keyed by (key, seq)) in ONE transaction: every existing turn becomes
// session 1 of its key — same seq, same order, nothing lost — and session 1
// is registered as that key's (active) session. created_ts 0 is the same
// zero-sentinel the ts column already uses.
const migrateV1Stmt = `
ALTER TABLE turns RENAME TO turns_v1;
CREATE TABLE turns (
    key     TEXT    NOT NULL,
    session INTEGER NOT NULL,
    seq     INTEGER NOT NULL,
    role    TEXT    NOT NULL,
    content TEXT    NOT NULL,
    ts      INTEGER NOT NULL,
    PRIMARY KEY (key, session, seq)
) WITHOUT ROWID;
INSERT INTO turns (key, session, seq, role, content, ts)
    SELECT key, 1, seq, role, content, ts FROM turns_v1;
CREATE TABLE IF NOT EXISTS sessions (
    key        TEXT    NOT NULL,
    id         INTEGER NOT NULL,
    created_ts INTEGER NOT NULL,
    PRIMARY KEY (key, id)
) WITHOUT ROWID;
INSERT INTO sessions (key, id, created_ts)
    SELECT DISTINCT key, 1, 0 FROM turns_v1;
DROP TABLE turns_v1;`

// migrateIfV1 detects a pre-session database (a turns table without the
// session column) and upgrades it. Idempotent by construction: a migrated or
// fresh database never matches the detection predicate again.
func migrateIfV1(db *sql.DB) error {
	var turnsExists int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'turns'`,
	).Scan(&turnsExists); err != nil {
		return fmt.Errorf("detect turns table: %w", err)
	}
	if turnsExists == 0 {
		return nil // fresh database — nothing to migrate
	}
	var hasSession int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('turns') WHERE name = 'session'`,
	).Scan(&hasSession); err != nil {
		return fmt.Errorf("detect session column: %w", err)
	}
	if hasSession > 0 {
		return nil // already v2
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(migrateV1Stmt); err != nil {
		return fmt.Errorf("run migration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}
	return nil
}

// activeSessionQuery resolves a key's active session id (0 = no sessions).
const activeSessionQuery = `SELECT COALESCE(MAX(id), 0) FROM sessions WHERE key = ?`

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
		// 0o700: config + DB + conversation memory are one person's data
		// (aligned 2026-07-25 with the shell's first-run dir; an existing
		// directory keeps its mode — MkdirAll never chmods retroactively).
		if err := os.MkdirAll(dir, 0o700); err != nil {
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

	// Pre-session databases upgrade HERE, before the v2 bootstrap — same
	// boot-fatal posture as a bad path: a failed migration fails Open, never
	// the first message (AS-12: the fixture test proves nothing is lost).
	if err := migrateIfV1(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite: migrate %q: %w", abs, err)
	}
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
	// Take the last n of the ACTIVE session by seq DESC, then reverse to
	// oldest-first. Scoping to the active session is FR-SESS-2: a session
	// reset is a hard context cut — previous sessions never surface here.
	rows, err := s.db.QueryContext(ctx,
		`SELECT role, content, ts, seq FROM turns
		 WHERE key = ? AND session = (`+activeSessionQuery+`)
		 ORDER BY seq DESC LIMIT ?`,
		string(key), string(key), n)
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

	// Resolve the ACTIVE session, creating session 1 for a brand-new key
	// (the same implicit-first-session behavior MemStore has). Running inside
	// the (serialized) transaction means no other writer can race the same
	// resolution.
	var active int
	if err := tx.QueryRowContext(ctx, activeSessionQuery, string(key)).Scan(&active); err != nil {
		return nil, fmt.Errorf("sqlite: AppendTurns active-session %q: %w", key, err)
	}
	if active == 0 {
		active = 1
		var created int64
		if !turns[0].Timestamp.IsZero() {
			created = turns[0].Timestamp.UTC().UnixNano()
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO sessions(key, id, created_ts) VALUES (?, ?, ?)`,
			string(key), active, created); err != nil {
			return nil, fmt.Errorf("sqlite: AppendTurns open session %q: %w", key, err)
		}
	}

	// Next seq WITHIN the active session. COALESCE(...,0) handles the
	// empty-session case. Same serialized-transaction guarantee as above.
	var base int
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq)+1, 0) FROM turns WHERE key = ? AND session = ?`,
		string(key), active).Scan(&base); err != nil {
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
			`INSERT INTO turns(key, session, seq, role, content, ts) VALUES (?, ?, ?, ?, ?, ?)`,
			string(key), active, seq, string(turn.Role), turn.Content, ns,
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

// LoadSessionTail returns the LAST n turns of the given session of key,
// oldest-first among themselves (conversation.SessionStore, minimal-memory
// FR-STORE-A1). It is LoadRecent's exact pattern (ORDER BY seq DESC LIMIT n
// + reverse) scoped to the REQUESTED session instead of the active one.
// n <= 0 or an unknown key/session returns no turns, no error.
func (s *SqliteStore) LoadSessionTail(ctx context.Context, key conversation.Key, session, n int) ([]conversation.Turn, error) {
	if n <= 0 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT role, content, ts, seq FROM turns
		 WHERE key = ? AND session = ?
		 ORDER BY seq DESC LIMIT ?`,
		string(key), session, n)
	if err != nil {
		return nil, fmt.Errorf("sqlite: LoadSessionTail %q/%d: %w", key, session, err)
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
			return nil, fmt.Errorf("sqlite: LoadSessionTail scan %q/%d: %w", key, session, err)
		}
		// ts == 0 is the zero-Timestamp sentinel (see AppendTurns/LoadRecent).
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
		return nil, fmt.Errorf("sqlite: LoadSessionTail rows %q/%d: %w", key, session, err)
	}

	// Reverse in place to oldest-first.
	for i, j := 0, len(desc)-1; i < j; i, j = i+1, j-1 {
		desc[i], desc[j] = desc[j], desc[i]
	}
	return desc, nil
}

// AppendNote implements conversation.NoteStore (minimal-memory FR-STORE-1/2):
// pair validation, then count + insert in ONE transaction — the serialized
// single writer makes the cap race-free (ADR-0019 §3). The store stamps ts.
func (s *SqliteStore) AppendNote(ctx context.Context, brain string, scope conversation.NoteScope, key conversation.Key, content string, maxNotes int) (conversation.Note, error) {
	if err := conversation.CheckNotePair(scope, key); err != nil {
		return conversation.Note{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return conversation.Note{}, fmt.Errorf("sqlite: AppendNote begin %q/%q: %w", brain, key, err)
	}
	defer func() { _ = tx.Rollback() }()

	var count int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notes WHERE brain = ? AND key = ?`, brain, string(key)).Scan(&count); err != nil {
		return conversation.Note{}, fmt.Errorf("sqlite: AppendNote count %q/%q: %w", brain, key, err)
	}
	if maxNotes > 0 && count >= maxNotes {
		return conversation.Note{}, conversation.ErrNotesFull
	}
	var next int
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) + 1 FROM notes WHERE brain = ? AND key = ?`, brain, string(key)).Scan(&next); err != nil {
		return conversation.Note{}, fmt.Errorf("sqlite: AppendNote seq %q/%q: %w", brain, key, err)
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO notes (brain, key, seq, content, ts) VALUES (?, ?, ?, ?, ?)`,
		brain, string(key), next, content, now.UnixNano()); err != nil {
		return conversation.Note{}, fmt.Errorf("sqlite: AppendNote insert %q/%q: %w", brain, key, err)
	}
	if err := tx.Commit(); err != nil {
		return conversation.Note{}, fmt.Errorf("sqlite: AppendNote commit %q/%q: %w", brain, key, err)
	}
	return conversation.Note{Seq: next, Content: content, Timestamp: now}, nil
}

// ListNotes implements conversation.NoteStore (minimal-memory FR-STORE-1/2):
// the scope's notes oldest-first; an unknown scope returns an empty slice.
func (s *SqliteStore) ListNotes(ctx context.Context, brain string, _ conversation.NoteScope, key conversation.Key) ([]conversation.Note, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT seq, content, ts FROM notes WHERE brain = ? AND key = ? ORDER BY seq ASC`,
		brain, string(key))
	if err != nil {
		return nil, fmt.Errorf("sqlite: ListNotes %q/%q: %w", brain, key, err)
	}
	defer func() { _ = rows.Close() }()

	var out []conversation.Note
	for rows.Next() {
		var (
			seq     int
			content string
			tsNanos int64
		)
		if err := rows.Scan(&seq, &content, &tsNanos); err != nil {
			return nil, fmt.Errorf("sqlite: ListNotes scan %q/%q: %w", brain, key, err)
		}
		var ts time.Time
		if tsNanos != 0 {
			ts = time.Unix(0, tsNanos).UTC()
		}
		out = append(out, conversation.Note{Seq: seq, Content: content, Timestamp: ts})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: ListNotes rows %q/%q: %w", brain, key, err)
	}
	return out, nil
}

// ClearNotes implements conversation.NoteStore (minimal-memory FR-STORE-1/2):
// the scope's notes gone; an unknown scope is a no-op.
func (s *SqliteStore) ClearNotes(ctx context.Context, brain string, _ conversation.NoteScope, key conversation.Key) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM notes WHERE brain = ? AND key = ?`, brain, string(key)); err != nil {
		return fmt.Errorf("sqlite: ClearNotes %q/%q: %w", brain, key, err)
	}
	return nil
}

// DeleteConversation implements conversation.SessionStore (FR-DEL-1): every
// turn and session row of key gone in ONE transaction — really deleted, not
// hidden. An unknown key is a no-op.
func (s *SqliteStore) DeleteConversation(ctx context.Context, key conversation.Key) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: DeleteConversation begin %q: %w", key, err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM turns WHERE key = ?`, string(key)); err != nil {
		return fmt.Errorf("sqlite: DeleteConversation turns %q: %w", key, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE key = ?`, string(key)); err != nil {
		return fmt.Errorf("sqlite: DeleteConversation sessions %q: %w", key, err)
	}
	// The key's notes go with the conversation, across all brains — FR-DEL-1
	// "really gone" stays true (minimal-memory FR-STORE-2). Brain-global
	// notes live under the EMPTY key and are not the conversation's: a
	// non-empty key match never touches them, stated to the face.
	if _, err := tx.ExecContext(ctx, `DELETE FROM notes WHERE key = ?`, string(key)); err != nil {
		return fmt.Errorf("sqlite: DeleteConversation notes %q: %w", key, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: DeleteConversation commit %q: %w", key, err)
	}
	return nil
}

// DeleteSession implements conversation.SessionStore (FR-DEL-1): one
// ARCHIVED session's turns and registration, in one transaction. The active
// session is protected by conversation.ErrActiveSession; an unknown key or
// session is a no-op.
func (s *SqliteStore) DeleteSession(ctx context.Context, key conversation.Key, session int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: DeleteSession begin %q: %w", key, err)
	}
	defer func() { _ = tx.Rollback() }()
	var active int
	if err := tx.QueryRowContext(ctx, activeSessionQuery, string(key)).Scan(&active); err != nil {
		return fmt.Errorf("sqlite: DeleteSession active %q: %w", key, err)
	}
	if active == 0 {
		return nil // unknown key: no-op
	}
	if session == active {
		return conversation.ErrActiveSession
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM turns WHERE key = ? AND session = ?`, string(key), session); err != nil {
		return fmt.Errorf("sqlite: DeleteSession turns %q/%d: %w", key, session, err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM sessions WHERE key = ? AND id = ?`, string(key), session); err != nil {
		return fmt.Errorf("sqlite: DeleteSession session %q/%d: %w", key, session, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: DeleteSession commit %q/%d: %w", key, session, err)
	}
	return nil
}

// likeEscape makes a user query safe inside a LIKE pattern: backslash the
// escape character itself, then the %% and _ wildcards, so they match
// literally (FR-SEARCH).
func likeEscape(q string) string {
	q = strings.ReplaceAll(q, `\`, `\\`)
	q = strings.ReplaceAll(q, `%`, `\%`)
	q = strings.ReplaceAll(q, `_`, `\_`)
	return q
}

// SearchTurns implements conversation.SessionStore (FR-SEARCH):
// case-insensitive LIKE over every turn (sqlite LIKE is case-insensitive
// for ASCII by default), wildcards escaped so the query matches literally,
// newest first with the key/seq tiebreak, up to limit.
func (s *SqliteStore) SearchTurns(ctx context.Context, query string, limit int) ([]conversation.SearchHit, error) {
	q := strings.TrimSpace(query)
	if q == "" || limit <= 0 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT key, session, seq, role, content, ts FROM turns
		WHERE content LIKE ? ESCAPE '\'
		ORDER BY ts DESC, key ASC, session DESC, seq DESC
		LIMIT ?`,
		"%"+likeEscape(q)+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("sqlite: SearchTurns: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []conversation.SearchHit
	for rows.Next() {
		var (
			key, role, content string
			session, seq       int
			ns                 int64
		)
		if err := rows.Scan(&key, &session, &seq, &role, &content, &ns); err != nil {
			return nil, fmt.Errorf("sqlite: SearchTurns scan: %w", err)
		}
		out = append(out, conversation.SearchHit{
			Key:       conversation.Key(key),
			Session:   session,
			Seq:       seq,
			Role:      conversation.Role(role),
			Content:   content,
			Timestamp: tsToTime(ns),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: SearchTurns rows: %w", err)
	}
	return out, nil
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The ledger's error classes (train E, plan §5): what a failure of the
// storage IS, decided by the SQLite result code the driver attaches to it —
// never by its text.

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrLedgerEnvironment names a ledger the storage AROUND it could not serve:
// permissions, a read-only or full disk, an input/output failure, a file that
// cannot be opened (SQLITE_PERM, READONLY, IOERR, FULL, CANTOPEN, NOLFS,
// AUTH). It is never a verdict on the ledger's contents.
var ErrLedgerEnvironment = errors.New("action/sqlite: ledger_environment: the storage around the ledger failed")

// ledgerClass is the class of a storage failure.
type ledgerClass int

const (
	// classNone is every failure the ledger does not name: no code, or a
	// code outside classStructural, classBusy and classEnvironment.
	classNone ledgerClass = iota
	// classStructural is a file the ledger cannot read as its own: ERROR,
	// CORRUPT, MISMATCH, FORMAT, NOTADB.
	classStructural
	// classBusy is another connection in the way: BUSY, LOCKED.
	classBusy
	// classEnvironment is the storage around the file failing: PERM,
	// READONLY, IOERR, FULL, CANTOPEN, NOLFS, AUTH.
	classEnvironment
)

// String names the class for the moulds' messages.
func (c ledgerClass) String() string {
	switch c {
	case classStructural:
		return "structural"
	case classBusy:
		return "busy"
	case classEnvironment:
		return "environment"
	default:
		return "none"
	}
}

// classOf reads the class of err from the primary result code (the low byte
// of the code, so the extended codes fold into their primary) of the first
// error in its chain that carries one.
func classOf(err error) ledgerClass {
	var coded interface{ Code() int }
	if !errors.As(err, &coded) {
		return classNone
	}
	switch coded.Code() & 0xff {
	case 1, 11, 20, 24, 26: // ERROR, CORRUPT, MISMATCH, FORMAT, NOTADB
		return classStructural
	case 5, 6: // BUSY, LOCKED
		return classBusy
	case 3, 8, 10, 13, 14, 22, 23: // PERM, READONLY, IOERR, FULL, CANTOPEN, NOLFS, AUTH
		return classEnvironment
	default:
		return classNone
	}
}

// readVerdict is the ONE decision a failed judge read meets: whether the
// failure is a verdict on the file or an error of the moment. A read that
// really ran and failed with a structural code is a verdict; a connection
// that could not be born is never one (the judge reads only through a
// connection already born, so its failure is not the read's); every other
// failure is an error of the moment. Every judge site asks it, so the moulds
// can prove which decision a failure met.
func readVerdict(site judgeQuerySite, err error) bool {
	verdict := !isBirthFailure(err) && classOf(err) == classStructural
	noteReadDecision(site, err, verdict)
	return verdict
}

// classifyLedgerError names a failure that leaves the store through one of
// its public boundaries (OpenFor, OpenOperatorFor, OpenReadOnlyFor, Standing)
// or through the judgement of a write, an adoption or a maintenance: the
// class of its code wraps the original error, which stays in the chain.
// nil, a failure the package already names, a context's own end and every
// failure outside the three classes come back as they are.
func classifyLedgerError(err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || namedByTheStore(err) {
		return err
	}
	switch classOf(err) {
	case classStructural:
		return fmt.Errorf("%w: %w", ErrLedgerUnreadable, err)
	case classBusy:
		return fmt.Errorf("%w: %w", ErrLedgerBusy, err)
	case classEnvironment:
		return fmt.Errorf("%w: %w", ErrLedgerEnvironment, err)
	default:
		return err
	}
}

// storeNames are the failures the package names itself: the classification
// never renames them.
var storeNames = []error{
	ErrLedgerUnreadable, ErrLedgerMarkMalformed, ErrLedgerBusy, ErrLedgerEnvironment,
	ErrNoActionStore, ErrSchemaBehind, ErrSchemaFromTheFuture, ErrLedgerForeignProfile,
	ErrLedgerGuardUnset, ErrGuardHandleUnknown, ErrNoProfileIdentity,
	ErrProfileIdentityMalformed, ErrProfileIdentityAlreadySet,
}

func namedByTheStore(err error) bool {
	for _, name := range storeNames {
		if errors.Is(err, name) {
			return true
		}
	}
	return false
}

// connectionBirthError names a connection the pool could not hand over: the
// birth of a new one failed (a pragma of the DSN, the guard's hook) or the
// acquisition was refused (a closed pool, an ended context). It is never a
// judgement of the file — the judge reads only through a connection obtained
// before its first read (withLedgerConnection) — and its text carries the
// cause's.
type connectionBirthError struct{ err error }

func (e *connectionBirthError) Error() string {
	return "action/sqlite: obtain a connection: " + e.err.Error()
}

func (e *connectionBirthError) Unwrap() error { return e.err }

// isBirthFailure tells a connection that could not be obtained from a read
// that failed on one.
func isBirthFailure(err error) bool {
	var birth *connectionBirthError
	return errors.As(err, &birth)
}

// withLedgerConnection runs fn on ONE connection of db, obtained before fn
// reads anything and returned to the pool when fn returns: a connection that
// cannot be born fails HERE, as a connectionBirthError, never inside one of
// fn's reads. The pool holds one connection; fn must not ask db for another.
func withLedgerConnection(ctx context.Context, db *sql.DB, fn func(*sql.Conn) error) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return &connectionBirthError{err: err}
	}
	defer func() { _ = conn.Close() }()
	return fn(conn)
}

// parseStoredVersion reads one stored action_schema.version cell — the ONE
// contract of every reader of it (the shape, the migration, the foreign
// version check, SchemaVersion): an integer, or text or bytes that, trimmed
// of spaces, are a decimal integer. NULL and anything else are named with
// the field and what it holds.
func parseStoredVersion(v any) (int, error) {
	switch x := v.(type) {
	case int64:
		return parseVersionText(strconv.FormatInt(x, 10))
	case []byte:
		return parseVersionText(string(x))
	case string:
		return parseVersionText(x)
	case nil:
		return 0, errors.New("action_schema.version is NULL")
	default:
		return 0, fmt.Errorf("action_schema.version holds a %T, not an integer", v)
	}
}

func parseVersionText(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("action_schema.version %q is not a number", s)
	}
	return n, nil
}

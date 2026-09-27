// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The durable mark «ledger founded by this profile» (director's order,
// 2026-09-24; redesigned the same day, train B).
//
// A ledger is founded by a profile (the `enable-storage` door) and can be
// ADOPTED by another with an explicit act. Both leave a signed receipt whose
// result_digest carries the mark `profile:sha256:<digest of the profile's
// identity>` as evidence, and both write the one-row table ledger_identity in
// the same transaction as that receipt: the ROW is the state of who owns the
// ledger. Every handle is opened for the profile it serves (OpenFor,
// OpenOperatorFor, OpenReadOnlyFor) and judges the row on EVERY write that
// creates an act, inside the write's own immediate transaction — never from
// a cached verdict, so two live handles cannot disagree for long and a
// profile that adopts is seen by the former owner on its next write — and a
// FOREIGN ledger refuses those writes by name. Reads never block. Closing an
// act already sealed is not a new act. A ledger with no row and no mark is
// LEGACY: named, not corrupt, not blocking — every ledger written before the
// row existed, and a ledger founded whose founding act has not closed yet. A
// mark without its row is UNREADABLE: named, refused, repaired by no door.
//
// The mark's prefix is reserved: FinishWithResult refuses it, so a brain's
// close cannot reassign the owner; only FinishFounding and AdoptLedger write
// it, and both refuse without a sealer, because a mark whose receipt was never
// born would be lost in silence.

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/identity"
)

// LedgerStanding is what a ledger is to the profile a handle serves.
type LedgerStanding string

const (
	// LedgerStandingOK · the last mark names this profile.
	LedgerStandingOK LedgerStanding = "ok"
	// LedgerStandingLegacyUnfounded · no mark at all: a ledger from before
	// this version, or one founded whose founding act has not closed.
	LedgerStandingLegacyUnfounded LedgerStanding = "legacy_unfounded"
	// LedgerStandingForeignProfile · the last mark names another profile.
	LedgerStandingForeignProfile LedgerStanding = "ledger_foreign_profile"
)

// ProfileMarkPrefix opens the result_digest of a founding or adoption receipt.
const ProfileMarkPrefix = "profile:"

// AdoptionVerb is the operation name of the one act a foreign ledger admits.
const AdoptionVerb = "config.adopt-ledger"

var (
	// ErrLedgerForeignProfile refuses a new act on a ledger another profile owns.
	ErrLedgerForeignProfile = errors.New("action/sqlite: ledger_foreign_profile: this ledger was founded or adopted by another profile; adopt it before recording a new act")
	// ErrReservedResultDigest refuses a close whose result carries the profile
	// mark outside the two doors that may write it.
	ErrReservedResultDigest = errors.New("action/sqlite: the result digest prefix 'profile:' is reserved to the founding and adoption of the ledger")
	// ErrNoSealer refuses a founding or adoption close on a handle with no
	// receipt sealer: the mark lives in the receipt, and no receipt would be born.
	ErrNoSealer = errors.New("action/sqlite: no receipt sealer is wired, and the profile mark lives in the receipt")
	// ErrNoProfileIdentity reports a standing asked of a handle that was never
	// told which profile it serves.
	ErrNoProfileIdentity = errors.New("action/sqlite: this handle has no profile identity")
	// ErrProfileIdentityMalformed refuses an identity that is not the one
	// form app.ProfileIdentity yields — "sha256:" and 64 lower-case hex
	// digits — at the handle and at the two doors that write the mark, so
	// the ledger never carries a mark the reader would have to guess at.
	ErrProfileIdentityMalformed = errors.New("action/sqlite: the profile identity is not a canonical sha256 digest")
	// ErrProfileIdentityAlreadySet refuses a SECOND, different identity on a
	// handle: a handle serves one profile for its life, so no later call — in
	// the opener's body or in any callee that receives the handle — can
	// re-identify it and slip past the judgement made under the first.
	ErrProfileIdentityAlreadySet = errors.New("action/sqlite: this handle already serves another profile")
	// ErrLedgerMarkMalformed names the mark the reader judges — the LAST
	// SUCCEEDED receipt whose result carries the mark's prefix under LIKE's
	// case folding — when it is not exactly the prefix followed by a canonical
	// digest ("sha256:" and 64 lower-case hex digits): another spelling of the
	// prefix, no digest, or anything that is not a digest. The mark EXISTS and
	// cannot be read — corruption named, never absence and never «another
	// profile», so no door adopts it. Every act refuses on it, the owner's
	// too; `korvun ledger check` names the broken receipt, and no door
	// repairs it: its remedy is replacing the ledger's file
	// (docs/operations/ledger-restore.md), and editing it by hand would lift
	// it too. Earlier receipts, and receipts that did not succeed, are not
	// this reader's to judge: the chain's hash is.
	ErrLedgerMarkMalformed = errors.New("action/sqlite: ledger_mark_malformed: the ledger's profile mark is not in the spelling its doors write; judge the chain with `korvun ledger check`")
	// ErrNotAnAdoption refuses an adoption act that is not named as one.
	ErrNotAnAdoption = errors.New("action/sqlite: an adoption act must be named " + AdoptionVerb)
)

// profileIdentity is the per-handle identity of the profile it serves, or "",
// with the owner mark last read and the chain length it was read at: the owner
// can only change when a receipt lands, so a chain that has not grown keeps
// its mark, and a grown one is scanned again. Never a verdict cached blind.
type profileIdentity struct {
	mu     sync.RWMutex
	digest string
}

func (p *profileIdentity) get() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.digest
}

// setIdentity tells this handle which profile it serves — once: the same
// identity again is a no-op, a different one is refused by name, because a
// handle serves one profile for its life. Only the openers call it: a handle
// is born with its profile (OpenFor and its siblings) and never re-identified.
func (s *Store) setIdentity(digest string) error {
	if digest == "" {
		return ErrNoProfileIdentity
	}
	if !canonicalDigest.MatchString(digest) {
		return ErrProfileIdentityMalformed
	}
	s.profile.mu.Lock()
	defer s.profile.mu.Unlock()
	if s.profile.digest != "" && s.profile.digest != digest {
		return ErrProfileIdentityAlreadySet
	}
	s.profile.digest = digest
	return nil
}

// ProfileIdentity answers the identity this handle was told, or "".
func (s *Store) ProfileIdentity() string { return s.profile.get() }

// canonicalDigest is the one form a profile identity takes (app.ProfileIdentity):
// "sha256:" and 64 lower-case hex digits.
var canonicalDigest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// Standing judges the ledger for this handle's profile NOW — its shape
// (judgeShape), then its identity rows, never a cache — and names the owner
// ("" when the ledger has no row). A shape that is not this binary's current
// one is ErrLedgerUnreadable with its cause. Every error it returns carries
// its class (classifyLedgerError). Without a row: legacy when no receipt
// carries a mark, unreadable (ErrLedgerUnreadable) when a canonical mark
// exists in the receipts, and ErrLedgerMarkMalformed when the mark that
// exists is not canonical (the migration refused to turn it into a row);
// both errors answer the standing LedgerStandingUnreadable.
func (s *Store) Standing(ctx context.Context) (LedgerStanding, string, error) {
	me := s.profile.get()
	if me == "" {
		return "", "", ErrNoProfileIdentity
	}
	standing, owner, err := s.judgeIn(withJudgeOrigin(ctx, originPublicStanding, s.path), s.db, me)
	return standing, owner, classifyLedgerError(err)
}

// judgeIn reads the identity row through q — the pool, or the transaction of
// the write that must act on the row as it is at that moment.
// ledgerQuerier is what judgeIn reads through: one row for the identity,
// many for the shape — the pool or an open transaction.
type ledgerQuerier interface {
	rowQuerier
	rowsQuerier
}

func (s *Store) judgeIn(ctx context.Context, q ledgerQuerier, me string) (LedgerStanding, string, error) {
	// Handed the pool, the judgement reads through ONE connection obtained
	// first: a connection that cannot be born fails there — an error, never
	// a verdict — and never inside one of judgeOn's reads.
	if db, ok := q.(*sql.DB); ok {
		standing, owner := LedgerStandingUnreadable, ""
		var jerr error
		if err := withLedgerConnection(ctx, db, func(c *sql.Conn) error {
			standing, owner, jerr = s.judgeOn(ctx, c, me)
			return nil
		}); err != nil {
			return LedgerStandingUnreadable, "", err
		}
		return standing, owner, jerr
	}
	return s.judgeOn(ctx, q, me)
}

// judgeOn is judgeIn on a connection or transaction already held.
func (s *Store) judgeOn(ctx context.Context, q ledgerQuerier, me string) (LedgerStanding, string, error) {
	// The SHAPE first (judgeShape, train D): a file that is not a ledger of
	// this binary's schema is unreadable by name, whatever its row says.
	shape, err := judgeShape(ctx, dbShapeQuerier{q: q})
	if err != nil {
		return LedgerStandingUnreadable, "", err
	}
	if shape.shape != shapeCurrent {
		return LedgerStandingUnreadable, "", shape.unreadable()
	}
	rows, err := readIdentityRows(ctx, q)
	if err != nil {
		if readVerdict(siteOwnerDB, err) {
			return LedgerStandingUnreadable, "", fmt.Errorf("%w: the identity row cannot be read: %w", ErrLedgerUnreadable, err)
		}
		return LedgerStandingUnreadable, "", fmt.Errorf("action/sqlite: read the identity row: %w", err)
	}
	owner, found, verr := identityOwner(rows)
	switch {
	case verr != nil:
		return LedgerStandingUnreadable, "", fmt.Errorf("%w: %w", ErrLedgerUnreadable, verr)
	case !found:
		return s.judgeWithoutRow(ctx, q)
	case owner == me:
		return LedgerStandingOK, owner, nil
	default:
		return LedgerStandingForeignProfile, owner, nil
	}
}

// identityRow is one row of ledger_identity as stored: whether its id is 1
// (as SQLite compares it: `id = 1`, the driver judge's WHERE) and its owner
// cell, raw.
type identityRow struct {
	idIsOne any
	owner   any
}

// readIdentityRows reads AT MOST TWO rows of ledger_identity in ONE query,
// none filtered out, so the count and the owner are one reading of the same
// rows. Each stage of the read is a site of the judge's instrumentation.
func readIdentityRows(ctx context.Context, q rowsQuerier) ([]identityRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT CASE WHEN id = 1 THEN 1 ELSE 0 END, owner_digest FROM ledger_identity LIMIT 2`)
	if err = judgeRead(ctx, siteOwnerDB, stageQuery, err); err != nil {
		if rows != nil {
			_ = rows.Close()
		}
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []identityRow
	for rows.Next() {
		var r identityRow
		if err := judgeRead(ctx, siteOwnerDB, stageScan, rows.Scan(&r.idIsOne, &r.owner)); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, judgeRead(ctx, siteOwnerDB, stageNext, rows.Err())
}

// identityOwner judges the identity rows read: none is «no row» (legacy, or
// the mark decides); exactly one, whose id is 1 and whose owner is a
// canonical digest, is its owner; anything else — two rows, another id, a
// NULL or non-canonical owner — is named for the verdict, never read as «no
// row».
func identityOwner(rows []identityRow) (owner string, found bool, err error) {
	switch len(rows) {
	case 0:
		return "", false, nil
	case 1:
	default:
		return "", false, errors.New("ledger_identity holds more than one row")
	}
	r := rows[0]
	if id, ok := r.idIsOne.(int64); !ok || id != 1 {
		return "", false, errors.New("the identity row's id is not 1")
	}
	switch o := r.owner.(type) {
	case nil:
		return "", false, errors.New("ledger_identity.owner_digest is NULL")
	case string:
		owner = o
	case []byte:
		owner = string(o)
	default:
		return "", false, fmt.Errorf("ledger_identity.owner_digest holds a %T", r.owner)
	}
	if !canonicalDigest.MatchString(owner) {
		// The CHECK forbids this; a row that carries it anyway is a storage
		// the reader cannot trust.
		return "", false, errors.New("the identity row is not canonical")
	}
	return owner, true, nil
}

// judgeWithoutRow decides what a ledger with no identity row is: legacy when
// no receipt ever carried a mark; unreadable when one did (the row is
// missing), malformed when the mark that exists is not one the doors write.
func (s *Store) judgeWithoutRow(ctx context.Context, q rowsQuerier) (LedgerStanding, string, error) {
	mark, err := judgeMarkRow(ctx, q)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return LedgerStandingLegacyUnfounded, "", nil
	case err != nil:
		if readVerdict(siteMarkDB, err) {
			return LedgerStandingUnreadable, "", fmt.Errorf("%w: the profile mark cannot be read: %w", ErrLedgerUnreadable, err)
		}
		return LedgerStandingUnreadable, "", fmt.Errorf("action/sqlite: read the profile mark: %w", err)
	case strings.HasPrefix(mark, ProfileMarkPrefix) && canonicalDigest.MatchString(strings.TrimPrefix(mark, ProfileMarkPrefix)):
		return LedgerStandingUnreadable, "", fmt.Errorf("%w: the identity row is missing while a marked receipt exists", ErrLedgerUnreadable)
	default:
		return LedgerStandingUnreadable, "", ErrLedgerMarkMalformed
	}
}

// judgeMarkRow reads the last succeeded receipt's profile mark through q:
// sql.ErrNoRows when no receipt carries one. Each stage of the read is a site
// of the judge's instrumentation.
func judgeMarkRow(ctx context.Context, q rowsQuerier) (string, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT result_digest FROM receipts
		  WHERE partition = ? AND outcome = ? AND result_digest LIKE ?
		  ORDER BY chain_seq DESC LIMIT 1`,
		ledgerPartition, string(action.StateSucceeded), ProfileMarkPrefix+"%")
	if err = judgeRead(ctx, siteMarkDB, stageQuery, err); err != nil {
		if rows != nil {
			_ = rows.Close()
		}
		return "", err
	}
	defer func() { _ = rows.Close() }()
	more := rows.Next()
	if err := judgeRead(ctx, siteMarkDB, stageNext, rows.Err()); err != nil {
		return "", err
	}
	if !more {
		return "", sql.ErrNoRows
	}
	var mark string
	if err := judgeRead(ctx, siteMarkDB, stageScan, rows.Scan(&mark)); err != nil {
		return "", err
	}
	return mark, nil
}

// seedIdentityRowV15toV16 is the migration's copy: the last SUCCEEDED receipt
// that carries a canonical mark becomes the identity row; a mark that is not
// canonical becomes NO row, and the ledger opens ErrLedgerMarkMalformed
// (judgeWithoutRow names it) until restored. Ledgers with no mark get no row
// and are legacy.
func seedIdentityRowV15toV16(tx *sql.Tx) error {
	var mark, actionID string
	err := tx.QueryRow(
		`SELECT result_digest, action_id FROM receipts
		  WHERE partition = ? AND outcome = ? AND result_digest LIKE ?
		  ORDER BY chain_seq DESC LIMIT 1`,
		ledgerPartition, string(action.StateSucceeded), ProfileMarkPrefix+"%").Scan(&mark, &actionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("action/sqlite: read the receipt mark for the identity row: %w", err)
	}
	owner := strings.TrimPrefix(mark, ProfileMarkPrefix)
	if !strings.HasPrefix(mark, ProfileMarkPrefix) || !canonicalDigest.MatchString(owner) {
		return nil // named at open by judgeWithoutRow, never turned into a row
	}
	// A row already there (a ledger downgraded and re-lifted) is left as it
	// is: the seed never overwrites the state.
	_, err = tx.Exec(`INSERT INTO ledger_identity (id, owner_digest, founded_by_action, adopted_by_action, written_at)
		SELECT 1, ?, ?, NULL, ? WHERE NOT EXISTS (SELECT 1 FROM ledger_identity WHERE id = 1)`, owner, actionID, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("action/sqlite: seed the identity row: %w", err)
	}
	return nil
}

// FinishFounding closes the founding act SUCCEEDED with this ledger's mark
// naming digest — the ONE door, with AdoptLedger, that writes the mark. It
// refuses without a sealer: the mark lives in the receipt.
func (s *Store) FinishFounding(ctx context.Context, actionID, digest string) error {
	if s.sealer == nil {
		return ErrNoSealer
	}
	if digest == "" {
		return ErrNoProfileIdentity
	}
	if !canonicalDigest.MatchString(digest) {
		return ErrProfileIdentityMalformed
	}
	tx, err := s.beginWrite(ctx)
	if err != nil {
		return fmt.Errorf("action/sqlite: begin founding: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.finishWithResultTx(ctx, tx, actionID, action.StateSucceeded, time.Now().UTC(), ProfileMarkPrefix+digest); err != nil {
		return err
	}
	if s.seams.beforeIdentityRow != nil {
		if err := s.seams.beforeIdentityRow(); err != nil {
			return err
		}
	}
	// The row and the receipt are one transaction: a crash between them
	// leaves neither (R11).
	if _, err := s.txExec(ctx, tx, `INSERT INTO ledger_identity (id, owner_digest, founded_by_action, adopted_by_action, written_at)
		VALUES (1, ?, ?, NULL, ?)`, digest, actionID, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("action/sqlite: write the identity row: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("action/sqlite: commit founding %q: %w", actionID, err)
	}
	return nil
}

// AdoptLedger records the adoption act — named AdoptionVerb — and closes it
// SUCCEEDED with the mark naming digest, in ONE transaction: the act with its
// decision and evidence, the terminal close, the marked receipt. It is the one
// write a foreign ledger admits. Without a sealer it refuses; a failure
// anywhere leaves nothing.
func (s *Store) AdoptLedger(ctx context.Context, env action.Envelope, d Decision, evidence identity.Evidence, digest string) (string, error) {
	if s.sealer == nil {
		return "", ErrNoSealer
	}
	if digest == "" {
		return "", ErrNoProfileIdentity
	}
	if !canonicalDigest.MatchString(digest) {
		return "", ErrProfileIdentityMalformed
	}
	if env.Operation.Namespace != "config" || env.Operation.Name != AdoptionVerb {
		return "", fmt.Errorf("%w: got %s.%s", ErrNotAnAdoption, env.Operation.Namespace, env.Operation.Name)
	}
	// The adoption is the one act a FOREIGN ledger admits: its transaction
	// judges the row inside itself — a row that vanished or cannot be read
	// refuses by name; one that changed hands is followed (L6: the last
	// adopter owns) — and lifts the connection's guard for its own writes.
	tx, err := s.beginAdoption(ctx, digest)
	if err != nil {
		return "", err
	}
	// The lift of the guard is inside this transaction and rolls back with
	// it: an aborted adoption leaves the connection's guard saying the
	// standing it judged, and the trigger names it (no copy to re-sync).
	defer func() { _ = tx.Rollback() }()
	if err := s.recordAuthenticatedTx(ctx, tx, env, d, action.StateAuthorized, evidence); err != nil {
		return "", err
	}
	finishedAt := s.identityNow().UTC()
	if _, err := s.txExec(ctx, tx,
		`UPDATE actions SET state = ?, finished_at = ? WHERE action_id = ?`,
		string(action.StateSucceeded), finishedAt.Format(time.RFC3339Nano), env.ActionID); err != nil {
		return "", fmt.Errorf("action/sqlite: close adoption %q: %w", env.ActionID, err)
	}
	receipt, err := s.receiptForFinish(ctx, tx, env.ActionID, action.StateSucceeded, finishedAt, ProfileMarkPrefix+digest, false, true)
	if err != nil {
		return "", err
	}
	if err := s.appendReceiptTx(ctx, tx, receipt); err != nil {
		return "", err
	}
	// The row follows the receipt in the same transaction: an adoption is a
	// receipt AND a row, or nothing.
	if _, err := s.txExec(ctx, tx, `INSERT INTO ledger_identity (id, owner_digest, founded_by_action, adopted_by_action, written_at)
		VALUES (1, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET owner_digest = excluded.owner_digest, adopted_by_action = excluded.adopted_by_action, written_at = excluded.written_at`,
		digest, env.ActionID, env.ActionID, finishedAt.Format(time.RFC3339Nano)); err != nil {
		return "", fmt.Errorf("action/sqlite: write the identity row: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("action/sqlite: commit adoption %q: %w", env.ActionID, err)
	}
	s.noteWrite(ctx)
	return receipt.ReceiptID, nil
}

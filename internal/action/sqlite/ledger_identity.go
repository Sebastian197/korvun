// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The ledger's identity, redesigned (train B, 2026-09-24): the STATE of who
// owns the ledger lives in its own one-row table, ledger_identity, written
// only by the founding and the adoption in the same transaction as their
// receipt, read with one query and never cached; the BLOCK of every act on a
// ledger this handle's profile does not own lives in SQLite itself, as
// temporary triggers installed on every connection the handle uses; and no
// handle exists without the profile it serves. Train C (2026-09-25): the
// hook judges a new connection by enumerating the benign shapes.

package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	msqlite "modernc.org/sqlite"
)

var (
	// ErrLedgerUnreadable names a ledger the store judges unreadable, a
	// verdict: its shape is not this binary's current one (judgeShape), its
	// identity row is missing while a marked receipt exists, or a read of
	// either failed with a structural code — never a busy ledger or a failing
	// environment, which have their own names. Every act refuses on it, the
	// adoption too; no door repairs it: its remedy is replacing the ledger's
	// file (docs/operations/ledger-restore.md), and a row written by hand
	// would lift it too (the row is state, not evidence, and that limit is
	// declared).
	ErrLedgerUnreadable = errors.New("action/sqlite: ledger_unreadable: the ledger's identity row cannot be read")
	// ErrLedgerGuardUnset names a connection whose guard row is missing: the
	// block fails CLOSED, never open.
	ErrLedgerGuardUnset = errors.New("action/sqlite: ledger_guard_unset: this connection's guard is missing")
	// ErrLedgerBusy names a ledger another connection held: a write that
	// waited the busy timeout out behind another writer's immediate
	// transaction, a judgement of the ledger whose read met SQLITE_BUSY or
	// SQLITE_LOCKED, or a connection that could not be born while another held
	// the file; nothing was written. It is never a verdict. Its text speaks of
	// the busy timeout, and that is not always so: two opens born together on
	// a file that does not exist yet can meet it at once, without waiting (a
	// known limit of v0.16.2; the text is train H's).
	ErrLedgerBusy = errors.New("action/sqlite: ledger_busy: another writer held the ledger past the busy timeout")

	// ErrNoActionStore names an existing file with no action store in it —
	// the shared file before its first boot, for the doors that never seed.
	ErrNoActionStore = errors.New("action/sqlite: no action store in the file")

	// ErrSchemaBehind names a ledger older than this binary's schema, for
	// the doors that never migrate (the operator's and the reader's).
	ErrSchemaBehind = errors.New("action/sqlite: the ledger's schema is behind this binary")

	// ErrGuardHandleUnknown names a connection whose DSN carries a guard
	// nonce the registry does not know: a handle closed before its
	// connection was born. The hook refuses the connection with it.
	ErrGuardHandleUnknown = errors.New("action/sqlite: connection guard: unknown handle")
)

// LedgerStandingUnreadable is the standing of a ledger whose identity cannot
// be read (ErrLedgerUnreadable, ErrLedgerMarkMalformed).
const LedgerStandingUnreadable LedgerStanding = "ledger_unreadable"

// OpenFor opens the store for the profile whose canonical identity digest is
// identity: the boot's door — migrations, then the retention prune when the
// ledger is this profile's (a ledger another profile owns, or one whose
// identity cannot be read, gets no maintenance from this handle: R10). The
// handle is born with its profile and never re-identified. Every error it
// returns carries its class (classifyLedgerError).
func OpenFor(path, identity string) (*Store, error) {
	store, err := openFor(path, identity)
	return store, classifyLedgerError(err)
}

func openFor(path, identity string) (*Store, error) {
	if err := checkIdentity(identity); err != nil {
		return nil, err
	}
	store, err := openWithIdentity(path, identity)
	if err != nil {
		return nil, err
	}
	if err := store.setIdentity(identity); err != nil {
		_ = store.Close()
		return nil, err
	}
	if err := store.installGuard(); err != nil {
		_ = store.Close()
		return nil, err
	}
	// Prune only what is this profile's: a ledger another profile owns, or
	// one whose identity cannot be read, gets no maintenance (R10).
	if seam := openStandingSeam.Load(); seam != nil {
		(*seam)()
	}
	standing, _, err := store.Standing(withJudgeOrigin(context.Background(), originOpenForStanding, store.path))
	if err != nil && (!isVerdict(err) || isBirthFailure(err)) {
		// A failure to judge is the open's error; only a verdict (foreign,
		// unreadable) is a reason to open without maintenance. A connection
		// that could not be born is never a verdict, even when Standing names
		// its structural code unreadable.
		_ = store.Close()
		return nil, err
	}
	if err == nil && (standing == LedgerStandingOK || standing == LedgerStandingLegacyUnfounded) {
		if seam := openPruneSeam.Load(); seam != nil {
			(*seam)()
		}
		// The prune judges inside its own transaction and decides: a ledger
		// that changed hands between the judgement above and the prune is
		// «no prune», never a failed boot (D08).
		if _, err := store.Prune(context.Background()); err != nil && !errors.Is(err, ErrLedgerForeignProfile) && !errors.Is(err, ErrLedgerUnreadable) {
			_ = store.Close()
			return nil, err
		}
	}
	return store, nil
}

// OpenOperatorFor opens the store for identity through the operator's door
// (no recovery, no prune, never a migration of an existing store). Every
// error it returns carries its class (classifyLedgerError).
func OpenOperatorFor(path, identity string) (*Store, error) {
	store, err := openOperatorFor(path, identity)
	return store, classifyLedgerError(err)
}

func openOperatorFor(path, identity string) (*Store, error) {
	if err := checkIdentity(identity); err != nil {
		return nil, err
	}
	store, err := openOperatorWithIdentity(path, identity)
	if err != nil {
		return nil, err
	}
	if err := store.setIdentity(identity); err != nil {
		_ = store.Close()
		return nil, err
	}
	if err := store.installGuard(); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

// OpenReadOnlyFor opens the store read-only for identity, which the readers
// use only to NAME the standing; a read-only handle blocks nothing and
// installs no guard. Every error it returns carries its class
// (classifyLedgerError).
func OpenReadOnlyFor(path, identity string) (*Store, error) {
	store, err := openReadOnlyFor(path, identity)
	return store, classifyLedgerError(err)
}

func openReadOnlyFor(path, identity string) (*Store, error) {
	if err := checkIdentity(identity); err != nil {
		return nil, err
	}
	store, err := openReadOnly(path)
	if err != nil {
		return nil, err
	}
	if err := store.setIdentity(identity); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

func checkIdentity(identity string) error {
	if identity == "" {
		return ErrNoProfileIdentity
	}
	if !canonicalDigest.MatchString(identity) {
		return ErrProfileIdentityMalformed
	}
	return nil
}

// beginWrite begins a write transaction — IMMEDIATE, through the handle's
// DSN — judges the identity row INSIDE it (R16: nothing another connection
// commits can slip between the judgement and the write), refuses a ledger
// this profile does not own or cannot read by name, and refreshes the
// connection's guard. A busy ledger is named, never leaked as driver text.
func (s *Store) beginWrite(ctx context.Context) (*sql.Tx, error) {
	ctx = withJudgeOrigin(ctx, originBeginWrite, s.path)
	tx, err := s.db.BeginTx(ctx, nil)
	if err = stageFault(s.path, "write:begin", err); err != nil {
		if tx != nil {
			_ = tx.Rollback()
		}
		return nil, s.mapGuardError(err)
	}
	me := s.profile.get()
	if me == "" {
		return tx, nil
	}
	standing, owner, err := s.judgeIn(ctx, tx, me)
	if err != nil {
		_ = tx.Rollback()
		if isVerdict(err) {
			_ = s.setGuardIn(ctx, s.db, string(LedgerStandingUnreadable))
		}
		return nil, classifyLedgerError(err)
	}
	if standing == LedgerStandingForeignProfile {
		// The refusal is named here; the guard row follows, so a door that
		// bypasses this judgement dies on the last one (R17).
		_ = tx.Rollback()
		_ = s.setGuardIn(ctx, s.db, string(standing))
		return nil, fmt.Errorf("%w (owner %s, this profile %s)", ErrLedgerForeignProfile, owner, me)
	}
	// The lift never overwrites an unreadable guard (setGuardRowIn's WHERE):
	// on such a connection the write below dies in the trigger by name.
	if err := s.setGuardIn(ctx, tx, "ok"); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}

// beginAdoption is beginWrite for the adoption: a foreign or legacy ledger is
// admitted, an unreadable one refuses, and the guard is lifted for the
// adoption's own writes inside its transaction.
func (s *Store) beginAdoption(ctx context.Context, me string) (*sql.Tx, error) {
	ctx = withJudgeOrigin(ctx, originBeginAdoption, s.path)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, s.mapGuardError(err)
	}
	// judgeIn answers an unreadable ledger as an ERROR (ErrLedgerUnreadable
	// or ErrLedgerMarkMalformed): that error is the refusal, and the guard
	// row follows it, so a door that bypassed this judgement dies on it too.
	// Foreign and legacy are admitted: adoption is what they are for.
	if _, _, err := s.judgeIn(ctx, tx, me); err != nil {
		_ = tx.Rollback()
		if isVerdict(err) {
			_ = s.setGuardIn(ctx, s.db, string(LedgerStandingUnreadable))
		}
		return nil, classifyLedgerError(err)
	}
	// The lift is inside the transaction: it rolls back with it, so an
	// aborted adoption leaves the guard row saying the standing it judged.
	if _, err := s.setGuardRowIn(ctx, tx, "ok"); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}

// txExec runs a statement inside a write transaction and names the guard's
// refusal (R1): every door writes through it, so a write the guard kills
// comes back as the sentinel of this connection's standing, not as driver
// text. There is no pool-level twin: since train C every production write,
// the prune included, runs inside beginWrite's transaction.
func (s *Store) txExec(ctx context.Context, tx *sql.Tx, query string, args ...any) (sql.Result, error) {
	res, err := tx.ExecContext(ctx, query, args...)
	return res, s.mapGuardError(err)
}

// execer is what setGuardIn needs: the pool or an open transaction.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// isVerdict tells a judgement of the ledger (unreadable, malformed mark)
// from a failure of the moment (a query that could not run): only a verdict
// sets the connection's guard; a failure refuses the one write and leaves
// the next judgement to its own query.
func isVerdict(err error) bool {
	return errors.Is(err, ErrLedgerUnreadable) || errors.Is(err, ErrLedgerMarkMalformed)
}

// setGuardIn writes the connection's guard row through q; a guard row that is
// missing is never re-created (R15: the block fails closed, and the triggers
// name it ledger_guard_unset).
func (s *Store) setGuardIn(ctx context.Context, q execer, standing string) error {
	_, err := s.setGuardRowIn(ctx, q, standing)
	return err
}

// setGuardRowIn writes the guard row and reports how many rows it touched.
func (s *Store) setGuardRowIn(ctx context.Context, q execer, standing string) (int64, error) {
	if s.guard.nonce == "" {
		return 1, nil
	}
	// An unreadable guard is sticky: nothing but a new connection's hook
	// re-judges it (the internal pass of train C: a first line that found the
	// row ok used to lift the hook's verdict on a ledger without its receipts
	// table).
	res, err := q.ExecContext(ctx, `UPDATE temp.profile_guard SET standing = ? WHERE standing <> ?`, standing, string(LedgerStandingUnreadable))
	if err != nil {
		return 0, fmt.Errorf("action/sqlite: refresh the connection guard: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// refreshGuard judges the ledger on the handle's connection and sets the
// guard accordingly; installGuard calls it once the schema is there and the
// handle knows its profile. Legacy blocks nothing.
func (s *Store) refreshGuard(ctx context.Context) error {
	me := s.profile.get()
	if me == "" || s.guard.nonce == "" {
		return nil
	}
	standing, _, err := s.judgeIn(ctx, s.db, me)
	if err != nil {
		// A verdict sets the guard; a failure of the moment is returned
		// as it is, never turned into a sticky verdict (the internal pass
		// of train D: the third door of the verdict).
		if !isVerdict(err) {
			return err
		}
		standing = LedgerStandingUnreadable
	}
	if standing == LedgerStandingLegacyUnfounded {
		standing = LedgerStandingOK
	}
	return stageFault(s.path, "install:guard-update", s.setGuardIn(ctx, s.db, string(standing)))
}

// installGuard is what the openers call once the handle knows its profile:
// the guard itself was installed by the connection hook at connection time,
// open for the package's own open sequence; this sets it to the judged
// standing.
func (s *Store) installGuard() error {
	if s.guard.nonce == "" {
		return nil
	}
	// The schema exists now (or the shape is bad and stays as it is): every
	// trigger the hook could not create is created here, on the handle's
	// connection, for the guarded tables the file HAS.
	ctx := withJudgeOrigin(context.Background(), originRefresh, s.path)
	present, err := dbShapeQuerier{q: s.db}.strings(ctx, siteNone, `SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err = stageFault(s.path, "install:catalog", err); err != nil {
		return fmt.Errorf("action/sqlite: connection guard: read the catalog: %w", err)
	}
	for i, q := range guardTriggerStatementsFor(present) {
		_, err := s.db.ExecContext(ctx, q)
		if err = stageFault(s.path, fmt.Sprintf("install:trigger#%d", i), err); err != nil {
			return fmt.Errorf("action/sqlite: connection guard: %w", err)
		}
	}
	return s.refreshGuard(ctx)
}

// refuseMaintenance is what recovery runs first: with an identity, only a
// ledger this profile owns (or a legacy one) gets its maintenance. The prune
// judges inside its own write transaction instead (beginWrite).
func (s *Store) refuseMaintenance(ctx context.Context) error {
	me := s.profile.get()
	if me == "" {
		return nil
	}
	ctx = withJudgeOrigin(ctx, originRefuseMaintenance, s.path)
	standing, owner, err := s.judgeIn(ctx, s.db, me)
	if err != nil {
		return classifyLedgerError(err)
	}
	if standing == LedgerStandingForeignProfile {
		return fmt.Errorf("%w (owner %s, this profile %s): no maintenance on a ledger this profile does not own", ErrLedgerForeignProfile, owner, me)
	}
	return nil
}

// mapGuardError translates the storage's errors into the store's names: a
// busy ledger (SQLITE_BUSY, SQLITE_LOCKED, their extended codes) is
// ErrLedgerBusy; the guard's constraint (1811 with the message
// ledger_guard:<standing>) is the sentinel of the standing the trigger itself
// evaluated and named; anything else passes through untouched.
func (s *Store) mapGuardError(err error) error {
	if err == nil {
		return nil
	}
	var coded interface{ Code() int }
	if !errors.As(err, &coded) {
		return err
	}
	code := coded.Code()
	switch code & 0xff { // the primary code: the extended BUSY/LOCKED variants fold into it
	case 5, 6: // SQLITE_BUSY, SQLITE_LOCKED
		return fmt.Errorf("%w: %v", ErrLedgerBusy, err)
	}
	switch code {
	case 1811: // SQLITE_CONSTRAINT_TRIGGER
		text := err.Error()
		if !strings.Contains(text, guardMessage+":") {
			return err
		}
		// The trigger that fired names the standing it evaluated.
		switch {
		case strings.Contains(text, guardMessage+":"+string(LedgerStandingForeignProfile)):
			return fmt.Errorf("%w (this connection's guard)", ErrLedgerForeignProfile)
		case strings.Contains(text, guardMessage+":"+string(LedgerStandingUnreadable)):
			return fmt.Errorf("%w (this connection's guard)", ErrLedgerUnreadable)
		default:
			return fmt.Errorf("%w: %v", ErrLedgerGuardUnset, err)
		}
	}
	return err
}

// guardState is what the store keeps of its connection guard: the nonce the
// hook recognises. The standing itself lives ONLY in the connection's guard
// row, which the triggers evaluate and name; the store keeps no copy of it.
type guardState struct {
	nonce string
}

// guardMessage is the prefix of the message the guard's triggers raise: the
// trigger that fires appends the standing it evaluated
// (`ledger_guard:<standing>`), and mapGuardError translates that name to the
// sentinel without reading or keeping a copy of the standing.
const guardMessage = "ledger_guard"

// guardParam is the DSN parameter that selects the connection hook.
const guardParam = "_korvun_guard"

// guardedTables are the tables the guard covers: every table an act, an
// approval, an intent, its versions or events, a grant or an execution
// binding is inserted into, the two whose lifecycle is an UPDATE, and the
// three identity tables (R19).
var guardedTables = []struct {
	table string
	event string
}{
	{"actions", "INSERT"}, {"approvals", "INSERT"}, {"intents", "INSERT"}, {"intent_versions", "INSERT"},
	{"intent_events", "INSERT"}, {"grants", "INSERT"}, {"execution_bindings", "INSERT"},
	{"intents", "UPDATE"}, {"grants", "UPDATE"},
	{"principals", "INSERT"}, {"principal_events", "INSERT"}, {"principal_bindings", "INSERT"},
}

// guardRegistry maps a handle's nonce to its identity while the handle lives,
// so the connection hook — which sees only the DSN — knows the nonce is ours.
var guardRegistry sync.Map

var guardHookOnce sync.Once
var guardNonces atomic.Uint64

// registerGuard mints a nonce for identity and registers the connection hook
// once; "" for a handle without a profile, which gets no guard.
func registerGuard(identity string) string {
	if identity == "" {
		return ""
	}
	guardHookOnce.Do(func() { msqlite.RegisterConnectionHook(guardHook) })
	nonce := fmt.Sprintf("g%d-%d", os.Getpid(), guardNonces.Add(1))
	guardRegistry.Store(nonce, identity)
	return nonce
}

func forgetGuard(nonce string) {
	if nonce != "" {
		guardRegistry.Delete(nonce)
	}
}

// guardedDSN appends the guard's nonce to a DSN.
func guardedDSN(dsn, nonce string) string {
	if nonce == "" {
		return dsn
	}
	return dsn + "&" + guardParam + "=" + nonce
}

// guardHook is the driver's connection hook: on every connection whose DSN
// names a registered handle it judges the connection on itself
// (judgeOnConn), installs the temp guard table with that standing, and
// creates the guard's triggers on every guarded table the file has. A failed
// read that is a verdict (a structural code) is judged, like the shape: the
// guard says unreadable. Any other failure of the judgement, and a failure of
// the hook's own statements after it, refuses the connection with its error
// (the pool births another on the next call); a nonce the registry does not
// know refuses it with ErrGuardHandleUnknown; a connection with no nonce is
// not the guard's business.
func guardHook(conn msqlite.ExecQuerierContext, dsn string) error {
	u, err := url.Parse(dsn)
	if err != nil {
		return nil
	}
	nonce := u.Query().Get(guardParam)
	if nonce == "" {
		return nil
	}
	identity, ok := guardRegistry.Load(nonce)
	if !ok {
		return fmt.Errorf("%w: %q", ErrGuardHandleUnknown, nonce)
	}
	path := dsnPath(dsn)
	ctx := withJudgeOrigin(context.Background(), originHookAtBirth, path)
	if fault := hookShapeFault.Load(); fault != nil {
		if err := (*fault)(); err != nil {
			return fmt.Errorf("action/sqlite: connection guard: judge: %w", err)
		}
	}
	standing, err := judgeOnConn(ctx, conn, identity.(string))
	if err != nil {
		return fmt.Errorf("action/sqlite: connection guard: judge: %w", err)
	}
	noteHookBirth(path, standing)
	for _, st := range []struct{ stage, q string }{
		{"hook:temp-create", `CREATE TEMP TABLE IF NOT EXISTS profile_guard (standing TEXT NOT NULL)`},
		{"hook:temp-delete", `DELETE FROM temp.profile_guard`},
		{"hook:temp-insert", fmt.Sprintf(`INSERT INTO temp.profile_guard (standing) VALUES ('%s')`, standing)},
	} {
		_, err := conn.ExecContext(ctx, st.q, nil)
		if err = stageFault(path, st.stage, err); err != nil {
			return fmt.Errorf("action/sqlite: connection guard: %w", err)
		}
	}
	// The triggers go on the guarded tables the file HAS: on a fresh file,
	// or one older than a table, that table does not exist yet and nothing
	// can be inserted into it either; installGuard creates the rest once the
	// schema is there. No error text is read.
	present, err := driverShapeQuerier{conn: conn}.strings(ctx, siteNone, `SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err = stageFault(path, "hook:catalog", err); err != nil {
		return fmt.Errorf("action/sqlite: connection guard: read the catalog: %w", err)
	}
	for i, q := range guardTriggerStatementsFor(present) {
		_, err := conn.ExecContext(ctx, q, nil)
		if err = stageFault(path, fmt.Sprintf("hook:trigger#%d", i), err); err != nil {
			return fmt.Errorf("action/sqlite: connection guard: %w", err)
		}
	}
	return nil
}

// guardTriggerStatementsFor is the guard's triggers for the guarded tables
// among present (a name list from sqlite_master): three per table event,
// idempotent on a connection — one per blocking standing and one for a
// missing or unknown guard — each raising `ledger_guard:<standing>`.
func guardTriggerStatementsFor(present []string) []string {
	has := map[string]bool{}
	for _, n := range present {
		has[strings.ToLower(n)] = true // SQLite resolves identifiers without regard to case
	}
	stmts := make([]string, 0, len(guardedTables)*3)
	for _, g := range guardedTables {
		if !has[g.table] {
			continue
		}
		for _, standing := range []struct{ when, name string }{
			{`(SELECT standing FROM temp.profile_guard) = 'ledger_foreign_profile'`, string(LedgerStandingForeignProfile)},
			{`(SELECT standing FROM temp.profile_guard) = 'ledger_unreadable'`, string(LedgerStandingUnreadable)},
			{`COALESCE((SELECT standing FROM temp.profile_guard), 'unset') NOT IN ('ok', 'ledger_foreign_profile', 'ledger_unreadable')`, guardUnsetName},
		} {
			stmts = append(stmts, fmt.Sprintf(`CREATE TEMP TRIGGER IF NOT EXISTS korvun_guard_%s_%s_%s BEFORE %s ON main.%s
			  WHEN %s
			  BEGIN SELECT RAISE(ABORT, '%s'); END;`, g.table, strings.ToLower(g.event), strings.TrimPrefix(standing.name, "ledger_"), g.event, g.table, standing.when, guardMessage+":"+standing.name))
		}
	}
	return stmts
}

// guardUnsetName is what a missing or unknown guard row raises.
const guardUnsetName = "ledger_guard_unset"

// judgeOnConn is the hook's judgement of a NEW connection, on itself, before
// it serves anyone (R14): the SHAPE first (judgeShape: fresh and older open
// ok, bad and newer open unreadable), then, on a shape that has the identity
// row, the row itself — a canonical row of this identity opens ok, of another
// foreign, a non-canonical one unreadable; an empty identity table opens ok
// without a profile mark in the receipts and unreadable with one; two rows
// or a row with another id, unreadable. A read that fails with a structural
// code opens unreadable too; any other failed read is returned as an error,
// never judged: the hook refuses the connection and the pool births another
// on the next call. An unreadable verdict is STICKY for the connection
// (setGuardRowIn never overwrites it; only a new connection's hook
// re-judges).
func judgeOnConn(ctx context.Context, conn msqlite.ExecQuerierContext, identity string) (string, error) {
	unreadable := string(LedgerStandingUnreadable)
	v, err := judgeShape(ctx, driverShapeQuerier{conn: conn})
	if err != nil {
		return "", err
	}
	switch v.shape {
	case shapeFresh, shapeOlder:
		return "ok", nil // the open sequence seeds or migrates next; installGuard judges again
	case shapeCurrent:
	default:
		return unreadable, nil
	}
	rows, ok, err := driverScalar(ctx, conn, siteCountDriver, `SELECT COUNT(*) FROM ledger_identity`)
	if err != nil {
		if readVerdict(siteCountDriver, err) {
			return unreadable, nil
		}
		return "", err
	}
	if !ok {
		return unreadable, nil
	}
	switch rows {
	case "1":
		owner, found, err := driverScalar(ctx, conn, siteOwnerDriver, `SELECT owner_digest FROM ledger_identity WHERE id = 1`)
		if err != nil {
			if readVerdict(siteOwnerDriver, err) {
				return unreadable, nil
			}
			return "", err
		}
		if !found || !canonicalDigest.MatchString(owner) {
			return unreadable, nil
		}
		if owner == identity {
			return "ok", nil
		}
		return string(LedgerStandingForeignProfile), nil
	case "0":
		_, marked, err := driverScalar(ctx, conn, siteMarkDriver, fmt.Sprintf(
			`SELECT result_digest FROM receipts WHERE partition = '%s' AND outcome = '%s' AND result_digest LIKE '%s%%' ORDER BY chain_seq DESC LIMIT 1`,
			ledgerPartition, string(action.StateSucceeded), ProfileMarkPrefix))
		if err != nil {
			if readVerdict(siteMarkDriver, err) {
				return unreadable, nil
			}
			return "", err
		}
		if marked {
			return unreadable, nil
		}
		return "ok", nil
	default:
		return unreadable, nil
	}
}

// driverScalar reads one text value through a raw driver connection; site
// names the read for the judge's instrumentation.
func driverScalar(ctx context.Context, conn msqlite.ExecQuerierContext, site judgeQuerySite, query string) (string, bool, error) {
	rows, err := conn.QueryContext(ctx, query, nil)
	if err = judgeRead(ctx, site, stageQuery, err); err != nil {
		if rows != nil {
			_ = rows.Close()
		}
		return "", false, err
	}
	defer func() { _ = rows.Close() }()
	dest := make([]driver.Value, len(rows.Columns()))
	if err := judgeRead(ctx, site, stageNext, rows.Next(dest)); err != nil {
		if errors.Is(err, io.EOF) {
			return "", false, nil
		}
		return "", false, err
	}
	switch v := dest[0].(type) {
	case string:
		return v, true, nil
	case []byte:
		return string(v), true, nil
	default:
		return fmt.Sprint(v), true, nil
	}
}

// readOwner reads the identity row for the openers, through ONE connection
// obtained first: found and canonical, or not found (legacy: no row yet).
// The row set is judged like judgeIn judges it (identityOwner): two rows, a
// row whose id is not 1, a NULL or non-canonical owner, and a read that fails
// with a structural code are ErrLedgerUnreadable — a verdict only a
// current-shape opener may absorb; a connection that cannot be born and any
// other failed read are errors of the open. No error text is read.
func readOwner(ctx context.Context, db *sql.DB) (owner string, found bool, err error) {
	var rows []identityRow
	if err := withLedgerConnection(ctx, db, func(c *sql.Conn) error {
		var rerr error
		rows, rerr = readIdentityRows(ctx, c)
		return rerr
	}); err != nil {
		if !isBirthFailure(err) && readVerdict(siteOwnerDB, err) {
			return "", false, fmt.Errorf("%w: the identity row cannot be read: %w", ErrLedgerUnreadable, err)
		}
		return "", false, fmt.Errorf("action/sqlite: read the identity row: %w", err)
	}
	owner, found, verr := identityOwner(rows)
	if verr != nil {
		return "", false, fmt.Errorf("%w: %w", ErrLedgerUnreadable, verr)
	}
	return owner, found, nil
}

// requireCurrentSchema is what the writer openers ask of a ledger this
// profile does NOT own: it is opened at this binary's version or not at
// all — a foreign ledger is never migrated by this profile, and one from a
// newer binary is named ErrSchemaFromTheFuture.
func requireCurrentSchema(db *sql.DB, abs string) error {
	var stored any
	if err := db.QueryRow(`SELECT version FROM action_schema`).Scan(&stored); err != nil {
		return fmt.Errorf("action/sqlite: read schema version of %q: %w", abs, err)
	}
	version, err := parseStoredVersion(stored)
	if err != nil {
		return fmt.Errorf("%w: %w (%s)", ErrLedgerUnreadable, err, abs)
	}
	if version > schemaVersionCurrent {
		return fmt.Errorf("%w: store %q is at schema v%d, this binary writes v%d — a ledger another profile owns is not migrated by this profile", ErrSchemaFromTheFuture, abs, version, schemaVersionCurrent)
	}
	if version != schemaVersionCurrent {
		return fmt.Errorf("action/sqlite: store %q is at schema v%d, this binary writes v%d — a ledger another profile owns is not migrated by this profile", abs, version, schemaVersionCurrent)
	}
	return nil
}

// beforeIdentityRow is the founding's crash seam (R11): a test sets it to
// interrupt the founding transaction between the receipt and the row.
type identitySeams struct {
	beforeIdentityRow func() error
	beforePruneDelete func()
}

// openPruneSeam runs between OpenFor's judgement of the standing and its
// prune (train D, D08); only the moulds set it.
var openPruneSeam atomic.Pointer[func()]

// poolLifetimeForTest, when set, is the ConnMaxLifetime a writer opener
// gives its pool (train D, D20: a mould that needs the pool to birth a new
// connection between two steps of the open); production never sets it and
// the pool keeps its connection for the handle's life.
var poolLifetimeForTest atomic.Pointer[time.Duration]

// openStandingSeam runs just before OpenFor judges the standing (train D,
// D20: a failure of that judgement is the open's error, never swallowed);
// only the moulds set it.
var openStandingSeam atomic.Pointer[func()]

// hookShapeFault, when set, is a failure the connection hook returns BEFORE
// it judges a new connection (train D, D07): it never reaches a judge read,
// so it proves the refusal of a failure that is not a verdict, not the
// judge's decision (train E's judgeReadFaultSeam does that); only the moulds
// set it.
var hookShapeFault atomic.Pointer[func() error]

// schemaTablesV16 is every table a fresh store of the current schema
// creates, captured by execution on 2026-09-25 (train D, D02 ties it to a
// fresh OpenFor): the shape of a ledger is judged against this list.
var schemaTablesV16 = []string{
	"action_decisions", "action_schema", "actions",
	"approval_birth_events", "approval_birth_heads", "approval_tombstones", "approvals",
	"authority_write_lock", "authorization_snapshots", "authorization_starts",
	"budget_accounts", "budget_counters", "budget_debits", "budget_spent",
	"config_authority_heads", "config_authority_snapshots",
	"evidence", "execution_bindings",
	"grant_events", "grant_heads", "grant_versions", "grants",
	"identity_evidence_v2",
	"intent_events", "intent_heads", "intent_versions", "intents",
	"ledger_identity", "legacy_authority_imports",
	"principal_bindings", "principal_events", "principals",
	"receipts", "signing_keys",
}

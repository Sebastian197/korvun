// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Train E, batch 1 (GE2, GE3, GE4): the harness of the moulds TE13–TE32,
// TE63–TE65 and TE67 (plan v3, §§6–7 and §13).
//
// A judge fault is a SYNTHETIC coded error (codedFault: the driver's own
// shape, Code() int) or an uncoded one, put by judgeReadFaultSeam on the
// RESULT of a read that really ran, at one file, origin, site and stage; the
// production decision it met is read back from the seam (entry, exactly one
// hit, the site's decision) after the caller has returned. Every subcase runs
// on its OWN copy of a cold template file.

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/identity"
)

// codedFault is a synthetic storage failure carrying a SQLite result code,
// in the shape of the driver's *sqlite.Error: errors.As finds its Code()
// where it finds the driver's.
type codedFault struct {
	code int
	tag  string
}

func (f *codedFault) Error() string {
	return fmt.Sprintf("injected coded fault %d (%s)", f.code, f.tag)
}
func (f *codedFault) Code() int { return f.code }

// e1Fixture is the cold templates a batch-1 mould copies: a ledger founded by
// profileA and a current ledger with no identity row and no mark (legacy),
// each registered and inked (its principals and signing key in the file),
// with the ink and the evidence a second profile's adoption seals with.
type e1Fixture struct {
	founded, legacy                       string
	foundedInk, legacyInk                 *Store
	foundedEvidenceFor, legacyEvidenceFor func(id string) (action.Envelope, identity.Evidence)
}

// ink and evidenceFor are the fixture's own, for the template named.
func (fx e1Fixture) ink(fixture string) *Store {
	if fixture == "legacy" {
		return fx.legacyInk
	}
	return fx.foundedInk
}

func (fx e1Fixture) evidenceFor(fixture, id string) (action.Envelope, identity.Evidence) {
	if fixture == "legacy" {
		return fx.legacyEvidenceFor(id)
	}
	return fx.foundedEvidenceFor(id)
}

// newE1Fixture builds both templates once per mould.
func newE1Fixture(t *testing.T) e1Fixture {
	t.Helper()
	store, foundedEvidenceFor := foundedFor(t, profileA)
	founded := filepath.Join(t.TempDir(), "founded-template.db")
	coldCopy(t, store.path, founded)

	// The legacy template is the identity fixture never founded: registered
	// and inked like the founded one, with no identity row and no mark.
	lstore, resolver, issuer, _, now := identityStoreFixture(t)
	probe, err := OpenReadOnlyFor(lstore.path, profileA)
	if err != nil {
		t.Fatalf("read the legacy template: %v", err)
	}
	if st, _, err := probe.Standing(context.Background()); err != nil || st != LedgerStandingLegacyUnfounded {
		t.Fatalf("the legacy template stands %q %v, want legacy_unfounded", st, err)
	}
	_ = probe.Close()
	legacy := filepath.Join(t.TempDir(), "legacy-template.db")
	coldCopy(t, lstore.path, legacy)
	return e1Fixture{
		founded: founded, legacy: legacy,
		foundedInk: store, legacyInk: lstore,
		foundedEvidenceFor: foundedEvidenceFor,
		legacyEvidenceFor: func(id string) (action.Envelope, identity.Evidence) {
			return identityAttempt(t, resolver, issuer, id, now)
		},
	}
}

// coldCopy checkpoints src through a raw connection (every committed frame
// into the main file, the WAL truncated) and copies the main file to dst.
func coldCopy(t *testing.T, src, dst string) {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+src+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	var busy, logFrames, done int
	if err := raw.QueryRow(`PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &logFrames, &done); err != nil {
		t.Fatalf("checkpoint %s: %v", src, err)
	}
	_ = raw.Close()
	if busy != 0 || logFrames != done {
		t.Fatalf("checkpoint %s incomplete: busy=%d log=%d checkpointed=%d", src, busy, logFrames, done)
	}
	copyFile(t, src, dst)
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src) //nolint:gosec // G304: a test's own temp file
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst) //nolint:gosec // G304: a test's own temp file
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

// ledgerCopy is a fresh copy of a template, in its own directory.
func ledgerCopy(t *testing.T, template string) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "korvun.db")
	copyFile(t, template, dst)
	return dst
}

// armJudgeFault arms the one judge fault a subcase injects.
func armJudgeFault(t *testing.T, path string, origin judgeOrigin, site judgeQuerySite, stage readStage, fault error) *judgeReadFault {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	f := &judgeReadFault{path: abs, origin: origin, site: site, stage: stage, fault: fault}
	if !judgeReadFaultSeam.CompareAndSwap(nil, f) {
		t.Fatal("another judge fault is armed: the moulds of this batch are sequential")
	}
	t.Cleanup(func() { judgeReadFaultSeam.CompareAndSwap(f, nil) })
	return f
}

// armStageFault arms the one stage fault a subcase injects.
func armStageFault(t *testing.T, path, stage string, fault error) *openStageFault {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	f := &openStageFault{path: abs, stage: stage, fault: fault}
	if !openStageFaultSeam.CompareAndSwap(nil, f) {
		t.Fatal("another stage fault is armed: the moulds of this batch are sequential")
	}
	t.Cleanup(func() { openStageFaultSeam.CompareAndSwap(f, nil) })
	return f
}

// wasConsumed reports whether the stage fault reached its operation.
func (f *openStageFault) wasConsumed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.consumed
}

// faultReceipt is what a judge fault saw, read after the caller returned.
type faultReceipt struct {
	matches      int
	consumed     bool
	otherOrigins []judgeOrigin
	decisions    []readDecision
}

func (f *judgeReadFault) receipt() faultReceipt {
	f.mu.Lock()
	defer f.mu.Unlock()
	return faultReceipt{
		matches: f.matches, consumed: f.consumed,
		otherOrigins: append([]judgeOrigin(nil), f.otherOrigins...),
		decisions:    append([]readDecision(nil), f.decisions...),
	}
}

// disarm takes f out of the seam (the follow-up operations of a subcase run
// on healthy reads) and returns what it saw.
func (f *judgeReadFault) disarm() faultReceipt {
	judgeReadFaultSeam.CompareAndSwap(f, nil)
	return f.receipt()
}

// checkReceipt is the instrumentation oracle, asked AFTER the behavioural
// one: the fault reached its read exactly once and met ONE decision of its
// site, the one the class demands.
func checkReceipt(t *testing.T, label string, r faultReceipt, site judgeQuerySite, wantVerdict bool, wantClass ledgerClass) {
	t.Helper()
	if !r.consumed || r.matches != 1 {
		t.Fatalf("%s: the fault reached its read %d time(s), consumed=%v; want exactly one hit", label, r.matches, r.consumed)
	}
	if len(r.decisions) != 1 {
		t.Fatalf("%s: the failure met %d site decision(s) %+v, want exactly one", label, len(r.decisions), r.decisions)
	}
	d := r.decisions[0]
	if d.site != site || d.verdict != wantVerdict || d.class != wantClass {
		t.Fatalf("%s: the site decision was %+v, want site %s verdict %v class %s", label, d, site, wantVerdict, wantClass)
	}
}

// markConn plants a marker on the handle's one connection, so a later
// sameConn proves the connection was never replaced between two
// observations of its guard.
func markConn(t *testing.T, s *Store) {
	t.Helper()
	if _, err := s.db.Exec(`CREATE TEMP TABLE IF NOT EXISTS e1_conn_marker (x INTEGER)`); err != nil {
		t.Fatalf("mark the connection: %v", err)
	}
}

func sameConn(t *testing.T, s *Store) {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM temp.e1_conn_marker`).Scan(&n); err != nil {
		t.Fatalf("the handle's connection was replaced between the observations: %v", err)
	}
}

// guardRow reads the connection's guard row.
func guardRow(t *testing.T, s *Store) string {
	t.Helper()
	var row string
	if err := s.db.QueryRow(`SELECT standing FROM temp.profile_guard`).Scan(&row); err != nil {
		t.Fatalf("read the guard row: %v", err)
	}
	return row
}

// guardedWrite is one act through beginWrite.
func guardedWrite(s *Store, id string) error {
	return s.RecordAttempt(context.Background(), testEnvelope(id), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized)
}

// actionsIn counts the actions through a raw connection.
func actionsIn(t *testing.T, path string) int {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	return rawCount(t, raw, `SELECT COUNT(*) FROM actions`)
}

// classSentinel is the store's name for a class.
func classSentinel(c ledgerClass) error {
	switch c {
	case classStructural:
		return ErrLedgerUnreadable
	case classBusy:
		return ErrLedgerBusy
	case classEnvironment:
		return ErrLedgerEnvironment
	default:
		return nil
	}
}

// namesNoClass is true when err carries none of the three class names.
func namesNoClass(err error) bool {
	return !errors.Is(err, ErrLedgerUnreadable) && !errors.Is(err, ErrLedgerBusy) && !errors.Is(err, ErrLedgerEnvironment)
}

// closedPoolError is the error database/sql answers on a closed pool.
func closedPoolError(t *testing.T) error {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "closed.db"))
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	perr := db.Ping()
	if perr == nil {
		t.Fatal("a closed pool pinged")
	}
	return perr
}

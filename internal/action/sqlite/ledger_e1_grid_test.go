// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Train E, batch 1 — the judge's fault grid, TE19–TE28 (plan v3, §§5–7):
// every reachable (origin, site, stage) of the judge, crossed with every
// fault of one row, each subcase on its own copy of a cold template, with ONE
// exact outcome per subcase (§7's single-hit table for the structural codes;
// the class sentinel and the original cause for the operational ones; the
// original error untouched for the rest).
//
// Evidence level: in process, native connections on a real file; the faults
// are SYNTHETIC (codedFault, or an uncoded error) put on the result of a read
// that really ran. Reachability: a (site, stage) that the fixture cannot
// reach is not in the grid — the owner read's scan and the mark read's scan
// need a row the legacy fixture does not have; SchemaVersion is in the grid
// for every row but busy and environment (the plan names no outcome of those
// for an accessor that is not one of the four public boundaries).

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
)

// e1Kind is one fault of a row: a fresh value per subcase, its class.
type e1Kind struct {
	name  string
	make  func() error
	class ledgerClass
}

func codedKinds(class ledgerClass, codes ...int) []e1Kind {
	out := make([]e1Kind, 0, len(codes))
	for _, code := range codes {
		out = append(out, e1Kind{name: fmt.Sprintf("code%d", code), make: func() error { return &codedFault{code: code, tag: "grid"} }, class: class})
	}
	return out
}

// e1Case is one subcase: its entry, fixture, site, stage and fault.
type e1Case struct {
	label   string
	entry   e1Entry
	fixture string
	site    judgeQuerySite
	stage   readStage
	kind    e1Kind
	fault   error
	path    string
	fx      e1Fixture
}

func (c e1Case) arm(t *testing.T) *judgeReadFault {
	return armJudgeFault(t, c.path, c.entry.origin, c.site, c.stage, c.fault)
}

// healthy is the standing the fixture stands for profileA.
func (c e1Case) healthy() LedgerStanding {
	if c.fixture == "legacy" {
		return LedgerStandingLegacyUnfounded
	}
	return LedgerStandingOK
}

// e1Entry is one caller of the judge: its origin, the sites and stages it
// reaches on each fixture, and its run.
type e1Entry struct {
	name   string
	origin judgeOrigin
	reach  map[string]map[judgeQuerySite][]readStage
	run    func(t *testing.T, c e1Case)
	// structuralOnly: the plan names outcomes for the structural and uncoded
	// rows only.
	structuralOnly bool
}

var (
	e1DriverStages = []readStage{stageQuery, stageNext}
	e1DBStages     = []readStage{stageQuery, stageNext, stageScan}
	e1NoRowStages  = []readStage{stageQuery, stageNext}
)

// e1StandingReach is the reach of every entry that judges through judgeIn.
var e1StandingReach = map[string]map[judgeQuerySite][]readStage{
	"founded": {siteCatalog: e1DBStages, siteVersion: e1DBStages, siteOwnerDB: e1DBStages},
	"legacy":  {siteMarkDB: e1NoRowStages},
}

func e1Entries() []e1Entry {
	return []e1Entry{
		{name: "hook-at-birth", origin: originHookAtBirth, run: e1RunOpenFor(true),
			reach: map[string]map[judgeQuerySite][]readStage{
				"founded": {siteCatalog: e1DriverStages, siteVersion: e1DriverStages, siteCountDriver: e1DriverStages, siteOwnerDriver: e1DriverStages},
				"legacy":  {siteMarkDriver: e1DriverStages},
			}},
		{name: "pool-open-shape", origin: originPoolOpenShape, run: e1RunOpenFor(false),
			reach: map[string]map[judgeQuerySite][]readStage{"founded": {siteCatalog: e1DBStages, siteVersion: e1DBStages}}},
		{name: "readOwner-at-open", origin: originReadOwnerAtOpen, run: e1RunOpenFor(false),
			reach: map[string]map[judgeQuerySite][]readStage{"founded": {siteOwnerDB: e1DBStages}, "legacy": {siteOwnerDB: e1NoRowStages}}},
		{name: "operator-probe", origin: originOperatorProbe, run: e1RunOperator,
			reach: map[string]map[judgeQuerySite][]readStage{"founded": {siteCatalog: e1DBStages, siteVersion: e1DBStages}}},
		{name: "read-only-open", origin: originReadOnlyOpen, run: e1RunReader,
			reach: map[string]map[judgeQuerySite][]readStage{"founded": {siteCatalog: e1DBStages, siteVersion: e1DBStages}}},
		{name: "installGuard-refresh", origin: originRefresh, run: e1RunOpenFor(true), reach: e1StandingReach},
		{name: "OpenFor-final-Standing", origin: originOpenForStanding, run: e1RunOpenFor(false), reach: e1StandingReach},
		{name: "public-Standing", origin: originPublicStanding, run: e1RunStanding, reach: e1StandingReach},
		{name: "beginWrite", origin: originBeginWrite, run: e1RunWrite, reach: e1StandingReach},
		{name: "prune-through-OpenFor", origin: originBeginWrite, run: e1RunOpenFor(true), reach: e1StandingReach},
		{name: "beginAdoption", origin: originBeginAdoption, run: e1RunAdoption, reach: e1StandingReach},
		{name: "refuseMaintenance", origin: originRefuseMaintenance, run: e1RunMaintenance, reach: e1StandingReach},
		{name: "SchemaVersion-accessor", origin: originSchemaVersion, run: e1RunSchemaVersion, structuralOnly: true,
			reach: map[string]map[judgeQuerySite][]readStage{"founded": {siteVersion: e1DBStages}}},
	}
}

// runE1Grid runs every reachable subcase of the entries for the kinds.
func runE1Grid(t *testing.T, kinds []e1Kind, only func(e1Entry) bool) {
	fx := newE1Fixture(t)
	for _, entry := range e1Entries() {
		if only != nil && !only(entry) {
			continue
		}
		for _, fixture := range []string{"founded", "legacy"} {
			for site, stages := range entry.reach[fixture] {
				for _, stage := range stages {
					for _, kind := range kinds {
						if entry.structuralOnly && kind.class != classStructural && kind.class != classNone {
							continue
						}
						entry, fixture, site, stage, kind := entry, fixture, site, stage, kind
						label := fmt.Sprintf("%s/%s/%s/%s/%s", entry.name, fixture, site, stage, kind.name)
						t.Run(label, func(t *testing.T) {
							template := fx.founded
							if fixture == "legacy" {
								template = fx.legacy
							}
							c := e1Case{label: label, entry: entry, fixture: fixture, site: site, stage: stage,
								kind: kind, fault: kind.make(), path: ledgerCopy(t, template), fx: fx}
							entry.run(t, c)
						})
					}
				}
			}
		}
	}
}

// e1RunOpenFor is the entry of every origin OpenFor itself reaches: the fault
// is armed before the open. sticky is §7's column: the origins whose
// structural verdict is COMMITTED to the connection's guard (the hook, the
// refresh, the prune's write) against the observation-only ones.
func e1RunOpenFor(sticky bool) func(t *testing.T, c e1Case) {
	return func(t *testing.T, c e1Case) {
		ctx := context.Background()
		f := c.arm(t)
		before := actionsIn(t, c.path)
		h, err := OpenFor(c.path, profileA)
		r := f.disarm()
		switch c.kind.class {
		case classStructural:
			if err != nil || h == nil {
				t.Fatalf("%s: OpenFor = %v, want a handle (one structural hit on healthy data)", c.label, err)
			}
			defer func() { _ = h.Close() }()
			markConn(t, h)
			if st, _, err := h.Standing(ctx); err != nil || st != c.healthy() {
				t.Fatalf("%s: the next Standing = %q %v, want %q", c.label, st, err, c.healthy())
			}
			if sticky {
				if row := guardRow(t, h); row != string(LedgerStandingUnreadable) {
					t.Fatalf("%s: the guard row = %q, want ledger_unreadable (a committed verdict)", c.label, row)
				}
				if err := guardedWrite(h, "e1_after"); !errors.Is(err, ErrLedgerUnreadable) {
					t.Fatalf("%s: a write on the same connection = %v, want ErrLedgerUnreadable", c.label, err)
				}
				if n := actionsIn(t, c.path); n != before {
					t.Fatalf("%s: actions %d → %d: the refused write landed", c.label, before, n)
				}
			} else {
				if row := guardRow(t, h); row != "ok" {
					t.Fatalf("%s: the guard row = %q, want ok (an observation stores no verdict)", c.label, row)
				}
				if err := guardedWrite(h, "e1_after"); err != nil {
					t.Fatalf("%s: a write after the healthy re-judgement = %v, want it accepted", c.label, err)
				}
			}
			sameConn(t, h)
			checkReceipt(t, c.label, r, c.site, true, classStructural)
		default:
			e1FailedOpen(t, c, h, err, r)
			e1RetryOpens(t, c)
		}
	}
}

// e1FailedOpen is the one outcome of a failed open: no handle, the class's
// sentinel wrapping the original cause (or, for no class, the original error
// alone), never a verdict.
func e1FailedOpen(t *testing.T, c e1Case, h *Store, err error, r faultReceipt) {
	t.Helper()
	if h != nil {
		_ = h.Close()
		t.Fatalf("%s: the opener handed out a handle over a %s failure", c.label, c.kind.class)
	}
	e1CheckError(t, c, err)
	if isVerdict(err) && c.kind.class != classStructural {
		t.Fatalf("%s: %v is a verdict", c.label, err)
	}
	checkReceipt(t, c.label, r, c.site, false, c.kind.class)
}

// e1CheckError: the class sentinel and the original cause, or the original
// error with no class at all.
func e1CheckError(t *testing.T, c e1Case, err error) {
	t.Helper()
	if !errors.Is(err, c.fault) {
		t.Fatalf("%s: the error %v lost the original cause %v", c.label, err, c.fault)
	}
	if want := classSentinel(c.kind.class); want != nil {
		if !errors.Is(err, want) {
			t.Fatalf("%s: the error %v is not named %v", c.label, err, want)
		}
		return
	}
	if !namesNoClass(err) {
		t.Fatalf("%s: the error %v carries a class; want the original error, unnamed", c.label, err)
	}
}

// e1RetryOpens: with the fault spent, the next open births a healthy handle
// that judges and writes.
func e1RetryOpens(t *testing.T, c e1Case) {
	t.Helper()
	h, err := OpenFor(c.path, profileA)
	if err != nil {
		t.Fatalf("%s: the next OpenFor = %v, want a healthy handle", c.label, err)
	}
	defer func() { _ = h.Close() }()
	if st, _, err := h.Standing(context.Background()); err != nil || st != c.healthy() {
		t.Fatalf("%s: the next handle stands %q %v, want %q", c.label, st, err, c.healthy())
	}
	if err := guardedWrite(h, "e1_retry"); err != nil {
		t.Fatalf("%s: a write on the next handle = %v", c.label, err)
	}
}

func e1RunOperator(t *testing.T, c e1Case) {
	f := c.arm(t)
	h, err := OpenOperatorFor(c.path, profileA)
	r := f.disarm()
	if c.kind.class != classStructural {
		e1FailedOpen(t, c, h, err, r)
		e1RetryOpens(t, c)
		return
	}
	if err != nil || h == nil {
		t.Fatalf("%s: OpenOperatorFor = %v, want a handle (the probe's hit, then a healthy writer open)", c.label, err)
	}
	defer func() { _ = h.Close() }()
	if st, _, err := h.Standing(context.Background()); err != nil || st != c.healthy() {
		t.Fatalf("%s: the next Standing = %q %v, want %q", c.label, st, err, c.healthy())
	}
	if row := guardRow(t, h); row != "ok" {
		t.Fatalf("%s: the guard row = %q, want ok", c.label, row)
	}
	if err := guardedWrite(h, "e1_after"); err != nil {
		t.Fatalf("%s: a write = %v, want it accepted", c.label, err)
	}
	checkReceipt(t, c.label, r, c.site, true, classStructural)
}

func e1RunReader(t *testing.T, c e1Case) {
	f := c.arm(t)
	h, err := OpenReadOnlyFor(c.path, profileA)
	r := f.disarm()
	if c.kind.class != classStructural {
		e1FailedOpen(t, c, h, err, r)
		return
	}
	if err != nil || h == nil {
		t.Fatalf("%s: OpenReadOnlyFor = %v, want the reader (one structural hit on healthy data)", c.label, err)
	}
	defer func() { _ = h.Close() }()
	if st, _, err := h.Standing(context.Background()); err != nil || st != c.healthy() {
		t.Fatalf("%s: the next Standing = %q %v, want %q", c.label, st, err, c.healthy())
	}
	if _, err := h.db.Exec(hookDoorInsert, "e1_sealed"); err == nil {
		t.Fatalf("%s: the reader wrote: the query-only seal is gone", c.label)
	}
	checkReceipt(t, c.label, r, c.site, true, classStructural)
}

// e1OpenHealthy is a healthy handle, opened before the fault is armed.
func e1OpenHealthy(t *testing.T, c e1Case) *Store {
	t.Helper()
	h, err := OpenFor(c.path, profileA)
	if err != nil {
		t.Fatalf("%s: open the healthy handle: %v", c.label, err)
	}
	t.Cleanup(func() { _ = h.Close() })
	markConn(t, h)
	return h
}

func e1RunStanding(t *testing.T, c e1Case) {
	ctx := context.Background()
	h := e1OpenHealthy(t, c)
	f := c.arm(t)
	st, owner, err := h.Standing(ctx)
	r := f.disarm()
	switch c.kind.class {
	case classStructural:
		if st != LedgerStandingUnreadable || owner != "" || !errors.Is(err, ErrLedgerUnreadable) {
			t.Fatalf("%s: Standing = %q %q %v, want unreadable, no owner, ErrLedgerUnreadable", c.label, st, owner, err)
		}
		e1CheckCode(t, c, err)
	default:
		e1CheckError(t, c, err)
		if isVerdict(err) {
			t.Fatalf("%s: %v is a verdict", c.label, err)
		}
		if c.kind.class == classNone && st != LedgerStandingUnreadable {
			t.Fatalf("%s: the standing value beside a failure = %q, want ledger_unreadable (D15)", c.label, st)
		}
	}
	if st, _, err := h.Standing(ctx); err != nil || st != c.healthy() {
		t.Fatalf("%s: the next Standing = %q %v, want %q", c.label, st, err, c.healthy())
	}
	if row := guardRow(t, h); row != "ok" {
		t.Fatalf("%s: the guard row = %q, want ok (Standing stores nothing)", c.label, row)
	}
	if err := guardedWrite(h, "e1_after"); err != nil {
		t.Fatalf("%s: a write after the failed Standing = %v", c.label, err)
	}
	sameConn(t, h)
	checkReceipt(t, c.label, r, c.site, c.kind.class == classStructural, c.kind.class)
}

// e1CheckCode: the original cause and its code survive in the returned error.
func e1CheckCode(t *testing.T, c e1Case, err error) {
	t.Helper()
	if !errors.Is(err, c.fault) {
		t.Fatalf("%s: the returned error %v lost the original cause", c.label, err)
	}
	var coded interface{ Code() int }
	if cf, ok := c.fault.(*codedFault); ok && (!errors.As(err, &coded) || coded.Code() != cf.code) {
		t.Fatalf("%s: the returned error %v lost the code %d", c.label, err, cf.code)
	}
}

func e1RunWrite(t *testing.T, c e1Case) {
	h := e1OpenHealthy(t, c)
	before := actionsIn(t, c.path)
	f := c.arm(t)
	err := guardedWrite(h, "e1_faulted")
	r := f.disarm()
	if n := actionsIn(t, c.path); n != before {
		t.Fatalf("%s: actions %d → %d: the faulted write landed", c.label, before, n)
	}
	if c.kind.class == classStructural {
		if !errors.Is(err, ErrLedgerUnreadable) {
			t.Fatalf("%s: the write = %v, want ErrLedgerUnreadable", c.label, err)
		}
		e1CheckCode(t, c, err)
		if row := guardRow(t, h); row != string(LedgerStandingUnreadable) {
			t.Fatalf("%s: the guard row = %q, want ledger_unreadable", c.label, row)
		}
		if err := guardedWrite(h, "e1_after"); !errors.Is(err, ErrLedgerUnreadable) {
			t.Fatalf("%s: the next write on the same connection = %v, want ErrLedgerUnreadable", c.label, err)
		}
	} else {
		e1CheckError(t, c, err)
		if isVerdict(err) {
			t.Fatalf("%s: %v is a verdict", c.label, err)
		}
		if row := guardRow(t, h); row != "ok" {
			t.Fatalf("%s: the guard row = %q, want ok (no verdict)", c.label, row)
		}
		if err := guardedWrite(h, "e1_after"); err != nil {
			t.Fatalf("%s: the next write = %v, want it accepted", c.label, err)
		}
	}
	sameConn(t, h)
	checkReceipt(t, c.label, r, c.site, c.kind.class == classStructural, c.kind.class)
}

func e1RunAdoption(t *testing.T, c e1Case) {
	ctx := context.Background()
	b, err := OpenOperatorFor(c.path, profileB)
	if err != nil {
		t.Fatalf("%s: open B: %v", c.label, err)
	}
	defer func() { _ = b.Close() }()
	wireSealedLike(t, b, c.fx.ink(c.fixture))
	markConn(t, b)
	guardBefore := guardRow(t, b)
	ownerRows := func() int {
		raw, err := sql.Open("sqlite", "file:"+c.path+"?_pragma=busy_timeout(5000)")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = raw.Close() }()
		return rawCount(t, raw, `SELECT COUNT(*) FROM ledger_identity WHERE owner_digest = '`+profileB+`'`)
	}
	env, evidence := c.fx.evidenceFor(c.fixture, "e1_adopt")
	f := c.arm(t)
	_, err = b.AdoptLedger(ctx, adoptionEnv(env), Decision{Outcome: "allow", Rule: "operator"}, evidence, profileB)
	r := f.disarm()
	if n := ownerRows(); n != 0 {
		t.Fatalf("%s: the faulted adoption wrote B's row", c.label)
	}
	if c.kind.class == classStructural {
		if !errors.Is(err, ErrLedgerUnreadable) {
			t.Fatalf("%s: the adoption = %v, want ErrLedgerUnreadable", c.label, err)
		}
		e1CheckCode(t, c, err)
		if row := guardRow(t, b); row != string(LedgerStandingUnreadable) {
			t.Fatalf("%s: the guard row = %q, want ledger_unreadable", c.label, row)
		}
		if err := b.unguardedDoorForTest(ctx, hookDoorInsert, "e1_after"); !errors.Is(err, ErrLedgerUnreadable) {
			t.Fatalf("%s: a write on the same connection = %v, want ErrLedgerUnreadable", c.label, err)
		}
	} else {
		e1CheckError(t, c, err)
		if isVerdict(err) {
			t.Fatalf("%s: %v is a verdict", c.label, err)
		}
		if row := guardRow(t, b); row != guardBefore {
			t.Fatalf("%s: the guard row = %q, want it unchanged (%q)", c.label, row, guardBefore)
		}
		env2, evidence2 := c.fx.evidenceFor(c.fixture, "e1_adopt_retry")
		if _, err := b.AdoptLedger(ctx, adoptionEnv(env2), Decision{Outcome: "allow", Rule: "operator"}, evidence2, profileB); err != nil {
			t.Fatalf("%s: the retried adoption = %v, want it recorded", c.label, err)
		}
		if n := ownerRows(); n != 1 {
			t.Fatalf("%s: after the retried adoption B owns %d rows, want 1", c.label, n)
		}
	}
	sameConn(t, b)
	checkReceipt(t, c.label, r, c.site, c.kind.class == classStructural, c.kind.class)
}

func e1RunMaintenance(t *testing.T, c e1Case) {
	ctx := context.Background()
	h := e1OpenHealthy(t, c)
	f := c.arm(t)
	_, err := h.RecoverPreviousLife(ctx)
	r := f.disarm()
	if c.kind.class == classStructural {
		if !errors.Is(err, ErrLedgerUnreadable) {
			t.Fatalf("%s: the maintenance = %v, want ErrLedgerUnreadable", c.label, err)
		}
		e1CheckCode(t, c, err)
	} else {
		e1CheckError(t, c, err)
		if isVerdict(err) {
			t.Fatalf("%s: %v is a verdict", c.label, err)
		}
	}
	if row := guardRow(t, h); row != "ok" {
		t.Fatalf("%s: the guard row = %q, want ok (the maintenance's judgement stores nothing)", c.label, row)
	}
	if _, err := h.RecoverPreviousLife(ctx); err != nil {
		t.Fatalf("%s: the next maintenance = %v, want it to proceed", c.label, err)
	}
	sameConn(t, h)
	checkReceipt(t, c.label, r, c.site, c.kind.class == classStructural, c.kind.class)
}

func e1RunSchemaVersion(t *testing.T, c e1Case) {
	h := e1OpenHealthy(t, c)
	f := c.arm(t)
	v, err := h.SchemaVersion(context.Background())
	r := f.disarm()
	if v != 0 {
		t.Fatalf("%s: SchemaVersion = %d beside a failure, want 0", c.label, v)
	}
	if c.kind.class == classStructural {
		if !errors.Is(err, ErrLedgerUnreadable) {
			t.Fatalf("%s: SchemaVersion = %v, want ErrLedgerUnreadable", c.label, err)
		}
		e1CheckCode(t, c, err)
	} else {
		e1CheckError(t, c, err)
	}
	if row := guardRow(t, h); row != "ok" {
		t.Fatalf("%s: the guard row = %q, want ok", c.label, row)
	}
	checkReceipt(t, c.label, r, c.site, c.kind.class == classStructural, c.kind.class)
}

// TE19 · an UNCODED failure of a judge read — the literal «database is
// locked» string, a finished transaction, a closed pool, a cancelled or
// expired context — is its original error at every caller: never a verdict,
// never a class, never «no row»; the judgeIn standing beside it stays
// ledger_unreadable (D15); the next healthy call works.
//
// PROBING MUTATIONS (MU19): every uncoded Scan error becomes shape (the
// structural decision on an error with no code) → the subcases reading a
// verdict redden; «unknown means no rows» at the owner read → the handle
// judges legacy or the mark → reddens.
//
// Evidence level: in process native handle + controlled query failure.
func TestE1_TE19_anUncodedJudgeFailureIsItsOriginalError(t *testing.T) {
	closed := closedPoolError(t)
	kinds := []e1Kind{
		{name: "locked-text", make: func() error { return errors.New("database is locked") }, class: classNone},
		{name: "tx-done", make: func() error { return sql.ErrTxDone }, class: classNone},
		{name: "closed-pool", make: func() error { return closed }, class: classNone},
		{name: "canceled", make: func() error { return context.Canceled }, class: classNone},
		{name: "deadline", make: func() error { return context.DeadlineExceeded }, class: classNone},
	}
	runE1Grid(t, kinds, nil)
}

// TE20–TE23 · one CORRUPT, MISMATCH, FORMAT or NOTADB result on a judge read
// is a structural verdict at its site, and its caller's outcome is §7's
// single-hit row for that origin — a handle over healthy data for the
// observation-only origins (the later healthy write REQUIRED), a committed
// unreadable guard for the hook, the refresh and the prune's write, a typed
// ErrLedgerUnreadable carrying the original cause and code for the callers
// that return one.
//
// PROBING MUTATIONS (MU20–MU23): remove the code from the structural set in
// each judge family (judgeShape; judgeOnConn's own reads; judgeIn's owner and
// mark reads with readOwner) → that family's subcases redden.
//
// Evidence level: in process native handle + synthetic coded failure.
func TestE1_TE20_code11IsAStructuralVerdictAtEverySite(t *testing.T) {
	runE1Grid(t, codedKinds(classStructural, 11), nil)
}

func TestE1_TE21_code20IsAStructuralVerdictAtEverySite(t *testing.T) {
	runE1Grid(t, codedKinds(classStructural, 20), nil)
}

func TestE1_TE22_code24IsAStructuralVerdictAtEverySite(t *testing.T) {
	runE1Grid(t, codedKinds(classStructural, 24), nil)
}

func TestE1_TE23_code26IsAStructuralVerdictAtEverySite(t *testing.T) {
	runE1Grid(t, codedKinds(classStructural, 26), nil)
}

// TE24–TE26 · BUSY, LOCKED and every environment code on a judge read are
// operational failures at every caller: ErrLedgerBusy or ErrLedgerEnvironment
// wrapping the original cause, never a verdict, never a poisoned guard; a
// failed open hands out nothing, a failed write names its cause, and after
// the condition the next open, judgement and write work.
//
// PROBING MUTATIONS (MU24–MU26): map the class to a verdict inside
// judgeOnConn or judgeIn → redden; broaden isVerdict in refresh, write or
// adoption to the class → redden.
//
// Evidence level: in process native connections + controlled failure.
func TestE1_TE24_busyIsNeverAVerdict(t *testing.T) {
	runE1Grid(t, codedKinds(classBusy, 5), nil)
}

func TestE1_TE25_lockedIsNeverAVerdict(t *testing.T) {
	runE1Grid(t, codedKinds(classBusy, 6), nil)
}

func TestE1_TE26_theEnvironmentIsNeverAVerdict(t *testing.T) {
	runE1Grid(t, codedKinds(classEnvironment, 3, 8, 10, 13, 14, 22, 23), nil)
}

// TE27 · the EXTENDED code 522 (IOERR_SHORT_READ) on a Standing read is the
// environment class — the primary code is its low byte — and the returned
// error still carries 522.
//
// PROBING MUTATION (MU27): compare the whole code, without the mask → 522 is
// no class → reddens.
//
// Evidence level: in process native handle + synthetic coded failure (the
// recorder's projection is the app package's half of this row).
func TestE1_TE27_anExtendedCodeKeepsItsPrimaryClass(t *testing.T) {
	runE1Grid(t, codedKinds(classEnvironment, 522), func(e e1Entry) bool { return e.origin == originPublicStanding })
}

// TE28 · every other primary code — and a numeric status that is not an
// error at all — is its original coded error at every caller: no class, no
// verdict, nothing swallowed; 15 (PROTOCOL, a WAL lock protocol retry) in
// particular must never poison a guard.
//
// PROBING MUTATIONS (MU28): classify each listed code as structural; default
// every code to busy; an unknown code as a verdict → redden.
//
// Evidence level: in process with coded failure.
func TestE1_TE28_anUncategorisedCodeIsItsOriginalError(t *testing.T) {
	kinds := codedKinds(classNone, 2, 4, 7, 9, 12, 15, 16, 17, 18, 19, 21, 25, 27, 28)
	kinds = append(kinds, codedKinds(classNone, 0, 101)...)
	runE1Grid(t, kinds, nil)
}

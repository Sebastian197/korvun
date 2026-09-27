// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Train E, batch 1 — failures AROUND the judgement (plan v3, §6 rows TE29–
// TE32, §7 «S3», §13.3): a connection whose birth fails is an error of the
// opener with its class, never a shape; every escaping error of the openers
// carries its class; a file that is not a database and a virtual table where
// a guarded one belongs are named by the store, with the real native codes.
//
// Evidence level: in process, native connections on a real file. TE29/TE30
// replace the result of ONE named operation that really ran (openStageFault:
// synthetic coded or uncoded errors); TE31/TE32 are native failures, no seam.

package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	msqlite "modernc.org/sqlite"
)

// e1StageCodes are the codes TE29/TE30 put on an operation: two structural
// families and NOTADB, BUSY, IOERR.
var e1StageCodes = []int{1, 11, 26, 5, 10}

// e1TriggerCount is how many triggers the guard creates on a v16 file.
func e1TriggerCount() int { return len(guardTriggerStatementsFor(schemaTablesV16)) }

// e1FailedStageOpen: no handle, the class of the code wrapping the fault (or
// the fault alone when uncoded), nothing written, and the next open healthy.
// causeIdentity is false only where the error passes through mapGuardError's
// busy branch, which names the class and keeps the cause as TEXT (train H
// owns that function; recorded for the closing list, not changed here).
func e1FailedStageOpen(t *testing.T, label, path string, h *Store, err, fault error, code int, before map[string]string, causeIdentity bool) {
	t.Helper()
	if h != nil {
		_ = h.Close()
		t.Fatalf("%s: the opener handed out a handle over a failed operation", label)
	}
	if causeIdentity && !errors.Is(err, fault) {
		t.Fatalf("%s: the error %v lost the original cause", label, err)
	}
	if code == 0 {
		if !namesNoClass(err) {
			t.Fatalf("%s: the uncoded failure %v was named", label, err)
		}
	} else if want := classSentinel(classOf(&codedFault{code: code})); !errors.Is(err, want) {
		t.Fatalf("%s: the error %v is not named %v", label, err, want)
	}
	raw := rawConnPath(t, path)
	if after := catalogOf(t, raw); before != nil && !reflect.DeepEqual(before, after) {
		t.Fatalf("%s: the failed open changed the catalog", label)
	}
	next, err := OpenFor(path, profileA)
	if err != nil {
		t.Fatalf("%s: the next OpenFor = %v, want a healthy handle", label, err)
	}
	if st, _, err := next.Standing(context.Background()); err != nil || st != LedgerStandingOK {
		_ = next.Close()
		t.Fatalf("%s: the next handle stands %q %v", label, st, err)
	}
	_ = next.Close()
}

// TE29 · a failure of one of the hook's own statements AFTER its judgement —
// the temp guard table's create, delete or insert, the catalog read, any of
// the generated triggers — is a failed connection birth: the writer openers
// hand out no handle, the error carries the class of its code (1/11/26
// unreadable, 5 busy, 10 environment) and its original cause, nothing is
// written, and the connection is never re-born into a usable handle.
//
// PROBING MUTATIONS (MU29): judge on the pool instead of the held connection
// (the birth fails inside the judge's query and a one-hit structural code is
// swallowed as a bad shape: a blocked handle) → reddens; swallow a hook
// statement's error → a handle → reddens; strip the public class → reddens.
func TestE1_TE29_aHookStatementFailureIsAFailedBirth(t *testing.T) {
	fx := newE1Fixture(t)
	stages := []string{"hook:temp-create", "hook:temp-delete", "hook:temp-insert", "hook:catalog"}
	for i := 0; i < e1TriggerCount(); i++ {
		stages = append(stages, fmt.Sprintf("hook:trigger#%d", i))
	}
	for _, opener := range e1Openers[:2] {
		for _, stage := range stages {
			for _, code := range e1StageCodes {
				opener, stage, code := opener, stage, code
				label := fmt.Sprintf("%s/%s/code%d", opener.name, stage, code)
				t.Run(label, func(t *testing.T) {
					path := ledgerCopy(t, fx.founded)
					before := catalogOf(t, rawConnPath(t, path))
					fault := &codedFault{code: code, tag: stage}
					f := armStageFault(t, path, stage, fault)
					h, err := opener.open(path, profileA)
					openStageFaultSeam.CompareAndSwap(f, nil)
					e1FailedStageOpen(t, label, path, h, err, fault, code, before, true)
					if !f.wasConsumed() {
						t.Fatalf("%s: the stage was never reached", label)
					}
				})
			}
		}
	}
}

// TE30 · every escaping error site of the openers carries its class: the
// guard's catalog read, each trigger it re-creates and its guard update, the
// prune's begin, count, delete, rows-affected and commit, the probe's and the
// reader's seals and the ping. A coded failure is its class wrapping the
// cause; an uncoded one is itself, unnamed; no handle, no later work.
//
// PROBING MUTATIONS (MU30): remove the classification from one public
// boundary (OpenFor, OpenOperatorFor, OpenReadOnlyFor) → its sites redden;
// swallow one install or prune error → a handle → reddens.
func TestE1_TE30_everyEscapingErrorCarriesItsClass(t *testing.T) {
	fx := newE1Fixture(t)
	type site struct {
		stage   string
		openers []int // indexes into e1Openers
		prune   bool
	}
	sites := []site{
		{stage: "install:catalog", openers: []int{0, 1}},
		{stage: "install:guard-update", openers: []int{0, 1}},
		{stage: "write:begin", openers: []int{0}},
		{stage: "prune:count", openers: []int{0}, prune: true},
		{stage: "prune:delete", openers: []int{0}, prune: true},
		{stage: "prune:rows-affected", openers: []int{0}, prune: true},
		{stage: "prune:commit", openers: []int{0}, prune: true},
		{stage: "probe:seal", openers: []int{1}},
		{stage: "readonly:seal", openers: []int{2}},
		{stage: "open:ping", openers: []int{0, 1}},
	}
	for i := 0; i < e1TriggerCount(); i++ {
		sites = append(sites, site{stage: fmt.Sprintf("install:trigger#%d", i), openers: []int{0, 1}})
	}
	codes := append(append([]int(nil), e1StageCodes...), 0)
	for _, s := range sites {
		for _, oi := range s.openers {
			for _, code := range codes {
				s, opener, code := s, e1Openers[oi], code
				label := fmt.Sprintf("%s/%s/code%d", opener.name, s.stage, code)
				t.Run(label, func(t *testing.T) {
					path := ledgerCopy(t, fx.founded)
					actions := actionsIn(t, path)
					if s.prune {
						// A cap one row under the ledger's: the prune removes exactly one act.
						abs, _ := filepath.Abs(path)
						o := &openCapOverride{path: abs, capRows: actions - 1}
						openCapSeam.Store(o)
						t.Cleanup(func() { openCapSeam.CompareAndSwap(o, nil) })
					}
					before := catalogOf(t, rawConnPath(t, path))
					var fault error = &codedFault{code: code, tag: s.stage}
					if code == 0 {
						fault = errors.New("TE30: an uncoded failure at " + s.stage)
					}
					f := armStageFault(t, path, s.stage, fault)
					h, err := opener.open(path, profileA)
					openStageFaultSeam.CompareAndSwap(f, nil)
					openCapSeam.Store(nil)
					if !f.wasConsumed() {
						if h != nil {
							_ = h.Close()
						}
						t.Fatalf("%s: the stage was never reached (err %v)", label, err)
					}
					if s.stage == "prune:commit" {
						// The commit really ran before its result was replaced: the
						// pruned act is gone — read from the file, never inferred
						// from the returned error.
						if n := actionsIn(t, path); n != actions-1 {
							t.Fatalf("%s: actions %d → %d after a commit that ran", label, actions, n)
						}
						before = nil
					} else if n := actionsIn(t, path); n != actions {
						t.Fatalf("%s: actions %d → %d: the failed step did not roll back", label, actions, n)
					}
					causeIdentity := !(s.stage == "write:begin" && code != 0 && classOf(&codedFault{code: code}) == classBusy)
					e1FailedStageOpen(t, label, path, h, err, fault, code, before, causeIdentity)
				})
			}
		}
	}
}

// TE30, the pair of §13.3 · OpenFor's final Standing is the one place where
// a failed BIRTH and a structural QUERY both reach isVerdict. A birth that
// fails with a structural code is the open's error — no handle — while one
// structural hit on the Standing's own read is an observation: a handle, no
// maintenance.
//
// PROBING MUTATION (MU72): remove the birth exclusion before isVerdict at
// OpenFor's Standing → the failed birth, named unreadable at Standing's exit,
// reads as a verdict → a handle → reddens.
func TestE1_TE30_aFailedBirthAtOpenForsStandingIsNotAVerdict(t *testing.T) {
	fx := newE1Fixture(t)
	t.Run("birth", func(t *testing.T) {
		path := ledgerCopy(t, fx.founded)
		abs, _ := filepath.Abs(path)
		fault := &codedFault{code: 11, tag: "birth at OpenFor's Standing"}
		var armed *openStageFault
		seam := func() {
			armed = &openStageFault{path: abs, stage: "hook:temp-create", fault: fault}
			openStageFaultSeam.Store(armed)
			time.Sleep(30 * time.Millisecond) // the pool's connection expires: Standing births a new one
		}
		openStandingSeam.Store(&seam)
		prev := poolLifetimeForTest.Load()
		lifetime := time.Millisecond
		poolLifetimeForTest.Store(&lifetime)
		t.Cleanup(func() {
			openStandingSeam.Store(nil)
			openStageFaultSeam.Store(nil)
			poolLifetimeForTest.Store(prev)
		})
		h, err := OpenFor(path, profileA)
		openStageFaultSeam.Store(nil)
		if h != nil {
			_ = h.Close()
			t.Fatal("OpenFor handed out a handle over a failed birth at its Standing")
		}
		if !errors.Is(err, ErrLedgerUnreadable) || !errors.Is(err, fault) {
			t.Fatalf("OpenFor = %v, want the failed birth named unreadable with its cause", err)
		}
		if armed == nil || !armed.wasConsumed() {
			t.Fatal("the seam never reached the Standing's birth")
		}
	})
	t.Run("query", func(t *testing.T) {
		path := ledgerCopy(t, fx.founded)
		fault := &codedFault{code: 11, tag: "query at OpenFor's Standing"}
		f := armJudgeFault(t, path, originOpenForStanding, siteCatalog, stageQuery, fault)
		h, err := OpenFor(path, profileA)
		f.disarm()
		if err != nil || h == nil {
			t.Fatalf("OpenFor after one structural hit on its Standing's read = %v, want a handle", err)
		}
		_ = h.Close()
	})
}

// TE31 · a file of text where the ledger belongs: the native NOTADB (26) is
// met before any judgement, at the probe's seal, the reader's seal and the
// writer's birth — no opener hands anything out, each names
// ErrLedgerUnreadable with the native code in the chain, and the file's
// bytes are untouched.
//
// PROBING MUTATIONS (MU31): omit OpenOperatorFor's outer classification →
// reddens; omit the reader's → reddens.
func TestE1_TE31_aTextFileIsNamedUnreadableByEveryOpener(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "korvun.db")
	text := bytes.Repeat([]byte("this file is not a SQLite database; it is text.\n"), 128)
	if err := os.WriteFile(path, text, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(text)
	for _, o := range e1Openers {
		h, err := o.open(path, profileA)
		if h != nil {
			_ = h.Close()
			t.Fatalf("%s handed out a handle over a text file", o.name)
		}
		var native *msqlite.Error
		if !errors.As(err, &native) || native.Code()&0xff != 26 {
			t.Fatalf("%s over a text file = %v, want the native NOTADB (26) in the chain", o.name, err)
		}
		if !errors.Is(err, ErrLedgerUnreadable) {
			t.Fatalf("%s over a text file = %v, want ErrLedgerUnreadable", o.name, err)
		}
	}
	now, err := os.ReadFile(path) //nolint:gosec // G304: the test's own temp file
	if err != nil || sha256.Sum256(now) != sum {
		t.Fatalf("the text file's bytes changed (%v)", err)
	}
}

// TE32 · N10-6's limit: an FTS5 virtual table named `actions` where the
// guarded table belongs. The shape validates the KIND of every table it
// needs, so the reader opens and names the virtual table. The writers hand
// out nothing, named unreadable with the trigger's cause: SQLite refuses a
// trigger on a virtual table, at the connection's birth (the hook) and,
// were that refusal swallowed there, again at installGuard. The judge's
// verdict alone would open them, with a handle standing unreadable, and
// this mould rejects any handle. Nothing repairs the virtual table.
//
// PROBING MUTATIONS (MU32): omit the virtual-kind validation → the reader
// stands ok → reddens; swallow the trigger's failure at BOTH sites → the
// writer opens → reddens. Swallowing it at the hook alone does not redden
// this mould, because installGuard meets the same refusal:
// TestE1_TE29_aHookStatementFailureIsAFailedBirth kills that mutation.
func TestE1_TE32_aVirtualGuardedTableIsNamedNotSwallowed(t *testing.T) {
	t.Parallel()
	path, exec := e1Founded(t)
	exec(`PRAGMA foreign_keys = OFF`)
	exec(`DROP TABLE actions`)
	exec(`CREATE VIRTUAL TABLE actions USING fts5(action_id)`)
	raw := rawConnPath(t, path)
	var ddl string
	if err := raw.QueryRow(`SELECT sql FROM sqlite_master WHERE name = 'actions'`).Scan(&ddl); err != nil || !strings.HasPrefix(strings.ToUpper(ddl), "CREATE VIRTUAL TABLE") {
		t.Fatalf("the fixture is not a virtual actions table: %q %v", ddl, err)
	}
	before := catalogOf(t, raw)
	for _, o := range e1Openers[:2] {
		h, err := o.open(path, profileA)
		if h != nil {
			_ = h.Close()
			t.Fatalf("%s handed out a handle over a virtual guarded table", o.name)
		}
		var native *msqlite.Error
		if !errors.Is(err, ErrLedgerUnreadable) || !strings.Contains(err.Error(), "trigger") || !errors.As(err, &native) || native.Code()&0xff != 1 {
			t.Fatalf("%s = %v, want ErrLedgerUnreadable carrying the native trigger refusal (code 1)", o.name, err)
		}
	}
	h, err := OpenReadOnlyFor(path, profileA)
	if err != nil {
		t.Fatalf("OpenReadOnlyFor over a virtual guarded table = %v, want the reader", err)
	}
	st, _, serr := h.Standing(context.Background())
	_ = h.Close()
	if st != LedgerStandingUnreadable || !errors.Is(serr, ErrLedgerUnreadable) || !strings.Contains(serr.Error(), "actions") || !strings.Contains(serr.Error(), "virtual") {
		t.Fatalf("the reader's Standing = %q %v, want unreadable naming the virtual actions table", st, serr)
	}
	if after := catalogOf(t, raw); !reflect.DeepEqual(before, after) {
		t.Fatal("an opener repaired the virtual table")
	}
}

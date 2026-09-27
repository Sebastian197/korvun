// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
)

// Train E, batch 3 (GE6) — the app's moulds of the migration: TE36, TE37 and
// the app's half of TE38 (plan v3, §4 «Migration», §6). The fixtures are
// ledgers founded by the boot's own profile and taken back to an older
// version by a raw connection (the plan's downgraded fixtures): the step
// that lifts them runs inside the REAL boot.
//
// Evidence level: real app, in-process host, real file (Build, and for TE37
// Run and HTTP on loopback).

// e3Conversation are the objects the boot's conversation store may create on
// a file that never held them (plan §7, TE41's adjacent variant).
var e3Conversation = map[string]bool{"table:sessions": true, "table:turns": true, "table:notes": true}

// e3SameLedger fails unless after holds every object of before, unchanged,
// plus exactly the objects allowed.
func e3SameLedger(t *testing.T, label string, before, after map[string]string, allowed ...string) {
	t.Helper()
	extra := map[string]bool{}
	for k := range e3Conversation {
		extra[k] = true
	}
	for _, k := range allowed {
		extra[k] = true
	}
	for k, v := range before {
		if after[k] != v {
			t.Fatalf("%s: %s changed or disappeared", label, k)
		}
	}
	for k := range after {
		if _, ok := before[k]; !ok && !extra[k] {
			t.Fatalf("%s: %s appeared", label, k)
		}
	}
}

// e3Version reads the stored version, raw.
func e3Version(t *testing.T, raw *sql.DB) int {
	t.Helper()
	var v int
	if err := raw.QueryRow(`SELECT version FROM action_schema`).Scan(&v); err != nil {
		t.Fatalf("read the version: %v", err)
	}
	return v
}

// e3BuildFails runs build and returns Build's error; a Build that succeeds is
// shut down and fails the mould.
func e3BuildFails(t *testing.T, label string, build func() (*App, error)) error {
	t.Helper()
	a, err := build()
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = a.Shutdown(ctx)
		t.Fatalf("%s: Build = a running app, want the boot to fail", label)
	}
	return err
}

// TE36 · a step that fails inside the real boot kills the boot: a v15 ledger
// without its receipts table fails the 15→16 copy that reads the mark, and
// Build fails «app: open action store» with ErrLedgerUnreadable naming
// receipts; the step rolled back whole — the version still 15, no identity
// table — twice.
//
// PROBING MUTATIONS (MU36): the copy's failure loses its code (%v) → no
// class → reddens; the step goes on after a failed copy → the boot lives
// blocked → reddens.
func TestE3_TE36_aFailedStepKillsTheBootAndKeepsItsVersion(t *testing.T) {
	cfg := e1Cfg(t)
	path, founder := e1FoundedFor(t, cfg)
	raw := e1Raw(t, path)
	e1Exec(t, raw, `PRAGMA foreign_keys = OFF`)
	e1Exec(t, raw, `DROP TABLE receipts`)
	e1Exec(t, raw, `DROP TABLE ledger_identity`)
	e1Exec(t, raw, `UPDATE action_schema SET version = 15`)
	before := ledgerCatalog(t, raw)
	for attempt := 1; attempt <= 2; attempt++ {
		label := fmt.Sprintf("attempt %d", attempt)
		err := e3BuildFails(t, label, func() (*App, error) {
			return Build(cfg, withChannelFactory(okFactory(newFakeChannel("telegram"))),
				WithReloader(&profileReloader{cfg: cfg}), WithProfilePath(founder))
		})
		if !strings.Contains(err.Error(), "app: open action store") || !errors.Is(err, actionsqlite.ErrLedgerUnreadable) || !strings.Contains(err.Error(), "receipts") {
			t.Fatalf("%s: Build = %v, want «app: open action store» with ErrLedgerUnreadable naming receipts", label, err)
		}
		if v := e3Version(t, raw); v != 15 {
			t.Fatalf("%s: the version is %d after the failed step, want 15", label, v)
		}
		e3SameLedger(t, label+": the failed step", before, ledgerCatalog(t, raw))
	}
}

// TE37 · a step that completes over an old ledger missing a table the step
// does not read (intents, at v15) commits, and the ledger is then judged at
// its new version: the boot lives, blocked, the screen names intents; the
// version is 16, the identity table the step creates is there, seeded from
// the founding mark, and intents is still missing — the migration neither
// failed nor repaired it.
//
// PROBING MUTATION (MU37): the current-table check of judgeShape ignores a
// missing intents → the boot takes the ledger for healthy and dies reading
// the missing table instead of naming it → reddens.
func TestE3_TE37_aLiftedLedgerIsJudgedAtItsNewVersion(t *testing.T) {
	cfg := e1Cfg(t)
	path, founder := e1FoundedFor(t, cfg)
	raw := e1Raw(t, path)
	e1Exec(t, raw, `PRAGMA foreign_keys = OFF`)
	e1Exec(t, raw, `DROP TABLE intents`)
	e1Exec(t, raw, `DROP TABLE ledger_identity`)
	e1Exec(t, raw, `UPDATE action_schema SET version = 15`)
	before := ledgerCatalog(t, raw)
	e1WantUnreadable(t, e1Boot(t, cfg, founder), "intents")
	if v := e3Version(t, raw); v != 16 {
		t.Fatalf("the version is %d after the boot, want the completed step's 16", v)
	}
	after := ledgerCatalog(t, raw)
	if _, ok := after["table:intents"]; ok {
		t.Fatal("the boot recreated intents: the migration repaired a table it does not own")
	}
	e3SameLedger(t, "the completed step", before, after, "table:ledger_identity")
	var owner string
	if err := raw.QueryRow(`SELECT owner_digest FROM ledger_identity`).Scan(&owner); err != nil || owner != ProfileIdentity(founder) {
		t.Fatalf("the identity row = %q (%v), want the founder's, seeded from the mark", owner, err)
	}
}

// TE38 (the app's half) · a tombstone whose human verb carries no deciding
// principal, in a founded ledger taken back to v11, kills the boot at the
// 11→12 revalidation: Build fails «app: open action store» with
// ErrLedgerUnreadable and the TombstoneFault reachable — its row and field —
// and the ledger stays at v11 with its tombstone as it was, twice. This half
// meets the typed fault through the 11→12 revalidation, not the 10→11 copy
// of the sqlite half: its fixture is a founded current ledger labelled v11,
// whose tombstone table is already v11's. The native constraint of TE38 has
// no app half here: that table keys the approval, so the colliding pair of
// the sqlite half cannot be laid in it.
//
// PROBING MUTATIONS (MU38): the typed fault exempted from the migration's
// class, or named with %v → reddens.
func TestE3_TE38_theBootOverACorruptTombstoneIsFatalAndTyped(t *testing.T) {
	cfg := e1Cfg(t)
	path, founder := e1FoundedFor(t, cfg)
	raw := e1Raw(t, path)
	e1Exec(t, raw, `INSERT INTO approval_tombstones
	    (approval_id, approval_digest, action_id, action_digest, preview_digest,
	     policy_version, policy_digest, decision_principal_id, decision, decision_at)
	 VALUES ('apr_e3app000000000000000000000001', 'sha256:`+strings.Repeat("ab", 32)+`', 'act_e3app',
	         'sha256:aaaa', 'sha256:pppp', 3, 'sha256:llll', '', 'rejected', '2026-09-03T01:02:03Z')`)
	e1Exec(t, raw, `UPDATE action_schema SET version = 11`)
	before := ledgerCatalog(t, raw)
	var rowsBefore int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM approval_tombstones WHERE decision_principal_id = '' AND decision = 'rejected'`).Scan(&rowsBefore); err != nil || rowsBefore != 1 {
		t.Fatalf("the corrupt tombstone was not laid: %d (%v)", rowsBefore, err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		label := fmt.Sprintf("attempt %d", attempt)
		err := e3BuildFails(t, label, func() (*App, error) {
			return Build(cfg, withChannelFactory(okFactory(newFakeChannel("telegram"))),
				WithReloader(&profileReloader{cfg: cfg}), WithProfilePath(founder))
		})
		if !strings.Contains(err.Error(), "app: open action store") || !errors.Is(err, actionsqlite.ErrLedgerUnreadable) {
			t.Fatalf("%s: Build = %v, want «app: open action store» with ErrLedgerUnreadable", label, err)
		}
		var fault *actionsqlite.TombstoneFault
		if !errors.As(err, &fault) || fault.Field != "decision_principal_id" || fault.ApprovalID != "apr_e3app000000000000000000000001" {
			t.Fatalf("%s: Build = %v, want the TombstoneFault of apr_e3app…1 at decision_principal_id reachable", label, err)
		}
		if v := e3Version(t, raw); v != 11 {
			t.Fatalf("%s: the version is %d after the failed step, want 11", label, v)
		}
		var rows int
		if err := raw.QueryRow(`SELECT COUNT(*) FROM approval_tombstones WHERE decision_principal_id = '' AND decision = 'rejected'`).Scan(&rows); err != nil || rows != 1 {
			t.Fatalf("%s: the corrupt tombstone is %d row(s) (%v), want it as it was", label, rows, err)
		}
		e3SameLedger(t, label+": the failed step", before, ledgerCatalog(t, raw))
	}
}

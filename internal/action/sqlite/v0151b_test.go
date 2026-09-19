// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.15.1 block B — store-level moulds for the P2-6 class. RED before any cure.
//
// The class: corrupt approval bytes and a transient failure must never trade
// names. A conversion or parse error of a row that WAS read is
// ErrApprovalEvidenceCorrupt (permanent); a context or driver failure is
// ErrApprovalUnreadable (transient) and keeps its cause in the chain. And the
// sealer never turns a corrupt row into an absent one.

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// secondConn is a REAL second *sql.DB on the store's file: the attacker the
// store's own pool never sees coming.
func secondConn(t *testing.T, store *Store) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(store.Path())))
	if err != nil {
		t.Fatalf("second connection: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func attack(t *testing.T, store *Store, stmt string, args ...any) {
	t.Helper()
	if _, err := secondConn(t, store).Exec(stmt, args...); err != nil {
		t.Fatalf("attack %q: %v", stmt, err)
	}
}

// scanCorrupt writes a TEXT into an INTEGER column: database/sql's Scan fails
// with a conversion error ("converting driver.Value type string … to a int64")
// before any parse runs — the level a parse-only cure never sees.
func scanCorrupt(t *testing.T, store *Store, approvalID string) {
	t.Helper()
	attack(t, store, `UPDATE approvals SET policy_version = 'x' WHERE approval_id = ?`, approvalID)
}

// parkedLaw is the pin pendingRequest parks under (v7, "sha256:law"): the
// exported decide judges an approve under it and ignores it for a reject.
var parkedLaw = PolicyPin{Version: 7, Digest: "sha256:law"}

// errOnlyCancelled reports context.Canceled from Err() while its Done() is
// nil: database/sql and the driver select on Done() only, so they proceed,
// and the first code to ask Err() is the caller's own check.
type errOnlyCancelled struct{ context.Context }

func (errOnlyCancelled) Done() <-chan struct{} { return nil }
func (errOnlyCancelled) Err() error            { return context.Canceled }

func cancelled() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// TestV0151B_P2_6_scanLevelCorruptionIsEvidenceCorruptOnTheDecide: the decide
// reads the row through approvalTx, which returns scanApproval's error bare.
// Both verbs.
//
// Evidence: real store file, corruption through a second real connection,
// in-process.
// Probing mutation (planned): leave approvalTx's scan error bare ⇒ both rows red.
func TestV0151B_P2_6_scanLevelCorruptionIsEvidenceCorruptOnTheDecide(t *testing.T) {
	t.Parallel()
	for _, verb := range []string{action.DecisionApproved, action.DecisionRejected} {
		t.Run(verb, func(t *testing.T) {
			t.Parallel()
			store, _ := sealedStore(t)
			a, _ := pendingRequest(t, store, "act_scan_"+verb)
			scanCorrupt(t, store, a.ApprovalID)
			env, ident := operatorDecisionEnv(verb, a.ApprovalID)
			_, err := store.DecideApprovalUnderLaw(context.Background(), a.ApprovalID, verb,
				a.RequestedAt.Add(time.Minute), env, ident, "", parkedLaw)
			if !errors.Is(err, ErrApprovalEvidenceCorrupt) {
				t.Fatalf("decide err = %v, want ErrApprovalEvidenceCorrupt", err)
			}
			if errors.Is(err, ErrApprovalUnreadable) {
				t.Fatalf("err = %v carries BOTH sentinels: corrupt and unreadable are exclusive (class i)", err)
			}
		})
	}
}

// TestV0151B_P2_6_scanLevelCorruptionIsEvidenceCorruptOnTheList.
//
// Evidence: real store file, second real connection, in-process.
// Probing mutation (planned): leave ListApprovals' scan error bare ⇒ red.
func TestV0151B_P2_6_scanLevelCorruptionIsEvidenceCorruptOnTheList(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	a, _ := pendingRequest(t, store, "act_scan_list")
	scanCorrupt(t, store, a.ApprovalID)
	_, err := store.ListApprovals(context.Background(), action.ApprovalPending)
	if !errors.Is(err, ErrApprovalEvidenceCorrupt) {
		t.Fatalf("list err = %v, want ErrApprovalEvidenceCorrupt", err)
	}
	if errors.Is(err, ErrApprovalUnreadable) {
		t.Fatalf("err = %v carries BOTH sentinels: corrupt and unreadable are exclusive (class i)", err)
	}
}

// TestV0151B_P2_6_scanLevelCorruptionIsEvidenceCorruptOnTheSweep: the expiry
// sweep reads the same row through approvalTx (sweepExpiredOne).
//
// Evidence: real store file, second real connection, in-process.
// Probing mutation (planned): leave approvalTx's scan error bare ⇒ red.
func TestV0151B_P2_6_scanLevelCorruptionIsEvidenceCorruptOnTheSweep(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	a, _ := pendingRequest(t, store, "act_scan_sweep")
	scanCorrupt(t, store, a.ApprovalID)
	_, _, err := store.SweepExpiredApprovals(context.Background(), a.ExpiresAt.Add(time.Hour))
	if !errors.Is(err, ErrApprovalEvidenceCorrupt) {
		t.Fatalf("sweep err = %v, want ErrApprovalEvidenceCorrupt", err)
	}
	if errors.Is(err, ErrApprovalUnreadable) {
		t.Fatalf("err = %v carries BOTH sentinels: corrupt and unreadable are exclusive (class i)", err)
	}
}

// TestV0151B_P2_6_aCancelledContextIsNeverCorruption: database/sql defers a
// query's error to Scan, so wrapping every Scan error as corruption publishes a
// cancelled request as permanent evidence damage. Each read door must answer
// the transient name with the cause kept.
//
// Evidence: real store file, in-process; the context is cancelled before the
// call, so the failure is forced, not raced.
// Probing mutation (planned): wrap GetApproval's scan error as corrupt
// unconditionally again ⇒ the get row reddens.
func TestV0151B_P2_6_aCancelledContextIsNeverCorruption(t *testing.T) {
	t.Parallel()
	doors := map[string]func(s *Store, a action.Approval) error{
		"get": func(s *Store, a action.Approval) error {
			_, _, err := s.GetApproval(cancelled(), a.ApprovalID)
			return err
		},
		"list": func(s *Store, _ action.Approval) error {
			_, err := s.ListApprovals(cancelled(), action.ApprovalPending)
			return err
		},
		"decide-approve": func(s *Store, a action.Approval) error {
			env, ident := operatorDecisionEnv("approve", a.ApprovalID)
			_, err := s.DecideApprovalUnderLaw(cancelled(), a.ApprovalID, action.DecisionApproved,
				a.RequestedAt.Add(time.Minute), env, ident, "", parkedLaw)
			return err
		},
		"decide-reject": func(s *Store, a action.Approval) error {
			env, ident := operatorDecisionEnv("reject", a.ApprovalID)
			_, err := s.DecideApprovalUnderLaw(cancelled(), a.ApprovalID, action.DecisionRejected,
				a.RequestedAt.Add(time.Minute), env, ident, "", parkedLaw)
			return err
		},
		// ADDED in the green step (diff pass #1, P2-a): the sweep's first
		// read, under a context cancelled before the call.
		"sweep": func(s *Store, a action.Approval) error {
			_, _, err := s.SweepExpiredApprovals(cancelled(), a.ExpiresAt.Add(time.Hour))
			return err
		},
		// The sweep's loop check: errOnlyCancelled's Done() is nil, so
		// database/sql and the driver never see it and the first read
		// succeeds; the loop's own ctx.Err() is the first to report it.
		"sweep-loop": func(s *Store, a action.Approval) error {
			_, _, err := s.SweepExpiredApprovals(errOnlyCancelled{context.Background()}, a.ExpiresAt.Add(time.Hour))
			// Instrument, by text on purpose: the row must reach the loop,
			// not the first read, and only the message tells the two apart.
			if err != nil && !strings.Contains(err.Error(), "expiry sweep interrupted") {
				return fmt.Errorf("instrument: the sweep failed before its loop: %w", err)
			}
			return err
		},
	}
	for name, door := range doors {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store, _ := sealedStore(t)
			a, _ := pendingRequest(t, store, "act_cancel_"+name)
			err := door(store, a)
			if errors.Is(err, ErrApprovalEvidenceCorrupt) {
				t.Fatalf("err = %v: a cancelled request published as corrupt evidence", err)
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("err = %v: the cause (context.Canceled) must survive in the chain", err)
			}
			if !errors.Is(err, ErrApprovalUnreadable) {
				t.Fatalf("err = %v, want ErrApprovalUnreadable — transient, by name", err)
			}
		})
	}
}

// The sealer's marks (director, 2026-09-19). Over an approval row it cannot
// use, a receipt carries an explicit mark — never "" (read as «no approval
// existed»), never a digest the row does not re-derive, never a refusal that
// would block a close, a recovery or a boot:
//
//   - corrupt:<code>    the row WAS read and is invalid;
//   - unreadable:<code> the store could not read it (a driver failure).
//
// <code> comes from the CLOSED list the pre-test paper fixes (§4):
// corrupt:row_scan, corrupt:decision_at, corrupt:decision_principal,
// corrupt:decision_verb, corrupt:approval_missing, corrupt:status,
// unreadable:driver. Never err.Error(), never a cell's bytes — every case of
// sealerCases asserts the EXACT mark.
//
// corrupt:decision_principal follows the repository's origin rule
// (judgeTombstoneOrigin): a HUMAN verb requires a principal, the CLOCK verb
// requires none; any other pairing is the mark.
//
// corrupt:approval_missing: the approval row is gone, or no longer decided,
// while a decision provably existed. The proof, per door (pre-test paper §1):
// the finish door decides from the `current` state it already reads (APPROVED
// ⇒ decided); the recovery decides from the OWNING PASS — closeCrashOrphan's
// UPDATE under claimedOrphanPredicate with RowsAffected == 1 proves, inside
// that one statement, that the action was an APPROVED claimed orphan (no prior
// SELECT: a read before the UPDATE turns the DEFERRED transaction into a
// reader that must promote, and under a concurrent real writer the UPDATE
// gets SQLITE_BUSY and the orphan is skipped —
// TestRecoveryVsFinish_acrossRealConnections watches that); the reject path
// leaves PENDING_APPROVAL through a decision (decided). NOT from the target
// state and NOT from a tombstone looked up by action_id. The shapes that must keep
// sealing "" are moulded in TestV0151B_P2_6_sister_aPlainActionSealsNoApprovalMark:
// a plain AUTHORIZED action finished SUCCEEDED and finished FAILED, a plain
// AUTHORIZED orphan and a plain crash orphan (a pre-AUTHORIZED state) at
// recovery, and a plain life reusing the action id of an earlier approved,
// pruned life, closed by the finish and by the recovery.
//
// A coherent rewrite of a decided row to another VALID instant, principal and
// verb re-derives a valid digest and stays undetectable here by design (R11).
var sealerCases = []struct {
	name, stmt, want string
}{
	{"scan", `UPDATE approvals SET policy_version = 'x' WHERE approval_id = ?`, "corrupt:row_scan"},
	{"parse", `UPDATE approvals SET decision_at = 'garbage' WHERE approval_id = ?`, "corrupt:decision_at"},
	// A DECIDED row whose decision instant is empty or NULL: today sealed as
	// the digest of a history at instant zero.
	{"decision-at-empty", `UPDATE approvals SET decision_at = '' WHERE approval_id = ?`, "corrupt:decision_at"},
	{"decision-at-null", `UPDATE approvals SET decision_at = NULL WHERE approval_id = ?`, "corrupt:decision_at"},
	// A DECIDED row with no decider, or a verb outside the set.
	{"decision-principal-empty", `UPDATE approvals SET decision_principal_id = '' WHERE approval_id = ?`, "corrupt:decision_principal"},
	{"decision-verb-unknown", `UPDATE approvals SET decision = 'bogus' WHERE approval_id = ?`, "corrupt:decision_verb"},
	// A CLOCK verb on an APPROVED row that carries its human principal: judged
	// by the verb, the principal is forbidden (a status-based cure passes it).
	{"clock-verb-on-approved", `UPDATE approvals SET decision = 'clock' WHERE approval_id = ?`, "corrupt:decision_principal"},
	// The approval that existed is gone or no longer decided.
	{"status-reset-pending", `UPDATE approvals SET status = 'PENDING' WHERE approval_id = ?`, "corrupt:approval_missing"},
	{"row-deleted", `DELETE FROM approvals WHERE approval_id = ?`, "corrupt:approval_missing"},
	{"status-unknown", `UPDATE approvals SET status = 'bogus' WHERE approval_id = ?`, "corrupt:status"},
	// Hostile bytes in the corrupt cells: the mark must not carry them.
	{"hostile-bytes", `UPDATE approvals SET policy_version = 'EVIL<script>', decision_at = 'EVIL' || char(8238) WHERE approval_id = ?`, "corrupt:row_scan"},
	// The column the sealer selects no longer exists: the statement cannot be
	// prepared, a driver failure, and no row was read.
	{"unreadable", `ALTER TABLE approvals RENAME COLUMN decision_at TO decision_at_gone -- ?`, "unreadable:driver"},
}

func attackMaybeArg(t *testing.T, store *Store, stmt, arg string) {
	t.Helper()
	if strings.Contains(stmt, "-- ?") {
		attack(t, store, strings.TrimSuffix(stmt, " -- ?"))
		return
	}
	attack(t, store, stmt, arg)
}

// approvedThenClaimed approves a parked request and claims its parameters, so
// the action stays APPROVED with its params purged: exactly the orphan
// RecoverPreviousLife's first pass closes as OUTCOME_UNKNOWN.
func approvedThenClaimed(t *testing.T, store *Store, id string) action.Approval {
	t.Helper()
	ctx := context.Background()
	a, _ := pendingRequest(t, store, id)
	env, ident := operatorDecisionEnv("approve", a.ApprovalID)
	if _, err := store.DecideApprovalUnderLaw(ctx, a.ApprovalID, action.DecisionApproved,
		a.RequestedAt.Add(time.Minute), env, ident, "", parkedLaw); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, _, err := store.ClaimApprovalParamsUnderDigest(ctx, a.ApprovalID, &parkedLaw, a.ActionDigest, nil); err != nil {
		t.Fatalf("claim: %v", err)
	}
	return a
}

func digestsOf(rs []action.Receipt) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.ApprovalDigest)
	}
	return out
}

// TestV0151B_P2_6_sister_theFinishSealsTheExactMark: the finish door. The
// close lands and its receipt carries the exact mark of its case.
//
// Evidence: real sealed store, row surgery through a second real connection,
// in-process.
// Probing mutation (planned): return "" on the scan error ⇒ /scan and
// /hostile-bytes red; drop parseNullTime's error ⇒ /parse red; accept an
// empty/NULL decision instant, principal or unknown verb ⇒ its row red; return
// "" on a driver failure ⇒ /unreadable red; leak err.Error() or cell bytes into
// one branch's code ⇒ that branch's rows red (each leak reddens only its own
// branch).
func TestV0151B_P2_6_sister_theFinishSealsTheExactMark(t *testing.T) {
	t.Parallel()
	for _, c := range sealerCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			store, _ := sealedStore(t)
			ctx := context.Background()
			id := "act_mark_finish_" + c.name
			a, _ := pendingRequest(t, store, id)
			env, ident := operatorDecisionEnv("approve", a.ApprovalID)
			if _, err := store.DecideApprovalUnderLaw(ctx, a.ApprovalID, action.DecisionApproved,
				a.RequestedAt.Add(time.Minute), env, ident, "", parkedLaw); err != nil {
				t.Fatalf("approve: %v", err)
			}
			attackMaybeArg(t, store, c.stmt, a.ApprovalID)
			if err := store.FinishWithResult(ctx, id, action.StateSucceeded,
				a.RequestedAt.Add(2*time.Minute), "sha256:result"); err != nil {
				t.Fatalf("finish err = %v: an unusable approval row must not block the close", err)
			}
			rs, _ := store.ReceiptsByAction(ctx, id)
			if len(rs) != 1 || rs[0].ApprovalDigest != c.want {
				t.Fatalf("receipt approval_digest = %q, want exactly %q", digestsOf(rs), c.want)
			}
		})
	}
}

// TestV0151B_P2_6_sister_theRecoverySealsTheExactMark: the recovery door
// (RecoverPreviousLife → closeCrashOrphan → receiptForFinish). Recovery runs
// at boot, so it must not fail either.
//
// Evidence: real sealed store, row surgery through a second real connection,
// in-process.
// Probing mutation (planned): cure only the finish door ⇒ every row red.
func TestV0151B_P2_6_sister_theRecoverySealsTheExactMark(t *testing.T) {
	t.Parallel()
	for _, c := range sealerCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			store, _ := sealedStore(t)
			ctx := context.Background()
			id := "act_mark_recover_" + c.name
			a := approvedThenClaimed(t, store, id)
			attackMaybeArg(t, store, c.stmt, a.ApprovalID)
			if _, err := store.RecoverPreviousLife(ctx); err != nil {
				t.Fatalf("recovery err = %v: an unusable approval row must never block recovery", err)
			}
			rs, _ := store.ReceiptsByAction(ctx, id)
			if len(rs) != 1 || rs[0].ApprovalDigest != c.want {
				t.Fatalf("recovered receipt approval_digest = %q, want exactly %q", digestsOf(rs), c.want)
			}
		})
	}
}

// TestV0151B_P2_6_aDriverFailureIsNeverCorruption: a context-only classifier
// passes the cancelled-context rows and still publishes a DRIVER failure as
// corrupt (the adversary's reproduction: a closed store → GetApproval
// «sql: database is closed» → corrupt). Two driver failures, each at the point
// it surfaces:
//
//   - closed: the store is closed. GetApproval fails at Scan (QueryRow defers
//     the error); the decide fails at BeginTx; the list at Query.
//   - dropped: the approvals table is dropped through a second connection. The
//     query cannot be prepared, and QueryRow defers that to Scan — so the
//     decide's approvalTx and GetApproval fail AT SCAN TIME with a driver error,
//     which is the case a context-only classifier mislabels.
//
// Evidence: real store file; the drop goes through a second real connection;
// in-process.
// Probing mutation (planned): classify by context only ⇒ the driver rows red.
func TestV0151B_P2_6_aDriverFailureIsNeverCorruption(t *testing.T) {
	t.Parallel()
	doors := map[string]func(s *Store, a action.Approval) error{
		"get": func(s *Store, a action.Approval) error {
			_, _, err := s.GetApproval(context.Background(), a.ApprovalID)
			return err
		},
		"list": func(s *Store, _ action.Approval) error {
			_, err := s.ListApprovals(context.Background(), action.ApprovalPending)
			return err
		},
		"decide-approve": func(s *Store, a action.Approval) error {
			env, ident := operatorDecisionEnv("approve", a.ApprovalID)
			_, err := s.DecideApprovalUnderLaw(context.Background(), a.ApprovalID, action.DecisionApproved,
				a.RequestedAt.Add(time.Minute), env, ident, "", parkedLaw)
			return err
		},
		"decide-reject": func(s *Store, a action.Approval) error {
			env, ident := operatorDecisionEnv("reject", a.ApprovalID)
			_, err := s.DecideApprovalUnderLaw(context.Background(), a.ApprovalID, action.DecisionRejected,
				a.RequestedAt.Add(time.Minute), env, ident, "", parkedLaw)
			return err
		},
		// ADDED in the green step (diff pass #1, P2-a): the sweep's first read.
		"sweep": func(s *Store, a action.Approval) error {
			_, _, err := s.SweepExpiredApprovals(context.Background(), a.ExpiresAt.Add(time.Hour))
			return err
		},
	}
	breaks := map[string]func(t *testing.T, s *Store){
		"closed": func(t *testing.T, s *Store) {
			if err := s.Close(); err != nil {
				t.Fatalf("close: %v", err)
			}
		},
		"dropped": func(t *testing.T, s *Store) {
			attack(t, s, `DROP TABLE approvals`)
		},
	}
	for bname, brk := range breaks {
		for dname, door := range doors {
			t.Run(bname+"/"+dname, func(t *testing.T) {
				t.Parallel()
				store, _ := sealedStore(t)
				a, _ := pendingRequest(t, store, "act_drv_"+bname+"_"+dname)
				brk(t, store)
				err := door(store, a)
				if err == nil {
					t.Fatal("instrument: the broken store answered nil")
				}
				if errors.Is(err, ErrApprovalEvidenceCorrupt) {
					t.Fatalf("err = %v: a driver failure published as corrupt evidence", err)
				}
				if !errors.Is(err, ErrApprovalUnreadable) {
					t.Fatalf("err = %v, want ErrApprovalUnreadable", err)
				}
			})
		}
	}
}

// TestV0151B_P2_6_sister_theRejectPathSealsTheExactMark: the THIRD sealing door.
// rejectParkedActionTx → receiptForFinish → approvalDigestTx seals the parked
// action's REJECTED receipt on three paths: the decide-reject (a HUMAN verb),
// and the expiry touch and the sweep (the CLOCK verb). A trigger damages the
// approval row the moment its status leaves PENDING — inside the closing
// transaction, before the seal reads it — including DELETING the row or
// resetting it to PENDING (corrupt:approval_missing). The clock rows are
// moulded both ways under the origin rule: untouched, they seal the approval's
// real digest (the control); with a principal forged onto a clock decision,
// they seal corrupt:decision_principal. The decide-cancel path is the
// CANCELLED control of corrupt:status (reachable only through the exported
// DecideApprovalUnderLaw today: no production caller passes
// DecisionCancelled). unreadable:driver has NO committed-receipt mould on this
// door: its branch IS reachable (a zeroed index page of approvals makes the
// sealer's read answer «database disk image is malformed»), but with every
// placement tried the close then fails at the receipt INSERT and nothing is
// committed (receipts = 0); SQLITE_BUSY was not placeable at the sealer's
// read. The exact cause of that INSERT failure is not established.
//
// Probing mutations EXECUTED (by the adversary, pass #7, on a correct cure):
// M7 — rejectParkedActionTx passes decided=false ⇒ exactly the 8
// approval_missing rows red; Mcancel — a CANCELLED row read as an unknown
// status ⇒ exactly seven decide-cancel rows red — the control, decision_at ×3,
// the unknown verb, human-without-principal and clock-verb-with-principal —
// while row_scan, row-deleted, status-reset-pending and status-unknown stay
// green (as executed by the adversary, pass #10, on a correct cure).
//
// Evidence: real sealed store, trigger installed through a second real
// connection, in-process.
// Probing mutation (planned): cure only the finish and recovery doors ⇒ the
// damage rows red; mark every clock row with an empty principal ⇒ the controls
// red.
func TestV0151B_P2_6_sister_theRejectPathSealsTheExactMark(t *testing.T) {
	t.Parallel()
	paths := map[string]struct {
		clock bool
		run   func(s *Store, a action.Approval) error
	}{
		"decide-reject": {false, func(s *Store, a action.Approval) error {
			env, ident := operatorDecisionEnv("reject", a.ApprovalID)
			_, err := s.DecideApprovalUnderLaw(context.Background(), a.ApprovalID, action.DecisionRejected,
				a.RequestedAt.Add(time.Minute), env, ident, "", parkedLaw)
			return err
		}},
		"expiry-touch": {true, func(s *Store, a action.Approval) error {
			env, ident := operatorDecisionEnv("approve", a.ApprovalID)
			_, err := s.DecideApprovalUnderLaw(context.Background(), a.ApprovalID, action.DecisionApproved,
				a.ExpiresAt.Add(time.Minute), env, ident, "", parkedLaw)
			return err
		}},
		"decide-cancel": {false, func(s *Store, a action.Approval) error {
			env, ident := operatorDecisionEnv("cancel", a.ApprovalID)
			_, err := s.DecideApprovalUnderLaw(context.Background(), a.ApprovalID, action.DecisionCancelled,
				a.RequestedAt.Add(time.Minute), env, ident, "", parkedLaw)
			return err
		}},
		"sweep": {true, func(s *Store, a action.Approval) error {
			_, _, err := s.SweepExpiredApprovals(context.Background(), a.ExpiresAt.Add(time.Minute))
			return err
		}},
	}
	damages := []struct {
		name, body, want string
		clockOnly        bool
		humanOnly        bool
	}{
		{"control", "", "", false, false},
		{"row_scan", `UPDATE approvals SET policy_version = 'x' WHERE approval_id = NEW.approval_id;`, "corrupt:row_scan", false, false},
		{"status-unknown", `UPDATE approvals SET status = 'bogus' WHERE approval_id = NEW.approval_id;`, "corrupt:status", false, false},
		{"clock-with-principal", `UPDATE approvals SET decision_principal_id = 'principal_forged' WHERE approval_id = NEW.approval_id;`, "corrupt:decision_principal", true, false},
		{"row-deleted", `DELETE FROM approvals WHERE approval_id = NEW.approval_id;`, "corrupt:approval_missing", false, false},
		{"status-reset-pending", `UPDATE approvals SET status = 'PENDING' WHERE approval_id = NEW.approval_id;`, "corrupt:approval_missing", false, false},
		// The rest of the closed list on ALL FOUR paths: the wrong cures
		// «validate only APPROVED rows» (human paths) and «skip decision_at /
		// verb validation when status is EXPIRED» (clock paths, the adversary's
		// M6 and M6-verb) passed every earlier mould. The principal row stays
		// on the human paths only: a clock decision legitimately has NO
		// principal (the origin rule), and its forged-principal case is
		// clock-with-principal.
		{"decision_at-garbage", `UPDATE approvals SET decision_at = 'garbage' WHERE approval_id = NEW.approval_id;`, "corrupt:decision_at", false, false},
		{"decision_at-empty", `UPDATE approvals SET decision_at = '' WHERE approval_id = NEW.approval_id;`, "corrupt:decision_at", false, false},
		{"decision_at-null", `UPDATE approvals SET decision_at = NULL WHERE approval_id = NEW.approval_id;`, "corrupt:decision_at", false, false},
		{"decision-verb-unknown", `UPDATE approvals SET decision = 'bogus' WHERE approval_id = NEW.approval_id;`, "corrupt:decision_verb", false, false},
		{"human-without-principal", `UPDATE approvals SET decision_principal_id = '' WHERE approval_id = NEW.approval_id;`, "corrupt:decision_principal", false, true},
		// Cross-pairings: the principal is judged by the VERB, never by the
		// status. A cure testing `(status == EXPIRED) != (principal == "")`
		// passes every other damage row of this table and reddens these two.
		{"human-verb-on-clock-row", `UPDATE approvals SET decision = 'rejected' WHERE approval_id = NEW.approval_id;`, "corrupt:decision_principal", true, false},
		{"clock-verb-with-principal", `UPDATE approvals SET decision = 'clock' WHERE approval_id = NEW.approval_id;`, "corrupt:decision_principal", false, true},
	}
	for pname, path := range paths {
		for _, d := range damages {
			if (d.clockOnly && !path.clock) || (d.humanOnly && path.clock) {
				continue
			}
			t.Run(pname+"/"+d.name, func(t *testing.T) {
				t.Parallel()
				store, _ := sealedStore(t)
				id := "act_mark_reject_" + pname + "_" + d.name
				a, _ := pendingRequest(t, store, id)
				if d.body != "" {
					attack(t, store, `CREATE TRIGGER damage_on_close AFTER UPDATE OF status ON approvals
					  WHEN NEW.approval_id = '`+a.ApprovalID+`' AND OLD.status = 'PENDING' AND NEW.status != 'PENDING'
					  BEGIN `+d.body+` END`) // #nosec G202 -- test-owned literal
				}
				if err := path.run(store, a); err != nil {
					t.Fatalf("%s err = %v: an unusable approval row must not block the close", pname, err)
				}
				rs, _ := store.ReceiptsByAction(context.Background(), id)
				if len(rs) != 1 {
					t.Fatalf("instrument: one REJECTED receipt expected, got %d", len(rs))
				}
				if d.want == "" {
					consumed, _, err := store.GetApproval(context.Background(), a.ApprovalID)
					if err != nil {
						t.Fatalf("control: re-read: %v", err)
					}
					if rs[0].ApprovalDigest != consumed.Digest() {
						t.Fatalf("control: approval_digest = %q, want the approval's own digest %q", rs[0].ApprovalDigest, consumed.Digest())
					}
					return
				}
				if rs[0].ApprovalDigest != d.want {
					t.Fatalf("REJECTED receipt approval_digest = %q, want exactly %q", rs[0].ApprovalDigest, d.want)
				}
			})
		}
	}
}

// TestV0151B_P2_6_sister_aPlainActionSealsNoApprovalMark: the inverse half. An
// action with no approval in its life keeps approval_digest "" in six shapes:
// finished SUCCEEDED, finished FAILED, an AUTHORIZED orphan and a crash orphan
// at recovery, and a plain life reusing an earlier approved life's action id,
// closed by the finish and by the recovery.
// GREEN today, by design: it guards the cure against deriving «a decision
// existed» from the target state, from the recovery pass, or from a tombstone
// looked up by action_id. Its probing mutations (M2b, M2c, `current ==
// APPROVED || to == FAILED`, a tombstone lookup by action_id) are planned for
// the green step, where the decided flag exists.
//
// Evidence: real sealed store, state and prune surgery through a second real
// connection, in-process.
func TestV0151B_P2_6_sister_aPlainActionSealsNoApprovalMark(t *testing.T) {
	t.Parallel()
	shapes := map[string]func(t *testing.T, s *Store, id string){
		"finish-succeeded": func(t *testing.T, s *Store, id string) {
			v0151bRecordPlain(t, s, id)
			if err := s.FinishWithResult(context.Background(), id, action.StateSucceeded, time.Now().UTC(), "sha256:r"); err != nil {
				t.Fatalf("finish: %v", err)
			}
		},
		// The mutation `current == APPROVED || to == FAILED` reddens this row.
		"finish-failed": func(t *testing.T, s *Store, id string) {
			v0151bRecordPlain(t, s, id)
			if err := s.FinishWithResult(context.Background(), id, action.StateFailed, time.Now().UTC(), ""); err != nil {
				t.Fatalf("finish: %v", err)
			}
		},
		"recovery-authorized-orphan": func(t *testing.T, s *Store, id string) {
			v0151bRecordPlain(t, s, id)
			v0151bRecoverNow(t, s)
		},
		// A crash orphan: a pre-AUTHORIZED state an older life could leave (the
		// recovery_legacy_test.go placement). M2c — decided on every pass but
		// the claimed-orphan one — reddens this row.
		"recovery-crash-orphan": func(t *testing.T, s *Store, id string) {
			v0151bRecordPlain(t, s, id)
			attack(t, s, `UPDATE actions SET state = 'PREPARING' WHERE action_id = ?`, id)
			v0151bRecoverNow(t, s)
		},
		// The same reused id, closed by the RECOVERY instead of the finish: a
		// closeCrashOrphan deriving decided from a tombstone count by
		// action_id passes every other mould.
		"reused-action-id-recovery": func(t *testing.T, s *Store, id string) {
			a, _ := pendingRequest(t, s, id)
			env, ident := operatorDecisionEnv("approve", a.ApprovalID)
			if _, err := s.DecideApprovalUnderLaw(context.Background(), a.ApprovalID, action.DecisionApproved,
				a.RequestedAt.Add(time.Minute), env, ident, "", parkedLaw); err != nil {
				t.Fatalf("approve the earlier life: %v", err)
			}
			if err := s.FinishWithResult(context.Background(), id, action.StateSucceeded, a.RequestedAt.Add(2*time.Minute), "sha256:r"); err != nil {
				t.Fatalf("finish the earlier life: %v", err)
			}
			attack(t, s, `DELETE FROM actions WHERE action_id = ?`, id)
			v0151bRecordPlain(t, s, id)
			v0151bRecoverNow(t, s)
		},
		// A plain life reusing the action id of an earlier APPROVED life whose
		// rows the cascade pruned (its tombstone survives). A cure that looks a
		// tombstone up by action_id misattributes the old decision to this life.
		"reused-action-id": func(t *testing.T, s *Store, id string) {
			a, _ := pendingRequest(t, s, id)
			env, ident := operatorDecisionEnv("approve", a.ApprovalID)
			if _, err := s.DecideApprovalUnderLaw(context.Background(), a.ApprovalID, action.DecisionApproved,
				a.RequestedAt.Add(time.Minute), env, ident, "", parkedLaw); err != nil {
				t.Fatalf("approve the earlier life: %v", err)
			}
			if err := s.FinishWithResult(context.Background(), id, action.StateSucceeded, a.RequestedAt.Add(2*time.Minute), "sha256:r"); err != nil {
				t.Fatalf("finish the earlier life: %v", err)
			}
			attack(t, s, `DELETE FROM actions WHERE action_id = ?`, id)
			v0151bRecordPlain(t, s, id)
			if err := s.FinishWithResult(context.Background(), id, action.StateSucceeded, time.Now().UTC(), "sha256:r2"); err != nil {
				t.Fatalf("finish the plain life: %v", err)
			}
		},
	}
	for name, run := range shapes {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store, _ := sealedStore(t)
			id := "act_plain_" + name
			run(t, store, id)
			rs, _ := store.ReceiptsByAction(context.Background(), id)
			if len(rs) == 0 {
				t.Fatalf("instrument: no receipt for the plain action")
			}
			last := rs[len(rs)-1]
			if last.ApprovalDigest != "" {
				t.Fatalf("plain action receipt approval_digest = %q, want \"\" (no approval in this life)", last.ApprovalDigest)
			}
		})
	}
}

func v0151bRecordPlain(t *testing.T, s *Store, id string) {
	t.Helper()
	if err := s.RecordAttempt(context.Background(), testEnvelope(id),
		Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized); err != nil {
		t.Fatalf("record: %v", err)
	}
}

func v0151bRecoverNow(t *testing.T, s *Store) {
	t.Helper()
	if _, err := s.RecoverPreviousLife(context.Background()); err != nil {
		t.Fatalf("recovery: %v", err)
	}
}

// TestV0151B_P2_6_sister_theRecoverySealsTheRealDigestWhenIntact: the recovery
// × control cell. An approved and claimed orphan whose approval row is INTACT
// is closed by the recovery with the approval's REAL digest — never a mark,
// never "". GREEN today, by design; its probing mutation (blank the digest at
// closeCrashOrphan) is EXECUTED in the red phase (pre-test paper §5).
//
// Evidence: real sealed store, in-process.
func TestV0151B_P2_6_sister_theRecoverySealsTheRealDigestWhenIntact(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	id := "act_recover_intact"
	a := approvedThenClaimed(t, store, id)
	v0151bRecoverNow(t, store)
	rs, _ := store.ReceiptsByAction(ctx, id)
	if len(rs) != 1 {
		t.Fatalf("instrument: one recovered receipt expected, got %d", len(rs))
	}
	consumed, _, err := store.GetApproval(ctx, a.ApprovalID)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if rs[0].ApprovalDigest != consumed.Digest() {
		t.Fatalf("recovered receipt approval_digest = %q, want the approval's real digest %q", rs[0].ApprovalDigest, consumed.Digest())
	}
}

// TestV0151B_P2_6_sister_theClaimedPassClosesUnderAConcurrentWriter watches the
// decided wire on the pass where `decided` lives: RecoverPreviousLife's
// claimed-orphan pass (claimedOrphanPredicate). The interleaving is FORCED and
// OBSERVED, not hoped for:
//
//  1. a second real connection opens BEGIN IMMEDIATE and writes, holding the
//     write lock BEFORE the recovery starts;
//  2. a SIBLING connection is parked in SQLite's busy wait on the same lock
//     BEFORE the recovery starts (a foreign busy waiter the observation must
//     not mistake for the recovery);
//  3. the recovery starts on a goroutine that records its own goroutine id;
//     the mould polls the goroutine stacks until THAT goroutine's stack holds
//     SQLite's busy-wait frame (_sqliteDefaultBusyCallback, reached only when
//     the close's transaction holds no read snapshot yet), or the recovery has
//     returned;
//  4. only then the writer commits.
//
// With the correct wire (the close's first statement is its UPDATE) the close
// waits in the busy handler, the writer commits, and every orphan closes. With
// a read of the action's state before the UPDATE, the transaction already holds
// a snapshot, SQLite answers SQLITE_BUSY WITHOUT calling the busy handler, and
// every close is counted skipped. Six APPROVED and claimed orphans; all six
// must close (OUTCOME_UNKNOWN, one receipt carrying its approval's sha256
// digest) with zero skipped. The approved TestRecoveryVsFinish_acrossRealConnections
// (R5-S5) seeds only AUTHORIZED rows and watches the other pass.
//
// Evidence: two real connections on one file (the store's pool and a raw
// *sql.Conn holding BEGIN IMMEDIATE), in-process; the observation reads the
// runtime's goroutine stacks (runtime.Stack), not the clock.
// Probing mutations EXECUTED: a SELECT of the state before closeCrashOrphan's
// UPDATE, conditioned on the claimed pass ⇒ red 30/30, with and without -race
// (green phase, m40b). The instrument's own mutation — accept a busy frame on
// ANY goroutine, with that same SELECT in place — lets the sibling's frame
// release the writer before the recovery is observed, and the production
// mutation then escaped 7 of 30 runs under -race and 0 of 30 without (m41b):
// a loose observation makes this mould depend on scheduling, which is what the
// recovery-tied observation exists to prevent.
func TestV0151B_P2_6_sister_theClaimedPassClosesUnderAConcurrentWriter(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	const n = 6
	for i := 0; i < n; i++ {
		approvedThenClaimed(t, store, fmt.Sprintf("act_claimed_race_%d", i))
	}
	v0151bRecordPlain(t, store, "act_writer_race")
	v0151bRecordPlain(t, store, "act_sibling_race")

	conn, err := secondConn(t, store).Conn(ctx)
	if err != nil {
		t.Fatalf("writer connection: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		t.Fatalf("writer BEGIN IMMEDIATE: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE actions SET finished_at = ? WHERE action_id = 'act_writer_race'`,
		time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("writer write: %v", err)
	}

	// The sibling: a third connection whose UPDATE waits for the same lock.
	sibling, err := secondConn(t, store).Conn(ctx)
	if err != nil {
		t.Fatalf("sibling connection: %v", err)
	}
	defer func() { _ = sibling.Close() }()
	siblingDone := make(chan struct{})
	go func() {
		defer close(siblingDone)
		_, _ = sibling.ExecContext(ctx, `UPDATE actions SET finished_at = ? WHERE action_id = 'act_sibling_race'`,
			time.Now().UTC().Format(time.RFC3339Nano))
	}()
	if !waitForBusyFrame(t, func(string) bool { return true }, nil) {
		t.Fatal("instrument: the sibling was never observed in the busy wait")
	}

	type result struct {
		skipped int
		err     error
	}
	done := make(chan result, 1)
	gid := make(chan string, 1)
	go func() {
		gid <- currentGoroutineID()
		skipped, err := store.RecoverPreviousLife(ctx)
		done <- result{skipped, err}
	}()
	recoveryID := <-gid

	// Observe: the RECOVERY's own goroutine blocked in the busy wait, or the
	// recovery already returned.
	var res *result
	observed := waitForBusyFrame(t, func(id string) bool { return id == recoveryID }, func() bool {
		select {
		case r := <-done:
			res = &r
			return true
		default:
			return false
		}
	})
	if !observed && res == nil {
		t.Fatal("instrument: the recovery was neither observed in the busy wait nor returned before the deadline")
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		t.Fatalf("writer COMMIT: %v", err)
	}
	if res == nil {
		r := <-done
		res = &r
	}
	<-siblingDone
	if res.err != nil {
		t.Fatalf("recovery err = %v", res.err)
	}
	if res.skipped != 0 {
		t.Fatalf("recovery skipped %d row(s), want 0 — the claimed pass must close under a concurrent writer", res.skipped)
	}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("act_claimed_race_%d", i)
		rec, err := store.Get(ctx, id)
		if err != nil || rec.State != action.StateOutcomeUnknown {
			t.Fatalf("%s: claimed orphan not closed: %v %v", id, err, rec.State)
		}
		rs, _ := store.ReceiptsByAction(ctx, id)
		if len(rs) != 1 || !strings.HasPrefix(rs[0].ApprovalDigest, "sha256:") {
			t.Fatalf("%s: receipts %v, want exactly one sealing the approval's sha256 digest", id, digestsOf(rs))
		}
	}
}

// currentGoroutineID reads the calling goroutine's id from its own stack
// header ("goroutine N [running]:").
func currentGoroutineID() string {
	buf := make([]byte, 64)
	buf = buf[:runtime.Stack(buf, false)]
	f := strings.Fields(string(buf))
	if len(f) < 2 {
		return ""
	}
	return f[1]
}

// waitForBusyFrame polls every goroutine's stack until one whose id `match`
// accepts holds SQLite's busy-wait frame (true), or `stop` reports the watched
// work finished (false), or the deadline — under the store's 5 s busy_timeout —
// passes (false). It observes the runtime; it never sleeps a fixed time.
func waitForBusyFrame(t *testing.T, match func(id string) bool, stop func() bool) bool {
	t.Helper()
	buf := make([]byte, 1<<22)
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if stop != nil && stop() {
			return false
		}
		for _, g := range strings.Split(string(buf[:runtime.Stack(buf, true)]), "\n\n") {
			if !strings.HasPrefix(g, "goroutine ") || !strings.Contains(g, "_sqliteDefaultBusyCallback") {
				continue
			}
			if f := strings.Fields(g); len(f) > 1 && match(f[1]) {
				return true
			}
		}
		runtime.Gosched()
	}
	return false
}

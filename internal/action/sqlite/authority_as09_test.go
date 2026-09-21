// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// TestAuthority_StartDebitAndApprovalClaimAreAtomic (AS-AUTH-09) interrupts an
// approved start at the EXACT point where the debits are already written and
// nothing is committed yet, and demands that the interruption undoes all of it:
// the probe's own error comes back — not merely "an error" — and afterwards
// there is no debit, no durable start, the action is still APPROVED, and the
// parked parameters are still there to be claimed by a later, successful start.
//
// Evidence level: in-process, one real SQLite store; the interruption is an
// in-transaction probe, NOT a crash — the crash is AS-AUTH-11's.
// Probing mutation executed: commit the transaction right before the probe, so
// the debits are durable when the interruption comes — red on «budget debits =
// 4, want 0», and the approval could no longer start afterwards.
func TestAuthority_StartDebitAndApprovalClaimAreAtomic(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 2)
	parked := parkStrictAuthorityFixture(t, f)
	a := approveStrictAuthorityFixture(t, f, parked)
	law := PolicyPin{Version: 1, Digest: "sha256:authority-law"}

	errProbe := errors.New("probe: interrupted after the debit, before the commit")
	reached := false
	probe := func(point AuthorityStartProbe) error {
		if point == AuthorityProbeAfterDebitBeforeCommit {
			reached = true
			return errProbe
		}
		return nil
	}
	_, err := f.store.startApprovedAuthorization(context.Background(), a.ApprovalID, law, a.ActionDigest,
		f.now.Add(2*time.Second), probe)
	if !errors.Is(err, errProbe) {
		t.Errorf("error = %v, want the probe's own error", err)
	}
	if !reached {
		t.Fatal("the after-debit probe was never reached: this mould interrupted nothing")
	}
	for what, query := range map[string]string{
		"budget debits":  `SELECT COUNT(*) FROM budget_debits`,
		"durable starts": `SELECT COUNT(*) FROM authorization_starts`,
	} {
		if n := authorityScalar(t, f.store, query); n != 0 {
			t.Errorf("%s = %d, want 0: the interrupted start committed part of itself", what, n)
		}
	}
	record, err := f.store.Get(context.Background(), parked.ActionID)
	if err != nil || record.State != action.StateApproved {
		t.Errorf("action state after the interrupted start = %v, %v; want %v", record.State, err, action.StateApproved)
	}
	params, err := f.store.ApprovalParams(context.Background(), a.ApprovalID)
	if err != nil || string(params) != `{}` {
		t.Errorf("parked parameters = %q, %v; want them intact", params, err)
	}
	// And the interruption cost nothing: the same approval still starts.
	if _, err := f.store.StartApprovedAuthorization(context.Background(), a.ApprovalID, law, a.ActionDigest,
		f.now.Add(3*time.Second)); err != nil {
		t.Errorf("the approval could not start after a rolled-back attempt: %v", err)
	}
}

// TestAuthority_ApprovedStartPurgeIsInsideTheStart attacks the OTHER half of
// FR-AUTH-11: the purge of the parked parameters belongs to the same
// transaction as the debits and the start proof. The mould above interrupts
// BEFORE the purge, so what it says about the parameters is said of a purge
// that had not happened yet (the adversary's pass over this phase, F3). Here
// the purge itself is the point of attack, from both sides:
//
//   - the purge CANNOT happen: a trigger aborts exactly that UPDATE. An oracle
//     by impossibility — if the purge is part of the start, a purge that cannot
//     be written means a start that did not happen: no debit, no start proof,
//     the action still APPROVED, the parameters still parked;
//   - the purge HAS happened and the commit has not: interrupted at the
//     before-commit probe, the purge must be undone with everything else.
//
// Both rows end by starting the same approval successfully, so neither refusal
// cost the operator anything.
//
// Evidence level: in-process, one real SQLite store; an abort trigger and an
// in-transaction probe, NOT a crash.
// Probing mutations executed, each alone: (1) the adversary's M1 verbatim —
// the in-transaction purge and its two belts deleted, the purge done through
// the pool AFTER the commit — red on the trigger row with «budget debits = 4,
// want 0» and «durable starts = 1, want 0»; (2) commit right before the
// before-commit probe — red on the probe row with the same two figures and
// «parked parameters = "", want them intact».
func TestAuthority_ApprovedStartPurgeIsInsideTheStart(t *testing.T) {
	law := PolicyPin{Version: 1, Digest: "sha256:authority-law"}
	nothingHappened := func(t *testing.T, f authoritySQLiteFixture, parked AuthorityPendingResult, a action.Approval) {
		t.Helper()
		for what, query := range map[string]string{
			"budget debits":  `SELECT COUNT(*) FROM budget_debits`,
			"durable starts": `SELECT COUNT(*) FROM authorization_starts`,
		} {
			if n := authorityScalar(t, f.store, query); n != 0 {
				t.Errorf("%s = %d, want 0: the start committed without its purge", what, n)
			}
		}
		record, err := f.store.Get(context.Background(), parked.ActionID)
		if err != nil || record.State != action.StateApproved {
			t.Errorf("action state = %v, %v; want %v", record.State, err, action.StateApproved)
		}
		params, err := f.store.ApprovalParams(context.Background(), a.ApprovalID)
		if err != nil || string(params) != `{}` {
			t.Errorf("parked parameters = %q, %v; want them intact", params, err)
		}
	}

	t.Run("a purge that cannot be written is a start that did not happen", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		parked := parkStrictAuthorityFixture(t, f)
		a := approveStrictAuthorityFixture(t, f, parked)
		const refusal = "probe: the purge is refused"
		if _, err := f.store.db.Exec(`CREATE TRIGGER probe_refuse_purge BEFORE UPDATE OF canonical_params ON approvals
			WHEN NEW.canonical_params='' BEGIN SELECT RAISE(ABORT,'` + refusal + `'); END`); err != nil {
			t.Fatal(err)
		}
		_, err := f.store.StartApprovedAuthorization(context.Background(), a.ApprovalID, law, a.ActionDigest,
			f.now.Add(2*time.Second))
		if err == nil || !strings.Contains(err.Error(), refusal) {
			t.Errorf("error = %v, want the trigger's own refusal %q", err, refusal)
		}
		nothingHappened(t, f, parked, a)
		if _, err := f.store.db.Exec(`DROP TRIGGER probe_refuse_purge`); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.StartApprovedAuthorization(context.Background(), a.ApprovalID, law, a.ActionDigest,
			f.now.Add(3*time.Second)); err != nil {
			t.Errorf("the approval could not start once the purge could be written: %v", err)
		}
	})

	t.Run("a purge already written is undone with the rest", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		parked := parkStrictAuthorityFixture(t, f)
		a := approveStrictAuthorityFixture(t, f, parked)
		errProbe := errors.New("probe: interrupted after the purge, before the commit")
		reached := false
		_, err := f.store.startApprovedAuthorization(context.Background(), a.ApprovalID, law, a.ActionDigest,
			f.now.Add(2*time.Second), func(point AuthorityStartProbe) error {
				if point == AuthorityProbeBeforeCommit {
					reached = true
					return errProbe
				}
				return nil
			})
		if !errors.Is(err, errProbe) {
			t.Errorf("error = %v, want the probe's own error", err)
		}
		if !reached {
			t.Fatal("the before-commit probe was never reached: this row interrupted nothing")
		}
		nothingHappened(t, f, parked, a)
		if _, err := f.store.StartApprovedAuthorization(context.Background(), a.ApprovalID, law, a.ActionDigest,
			f.now.Add(3*time.Second)); err != nil {
			t.Errorf("the approval could not start after a rolled-back attempt: %v", err)
		}
	})
}

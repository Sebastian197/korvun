// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
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

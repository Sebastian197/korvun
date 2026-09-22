// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/identity"
)

// TestAuthority_ApprovedResumeJudgesFreshnessAtTheParkAndLivenessAtTheStart
// pins the TWO instants of the strict approved resume, in both directions, and
// the reason they are two.
//
// The ingress capability proves WHO ASKED. That question was answered when the
// request was parked, and the signed pending snapshot records the very instant
// it was answered, so its expiry is judged THERE. Judging it at the resume made
// a strict approval unstartable five minutes after its birth against a
// one-hour approval window — production's numbers (`internal/app/identity.go`
// and `defaultApprovalTTL`) — which is what the adversary's pass found (F6) and
// what the director adjudicated on 2026-09-22.
//
// Everything that can CHANGE while the human decides is still judged at the
// start: a principal disabled in between, a binding revoked or advanced. Those
// are not about who asked; they are about who may act now.
//
// Three rows, and the third is why the first is safe: the park instant is read
// from the SIGNED snapshot, so a writer with the database and without the key
// cannot move it.
//
// Evidence level: in-process, one real SQLite store; the disable goes through
// the store's signed DisablePrincipal door, the snapshot rewrite through SQL.
// Probing mutations executed, each alone: (1) freshness judged at the resume
// again — red on the first row; (2) the disabled principal judged at the park
// instead — red on the second row.
func TestAuthority_ApprovedResumeJudgesFreshnessAtTheParkAndLivenessAtTheStart(t *testing.T) {
	law := PolicyPin{Version: 1, Digest: "sha256:authority-law"}
	// The fixture's ingress capability lives ONE minute. Fifty minutes later is
	// far past it, and harsher than production's five against sixty.
	const long = 50 * time.Minute

	t.Run("the human decided long after the capability died: it still starts", func(t *testing.T) {
		f, approval := approvedAuthorityResumeFixture(t)
		started, err := f.store.StartApprovedAuthorization(context.Background(), approval.ApprovalID,
			law, approval.ActionDigest, f.now.Add(long))
		if err != nil {
			t.Fatalf("resume %v after the park: %v", long, err)
		}
		if started.ActionID != approval.ActionID || string(started.Params) != `{}` {
			t.Errorf("started = %#v, want the parked action and its parked bytes", started)
		}
		if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM authorization_starts WHERE action_id=?`, approval.ActionID); n != 1 {
			t.Errorf("durable starts = %d, want 1", n)
		}
		if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM budget_debits WHERE action_id=?`, approval.ActionID); n != 4 {
			t.Errorf("budget debits = %d, want the four of one committed start", n)
		}
	})

	t.Run("a principal disabled while the human decided: nothing starts", func(t *testing.T) {
		f, approval := approvedAuthorityResumeFixture(t)
		// Disabled AFTER the park and BEFORE the resume: the actor may no
		// longer act, whatever the capability once proved.
		if err := f.store.DisablePrincipal(context.Background(), "principal_brain_alpha", f.now.Add(time.Minute)); err != nil {
			t.Fatalf("disable the actor: %v", err)
		}
		_, err := f.store.StartApprovedAuthorization(context.Background(), approval.ApprovalID,
			law, approval.ActionDigest, f.now.Add(2*time.Minute))
		if !errors.Is(err, identity.ErrPrincipalDisabled) {
			t.Errorf("error = %v, want %v", err, identity.ErrPrincipalDisabled)
		}
		for what, query := range map[string]string{
			"budget debits":  `SELECT COUNT(*) FROM budget_debits`,
			"durable starts": `SELECT COUNT(*) FROM authorization_starts`,
		} {
			if n := authorityScalar(t, f.store, query); n != 0 {
				t.Errorf("%s = %d, want 0", what, n)
			}
		}
		if params, err := f.store.ApprovalParams(context.Background(), approval.ApprovalID); err != nil || string(params) != `{}` {
			t.Errorf("parked parameters = %q, %v; want them retained", params, err)
		}
	})

	t.Run("the park instant cannot be moved by a database writer", func(t *testing.T) {
		f, approval := approvedAuthorityResumeFixture(t)
		if _, err := f.store.db.Exec(`UPDATE authorization_snapshots SET recorded_at=? WHERE approval_id=?`,
			f.now.Add(24*time.Hour).UTC().Format(time.RFC3339Nano), approval.ApprovalID); err != nil {
			t.Fatal(err)
		}
		_, err := f.store.StartApprovedAuthorization(context.Background(), approval.ApprovalID,
			law, approval.ActionDigest, f.now.Add(long))
		if !errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
			t.Errorf("error = %v, want %v", err, ErrAuthorizationSnapshotCorrupt)
		}
		if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM budget_debits`); n != 0 {
			t.Errorf("budget debits = %d, want 0", n)
		}
	})
}

var _ = action.StateApproved

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// X1 — handed from v0.15.1 block A to block B (director, 2026-09-19).
//
// Taken into block B's tree BEFORE block A merges (coordinator's go,
// 2026-09-19). A's helper approvedForClaim lives only in A's test files, so
// this file carries a minimal copy under a DISTINCT name, x1ApprovedForClaim,
// that cannot collide after the rebase (declared in the pre-test paper). It
// also uses, without redeclaring: sealedStore (ledger_test.go),
// claimAttackConn (approvals_claim_authority_test.go), boundPark
// (approvals_r4f2_test.go), operatorDecisionEnv (approvals_test.go).
//
// Evidence level, honest: multiple real database connections. In-process.
package sqlite

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// x1ApprovedForClaim is B's minimal copy of block A's approvedForClaim: a
// request parked through the real door (boundPark) and APPROVED, params still
// held, so the claim is the next legitimate act. Distinct name on purpose.
func x1ApprovedForClaim(t *testing.T, store *Store, id string) (approvalID, actionDigest string) {
	t.Helper()
	a := boundPark(t, store, id)
	env, ident := operatorDecisionEnv("approve", a.ApprovalID)
	if rule, err := store.DecideApprovalUnderLaw(context.Background(), a.ApprovalID, action.DecisionApproved,
		time.Now().UTC(), env, ident, "", PolicyPin{Version: 7, Digest: "sha256:law"}); err != nil || rule != "" {
		t.Fatalf("approve: %q %v", rule, err)
	}
	return a.ApprovalID, a.ActionDigest
}

// TestClaim_aStoreThatCannotBeReadIsNotCorruptEvidence is X1 (block B's pass,
// P2): the claim wraps EVERY non-NotFound error of its first read in
// ErrApprovalEvidenceCorrupt. A driver failure — the approvals table dropped
// by a second connection — is published as corrupt evidence; once block B's
// approvalTx types its errors, the same failure would carry BOTH sentinels.
// The claim must pass the classification through: unreadable stays
// unreadable, never both.
//
// Delivered by block B (director, 2026-09-19): the claim can only pass
// through a class that approvalTx assigns, and typing approvalTx's errors is
// B's P2-6 cure, so this mould and the passthrough in
// ClaimApprovalParamsUnderDigest ship in B's PR, after B rebases on A.
//
// Planned probing mutation (after green): restore the blanket
// ErrApprovalEvidenceCorrupt wrap ⇒ this reddens with both sentinels.
func TestClaim_aStoreThatCannotBeReadIsNotCorruptEvidence(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	approvalID, digest := x1ApprovedForClaim(t, store, "act_x1_dropped")
	hand := claimAttackConn(t, store)
	if _, err := hand.Exec(`DROP TABLE approvals`); err != nil {
		t.Fatalf("drop the table from the second connection: %v", err)
	}

	params, _, err := store.ClaimApprovalParamsUnderDigest(context.Background(), approvalID, nil, digest, nil)
	if params != nil {
		t.Fatalf("a refused claim handed back parameters: %q", params)
	}
	corrupt, unreadable := errors.Is(err, ErrApprovalEvidenceCorrupt), errors.Is(err, ErrApprovalUnreadable)
	if !unreadable || corrupt {
		t.Fatalf("err = %v: corrupt=%v unreadable=%v, want corrupt=false unreadable=true — the store did not answer", err, corrupt, unreadable)
	}
}

// TestClaim_aCorruptRowStaysCorruptEvidence is X1's OTHER half: a conversion
// failure of the approval row the claim reads stays corrupt and is never
// reclassified as unreadable. GREEN today (the claim's blanket wrap is
// corrupt); it guards the passthrough against collapsing every class to
// unreadable. Its probing mutation is EXECUTED in the red phase (pre-test paper
// §5).
//
// Evidence level: multiple real database connections. In-process.
func TestClaim_aCorruptRowStaysCorruptEvidence(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	approvalID, digest := x1ApprovedForClaim(t, store, "act_x1_corrupt")
	hand := claimAttackConn(t, store)
	if _, err := hand.Exec(`UPDATE approvals SET policy_version = 'x' WHERE approval_id = ?`, approvalID); err != nil {
		t.Fatalf("corrupt from the second connection: %v", err)
	}
	params, _, err := store.ClaimApprovalParamsUnderDigest(context.Background(), approvalID, nil, digest, nil)
	if params != nil {
		t.Fatalf("a refused claim handed back parameters: %q", params)
	}
	corrupt, unreadable := errors.Is(err, ErrApprovalEvidenceCorrupt), errors.Is(err, ErrApprovalUnreadable)
	if !corrupt || unreadable {
		t.Fatalf("err = %v: corrupt=%v unreadable=%v, want corrupt=true unreadable=false", err, corrupt, unreadable)
	}
}

// TestApprovalDetail_oneClassPerFailure is P2-4 (block A's file, travelling in
// B's train with X1, cured after the rebase): ApprovalDetail types EVERY error
// of its row read as corrupt, so a store that cannot be read is published as
// damaged evidence. A driver failure is unreadable only; a conversion failure
// is corrupt only.
//
// Evidence level: multiple real database connections. In-process.
// Probing mutation (planned): keep the blanket corrupt wrap ⇒ /driver red;
// collapse to unreadable ⇒ /conversion red.
func TestApprovalDetail_oneClassPerFailure(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, stmt              string
		wantCorrupt, wantUnread bool
	}{
		{"driver", `DROP TABLE approvals`, false, true},
		{"conversion", `UPDATE approvals SET policy_version = 'x'`, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			store, _ := sealedStore(t)
			a := boundPark(t, store, "act_detail_"+c.name)
			if _, err := claimAttackConn(t, store).Exec(c.stmt); err != nil {
				t.Fatalf("attack: %v", err)
			}
			_, err := store.ApprovalDetail(context.Background(), a.ApprovalID)
			corrupt, unreadable := errors.Is(err, ErrApprovalEvidenceCorrupt), errors.Is(err, ErrApprovalUnreadable)
			if corrupt != c.wantCorrupt || unreadable != c.wantUnread {
				t.Fatalf("err = %v: corrupt=%v unreadable=%v, want corrupt=%v unreadable=%v", err, corrupt, unreadable, c.wantCorrupt, c.wantUnread)
			}
		})
	}
}

// TestReReadParams_aStoreThatCannotBeReadIsNotCorrupt is the approvalTx0 sister
// of X1 (block A's file, ReReadParams' second read): approvalTx0 wraps every
// non-NoRows error as corrupt. It is reachable deterministically, not only in
// a window: a CLAIMED request (params empty) makes ReReadParams read the row a
// second time, and a renamed column makes THAT statement unpreparable while the
// first read (canonical_params) still succeeds.
//
// Evidence level: multiple real database connections. In-process.
// Probing mutation (planned): keep approvalTx0's blanket wrap ⇒ red.
func TestReReadParams_aStoreThatCannotBeReadIsNotCorrupt(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	approvalID, digest := x1ApprovedForClaim(t, store, "act_x1_reread")
	if _, _, err := store.ClaimApprovalParamsUnderDigest(context.Background(), approvalID, nil, digest, nil); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if _, err := claimAttackConn(t, store).Exec(`ALTER TABLE approvals RENAME COLUMN decision_at TO decision_at_gone`); err != nil {
		t.Fatalf("rename: %v", err)
	}
	_, _, err := store.ReReadParams(context.Background(), approvalID)
	corrupt, unreadable := errors.Is(err, ErrApprovalEvidenceCorrupt), errors.Is(err, ErrApprovalUnreadable)
	if corrupt || !unreadable {
		t.Fatalf("err = %v: corrupt=%v unreadable=%v, want corrupt=false unreadable=true", err, corrupt, unreadable)
	}
}

// TestHistoryRow_conversionFailureIsCorruptOnBsDoors is the post-rebase
// interaction with block A's X2 cure: GetApproval and the decide both call
// verifyApprovalStoryTyped (block A's file), which today publishes a history
// row that will not convert as UNREADABLE. After A's X2 cure it is corrupt only
// — on B's doors too. RED in B's tree until A merges, by construction.
//
// Evidence level: multiple real database connections. In-process.
func TestHistoryRow_conversionFailureIsCorruptOnBsDoors(t *testing.T) {
	t.Parallel()
	doors := map[string]func(s *Store, a action.Approval) error{
		"get": func(s *Store, a action.Approval) error {
			_, _, err := s.GetApproval(context.Background(), a.ApprovalID)
			return err
		},
		"decide-reject": func(s *Store, a action.Approval) error {
			env, ident := operatorDecisionEnv("reject", a.ApprovalID)
			_, err := s.DecideApprovalUnderLaw(context.Background(), a.ApprovalID, action.DecisionRejected,
				time.Now().UTC(), env, ident, "", PolicyPin{})
			return err
		},
	}
	for name, door := range doors {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store, _ := sealedStore(t)
			a := boundPark(t, store, "act_hist_"+name)
			if _, err := claimAttackConn(t, store).Exec(`UPDATE action_decisions SET policy_version = 'x' WHERE action_id = ?`, a.ActionID); err != nil {
				t.Fatalf("attack: %v", err)
			}
			err := door(store, a)
			corrupt, unreadable := errors.Is(err, ErrApprovalEvidenceCorrupt), errors.Is(err, ErrApprovalUnreadable)
			if !corrupt || unreadable {
				t.Fatalf("err = %v: corrupt=%v unreadable=%v, want corrupt=true unreadable=false", err, corrupt, unreadable)
			}
		})
	}
}

// cancelAfter is a context that turns cancelled after k calls to Done() or
// Err(): the transient the block-A diff adversary reproduced at the claim's
// first read. It is NOT deterministic in general: database/sql also calls
// ctx.Done() from its own background goroutines (tx.awaitDone, rs.awaitDone),
// so the call on which the cancellation lands depends on scheduling. Only the
// k whose outcome was stable over 20 runs in both modes are kept (see
// TestClaim_aTransientOrMissingSchemaIsNeverCorrupt).
type cancelAfter struct {
	context.Context
	mu    sync.Mutex
	left  int
	done  chan struct{}
	fired bool
}

func newCancelAfter(k int) *cancelAfter {
	return &cancelAfter{Context: context.Background(), left: k, done: make(chan struct{})}
}

func (c *cancelAfter) tick() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fired {
		return
	}
	c.left--
	if c.left <= 0 {
		c.fired = true
		close(c.done)
	}
}

func (c *cancelAfter) Done() <-chan struct{} { c.tick(); return c.done }

func (c *cancelAfter) Err() error {
	c.tick()
	select {
	case <-c.done:
		return context.Canceled
	default:
		return nil
	}
}

// TestClaim_aTransientOrMissingSchemaIsNeverCorrupt is X1's queued rows: the
// claim's first read under a context that turns cancelled after k calls, and
// over a schema a second connection altered (a renamed or dropped column). Each
// is a store that could not answer, never evidence that was read and found
// invalid: unreadable only, never corrupt.
//
// The context rows keep only k = 4, 5, 6, the values measured stable (red
// 40/40 across -count=20 plain and -race); the rest flip because database/sql's
// own goroutines also call Done(). The schema rows are deterministic.
//
// Evidence level: multiple real database connections for the schema rows;
// in-process for the context rows. Probing mutation (planned, after green):
// keep the claim's blanket corrupt wrap ⇒ the five rows red.
func TestClaim_aTransientOrMissingSchemaIsNeverCorrupt(t *testing.T) {
	t.Parallel()
	type row struct {
		name  string
		setup func(t *testing.T, s *Store)
		ctx   func() context.Context
	}
	var rows []row
	// Measured 2026-09-19 with -count=20, plain and -race: k = 4, 5, 6 were red
	// 40/40. k = 1 was green 40/40 (the cancellation lands at BeginTx, already
	// typed). k = 2, 3, 7, 8 FLIPPED between red and green across runs, so
	// they are not moulds and are not here. Stable over 20 runs is a measured
	// property of this code on this machine, not a proof of determinism.
	for _, k := range []int{4, 5, 6} {
		k := k
		rows = append(rows, row{
			name:  fmt.Sprintf("ctx-cancelled-after-%d", k),
			setup: func(*testing.T, *Store) {},
			ctx:   func() context.Context { return newCancelAfter(k) },
		})
	}
	rows = append(rows,
		row{"rename-column", func(t *testing.T, s *Store) {
			if _, err := claimAttackConn(t, s).Exec(`ALTER TABLE approvals RENAME COLUMN decision_at TO decision_at_gone`); err != nil {
				t.Fatalf("rename: %v", err)
			}
		}, context.Background},
		row{"drop-column", func(t *testing.T, s *Store) {
			if _, err := claimAttackConn(t, s).Exec(`ALTER TABLE approvals DROP COLUMN comment`); err != nil {
				t.Fatalf("drop: %v", err)
			}
		}, context.Background},
	)
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			store, _ := sealedStore(t)
			approvalID, digest := x1ApprovedForClaim(t, store, "act_x1_"+r.name)
			r.setup(t, store)
			params, _, err := store.ClaimApprovalParamsUnderDigest(r.ctx(), approvalID, nil, digest, nil)
			if params != nil {
				t.Fatalf("a refused claim handed back parameters: %q", params)
			}
			corrupt, unreadable := errors.Is(err, ErrApprovalEvidenceCorrupt), errors.Is(err, ErrApprovalUnreadable)
			if !unreadable || corrupt {
				t.Fatalf("err = %v: corrupt=%v unreadable=%v, want corrupt=false unreadable=true", err, corrupt, unreadable)
			}
		})
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R5-S5 (fourth Codex pass): the WILD forms of the F3 races over TWO
// Stores on the same file — independent connection pools, the
// CLI-beside-the-server mold — so the ownership predicates are judged
// by SQLite itself across real connections, not by one pool's
// serialized writer. The deterministic forms stay where they were.
// Reproduction-first contract.

package sqlite

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// twoSealedStores opens TWO stores over one file, both sealing with
// the same registered key (the server + a second process).
func twoSealedStores(t *testing.T) (*Store, *Store, ed25519.PublicKey) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "korvun.db")
	s1, err := Open(path)
	if err != nil {
		t.Fatalf("open 1: %v", err)
	}
	t.Cleanup(func() { _ = s1.Close() })
	s2, err := OpenOperator(path)
	if err != nil {
		t.Fatalf("open 2: %v", err)
	}
	t.Cleanup(func() { _ = s2.Close() })
	sealer, pub := testSealer(t)
	registerSealerKey(t, s1, pub)
	s1.SetReceiptSealer(sealer)
	s2.SetReceiptSealer(sealer)
	return s1, s2, pub
}

func TestRecoveryVsFinish_acrossRealConnections(t *testing.T) {
	t.Parallel()
	s1, s2, _ := twoSealedStores(t)
	ctx := context.Background()
	for i := 0; i < 6; i++ {
		mustRecord(t, s1, fmt.Sprintf("act_s5a_%d", i), action.StateAuthorized)
	}
	var wg sync.WaitGroup
	recErr := make(chan error, 1)
	finErr := make(chan error, 1)
	wg.Add(2)
	go func() { defer wg.Done(); recErr <- s1.RecoverPreviousLife(ctx) }()
	go func() {
		defer wg.Done()
		finErr <- s2.FinishWithResult(ctx, "act_s5a_0", action.StateSucceeded,
			time.Now().UTC(), "sha256:r")
	}()
	wg.Wait()
	if err := <-recErr; err != nil {
		t.Fatalf("the recovery must never error on a clean cross-connection race: %v", err)
	}
	if err := <-finErr; err != nil && !errorsIsInvalidTransition(err) && !isBusy(err) {
		// Across REAL connections the finish has three legitimate
		// outcomes: it wins (nil), it loses the race (named invalid
		// transition), or the 5s busy policy expires while the other
		// connection's recovery holds the writer (SQLITE_BUSY — the
		// real world of two processes; the CI red of 2026-09-02
		// captured it on both OS families: nothing written, one close,
		// one receipt — invariants intact).
		t.Fatalf("the finish may only lose by the named transition or the busy policy: %v", err)
	}
	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("act_s5a_%d", i)
		rec, err := s1.Get(ctx, id)
		if err != nil || !rec.State.Terminal() {
			t.Fatalf("%s terminal: %v %v", id, err, rec.State)
		}
		receipts, _ := s1.ReceiptsByAction(ctx, id)
		if len(receipts) != 1 {
			t.Fatalf("%s: exactly one receipt across connections: %d", id, len(receipts))
		}
	}
}

func TestTwoRecoveries_acrossRealConnections(t *testing.T) {
	t.Parallel()
	s1, s2, _ := twoSealedStores(t)
	ctx := context.Background()
	for i := 0; i < 6; i++ {
		mustRecord(t, s1, fmt.Sprintf("act_s5b_%d", i), action.StateAuthorized)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() { defer wg.Done(); errs <- s1.RecoverPreviousLife(ctx) }()
	go func() { defer wg.Done(); errs <- s2.RecoverPreviousLife(ctx) }()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("two recoveries across real connections must both finish clean: %v", err)
		}
	}
	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("act_s5b_%d", i)
		rec, err := s1.Get(ctx, id)
		if err != nil || rec.State != action.StateFailed || rec.RecoveryMarker != "crash_recovered" {
			t.Fatalf("%s closes exactly once: %v %v %q", id, err, rec.State, rec.RecoveryMarker)
		}
		receipts, _ := s1.ReceiptsByAction(ctx, id)
		if len(receipts) != 1 {
			t.Fatalf("%s: exactly one receipt whoever won: %d", id, len(receipts))
		}
	}
}

func errorsIsInvalidTransition(err error) bool {
	return errors.Is(err, action.ErrInvalidTransition)
}

func isBusy(err error) bool {
	// The busy class across its spellings: plain SQLITE_BUSY (5) and
	// the WAL snapshot variant "database is locked (517)" — a reader's
	// transaction losing its write upgrade to the other connection.
	return err != nil && (strings.Contains(err.Error(), "SQLITE_BUSY") ||
		strings.Contains(err.Error(), "database is locked"))
}

// R6-X4: sweep-vs-operator gains its TWO-connection form like the
// other races — the server's boot sweep against a second connection's
// operator decisions, under -race. Every expired approval ends
// decided EXACTLY once (EXPIRED by the sweep or the operator's
// verdict), receipts stay one-per-action, and clean losses never
// error beyond the named outcomes.
func TestSweepVsOperator_acrossRealConnections(t *testing.T) {
	t.Parallel()
	s1, s2, _ := twoSealedStores(t)
	ctx := context.Background()
	ids := make([]string, 6)
	aprs := make([]string, 6)
	for i := 0; i < 6; i++ {
		ids[i] = fmt.Sprintf("act_s5c_%d", i)
		a := expiredParked(t, s1, ids[i])
		aprs[i] = a.ApprovalID
	}
	var wg sync.WaitGroup
	wg.Add(2)
	sweepErr := make(chan error, 1)
	go func() {
		defer wg.Done()
		_, _, err := s1.SweepExpiredApprovals(ctx, time.Now().UTC())
		sweepErr <- err
	}()
	go func() {
		defer wg.Done()
		// The operator races the sweep on half of them from the OTHER
		// connection; losing to the sweep is the named expiry rule.
		for i := 0; i < 3; i++ {
			envD, identD := operatorDecisionEnv("reject", aprs[i])
			_, err := s2.decideApproval(ctx, aprs[i], "rejected",
				time.Now().UTC(), envD, identD, "")
			if err != nil && !isBusy(err) {
				t.Errorf("operator decide %d: only busy may error: %v", i, err)
			}
		}
	}()
	wg.Wait()
	if err := <-sweepErr; err != nil {
		t.Fatalf("the sweep must never error on clean races: %v", err)
	}
	// A row BOTH sides yielded on (mutual busy) is legitimately
	// POSTPONED — the next cadence owns it. Converge: one more sweep.
	if _, _, err := s1.SweepExpiredApprovals(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("the converging sweep: %v", err)
	}
	for i, id := range ids {
		var status string
		if err := s1.db.QueryRow(`SELECT status FROM approvals WHERE approval_id = ?`, aprs[i]).Scan(&status); err != nil {
			t.Fatalf("status %s: %v", id, err)
		}
		if status != "EXPIRED" && status != "REJECTED" {
			t.Fatalf("%s must end decided after convergence: %s", id, status)
		}
		receipts, _ := s1.ReceiptsByAction(ctx, id)
		if len(receipts) != 1 {
			t.Fatalf("%s: one receipt whoever won: %d", id, len(receipts))
		}
	}
}

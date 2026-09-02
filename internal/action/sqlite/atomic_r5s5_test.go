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
	return err != nil && strings.Contains(err.Error(), "SQLITE_BUSY")
}

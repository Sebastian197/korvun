// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The cross-check family — permanent members of the suite (external
// audit 2026-08-31; cross-check law point 4: cross-scenarios are
// first-class). These tests attack claims FROM OUTSIDE the component:
// another process's store handle, another feature's retention, real
// concurrency between the operator's consults and the server's writes.

package cli

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
)

// CLI×server under REAL concurrency: consults hammer the store while
// the server records and closes actions in parallel. Every consult
// must succeed or fail loudly WITHOUT ever mutating; every server
// write must land; nothing crash-recovers, nothing migrates.
func TestCrossCheck_consultsBesideAWritingServer(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, receiptID, _ := operatorReceipt(t)
	server, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("server open: %v", err)
	}
	defer func() { _ = server.Close() }()
	ctx := context.Background()
	const rounds = 8
	var wg sync.WaitGroup
	errs := make(chan string, rounds*2)
	wg.Add(2)
	go func() { // the server's life
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			id := "act_cc_" + string(rune('a'+i))
			env := operatorProbeEnvelope(id)
			if err := server.RecordAttempt(ctx, env,
				actionsqlite.Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized); err != nil {
				errs <- "server record: " + err.Error()
				return
			}
			if err := server.Finish(ctx, id, action.StateSucceeded, time.Now().UTC()); err != nil {
				errs <- "server finish: " + err.Error()
				return
			}
		}
	}()
	go func() { // the operator's consults, in parallel
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			if code, _, stderr := runIntentCLI(t, "receipt", "verify", "--config", cfgPath, receiptID); code != 0 {
				errs <- "verify beside writer: " + stderr
				return
			}
		}
	}()
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatalf("cross-check violation: %s", e)
	}
	// The server's whole life landed, untouched by the consults.
	for i := 0; i < rounds; i++ {
		id := "act_cc_" + string(rune('a'+i))
		rec, err := server.Get(ctx, id)
		if err != nil || rec.State != action.StateSucceeded || rec.RecoveryMarker != "" {
			t.Fatalf("action %s: %v state=%v marker=%q", id, err, rec.State, rec.RecoveryMarker)
		}
	}
	// And the final check walks a chain the concurrency never bent.
	if code, stdout, _ := runIntentCLI(t, "ledger", "check", "--config", cfgPath); code != 0 || !strings.Contains(stdout, "chain intact") {
		t.Fatalf("post-concurrency chain: %d %q", code, stdout)
	}
}

// Retention×verifier with the REAL prune lives in the sqlite package
// (TestCrossCheck_realPruneMeetsTheVerifierReads), where the sealed
// no-config retention cap has its test seam; the CLI half of that
// scenario — an absent row meeting the verdict surface — is
// TestReceiptVerify_prunedActionRowIsANamedNoteNotALie in this package.

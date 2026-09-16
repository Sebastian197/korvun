// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// A close that is not SUCCEEDED seals no result digest.
//
// The adversary's eighth finding of 2026-09-16: the line that empties
// resultDigest for every outcome but success was rewritten by this train and
// guarded by nothing — deleting it left the suite green, while
// receiptForFinish copies that digest into the SIGNED receipt. A FAILED or
// OUTCOME_UNKNOWN receipt carrying the fingerprint of a result nobody received
// is evidence of something that did not happen.
//
// Evidence level, honest: in-process, a real boot and a real SQLite file; the
// assertion reads the sealed receipt, not the in-memory value.
package app

import (
	"context"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/action/executor"
	"github.com/Sebastian197/korvun/internal/tool"
)

// TestExecuteApprovedAction_aFailedCloseSealsNoResultDigest is the mould.
//
// Probing mutation (executed, red, declared in the canto): delete the
// `if outcome != action.StateSucceeded { resultDigest = "" }` block ⇒ this
// reddens.
func TestExecuteApprovedAction_aFailedCloseSealsNoResultDigest(t *testing.T) {
	store, _, _, approvalID := approvedFlow(t)
	fail := &failingTool{}
	exec := executor.New(tool.Registry{"webhook_call": fail}, 0, time.Now)
	ctx := context.Background()
	actionID := actionOf(t, store, approvalID)

	run, err := ExecuteApprovedAction(ctx, store, exec, approvalID, testLaw, digestOf(t, store, approvalID))
	if err != nil {
		t.Fatalf("a tool that says no is an outcome, not an error of this call: %v", err)
	}
	if !run.Failed {
		t.Fatalf("Failed = false over a tool that refused: %+v", run)
	}
	if run.ResultDigest != "" {
		t.Fatalf("ResultDigest = %q over a FAILED close", run.ResultDigest)
	}
	receipts, rerr := store.ReceiptsByAction(ctx, actionID)
	if rerr != nil {
		t.Fatalf("read the receipts: %v", rerr)
	}
	if len(receipts) == 0 {
		t.Fatal("a closed execution seals its receipt")
	}
	for _, r := range receipts {
		if r.Outcome == string(action.StateFailed) && r.ResultDigest != "" {
			t.Fatalf("the FAILED receipt seals a result digest: %q", r.ResultDigest)
		}
	}
}

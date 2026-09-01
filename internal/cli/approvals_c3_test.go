// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// C3 of the E5 consolidation (second external audit): a crash between
// the decision and the deferred execution must be RESUMABLE — the
// APPROVED-with-params-held request is picked up by an explicit
// operator act, `korvun approvals execute`, which runs the exact
// approved object through the same one-executor path (the atomic claim
// already guarantees single effect). A second execute reports honestly
// instead of re-running; a PENDING request refuses.
// Reproduction-first contract.

package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/app"
	"github.com/Sebastian197/korvun/internal/config"
)

// decidedNotExecuted parks and approves WITHOUT executing — the exact
// state a crash between decision and execution leaves behind.
func decidedNotExecuted(t *testing.T) (cfgPath, dbPath, approvalID string) {
	t.Helper()
	cfgPath, dbPath, approvalID = parkedRequest(t)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	law, err := app.PolicyPinFor(cfg, "a")
	if err != nil {
		t.Fatalf("law: %v", err)
	}
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	env, ident, err := operatorEnvelope("approval", "approve", `{"approval_id":"`+approvalID+`"}`)
	if err != nil {
		t.Fatalf("env: %v", err)
	}
	rule, err := store.DecideApprovalUnderLaw(context.Background(), approvalID, "approved",
		time.Now().UTC(), env, actionsqlite.AttemptIdentity{
			PrincipalID: ident.PrincipalID, IntentID: ident.IntentID, Evidence: ident.Evidence,
		}, "", law)
	if err != nil || rule != "" {
		t.Fatalf("decide: %q %v", rule, err)
	}
	return cfgPath, dbPath, approvalID
}

func TestApprovalsExecute_resumesTheCrashedApproval(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, approvalID := decidedNotExecuted(t)
	code, stdout, stderr := runIntentCLI(t, "approvals", "execute", "--config", cfgPath, approvalID)
	if code != 0 {
		t.Fatalf("AUDIT C3: the crashed approval must resume: %d %q", code, stderr)
	}
	if !strings.Contains(stdout, "42") || !strings.Contains(stdout, "SUCCEEDED") {
		t.Fatalf("execute must report the REAL outcome: %q", stdout)
	}
	store, err := actionsqlite.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	rec, err := store.Get(context.Background(), "act_inbox1")
	if err != nil || rec.State != action.StateSucceeded {
		t.Fatalf("the resumed action must close for real: %v %v", err, rec.State)
	}
}

func TestApprovalsExecute_neverTwiceAndNeverPending(t *testing.T) {
	t.Parallel()
	cfgPath, _, approvalID := decidedNotExecuted(t)
	if code, _, stderr := runIntentCLI(t, "approvals", "execute", "--config", cfgPath, approvalID); code != 0 {
		t.Fatalf("first execute: %d %q", code, stderr)
	}
	// The second execute reports honestly — the claim is one-shot.
	code, _, stderr := runIntentCLI(t, "approvals", "execute", "--config", cfgPath, approvalID)
	if code == 0 {
		t.Fatal("a consumed approval must not execute twice")
	}
	if !strings.Contains(stderr, "already") {
		t.Fatalf("the refusal must say it was already executed/closed: %q", stderr)
	}
	// And a PENDING one refuses — execute resumes decisions, never takes them.
	cfgPath2, _, pendingID := parkedRequest(t)
	code, _, stderr = runIntentCLI(t, "approvals", "execute", "--config", cfgPath2, pendingID)
	if code == 0 {
		t.Fatal("execute must never take the decision itself")
	}
	if !strings.Contains(stderr, "PENDING") && !strings.Contains(stderr, "APPROVED") {
		t.Fatalf("the refusal must name the status rule: %q", stderr)
	}
}

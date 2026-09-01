// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// C1 of the E5 consolidation (second external audit, AS-6 RE-MAPPED):
// the invalidation law enforced on the PRODUCTION path — the auditor's
// own scenario as a permanent member. A policy change between park and
// decision must refuse the approve NAMING approval_invalidated/policy
// (the request stays PENDING, ZERO execution); a tool revoked from the
// CURRENT config must NEVER execute; a reject stays possible whatever
// the law did. Reproduction-first contract.

package cli

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
)

// mutateConfig rewrites the harness config file through a JSON edit —
// the operator changing the law between park and decision.
func mutateConfig(t *testing.T, cfgPath string, edit func(map[string]any)) {
	t.Helper()
	raw, err := os.ReadFile(cfgPath) // #nosec G304 -- test-owned temp path
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	edit(cfg)
	out, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(cfgPath, out, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func TestApprovalsApprove_policyChangeInvalidates(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, approvalID := parkedRequest(t)
	// The law moves: the brain's sensitivity changes after the park.
	mutateConfig(t, cfgPath, func(cfg map[string]any) {
		cfg["brains"].([]any)[0].(map[string]any)["sensitivity"] = "private"
	})
	code, _, stderr := runIntentCLI(t, "approvals", "approve", "--config", cfgPath, approvalID)
	if code == 0 {
		t.Fatal("AUDIT C1: an approve under a DIFFERENT law must refuse")
	}
	if !strings.Contains(stderr, "approval_invalidated") || !strings.Contains(stderr, "policy") {
		t.Fatalf("the refusal must name approval_invalidated/policy: %q", stderr)
	}
	// ZERO execution, and the request still awaits an explicit human act.
	store, err := actionsqlite.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	a, _, err := store.GetApproval(context.Background(), approvalID)
	if err != nil || a.Status != action.ApprovalPending {
		t.Fatalf("the request must stay PENDING for an explicit decision: %v %v", err, a.Status)
	}
	rec, err := store.Get(context.Background(), a.ActionID)
	if err != nil || rec.State != action.StatePendingApproval {
		t.Fatalf("ZERO execution — the action must stay parked: %v %v", err, rec.State)
	}
}

func TestApprovalsApprove_revokedToolNeverExecutes(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, approvalID := parkedRequest(t)
	// The operator revokes calc from the brain's grant list (another
	// tool stays — a real revocation, not an empty agent block).
	mutateConfig(t, cfgPath, func(cfg map[string]any) {
		cfg["brains"].([]any)[0].(map[string]any)["agent"].(map[string]any)["tools"] = []any{"time"}
	})
	code, _, stderr := runIntentCLI(t, "approvals", "approve", "--config", cfgPath, approvalID)
	if code == 0 {
		t.Fatal("AUDIT C1: a tool revoked from config must NEVER execute")
	}
	// Revoking a tool changes the law itself (tools are IN the pin), so
	// the policy axis catches it at decide; the executor's membership
	// check against the CURRENT grant list is the depth behind it
	// (pinned by its own test in internal/app).
	if !strings.Contains(stderr, "approval_invalidated") && !strings.Contains(stderr, "calc") {
		t.Fatalf("the refusal must name the invalidation or the revoked tool: %q", stderr)
	}
	store, err := actionsqlite.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	a, _, err := store.GetApproval(context.Background(), approvalID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	rec, err := store.Get(context.Background(), a.ActionID)
	if err != nil || rec.State == action.StateSucceeded {
		t.Fatalf("the revoked tool must not have run: %v %v", err, rec.State)
	}
}

func TestApprovalsReject_worksWhateverTheLawDid(t *testing.T) {
	t.Parallel()
	cfgPath, _, approvalID := parkedRequest(t)
	mutateConfig(t, cfgPath, func(cfg map[string]any) {
		cfg["brains"].([]any)[0].(map[string]any)["sensitivity"] = "private"
	})
	code, stdout, stderr := runIntentCLI(t, "approvals", "reject", "--config", cfgPath, approvalID)
	if code != 0 {
		t.Fatalf("a reject is safe under ANY law: %d %q", code, stderr)
	}
	if !strings.Contains(stdout, "rejected") {
		t.Fatalf("reject must report: %q", stdout)
	}
}

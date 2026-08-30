// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The operator's act, grants half — Etapa 2, lote 5, pieza 2: issue and
// delegate under an intent, with ValidateAttenuation as the wall HERE
// TOO — the operator's CLI cannot widen either: same validator, same
// denial naming the dimension, and a refused child never touches the
// disk. Inactive/expired/revoked authority fails CLOSED with its sealed
// rule, receipt included. Approved-red contract.

package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
)

// activeIntentID creates and activates one intent, returning its id.
func activeIntentID(t *testing.T, cfgPath string) string {
	t.Helper()
	code, stdout, stderr := runIntentCLI(t, "intent", "create", "--config", cfgPath,
		"--purpose", "grants sandbox", "--operations", "calc,time,read_file")
	if code != 0 {
		t.Fatalf("create intent: %d %q", code, stderr)
	}
	id := extractID(t, stdout, "int_")
	if code, _, stderr := runIntentCLI(t, "intent", "activate", "--config", cfgPath, id); code != 0 {
		t.Fatalf("activate intent: %d %q", code, stderr)
	}
	return id
}

// issueGrantID issues a parent grant under the intent, returning its id.
func issueGrantID(t *testing.T, cfgPath, intentID string) string {
	t.Helper()
	code, stdout, stderr := runIntentCLI(t, "grant", "issue", "--config", cfgPath,
		"--intent", intentID, "--subject", "principal_brain_a",
		"--operations", "calc,time", "--max-actions", "50",
		"--expires", "2026-09-30T00:00:00Z", "--depth", "2")
	if code != 0 {
		t.Fatalf("issue: %d %q", code, stderr)
	}
	return extractID(t, stdout, "grant_")
}

func TestGrantIssue_underActiveIntentWithReceipt(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath := intentTestConfig(t)
	intentID := activeIntentID(t, cfgPath)
	grantID := issueGrantID(t, cfgPath, intentID)
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	grant, err := store.GetGrant(context.Background(), grantID)
	if err != nil {
		t.Fatalf("the grant must persist: %v", err)
	}
	if grant.IntentID != intentID || grant.SubjectPrincipalID != "principal_brain_a" {
		t.Fatalf("grant terms: %+v", grant)
	}
	if grant.IssuerPrincipalID != action.OperatorPrincipal().PrincipalID {
		t.Fatalf("the operator issues, got %q", grant.IssuerPrincipalID)
	}
	if grant.Status != action.LifecycleActive || grant.DelegationDepthRemaining != 2 {
		t.Fatalf("grant shape: %+v", grant)
	}
	rec, evidence := receiptOf(t, dbPath, "grant", "issue")
	if rec.State != action.StateSucceeded || evidence.Credential != action.CredentialLoopbackInProcess {
		t.Fatalf("receipt: %s %s", rec.State, evidence.Credential)
	}
}

func TestGrantIssue_inactiveOrExpiredIntentFailsClosed(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath := intentTestConfig(t)
	// DRAFT intent: not in force -> intent_inactive.
	code, stdout, _ := runIntentCLI(t, "intent", "create", "--config", cfgPath,
		"--purpose", "draft", "--operations", "calc")
	if code != 0 {
		t.Fatalf("create: %d", code)
	}
	draftID := extractID(t, stdout, "int_")
	code, _, stderr := runIntentCLI(t, "grant", "issue", "--config", cfgPath,
		"--intent", draftID, "--subject", "principal_brain_a", "--operations", "calc")
	if code != 1 || !strings.Contains(stderr, action.RuleIntentInactive) {
		t.Fatalf("issuing under DRAFT must fail closed naming %s: %d %q",
			action.RuleIntentInactive, code, stderr)
	}
	// Expired intent (ACTIVE status, window past): the clock wins.
	code, stdout, _ = runIntentCLI(t, "intent", "create", "--config", cfgPath,
		"--purpose", "expired", "--operations", "calc",
		"--valid-from", "2026-08-01T00:00:00Z", "--expires", "2026-08-02T00:00:00Z")
	if code != 0 {
		t.Fatalf("create expired: %d", code)
	}
	expiredID := extractID(t, stdout, "int_")
	if code, _, stderr := runIntentCLI(t, "intent", "activate", "--config", cfgPath, expiredID); code != 0 {
		t.Fatalf("activate: %d %q", code, stderr)
	}
	code, _, stderr = runIntentCLI(t, "grant", "issue", "--config", cfgPath,
		"--intent", expiredID, "--subject", "principal_brain_a", "--operations", "calc")
	if code != 1 || !strings.Contains(stderr, action.RuleIntentExpired) {
		t.Fatalf("issuing under an expired window must name %s: %d %q",
			action.RuleIntentExpired, code, stderr)
	}
	// Both refusals leave DENIED receipts with their sealed rules.
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	recs, err := store.ListByOperation(context.Background(), "grant", "issue")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("both refusals leave receipts, got %d", len(recs))
	}
	rules := map[string]bool{}
	for _, r := range recs {
		if r.State != action.StateDenied {
			t.Fatalf("refusal state = %s", r.State)
		}
		rules[r.Decision.Rule] = true
	}
	if !rules[action.RuleIntentInactive] || !rules[action.RuleIntentExpired] {
		t.Fatalf("sealed rules on the receipts, got %v", rules)
	}
}

func TestGrantDelegate_wideningDeniedNamingTheDimension(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath := intentTestConfig(t)
	intentID := activeIntentID(t, cfgPath)
	parentID := issueGrantID(t, cfgPath, intentID)
	// The child asks for a LARGER budget than the parent's 50: the wall.
	code, _, stderr := runIntentCLI(t, "grant", "delegate", "--config", cfgPath,
		"--parent", parentID, "--subject", "principal_ch_hooks",
		"--operations", "calc", "--max-actions", "500")
	if code != 1 || !strings.Contains(stderr, "budget") {
		t.Fatalf("the denial must NAME the widened dimension: %d %q", code, stderr)
	}
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	// The widening child NEVER touched the disk: only the parent exists.
	recs, err := store.ListByOperation(context.Background(), "grant", "delegate")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(recs) != 1 || recs[0].State != action.StateDenied {
		t.Fatalf("the refused delegation leaves its DENIED receipt, got %+v", recs)
	}
	if recs[0].Decision.Rule != "attenuation_violated" {
		t.Fatalf("receipt rule = %q", recs[0].Decision.Rule)
	}
}

func TestGrantDelegate_attenuatedChildPersists(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath := intentTestConfig(t)
	intentID := activeIntentID(t, cfgPath)
	parentID := issueGrantID(t, cfgPath, intentID)
	code, stdout, stderr := runIntentCLI(t, "grant", "delegate", "--config", cfgPath,
		"--parent", parentID, "--subject", "principal_ch_hooks",
		"--operations", "calc", "--max-actions", "5")
	if code != 0 {
		t.Fatalf("a strict subset delegation must pass: %d %q", code, stderr)
	}
	childID := extractID(t, stdout, "grant_")
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	child, err := store.GetGrant(context.Background(), childID)
	if err != nil {
		t.Fatalf("child must persist: %v", err)
	}
	if child.ParentGrantID != parentID || child.IssuerPrincipalID != "principal_brain_a" {
		t.Fatalf("delegation chain: %+v", child)
	}
	if child.DelegationDepthRemaining != 1 {
		t.Fatalf("depth defaults to parent-1, got %d", child.DelegationDepthRemaining)
	}
}

func TestGrantDelegate_revokedParentFailsClosed(t *testing.T) {
	t.Parallel()
	cfgPath, _ := intentTestConfig(t)
	intentID := activeIntentID(t, cfgPath)
	parentID := issueGrantID(t, cfgPath, intentID)
	if code, _, stderr := runIntentCLI(t, "grant", "revoke", "--config", cfgPath, parentID); code != 0 {
		t.Fatalf("revoke parent: %d %q", code, stderr)
	}
	code, _, stderr := runIntentCLI(t, "grant", "delegate", "--config", cfgPath,
		"--parent", parentID, "--subject", "principal_ch_hooks",
		"--operations", "calc", "--max-actions", "5")
	if code != 1 || !strings.Contains(stderr, action.RuleAuthorityRevoked) {
		t.Fatalf("a revoked parent must fail closed naming %s: %d %q",
			action.RuleAuthorityRevoked, code, stderr)
	}
}

func TestGrantUsageErrors(t *testing.T) {
	t.Parallel()
	cfgPath, _ := intentTestConfig(t)
	cases := [][]string{
		{"grant"},
		{"grant", "bogus", "--config", cfgPath},
		{"grant", "issue", "--config", cfgPath},    // no intent/subject/ops
		{"grant", "delegate", "--config", cfgPath}, // no parent
		{"grant", "revoke", "--config", cfgPath},   // no id
		{"grant", "issue", "--intent", "int_x", "--subject", "s",
			"--operations", "calc"}, // no config
	}
	for _, args := range cases {
		if code, _, _ := runIntentCLI(t, args...); code != 2 {
			t.Fatalf("%v must be a usage error (2), got %d", args, code)
		}
	}
}

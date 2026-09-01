// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The operator's approvals inbox — Etapa 5, lote 4, pieza 1 (spec
// FR-CLI, sealed NC-1b): `korvun approvals list|show` through the
// consolidation's READ-ONLY door (the audit's lesson applied to the
// new surface FROM BIRTH — consults never mutate, pinned);
// `approve|reject` as mutating operator acts through the sealed store
// (the rotate-key mold), approve firing the lote-3 deferred execution
// (claim + belt) and reporting the REAL outcome. show renders the full
// §15.2 preview, the raw params (the operator's loopback right,
// ADR-0024) and THE DIGEST the human approves, prominently.
// Approved-red contract.

package cli

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/app"
	"github.com/Sebastian197/korvun/internal/config"
)

// parkedRequest hand-parks a calc action with its approval (the gate
// normally parks only irreversibles; the CLI surface is class-agnostic
// — the approval IS the authorization).
func parkedRequest(t *testing.T) (cfgPath, dbPath, approvalID string) {
	t.Helper()
	cfgPath, dbPath = intentTestConfig(t)
	// The park records the REAL current law (C1): the pin the approve
	// will re-derive from the same config and validate against.
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
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
	env := action.NewEnvelope("act_inbox1", "env-inbox",
		action.Source{Kind: "agent_brain", Protocol: "text", Channel: "console"},
		action.Operation{Namespace: "tool", Name: "calc", Version: 1},
		`7*6`, time.Now().UTC())
	env.IntentID = action.RootIntentID
	env.Principal = action.PrincipalRef{PrincipalID: "principal_brain_a"}
	env.Effect = action.Effect{Class: string(action.EffectPure)}
	preview := action.ActionPreview{
		ActionID: env.ActionID, SchemaVersion: 1,
		IntentPurpose: "semana de pruebas", PrincipalID: "principal_brain_a",
		Operation: "tool/calc", Resources: []string{"console"},
		DataEgress: "no declared data egress", ArgsDigest: env.ParametersDigest,
		CostLine: "unbudgeted", EffectClass: action.EffectPure,
		Reversibility: "pure — no external effect", ToolCage: "calc",
		PolicyVersion: law.Version, PolicyDigest: law.Digest, RequiredRule: "require_approval",
	}
	a := action.Approval{
		ApprovalID: action.NewApprovalID(), SchemaVersion: 1,
		ActionID: env.ActionID, ActionDigest: env.ParametersDigest,
		PreviewDigest: preview.Digest(),
		RequestedFrom: action.OperatorPrincipal().PrincipalID,
		Reason:        "require_approval", RiskSummary: "pure — no external effect",
		PolicyVersion: law.Version, PolicyDigest: law.Digest,
		RequestedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Hour),
		Status: action.ApprovalPending,
	}
	if err := store.CreateApprovalRequest(context.Background(), env,
		actionsqlite.Decision{Outcome: "require_approval", Rule: "require_approval", PolicyVersion: law.Version, PolicyDigest: law.Digest},
		a, preview, `7*6`); err != nil {
		t.Fatalf("park: %v", err)
	}
	return cfgPath, dbPath, a.ApprovalID
}

func TestApprovalsList_readOnlyDoorFromBirth(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, approvalID := parkedRequest(t)
	before, _ := os.Stat(dbPath)
	code, stdout, stderr := runIntentCLI(t, "approvals", "list", "--config", cfgPath)
	if code != 0 {
		t.Fatalf("list: %d %q", code, stderr)
	}
	if !strings.Contains(stdout, approvalID) || !strings.Contains(stdout, "PENDING") {
		t.Fatalf("the pending request must show with its status: %q", stdout)
	}
	if !strings.Contains(stdout, "expires") && !strings.Contains(stdout, "EXPIRES") {
		t.Fatalf("expiry must be visible on the list: %q", stdout)
	}
	// The audit's lesson from birth: the consult NEVER mutates — same
	// file size+mtime class check as the RO pin (no state change).
	after, _ := os.Stat(dbPath)
	if before.ModTime() != after.ModTime() || before.Size() != after.Size() {
		t.Fatal("AUDIT LESSON: approvals list must go through the read-only door")
	}
	// And on a profile with no store: honest failure, no file created.
	cfg2, db2 := intentTestConfig(t)
	if code, _, _ := runIntentCLI(t, "approvals", "list", "--config", cfg2); code != 1 {
		t.Fatalf("missing store must fail honest: %d", code)
	}
	if _, err := os.Stat(db2); !os.IsNotExist(err) {
		t.Fatal("the RO door must not create the store")
	}
}

func TestApprovalsShow_theFullTruthForTheHuman(t *testing.T) {
	t.Parallel()
	cfgPath, _, approvalID := parkedRequest(t)
	code, stdout, stderr := runIntentCLI(t, "approvals", "show", "--config", cfgPath, approvalID)
	if code != 0 {
		t.Fatalf("show: %d %q", code, stderr)
	}
	// The §15.2 rows, the raw params (loopback right) and THE DIGEST.
	for _, must := range []string{
		"semana de pruebas",         // purpose
		"principal_brain_a",         // actor
		"tool/calc",                 // operation
		"no declared data egress",   // egress
		"unbudgeted",                // cost
		"pure — no external effect", // reversibility
		"sha256:",                   // pinned law digest, visible
		`7*6`,                       // RAW params — the operator's right
		"digest",                    // the digest label, prominent
	} {
		if !strings.Contains(stdout, must) {
			t.Fatalf("show must render %q — got:\n%s", must, stdout)
		}
	}
}

func TestApprovalsApprove_executesAndReportsTheRealOutcome(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, approvalID := parkedRequest(t)
	code, stdout, stderr := runIntentCLI(t, "approvals", "approve", "--config", cfgPath, approvalID)
	if code != 0 {
		t.Fatalf("approve: %d %q %q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "42") || !strings.Contains(stdout, "SUCCEEDED") {
		t.Fatalf("approve must report the REAL outcome of the real execution: %q", stdout)
	}
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	rec, _ := store.Get(ctx, "act_inbox1")
	if rec.State != action.StateSucceeded {
		t.Fatalf("the parked action closed for real: %v", rec.State)
	}
	// The E4 ink: the decision act's receipt (proof) + the outcome receipt.
	a, _, _ := store.GetApproval(ctx, approvalID)
	if a.Status != action.ApprovalApproved || a.DecisionReceiptID == "" {
		t.Fatalf("the act must leave its proof: %+v", a.Status)
	}
	outcome, _ := store.ReceiptsByAction(ctx, "act_inbox1")
	if len(outcome) != 1 || outcome[0].Outcome != string(action.StateSucceeded) {
		t.Fatalf("the outcome receipt: %+v", outcome)
	}
}

func TestApprovalsReject_closesWithItsInk(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, approvalID := parkedRequest(t)
	code, stdout, stderr := runIntentCLI(t, "approvals", "reject", "--config", cfgPath, "--comment", "not today", approvalID)
	if code != 0 {
		t.Fatalf("reject: %d %q", code, stderr)
	}
	if !strings.Contains(stdout, "rejected") {
		t.Fatalf("the verdict names itself: %q", stdout)
	}
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	rec, _ := store.Get(ctx, "act_inbox1")
	if rec.State != action.StateRejected {
		t.Fatalf("rejected action state: %v", rec.State)
	}
	a, _, _ := store.GetApproval(ctx, approvalID)
	if a.Status != action.ApprovalRejected || a.Comment != "not today" || a.DecisionReceiptID == "" {
		t.Fatalf("the reject act with its comment and proof: %+v", a)
	}
	// A rejected request NEVER executes afterwards (the lote-3 pin at
	// the CLI surface).
	if code, _, _ := runIntentCLI(t, "approvals", "approve", "--config", cfgPath, approvalID); code == 0 {
		t.Fatal("approving after a reject must refuse")
	}
}

func TestApprovalsCmd_usage(t *testing.T) {
	t.Parallel()
	if code, _, stderr := runIntentCLI(t, "approvals"); code != 2 || !strings.Contains(stderr, "expected a subcommand") {
		t.Fatalf("bare approvals: %d %q", code, stderr)
	}
	if code, _, stderr := runIntentCLI(t, "approvals", "burn"); code != 2 || !strings.Contains(stderr, "unknown subcommand") {
		t.Fatalf("unknown verb: %d %q", code, stderr)
	}
	if code, _, stderr := runIntentCLI(t, "approvals", "show", "--config", "x.json"); code != 2 || !strings.Contains(stderr, "usage") {
		t.Fatalf("show without id: %d %q", code, stderr)
	}
}

// The v2 receipt meets the judge — Etapa 5 lote 4 pieza 2 (FR-RCP):
// a receipt that references its approval must COHERE with the approval
// row, or fail by name; v1 historical receipts verify forever; mixed
// chains walk whole.
func TestReceiptVerify_approvalCoherence(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, approvalID := parkedRequest(t)
	if code, _, stderr := runIntentCLI(t, "approvals", "approve", "--config", cfgPath, approvalID); code != 0 {
		t.Fatalf("approve: %q", stderr)
	}
	// The executed outcome receipt seals the approval — verify green.
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	receipts, _ := store.ReceiptsByAction(context.Background(), "act_inbox1")
	_ = store.Close()
	if len(receipts) != 1 || receipts[0].ApprovalDigest == "" {
		t.Fatalf("the outcome receipt must seal its approval: %+v", receipts)
	}
	receiptID := receipts[0].ReceiptID
	if code, stdout, _ := runIntentCLI(t, "receipt", "verify", "--config", cfgPath, receiptID); code != 0 || !strings.Contains(stdout, "OK") {
		t.Fatalf("the sealed-approval receipt verifies: %d %q", code, stdout)
	}
	// The saboteur rewrites the approval row's decision out of band:
	// the receipt's sealed reference no longer matches — FAIL by name.
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("raw: %v", err)
	}
	if _, err := db.Exec(`UPDATE approvals SET decision_principal_id = 'principal_forged' WHERE approval_id = ?`, approvalID); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	_ = db.Close()
	code, stdout, stderr := runIntentCLI(t, "receipt", "verify", "--config", cfgPath, receiptID)
	if code != 1 || !strings.Contains(stdout+stderr, "approval_mismatch") {
		t.Fatalf("a rewritten approval row must FAIL by name: %d %q %q", code, stdout, stderr)
	}
	// And ledger check names it too.
	if code, stdout, _ := runIntentCLI(t, "ledger", "check", "--config", cfgPath); code != 1 || !strings.Contains(stdout, "approval_mismatch") {
		t.Fatalf("the chain walk names the approval mismatch: %d %q", code, stdout)
	}
}

func TestApprovals_moreErrorPaths(t *testing.T) {
	t.Parallel()
	// Ghost ids fail loud on every verb.
	cfgPath, _, _ := parkedRequest(t)
	if code, _, stderr := runIntentCLI(t, "approvals", "show", "--config", cfgPath, "apr_ghost"); code != 1 || !strings.Contains(stderr, "apr_ghost") {
		t.Fatalf("ghost show: %d %q", code, stderr)
	}
	if code, _, _ := runIntentCLI(t, "approvals", "approve", "--config", cfgPath, "apr_ghost"); code != 1 {
		t.Fatal("ghost approve must fail")
	}
	if code, _, _ := runIntentCLI(t, "approvals", "reject", "--config", cfgPath, "apr_ghost"); code != 1 {
		t.Fatal("ghost reject must fail")
	}
	// Usage paths.
	if code, _, _ := runIntentCLI(t, "approvals", "list"); code != 2 {
		t.Fatal("list without config: usage")
	}
	if code, _, _ := runIntentCLI(t, "approvals", "approve", "--config", cfgPath); code != 2 {
		t.Fatal("approve without id: usage")
	}
	// Double-decide at the CLI surface: the second verb reports the rule.
	cfg2, _, apr2 := parkedRequest(t)
	if code, _, _ := runIntentCLI(t, "approvals", "reject", "--config", cfg2, apr2); code != 0 {
		t.Fatal("first reject")
	}
	if code, _, stderr := runIntentCLI(t, "approvals", "reject", "--config", cfg2, apr2); code != 1 || !strings.Contains(stderr, "approval_already_decided") {
		t.Fatalf("second decide reports the rule: %d %q", code, stderr)
	}
}

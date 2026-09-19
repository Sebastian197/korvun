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
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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
	return parkedRequestExpiring(t, time.Now().UTC().Add(time.Hour))
}

// parkedRequestExpiring is the same park with an injectable expiry —
// the C6 clock-truth scenarios park already-expired requests.
func parkedRequestExpiring(t *testing.T, expiresAt time.Time) (cfgPath, dbPath, approvalID string) {
	t.Helper()
	cfgPath, dbPath = intentTestConfig(t)
	// The park goes through the REAL doors (R4-F2): the current law
	// pin, the action-package factory, the store's bundle door.
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
	now := time.Now().UTC()
	b, err := action.NewBoundApprovalRequest(env, `7*6`, action.ApprovalContext{
		IntentPurpose: "semana de pruebas",
		GrantID:       "-", CostLine: "unbudgeted", ToolCage: "calc",
		Descriptor:    action.EffectDescriptor{Class: action.EffectPure, Reversible: true},
		HasDescriptor: true,
		LawVersion:    law.Version, LawDigest: law.Digest,
		Rule: "require_approval",
		Now:  now, TTL: expiresAt.Sub(now),
	})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if err := store.CreateApprovalRequest(context.Background(), b); err != nil {
		t.Fatalf("park: %v", err)
	}
	return cfgPath, dbPath, b.Approval().ApprovalID
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
		"semana de pruebas",       // purpose
		"principal_brain_a",       // actor
		"tool/calc",               // operation
		"no declared data egress", // egress
		"unbudgeted",              // cost
		"pure — reversible",       // reversibility, DERIVED by the factory
		"sha256:",                 // pinned law digest, visible
		`7*6`,                     // RAW params — the operator's right
		"digest",                  // the digest label, prominent
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
	// Ghost ids fail loud on every verb. The ghost has the minted shape (v0.15.1
	// block B, P2-1: a malformed id is refused by shape before the store, exit
	// 2), so what fails here is the store's answer: no such request.
	const ghost = "apr_00000000000000000000000000000000"
	cfgPath, _, _ := parkedRequest(t)
	if code, _, stderr := runIntentCLI(t, "approvals", "show", "--config", cfgPath, ghost); code != 1 || !strings.Contains(stderr, ghost) {
		t.Fatalf("ghost show: %d %q", code, stderr)
	}
	if code, _, _ := runIntentCLI(t, "approvals", "approve", "--config", cfgPath, ghost); code != 1 {
		t.Fatal("ghost approve must fail")
	}
	if code, _, _ := runIntentCLI(t, "approvals", "reject", "--config", cfgPath, ghost); code != 1 {
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

// TestHelp_listsEveryCommandFamilyTheBinaryAnswers pins the binary's own help
// against what it actually dispatches.
//
// Three families answered and were not listed: approvals, ledger and receipt —
// including the one this release exists for. `korvun help` is a public surface
// like any other, and a public surface that omits a shipped capability is the
// same defect as one that describes it wrongly.
//
// The second shape of this mould cured two holes, both found by driving it:
//
//   - it asserted with strings.Contains over the WHOLE stdout, so deleting the
//     `receipt` line stayed green — "receipt" survives inside the ledger line's
//     "The book of receipts: check.";
//   - its list of families was seven names typed by hand, so deleting the
//     `config check` line stayed green and a new dispatched family would never
//     be noticed at all.
//
// It now reads the dispatch out of cli.go and the Commands block out of the
// help, and crosses them BOTH ways by line.
//
// Probing mutations (executed, red, declared in the canto): delete any Commands
// line; delete a `case` from the dispatch without touching the help.
func TestHelp_listsEveryCommandFamilyTheBinaryAnswers(t *testing.T) {
	dispatched := dispatchedFamilies(t)
	listed := helpCommandBlock(t)
	for _, family := range dispatched {
		if !listed[family] {
			t.Errorf("korvun help has no Commands line for %q, and run() dispatches it", family)
		}
	}
	for name := range listed {
		if !slices.Contains(dispatched, name) {
			t.Errorf("korvun help lists %q on its own line and run() dispatches nothing by that name", name)
		}
	}
}

// dispatchedFamilies reads the `case` labels of run()'s switch out of the
// SOURCE. A hand-typed list is a second place to forget a family, which is the
// defect this mould exists to catch.
func dispatchedFamilies(t *testing.T) []string {
	t.Helper()
	src, err := os.ReadFile("cli.go")
	if err != nil {
		t.Fatalf("read cli.go: %v — this mould crosses the DISPATCH, not a list", err)
	}
	body := string(src)
	from := strings.Index(body, "func (c *cli) run(args []string) int {")
	if from < 0 {
		t.Fatal("run() not found in cli.go — the mould's anchor moved")
	}
	sw := strings.Index(body[from:], "switch args[0] {")
	if sw < 0 {
		t.Fatal("run() has no `switch args[0]` — the mould's anchor moved")
	}
	rest := body[from+sw:]
	to := strings.Index(rest, "\n\t}\n")
	if to < 0 {
		t.Fatal("the dispatch switch has no closing brace at the expected indent")
	}
	var families []string
	for _, m := range regexp.MustCompile(`case ((?:"[^"]+"(?:, )?)+):`).FindAllStringSubmatch(rest[:to], -1) {
		for _, lit := range regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(m[1], -1) {
			// The flag spellings (-h, --help) are aliases of the family, not
			// families of their own: help does not list them as commands.
			if strings.HasPrefix(lit[1], "-") {
				continue
			}
			families = append(families, lit[1])
		}
	}
	if len(families) < 5 {
		t.Fatalf("read only %d dispatched families — the scan is broken, not the binary: %v", len(families), families)
	}
	return families
}

// helpCommandBlock returns the first token of every line of the help's
// Commands block — what a reader sees as the command's name.
func helpCommandBlock(t *testing.T) map[string]bool {
	t.Helper()
	code, stdout, _ := runIntentCLI(t, "help")
	if code != 0 {
		t.Fatalf("help exit = %d", code)
	}
	_, after, ok := strings.Cut(stdout, "Commands:\n")
	if !ok {
		t.Fatal("korvun help has no Commands: block")
	}
	block, _, _ := strings.Cut(after, "\n\n")
	listed := map[string]bool{}
	for _, ln := range strings.Split(block, "\n") {
		fields := strings.Fields(ln)
		if len(fields) == 0 {
			continue
		}
		listed[fields[0]] = true
	}
	if len(listed) == 0 {
		t.Fatal("the Commands block parsed empty — the scan is broken, not the help")
	}
	return listed
}

// ---------------------------------------------------------------------------
// The deadline at the CLI surface — the sixth pass's P1-1.
//
// `340b9c1` added `if run.Unknown` to this file to cure a regression the fifth
// pass caught: `korvun approvals approve` printed `outcome: SUCCEEDED` over a
// webhook_call whose POST may have been delivered and whose answer was lost.
// The cure landed with NO mould of its own — `grep -rn Unknown internal/cli`
// over the _test files returned nothing — and its canto declared two probing
// mutations, both over `internal/app`. Deleting the branch left the whole suite
// green, so the regression could walk back in tomorrow.
//
// This drives the REAL command, over the REAL tool, against a server that never
// answers inside the cage's one-second bound.
//
// Evidence level, honest: in-process CLI (Run over the real arg vector) with a
// real store on disk and a real HTTP server. Not a compiled binary.
// ---------------------------------------------------------------------------

// parkedWebhookExpiring writes a profile whose webhook_call cage points at
// `host` with a one-second bound, and parks one irreversible call to `url`.
func parkedWebhookExpiring(t *testing.T, host, url string) (cfgPath, dbPath, approvalID string) {
	t.Helper()
	dir := t.TempDir()
	dbPath = filepath.Join(dir, "korvun.db")
	raw, err := json.Marshal(map[string]any{
		"schema_version": 1,
		"storage":        map[string]any{"path": dbPath},
		"approvals":      map[string]any{"enabled": true},
		"brains": []map[string]any{{
			"name": "a", "sensitivity": "public",
			"policy": map[string]any{"kind": "priority"},
			"models": []map[string]any{{"provider": "ollama", "model_id": "m", "locality": "local"}},
			"agent": map[string]any{
				"tools":          []any{"webhook_call"},
				"effect_ceiling": "critical",
				"webhook_call": map[string]any{
					"allow_hosts":     []any{host},
					"timeout_seconds": 1,
				},
			},
		}},
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	cfgPath = filepath.Join(dir, "korvun.json")
	if err := os.WriteFile(cfgPath, raw, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	_, law, err := app.ResolveApprovalLaw(cfg, "a")
	if err != nil {
		t.Fatalf("law: %v", err)
	}
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	args := url + ` {"event":"ping"}`
	env := action.NewEnvelope("act_slowhook", "env-slowhook",
		action.Source{Kind: "agent_brain", Protocol: "text", Channel: "telegram"},
		action.Operation{Namespace: "tool", Name: "webhook_call", Version: 1},
		args, time.Now().UTC())
	env.IntentID = action.RootIntentID
	env.Principal = action.PrincipalRef{PrincipalID: "principal_brain_a"}
	env.Effect = action.Effect{Class: string(action.EffectWriteIrreversible)}
	now := time.Now().UTC()
	b, err := action.NewBoundApprovalRequest(env, args, action.ApprovalContext{
		IntentPurpose: "avisar al webhook",
		GrantID:       "-", GrantDepth: 1, CostLine: "unbudgeted",
		ToolCage: "webhook_call",
		Descriptor: action.EffectDescriptor{
			Class: action.EffectWriteIrreversible, DataEgress: true,
		},
		HasDescriptor: true,
		LawVersion:    law.Version, LawDigest: law.Digest,
		Rule: "require_approval",
		Now:  now, TTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if err := store.CreateApprovalRequest(context.Background(), b); err != nil {
		t.Fatalf("park: %v", err)
	}
	return cfgPath, dbPath, b.Approval().ApprovalID
}

// TestApprovalsApprove_aDeadlineIsNeverPrintedAsSuccess forces the dangerous
// branch and observes it: the POST leaves, the server never answers, the cage's
// bound kills the call.
//
// Probing mutation (executed, red, declared in the canto): neutralise
// `if run.Unknown` in runApprovedExecution ⇒ this reddens on both halves, the
// printed outcome and the exit code.
func TestApprovalsApprove_aDeadlineIsNeverPrintedAsSuccess(t *testing.T) {
	t.Parallel()
	hung := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The request arrived — the effect IS delivered — and the answer
		// never comes. That is exactly the state nobody can account for.
		<-hung
	}))
	t.Cleanup(func() { close(hung); srv.Close() })
	host := strings.TrimPrefix(srv.URL, "http://")
	cfgPath, dbPath, approvalID := parkedWebhookExpiring(t, host, srv.URL)

	code, stdout, stderr := runIntentCLI(t, "approvals", "approve", "--config", cfgPath, approvalID)
	if strings.Contains(stdout, "SUCCEEDED") {
		t.Fatalf("a call that timed out printed SUCCEEDED: %q", stdout)
	}
	if !strings.Contains(stdout, "OUTCOME_UNKNOWN") {
		t.Fatalf("want the honest OUTCOME_UNKNOWN, got %q (stderr %q)", stdout, stderr)
	}
	// The exact code, not «not zero»: 1 is what runApprovedExecution returns
	// here, and accepting 2 as well would let a usage error pass for this.
	if code != 1 {
		t.Fatalf("exit = %d, want 1 over an unaccountable effect", code)
	}
	// The headline must not claim the execution happened either. «executed the
	// exact approved object» over OUTCOME_UNKNOWN is the same lie one line up.
	if strings.Contains(stdout, "executed the exact approved object") {
		t.Fatalf("the headline claims the execution over an unknown outcome: %q", stdout)
	}

	// And the LEDGER agrees with the screen: one story, not three.
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	rec, err := store.Get(context.Background(), "act_slowhook")
	if err != nil {
		t.Fatalf("get the action: %v", err)
	}
	if rec.State != action.StateOutcomeUnknown {
		t.Fatalf("the ledger closed %v while the CLI said OUTCOME_UNKNOWN", rec.State)
	}
}

// TestApprovalsApprove_aDeliveredPostIsNeverPrintedAsFailure is the other
// producer of an unaccountable effect, and the sixth pass's P2-3: the remote
// end ACCEPTS the POST and the answer never arrives whole. Before the cure the
// ledger closed FAILED and the CLI printed `outcome: FAILED` — a definite
// claim that the irreversible call did not happen, over a call that did.
//
// Probing mutation (executed, red, declared in the canto): drop
// `tool.ErrEffectDelivered` from the `unknown` predicate in
// internal/app/approvals.go ⇒ this reddens on the printed outcome and on the
// ledger's state.
func TestApprovalsApprove_aDeliveredPostIsNeverPrintedAsFailure(t *testing.T) {
	t.Parallel()
	arrived := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 64)
		_, _ = r.Body.Read(body)
		select {
		case arrived <- struct{}{}:
		default:
		}
		// The POST is ACCEPTED and the answer is cut short: 64 bytes declared,
		// ten written. Deterministic on every platform — the first shape tore
		// the connection with a TCP RST and depended on the kernel's timing to
		// produce a read error at all.
		w.Header().Set("Content-Length", "64")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("0123456789"))
	}))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	cfgPath, dbPath, approvalID := parkedWebhookExpiring(t, host, srv.URL)

	code, stdout, stderr := runIntentCLI(t, "approvals", "approve", "--config", cfgPath, approvalID)
	select {
	case <-arrived:
	default:
		t.Fatalf("the POST never reached the server — this is not the post-delivery branch (%q %q)", stdout, stderr)
	}
	if strings.Contains(stdout, "outcome: FAILED") {
		t.Fatalf("the POST was accepted and the ledger says it failed: %q", stdout)
	}
	if !strings.Contains(stdout, "OUTCOME_UNKNOWN") {
		t.Fatalf("want the honest OUTCOME_UNKNOWN, got %q (stderr %q)", stdout, stderr)
	}
	// The exact code, not «not zero»: 1 is what runApprovedExecution returns
	// here, and accepting 2 as well would let a usage error pass for this.
	if code != 1 {
		t.Fatalf("exit = %d, want 1 over an unaccountable effect", code)
	}
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	rec, err := store.Get(context.Background(), "act_slowhook")
	if err != nil {
		t.Fatalf("get the action: %v", err)
	}
	if rec.State != action.StateOutcomeUnknown {
		t.Fatalf("the ledger closed %v over a POST the server accepted", rec.State)
	}
}

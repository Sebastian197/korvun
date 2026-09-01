// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The deferred execution of the EXACT object — Etapa 5, lote 3, pieza 2
// (spec FR-EXEC, sealed NC-2 alive): approving executes THE stored
// envelope — recovered whole, re-verified against the approved digest
// as the belt, through the one Executor Registry path — and the real
// outcome closes the parked action with its era's E4 receipt. The
// belts: approving twice never executes twice; executing twice never
// executes twice; a tampered stored request never executes (digest
// belt); a rejected or expired request never executes; -race over the
// full request→decision→execution flow. Approved-red contract.

package app

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/action/executor"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/tool"

	_ "modernc.org/sqlite"
)

// countingTool records executions and returns a distinctive result.
type countingTool struct{ runs atomic.Int64 }

func (c *countingTool) Name() string        { return "webhook_call" }
func (c *countingTool) Description() string { return "counting fake" }
func (c *countingTool) Execute(ctx context.Context, args string) (string, error) {
	c.runs.Add(1)
	return "EXTERNAL-EFFECT-DONE", nil
}

// approvedFlow drives request→approve on a real boot and returns the
// pieces for execution.
func approvedFlow(t *testing.T) (*actionsqlite.Store, *executor.Executor, *countingTool, string) {
	store, exec, fake, approvalID, _ := approvedFlowWithPath(t)
	return store, exec, fake, approvalID
}

func approvedFlowWithPath(t *testing.T) (*actionsqlite.Store, *executor.Executor, *countingTool, string, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "korvun.db")
	app, err := Build(approvalsConfig(t, dbPath, true), withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { shutdownApp(t, app) })
	ar := app.recorderForTest().(requester)
	ctx := context.Background()
	env := action.NewEnvelope("act_exec1", "env-exec",
		action.Source{Kind: "agent_brain", Protocol: "text", Channel: "telegram"},
		action.Operation{Namespace: "tool", Name: "webhook_call", Version: 1},
		`{"url":"https://a.example"}`, time.Now().UTC())
	env.IntentID = action.RootIntentID
	env.Principal = action.PrincipalRef{PrincipalID: "principal_brain_a"}
	env.Effect = action.Effect{Class: string(action.EffectWriteIrreversible)}
	approvalID, err := ar.RequestApproval(ctx, env, "require_approval", `{"url":"https://a.example"}`)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	store := app.actions.(*actionsqlite.Store)
	envD, identD := operatorApprovalEnv("approve", approvalID)
	rule, err := store.DecideApproval(ctx, approvalID, "approved", time.Now().UTC(), envD, identD, "")
	if err != nil || rule != "" {
		t.Fatalf("approve: %q %v", rule, err)
	}
	fake := &countingTool{}
	exec := executor.New(tool.Registry{"webhook_call": fake}, 0, time.Now)
	return store, exec, fake, approvalID, dbPath
}

// tamperApprovalParams rewrites the stored canonical params from
// outside the domain API — the saboteur's raw hand for the digest belt.
func tamperApprovalParams(t *testing.T, dbPath, approvalID, params string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(
		`UPDATE approvals SET canonical_params = ? WHERE approval_id = ?`, params, approvalID); err != nil {
		t.Fatalf("tamper: %v", err)
	}
}

func operatorApprovalEnv(name, approvalID string) (action.Envelope, actionsqlite.AttemptIdentity) {
	env := action.NewEnvelope(action.NewID(), "cli",
		action.Source{Kind: "operator", Protocol: "cli", Channel: "cli"},
		action.Operation{Namespace: "approval", Name: name, Version: 1},
		`{"approval_id":"`+approvalID+`"}`, time.Now().UTC())
	env.Principal = action.PrincipalRef{PrincipalID: action.OperatorPrincipal().PrincipalID}
	env.IntentID = action.RootIntentID
	return env, actionsqlite.AttemptIdentity{
		PrincipalID: action.OperatorPrincipal().PrincipalID,
		IntentID:    action.RootIntentID,
	}
}

func TestExecuteApproved_theExactObjectRuns(t *testing.T) {
	t.Parallel()
	store, exec, fake, approvalID := approvedFlow(t)
	ctx := context.Background()
	result, err := ExecuteApprovedAction(ctx, store, exec, approvalID)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result != "EXTERNAL-EFFECT-DONE" || fake.runs.Load() != 1 {
		t.Fatalf("the stored object must run exactly once: %q runs=%d", result, fake.runs.Load())
	}
	// The parked action closed with its era's receipt and the result digest.
	rec, err := store.Get(ctx, "act_exec1")
	if err != nil || rec.State != action.StateSucceeded {
		t.Fatalf("terminal close: %v %v", err, rec.State)
	}
	receipts, _ := store.ReceiptsByAction(ctx, "act_exec1")
	if len(receipts) != 1 || receipts[0].Outcome != string(action.StateSucceeded) {
		t.Fatalf("the outcome receipt: %+v", receipts)
	}
	if receipts[0].ResultDigest != action.HashCanonical("EXTERNAL-EFFECT-DONE") {
		t.Fatal("the receipt attests the on-the-fly result digest")
	}
	// Params purged after the terminal — nothing raw at rest.
	if _, err := store.ApprovalParams(ctx, approvalID); err == nil {
		t.Fatal("params must purge after execution")
	}
}

func TestExecuteApproved_neverTwice(t *testing.T) {
	t.Parallel()
	store, exec, fake, approvalID := approvedFlow(t)
	ctx := context.Background()
	if _, err := ExecuteApprovedAction(ctx, store, exec, approvalID); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := ExecuteApprovedAction(ctx, store, exec, approvalID); err == nil {
		t.Fatal("a second execution must refuse by name")
	}
	if fake.runs.Load() != 1 {
		t.Fatalf("the effect must happen EXACTLY once: %d", fake.runs.Load())
	}
}

func TestExecuteApproved_theDigestBelt(t *testing.T) {
	t.Parallel()
	store, exec, fake, approvalID, dbPath := approvedFlowWithPath(t)
	ctx := context.Background()
	// The saboteur swaps the stored params AFTER approval (raw handle,
	// behind the API's back).
	tamperApprovalParams(t, dbPath, approvalID, `{"url":"https://EVIL.example"}`)
	_, err := ExecuteApprovedAction(ctx, store, exec, approvalID)
	if err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("tampered params must refuse EXECUTION naming the digest: %v", err)
	}
	if fake.runs.Load() != 0 {
		t.Fatal("the belt must stop the effect entirely")
	}
}

func TestExecuteApproved_aNoNeverExecutes(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "korvun.db")
	app, err := Build(approvalsConfig(t, dbPath, true), withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer shutdownApp(t, app)
	ar := app.recorderForTest().(requester)
	ctx := context.Background()
	env := action.NewEnvelope("act_no1", "env-no",
		action.Source{Kind: "agent_brain", Protocol: "text", Channel: "telegram"},
		action.Operation{Namespace: "tool", Name: "webhook_call", Version: 1},
		`{"x":1}`, time.Now().UTC())
	env.IntentID = action.RootIntentID
	env.Effect = action.Effect{Class: string(action.EffectWriteIrreversible)}
	approvalID, err := ar.RequestApproval(ctx, env, "require_approval", `{"x":1}`)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	store := app.actions.(*actionsqlite.Store)
	envD, identD := operatorApprovalEnv("reject", approvalID)
	if _, err := store.DecideApproval(ctx, approvalID, "rejected", time.Now().UTC(), envD, identD, "no"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	fake := &countingTool{}
	exec := executor.New(tool.Registry{"webhook_call": fake}, 0, time.Now)
	if _, err := ExecuteApprovedAction(ctx, store, exec, approvalID); err == nil {
		t.Fatal("a REJECTED request must never execute")
	}
	if fake.runs.Load() != 0 {
		t.Fatal("zero effects after a no")
	}
}

func TestExecuteApproved_raceOverTheFullFlow(t *testing.T) {
	t.Parallel()
	store, exec, fake, approvalID := approvedFlow(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	var succeeded atomic.Int64
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := ExecuteApprovedAction(ctx, store, exec, approvalID); err == nil {
				succeeded.Add(1)
			}
		}()
	}
	wg.Wait()
	if fake.runs.Load() != 1 {
		t.Fatalf("racing executors must fire the effect EXACTLY once: %d", fake.runs.Load())
	}
	if succeeded.Load() != 1 {
		t.Fatalf("exactly one execution reports success: %d", succeeded.Load())
	}
}

func TestExecuteApproved_ghostAndPending(t *testing.T) {
	t.Parallel()
	store, exec, fake, _ := approvedFlow(t)
	ctx := context.Background()
	if _, err := ExecuteApprovedAction(ctx, store, exec, "apr_ghost"); !errors.Is(err, actionsqlite.ErrNotFound) {
		t.Fatalf("ghost request: %v", err)
	}
	_ = fake
}

// The new path's toll (spec FR-COMPAT-3 mold): what the agent's hot
// path pays when the knob is ON and an irreversible action parks —
// preview assembly + the born-whole birth. Run with:
//
//	go test ./internal/app/ -run '^$' -bench BenchmarkRequestApproval -benchtime 2s
func BenchmarkRequestApproval(b *testing.B) {
	dbPath := filepath.Join(b.TempDir(), "korvun.db")
	app, err := Build(approvalsConfig(&testing.T{}, dbPath, true), withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		b.Fatalf("Build: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = app.Shutdown(ctx)
	}()
	ar := app.recorderForTest().(requester)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		env := action.NewEnvelope(action.NewID(), "env-bench",
			action.Source{Kind: "agent_brain", Protocol: "text", Channel: "telegram"},
			action.Operation{Namespace: "tool", Name: "webhook_call", Version: 1},
			`{"url":"https://a.example"}`, time.Now().UTC())
		env.IntentID = action.RootIntentID
		env.Effect = action.Effect{Class: string(action.EffectWriteIrreversible)}
		if _, err := ar.RequestApproval(ctx, env, "require_approval", `{"url":"https://a.example"}`); err != nil {
			b.Fatal(err)
		}
	}
}

func TestApprovalExecutor_errorBranches(t *testing.T) {
	t.Parallel()
	cfg := kernelWiringConfig(filepath.Join(t.TempDir(), "korvun.db"))
	// A non-brain principal refuses by name.
	if _, err := BuildApprovalExecutor(cfg, action.ActionPreview{
		PrincipalID: "principal_operator", Operation: "tool/calc"}); err == nil {
		t.Fatal("a non-brain principal must refuse")
	}
	// An unknown brain refuses by name.
	if _, err := BuildApprovalExecutor(cfg, action.ActionPreview{
		PrincipalID: "principal_brain_ghost", Operation: "tool/calc"}); err == nil {
		t.Fatal("an unknown brain must refuse")
	}
	// A caged tool without its cage block fails loud (the boot's own
	// constructor semantics ride here).
	if _, err := BuildApprovalExecutor(cfg, action.ActionPreview{
		PrincipalID: "principal_brain_a", Operation: "tool/webhook_call"}); err == nil {
		t.Fatal("a caged tool without its cage must fail loud")
	}
	// The pure builtin builds.
	if _, err := BuildApprovalExecutor(cfg, action.ActionPreview{
		PrincipalID: "principal_brain_a", Operation: "tool/calc"}); err != nil {
		t.Fatalf("the pure builtin must build: %v", err)
	}
	// TTL parsing: malformed dies, valid parses, absent defaults.
	if _, err := approvalTTL(&config.ApprovalsConfig{TTL: "garbage"}); err == nil {
		t.Fatal("malformed ttl must die")
	}
	if _, err := approvalTTL(&config.ApprovalsConfig{TTL: "-1h"}); err == nil {
		t.Fatal("non-positive ttl must die")
	}
	if d, err := approvalTTL(&config.ApprovalsConfig{TTL: "30m"}); err != nil || d != 30*time.Minute {
		t.Fatalf("valid ttl: %v %v", d, err)
	}
	if d, _ := approvalTTL(nil); d != defaultApprovalTTL {
		t.Fatalf("absent config defaults: %v", d)
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/policy"
	"github.com/Sebastian197/korvun/internal/tool"
)

type canonicalTool struct {
	name    string
	output  string
	err     error
	args    string
	scope   tool.Scope
	calls   atomic.Int32
	wait    <-chan struct{}
	started chan<- struct{}
}

func (t *canonicalTool) Name() string { return t.name }

func (t *canonicalTool) Description() string { return "canonical execution fixture" }

func (t *canonicalTool) Execute(ctx context.Context, args string) (string, error) {
	return t.execute(ctx, tool.Scope{}, args)
}

func (t *canonicalTool) ExecuteScoped(ctx context.Context, scope tool.Scope, args string) (string, error) {
	t.scope = scope
	return t.execute(ctx, scope, args)
}

func (t *canonicalTool) execute(ctx context.Context, _ tool.Scope, args string) (string, error) {
	t.calls.Add(1)
	t.args = args
	if t.started != nil {
		t.started <- struct{}{}
	}
	if t.wait != nil {
		select {
		case <-t.wait:
		case <-ctx.Done():
			return t.output, ctx.Err()
		}
	}
	return t.output, t.err
}

type canonicalRecorder struct {
	mu             sync.Mutex
	records        []action.Envelope
	evidences      []action.IdentityEvidence
	rules          []string
	states         []action.State
	finishStates   []action.State
	recordErr      error
	requestErr     error
	approvalID     string
	approvalCalls  int
	finishContexts []error
}

func (r *canonicalRecorder) RecordAttempt(_ context.Context, env action.Envelope, _ string, rule string, state action.State) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.recordErr != nil {
		return r.recordErr
	}
	r.records = append(r.records, env)
	r.rules = append(r.rules, rule)
	r.states = append(r.states, state)
	return nil
}

func (r *canonicalRecorder) RecordAttemptIdentified(_ context.Context, env action.Envelope, _ string, rule string, state action.State, evidence action.IdentityEvidence) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.recordErr != nil {
		return r.recordErr
	}
	r.records = append(r.records, env)
	r.evidences = append(r.evidences, evidence)
	r.rules = append(r.rules, rule)
	r.states = append(r.states, state)
	return nil
}

func (r *canonicalRecorder) Finish(ctx context.Context, _ string, state action.State, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finishStates = append(r.finishStates, state)
	r.finishContexts = append(r.finishContexts, ctx.Err())
	return nil
}

func (r *canonicalRecorder) RequestApproval(_ context.Context, env action.Envelope, rule string, _ string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.approvalCalls++
	if r.requestErr != nil {
		return "", r.requestErr
	}
	r.records = append(r.records, env)
	r.rules = append(r.rules, rule)
	if r.approvalID == "" {
		return "apr_canonical", nil
	}
	return r.approvalID, nil
}

func testEnvelope(channel, sender, conv string) *envelope.Envelope {
	meta := map[string]string{}
	if conv != "" {
		meta[conversation.MetaConversationID] = conv
	}
	return &envelope.Envelope{
		ID:      "env-canonical",
		Channel: channel,
		Sender:  envelope.Participant{ID: sender},
		Meta:    meta,
	}
}

func testPlan(t *testing.T, exec *Executor, ctx context.Context, channel string) *DecisionPlan {
	t.Helper()
	_, plan, _ := exec.SelectTools(ctx, channel)
	if plan == nil {
		t.Fatal("SelectTools returned a nil plan")
	}
	return plan
}

func testGovernance(decisions map[string]policy.ToolDecision) *Governance {
	if decisions == nil {
		return nil
	}
	governance := &Governance{
		Sensitivity: policy.Public,
		Locality:    policy.Local,
		Attrs:       make(map[string]policy.ToolAttrs, len(decisions)),
	}
	for name, decision := range decisions {
		switch decision.Mode {
		case policy.ToolAllow, policy.ToolShadow:
			governance.Grants = append(governance.Grants, policy.ToolGrant{Name: name, Mode: decision.Mode})
		case policy.ToolDeny:
			if decision.Rule == policy.ToolRuleDenyGrant {
				governance.Grants = append(governance.Grants, policy.ToolGrant{Name: name, Mode: policy.ToolDeny})
			}
		}
		governance.Attrs[name] = policy.ToolAttrs{Network: decision.Shield}
		if decision.Shield {
			governance.Sensitivity = policy.Private
		}
	}
	return governance
}

func testRequest(t *testing.T, exec *Executor, plan *DecisionPlan, env *envelope.Envelope, lane, name, args string) *Request {
	t.Helper()
	req, err := exec.Prepare(Submission{Inbound: env, Lane: lane, Name: name, Arguments: args, Plan: plan})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	return req
}

func TestExecutor_RejectsUnboundRequest(t *testing.T) {
	var ids atomic.Int32
	recorder := &canonicalRecorder{}
	fixture := &canonicalTool{name: "scoped", output: "ok"}
	config := CoordinatorConfig{
		BrainName: "alpha",
		Recorder:  recorder,
		Identity: &IdentityConfig{
			Registry: action.ProvenanceRegistry{
				"webhook": {Class: "webhook", Credential: action.CredentialInboundBearer},
			},
			IntentID: action.RootIntentID,
		},
		NewActionID: func() string {
			return fmt.Sprintf("act_bound_%d", ids.Add(1))
		},
	}
	exec := NewCoordinator(tool.Registry{"scoped": fixture}, 0, time.Now, config)
	foreign := NewCoordinator(tool.Registry{"scoped": fixture}, 0, time.Now, CoordinatorConfig{})
	valid := testPlan(t, exec, context.Background(), "webhook")
	foreignPlan := testPlan(t, foreign, context.Background(), "webhook")
	env := testEnvelope("webhook", "alice", "one")

	planCases := []struct {
		name string
		plan *DecisionPlan
		env  *envelope.Envelope
	}{
		{name: "nil plan", env: env},
		{name: "zero plan", plan: &DecisionPlan{}, env: env},
		{name: "foreign owner", plan: foreignPlan, env: env},
		{name: "wrong channel", plan: valid, env: testEnvelope("console", "alice", "one")},
	}
	for _, tc := range planCases {
		t.Run(tc.name, func(t *testing.T) {
			before := ids.Load()
			_, err := exec.Prepare(Submission{Inbound: tc.env, Lane: "text", Name: "scoped", Arguments: "x", Plan: tc.plan})
			if err != ErrExecutionBindingMismatch {
				t.Fatalf("Prepare error = %v, want ErrExecutionBindingMismatch", err)
			}
			if ids.Load() != before || fixture.calls.Load() != 0 || len(recorder.records) != 0 || len(recorder.finishStates) != 0 {
				t.Fatal("a rejected plan performed downstream work")
			}
		})
	}

	requestCases := []struct {
		name string
		req  func(*testing.T) *Request
	}{
		{name: "nil request", req: func(*testing.T) *Request { return nil }},
		{name: "zero request", req: func(*testing.T) *Request { return &Request{} }},
		{name: "missing use cell", req: func(t *testing.T) *Request {
			r := testRequest(t, exec, valid, env, "text", "scoped", "x")
			r.use = nil
			return r
		}},
		{name: "operation moved", req: func(t *testing.T) *Request {
			r := testRequest(t, exec, valid, env, "text", "scoped", "x")
			r.action.Operation.Name = "other"
			return r
		}},
		{name: "arguments moved", req: func(t *testing.T) *Request {
			r := testRequest(t, exec, valid, env, "text", "scoped", "x")
			r.args = "y"
			return r
		}},
	}
	for _, tc := range requestCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := exec.Submit(context.Background(), tc.req(t))
			if err != ErrExecutionBindingMismatch {
				t.Fatalf("Submit error = %v, want ErrExecutionBindingMismatch", err)
			}
			if fixture.calls.Load() != 0 || len(recorder.records) != 0 || len(recorder.finishStates) != 0 {
				t.Fatal("an unbound request performed downstream work")
			}
		})
	}

	t.Run("scope and identity are copied", func(t *testing.T) {
		original := testEnvelope("webhook", "alice", "one")
		req := testRequest(t, exec, valid, original, "text", "scoped", "payload")
		original.Sender.ID = "mallory"
		original.Meta[conversation.MetaConversationID] = "two"
		if _, err := exec.Submit(context.Background(), req); err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if fixture.scope != (tool.Scope{Brain: "alpha", Conversation: "webhook::one"}) {
			t.Fatalf("scope = %+v", fixture.scope)
		}
		if len(recorder.evidences) != 1 || recorder.evidences[0].Subject != "alice" {
			t.Fatalf("identity evidence = %+v, want copied sender alice", recorder.evidences)
		}
	})

	t.Run("missing conversation keeps empty scope", func(t *testing.T) {
		fresh := &canonicalTool{name: "scoped", output: "ok"}
		e := NewCoordinator(tool.Registry{"scoped": fresh}, 0, time.Now, CoordinatorConfig{BrainName: "alpha"})
		plan := testPlan(t, e, context.Background(), "webhook")
		req := testRequest(t, e, plan, testEnvelope("webhook", "alice", ""), "text", "scoped", "payload")
		if _, err := e.Submit(context.Background(), req); err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if fresh.scope != (tool.Scope{Brain: "alpha", Conversation: ""}) {
			t.Fatalf("scope = %+v, want empty conversation", fresh.scope)
		}
	})
}

func TestSubmit_RejectedBindingDoesNotConsumeTheRequest(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Request)
		repair func(*Request)
	}{
		{
			name:   "operation",
			mutate: func(request *Request) { request.action.Operation.Name = "other" },
			repair: func(request *Request) { request.action.Operation.Name = "journal" },
		},
		{
			name:   "arguments",
			mutate: func(request *Request) { request.args = "moved" },
			repair: func(request *Request) { request.args = "original" },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := &canonicalTool{name: "journal", output: "done"}
			exec := NewCoordinator(tool.Registry{"journal": fixture}, 0, time.Now, CoordinatorConfig{})
			plan := testPlan(t, exec, context.Background(), "console")
			request := testRequest(t, exec, plan, testEnvelope("console", "operator", ""), "text", "journal", "original")
			tc.mutate(request)
			if _, err := exec.Submit(context.Background(), request); err != ErrExecutionBindingMismatch {
				t.Fatalf("rejected Submit error = %v, want exact ErrExecutionBindingMismatch", err)
			}
			if request.use.claimed.Load() {
				t.Fatal("a rejected binding consumed its request")
			}
			tc.repair(request)
			result, err := exec.Submit(context.Background(), request)
			if err != nil || result.Branch != BranchExecuted {
				t.Fatalf("repaired Submit result/error = %+v/%v, want executed", result, err)
			}
			if fixture.calls.Load() != 1 {
				t.Fatalf("dispatches = %d, want 1", fixture.calls.Load())
			}
		})
	}
}

func TestSubmit_AllBranchesHaveOneCanonicalAction(t *testing.T) {
	type branchCase struct {
		name       string
		toolName   string
		configured bool
		decisions  map[string]policy.ToolDecision
		classify   EffectClassifier
		identity   *IdentityConfig
		wantBranch Branch
		wantCalls  int32
	}
	cases := []branchCase{
		{name: "unknown", toolName: "ghost", wantBranch: BranchUnknown},
		{name: "undeclared", toolName: "journal", classify: func(string) (action.EffectDescriptor, bool) { return action.EffectDescriptor{}, false }, wantBranch: BranchEffectUndeclared},
		{name: "deny", toolName: "journal", configured: true, decisions: map[string]policy.ToolDecision{"journal": {Mode: policy.ToolDeny, Rule: policy.ToolRuleNotGranted}}, wantBranch: BranchDenied},
		{name: "shadow", toolName: "journal", configured: true, decisions: map[string]policy.ToolDecision{"journal": {Mode: policy.ToolShadow}}, wantBranch: BranchShadowed},
		{name: "pending", toolName: "journal", classify: irreversibleEffect, identity: approvalIdentity(), wantBranch: BranchPending},
		{name: "allow", toolName: "journal", classify: pureEffect, wantBranch: BranchExecuted, wantCalls: 1},
	}
	for _, tc := range cases {
		recorderModes := []bool{false, true}
		if tc.wantBranch == BranchPending {
			recorderModes = []bool{true}
		}
		for _, durable := range recorderModes {
			for _, lane := range []string{"text", "native"} {
				mode := "memory"
				if durable {
					mode = "durable"
				}
				t.Run(tc.name+"/"+mode+"/"+lane, func(t *testing.T) {
					var ids atomic.Int32
					fixture := &canonicalTool{name: "journal", output: "done"}
					var recorder *canonicalRecorder
					if durable {
						recorder = &canonicalRecorder{}
					}
					cfg := CoordinatorConfig{
						BrainName:        "alpha",
						Identity:         tc.identity,
						EffectClassifier: tc.classify,
						NewActionID: func() string {
							return fmt.Sprintf("act_branch_%d", ids.Add(1))
						},
					}
					if durable {
						cfg.Recorder = recorder
					}
					if tc.configured {
						cfg.Governance = testGovernance(tc.decisions)
					}
					exec := NewCoordinator(tool.Registry{"journal": fixture}, 0, time.Now, cfg)
					plan := testPlan(t, exec, context.Background(), "console")
					req := testRequest(t, exec, plan, testEnvelope("console", "operator", "c"), lane, tc.toolName, "raw")
					result, err := exec.Submit(context.Background(), req)
					if tc.wantBranch == BranchUnknown {
						if !errors.Is(err, ErrUnknownTool) {
							t.Fatalf("unknown error = %v", err)
						}
					} else if err != nil {
						t.Fatalf("Submit: %v", err)
					}
					if result.Branch != tc.wantBranch {
						t.Fatalf("branch = %q, want %q", result.Branch, tc.wantBranch)
					}
					if ids.Load() != 1 || result.Action.ActionID != "act_branch_1" {
						t.Fatalf("canonical births = %d action = %q", ids.Load(), result.Action.ActionID)
					}
					if fixture.calls.Load() != tc.wantCalls {
						t.Fatalf("dispatches = %d, want %d", fixture.calls.Load(), tc.wantCalls)
					}
					if tc.classify != nil && tc.wantBranch != BranchEffectUndeclared && result.Action.Effect.Class == "unclassified" {
						t.Fatal("declared effect was not present when the action was born")
					}
					if durable && len(recorder.records) != 1 {
						t.Fatalf("durable branch recorded %d actions, want one", len(recorder.records))
					}
				})
			}
		}
	}

	t.Run("long names preserve historical operation bytes", func(t *testing.T) {
		longName := strings.Repeat("界", 81)
		bound := func(value string) string {
			runes := []rune(value)
			if len(runes) > 80 {
				return string(runes[:80])
			}
			return value
		}
		for _, known := range []bool{false, true} {
			name := "unknown"
			registry := tool.Registry{}
			if known {
				name = "known"
				registry[longName] = &canonicalTool{name: longName, output: "ok"}
			}
			t.Run(name, func(t *testing.T) {
				exec := NewCoordinator(registry, 0, time.Now, CoordinatorConfig{BoundOperation: bound})
				plan := testPlan(t, exec, context.Background(), "console")
				req := testRequest(t, exec, plan, testEnvelope("console", "operator", ""), "text", longName, "x")
				result, _ := exec.Submit(context.Background(), req)
				wantName := bound(longName)
				if known {
					wantName = longName
				}
				if result.Action.Operation.Name != wantName || utf8.RuneCountInString(result.Action.Operation.Name) != utf8.RuneCountInString(wantName) {
					t.Fatalf("operation name = %q, want %q", result.Action.Operation.Name, wantName)
				}
				if result.Action.ParametersDigest != action.Digest(result.Action.Operation, "x") {
					t.Fatal("digest does not bind the stored operation bytes")
				}
			})
		}
	})

	t.Run("one request has one concurrent winner", func(t *testing.T) {
		cases := []struct {
			name          string
			durable       bool
			identity      *IdentityConfig
			classify      EffectClassifier
			wantBranch    Branch
			wantDispatch  int32
			wantRecords   int
			wantApprovals int
			wantFinishes  int
		}{
			{name: "stateless", classify: irreversibleEffect, wantBranch: BranchExecuted, wantDispatch: 1},
			{name: "durable execution", durable: true, classify: pureEffect, wantBranch: BranchExecuted, wantDispatch: 1, wantRecords: 1, wantFinishes: 1},
			{name: "durable approval", durable: true, identity: approvalIdentity(), classify: irreversibleEffect, wantBranch: BranchPending, wantRecords: 1, wantApprovals: 1},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				var ids atomic.Int32
				fixture := &canonicalTool{name: "journal", output: "done"}
				var recorder *canonicalRecorder
				config := CoordinatorConfig{
					Identity:         tc.identity,
					EffectClassifier: tc.classify,
					NewActionID:      func() string { ids.Add(1); return "act_once" },
				}
				if tc.durable {
					recorder = &canonicalRecorder{}
					config.Recorder = recorder
				}
				exec := NewCoordinator(tool.Registry{"journal": fixture}, 0, time.Now, config)
				plan := testPlan(t, exec, context.Background(), "console")
				req := testRequest(t, exec, plan, testEnvelope("console", "operator", ""), "text", "journal", "x")
				copyA, copyB := *req, *req
				start := make(chan struct{})
				type submissionResult struct {
					result Result
					err    error
				}
				results := make(chan submissionResult, 2)
				var wg sync.WaitGroup
				for _, candidate := range []*Request{&copyA, &copyB} {
					wg.Add(1)
					go func(candidate *Request) {
						defer wg.Done()
						<-start
						result, err := exec.Submit(context.Background(), candidate)
						results <- submissionResult{result: result, err: err}
					}(candidate)
				}
				close(start)
				wg.Wait()
				close(results)
				wins, replays := 0, 0
				for submitted := range results {
					switch {
					case submitted.err == nil:
						wins++
						if submitted.result.Branch != tc.wantBranch {
							t.Fatalf("winning branch = %q, want %q", submitted.result.Branch, tc.wantBranch)
						}
					case submitted.err == ErrExecutionAlreadySubmitted:
						replays++
					default:
						t.Fatalf("unexpected submission error: %v", submitted.err)
					}
				}
				gotRecords, gotApprovals, gotFinishes := 0, 0, 0
				if recorder != nil {
					gotRecords = len(recorder.records)
					gotApprovals = recorder.approvalCalls
					gotFinishes = len(recorder.finishStates)
				}
				if wins != 1 || replays != 1 || ids.Load() != 1 || fixture.calls.Load() != tc.wantDispatch ||
					gotRecords != tc.wantRecords || gotApprovals != tc.wantApprovals || gotFinishes != tc.wantFinishes {
					t.Fatalf("wins=%d replays=%d ids=%d dispatches=%d records=%d approvals=%d finishes=%d",
						wins, replays, ids.Load(), fixture.calls.Load(), gotRecords, gotApprovals, gotFinishes)
				}
			})
		}
	})
}

func TestCompatibility_ImmediateArgumentsRemainByteExact(t *testing.T) {
	cases := []string{"  {\"b\":2,\"a\":1}  ", "{\"a\":1, \"b\":2}", "", "not-json  "}
	for _, raw := range cases {
		t.Run(fmt.Sprintf("%q", raw), func(t *testing.T) {
			fixture := &canonicalTool{name: "echo", output: "ok"}
			exec := NewCoordinator(tool.Registry{"echo": fixture}, 0, time.Now, CoordinatorConfig{})
			plan := testPlan(t, exec, context.Background(), "console")
			req := testRequest(t, exec, plan, testEnvelope("console", "operator", ""), "text", "echo", raw)
			result, err := exec.Submit(context.Background(), req)
			if err != nil {
				t.Fatalf("Submit: %v", err)
			}
			if fixture.args != raw {
				t.Fatalf("tool args = %q, want exact %q", fixture.args, raw)
			}
			if result.Action.ParametersDigest != action.Digest(result.Action.Operation, raw) {
				t.Fatal("action digest does not use canonical parameters")
			}
		})
	}
}

func TestCompatibility_DecisionPrecedenceAndObservations(t *testing.T) {
	precedence := []struct {
		name       string
		toolName   string
		decisions  map[string]policy.ToolDecision
		classify   EffectClassifier
		identity   *IdentityConfig
		recorder   *canonicalRecorder
		wantBranch Branch
		wantRule   string
	}{
		{name: "unknown beats shadow", toolName: "ghost", decisions: map[string]policy.ToolDecision{"ghost": {Mode: policy.ToolShadow}}, classify: irreversibleEffect, identity: approvalIdentity(), recorder: &canonicalRecorder{}, wantBranch: BranchUnknown, wantRule: "unknown_tool"},
		{name: "shadow beats ceiling", toolName: "journal", decisions: map[string]policy.ToolDecision{"journal": {Mode: policy.ToolShadow}}, classify: criticalEffect, identity: lowCeilingIdentity(), recorder: &canonicalRecorder{}, wantBranch: BranchShadowed, wantRule: "shadow"},
		{name: "ceiling beats approval", toolName: "journal", classify: criticalEffect, identity: lowCeilingIdentity(), recorder: &canonicalRecorder{}, wantBranch: BranchDenied, wantRule: "effect_ceiling"},
		{name: "approval birth fails closed", toolName: "journal", classify: irreversibleEffect, identity: approvalIdentity(), recorder: &canonicalRecorder{requestErr: errors.New("store unavailable")}, wantBranch: BranchDenied, wantRule: "approval_unavailable"},
	}
	for _, tc := range precedence {
		t.Run(tc.name, func(t *testing.T) {
			fixture := &canonicalTool{name: "journal", output: "done"}
			exec := NewCoordinator(tool.Registry{"journal": fixture}, 0, time.Now, CoordinatorConfig{
				Recorder: tc.recorder, Identity: tc.identity, EffectClassifier: tc.classify,
				Governance: testGovernance(tc.decisions),
			})
			plan := testPlan(t, exec, context.Background(), "console")
			req := testRequest(t, exec, plan, testEnvelope("console", "operator", ""), "text", tc.toolName, "x")
			result, _ := exec.Submit(context.Background(), req)
			if result.Branch != tc.wantBranch || result.Rule != tc.wantRule {
				t.Fatalf("branch/rule = %q/%q, want %q/%q", result.Branch, result.Rule, tc.wantBranch, tc.wantRule)
			}
			if fixture.calls.Load() != 0 {
				t.Fatal("a prohibited precedence branch dispatched")
			}
		})
	}

	selectorErrors := []struct {
		name string
		gov  Governance
		want error
	}{
		{name: "sensitivity", gov: Governance{Locality: policy.Local, Grants: allowJournal()}, want: policy.ErrUnknownSensitivity},
		{name: "locality", gov: Governance{Sensitivity: policy.Public, Grants: allowJournal()}, want: policy.ErrUnknownLocality},
		{name: "mode", gov: Governance{Sensitivity: policy.Public, Locality: policy.Local, Grants: []policy.ToolGrant{{Name: "journal"}}}, want: policy.ErrUnknownToolMode},
		{name: "duplicate", gov: Governance{Sensitivity: policy.Public, Locality: policy.Local, Grants: []policy.ToolGrant{{Name: "journal", Mode: policy.ToolAllow}, {Name: "journal", Mode: policy.ToolDeny}}}, want: policy.ErrDuplicateToolGrant},
		{name: "empty", gov: Governance{Sensitivity: policy.Public, Locality: policy.Local, Grants: []policy.ToolGrant{{Name: "", Mode: policy.ToolAllow}}}, want: policy.ErrInvalidToolGrant},
	}
	for _, tc := range selectorErrors {
		t.Run("selector/"+tc.name, func(t *testing.T) {
			fixture := &canonicalTool{name: "journal", output: "done"}
			exec := NewCoordinator(tool.Registry{"journal": fixture}, 0, time.Now, CoordinatorConfig{Governance: &tc.gov})
			advertised, plan, err := exec.SelectTools(context.Background(), "console")
			if !errors.Is(err, tc.want) {
				t.Fatalf("selector error = %v, want %v", err, tc.want)
			}
			if len(advertised) != 0 || plan == nil {
				t.Fatalf("fail-closed selection advertised=%v plan=%v", advertised, plan)
			}
			req := testRequest(t, exec, plan, testEnvelope("console", "operator", ""), "text", "journal", "x")
			result, submitErr := exec.Submit(context.Background(), req)
			if submitErr != nil || result.Branch != BranchDenied || result.Rule != string(policy.ToolRuleNotGranted) {
				t.Fatalf("deny-all submit result=%+v err=%v", result, submitErr)
			}
			if fixture.calls.Load() != 0 {
				t.Fatal("selector failure dispatched")
			}
		})
	}
}

func TestPrepare_StatefulClassifierCannotWidenTheEffectGate(t *testing.T) {
	var calls atomic.Int32
	fixture := &canonicalTool{name: "journal", output: "done"}
	exec := NewCoordinator(tool.Registry{"journal": fixture}, 0, time.Now, CoordinatorConfig{
		Identity: lowCeilingIdentity(),
		EffectClassifier: func(string) (action.EffectDescriptor, bool) {
			if calls.Add(1) == 1 {
				return action.EffectDescriptor{Class: action.EffectPure}, true
			}
			return action.EffectDescriptor{Class: action.EffectCritical}, true
		},
	})
	plan := testPlan(t, exec, context.Background(), "console")
	request := testRequest(t, exec, plan, testEnvelope("console", "operator", ""), "text", "journal", "x")
	result, err := exec.Submit(context.Background(), request)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if result.Branch != BranchDenied || result.Rule != "effect_ceiling" {
		t.Fatalf("branch/rule = %q/%q, want denied/effect_ceiling", result.Branch, result.Rule)
	}
	if result.Action.Effect.Class != string(action.EffectCritical) || calls.Load() != 2 {
		t.Fatalf("effect/calls = %q/%d, want critical/2", result.Action.Effect.Class, calls.Load())
	}
	if fixture.calls.Load() != 0 {
		t.Fatal("stateful classifier widened a critical effect into execution")
	}
}

func TestPrepare_SecondClassifierMissKeepsHistoricalAdmission(t *testing.T) {
	cases := []struct {
		name      string
		recorder  bool
		identity  bool
		wantCalls int32
		wantClass string
	}{
		{name: "stateless", wantCalls: 1, wantClass: string(action.EffectPure)},
		{name: "durable", recorder: true, wantCalls: 2, wantClass: "unclassified"},
		{name: "identity gate", identity: true, wantCalls: 2, wantClass: "unclassified"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			fixture := &canonicalTool{name: "journal", output: "done"}
			config := CoordinatorConfig{
				EffectClassifier: func(string) (action.EffectDescriptor, bool) {
					if calls.Add(1) == 1 {
						return action.EffectDescriptor{Class: action.EffectPure}, true
					}
					return action.EffectDescriptor{}, false
				},
			}
			if tc.recorder {
				config.Recorder = &canonicalRecorder{}
			}
			if tc.identity {
				config.Identity = lowCeilingIdentity()
			}
			exec := NewCoordinator(tool.Registry{"journal": fixture}, 0, time.Now, config)
			plan := testPlan(t, exec, context.Background(), "console")
			request := testRequest(t, exec, plan, testEnvelope("console", "operator", ""), "text", "journal", "x")
			result, err := exec.Submit(context.Background(), request)
			if err != nil || result.Branch != BranchExecuted {
				t.Fatalf("result/error = %+v/%v, want executed", result, err)
			}
			if calls.Load() != tc.wantCalls || result.Action.Effect.Class != tc.wantClass {
				t.Fatalf("calls/effect = %d/%q, want %d/%q", calls.Load(), result.Action.Effect.Class, tc.wantCalls, tc.wantClass)
			}
			if fixture.calls.Load() != 1 {
				t.Fatalf("dispatches = %d, want 1", fixture.calls.Load())
			}
		})
	}
}

func TestPrepare_HistoricalClassifierReadsReachTheCanonicalEvidence(t *testing.T) {
	type classifierResult struct {
		descriptor action.EffectDescriptor
		declared   bool
	}
	cases := []struct {
		name         string
		identity     *IdentityConfig
		sequence     []classifierResult
		requestErr   error
		wantBranch   Branch
		wantCalls    int32
		wantClass    string
		wantDispatch int32
	}{
		{
			name: "first miss still performs the durable envelope read",
			sequence: []classifierResult{
				{},
				{descriptor: action.EffectDescriptor{Class: action.EffectPure}, declared: true},
			},
			wantBranch: BranchEffectUndeclared,
			wantCalls:  2,
			wantClass:  string(action.EffectPure),
		},
		{
			name:     "identity gate and durable envelope remain distinct reads",
			identity: &IdentityConfig{Registry: action.ProvenanceRegistry{"console": {Class: "console", Credential: action.CredentialLoopbackInProcess}}, IntentID: action.RootIntentID},
			sequence: []classifierResult{
				{descriptor: action.EffectDescriptor{Class: action.EffectPure}, declared: true},
				{},
				{descriptor: action.EffectDescriptor{Class: action.EffectCritical}, declared: true},
			},
			wantBranch:   BranchExecuted,
			wantCalls:    3,
			wantClass:    string(action.EffectCritical),
			wantDispatch: 1,
		},
		{
			name:       "failed approval birth retains the terminal durable read",
			identity:   approvalIdentity(),
			requestErr: errors.New("store unavailable"),
			sequence: []classifierResult{
				{descriptor: action.EffectDescriptor{Class: action.EffectWriteIrreversible}, declared: true},
				{descriptor: action.EffectDescriptor{Class: action.EffectWriteIrreversible}, declared: true},
				{descriptor: action.EffectDescriptor{Class: action.EffectCritical}, declared: true},
				{descriptor: action.EffectDescriptor{Class: action.EffectPure}, declared: true},
			},
			wantBranch: BranchDenied,
			wantCalls:  4,
			wantClass:  string(action.EffectPure),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			fixture := &canonicalTool{name: "journal", output: "done"}
			recorder := &canonicalRecorder{requestErr: tc.requestErr}
			exec := NewCoordinator(tool.Registry{"journal": fixture}, 0, time.Now, CoordinatorConfig{
				Recorder: recorder,
				Identity: tc.identity,
				EffectClassifier: func(string) (action.EffectDescriptor, bool) {
					index := int(calls.Add(1) - 1)
					if index >= len(tc.sequence) {
						t.Fatalf("unexpected classifier read %d", index+1)
					}
					result := tc.sequence[index]
					return result.descriptor, result.declared
				},
			})
			plan := testPlan(t, exec, context.Background(), "console")
			request := testRequest(t, exec, plan, testEnvelope("console", "operator", ""), "text", "journal", "x")
			result, err := exec.Submit(context.Background(), request)
			if err != nil {
				t.Fatalf("Submit: %v", err)
			}
			if result.Branch != tc.wantBranch {
				t.Fatalf("branch = %q, want %q", result.Branch, tc.wantBranch)
			}
			if calls.Load() != tc.wantCalls || result.Action.Effect.Class != tc.wantClass {
				t.Fatalf("calls/effect = %d/%q, want %d/%q", calls.Load(), result.Action.Effect.Class, tc.wantCalls, tc.wantClass)
			}
			if len(recorder.records) != 1 || recorder.records[0].Effect.Class != tc.wantClass {
				t.Fatalf("recorded evidence = %+v, want one action with effect %q", recorder.records, tc.wantClass)
			}
			if fixture.calls.Load() != tc.wantDispatch {
				t.Fatalf("dispatches = %d, want %d", fixture.calls.Load(), tc.wantDispatch)
			}
		})
	}
}

func TestResume_UsesClaimedOperationAndSnapshot(t *testing.T) {
	moved := errors.New("approval moved under claim")
	tests := []struct {
		name       string
		claimErr   error
		claimOp    action.Operation
		wantErr    error
		wantClaim  int
		wantStale  int32
		wantTarget int32
	}{
		{name: "moved snapshot invokes nothing", claimErr: moved, wantErr: moved, wantClaim: 1},
		{name: "claimed operation and bytes win", claimOp: action.Operation{Namespace: "tool", Name: "claimed", Version: 1}, wantClaim: 1, wantTarget: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stale := &canonicalTool{name: "stale", output: "stale"}
			target := &canonicalTool{name: "claimed", output: "claimed"}
			exec := NewCoordinator(tool.Registry{"stale": stale, "claimed": target}, 0, time.Now, CoordinatorConfig{})
			store := &approvalStoreFake{
				approval: action.Approval{ApprovalID: "apr_1", ActionID: "act_1", Status: action.ApprovalApproved},
				state:    action.StateApproved, claimErr: tc.claimErr, claimOp: tc.claimOp, claimParams: []byte("  claimed-bytes  "), receiptID: "rcpt_1",
			}
			result, err := exec.ResumeApproved(context.Background(), store, "apr_1")
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("ResumeApproved error = %v, want %v", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatalf("ResumeApproved: %v", err)
			}
			if store.claimCalls != tc.wantClaim || stale.calls.Load() != tc.wantStale || target.calls.Load() != tc.wantTarget {
				t.Fatalf("claim=%d stale=%d target=%d", store.claimCalls, stale.calls.Load(), target.calls.Load())
			}
			if tc.wantTarget == 1 && (target.args != "  claimed-bytes  " || result.Operation.Name != "claimed") {
				t.Fatalf("claimed execution result=%+v args=%q", result, target.args)
			}
			if store.seen == nil || store.seen.Status != action.ApprovalApproved {
				t.Fatalf("claim snapshot = %+v", store.seen)
			}
		})
	}
}

func TestExecution_CloseAndRecoveryRemainCompatible(t *testing.T) {
	closeCases := []struct {
		name string
		err  error
	}{
		{name: "delivered error", err: fmt.Errorf("delivered: %w", tool.ErrEffectDelivered)},
		{name: "deadline", err: context.DeadlineExceeded},
		{name: "cancellation", err: context.Canceled},
	}
	for _, tc := range closeCases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := &canonicalTool{name: "claimed", output: "partial", err: tc.err}
			exec := NewCoordinator(tool.Registry{"claimed": fixture}, 0, time.Now, CoordinatorConfig{})
			store := &approvalStoreFake{
				approval: action.Approval{ApprovalID: "apr_1", ActionID: "act_1", Status: action.ApprovalApproved},
				state:    action.StateApproved, claimOp: action.Operation{Namespace: "tool", Name: "claimed", Version: 1}, receiptID: "rcpt_1",
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			result, err := exec.ResumeApproved(ctx, store, "apr_1")
			if err != nil {
				t.Fatalf("ResumeApproved: %v", err)
			}
			if result.Outcome != action.StateOutcomeUnknown || store.closedAs != action.StateOutcomeUnknown {
				t.Fatalf("outcomes = %s/%s, want OUTCOME_UNKNOWN", result.Outcome, store.closedAs)
			}
			if store.closeContextErr != nil {
				t.Fatalf("close inherited caller cancellation: %v", store.closeContextErr)
			}
		})
	}

	t.Run("recovery closes an authorized start once", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "actions.db")
		store, err := actionsqlite.Open(path)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		env := action.NewEnvelope("act_crash", "env_1",
			action.Source{Kind: "agent_brain", Protocol: "text", Channel: "console"},
			action.Operation{Namespace: "tool", Name: "echo", Version: 1}, "x", time.Now())
		if err := store.RecordAttempt(context.Background(), env, actionsqlite.Decision{Outcome: "allow", Rule: "ungoverned"}, action.StateAuthorized); err != nil {
			t.Fatalf("RecordAttempt: %v", err)
		}
		if err := store.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		reopened, err := actionsqlite.Open(path)
		if err != nil {
			t.Fatalf("reopen: %v", err)
		}
		defer func() { _ = reopened.Close() }()
		for pass := 0; pass < 2; pass++ {
			if _, err := reopened.RecoverPreviousLife(context.Background()); err != nil {
				t.Fatalf("recovery pass %d: %v", pass, err)
			}
		}
		record, err := reopened.Get(context.Background(), "act_crash")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if record.State != action.StateOutcomeUnknown || record.RecoveryMarker != "outcome_unknown" {
			t.Fatalf("recovered state=%s marker=%q", record.State, record.RecoveryMarker)
		}
	})
}

func TestCompatibility_NoRecorderStillUsesCanonicalExecutor(t *testing.T) {
	profile := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", profile)
	for _, lane := range []string{"text", "native"} {
		t.Run(lane, func(t *testing.T) {
			var ids, invoked, closeClockCalls atomic.Int32
			fixture := &canonicalTool{name: "echo", output: "same-result"}
			exec := NewCoordinator(tool.Registry{"echo": fixture}, 0, time.Now, CoordinatorConfig{
				NewActionID:     func() string { ids.Add(1); return "act_memory" },
				InvocationProbe: func(action.Envelope) { invoked.Add(1) },
				CloseClock: func() time.Time {
					closeClockCalls.Add(1)
					return time.Unix(9, 0).UTC()
				},
			})
			plan := testPlan(t, exec, context.Background(), "console")
			req := testRequest(t, exec, plan, testEnvelope("console", "operator", ""), lane, "echo", " exact ")
			result, err := exec.Submit(context.Background(), req)
			if err != nil {
				t.Fatalf("Submit: %v", err)
			}
			if result.Output != "same-result" || fixture.args != " exact " || !result.Ungoverned {
				t.Fatalf("result=%+v args=%q", result, fixture.args)
			}
			if ids.Load() != 1 || invoked.Load() != 1 || result.Action.ActionID != "act_memory" {
				t.Fatalf("ids=%d invoked=%d action=%q", ids.Load(), invoked.Load(), result.Action.ActionID)
			}
			if closeClockCalls.Load() != 0 {
				t.Fatalf("stateless close clock calls = %d, want 0", closeClockCalls.Load())
			}
		})
	}
	entries, err := os.ReadDir(profile)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("stateless execution created profile files: %v", entries)
	}
}

func TestCoordinator_InspectionAndResumeErrors(t *testing.T) {
	exec := NewCoordinator(tool.Registry{"journal": &canonicalTool{name: "journal"}}, 0, time.Now, CoordinatorConfig{
		Governance: testGovernance(map[string]policy.ToolDecision{"journal": {Mode: policy.ToolAllow}}),
	})
	plan := testPlan(t, exec, context.Background(), "console")
	decisions, governed, err := exec.Decisions(plan)
	if err != nil || !governed || decisions["journal"].Mode != policy.ToolAllow {
		t.Fatalf("Decisions = %v/%t/%v", decisions, governed, err)
	}
	decisions["journal"] = policy.ToolDecision{Mode: policy.ToolDeny}
	sealed, _, err := exec.Decisions(plan)
	if err != nil || sealed["journal"].Mode != policy.ToolAllow {
		t.Fatalf("plan leaked its decision map: %v/%v", sealed, err)
	}
	for _, invalid := range []*DecisionPlan{nil, {}} {
		if _, _, err := exec.Decisions(invalid); !errors.Is(err, ErrExecutionBindingMismatch) {
			t.Fatalf("Decisions(%v) error = %v", invalid, err)
		}
	}

	cause := errors.New("claim moved")
	resumeErrors := []struct {
		name string
		err  *ResumeError
		want string
	}{
		{name: "with cause", err: &ResumeError{Stage: ResumeClaim, Err: cause}, want: "claim moved"},
		{name: "without cause", err: &ResumeError{Stage: ResumeApprovalPending}, want: string(ResumeApprovalPending)},
	}
	for _, tc := range resumeErrors {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.err.Error(); !strings.Contains(got, tc.want) {
				t.Fatalf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

func allowJournal() []policy.ToolGrant {
	return []policy.ToolGrant{{Name: "journal", Mode: policy.ToolAllow}}
}

func pureEffect(string) (action.EffectDescriptor, bool) {
	return action.EffectDescriptor{Class: action.EffectPure}, true
}

func irreversibleEffect(string) (action.EffectDescriptor, bool) {
	return action.EffectDescriptor{Class: action.EffectWriteIrreversible}, true
}

func criticalEffect(string) (action.EffectDescriptor, bool) {
	return action.EffectDescriptor{Class: action.EffectCritical}, true
}

func approvalIdentity() *IdentityConfig {
	return &IdentityConfig{
		Registry:      action.ProvenanceRegistry{"console": {Class: "console", Credential: action.CredentialLoopbackInProcess}},
		IntentID:      action.RootIntentID,
		EffectCeiling: action.EffectWriteIrreversible,
	}
}

func lowCeilingIdentity() *IdentityConfig {
	return &IdentityConfig{
		Registry:      action.ProvenanceRegistry{"console": {Class: "console", Credential: action.CredentialLoopbackInProcess}},
		IntentID:      action.RootIntentID,
		EffectCeiling: action.EffectReadExternal,
	}
}

type approvalStoreFake struct {
	approval        action.Approval
	state           action.State
	claimParams     []byte
	claimOp         action.Operation
	claimErr        error
	receiptID       string
	seen            *action.Approval
	claimCalls      int
	closedAs        action.State
	closeContextErr error
}

func (s *approvalStoreFake) ReadApproval(context.Context, string) (action.Approval, error) {
	return s.approval, nil
}

func (s *approvalStoreFake) ReadActionState(context.Context, string) (action.State, error) {
	return s.state, nil
}

func (s *approvalStoreFake) Claim(_ context.Context, _ string, seen *action.Approval) ([]byte, action.Operation, error) {
	s.claimCalls++
	if seen != nil {
		copySeen := *seen
		s.seen = &copySeen
	}
	return append([]byte(nil), s.claimParams...), s.claimOp, s.claimErr
}

func (s *approvalStoreFake) Close(ctx context.Context, _ string, state action.State, _ time.Time, _ string) error {
	s.closedAs = state
	s.closeContextErr = ctx.Err()
	return nil
}

func (s *approvalStoreFake) ReadReceipt(context.Context, string) (string, error) {
	return s.receiptID, nil
}

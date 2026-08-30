// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The kernel adapter contract (lote 3, spec FR-ADAPT): every attempt lands
// with its decision BEFORE any effect; shadow records SHADOWED and never
// executes; unknown fails closed with its DENIED record; success/failure
// close through Finish; a failing record refuses to execute (fail-closed);
// and both lanes produce equivalent envelopes (same digest for the same
// logical action). Approved-red contract: not edited to fit an
// implementation.

package brain

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/policy"
	"github.com/Sebastian197/korvun/internal/tool"
)

// journalTool appends to the shared journal when executed — the order
// witness for record-before-effect and the never-executed assertions.
type journalTool struct {
	journal *[]string
	err     error
}

func (j *journalTool) Name() string        { return "journal" }
func (j *journalTool) Description() string { return "order witness" }
func (j *journalTool) Execute(ctx context.Context, args string) (string, error) {
	*j.journal = append(*j.journal, "execute")
	return "done", j.err
}

// fakeRecorder captures the kernel calls in order, optionally failing.
type fakeRecorder struct {
	journal   *[]string
	envs      []action.Envelope
	outcomes  []string
	rules     []string
	states    []action.State
	finishes  []action.State
	recordErr error
}

func (f *fakeRecorder) RecordAttempt(ctx context.Context, env action.Envelope, outcome, rule string, state action.State) error {
	if f.recordErr != nil {
		return f.recordErr
	}
	*f.journal = append(*f.journal, "record:"+string(state))
	f.envs = append(f.envs, env)
	f.outcomes = append(f.outcomes, outcome)
	f.rules = append(f.rules, rule)
	f.states = append(f.states, state)
	return nil
}

func (f *fakeRecorder) Finish(ctx context.Context, actionID string, to action.State, at time.Time) error {
	*f.journal = append(*f.journal, "finish:"+string(to))
	f.finishes = append(f.finishes, to)
	return nil
}

func kernelHarness(t *testing.T, recordErr error, toolErr error) (*AgentBrain, *fakeRecorder, *[]string) {
	t.Helper()
	journal := &[]string{}
	rec := &fakeRecorder{journal: journal, recordErr: recordErr}
	a := NewAgentBrain(
		&scriptedModel{},
		tool.Registry{"journal": &journalTool{journal: journal, err: toolErr}},
		WithActionRecorder(rec),
	)
	return a, rec, journal
}

func kernelEnv() *envelope.Envelope {
	return &envelope.Envelope{ID: "env-k", Channel: "console"}
}

func TestKernel_recordsAuthorizedBeforeTheEffectAndFinishesSucceeded(t *testing.T) {
	t.Parallel()
	a, rec, journal := kernelHarness(t, nil, nil)
	out := a.runTool(context.Background(), kernelEnv(), nil, laneText, "journal", `{"a":1}`)
	if out != "done" {
		t.Fatalf("observation = %q, want the tool result", out)
	}
	want := []string{"record:AUTHORIZED", "execute", "finish:SUCCEEDED"}
	if strings.Join(*journal, ",") != strings.Join(want, ",") {
		t.Fatalf("order must be record BEFORE effect, then finish: %v", *journal)
	}
	env := rec.envs[0]
	if env.Operation != (action.Operation{Namespace: "tool", Name: "journal", Version: 1}) {
		t.Fatalf("operation = %+v", env.Operation)
	}
	if env.Source != (action.Source{Kind: "agent_brain", Protocol: "text", Channel: "console"}) {
		t.Fatalf("source = %+v", env.Source)
	}
	if env.CorrelationID != "env-k" || !strings.HasPrefix(env.ActionID, "act_") {
		t.Fatalf("ids = %q %q", env.ActionID, env.CorrelationID)
	}
	if rec.outcomes[0] != "allow" || rec.rules[0] != "ungoverned" {
		t.Fatalf("ungoverned allow must record allow/ungoverned, got %s/%s", rec.outcomes[0], rec.rules[0])
	}
}

func TestKernel_governedAllowRecordsGrantedRule(t *testing.T) {
	t.Parallel()
	a, rec, _ := kernelHarness(t, nil, nil)
	decisions := map[string]policy.ToolDecision{"journal": {Mode: policy.ToolAllow}}
	_ = a.runTool(context.Background(), kernelEnv(), decisions, laneText, "journal", `{}`)
	if rec.outcomes[0] != "allow" || rec.rules[0] != "granted" {
		t.Fatalf("governed allow records allow/granted, got %s/%s", rec.outcomes[0], rec.rules[0])
	}
}

func TestKernel_toolErrorFinishesFailed(t *testing.T) {
	t.Parallel()
	a, rec, _ := kernelHarness(t, nil, errors.New("boom"))
	out := a.runTool(context.Background(), kernelEnv(), nil, laneText, "journal", `{}`)
	if !strings.Contains(out, "failed") {
		t.Fatalf("error observation must survive, got %q", out)
	}
	if len(rec.finishes) != 1 || rec.finishes[0] != action.StateFailed {
		t.Fatalf("a tool error closes FAILED, got %v", rec.finishes)
	}
}

func TestKernel_shadowRecordsShadowedAndNeverExecutes(t *testing.T) {
	t.Parallel()
	a, rec, journal := kernelHarness(t, nil, nil)
	decisions := map[string]policy.ToolDecision{"journal": {Mode: policy.ToolShadow}}
	out := a.runTool(context.Background(), kernelEnv(), decisions, laneText, "journal", `{}`)
	if out != shadowObservation("journal") {
		t.Fatalf("the shadow observation must stay byte-identical, got %q", out)
	}
	for _, entry := range *journal {
		if entry == "execute" {
			t.Fatal("shadow must NEVER execute — with or without a receipt")
		}
	}
	if rec.states[0] != action.StateShadowed || rec.outcomes[0] != "shadow" {
		t.Fatalf("shadow records SHADOWED/shadow, got %s/%s", rec.states[0], rec.outcomes[0])
	}
	if len(rec.finishes) != 0 {
		t.Fatalf("SHADOWED is terminal at record time; no Finish, got %v", rec.finishes)
	}
}

func TestKernel_deniedRecordsRuleAndNeverExecutes(t *testing.T) {
	t.Parallel()
	a, rec, journal := kernelHarness(t, nil, nil)
	decisions := map[string]policy.ToolDecision{"journal": {Mode: policy.ToolDeny, Rule: policy.ToolRuleNotGranted}}
	out := a.runTool(context.Background(), kernelEnv(), decisions, laneText, "journal", `{}`)
	if out != deniedObservation("journal") {
		t.Fatalf("the denial observation must stay byte-identical, got %q", out)
	}
	if len(*journal) != 1 || rec.states[0] != action.StateDenied {
		t.Fatalf("a denial records DENIED and nothing executes: %v", *journal)
	}
	if rec.outcomes[0] != "deny" || rec.rules[0] != string(policy.ToolRuleNotGranted) {
		t.Fatalf("denial carries its rule, got %s/%s", rec.outcomes[0], rec.rules[0])
	}
}

func TestKernel_unknownToolFailsClosedWithItsRecord(t *testing.T) {
	t.Parallel()
	a, rec, journal := kernelHarness(t, nil, nil)
	out := a.runTool(context.Background(), kernelEnv(), nil, laneText, "ghost", `{}`)
	if !strings.Contains(out, `tool "ghost" not found`) {
		t.Fatalf("the unknown observation must stay byte-identical, got %q", out)
	}
	if len(rec.states) != 1 || rec.states[0] != action.StateDenied || rec.rules[0] != "unknown_tool" {
		t.Fatalf("unknown records DENIED/unknown_tool, got %v %v", rec.states, rec.rules)
	}
	for _, entry := range *journal {
		if entry == "execute" {
			t.Fatal("unknown must never execute")
		}
	}
}

func TestKernel_recordFailureRefusesToExecute(t *testing.T) {
	t.Parallel()
	a, _, journal := kernelHarness(t, errors.New("disk full"), nil)
	out := a.runTool(context.Background(), kernelEnv(), nil, laneText, "journal", `{}`)
	if out != deniedObservation("journal") {
		t.Fatalf("an unrecordable attempt fails closed with the denial observation, got %q", out)
	}
	for _, entry := range *journal {
		if entry == "execute" {
			t.Fatal("no record, no effect — the blueprint's promise")
		}
	}
}

func TestKernel_lanesProduceEquivalentEnvelopes(t *testing.T) {
	t.Parallel()
	a, rec, _ := kernelHarness(t, nil, nil)
	_ = a.runTool(context.Background(), kernelEnv(), nil, laneText, "journal", `{"b":2, "a":1}`)
	_ = a.runTool(context.Background(), kernelEnv(), nil, laneNative, "journal", `{"a":1,"b":2}`)
	if len(rec.envs) != 2 {
		t.Fatalf("two envelopes expected, got %d", len(rec.envs))
	}
	if rec.envs[0].ParametersDigest != rec.envs[1].ParametersDigest {
		t.Fatalf("the same logical action must digest identically across lanes:\n %q\n %q",
			rec.envs[0].ParametersDigest, rec.envs[1].ParametersDigest)
	}
	if rec.envs[0].Source.Protocol != "text" || rec.envs[1].Source.Protocol != "native" {
		t.Fatalf("lanes must be labeled, got %q %q", rec.envs[0].Source.Protocol, rec.envs[1].Source.Protocol)
	}
}

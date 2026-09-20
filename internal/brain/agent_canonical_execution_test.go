// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/bus"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/policy"
	"github.com/Sebastian197/korvun/internal/tool"
)

type canonicalOrderPublisher struct {
	journal *[]string
}

func (p canonicalOrderPublisher) Publish(context.Context, bus.Event) {
	*p.journal = append(*p.journal, "audit")
}

type panickingFallbackRecorder struct{}

func (panickingFallbackRecorder) RecordAttempt(context.Context, action.Envelope, string, string, action.State) error {
	panic("fallback recorder aborted")
}

func (panickingFallbackRecorder) RecordAttemptIdentified(context.Context, action.Envelope, string, string, action.State, action.IdentityEvidence) error {
	panic("unresolved provenance reached the identified recorder")
}

func (panickingFallbackRecorder) Finish(context.Context, string, action.State, time.Time) error {
	return nil
}

func TestCompatibility_DecisionPrecedenceAndObservations(t *testing.T) {
	cases := []struct {
		name string
		gov  AgentGovernance
		want error
	}{
		{name: "unknown sensitivity", gov: AgentGovernance{Locality: policy.Local, Grants: canonicalAllowJournal()}, want: policy.ErrUnknownSensitivity},
		{name: "unknown locality", gov: AgentGovernance{Sensitivity: policy.Public, Grants: canonicalAllowJournal()}, want: policy.ErrUnknownLocality},
		{name: "unknown mode", gov: AgentGovernance{Sensitivity: policy.Public, Locality: policy.Local, Grants: []policy.ToolGrant{{Name: "journal"}}}, want: policy.ErrUnknownToolMode},
		{name: "duplicate grant", gov: AgentGovernance{Sensitivity: policy.Public, Locality: policy.Local, Grants: []policy.ToolGrant{{Name: "journal", Mode: policy.ToolAllow}, {Name: "journal", Mode: policy.ToolDeny}}}, want: policy.ErrDuplicateToolGrant},
		{name: "empty grant", gov: AgentGovernance{Sensitivity: policy.Public, Locality: policy.Local, Grants: []policy.ToolGrant{{Name: "", Mode: policy.ToolAllow}}}, want: policy.ErrInvalidToolGrant},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			journal := &[]string{}
			recorder := &fakeRecorder{journal: journal}
			a := NewAgentBrain(
				&scriptedModel{},
				tool.Registry{"journal": &journalTool{journal: journal}},
				WithAgentLogger(slog.New(slog.NewTextHandler(&logs, nil))),
				WithActionRecorder(recorder),
				WithAgentGovernance(&tc.gov),
			)
			env := kernelEnv()
			ctx := context.Background()
			advertised, decisions, plan := a.effectiveTools(ctx, env)
			if len(advertised) != 0 || decisions == nil || len(decisions) != 0 || plan == nil {
				t.Fatalf("fail-closed selection advertised=%v decisions=%v plan=%v", advertised, decisions, plan)
			}
			logText := logs.String()
			for _, want := range []string{
				`level=ERROR`,
				`msg="agent: governance misconfigured, failing closed (deny-all)"`,
				`envelope_id=env-k`,
				`channel=console`,
				`cause=`,
				tc.want.Error(),
			} {
				if !strings.Contains(logText, want) {
					t.Fatalf("selector log lacks %q: %s", want, logText)
				}
			}
			ctx = context.WithValue(ctx, executionPlanContextKey{}, plan)
			observation := a.runTool(ctx, env, decisions, laneText, "journal", "x")
			if observation != deniedObservation("journal") {
				t.Fatalf("observation = %q", observation)
			}
			if len(recorder.rules) != 1 || recorder.rules[0] != string(policy.ToolRuleNotGranted) || len(recorder.states) != 1 || recorder.states[0] != action.StateDenied {
				t.Fatalf("recorded rules=%v states=%v", recorder.rules, recorder.states)
			}
			for _, step := range *journal {
				if step == "execute" {
					t.Fatal("selector failure dispatched")
				}
			}
		})
	}
}

func canonicalAllowJournal() []policy.ToolGrant {
	return []policy.ToolGrant{{Name: "journal", Mode: policy.ToolAllow}}
}

func TestRunTool_AuditsTheInvocationBeforeTerminalClose(t *testing.T) {
	journal := &[]string{}
	recorder := &fakeRecorder{journal: journal}
	a := NewAgentBrain(
		&scriptedModel{},
		tool.Registry{"journal": &journalTool{journal: journal}},
		WithActionRecorder(recorder),
		WithAgentToolAudit(canonicalOrderPublisher{journal: journal}, "alpha"),
	)
	if got := a.runTool(context.Background(), kernelEnv(), nil, laneText, "journal", `{}`); got != "done" {
		t.Fatalf("observation = %q, want done", got)
	}
	want := "record:AUTHORIZED,execute,audit,finish:SUCCEEDED"
	if got := strings.Join(*journal, ","); got != want {
		t.Fatalf("execution order = %q, want %q", got, want)
	}
}

func TestRunTool_AuditsDenialBeforeItsBestEffortRecord(t *testing.T) {
	journal := &[]string{}
	recorder := &fakeRecorder{journal: journal}
	a := NewAgentBrain(
		&scriptedModel{},
		tool.Registry{"journal": &journalTool{journal: journal}},
		WithActionRecorder(recorder),
		WithAgentToolAudit(canonicalOrderPublisher{journal: journal}, "alpha"),
	)
	decisions := map[string]policy.ToolDecision{
		"journal": {Mode: policy.ToolDeny, Rule: policy.ToolRuleNotGranted},
	}
	if got := a.runTool(context.Background(), kernelEnv(), decisions, laneText, "journal", `{}`); got != deniedObservation("journal") {
		t.Fatalf("observation = %q", got)
	}
	want := "audit,record:DENIED"
	if got := strings.Join(*journal, ","); got != want {
		t.Fatalf("denial order = %q, want %q", got, want)
	}
}

func TestRunTool_FixedGovernanceCannotBeReplacedWithoutItsPlan(t *testing.T) {
	journal := &[]string{}
	recorder := &fakeRecorder{journal: journal}
	a := NewAgentBrain(
		&scriptedModel{},
		tool.Registry{"journal": &journalTool{journal: journal}},
		WithActionRecorder(recorder),
		WithAgentGovernance(&AgentGovernance{
			Grants:      []policy.ToolGrant{{Name: "journal", Mode: policy.ToolDeny}},
			Sensitivity: policy.Public,
			Locality:    policy.Local,
		}),
	)
	forged := map[string]policy.ToolDecision{"journal": {Mode: policy.ToolAllow}}
	if got := a.runTool(context.Background(), kernelEnv(), forged, laneText, "journal", `{}`); got != deniedObservation("journal") {
		t.Fatalf("observation = %q", got)
	}
	for _, step := range *journal {
		if step == "execute" {
			t.Fatal("caller decisions replaced fixed governance")
		}
	}
	if len(recorder.rules) != 1 || recorder.rules[0] != string(policy.ToolRuleDenyGrant) {
		t.Fatalf("recorded rules = %v, want fixed deny", recorder.rules)
	}
}

func TestRunTool_DenialRecordFailureLogsOnce(t *testing.T) {
	var logs bytes.Buffer
	journal := &[]string{}
	a := NewAgentBrain(
		&scriptedModel{},
		tool.Registry{"journal": &journalTool{journal: journal}},
		WithActionRecorder(&fakeRecorder{journal: journal, recordErr: errors.New("disk full")}),
		WithAgentLogger(slog.New(slog.NewTextHandler(&logs, nil))),
	)
	decisions := map[string]policy.ToolDecision{
		"journal": {Mode: policy.ToolDeny, Rule: policy.ToolRuleNotGranted},
	}
	if got := a.runTool(context.Background(), kernelEnv(), decisions, laneText, "journal", `{}`); got != deniedObservation("journal") {
		t.Fatalf("observation = %q", got)
	}
	if got := strings.Count(logs.String(), "agent: action record failed"); got != 1 {
		t.Fatalf("record failure log count = %d, want 1: %s", got, logs.String())
	}
}

func TestRunTool_LogsIdentityFallbackBeforeCallingTheLegacyRecorder(t *testing.T) {
	cases := []struct {
		name      string
		decisions map[string]policy.ToolDecision
	}{
		{name: "deny", decisions: map[string]policy.ToolDecision{"journal": {Mode: policy.ToolDeny, Rule: policy.ToolRuleNotGranted}}},
		{name: "shadow", decisions: map[string]policy.ToolDecision{"journal": {Mode: policy.ToolShadow}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			a := NewAgentBrain(
				&scriptedModel{},
				tool.Registry{"journal": &journalTool{journal: &[]string{}}},
				WithActionRecorder(panickingFallbackRecorder{}),
				WithActionIdentity(ActionIdentity{
					Registry: action.ProvenanceRegistry{
						"console": {Class: "console", Credential: action.CredentialLoopbackInProcess},
					},
					IntentID: action.RootIntentID,
				}),
				WithAgentLogger(slog.New(slog.NewTextHandler(&logs, nil))),
			)
			env := kernelEnv()
			env.Channel = "unregistered"
			deferred := false
			defer func() {
				if !deferred {
					return
				}
				if recovered := recover(); recovered == nil {
					t.Fatal("legacy recorder did not abort")
				}
				if !strings.Contains(logs.String(), "agent: unknown provenance, recording without identity") {
					t.Fatalf("identity fallback warning was not published before recorder abort: %s", logs.String())
				}
			}()
			deferred = true
			a.runTool(context.Background(), env, tc.decisions, laneText, "journal", `{}`)
		})
	}
}

// envelopeIdentityPublisher keeps the LAST audited envelope pointer, which is
// the only fact the mould below judges.
type envelopeIdentityPublisher struct {
	seen     int
	envelope *envelope.Envelope
}

func (p *envelopeIdentityPublisher) Publish(_ context.Context, ev bus.Event) {
	p.seen++
	p.envelope = ev.Envelope
}

// The twenty-first pass, P2-2: the audit event must carry the LIVE inbound
// envelope — the very pointer the brain was handed — on EVERY branch, because
// that is the object subscribers received before this phase and Phase 0 changes
// nothing observable. A snapshot travelled here once, on every branch but the
// parked one, and nothing watched it.
func TestRunTool_AuditCarriesTheLiveInboundEnvelope(t *testing.T) {
	for _, row := range []struct {
		name      string
		decisions map[string]policy.ToolDecision
	}{
		{name: "executed", decisions: nil},
		{
			name:      "denied",
			decisions: map[string]policy.ToolDecision{"journal": {Mode: policy.ToolDeny, Rule: policy.ToolRuleNotGranted}},
		},
	} {
		t.Run(row.name, func(t *testing.T) {
			journal := &[]string{}
			spy := &envelopeIdentityPublisher{}
			a := NewAgentBrain(
				&scriptedModel{},
				tool.Registry{"journal": &journalTool{journal: journal}},
				WithActionRecorder(&fakeRecorder{journal: journal}),
				WithAgentToolAudit(spy, "alpha"),
			)
			env := kernelEnv()
			a.runTool(context.Background(), env, row.decisions, laneText, "journal", `{}`)
			if spy.seen != 1 {
				t.Fatalf("audit events = %d, want 1", spy.seen)
			}
			if spy.envelope != env {
				t.Fatalf("audit envelope = %p, want the live inbound %p", spy.envelope, env)
			}
		})
	}
}

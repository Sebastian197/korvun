// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package executor_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/action/executor"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/identity"
	"github.com/Sebastian197/korvun/internal/policy"
	"github.com/Sebastian197/korvun/internal/tool"
)

type identityCountingTool struct{ calls atomic.Int64 }

type identityCountingRecorder struct {
	attempts     atomic.Int64
	approvals    atomic.Int64
	finishes     atomic.Int64
	recordErr    error
	requestErr   error
	lastAction   action.Envelope
	lastEvidence identity.Evidence
}

func (r *identityCountingRecorder) RecordAttempt(context.Context, action.Envelope,
	string, string, action.State,
) error {
	r.attempts.Add(1)
	return nil
}

func (r *identityCountingRecorder) RecordAttemptAuthenticated(_ context.Context,
	env action.Envelope, _ string, _ string, _ action.State, evidence identity.Evidence,
) error {
	r.attempts.Add(1)
	r.lastAction = env
	r.lastEvidence = evidence
	return r.recordErr
}

func (r *identityCountingRecorder) RequestApprovalAuthenticated(_ context.Context,
	env action.Envelope, _ string, _ string, evidence identity.Evidence,
) (string, error) {
	r.approvals.Add(1)
	r.lastAction = env
	r.lastEvidence = evidence
	if r.requestErr != nil {
		return "", r.requestErr
	}
	return "apr_identity", nil
}

func TestIdentity_AuthenticatedCoordinatorOwnsEveryTerminalBranch(t *testing.T) {
	now := time.Date(2026, 9, 21, 13, 30, 0, 0, time.UTC)
	registry := identity.Registry{
		Principals: []identity.Principal{
			{ID: "principal_webhook", Kind: identity.PrincipalExternalSystem},
			{ID: "principal_brain_alpha", Kind: identity.PrincipalWorkload},
			{ID: "principal_responsible", Kind: identity.PrincipalHuman},
		},
		Bindings: []identity.Binding{{
			ID: "binding_webhook", Provider: "webhook", Channel: "webhook",
			CredentialRef: "WEBHOOK_SECRET", SubjectNamespace: "payload.sender_id",
			VerifiedSubject: "shared_webhook_credential",
			PrincipalID:     "principal_webhook", Generation: 1, Status: identity.BindingActive,
		}},
		Workloads: []identity.Workload{{
			Brain: "alpha", PrincipalID: "principal_brain_alpha",
			ResponsiblePrincipalID: "principal_responsible",
		}},
	}
	resolver, err := identity.NewResolver(registry, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := resolver.NewIssuer(identity.IssuerConfig{
		BindingID: "binding_webhook", Method: "bearer",
		CredentialClass: "shared_secret", TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	classify := func(class action.EffectClass) executor.EffectClassifier {
		return func(string) (action.EffectDescriptor, bool) {
			return action.EffectDescriptor{Class: class}, true
		}
	}
	tests := []struct {
		name       string
		toolName   string
		classifier executor.EffectClassifier
		recordErr  error
		wantBranch executor.Branch
		wantCalls  int64
		wantRecord int64
		wantPark   int64
		wantFinish int64
	}{
		{"authorized execution", "probe", classify(action.EffectPure), nil, executor.BranchExecuted, 1, 1, 0, 1},
		{"unknown tool denial", "missing", classify(action.EffectPure), nil, executor.BranchUnknown, 0, 1, 0, 0},
		{"approval birth", "probe", classify(action.EffectWriteIrreversible), nil, executor.BranchPending, 0, 0, 1, 0},
		{"authenticated record refusal", "probe", classify(action.EffectPure), errors.New("identity store unavailable"), executor.BranchDenied, 0, 1, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probe := &identityCountingTool{}
			recorder := &identityCountingRecorder{recordErr: tt.recordErr}
			exec := executor.NewCoordinator(tool.Registry{"probe": probe}, time.Second,
				func() time.Time { return now }, executor.CoordinatorConfig{
					BrainName: "alpha", Recorder: recorder, PrincipalResolver: resolver,
					Governance: &executor.Governance{
						Grants:      []policy.ToolGrant{{Name: "probe", Mode: policy.ToolAllow}},
						Attrs:       map[string]policy.ToolAttrs{"probe": {}},
						Sensitivity: policy.Public, Locality: policy.Local,
					},
					Identity: &executor.IdentityConfig{
						IntentID: "int_identity", GrantID: "grant_identity",
						EffectCeiling: action.EffectCritical,
					},
					EffectClassifier: tt.classifier,
				})
			in := envelope.New("webhook", envelope.Inbound,
				envelope.Participant{ID: "forged-operator"}).AddText("run")
			in.ID = "request-" + tt.name
			in.Meta["conversation_id"] = "conv"
			ingress, err := issuer.Issue(in.ID, in.Sender.ID)
			if err != nil {
				t.Fatal(err)
			}
			_, plan, err := exec.SelectTools(context.Background(), "webhook")
			if err != nil {
				t.Fatal(err)
			}
			request, err := exec.Prepare(executor.Submission{
				Inbound: in, Ingress: ingress, Lane: "text", Name: tt.toolName,
				Arguments: "{}", Plan: plan,
			})
			if err != nil {
				t.Fatal(err)
			}
			result, submitErr := exec.Submit(context.Background(), request)
			if tt.wantBranch == executor.BranchUnknown {
				if !errors.Is(submitErr, executor.ErrUnknownTool) {
					t.Fatalf("Submit error = %v, want unknown tool", submitErr)
				}
			} else if submitErr != nil {
				t.Fatalf("Submit error = %v", submitErr)
			}
			if result.Branch != tt.wantBranch || probe.calls.Load() != tt.wantCalls ||
				recorder.attempts.Load() != tt.wantRecord || recorder.approvals.Load() != tt.wantPark ||
				recorder.finishes.Load() != tt.wantFinish {
				t.Fatalf("result=%+v calls=%d records=%d approvals=%d finishes=%d",
					result, probe.calls.Load(), recorder.attempts.Load(),
					recorder.approvals.Load(), recorder.finishes.Load())
			}
			if recorder.lastEvidence.RequesterPrincipalID != "principal_webhook" ||
				recorder.lastAction.Principal.PrincipalID != "principal_brain_alpha" ||
				recorder.lastAction.IntentID != "int_identity" {
				t.Fatalf("authenticated authority = action:%+v evidence:%+v",
					recorder.lastAction, recorder.lastEvidence)
			}
			if tt.name == "authorized execution" &&
				(len(recorder.lastAction.AuthorityRefs) != 1 || recorder.lastAction.AuthorityRefs[0] != "grant_identity") {
				t.Fatalf("authority refs = %v", recorder.lastAction.AuthorityRefs)
			}
		})
	}
}

func (r *identityCountingRecorder) Finish(context.Context, string, action.State, time.Time) error {
	r.finishes.Add(1)
	return nil
}

func (*identityCountingTool) Name() string        { return "probe" }
func (*identityCountingTool) Description() string { return "identity probe" }
func (t *identityCountingTool) Execute(context.Context, string) (string, error) {
	t.calls.Add(1)
	return "ok", nil
}

func TestIngress_ChannelLabelIsNotAuthentication(t *testing.T) {
	now := time.Date(2026, 9, 21, 13, 0, 0, 0, time.UTC)
	resolver, err := identity.NewResolver(identity.Registry{
		Principals: []identity.Principal{
			{ID: "principal_console", Kind: identity.PrincipalExternalSystem},
			{ID: "principal_brain_alpha", Kind: identity.PrincipalWorkload},
		},
		Bindings: []identity.Binding{{
			ID: "binding_console", Provider: "console", Channel: "console",
			CredentialRef: "console_bearer", SubjectNamespace: "local_profile",
			VerifiedSubject: "shared_console_credential",
			PrincipalID:     "principal_console", Generation: 1, Status: identity.BindingActive,
		}},
		Workloads: []identity.Workload{{Brain: "alpha", PrincipalID: "principal_brain_alpha"}},
	}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	probe := &identityCountingTool{}
	recorder := &identityCountingRecorder{}
	exec := executor.NewCoordinator(tool.Registry{"probe": probe}, time.Second,
		func() time.Time { return now }, executor.CoordinatorConfig{
			BrainName: "alpha", PrincipalResolver: resolver, Recorder: recorder,
		})
	in := envelope.New("console", envelope.Inbound, envelope.Participant{ID: "operator"}).AddText("run")
	in.ID = "request-console"
	in.Meta["conversation_id"] = "conv"
	_, plan, err := exec.SelectTools(context.Background(), "console")
	if err != nil {
		t.Fatal(err)
	}
	req, err := exec.Prepare(executor.Submission{
		Inbound: in, Lane: "text", Name: "probe", Arguments: "{}", Plan: plan,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := exec.Submit(context.Background(), req); !errors.Is(err, identity.ErrIdentityEvidenceMissing) {
		t.Fatalf("Submit error = %v, want ErrIdentityEvidenceMissing", err)
	}
	if got := probe.calls.Load(); got != 0 {
		t.Fatalf("tool calls = %d, want 0", got)
	}
	if recorder.attempts.Load() != 0 || recorder.approvals.Load() != 0 ||
		recorder.finishes.Load() != 0 {
		t.Fatalf("downstream writes = attempts:%d approvals:%d finishes:%d, want zero",
			recorder.attempts.Load(), recorder.approvals.Load(), recorder.finishes.Load())
	}
}

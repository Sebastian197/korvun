// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/identity"
	"github.com/Sebastian197/korvun/internal/tool"
)

// strictRecorder is the strict store seam as the coordinator sees it. Every
// door counts its calls, because what these moulds demand is as much about the
// doors that must NOT be reached as about the one that must.
type strictRecorder struct {
	startResult AuthorityStartResult
	startErr    error
	parkResult  AuthorityApprovalResult
	parkErr     error

	starts, parks              int
	legacyRecords, legacyParks int
	resolvedFor                []string
	resolveErr                 error
}

func (r *strictRecorder) RecordAttempt(context.Context, action.Envelope, string, string, action.State) error {
	r.legacyRecords++
	return nil
}

func (r *strictRecorder) RecordAttemptAuthenticated(context.Context, action.Envelope, string, string, action.State, identity.Evidence) error {
	r.legacyRecords++
	return nil
}

func (r *strictRecorder) RequestApprovalAuthenticated(context.Context, action.Envelope, string, string, identity.Evidence) (string, error) {
	r.legacyParks++
	return "apr_legacy", nil
}

func (r *strictRecorder) Finish(context.Context, string, action.State, time.Time) error { return nil }

func (r *strictRecorder) StartAuthorization(_ context.Context, request AuthorityStartRequest) (AuthorityStartResult, error) {
	r.starts++
	if r.startErr != nil {
		return AuthorityStartResult{}, r.startErr
	}
	// The store mints the final id and only then asks for the evidence of THAT
	// id; the fake does the same so the callback's wiring is exercised.
	evidence, err := request.ResolveEvidence(r.startResult.ActionID)
	r.resolvedFor, r.resolveErr = append(r.resolvedFor, r.startResult.ActionID), err
	if err != nil {
		return AuthorityStartResult{}, err
	}
	result := r.startResult
	result.Evidence = &evidence
	return result, nil
}

func (r *strictRecorder) RequestAuthorizationApproval(_ context.Context, request AuthorityApprovalRequest) (AuthorityApprovalResult, error) {
	r.parks++
	if r.parkErr != nil {
		return AuthorityApprovalResult{}, r.parkErr
	}
	evidence, err := request.ResolveEvidence(r.parkResult.ActionID)
	if err != nil {
		return AuthorityApprovalResult{}, err
	}
	result := r.parkResult
	result.Evidence = evidence
	return result, nil
}

// legacyOnlyRecorder implements the legacy recorder doors and NEITHER strict
// seam: what a strict coordinator is handed when the wiring forgot the store.
type legacyOnlyRecorder struct{ records, parks int }

func (r *legacyOnlyRecorder) RecordAttempt(context.Context, action.Envelope, string, string, action.State) error {
	r.records++
	return nil
}

func (r *legacyOnlyRecorder) RecordAttemptAuthenticated(context.Context, action.Envelope, string, string, action.State, identity.Evidence) error {
	r.records++
	return nil
}

func (r *legacyOnlyRecorder) RequestApprovalAuthenticated(context.Context, action.Envelope, string, string, identity.Evidence) (string, error) {
	r.parks++
	return "apr_legacy", nil
}

func (r *legacyOnlyRecorder) Finish(context.Context, string, action.State, time.Time) error {
	return nil
}

func strictIdentityFixture(t *testing.T, now time.Time) (*identity.Resolver, *identity.Issuer) {
	t.Helper()
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
		BindingID: "binding_webhook", Method: "bearer", CredentialClass: "shared_secret", TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	return resolver, issuer
}

func strictSubmit(t *testing.T, exec *Executor, issuer *identity.Issuer) (Result, error) {
	t.Helper()
	inbound := envelope.New("webhook", envelope.Inbound, envelope.Participant{ID: "authenticated-subject"})
	inbound.ID = "request-strict"
	submission := Submission{Inbound: inbound, Lane: "text", Name: "probe", Arguments: `{}`}
	if issuer != nil {
		ingress, err := issuer.Issue(inbound.ID, inbound.Sender.ID)
		if err != nil {
			t.Fatal(err)
		}
		submission.Ingress = ingress
	}
	_, plan, err := exec.SelectTools(context.Background(), inbound.Channel)
	if err != nil {
		t.Fatal(err)
	}
	submission.Plan = plan
	request, err := exec.Prepare(submission)
	if err != nil {
		t.Fatal(err)
	}
	return exec.Submit(context.Background(), request)
}

// TestAuthority_StrictImmediateStartDoor is the coordinator's half of the strict
// boundary for an effect that needs no approval: only a COMMITTED start hands
// the dispatcher a capability, and a strict coordinator never falls back to the
// legacy recorder — not when the store refuses, not when the wiring is wrong.
//
// Evidence level: in-process; the real coordinator and the real identity
// resolver over a FAKE strict store. What the real store decides is proved in
// internal/action/sqlite; what is proved here is what the coordinator does with
// each answer.
// Probing mutation executed: report a refused strict start as admitted — red,
// with the forbidden state observed: «branch="executed"» and «dispatches=1»
// for a start the store had refused.
func TestAuthority_StrictImmediateStartDoor(t *testing.T) {
	now := time.Date(2026, 9, 21, 13, 30, 0, 0, time.UTC)
	errRefused := errors.New("strict store: start refused")
	reversible := func(string) (action.EffectDescriptor, bool) {
		return action.EffectDescriptor{Class: action.EffectWriteReversible}, true
	}
	pure := func(string) (action.EffectDescriptor, bool) {
		return action.EffectDescriptor{Class: action.EffectPure}, true
	}

	t.Run("a committed start dispatches once, under the id the STORE minted", func(t *testing.T) {
		resolver, issuer := strictIdentityFixture(t, now)
		probe := &canonicalTool{name: "probe", output: "done"}
		recorder := &strictRecorder{startResult: AuthorityStartResult{
			ActionID: "act3_1_minted", IntentID: "int_authority", AuthorityRefs: []string{"grant_root"},
		}}
		exec := NewCoordinator(tool.Registry{"probe": probe}, time.Second, func() time.Time { return now },
			CoordinatorConfig{BrainName: "alpha", Recorder: recorder, PrincipalResolver: resolver,
				StrictAuthority: true, EffectClassifier: reversible})
		result, err := strictSubmit(t, exec, issuer)
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if result.Branch != BranchExecuted || probe.calls.Load() != 1 {
			t.Errorf("branch=%q dispatches=%d, want %q and 1", result.Branch, probe.calls.Load(), BranchExecuted)
		}
		if result.Action.ActionID != "act3_1_minted" || result.Action.IntentID != "int_authority" ||
			len(result.Action.AuthorityRefs) != 1 || result.Action.AuthorityRefs[0] != "grant_root" {
			t.Errorf("executed action = %+v, want the store's id, intent and refs", result.Action)
		}
		if len(recorder.resolvedFor) != 1 || recorder.resolvedFor[0] != "act3_1_minted" || recorder.resolveErr != nil {
			t.Errorf("evidence resolved for %v (%v), want exactly the minted id", recorder.resolvedFor, recorder.resolveErr)
		}
		if recorder.starts != 1 || recorder.legacyRecords != 0 {
			t.Errorf("strict starts=%d legacy records=%d, want 1 and 0", recorder.starts, recorder.legacyRecords)
		}
	})

	t.Run("a refused start is a denial with the store's own error and no dispatch", func(t *testing.T) {
		resolver, issuer := strictIdentityFixture(t, now)
		probe := &canonicalTool{name: "probe", output: "done"}
		recorder := &strictRecorder{startErr: errRefused}
		exec := NewCoordinator(tool.Registry{"probe": probe}, time.Second, func() time.Time { return now },
			CoordinatorConfig{BrainName: "alpha", Recorder: recorder, PrincipalResolver: resolver,
				StrictAuthority: true, EffectClassifier: reversible})
		result, err := strictSubmit(t, exec, issuer)
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if !errors.Is(result.RecordError, errRefused) || result.Branch != BranchDenied {
			t.Errorf("record error=%v branch=%q, want the store's refusal and %q", result.RecordError, result.Branch, BranchDenied)
		}
		if probe.calls.Load() != 0 || recorder.legacyRecords != 0 {
			t.Errorf("dispatches=%d legacy records=%d, want 0 and 0: a refused strict start fell back", probe.calls.Load(), recorder.legacyRecords)
		}
	})

	t.Run("a recorder with no strict seam is corrupt wiring, never a downgrade", func(t *testing.T) {
		resolver, issuer := strictIdentityFixture(t, now)
		probe := &canonicalTool{name: "probe", output: "done"}
		recorder := &legacyOnlyRecorder{}
		exec := NewCoordinator(tool.Registry{"probe": probe}, time.Second, func() time.Time { return now },
			CoordinatorConfig{BrainName: "alpha", Recorder: recorder, PrincipalResolver: resolver,
				StrictAuthority: true, EffectClassifier: reversible})
		result, err := strictSubmit(t, exec, issuer)
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if !errors.Is(result.RecordError, action.ErrAuthorityEvidenceCorrupt) || result.Branch != BranchDenied {
			t.Errorf("record error=%v branch=%q, want %v and %q", result.RecordError, result.Branch, action.ErrAuthorityEvidenceCorrupt, BranchDenied)
		}
		if probe.calls.Load() != 0 || recorder.records != 0 {
			t.Errorf("dispatches=%d legacy records=%d, want 0 and 0", probe.calls.Load(), recorder.records)
		}
	})

	t.Run("no principal resolver is missing identity, never a start", func(t *testing.T) {
		probe := &canonicalTool{name: "probe", output: "done"}
		recorder := &strictRecorder{startResult: AuthorityStartResult{ActionID: "act3_1_minted"}}
		exec := NewCoordinator(tool.Registry{"probe": probe}, time.Second, func() time.Time { return now },
			CoordinatorConfig{BrainName: "alpha", Recorder: recorder, StrictAuthority: true, EffectClassifier: reversible})
		result, err := strictSubmit(t, exec, nil)
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if !errors.Is(result.RecordError, identity.ErrIdentityEvidenceMissing) || !result.AuthorizationIdentityError {
			t.Errorf("record error=%v identity flag=%t, want %v and true", result.RecordError, result.AuthorizationIdentityError, identity.ErrIdentityEvidenceMissing)
		}
		if probe.calls.Load() != 0 || recorder.starts != 0 {
			t.Errorf("dispatches=%d strict starts=%d, want 0 and 0", probe.calls.Load(), recorder.starts)
		}
	})

	t.Run("a PURE action stays on the existing path: the strict door is not asked", func(t *testing.T) {
		resolver, issuer := strictIdentityFixture(t, now)
		probe := &canonicalTool{name: "probe", output: "done"}
		recorder := &strictRecorder{startErr: errRefused}
		exec := NewCoordinator(tool.Registry{"probe": probe}, time.Second, func() time.Time { return now },
			CoordinatorConfig{BrainName: "alpha", Recorder: recorder, PrincipalResolver: resolver,
				StrictAuthority: true, EffectClassifier: pure})
		result, err := strictSubmit(t, exec, issuer)
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if result.Branch != BranchExecuted || probe.calls.Load() != 1 {
			t.Errorf("branch=%q dispatches=%d, want %q and 1", result.Branch, probe.calls.Load(), BranchExecuted)
		}
		if recorder.starts != 0 || recorder.legacyRecords != 1 {
			t.Errorf("strict starts=%d legacy records=%d, want 0 and 1", recorder.starts, recorder.legacyRecords)
		}
	})
}

// TestAuthority_StrictPendingBirthDoor: an effect that needs a human is parked
// through the STRICT birth — which verifies authority and signs what the
// operator will read — and never through the legacy one.
//
// Evidence level: in-process; real coordinator and resolver, fake strict store.
// Probing mutation executed: disable the strict branch of the approval path, so
// a strict coordinator parks through the legacy requester — red on all three
// rows: «approval="apr_legacy"» and «legacy parks=1».
func TestAuthority_StrictPendingBirthDoor(t *testing.T) {
	now := time.Date(2026, 9, 21, 13, 30, 0, 0, time.UTC)
	errRefused := errors.New("strict store: pending birth refused")
	irreversible := func(string) (action.EffectDescriptor, bool) {
		return action.EffectDescriptor{Class: action.EffectWriteIrreversible}, true
	}
	config := func(recorder Recorder, resolver *identity.Resolver) CoordinatorConfig {
		return CoordinatorConfig{BrainName: "alpha", Recorder: recorder, PrincipalResolver: resolver,
			StrictAuthority: true, EffectClassifier: irreversible,
			Identity: &IdentityConfig{IntentID: "int_authority", GrantID: "grant_root", EffectCeiling: action.EffectCritical}}
	}

	t.Run("a strict birth parks under the store's ids and dispatches nothing", func(t *testing.T) {
		resolver, issuer := strictIdentityFixture(t, now)
		probe := &canonicalTool{name: "probe", output: "done"}
		recorder := &strictRecorder{parkResult: AuthorityApprovalResult{
			ActionID: "act3_1_parked", ApprovalID: "apr3_parked", IntentID: "int_authority", AuthorityRefs: []string{"grant_root"},
		}}
		exec := NewCoordinator(tool.Registry{"probe": probe}, time.Second, func() time.Time { return now }, config(recorder, resolver))
		result, err := strictSubmit(t, exec, issuer)
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if result.Branch != BranchPending || result.ApprovalID != "apr3_parked" || result.Action.ActionID != "act3_1_parked" {
			t.Errorf("branch=%q approval=%q action=%q, want pending under the store's ids", result.Branch, result.ApprovalID, result.Action.ActionID)
		}
		if probe.calls.Load() != 0 || recorder.parks != 1 || recorder.legacyParks != 0 || recorder.starts != 0 {
			t.Errorf("dispatches=%d strict parks=%d legacy parks=%d starts=%d, want 0, 1, 0, 0",
				probe.calls.Load(), recorder.parks, recorder.legacyParks, recorder.starts)
		}
	})

	t.Run("a refused birth is a denial carrying the store's error", func(t *testing.T) {
		resolver, issuer := strictIdentityFixture(t, now)
		probe := &canonicalTool{name: "probe", output: "done"}
		recorder := &strictRecorder{parkErr: errRefused}
		exec := NewCoordinator(tool.Registry{"probe": probe}, time.Second, func() time.Time { return now }, config(recorder, resolver))
		result, err := strictSubmit(t, exec, issuer)
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if !errors.Is(result.ApprovalError, errRefused) || result.Branch != BranchDenied {
			t.Errorf("approval error=%v branch=%q, want the store's refusal and %q", result.ApprovalError, result.Branch, BranchDenied)
		}
		if probe.calls.Load() != 0 || recorder.legacyParks != 0 {
			t.Errorf("dispatches=%d legacy parks=%d, want 0 and 0: a refused strict birth fell back", probe.calls.Load(), recorder.legacyParks)
		}
	})

	t.Run("a recorder with no strict birth seam never parks through the legacy one", func(t *testing.T) {
		resolver, issuer := strictIdentityFixture(t, now)
		probe := &canonicalTool{name: "probe", output: "done"}
		recorder := &legacyOnlyRecorder{}
		exec := NewCoordinator(tool.Registry{"probe": probe}, time.Second, func() time.Time { return now }, config(recorder, resolver))
		result, err := strictSubmit(t, exec, issuer)
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if !result.ApprovalIdentityError || result.Branch != BranchDenied {
			t.Errorf("identity flag=%t branch=%q, want true and %q", result.ApprovalIdentityError, result.Branch, BranchDenied)
		}
		if probe.calls.Load() != 0 || recorder.parks != 0 {
			t.Errorf("dispatches=%d legacy parks=%d, want 0 and 0", probe.calls.Load(), recorder.parks)
		}
	})
}

// strictApprovalStore adds the strict resume seam to the legacy approval store.
type strictApprovalStore struct {
	approvalStoreFake
	startParams []byte
	startOp     action.Operation
	startAction string
	startErr    error
	startCalls  int
}

func (s *strictApprovalStore) StartApprovedAuthorization(context.Context, string) ([]byte, action.Operation, string, error) {
	s.startCalls++
	return s.startParams, s.startOp, s.startAction, s.startErr
}

// TestAuthority_StrictApprovedResumeDoor: a strict resume goes through the
// authority-aware start and NEVER through the legacy claim — which would consume
// the parked parameters without rechecking anything.
//
// Evidence level: in-process; the real coordinator over a fake approval store.
// Probing mutation executed: disable the strict branch of the resume, so it falls
// to the legacy claim — red on all three strict rows, the worst of them with the
// withdrawn authority RESUMED: «error = <nil>», «dispatches=1 legacy claims=1
// closed="SUCCEEDED"».
func TestAuthority_StrictApprovedResumeDoor(t *testing.T) {
	errRevoked := errors.New("strict store: authority revoked since the approval")
	approved := func() approvalStoreFake {
		return approvalStoreFake{
			approval: action.Approval{ApprovalID: "apr3_1", ActionID: "act3_1_parked", Status: action.ApprovalApproved},
			state:    action.StateApproved, receiptID: "rcpt_1",
			claimParams: []byte(`{"legacy":true}`), claimOp: action.Operation{Namespace: "tool", Name: "probe", Version: 1},
		}
	}
	operation := action.Operation{Namespace: "tool", Name: "probe", Version: 1}

	t.Run("the strict start runs the tool once and the legacy claim is never called", func(t *testing.T) {
		probe := &canonicalTool{name: "probe", output: "done"}
		exec := NewCoordinator(tool.Registry{"probe": probe}, 0, time.Now, CoordinatorConfig{StrictAuthority: true})
		store := &strictApprovalStore{approvalStoreFake: approved(), startParams: []byte(`{"strict":true}`), startOp: operation, startAction: "act3_1_parked"}
		result, err := exec.ResumeApproved(context.Background(), store, "apr3_1")
		if err != nil {
			t.Fatalf("ResumeApproved: %v", err)
		}
		if probe.calls.Load() != 1 || probe.args != `{"strict":true}` {
			t.Errorf("dispatches=%d args=%q, want 1 with the strictly claimed bytes", probe.calls.Load(), probe.args)
		}
		if store.startCalls != 1 || store.claimCalls != 0 {
			t.Errorf("strict starts=%d legacy claims=%d, want 1 and 0", store.startCalls, store.claimCalls)
		}
		if result.ReceiptID != "rcpt_1" || result.Operation != operation {
			t.Errorf("result = %+v, want the store's receipt and operation", result)
		}
	})

	t.Run("authority withdrawn since the approval refuses at the claim stage and runs nothing", func(t *testing.T) {
		probe := &canonicalTool{name: "probe", output: "done"}
		exec := NewCoordinator(tool.Registry{"probe": probe}, 0, time.Now, CoordinatorConfig{StrictAuthority: true})
		store := &strictApprovalStore{approvalStoreFake: approved(), startErr: errRevoked}
		_, err := exec.ResumeApproved(context.Background(), store, "apr3_1")
		var resume *ResumeError
		if !errors.Is(err, errRevoked) || !errors.As(err, &resume) || resume.Stage != ResumeClaim {
			t.Errorf("error = %v, want the store's refusal at stage %q", err, ResumeClaim)
		}
		if probe.calls.Load() != 0 || store.claimCalls != 0 || store.closedAs != "" {
			t.Errorf("dispatches=%d legacy claims=%d closed=%q, want 0, 0 and nothing closed", probe.calls.Load(), store.claimCalls, store.closedAs)
		}
	})

	t.Run("a store with no strict seam is corrupt wiring, never the legacy claim", func(t *testing.T) {
		probe := &canonicalTool{name: "probe", output: "done"}
		exec := NewCoordinator(tool.Registry{"probe": probe}, 0, time.Now, CoordinatorConfig{StrictAuthority: true})
		legacy := approved()
		_, err := exec.ResumeApproved(context.Background(), &legacy, "apr3_1")
		var resume *ResumeError
		if !errors.Is(err, action.ErrAuthorityEvidenceCorrupt) || !errors.As(err, &resume) || resume.Stage != ResumeClaim {
			t.Errorf("error = %v, want %v at stage %q", err, action.ErrAuthorityEvidenceCorrupt, ResumeClaim)
		}
		if probe.calls.Load() != 0 || legacy.claimCalls != 0 {
			t.Errorf("dispatches=%d legacy claims=%d, want 0 and 0", probe.calls.Load(), legacy.claimCalls)
		}
	})

	t.Run("a NON-strict coordinator keeps the legacy claim, even over a strict-capable store", func(t *testing.T) {
		probe := &canonicalTool{name: "probe", output: "done"}
		exec := NewCoordinator(tool.Registry{"probe": probe}, 0, time.Now, CoordinatorConfig{})
		store := &strictApprovalStore{approvalStoreFake: approved(), startErr: errRevoked}
		if _, err := exec.ResumeApproved(context.Background(), store, "apr3_1"); err != nil {
			t.Fatalf("ResumeApproved: %v", err)
		}
		if store.claimCalls != 1 || store.startCalls != 0 || probe.calls.Load() != 1 {
			t.Errorf("legacy claims=%d strict starts=%d dispatches=%d, want 1, 0 and 1", store.claimCalls, store.startCalls, probe.calls.Load())
		}
	})
}

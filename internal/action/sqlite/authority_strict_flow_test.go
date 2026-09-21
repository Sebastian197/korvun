// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/identity"
)

func parkStrictAuthorityFixture(t *testing.T, f authoritySQLiteFixture) AuthorityPendingResult {
	t.Helper()
	const requestID = "request-strict-pending"
	ingress, err := f.issuer.Issue(requestID, "verified-subject")
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.store.ParkAuthorization(context.Background(), AuthorityPendingRequest{
		ActorPrincipalID: "principal_brain_alpha",
		CorrelationID:    requestID,
		SourceProtocol:   "native",
		Channel:          "webhook",
		Operation:        action.Operation{Namespace: "tool", Name: "probe", Version: 1},
		Arguments:        `{}`,
		EffectClass:      action.EffectWriteReversible,
		At:               f.now,
		ApprovalContext: action.ApprovalContext{
			ToolCage: "probe",
			Descriptor: action.EffectDescriptor{
				Class: action.EffectWriteReversible,
			},
			HasDescriptor: true,
			LawVersion:    1,
			LawDigest:     "sha256:authority-law",
			TTL:           time.Minute,
		},
		ResolveEvidence: func(actionID string) (identity.Evidence, error) {
			return f.resolver.Resolve(ingress, identity.ResolveRequest{
				ActionID: actionID, RequestID: requestID,
				Channel: "webhook", Brain: "alpha",
			})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func approveStrictAuthorityFixture(t *testing.T, f authoritySQLiteFixture, parked AuthorityPendingResult) action.Approval {
	t.Helper()
	approval, _, err := f.store.GetApproval(context.Background(), parked.ApprovalID)
	if err != nil {
		t.Fatal(err)
	}
	env, identified := operatorDecisionEnv("approve", approval.ApprovalID)
	if _, err := f.store.DecideApprovalUnderLaw(context.Background(), approval.ApprovalID,
		action.DecisionApproved, f.now.Add(time.Second), env, identified, "",
		PolicyPin{Version: 1, Digest: "sha256:authority-law"}); err != nil {
		t.Fatal(err)
	}
	return approval
}

func TestAuthority_StrictPendingDoesNotSpendAndApprovedResumeReusesAction(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 2)
	parked := parkStrictAuthorityFixture(t, f)
	if !strings.HasPrefix(parked.ActionID, "act3_") || !strings.HasPrefix(parked.ApprovalID, "apr3_") {
		t.Fatalf("ids = %q %q", parked.ActionID, parked.ApprovalID)
	}
	if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM budget_debits`); n != 0 {
		t.Fatalf("pending debits = %d", n)
	}
	approval := approveStrictAuthorityFixture(t, f, parked)
	started, err := f.store.StartApprovedAuthorization(context.Background(), approval.ApprovalID,
		PolicyPin{Version: 1, Digest: "sha256:authority-law"}, approval.ActionDigest,
		f.now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if started.ActionID != parked.ActionID || string(started.Params) != `{}` {
		t.Fatalf("started = %#v, parked = %#v", started, parked)
	}
	record, err := f.store.Get(context.Background(), parked.ActionID)
	if err != nil || record.State != action.StateAuthorized {
		t.Fatalf("started action state = %v, %v", record.State, err)
	}
	if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM authorization_starts WHERE action_id=?`, parked.ActionID); n != 1 {
		t.Fatalf("starts = %d", n)
	}
	if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM budget_debits WHERE action_id=?`, parked.ActionID); n != 4 {
		t.Fatalf("debits = %d, want total and operation for intent and grant", n)
	}
	if _, err := f.store.StartApprovedAuthorization(context.Background(), approval.ApprovalID,
		PolicyPin{Version: 1, Digest: "sha256:authority-law"}, approval.ActionDigest,
		f.now.Add(3*time.Second)); err == nil {
		t.Fatal("second approved start succeeded")
	}
	if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM authorization_snapshots WHERE action_id=? AND snapshot_kind='pending'`, parked.ActionID); n != 1 {
		t.Fatalf("pending snapshots = %d", n)
	}
}

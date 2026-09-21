// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionexecutor "github.com/Sebastian197/korvun/internal/action/executor"
	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/identity"
	"github.com/Sebastian197/korvun/internal/tool"
)

// authorityCountingTool is the physical effect of AS-AUTH-14: it does nothing
// but count how many times the coordinator's dispatcher reached it.
type authorityCountingTool struct{ dispatched atomic.Int64 }

func (*authorityCountingTool) Name() string        { return "probe" }
func (*authorityCountingTool) Description() string { return "authority dispatch counter" }
func (c *authorityCountingTool) Execute(context.Context, string) (string, error) {
	c.dispatched.Add(1)
	return "dispatched", nil
}

// authorityStrictRecorder is the strict store seam the coordinator talks to.
// It repeats, field for field, the mapping internal/app's actionRecorder makes
// in production; that adapter itself is NOT inside this mould, and the mould
// does not claim it is.
type authorityStrictRecorder struct{ store *Store }

func (r authorityStrictRecorder) RecordAttempt(ctx context.Context, env action.Envelope,
	outcome, rule string, state action.State,
) error {
	return r.store.RecordAttempt(ctx, env, Decision{Outcome: outcome, Rule: rule}, state)
}

func (r authorityStrictRecorder) RecordAttemptAuthenticated(ctx context.Context,
	env action.Envelope, outcome, rule string, state action.State, evidence identity.Evidence,
) error {
	return r.store.RecordAttemptAuthenticated(ctx, env, Decision{Outcome: outcome, Rule: rule}, state, evidence)
}

func (r authorityStrictRecorder) Finish(ctx context.Context, actionID string, state action.State, at time.Time) error {
	return r.store.Finish(ctx, actionID, state, at)
}

func (r authorityStrictRecorder) StartAuthorization(ctx context.Context,
	request actionexecutor.AuthorityStartRequest,
) (actionexecutor.AuthorityStartResult, error) {
	started, err := r.store.StartAuthorization(ctx, AuthorityStartRequest{
		ActorPrincipalID: request.ActorPrincipalID,
		CorrelationID:    request.CorrelationID, SourceProtocol: request.SourceProtocol,
		Channel: request.Channel, ConversationID: request.ConversationID,
		Operation: request.Operation, Arguments: request.Arguments,
		EffectClass: request.EffectClass, At: request.At,
		ResolveEvidence: request.ResolveEvidence,
	})
	if err != nil {
		return actionexecutor.AuthorityStartResult{}, err
	}
	return actionexecutor.AuthorityStartResult{
		ActionID: started.ActionID, IntentID: started.IntentID,
		AuthorityRefs: started.AuthorityRefs, Evidence: started.Evidence,
	}, nil
}

// authorityInsertSignedGrant persists a CORRECTLY SIGNED grant without asking
// any door whether its terms make sense — the attacker this mould models can
// sign, and what it cannot do is make a bad chain good.
func authorityInsertSignedGrant(t *testing.T, f authoritySQLiteFixture, grant action.AuthorityGrantV2, act string) {
	t.Helper()
	tx, err := f.store.beginAuthorityWrite(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.insertGrantTx(context.Background(), tx, grant, act,
		f.root.SubjectPrincipalID, false, "", f.now); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := f.store.ensureBudgetAccountTx(context.Background(), tx, grant.ProfileID,
		"grant", grant.GrantID, grant.Budget, f.now); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

// TestAuthority_SignedInvalidChainStillFails (AS-AUTH-14) enters through the
// door production uses: the real coordinator submits to the real strict store,
// and a real tool counts dispatches. Every chain below is CORRECTLY SIGNED with
// the profile key, so a verifier that stopped at the signature would hand the
// coordinator an invocation capability.
//
// Each row demands ITS named sentinel — never "some error" — and, as an oracle
// by impossibility, ZERO dispatches, zero durable starts and zero debits.
//
// Evidence level: in-process; the real executor coordinator over a real SQLite
// store file. The coordinator reaches the store through a test recorder that
// repeats app.actionRecorder's field mapping; that production adapter is not
// exercised here.
func TestAuthority_SignedInvalidChainStillFails(t *testing.T) {
	tests := []struct {
		name string
		want error
		// dimension, when set, is the exact attenuation dimension demanded of
		// an ErrAttenuationViolated: two different widenings must not be able
		// to hide behind one sentinel.
		dimension string
		build     func(*testing.T, authoritySQLiteFixture) action.AuthorityGrantV2
	}{
		{"cycle", action.ErrAuthorityEvidenceCorrupt, "",
			func(t *testing.T, f authoritySQLiteFixture) action.AuthorityGrantV2 {
				a := authorityChildForDoor(f, "grant_cycle_a")
				b := authorityChildForDoor(f, "grant_cycle_b")
				a.ParentGrantID, a.ParentGrantVersion = b.GrantID, b.Version
				b.ParentGrantID, b.ParentGrantVersion = a.GrantID, a.Version
				authorityInsertSignedGrant(t, f, a, "act_cycle_a")
				authorityInsertSignedGrant(t, f, b, "act_cycle_b")
				return a
			}},
		{"root legitimately issued under a FOREIGN intent", action.ErrAttenuationViolated, "intent",
			func(t *testing.T, f authoritySQLiteFixture) action.AuthorityGrantV2 {
				foreign := f.intent
				foreign.IntentID, foreign.Purpose = "int_foreign", "Somebody else's contract"
				if err := f.store.CreateIntentV2(context.Background(), foreign, foreign.OwnerPrincipalID, f.now.Add(-time.Minute)); err != nil {
					t.Fatal(err)
				}
				if err := f.store.ActivateIntentV2(context.Background(), foreign.IntentID, foreign.Version, foreign.OwnerPrincipalID, f.now); err != nil {
					t.Fatal(err)
				}
				root := f.root
				root.GrantID, root.IntentID, root.IntentDigest = "grant_foreign_root", foreign.IntentID, foreign.Digest()
				act := authorityActorAct(t, f.store, f.resolver, f.issuer, "issue", root.CanonicalBytes(), f.now.Add(5*time.Nanosecond))
				if err := f.store.IssueAuthority(context.Background(), root, act, f.now); err != nil {
					t.Fatal(err)
				}
				return root
			}},
		{"child that widens its signed parent", action.ErrAttenuationViolated, "operations",
			func(t *testing.T, f authoritySQLiteFixture) action.AuthorityGrantV2 {
				child := authorityChildForDoor(f, "grant_widening_child")
				child.Operations = append(child.Operations,
					action.OperationRef{Namespace: "tool", Name: "outside", Version: 1})
				authorityInsertSignedGrant(t, f, child, "act_widening_child")
				return child
			}},
		{"parent link naming a version that does not exist", action.ErrAuthorityEvidenceCorrupt, "",
			func(t *testing.T, f authoritySQLiteFixture) action.AuthorityGrantV2 {
				child := authorityChildForDoor(f, "grant_orphan_child")
				child.ParentGrantVersion = 7
				authorityInsertSignedGrant(t, f, child, "act_orphan_child")
				return child
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newAuthoritySQLiteFixture(t, 5)
			leaf := tt.build(t, f)
			// The coordinator scopes a request by the conversation KEY
			// ("<channel>::<id>"), so that is what the binding must name for the
			// hostile leaf — and not the fixture's valid root — to be selected.
			putAuthorityBinding(t, f, leaf, "webhook::hostile-chain")

			counter := &authorityCountingTool{}
			exec := actionexecutor.NewCoordinator(tool.Registry{"probe": counter}, time.Second,
				func() time.Time { return f.now }, actionexecutor.CoordinatorConfig{
					BrainName: "alpha", Recorder: authorityStrictRecorder{store: f.store},
					PrincipalResolver: f.resolver, StrictAuthority: true,
					EffectClassifier: func(string) (action.EffectDescriptor, bool) {
						return action.EffectDescriptor{Class: action.EffectWriteReversible}, true
					},
				})
			inbound := envelope.New("webhook", envelope.Inbound, envelope.Participant{ID: "authenticated-subject"})
			inbound.ID = "request-hostile-chain"
			inbound.Meta = map[string]string{conversation.MetaConversationID: "hostile-chain"}
			ingress, err := f.issuer.Issue(inbound.ID, inbound.Sender.ID)
			if err != nil {
				t.Fatal(err)
			}
			_, plan, err := exec.SelectTools(context.Background(), inbound.Channel)
			if err != nil {
				t.Fatal(err)
			}
			request, err := exec.Prepare(actionexecutor.Submission{
				Inbound: inbound, Ingress: ingress, Lane: "text", Name: "probe",
				Arguments: `{}`, Plan: plan,
			})
			if err != nil {
				t.Fatal(err)
			}
			result, submitErr := exec.Submit(context.Background(), request)
			if submitErr != nil {
				t.Errorf("Submit error = %v, want nil: a refused start is a DENIED branch, not a coordinator failure", submitErr)
			}
			if !errors.Is(result.RecordError, tt.want) {
				t.Errorf("record error = %v, want %v", result.RecordError, tt.want)
			}
			if tt.dimension != "" {
				var widened *action.AttenuationError
				if !errors.As(result.RecordError, &widened) || widened.Dimension != tt.dimension {
					t.Errorf("record error = %v, want attenuation dimension %q", result.RecordError, tt.dimension)
				}
			}
			if result.Branch != actionexecutor.BranchDenied {
				t.Errorf("branch = %q, want %q", result.Branch, actionexecutor.BranchDenied)
			}
			if n := counter.dispatched.Load(); n != 0 {
				t.Errorf("dispatcher calls = %d, want 0: a signed invalid chain reached the physical effect", n)
			}
			for what, query := range map[string]string{
				"durable starts": `SELECT COUNT(*) FROM authorization_starts`,
				"budget debits":  `SELECT COUNT(*) FROM budget_debits`,
			} {
				if n := authorityScalar(t, f.store, query); n != 0 {
					t.Errorf("%s = %d, want 0", what, n)
				}
			}
		})
	}
}

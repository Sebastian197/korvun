// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/action/executor"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/identity"
	"github.com/Sebastian197/korvun/internal/tool"
)

type strictDoorTool struct {
	name  string
	calls atomic.Int64
	args  atomic.Value
}

func (s *strictDoorTool) Name() string        { return s.name }
func (s *strictDoorTool) Description() string { return "strict door dispatch counter" }
func (s *strictDoorTool) Execute(_ context.Context, args string) (string, error) {
	s.calls.Add(1)
	s.args.Store(args)
	return "done", nil
}

// strictDoorFixture is a real strict store with one intent, one root grant and
// one binding for the authenticated brain, built ONLY through the store's
// exported doors — the same ones production uses.
type strictDoorFixture struct {
	store    *actionsqlite.Store
	resolver *identity.Resolver
	issuer   *identity.Issuer
	root     action.AuthorityGrantV2
	law      actionsqlite.PolicyPin
	dbPath   string
	seq      int
}

func newStrictDoorFixture(t *testing.T) *strictDoorFixture {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	dbPath := filepath.Join(t.TempDir(), "strict.db")
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutSigningKey(ctx, action.SigningKeyID(pub), hex.EncodeToString(pub), now); err != nil {
		t.Fatal(err)
	}
	store.SetReceiptSealer(func(r action.Receipt) action.Receipt { return action.SignReceipt(priv, r) })
	store.SetIdentitySigners(
		func(e identity.Evidence) identity.SignedEvidence { return identity.SignEvidence(priv, e) },
		func(e identity.PrincipalEvent) identity.SignedPrincipalEvent {
			return identity.SignPrincipalEvent(priv, e)
		},
	)
	store.SetIntentV2Signer(
		func(c action.IntentContractV2) action.SignedIntentContractV2 {
			return action.SignIntentContractV2(priv, c)
		},
		func(e action.IntentEventV1) action.SignedIntentEventV1 { return action.SignIntentEventV1(priv, e) },
	)
	store.SetAuthoritySigner(func(domain string, canonical []byte) action.AuthoritySignature {
		return action.SignAuthorityBytes(priv, domain, canonical)
	})
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
			Brain: "alpha", PrincipalID: "principal_brain_alpha", ResponsiblePrincipalID: "principal_responsible",
		}},
	}
	if err := store.RegisterIdentity(ctx, registry, now); err != nil {
		t.Fatal(err)
	}
	resolver, err := identity.NewResolver(registry, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := resolver.NewIssuer(identity.IssuerConfig{
		BindingID: "binding_webhook", Method: "bearer", CredentialClass: "shared_secret", TTL: 10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	f := &strictDoorFixture{store: store, resolver: resolver, issuer: issuer, dbPath: dbPath,
		law: actionsqlite.PolicyPin{Version: 1, Digest: "sha256:strict-door-law"}}

	maximum := int64(5)
	operations := []action.OperationRef{
		{Namespace: "tool", Name: "probe", Version: 1},
		{Namespace: "tool", Name: "launch", Version: 1},
	}
	effects := []action.EffectClass{action.EffectWriteReversible, action.EffectWriteIrreversible}
	intent := action.IntentContractV2{
		IntentID: "int_strict_doors", SchemaVersion: 2, Version: 1, ProfileID: "profile_a",
		OwnerPrincipalID: "principal_brain_alpha", Purpose: "Exercise the strict doors",
		Operations: operations, EffectClasses: effects,
		Budget:    action.IntentBudgetV2{Total: &maximum},
		ValidFrom: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour), MaxDelegationDepth: 2,
	}
	if err := store.CreateIntentV2(ctx, intent, intent.OwnerPrincipalID, now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateIntentV2(ctx, intent.IntentID, intent.Version, intent.OwnerPrincipalID, now); err != nil {
		t.Fatal(err)
	}
	f.root = action.AuthorityGrantV2{
		GrantID: "grant_strict_root", SchemaVersion: 2, Version: 1, ProfileID: intent.ProfileID,
		IntentID: intent.IntentID, IntentVersion: intent.Version, IntentDigest: intent.Digest(),
		IssuerPrincipalID: intent.OwnerPrincipalID, SubjectPrincipalID: "principal_brain_alpha",
		Operations: operations, Channels: []string{"webhook"}, EffectClasses: effects,
		EffectCeiling: action.EffectWriteIrreversible, Budget: action.IntentBudgetV2{Total: &maximum},
		ValidFrom: intent.ValidFrom, ExpiresAt: intent.ExpiresAt, DelegationDepthRemaining: 2,
		Status: action.LifecycleActive,
	}
	if err := store.IssueAuthority(ctx, f.root, f.actorAct(t, "issue", f.root.CanonicalBytes()), now); err != nil {
		t.Fatal(err)
	}
	if err := store.PutExecutionBinding(ctx, action.ExecutionBinding{
		BindingID: "binding_strict_root", ActorPrincipalID: "principal_brain_alpha", Channel: "webhook",
		IntentID: intent.IntentID, IntentVersion: intent.Version, IntentDigest: intent.Digest(),
		GrantID: f.root.GrantID, GrantVersion: f.root.Version, GrantDigest: f.root.Digest(),
		Revision: 1, Status: action.BindingActive,
	}); err != nil {
		t.Fatal(err)
	}
	return f
}

// actorAct records one authenticated act of the brain for an authority verb.
func (f *strictDoorFixture) actorAct(t *testing.T, verb string, params []byte) string {
	t.Helper()
	f.seq++
	id := fmt.Sprintf("act_strict_%s_%d", verb, f.seq)
	requestID := "request-" + id
	ingress, err := f.issuer.Issue(requestID, "authenticated-subject")
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := f.resolver.Resolve(ingress, identity.ResolveRequest{
		ActionID: id, RequestID: requestID, Channel: "webhook", Brain: "alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	operation := action.Operation{Namespace: "authority", Name: verb, Version: 1}
	env := action.NewEnvelope(id, requestID,
		action.Source{Kind: "agent_brain", Protocol: "text", Channel: "webhook"}, operation, string(params), time.Now().UTC())
	env.Effect = action.Effect{Class: string(action.EffectWriteReversible)}
	env.Principal = action.PrincipalRef{
		PrincipalID: evidence.ActorPrincipalID, ResponsibleHumanID: evidence.ResponsiblePrincipalID, EvidenceID: evidence.EvidenceID,
	}
	if err := f.store.RecordAttemptAuthenticated(context.Background(), env,
		actionsqlite.Decision{Outcome: "allow", Rule: "administrative"}, action.StateAuthorized, evidence); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *strictDoorFixture) coordinator(tools tool.Registry) *executor.Executor {
	recorder := approvalRecorder{actionRecorder: actionRecorder{store: f.store, pin: f.law}, ttl: time.Minute}
	return executor.NewCoordinator(tools, time.Second, time.Now, executor.CoordinatorConfig{
		BrainName: "alpha", Recorder: recorder, PrincipalResolver: f.resolver, StrictAuthority: true,
		Identity: &executor.IdentityConfig{EffectCeiling: action.EffectCritical},
		EffectClassifier: func(name string) (action.EffectDescriptor, bool) {
			if name == "launch" {
				return action.EffectDescriptor{Class: action.EffectWriteIrreversible}, true
			}
			return action.EffectDescriptor{Class: action.EffectWriteReversible}, true
		},
	})
}

func (f *strictDoorFixture) submit(t *testing.T, exec *executor.Executor, toolName string) executor.Result {
	t.Helper()
	f.seq++
	inbound := envelope.New("webhook", envelope.Inbound, envelope.Participant{ID: "authenticated-subject"})
	inbound.ID = fmt.Sprintf("request-strict-door-%d", f.seq)
	ingress, err := f.issuer.Issue(inbound.ID, inbound.Sender.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, plan, err := exec.SelectTools(context.Background(), inbound.Channel)
	if err != nil {
		t.Fatal(err)
	}
	request, err := exec.Prepare(executor.Submission{
		Inbound: inbound, Ingress: ingress, Lane: "text", Name: toolName, Arguments: `{}`, Plan: plan,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := exec.Submit(context.Background(), request)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	return result
}

// count reads through a SECOND, read-only connection: what it sees is what is
// committed, not what the store's own connection happens to hold.
func (f *strictDoorFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(f.dbPath)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// TestAuthority_StrictAppDoorsEndToEnd runs the strict boundary through the
// adapters PRODUCTION wires — actionRecorder, approvalRecorder and
// approvedExecutionStore — between the real coordinator and a real strict store.
// Before this mould no test executed any of the three: the store was proved
// alone, the coordinator was proved over fakes, and the seam that joins them in
// the running app was proved by nobody.
//
// It walks the whole life of authority in one store: an immediate start that
// commits and dispatches; a pending birth that dispatches nothing; the approved
// resume that dispatches once with the parked bytes; and then the revocation,
// after which NEITHER door starts anything.
//
// Evidence level: in-process; real coordinator, real production adapters, one
// real SQLite store built only through exported doors. NOT a booted App and NOT
// the compiled binary: app.Build under a strict config is exercised by no mould,
// and that is FILED by name — "strict boot end to end".
// Probing mutation executed: make actionRecorder.StartAuthorization answer a
// refused start with success — red, with the forbidden state observed after the
// revocation: «branch="executed"» and «probe=2».
func TestAuthority_StrictAppDoorsEndToEnd(t *testing.T) {
	f := newStrictDoorFixture(t)
	probe, launch := &strictDoorTool{name: "probe"}, &strictDoorTool{name: "launch"}
	exec := f.coordinator(tool.Registry{"probe": probe, "launch": launch})
	ctx := context.Background()

	// 1. An immediate effectful start: committed by the store, then dispatched.
	first := f.submit(t, exec, "probe")
	if first.Branch != executor.BranchExecuted || probe.calls.Load() != 1 {
		t.Fatalf("immediate start: branch=%q dispatches=%d record error=%v, want executed once", first.Branch, probe.calls.Load(), first.RecordError)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM authorization_starts WHERE action_id=?`, first.Action.ActionID); n != 1 {
		t.Errorf("durable starts for the executed action = %d, want 1", n)
	}
	if len(first.Action.AuthorityRefs) != 1 || first.Action.AuthorityRefs[0] != f.root.GrantID {
		t.Errorf("executed under refs %v, want the store-verified [%s]", first.Action.AuthorityRefs, f.root.GrantID)
	}

	// 2. An effect that needs a human: parked by the STRICT birth, nothing runs.
	parked := f.submit(t, exec, "launch")
	if parked.Branch != executor.BranchPending || parked.ApprovalID == "" || launch.calls.Load() != 0 {
		t.Fatalf("pending birth: branch=%q approval=%q dispatches=%d approval error=%v", parked.Branch, parked.ApprovalID, launch.calls.Load(), parked.ApprovalError)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM approvals WHERE approval_id=? AND authority_snapshot_required=1`, parked.ApprovalID); n != 1 {
		t.Errorf("strict approval rows = %d, want 1", n)
	}
	debitsBeforeResume := f.count(t, `SELECT COUNT(*) FROM budget_debits`)
	if n := f.count(t, `SELECT COUNT(*) FROM budget_debits WHERE action_id=?`, parked.Action.ActionID); n != 0 {
		t.Errorf("a pending request spent %d debits, want 0", n)
	}

	// 3. The human approves; the strict resume starts and dispatches ONCE.
	approval, _, err := f.store.GetApproval(ctx, parked.ApprovalID)
	if err != nil {
		t.Fatal(err)
	}
	decisionEnv := action.NewEnvelope(action.NewID(), "cli",
		action.Source{Kind: "operator", Protocol: "cli", Channel: "cli"},
		action.Operation{Namespace: "approval", Name: "approve", Version: 1},
		`{"approval_id":"`+approval.ApprovalID+`"}`, time.Now().UTC())
	decisionEnv.Principal = action.PrincipalRef{PrincipalID: action.OperatorPrincipal().PrincipalID}
	decisionEnv.IntentID = action.RootIntentID
	if _, err := f.store.DecideApprovalUnderLaw(ctx, approval.ApprovalID, action.DecisionApproved, time.Now().UTC(), decisionEnv,
		actionsqlite.AttemptIdentity{PrincipalID: action.OperatorPrincipal().PrincipalID, IntentID: action.RootIntentID}, "", f.law); err != nil {
		t.Fatal(err)
	}
	resumed, err := exec.ResumeApproved(ctx, approvedExecutionStore{store: f.store, law: f.law, approvedDigest: approval.ActionDigest}, approval.ApprovalID)
	if err != nil {
		t.Fatalf("strict resume: %v", err)
	}
	if launch.calls.Load() != 1 || launch.args.Load() != `{}` || resumed.Operation.Name != "launch" {
		t.Errorf("resume: dispatches=%d args=%v operation=%q, want one dispatch of the parked bytes", launch.calls.Load(), launch.args.Load(), resumed.Operation.Name)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM authorization_starts WHERE action_id=?`, parked.Action.ActionID); n != 1 {
		t.Errorf("durable starts for the resumed action = %d, want 1 under the SAME action id", n)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM budget_debits`); n <= debitsBeforeResume {
		t.Errorf("debits did not move across the resume (%d -> %d)", debitsBeforeResume, n)
	}

	// 4. The authority is revoked. Neither door starts anything any more.
	if err := f.store.RevokeAuthority(ctx, f.root.GrantID,
		f.actorAct(t, "revoke", actionsqlite.CanonicalAuthorityRevoke(f.root.GrantID, "stop")), "stop", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	startsBefore := f.count(t, `SELECT COUNT(*) FROM authorization_starts`)
	refused := f.submit(t, exec, "probe")
	if !errors.Is(refused.RecordError, actionsqlite.ErrAuthorityRevoked) || refused.Branch != executor.BranchDenied {
		t.Errorf("immediate start after the revocation: error=%v branch=%q, want %v and denied", refused.RecordError, refused.Branch, actionsqlite.ErrAuthorityRevoked)
	}
	refusedPark := f.submit(t, exec, "launch")
	if !errors.Is(refusedPark.ApprovalError, actionsqlite.ErrAuthorityRevoked) || refusedPark.Branch != executor.BranchDenied {
		t.Errorf("pending birth after the revocation: error=%v branch=%q, want %v and denied", refusedPark.ApprovalError, refusedPark.Branch, actionsqlite.ErrAuthorityRevoked)
	}
	if probe.calls.Load() != 1 || launch.calls.Load() != 1 {
		t.Errorf("dispatches after the revocation: probe=%d launch=%d, want both still 1", probe.calls.Load(), launch.calls.Load())
	}
	if n := f.count(t, `SELECT COUNT(*) FROM authorization_starts`); n != startsBefore {
		t.Errorf("durable starts moved from %d to %d after the revocation", startsBefore, n)
	}
}

// TestPrepareStrictAuthority_PinsTheRootAndSyncsTheClauses is the success half of
// the strict boot preparation, which no test reached: the refusal was pinned
// (TestPrepareStrictAuthority_RefusesUnverifiedActivation) and the path a
// correctly activated profile takes was executed by nobody.
//
// With the exact pinned digest the preparation verifies the root and persists
// one signed clause generation per agent brain; repeating it changes nothing;
// and any other digest is refused WITHOUT advancing a generation.
//
// Evidence level: in-process, one real SQLite store activated through the
// store's own administrative door. app.Build itself is still exercised by no
// mould — FILED as "strict boot end to end".
// Probing mutation executed: skip the activation check inside
// PrepareStrictAuthority — red, and NOT on the wrong-digest row: without the
// check the store never pins a profile, and the clause sync — a second belt —
// refuses the very first pass as «authority evidence corrupt».
func TestPrepareStrictAuthority_PinsTheRootAndSyncsTheClauses(t *testing.T) {
	f := newStrictDoorFixture(t)
	ctx := context.Background()
	manifest, err := f.store.AuthorityActivationManifest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const reason = "enable strict authority"
	act := f.actorAct(t, "activate", actionsqlite.CanonicalAuthorityActivation("profile_a", manifest, reason))
	digest, err := f.store.ActivateAuthority(ctx, "profile_a", act, reason, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	brains := []config.BrainConfig{{Name: "alpha", Sensitivity: "public", Agent: &config.AgentConfig{Tools: []string{"time"}}}}
	pinned := &config.Config{Authority: &config.AuthorityConfig{Mode: "strict", ActivationDigest: digest}, Brains: brains}

	for pass := 1; pass <= 2; pass++ {
		if err := PrepareStrictAuthority(ctx, pinned, f.store); err != nil {
			t.Fatalf("preparation pass %d under the pinned digest: %v", pass, err)
		}
		if n := f.count(t, `SELECT COUNT(*) FROM config_authority_heads WHERE brain_principal_id='principal_brain_alpha' AND generation=1`); n != 1 {
			t.Errorf("after pass %d, generation-1 heads for the brain = %d, want 1: a repeated preparation must not advance it", pass, n)
		}
	}
	if got := f.store.ActivatedAuthorityProfile(); got != "profile_a" {
		t.Errorf("activated profile = %q, want %q", got, "profile_a")
	}

	other := &config.Config{Authority: &config.AuthorityConfig{
		Mode: "strict", ActivationDigest: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
	}, Brains: brains}
	if err := PrepareStrictAuthority(ctx, other, f.store); !errors.Is(err, actionsqlite.ErrAuthorizationSnapshotCorrupt) {
		t.Errorf("preparation under another digest = %v, want %v", err, actionsqlite.ErrAuthorizationSnapshotCorrupt)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM config_authority_heads`); n != 1 {
		t.Errorf("config heads after the refused preparation = %d, want the same 1", n)
	}
	// A profile that is not strict is left alone, and a strict one needs a store.
	if err := PrepareStrictAuthority(ctx, &config.Config{}, f.store); err != nil {
		t.Errorf("a non-strict config = %v, want nil", err)
	}
	if err := PrepareStrictAuthority(ctx, pinned, nil); err == nil || !strings.Contains(err.Error(), "strict authority requires an action store") {
		t.Errorf("a strict config with no store = %v, want the refusal that names the missing store", err)
	}
}

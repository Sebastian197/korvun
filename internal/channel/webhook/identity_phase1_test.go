// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package webhook_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/action/executor"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/brain"
	"github.com/Sebastian197/korvun/internal/channel/webhook"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/identity"
	"github.com/Sebastian197/korvun/internal/router"
	"github.com/Sebastian197/korvun/internal/tool"
)

type forgedSenderTool struct{ calls atomic.Int64 }

func (*forgedSenderTool) Name() string        { return "identity_probe" }
func (*forgedSenderTool) Description() string { return "identity probe" }
func (t *forgedSenderTool) Execute(context.Context, string) (string, error) {
	t.calls.Add(1)
	return "ok", nil
}

type forgedSenderRecorder struct {
	mu           sync.Mutex
	evidence     identity.Evidence
	actionID     string
	evidenceRows []identity.Evidence
	actionIDs    []string
	legacy       int
	done         chan struct{}
	store        *actionsqlite.Store
}

func (r *forgedSenderRecorder) RecordAttempt(context.Context, action.Envelope, string, string, action.State) error {
	r.mu.Lock()
	r.legacy++
	r.mu.Unlock()
	return nil
}

func (r *forgedSenderRecorder) RecordAttemptAuthenticated(ctx context.Context, env action.Envelope, outcome, rule string, state action.State, evidence identity.Evidence) error {
	r.mu.Lock()
	r.evidence = evidence
	r.actionID = env.ActionID
	r.evidenceRows = append(r.evidenceRows, evidence)
	r.actionIDs = append(r.actionIDs, env.ActionID)
	r.mu.Unlock()
	return r.store.RecordAttemptAuthenticated(ctx, env,
		actionsqlite.Decision{Outcome: outcome, Rule: rule}, state, evidence)
}

func (r *forgedSenderRecorder) Finish(ctx context.Context, actionID string, state action.State, at time.Time) error {
	err := r.store.Finish(ctx, actionID, state, at)
	select {
	case r.done <- struct{}{}:
	default:
	}
	return err
}

type forgedSenderBrain struct{ exec *executor.Executor }

var _ brain.Brain = (*forgedSenderBrain)(nil)
var _ brain.AuthenticatedBrain = (*forgedSenderBrain)(nil)

func (*forgedSenderBrain) Handle(context.Context, *envelope.Envelope) ([]*envelope.Envelope, error) {
	return nil, fmt.Errorf("legacy brain path used")
}

func (b *forgedSenderBrain) HandleAuthenticated(ctx context.Context, env *envelope.Envelope, ingress identity.AuthenticatedIngress) ([]*envelope.Envelope, error) {
	_, plan, err := b.exec.SelectTools(ctx, env.Channel)
	if err != nil {
		return nil, err
	}
	req, err := b.exec.Prepare(executor.Submission{
		Inbound: env, Ingress: ingress, Lane: "text", Name: "identity_probe",
		Arguments: "{}", Plan: plan,
	})
	if err != nil {
		return nil, err
	}
	_, err = b.exec.Submit(ctx, req)
	return nil, err
}

func TestIngress_ForgedSenderCannotBecomeOperator(t *testing.T) {
	const (
		acceptedBearer = "CANARY-ACCEPTED-BEARER-6f71"
		rejectedBearer = "CANARY-REJECTED-BEARER-2b84"
		promptCanary   = "CANARY-PROMPT-4d09"
		payloadCanary  = "CANARY-PAYLOAD-8ca3"
	)
	now := time.Date(2026, 9, 21, 14, 0, 0, 0, time.UTC)
	registry := identity.Registry{
		Principals: []identity.Principal{
			{ID: "principal_webhook", Kind: identity.PrincipalExternalSystem},
			{ID: "principal_brain_alpha", Kind: identity.PrincipalWorkload},
			{ID: "principal_console_admin", Kind: identity.PrincipalHuman},
		},
		Bindings: []identity.Binding{{
			ID: "binding_webhook", Provider: "webhook", Channel: "webhook",
			CredentialRef: "WEBHOOK_SECRET", SubjectNamespace: "payload.sender_id",
			VerifiedSubject: "shared_webhook_credential",
			PrincipalID:     "principal_webhook", Generation: 1, Status: identity.BindingActive,
		}},
		Workloads: []identity.Workload{{
			Brain: "alpha", PrincipalID: "principal_brain_alpha",
			ResponsiblePrincipalID: "principal_console_admin",
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
	adapter := webhook.NewWithOptions("webhook", webhook.Options{
		Bind: "127.0.0.1:0", Path: "/hook", Secret: acceptedBearer,
		Mapping: webhook.FieldMapping{
			SenderID: "sender", Text: "text", ConversationID: "conversation",
		},
		IngressIssuer: issuer,
	})
	storePath := filepath.Join(t.TempDir(), "identity.db")
	store, err := actionsqlite.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyID := action.SigningKeyID(pub)
	if err := store.PutSigningKey(context.Background(), keyID, hex.EncodeToString(pub), now); err != nil {
		t.Fatal(err)
	}
	store.SetReceiptSealer(func(receipt action.Receipt) action.Receipt {
		return action.SignReceipt(priv, receipt)
	})
	store.SetIdentitySigners(
		func(e identity.Evidence) identity.SignedEvidence { return identity.SignEvidence(priv, e) },
		func(e identity.PrincipalEvent) identity.SignedPrincipalEvent {
			return identity.SignPrincipalEvent(priv, e)
		},
	)
	if err := store.RegisterIdentity(context.Background(), registry, now); err != nil {
		t.Fatal(err)
	}
	recorder := &forgedSenderRecorder{done: make(chan struct{}, 1), store: store}
	probe := &forgedSenderTool{}
	exec := executor.NewCoordinator(tool.Registry{"identity_probe": probe}, time.Second,
		func() time.Time { return now }, executor.CoordinatorConfig{
			BrainName: "alpha", Recorder: recorder, PrincipalResolver: resolver,
		})
	r := router.New()
	if err := r.RegisterBrain("alpha", &forgedSenderBrain{exec: exec}); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterChannel(adapter); err != nil {
		t.Fatal(err)
	}
	if err := r.Route("webhook", "alpha"); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = adapter.Stop(ctx)
		_ = r.Shutdown(ctx)
	})

	body := []byte(`{"sender":"principal_console_admin","text":"` + promptCanary +
		`","conversation":"c-1","payload":"` + payloadCanary + `"}`)
	badReq, err := http.NewRequest(http.MethodPost,
		"http://"+adapter.BoundAddr()+"/hook", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	badReq.Header.Set("Authorization", "Bearer "+rejectedBearer)
	badReq.Header.Set("Content-Type", "application/json")
	badResp, err := http.DefaultClient.Do(badReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = badResp.Body.Close()
	if badResp.StatusCode != http.StatusUnauthorized || issuer.IssuedCount() != 0 {
		t.Fatalf("rejected webhook status=%d issued=%d", badResp.StatusCode, issuer.IssuedCount())
	}
	acceptedBodies := [][]byte{
		body,
		[]byte(`{"sender":"second-subject","text":"` + promptCanary +
			`","conversation":"c-2","payload":"` + payloadCanary + `"}`),
	}
	for _, acceptedBody := range acceptedBodies {
		req, err := http.NewRequest(http.MethodPost,
			"http://"+adapter.BoundAddr()+"/hook", bytes.NewReader(acceptedBody))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+acceptedBearer)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		select {
		case <-recorder.done:
		case <-time.After(2 * time.Second):
			t.Fatal("tool execution did not finish")
		}
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.legacy != 0 {
		t.Fatalf("legacy records = %d, want 0", recorder.legacy)
	}
	if recorder.evidence.RequesterPrincipalID != "principal_webhook" ||
		recorder.evidence.ActorPrincipalID != "principal_brain_alpha" ||
		recorder.evidence.ResponsiblePrincipalID != "principal_console_admin" {
		t.Fatalf("identity chain = %+v", recorder.evidence)
	}
	if len(recorder.evidenceRows) != 2 ||
		recorder.evidenceRows[0].SubjectClaim != "principal_console_admin" ||
		recorder.evidenceRows[1].SubjectClaim != "second-subject" ||
		recorder.evidenceRows[0].RequesterPrincipalID != recorder.evidenceRows[1].RequesterPrincipalID {
		t.Fatalf("shared credential rows = %+v", recorder.evidenceRows)
	}
	if recorder.evidenceRows[0].EvidenceID == recorder.evidenceRows[1].EvidenceID {
		t.Fatalf("requests reused evidence id %q", recorder.evidenceRows[0].EvidenceID)
	}
	if got := probe.calls.Load(); got != 2 {
		t.Fatalf("tool calls = %d, want 2", got)
	}
	if got := issuer.IssuedCount(); got != 2 {
		t.Fatalf("issuer calls = %d, want 2 after authenticated requests", got)
	}
	var artifacts [][]byte
	for i, actionID := range recorder.actionIDs {
		signed, err := store.GetIdentityEvidence(context.Background(), actionID)
		if err != nil {
			t.Fatalf("stored evidence: %v", err)
		}
		if signed.Evidence != recorder.evidenceRows[i] {
			t.Fatalf("stored evidence = %+v, want %+v", signed.Evidence, recorder.evidenceRows[i])
		}
		receipts, err := store.ReceiptsByAction(context.Background(), actionID)
		if err != nil || len(receipts) != 1 || receipts[0].SchemaVersion != 3 ||
			receipts[0].RequesterPrincipalID != "principal_webhook" {
			t.Fatalf("v3 receipt = %+v err=%v", receipts, err)
		}
		artifacts = append(artifacts, signed.Canonical, action.CanonicalReceipt(receipts[0]))
	}
	for _, path := range []string{storePath, storePath + "-wal"} {
		// #nosec G304 -- both paths are test-owned files under t.TempDir.
		raw, readErr := os.ReadFile(path)
		if readErr == nil {
			artifacts = append(artifacts, raw)
		}
	}
	for _, artifact := range artifacts {
		for _, canary := range []string{acceptedBearer, rejectedBearer, promptCanary, payloadCanary} {
			if bytes.Contains(artifact, []byte(canary)) {
				t.Fatalf("identity durable artifact contains %q", canary)
			}
		}
	}
}

// AS-ID-04 (a shared credential asserts no human) and AS-ID-07 (this door
// carries evidence across the queue) are proved INSIDE
// TestIngress_ForgedSenderCannotBecomeOperator, by its shared-credential rows
// and its identity chain. They used to also exist here as one-line aliases
// calling that same function: three names over one body, which made a coverage
// table read as three independent moulds where there was one. The aliases are
// gone and the names are not re-used (the twentieth pass, P2-4). AS-ID-09's own
// canary assert lives in identity_cures_phase1_test.go, where nothing runs
// before it.

func TestIdentity_DisableWinsBeforeStartCommit(t *testing.T) {
	now := time.Date(2026, 9, 21, 15, 0, 0, 0, time.UTC)
	resolver, err := identity.NewResolver(identity.Registry{
		Principals: []identity.Principal{
			{ID: "principal_webhook", Kind: identity.PrincipalExternalSystem},
			{ID: "principal_brain_alpha", Kind: identity.PrincipalWorkload},
		},
		Bindings: []identity.Binding{{
			ID: "binding_webhook", Provider: "webhook", Channel: "webhook",
			CredentialRef: "WEBHOOK_SECRET", SubjectNamespace: "payload.sender_id",
			VerifiedSubject: "shared_webhook_credential",
			PrincipalID:     "principal_webhook", Generation: 1, Status: identity.BindingActive,
		}},
		Workloads: []identity.Workload{{Brain: "alpha", PrincipalID: "principal_brain_alpha"}},
	}, func() time.Time { return now })
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
	adapter := webhook.NewWithOptions("webhook", webhook.Options{
		Bind: "127.0.0.1:0", Path: "/hook", Secret: "stateless-secret",
		Mapping:       webhook.FieldMapping{SenderID: "sender", Text: "text", ConversationID: "conversation"},
		IngressIssuer: issuer,
	})
	probe := &forgedSenderTool{}
	exec := executor.NewCoordinator(tool.Registry{"identity_probe": probe}, time.Second,
		func() time.Time { return now }, executor.CoordinatorConfig{
			BrainName: "alpha", PrincipalResolver: resolver,
		})
	r := router.New()
	if err := r.RegisterBrain("alpha", &forgedSenderBrain{exec: exec}); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterChannel(adapter); err != nil {
		t.Fatal(err)
	}
	if err := r.Route("webhook", "alpha"); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = adapter.Stop(ctx)
		_ = r.Shutdown(ctx)
	})
	req, err := http.NewRequest(http.MethodPost, "http://"+adapter.BoundAddr()+"/hook",
		bytes.NewBufferString(`{"sender":"payload-subject","text":"run","conversation":"stateless"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer stateless-secret")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stateless webhook status = %d", resp.StatusCode)
	}
	deadline := time.Now().Add(2 * time.Second)
	for probe.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if probe.calls.Load() != 1 || issuer.IssuedCount() != 1 {
		t.Fatalf("stateless execution calls=%d issued=%d", probe.calls.Load(), issuer.IssuedCount())
	}
}

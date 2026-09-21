// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package router_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/brain"
	"github.com/Sebastian197/korvun/internal/channel"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/identity"
	"github.com/Sebastian197/korvun/internal/router"
)

type identityQueueChannel struct {
	name string
	in   chan *envelope.Envelope
}

func (c *identityQueueChannel) Name() string { return c.name }
func (*identityQueueChannel) Manifest() channel.Manifest {
	return channel.Manifest{Text: true}
}
func (*identityQueueChannel) Send(context.Context, *envelope.Envelope) error { return nil }
func (c *identityQueueChannel) Receive(context.Context) (<-chan *envelope.Envelope, error) {
	return c.in, nil
}

type identityQueueBrain struct {
	mu       sync.Mutex
	got      []identity.AuthenticatedIngress
	legacy   int
	received chan struct{}
}

var _ brain.Brain = (*identityQueueBrain)(nil)
var _ brain.AuthenticatedBrain = (*identityQueueBrain)(nil)

func (b *identityQueueBrain) Handle(context.Context, *envelope.Envelope) ([]*envelope.Envelope, error) {
	b.mu.Lock()
	b.legacy++
	b.mu.Unlock()
	return nil, nil
}

func (b *identityQueueBrain) HandleAuthenticated(_ context.Context, _ *envelope.Envelope, ingress identity.AuthenticatedIngress) ([]*envelope.Envelope, error) {
	b.mu.Lock()
	b.got = append(b.got, ingress)
	b.mu.Unlock()
	b.received <- struct{}{}
	return nil, nil
}

func TestIdentity_AllIngressDoorsCarryEvidence(t *testing.T) {
	now := time.Date(2026, 9, 21, 15, 0, 0, 0, time.UTC)
	doors := []struct {
		channel   string
		requester string
		method    string
	}{
		{"console", "principal_console", "bearer"},
		{"webhook", "principal_webhook", "bearer"},
		{"telegram-polling", "principal_telegram", "bot_polling"},
		{"telegram-webhook", "principal_telegram", "webhook_secret"},
		{"discord", "principal_discord", "gateway_session"},
	}
	registry := identity.Registry{
		Principals: []identity.Principal{
			{ID: "principal_console", Kind: identity.PrincipalExternalSystem},
			{ID: "principal_webhook", Kind: identity.PrincipalExternalSystem},
			{ID: "principal_telegram", Kind: identity.PrincipalExternalSystem},
			{ID: "principal_discord", Kind: identity.PrincipalExternalSystem},
			{ID: "principal_brain_alpha", Kind: identity.PrincipalWorkload},
		},
		Workloads: []identity.Workload{{Brain: "alpha", PrincipalID: "principal_brain_alpha"}},
	}
	for _, door := range doors {
		registry.Bindings = append(registry.Bindings, identity.Binding{
			ID: "binding_" + door.channel, Provider: door.channel,
			Channel: door.channel, CredentialRef: "configured_reference",
			SubjectNamespace: "provider_subject",
			VerifiedSubject:  "shared_" + door.channel + "_credential",
			PrincipalID:      door.requester,
			Generation:       1, Status: identity.BindingActive,
		})
	}
	resolver, err := identity.NewResolver(registry, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}

	r := router.New(router.WithBrainWorkers(1))
	t.Cleanup(func() { shutdown(t, r) })
	b := &identityQueueBrain{received: make(chan struct{}, len(doors))}
	if err := r.RegisterBrain("alpha", b); err != nil {
		t.Fatal(err)
	}

	for _, door := range doors {
		ch := &identityQueueChannel{name: door.channel, in: make(chan *envelope.Envelope, 1)}
		if err := r.RegisterChannel(ch); err != nil {
			t.Fatalf("RegisterChannel(%s): %v", door.channel, err)
		}
		if err := r.Route(door.channel, "alpha"); err != nil {
			t.Fatalf("Route(%s): %v", door.channel, err)
		}
		issuer, err := resolver.NewIssuer(identity.IssuerConfig{
			BindingID: "binding_" + door.channel, Method: door.method,
			CredentialClass: "configured", TTL: time.Minute,
		})
		if err != nil {
			t.Fatal(err)
		}
		requestID := "request-" + door.channel
		ingress, err := issuer.Issue(requestID, "subject-"+door.channel)
		if err != nil {
			t.Fatal(err)
		}
		env := envelope.New(door.channel, envelope.Inbound,
			envelope.Participant{ID: "forged-sender"}).AddText("hello")
		env.ID = requestID
		env.Meta[router.MetaConversationID] = "conversation"
		env.SetAuthenticatedIngress(ingress)
		ch.in <- env
		select {
		case <-b.received:
		case <-time.After(time.Second):
			t.Fatalf("%s did not cross the router queue", door.channel)
		}
	}
	b.mu.Lock()
	if b.legacy != 0 {
		t.Fatalf("legacy Handle calls = %d, want 0", b.legacy)
	}
	if len(b.got) != len(doors) {
		t.Fatalf("authenticated Handle calls = %d, want %d", len(b.got), len(doors))
	}
	b.mu.Unlock()
	testIdentityReloadCut(t)
}

type reloadIdentityBrain struct {
	resolver  *identity.Resolver
	entered   chan struct{}
	release   chan struct{}
	workerCtx chan (<-chan struct{})
	result    chan identity.Evidence
	calls     int
	mu        sync.Mutex
}

func (*reloadIdentityBrain) Handle(context.Context, *envelope.Envelope) ([]*envelope.Envelope, error) {
	return nil, nil
}

func (b *reloadIdentityBrain) HandleAuthenticated(ctx context.Context, env *envelope.Envelope, ingress identity.AuthenticatedIngress) ([]*envelope.Envelope, error) {
	b.mu.Lock()
	b.calls++
	call := b.calls
	b.mu.Unlock()
	if call == 1 {
		b.workerCtx <- ctx.Done()
		close(b.entered)
		<-b.release
	}
	evidence, err := b.resolver.Resolve(ingress, identity.ResolveRequest{
		ActionID: "act-" + env.ID, RequestID: env.ID,
		Channel: "webhook", Brain: "alpha",
	})
	if err == nil {
		b.result <- evidence
	}
	return nil, err
}

func testIdentityReloadCut(t *testing.T) {
	t.Helper()
	now := time.Date(2026, 9, 21, 15, 30, 0, 0, time.UTC)
	registry := identity.Registry{
		Principals: []identity.Principal{
			{ID: "principal_r1", Kind: identity.PrincipalExternalSystem},
			{ID: "principal_brain_alpha", Kind: identity.PrincipalWorkload},
		},
		Bindings: []identity.Binding{{
			ID: "binding_reload", Provider: "webhook", Channel: "webhook",
			CredentialRef: "R1", SubjectNamespace: "sender",
			VerifiedSubject: "shared_reload_credential",
			PrincipalID:     "principal_r1", Generation: 1, Status: identity.BindingActive,
		}},
		Workloads: []identity.Workload{{Brain: "alpha", PrincipalID: "principal_brain_alpha"}},
	}
	resolver, err := identity.NewResolver(registry, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := resolver.NewIssuer(identity.IssuerConfig{
		BindingID: "binding_reload", Method: "bearer",
		CredentialClass: "shared_secret", TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := router.New(router.WithBrainWorkers(1), router.WithQueueCapacity(2))
	b := &reloadIdentityBrain{
		resolver: resolver, entered: make(chan struct{}), release: make(chan struct{}),
		workerCtx: make(chan (<-chan struct{}), 1), result: make(chan identity.Evidence, 2),
	}
	ch := &identityQueueChannel{name: "webhook", in: make(chan *envelope.Envelope, 2)}
	if err := r.RegisterBrain("alpha", b); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterChannel(ch); err != nil {
		t.Fatal(err)
	}
	if err := r.Route("webhook", "alpha"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"r1-acquired", "r1-queued"} {
		ingress, issueErr := issuer.Issue(id, id)
		if issueErr != nil {
			t.Fatal(issueErr)
		}
		env := envelope.New("webhook", envelope.Inbound, envelope.Participant{ID: id}).AddText("x")
		env.ID = id
		env.Meta[router.MetaConversationID] = "reload"
		env.SetAuthenticatedIngress(ingress)
		ch.in <- env
	}
	select {
	case <-b.entered:
	case <-time.After(time.Second):
		t.Fatal("old worker did not acquire R1 work")
	}
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- r.Shutdown(context.Background()) }()
	workerDone := <-b.workerCtx
	select {
	case <-workerDone:
	case <-time.After(time.Second):
		t.Fatal("old router context was not cancelled")
	}
	close(b.release)
	if err := <-shutdownDone; err != nil {
		t.Fatal(err)
	}
	select {
	case evidence := <-b.result:
		if evidence.BindingGeneration != 1 || evidence.RequesterPrincipalID != "principal_r1" {
			t.Fatalf("acquired R1 work was rebuilt: %+v", evidence)
		}
	case <-time.After(time.Second):
		t.Fatal("acquired R1 work did not finish")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.calls != 1 {
		t.Fatalf("old queued work calls = %d, want only the acquired call", b.calls)
	}
}

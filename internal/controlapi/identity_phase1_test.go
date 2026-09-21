// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package controlapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/brain"
	"github.com/Sebastian197/korvun/internal/channel"
	"github.com/Sebastian197/korvun/internal/controlapi"
	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/identity"
	"github.com/Sebastian197/korvun/internal/router"
)

type consoleIdentityChannel struct{ inbound chan *envelope.Envelope }

func (*consoleIdentityChannel) Name() string { return "console" }
func (*consoleIdentityChannel) Manifest() channel.Manifest {
	return channel.Manifest{Text: true}
}
func (*consoleIdentityChannel) Send(context.Context, *envelope.Envelope) error { return nil }
func (c *consoleIdentityChannel) Receive(context.Context) (<-chan *envelope.Envelope, error) {
	return c.inbound, nil
}

type consoleIdentityResult struct {
	evidence identity.Evidence
	err      error
}

type consoleIdentityBrain struct {
	resolver *identity.Resolver
	results  chan consoleIdentityResult
}

var _ brain.AuthenticatedBrain = (*consoleIdentityBrain)(nil)

func (*consoleIdentityBrain) Handle(context.Context, *envelope.Envelope) ([]*envelope.Envelope, error) {
	return nil, errors.New("legacy console brain path used")
}

func (b *consoleIdentityBrain) HandleAuthenticated(_ context.Context, env *envelope.Envelope, ingress identity.AuthenticatedIngress) ([]*envelope.Envelope, error) {
	evidence, err := b.resolver.Resolve(ingress, identity.ResolveRequest{
		ActionID: "act-" + env.ID, RequestID: env.ID,
		Channel: "console", Brain: "alpha",
	})
	b.results <- consoleIdentityResult{evidence: evidence, err: err}
	return nil, err
}

func TestIdentity_AllIngressDoorsCarryEvidence(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	resolver, err := identity.NewResolver(identity.Registry{
		Principals: []identity.Principal{
			{ID: "principal_console", Kind: identity.PrincipalExternalSystem},
			{ID: "principal_brain_alpha", Kind: identity.PrincipalWorkload},
		},
		Bindings: []identity.Binding{{
			ID: "binding_console", Provider: "console", Channel: "console",
			CredentialRef: "console_bearer", SubjectNamespace: "local_profile",
			VerifiedSubject: "shared_console_credential",
			PrincipalID:     "principal_console", Generation: 1,
			Status: identity.BindingActive,
		}},
		Workloads: []identity.Workload{{Brain: "alpha", PrincipalID: "principal_brain_alpha"}},
	}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := resolver.NewIssuer(identity.IssuerConfig{
		BindingID: "binding_console", Method: "bearer",
		CredentialClass: "shared_secret", TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	r := router.New()
	brain := &consoleIdentityBrain{resolver: resolver, results: make(chan consoleIdentityResult, 1)}
	if err := r.RegisterBrain("alpha", brain); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterChannel(&consoleIdentityChannel{inbound: make(chan *envelope.Envelope)}); err != nil {
		t.Fatal(err)
	}
	if err := r.Route("console", "alpha"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = r.Shutdown(ctx)
	})

	mux := http.NewServeMux()
	controlapi.RegisterConsole(mux, "console-secret", conversation.NewMemStore(), r, issuer)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	post := func(token string) *http.Response {
		req, reqErr := http.NewRequest(http.MethodPost,
			server.URL+"/api/conversations/console::c-1/message",
			strings.NewReader(`{"text":"hello"}`))
		if reqErr != nil {
			t.Fatal(reqErr)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		response, doErr := http.DefaultClient.Do(req)
		if doErr != nil {
			t.Fatal(doErr)
		}
		return response
	}
	bad := post("wrong")
	_ = bad.Body.Close()
	if bad.StatusCode != http.StatusUnauthorized || issuer.IssuedCount() != 0 {
		t.Fatalf("rejected console status=%d issued=%d", bad.StatusCode, issuer.IssuedCount())
	}
	accepted := post("console-secret")
	_ = accepted.Body.Close()
	if accepted.StatusCode != http.StatusAccepted {
		t.Fatalf("accepted console status = %d", accepted.StatusCode)
	}
	select {
	case result := <-brain.results:
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.evidence.RequesterPrincipalID != "principal_console" ||
			result.evidence.Method != "bearer" ||
			result.evidence.BindingID != "binding_console" ||
			result.evidence.SubjectClaim != "local_profile" {
			t.Fatalf("console evidence = %+v", result.evidence)
		}
	case <-time.After(time.Second):
		t.Fatal("console message did not cross the router queue")
	}
}

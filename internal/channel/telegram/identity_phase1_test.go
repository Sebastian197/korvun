// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/brain"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/identity"
	"github.com/Sebastian197/korvun/internal/router"
)

type telegramIdentityResult struct {
	evidence identity.Evidence
	err      error
}

type telegramIdentityBrain struct {
	resolver *identity.Resolver
	results  chan telegramIdentityResult
}

var _ brain.AuthenticatedBrain = (*telegramIdentityBrain)(nil)

func (*telegramIdentityBrain) Handle(context.Context, *envelope.Envelope) ([]*envelope.Envelope, error) {
	return nil, errors.New("legacy Telegram brain path used")
}

func (b *telegramIdentityBrain) HandleAuthenticated(_ context.Context, env *envelope.Envelope, ingress identity.AuthenticatedIngress) ([]*envelope.Envelope, error) {
	evidence, err := b.resolver.Resolve(ingress, identity.ResolveRequest{
		ActionID: "act-" + env.ID, RequestID: env.ID,
		Channel: ChannelName, Brain: "alpha",
	})
	b.results <- telegramIdentityResult{evidence: evidence, err: err}
	return nil, err
}

func telegramIdentityRuntime(t *testing.T, method string) (*identity.Resolver, *identity.Issuer) {
	t.Helper()
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	resolver, err := identity.NewResolver(identity.Registry{
		Principals: []identity.Principal{
			{ID: "principal_telegram", Kind: identity.PrincipalExternalSystem},
			{ID: "principal_brain_alpha", Kind: identity.PrincipalWorkload},
		},
		Bindings: []identity.Binding{{
			ID: "binding_telegram", Provider: ChannelName, Channel: ChannelName,
			CredentialRef: "TELEGRAM_TOKEN", SubjectNamespace: "telegram_account",
			VerifiedSubject: "shared_telegram_bot_session",
			PrincipalID:     "principal_telegram", Generation: 1,
			Status: identity.BindingActive,
		}},
		Workloads: []identity.Workload{{Brain: "alpha", PrincipalID: "principal_brain_alpha"}},
	}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := resolver.NewIssuer(identity.IssuerConfig{
		BindingID: "binding_telegram", Method: method,
		CredentialClass: "bot_token_session", TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	return resolver, issuer
}

func runTelegramIdentityQueue(t *testing.T, adapter *Adapter, resolver *identity.Resolver, deliver func()) identity.Evidence {
	t.Helper()
	r := router.New()
	brain := &telegramIdentityBrain{resolver: resolver, results: make(chan telegramIdentityResult, 1)}
	if err := r.RegisterBrain("alpha", brain); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterChannel(adapter); err != nil {
		t.Fatal(err)
	}
	if err := r.Route(ChannelName, "alpha"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = r.Shutdown(ctx)
	})
	deliver()
	select {
	case result := <-brain.results:
		if result.err != nil {
			t.Fatalf("Resolve after queue: %v", result.err)
		}
		return result.evidence
	case <-time.After(time.Second):
		t.Fatal("authenticated Telegram work did not cross the router queue")
		return identity.Evidence{}
	}
}

func TestIdentity_AllIngressDoorsCarryEvidence(t *testing.T) {
	t.Run("polling callback", func(t *testing.T) {
		resolver, issuer := telegramIdentityRuntime(t, "polling")
		adapter, err := New(
			WithToken("test-token"), WithMode(ModePolling),
			WithIngressIssuer(issuer), withInjectedBotForTests(stubBotClient{}),
		)
		if err != nil {
			t.Fatal(err)
		}
		adapter.dispatchUpdate(context.Background(), newTextUpdate(77, 222, "pre-auth"))
		if got := issuer.IssuedCount(); got != 0 {
			t.Fatalf("direct polling dispatch issued %d capabilities, want 0", got)
		}
		<-adapter.inbound
		evidence := runTelegramIdentityQueue(t, adapter, resolver, func() {
			adapter.handleLibraryUpdate(context.Background(), nil,
				newTextUpdate(77, 222, "authenticated"))
		})
		if evidence.RequesterPrincipalID != "principal_telegram" ||
			evidence.Method != "polling" || evidence.BindingID != "binding_telegram" ||
			evidence.SubjectClaim != "1001" {
			t.Fatalf("polling evidence = %+v", evidence)
		}
	})

	t.Run("webhook secret", func(t *testing.T) {
		resolver, issuer := telegramIdentityRuntime(t, "webhook_secret")
		adapter, err := New(
			WithToken("test-token"), WithMode(ModeWebhook),
			WithWebhookURL("https://example.com/telegram"), WithListenAddr("127.0.0.1:0"),
			WithSecretToken("telegram-secret"), WithReverseProxyTermination(),
			WithIngressIssuer(issuer), withInjectedBotForTests(stubBotClient{}),
		)
		if err != nil {
			t.Fatal(err)
		}
		body := mustMarshal(t, newTextUpdate(88, 333, "authenticated"))
		bad := newWebhookRequest(t, "wrong-secret", body)
		badResponse := httptest.NewRecorder()
		adapter.webhookHandler().ServeHTTP(badResponse, bad)
		if badResponse.Code != http.StatusUnauthorized || issuer.IssuedCount() != 0 {
			t.Fatalf("rejected webhook status=%d issued=%d", badResponse.Code, issuer.IssuedCount())
		}
		evidence := runTelegramIdentityQueue(t, adapter, resolver, func() {
			response := httptest.NewRecorder()
			adapter.webhookHandler().ServeHTTP(response,
				newWebhookRequest(t, "telegram-secret", body))
			if response.Code != http.StatusOK {
				t.Fatalf("accepted webhook status = %d", response.Code)
			}
		})
		if evidence.RequesterPrincipalID != "principal_telegram" ||
			evidence.Method != "webhook_secret" || evidence.BindingID != "binding_telegram" ||
			evidence.SubjectClaim != "1001" {
			t.Fatalf("webhook evidence = %+v", evidence)
		}
	})
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/channel/webhook"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/conversation"
)

// This file is the SP5 TDD contract for wiring the webhook channel into internal/app
// (ADR-0038): defaultChannelFactory gains a "webhook" case that resolves the inbound
// token_env (env-only, ErrMissingSecret parity with telegram/discord), the optional
// outbound_token_env, and translates config.Effective{Bind,Path,Mapping} into
// webhook.Options; plus an isLoopbackBind decision helper that drives a non-loopback
// boot warning (ADR-0038 §7). RED-first: it references isLoopbackBind (undefined until
// SP5) so the app test package does not compile yet; and even compiled, the factory's
// missing "webhook" case makes the flow tests fail. Reuses fakeRegistrar
// (metrics_wiring_test.go) and newCapturingLogger/attrString (logfields_test.go).
//
// HARD RULE: every test bind is "127.0.0.1:0" (ephemeral). The 8090 default is NOT
// exercised by starting a server (a fixed port is forbidden in tests); it is already
// pinned by SP1 (config.DefaultWebhookBind) and SP2 (the adapter's defaultBind). The
// one non-loopback bind below ("0.0.0.0:0") is only ever BUILT, never Started, so no
// non-loopback socket is ever opened.

// webhookInboundEnv is the NAME of the env var holding the inbound secret (never the
// secret itself — that is the whole ADR-0010 point). Named without the word "secret"
// so gosec's credential heuristic does not false-positive on an env-var name.
const webhookInboundEnv = "KORVUN_WEBHOOK_INBOUND_ENV_TEST"

// webhookChannelCfg is a webhook ChannelConfig for the factory tests.
func webhookChannelCfg(bind, path string, mapping *config.WebhookMapping) config.ChannelConfig {
	return config.ChannelConfig{
		Type:     "webhook",
		TokenEnv: webhookInboundEnv,
		Webhook: &config.WebhookConfig{
			Bind:        bind,
			Path:        path,
			OutboundURL: "https://downstream.example/in",
			Mapping:     mapping,
		},
	}
}

// webhookCfg builds a one-webhook-channel, one-brain, one-route config for the
// Build-level tests.
func webhookCfg(cc config.ChannelConfig) *config.Config {
	c := cfgWith(ollamaBrain())
	c.Channels[0] = cc
	c.Routes[0].Channel = "webhook"
	return c
}

// postAuthed posts a JSON payload with the Bearer secret and returns the response.
func postAuthed(t *testing.T, url, secret string, payload map[string]string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

// TestWebhookFactory_effectivesFlow pins case (a): the factory builds a *webhook.Adapter
// from the config, threading config.Effective{Bind,Path,Mapping} into webhook.Options —
// proven by POSTing to a CUSTOM path with a RENAMED conversation field and seeing the
// Envelope carry the right conversation key. Built via defaultChannelFactory (the app's
// real construction path) and Started standalone so the read is race-free (the router
// pump would otherwise be the inbound consumer).
func TestWebhookFactory_effectivesFlow(t *testing.T) {
	t.Setenv(webhookInboundEnv, "the-inbound-secret")
	cc := webhookChannelCfg("127.0.0.1:0", "/custom-hook", &config.WebhookMapping{ConversationID: "thread"})
	b := &builder{logger: slog.New(slog.DiscardHandler)}

	ch, err := defaultChannelFactory(b, cc)
	if err != nil {
		t.Fatalf("factory error: %v", err)
	}
	ad, ok := ch.(*webhook.Adapter)
	if !ok {
		t.Fatalf("channel is %T, want *webhook.Adapter", ch)
	}
	if ad.Name() != "webhook" {
		t.Errorf("Name() = %q, want %q", ad.Name(), "webhook")
	}
	if err := ad.Start(context.Background()); err != nil {
		t.Fatalf("Start(): %v", err)
	}
	defer func() { _ = ad.Stop(context.Background()) }()

	resp := postAuthed(t, "http://"+ad.BoundAddr()+"/custom-hook", "the-inbound-secret", map[string]string{
		"sender_id": "user-1",
		"text":      "hi",
		"thread":    "T-1",
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST status = %d, want 200 (custom path + secret wired)", resp.StatusCode)
	}

	select {
	case env := <-ad.Inbound():
		if got := env.Meta[conversation.MetaConversationID]; got != "T-1" {
			t.Errorf("conversation key = %q, want %q (renamed mapping field flowed)", got, "T-1")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the Envelope")
	}
}

// TestWebhookFactory_defaultPathFlows pins case (b): a webhook block with no path
// serves at the default /webhook.
func TestWebhookFactory_defaultPathFlows(t *testing.T) {
	t.Setenv(webhookInboundEnv, "the-inbound-secret")
	cc := webhookChannelCfg("127.0.0.1:0", "", nil) // no path, no custom mapping
	b := &builder{logger: slog.New(slog.DiscardHandler)}

	ch, err := defaultChannelFactory(b, cc)
	if err != nil {
		t.Fatalf("factory error: %v", err)
	}
	ad := ch.(*webhook.Adapter)
	if err := ad.Start(context.Background()); err != nil {
		t.Fatalf("Start(): %v", err)
	}
	defer func() { _ = ad.Stop(context.Background()) }()

	resp := postAuthed(t, "http://"+ad.BoundAddr()+"/webhook", "the-inbound-secret", map[string]string{
		"sender_id": "user-1",
		"text":      "hi",
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /webhook status = %d, want 200 (default path)", resp.StatusCode)
	}
}

// TestWebhookBuild_inboundSecretMissing pins case (c): an unset inbound token_env is a
// loud, named boot error at the app layer (ErrMissingSecret parity with
// telegram/discord), naming the VAR and "(webhook inbound secret)".
func TestWebhookBuild_inboundSecretMissing(t *testing.T) {
	// The env var is deliberately NOT set.
	cfg := webhookCfg(webhookChannelCfg("127.0.0.1:0", "/hook", nil))

	_, err := Build(cfg)
	if !errors.Is(err, ErrMissingSecret) {
		t.Fatalf("err = %v, want ErrMissingSecret", err)
	}
	if !strings.Contains(err.Error(), webhookInboundEnv) {
		t.Errorf("error %q must name the missing env var", err.Error())
	}
	if !strings.Contains(err.Error(), "webhook inbound secret") {
		t.Errorf("error %q must read '(webhook inbound secret)' for telegram/discord parity", err.Error())
	}
}

// TestWebhookBuild_outboundSecretNamedButUnset pins case (d): an outbound_token_env that
// is NAMED but whose var is empty is a named boot error — an operator asking for
// outbound auth must not boot silently degraded without it.
func TestWebhookBuild_outboundSecretNamedButUnset(t *testing.T) {
	t.Setenv(webhookInboundEnv, "the-inbound-secret") // inbound resolves; outbound does not
	cc := webhookChannelCfg("127.0.0.1:0", "/hook", nil)
	cc.Webhook.OutboundTokenEnv = "KORVUN_WEBHOOK_OUTBOUND_TOKEN_TEST" // never set

	_, err := Build(webhookCfg(cc))
	if !errors.Is(err, ErrMissingSecret) {
		t.Fatalf("err = %v, want ErrMissingSecret", err)
	}
	if !strings.Contains(err.Error(), "KORVUN_WEBHOOK_OUTBOUND_TOKEN_TEST") {
		t.Errorf("error %q must name the missing outbound env var", err.Error())
	}
}

// TestWebhookBuild_outboundSecretAbsentIsOk pins case (e): with no outbound_token_env in
// the block, boot succeeds (outbound auth is optional).
func TestWebhookBuild_outboundSecretAbsentIsOk(t *testing.T) {
	t.Setenv(webhookInboundEnv, "the-inbound-secret")
	cc := webhookChannelCfg("127.0.0.1:0", "/hook", nil) // no OutboundTokenEnv

	app, err := Build(webhookCfg(cc))
	if err != nil {
		t.Fatalf("Build() error with no outbound token: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = app.Shutdown(ctx)
}

// TestWebhookFactory_registeredAsDroppedSource pins case (f): the webhook adapter is a
// droppedCounter, so registerDroppedSources registers it (same assert telegram/discord
// use in metrics_wiring_test.go).
func TestWebhookFactory_registeredAsDroppedSource(t *testing.T) {
	t.Setenv(webhookInboundEnv, "the-inbound-secret")
	b := &builder{logger: slog.New(slog.DiscardHandler)}
	ch, err := defaultChannelFactory(b, webhookChannelCfg("127.0.0.1:0", "/hook", nil))
	if err != nil {
		t.Fatalf("factory error: %v", err)
	}

	reg := &fakeRegistrar{}
	registerDroppedSources(reg, []Channel{ch}, slog.New(slog.DiscardHandler))

	if _, ok := reg.got["webhook"]; !ok {
		t.Fatalf("webhook dropped source not registered; got keys %v", reg.got)
	}
}

// TestIsLoopbackBind pins case (g): the loopback decision helper. A non-loopback or
// unparseable bind returns false (and is therefore warned about; Listen will fail on
// its own later for a truly bad address).
func TestIsLoopbackBind(t *testing.T) {
	cases := []struct {
		bind string
		want bool
	}{
		{"127.0.0.1:8090", true},
		{"[::1]:1", true},
		{"localhost:9", true},
		{"0.0.0.0:1", false},
		{"192.168.1.5:80", false},
		{"not a bind", false},
	}
	for _, tc := range cases {
		if got := isLoopbackBind(tc.bind); got != tc.want {
			t.Errorf("isLoopbackBind(%q) = %v, want %v", tc.bind, got, tc.want)
		}
	}
}

// TestWebhookBuild_nonLoopbackWarns pins the boot warning (ADR-0038 §7): building a
// webhook with a non-loopback bind emits a WARN carrying the bind, so an operator
// learns the Bearer secret would cross the network in cleartext. Built only (never
// Started), so no non-loopback socket is opened — the "127.0.0.1:0 only" rule holds.
func TestWebhookBuild_nonLoopbackWarns(t *testing.T) {
	t.Setenv(webhookInboundEnv, "the-inbound-secret")
	logger, recs, mu := newCapturingLogger()

	app, err := Build(webhookCfg(webhookChannelCfg("0.0.0.0:0", "/hook", nil)), WithLogger(logger))
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	defer func() { _ = app.Shutdown(ctx) }()

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, r := range *recs {
		if r.Level < slog.LevelWarn {
			continue
		}
		if v, ok := attrString(r, "bind"); ok && v == "0.0.0.0:0" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no WARN record carrying the non-loopback bind %q was captured", "0.0.0.0:0")
	}
}

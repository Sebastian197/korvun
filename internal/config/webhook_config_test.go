// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
)

// This file is the SP1 TDD contract for the generic Webhook channel schema
// (ADR-0038 §1). It is written RED-first: it references the config surface the
// implementation must add — ChannelConfig.Webhook (*WebhookConfig), the nested
// WebhookConfig / WebhookMapping types, and the EffectiveMapping/EffectiveBind/
// EffectivePath accessors (the EffectiveRequestTimeout pattern) — none of which
// exist yet, so the config test package does not compile until config.go grows
// the contract. No production code changes ride with this file.

// webhookFull is a config with a fully-specified webhook channel: type "webhook"
// + channel-level token_env (the INBOUND secret, reusing token_env per NC-1a) +
// the complete nested webhook block (bind, path, outbound_url, outbound_token_env,
// and every mapping field overridden), routed to a brain.
const webhookFull = `{
  "channels": [
    {
      "type": "webhook",
      "token_env": "WEBHOOK_INBOUND_SECRET",
      "webhook": {
        "bind": "0.0.0.0:9000",
        "path": "/hook",
        "outbound_url": "https://downstream.example/in",
        "outbound_token_env": "WEBHOOK_OUTBOUND_TOKEN",
        "mapping": {
          "sender_id": "from",
          "sender_name": "name",
          "text": "body",
          "media_url": "url",
          "media_type": "mime",
          "conversation_id": "thread"
        }
      }
    }
  ],
  "brains": [
    {"name": "w", "sensitivity": "public", "policy": {"kind": "priority"},
     "models": [{"provider": "ollama", "model_id": "m", "locality": "local"}]}
  ],
  "routes": [{"channel": "webhook", "brain": "w"}]
}`

// webhookMinimalBlock is a valid webhook channel whose nested block carries ONLY
// the required outbound_url: bind/path/mapping absent (so their canonical defaults
// must resolve) and outbound_token_env absent (optional, NC-5). The block is
// present with its single required field, which is what type "webhook" requires
// (NC-1b + outbound_url REQUIRED per ADR-0038 §1).
const webhookMinimalBlock = `{
  "channels": [
    {"type": "webhook", "token_env": "WEBHOOK_INBOUND_SECRET",
     "webhook": {"outbound_url": "https://downstream.example/in"}}
  ],
  "brains": [
    {"name": "w", "sensitivity": "public", "policy": {"kind": "priority"},
     "models": [{"provider": "ollama", "model_id": "m", "locality": "local"}]}
  ],
  "routes": [{"channel": "webhook", "brain": "w"}]
}`

// TestWebhook_parseFull pins the happy parse (ADR-0038 §1): a complete webhook
// channel loads into a correctly-typed Config, with the nested WebhookConfig and
// its overridden WebhookMapping populated verbatim.
func TestWebhook_parseFull(t *testing.T) {
	t.Parallel()
	cfg, err := config.Load(writeConfig(t, webhookFull))
	if err != nil {
		t.Fatalf("valid webhook config rejected: %v", err)
	}
	if len(cfg.Channels) != 1 {
		t.Fatalf("channels len = %d, want 1", len(cfg.Channels))
	}
	ch := cfg.Channels[0]
	if ch.Type != "webhook" {
		t.Errorf("type = %q, want %q", ch.Type, "webhook")
	}
	if ch.TokenEnv != "WEBHOOK_INBOUND_SECRET" {
		t.Errorf("token_env = %q, want %q (inbound secret reuses token_env)", ch.TokenEnv, "WEBHOOK_INBOUND_SECRET")
	}
	if ch.Webhook == nil {
		t.Fatal("Webhook block is nil, want populated")
	}
	if ch.Webhook.Bind != "0.0.0.0:9000" {
		t.Errorf("bind = %q, want %q", ch.Webhook.Bind, "0.0.0.0:9000")
	}
	if ch.Webhook.Path != "/hook" {
		t.Errorf("path = %q, want %q", ch.Webhook.Path, "/hook")
	}
	if ch.Webhook.OutboundURL != "https://downstream.example/in" {
		t.Errorf("outbound_url = %q", ch.Webhook.OutboundURL)
	}
	if ch.Webhook.OutboundTokenEnv != "WEBHOOK_OUTBOUND_TOKEN" {
		t.Errorf("outbound_token_env = %q, want %q", ch.Webhook.OutboundTokenEnv, "WEBHOOK_OUTBOUND_TOKEN")
	}
	if ch.Webhook.Mapping == nil {
		t.Fatal("mapping is nil, want the overridden mapping")
	}
	m := ch.Webhook.EffectiveMapping()
	want := config.WebhookMapping{
		SenderID:       "from",
		SenderName:     "name",
		Text:           "body",
		MediaURL:       "url",
		MediaType:      "mime",
		ConversationID: "thread",
	}
	if m != want {
		t.Errorf("EffectiveMapping() = %+v, want %+v", m, want)
	}
	if got := ch.Webhook.EffectiveBind(); got != "0.0.0.0:9000" {
		t.Errorf("EffectiveBind() = %q, want %q", got, "0.0.0.0:9000")
	}
	if got := ch.Webhook.EffectivePath(); got != "/hook" {
		t.Errorf("EffectivePath() = %q, want %q", got, "/hook")
	}
}

// TestWebhook_defaultsResolve pins NC-1c/1d and the exact defaults (ADR-0038 §1):
// with an empty webhook block, bind → "127.0.0.1:8090", path → "/webhook", and
// EffectiveMapping() → the six canonical field names. outbound_token_env absent
// is valid (optional).
func TestWebhook_defaultsResolve(t *testing.T) {
	t.Parallel()
	cfg, err := config.Load(writeConfig(t, webhookMinimalBlock))
	if err != nil {
		t.Fatalf("valid minimal webhook config rejected: %v", err)
	}
	ch := cfg.Channels[0]
	if ch.Webhook == nil {
		t.Fatal("Webhook block is nil, want present-but-empty")
	}
	if ch.Webhook.OutboundTokenEnv != "" {
		t.Errorf("outbound_token_env = %q, want empty (optional, absent)", ch.Webhook.OutboundTokenEnv)
	}
	if got := ch.Webhook.EffectiveBind(); got != "127.0.0.1:8090" {
		t.Errorf("EffectiveBind() default = %q, want %q", got, "127.0.0.1:8090")
	}
	if got := ch.Webhook.EffectivePath(); got != "/webhook" {
		t.Errorf("EffectivePath() default = %q, want %q", got, "/webhook")
	}
	got := ch.Webhook.EffectiveMapping()
	want := config.WebhookMapping{
		SenderID:       "sender_id",
		SenderName:     "sender_name",
		Text:           "text",
		MediaURL:       "media_url",
		MediaType:      "media_type",
		ConversationID: "conversation_id",
	}
	if got != want {
		t.Errorf("EffectiveMapping() defaults = %+v, want the six canonical names %+v", got, want)
	}
}

// TestWebhook_partialMappingMerges pins the merge semantics of EffectiveMapping:
// a field the operator sets is kept; every empty field falls back to its canonical
// default. (Strengthens NC-1d beyond the all-absent case.)
func TestWebhook_partialMappingMerges(t *testing.T) {
	t.Parallel()
	js := `{
  "channels": [
    {"type": "webhook", "token_env": "S",
     "webhook": {"outbound_url": "https://downstream.example/in", "mapping": {"text": "message"}}}
  ],
  "brains": [
    {"name": "w", "sensitivity": "public", "policy": {"kind": "priority"},
     "models": [{"provider": "ollama", "model_id": "m", "locality": "local"}]}
  ],
  "routes": [{"channel": "webhook", "brain": "w"}]
}`
	cfg, err := config.Load(writeConfig(t, js))
	if err != nil {
		t.Fatalf("valid webhook config rejected: %v", err)
	}
	got := cfg.Channels[0].Webhook.EffectiveMapping()
	want := config.WebhookMapping{
		SenderID:       "sender_id",
		SenderName:     "sender_name",
		Text:           "message", // the override survives
		MediaURL:       "media_url",
		MediaType:      "media_type",
		ConversationID: "conversation_id",
	}
	if got != want {
		t.Errorf("EffectiveMapping() partial = %+v, want %+v", got, want)
	}
}

// TestWebhook_fieldErrors pins the ADR-0038 §1 validation contract: each row is a
// schema violation that must wrap ErrInvalidConfig and name its offending field
// path.
func TestWebhook_fieldErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		json      string
		wantField string
	}{
		{
			name:      "webhook type without a webhook block",
			json:      `{"channels":[{"type":"webhook","token_env":"S"}],"brains":[{"name":"w","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[{"channel":"webhook","brain":"w"}]}`,
			wantField: "channels[0].webhook",
		},
		{
			name:      "webhook block present under telegram type",
			json:      `{"channels":[{"type":"telegram","mode":"polling","token_env":"T","webhook":{"outbound_url":"https://x.example"}}],"brains":[{"name":"w","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[{"channel":"telegram","brain":"w"}]}`,
			wantField: "channels[0].webhook",
		},
		{
			name:      "non-empty mode on a webhook channel",
			json:      `{"channels":[{"type":"webhook","mode":"http","token_env":"S","webhook":{}}],"brains":[{"name":"w","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[{"channel":"webhook","brain":"w"}]}`,
			wantField: "channels[0].mode",
		},
		{
			name:      "empty token_env on a webhook channel",
			json:      `{"channels":[{"type":"webhook","token_env":"","webhook":{}}],"brains":[{"name":"w","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[{"channel":"webhook","brain":"w"}]}`,
			wantField: "channels[0].token_env",
		},
		{
			name:      "webhook block without the required outbound_url",
			json:      `{"channels":[{"type":"webhook","token_env":"S","webhook":{}}],"brains":[{"name":"w","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[{"channel":"webhook","brain":"w"}]}`,
			wantField: "channels[0].webhook.outbound_url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.Load(writeConfig(t, tt.json))
			if !errors.Is(err, config.ErrInvalidConfig) {
				t.Fatalf("err = %v, want ErrInvalidConfig", err)
			}
			if !strings.Contains(err.Error(), tt.wantField) {
				t.Errorf("error %q does not name the offending field %q", err.Error(), tt.wantField)
			}
		})
	}
}

// TestWebhook_regressionExistingChannelsUnchanged is the AS-1 guard: adding the
// webhook schema must not change how existing telegram-only or discord+telegram
// configs parse or validate. Both load clean, and a non-webhook channel carries a
// nil Webhook block (the new field is absent-by-default, zero behavior change).
func TestWebhook_regressionExistingChannelsUnchanged(t *testing.T) {
	t.Parallel()

	t.Run("telegram-only unchanged", func(t *testing.T) {
		t.Parallel()
		cfg, err := config.Load(writeConfig(t, validMinimal))
		if err != nil {
			t.Fatalf("telegram-only config rejected after webhook schema landed: %v", err)
		}
		if cfg.Channels[0].Webhook != nil {
			t.Errorf("telegram channel carries a non-nil Webhook block: %+v", cfg.Channels[0].Webhook)
		}
	})

	t.Run("discord+telegram unchanged", func(t *testing.T) {
		t.Parallel()
		js := `{
  "channels": [
    {"type": "telegram", "mode": "polling", "token_env": "TELEGRAM_BOT_TOKEN"},
    {"type": "discord", "mode": "gateway", "token_env": "DISCORD_BOT_TOKEN"}
  ],
  "brains": [
    {"name": "d", "sensitivity": "public", "policy": {"kind": "priority"},
     "models": [{"provider": "ollama", "model_id": "m", "locality": "local"}]}
  ],
  "routes": [
    {"channel": "telegram", "brain": "d"},
    {"channel": "discord", "brain": "d"}
  ]
}`
		cfg, err := config.Load(writeConfig(t, js))
		if err != nil {
			t.Fatalf("discord+telegram config rejected after webhook schema landed: %v", err)
		}
		for i, ch := range cfg.Channels {
			if ch.Webhook != nil {
				t.Errorf("channel[%d] (%s) carries a non-nil Webhook block", i, ch.Type)
			}
		}
	})
}

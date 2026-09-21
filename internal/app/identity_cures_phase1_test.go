// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
)

// TestIdentity_DuplicateChannelTypeStillBoots attacks the boot itself with a
// configuration the validator ACCEPTS and that the tree booted before this
// phase: two channels of the same type. Deriving one identity binding per
// channel rather than per channel TYPE produced two rows under the same id,
// which identity.NewResolver refuses by construction — so a running deployment
// would have stopped starting after the upgrade, silently, with no line of the
// paper declaring it.
//
// The demanded outcome is the one master already gave: the boot survives. The
// mould also pins WHY it survives — one binding and one issuer per type, the
// same fold the provenance registry and the metrics collector already make.
//
// Evidence level: in-process, through Build, which is the exact call the
// finding reproduced with.
// Probing mutation executed: remove the seenChannelType fold from
// phase1IdentityRegistry — Build returns «duplicate binding "binding_webhook"»
// and this mould reddens.
func TestIdentity_DuplicateChannelTypeStillBoots(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Channels: []config.ChannelConfig{
			{Type: "webhook", TokenEnv: "WEBHOOK_SECRET", Webhook: &config.WebhookConfig{
				Bind: "127.0.0.1:0", OutboundURL: "http://127.0.0.1:1/first",
			}},
			{Type: "webhook", TokenEnv: "WEBHOOK_SECRET_TWO", Webhook: &config.WebhookConfig{
				Bind: "127.0.0.1:0", OutboundURL: "http://127.0.0.1:1/second",
			}},
		},
		Brains: []config.BrainConfig{ollamaBrain()},
		Routes: []config.RouteConfig{{Channel: "webhook", Brain: "default"}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("the configuration this mould defends is not even valid: %v", err)
	}

	if _, _, _, err := phase1IdentityRuntime(cfg); err != nil {
		t.Fatalf("identity runtime refused a valid configuration: %v", err)
	}
	registry := phase1IdentityRegistry(cfg)
	webhookBindings := 0
	for _, binding := range registry.Bindings {
		if binding.Channel == "webhook" {
			webhookBindings++
		}
	}
	if webhookBindings != 1 {
		t.Fatalf("webhook bindings = %d, want exactly one shared by both channels", webhookBindings)
	}

	built, err := Build(cfg, withChannelFactory(okFactory(newFakeChannel("webhook"))))
	if err != nil {
		t.Fatalf("Build with two channels of one type = %v, want a running app", err)
	}
	if built == nil {
		t.Fatal("Build returned no app and no error")
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package shell

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/app"
	"github.com/Sebastian197/korvun/internal/config"
)

// envCheckingFactory mimics the REAL channel factory's secret pre-check
// (app.Build: os.Getenv(cc.TokenEnv) == "" -> ErrMissingSecret). The SP6c
// harness's fake factory skipped this, which is why the SP6c e2e never
// caught F1 — this factory closes that gap for the reload provisioning test.
func envCheckingFactory() app.Option {
	return app.WithChannelFactory(func(cc config.ChannelConfig) (app.Channel, error) {
		if cc.TokenEnv != "" && os.Getenv(cc.TokenEnv) == "" {
			return nil, fmt.Errorf("%w: %q", app.ErrMissingSecret, cc.TokenEnv)
		}
		return newFakeChannel(cc.Type), nil
	})
}

// templateCfg is the channel-less first-run shape: zero channels, one ollama
// brain, the admin block. It BOOTS (no channel secret needed) and is the
// exact starting point the hardware F1 came from.
func templateCfg(ollamaURL string) *config.Config {
	return &config.Config{
		Channels: []config.ChannelConfig{},
		Brains: []config.BrainConfig{{
			Name:        "asistente",
			Sensitivity: "public",
			Policy:      config.PolicyConfig{Kind: "priority", Order: []string{"ollama"}},
			Models: []config.ModelConfig{
				{Provider: "ollama", ModelID: "llama3.2", Locality: "local", BaseURL: ollamaURL},
			},
		}},
		Routes:        []config.RouteConfig{},
		Admin:         &config.AdminConfig{TokenEnv: adminTokenEnv},
		Observability: &config.ObservabilityConfig{Addr: "127.0.0.1:0"},
	}
}

// F1 (hardware pass, 2026-07-25): a hot reload that adds a channel whose
// secret lives ONLY in the OS keychain (never exported to the environment)
// must re-provision that secret from the keychain in the reload's build seam
// — not only at Start. Before the fix, the reload's preflight built the new
// channel via the env-checking factory, found the var absent, and rolled
// back to "failed". This reproduces the EXACT scenario: core running with the
// 0-channel template + the discord token only in the keychain double + the
// assistant's POST -> the channel must connect.
func TestReload_reprovisionsKeychainSecret(t *testing.T) {
	t.Setenv(adminTokenEnv, "")   // the shell mints the bearer per cycle
	t.Setenv(discordTokenEnv, "") // the secret is NOT in the environment...
	store := newFakeStore()
	store.data[discordTokenEnv] = "the-discord-bot-token" // ...it lives ONLY in the keychain

	c := New(
		WithLogger(slog.New(slog.DiscardHandler)),
		WithSecretStore(store),
		WithBuildOptions(envCheckingFactory()),
	)
	ollama := fakeOllama(t)
	path := writeCfg(t, templateCfg(ollama.URL))
	if err := c.LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start (channel-less template): %v", err)
	}
	t.Cleanup(func() {
		if c.Status().Running {
			sctx, sc := context.WithTimeout(context.Background(), 10*time.Second)
			defer sc()
			_ = c.Stop(sctx)
		}
	})

	srv := httptest.NewServer(c.ProxyHandler())
	defer srv.Close()

	// The assistant adds the discord channel whose secret is keychain-only.
	withChannel := templateCfg(ollama.URL)
	withChannel.Channels = append(withChannel.Channels, config.ChannelConfig{
		Type: "discord", Mode: "gateway", TokenEnv: discordTokenEnv,
	})
	withChannel.Routes = append(withChannel.Routes, config.RouteConfig{Channel: "discord", Brain: "asistente"})
	body, err := json.Marshal(withChannel)
	if err != nil {
		t.Fatalf("marshal channel config: %v", err)
	}

	postConfigReload(t, srv.Client(), srv.URL, body) // fails here before the fix (rollback)

	_, chans := get(t, srv.Client(), srv.URL+"/api/channels", nil)
	if !strings.Contains(chans, `"discord"`) {
		t.Fatalf("after the reload /api/channels = %q, want the discord channel", chans)
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R5 of the third Codex pass (adjudicated 2026-09-01): ONE resolver of
// the effective cage feeds BOTH the law digest and the deferred
// executor. The pin digests EFFECTIVE conduct — same conduct, same
// law: an explicit cage value equal to its default pins identically to
// the absent value, and cage-irrelevant fields (system_prompt,
// skills_dir, max_iterations) stay OUT of the digest (exclusion
// declared in the spec). The deferred executor resolves the SAME cage
// the boot resolves — a config the boot refuses can never build an
// executor, and per-tool overrides ride into the rebuilt jail.
// Reproduction-first contract.

package app

import (
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/tool"
)

func webhookLawCfg(t *testing.T, maxBytes int64) *config.Config {
	t.Helper()
	cfg := lawBaseCfg(t, "r5")
	cfg.Brains[0].Agent = &config.AgentConfig{
		Tools:       []string{"webhook_call"},
		WebhookCall: &config.WebhookCallToolConfig{AllowHosts: []string{"hooks.example.com"}, MaxBytes: maxBytes},
	}
	return cfg
}

func TestPolicyPin_sameConductIsTheSameLaw(t *testing.T) {
	t.Parallel()
	absent, err := PolicyPinFor(webhookLawCfg(t, 0), "a")
	if err != nil {
		t.Fatalf("pin absent: %v", err)
	}
	explicit, err := PolicyPinFor(webhookLawCfg(t, tool.DefaultWebhookMaxBytes), "a")
	if err != nil {
		t.Fatalf("pin explicit: %v", err)
	}
	if absent.Digest != explicit.Digest {
		t.Fatal("AUDIT R5: max_bytes 0 and the explicit default are the SAME conduct — the SAME law")
	}
	tighter, err := PolicyPinFor(webhookLawCfg(t, 1024), "a")
	if err != nil {
		t.Fatalf("pin tighter: %v", err)
	}
	if tighter.Digest == absent.Digest {
		t.Fatal("a genuinely different cage bound IS a different law")
	}
}

func TestPolicyPin_cageIrrelevantFieldsStayOut(t *testing.T) {
	t.Parallel()
	base := lawBaseCfg(t, "r5b")
	pin0, err := PolicyPinFor(base, "a")
	if err != nil {
		t.Fatalf("pin: %v", err)
	}
	for i, mutate := range []func(*config.Config){
		func(c *config.Config) { c.Brains[0].Agent.SystemPrompt = "another persona entirely" },
		func(c *config.Config) { c.Brains[0].Agent.SkillsDir = "/somewhere/else" },
		func(c *config.Config) { c.Brains[0].Agent.MaxIterations = 9 },
	} {
		cfg := lawBaseCfg(t, "r5b")
		mutate(cfg)
		pin, err := PolicyPinFor(cfg, "a")
		if err != nil {
			t.Fatalf("mutation %d: %v", i, err)
		}
		if pin.Digest != pin0.Digest {
			t.Fatalf("AUDIT R5: mutation %d does not govern the cage and must NOT move the law", i)
		}
	}
}

// The single-resolver pin on the executor half: a config whose attrs
// the BOOT refuses (an override naming an unlisted tool) can never
// build a deferred executor either — one resolver, one verdict.
func TestBuildApprovalExecutor_resolvesTheSameCageAsTheBoot(t *testing.T) {
	t.Parallel()
	cfg := lawBaseCfg(t, "r5c")
	cfg.Brains[0].Agent.ToolAttrs = map[string]config.ToolAttrsConfig{
		"ghost": {},
	}
	_, err := BuildApprovalExecutor(cfg, action.ActionPreview{
		PrincipalID: "principal_brain_" + cfg.Brains[0].Name,
		Operation:   "tool/calc",
	})
	if err == nil {
		t.Fatal("AUDIT R5: a config the boot refuses must never build an executor")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("the refusal must name the offending override: %v", err)
	}
}

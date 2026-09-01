// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// C1 of the E5 consolidation (second external audit): policy identity
// must be STABLE (a digest of the effective content — tools, cages,
// allowlists, attrs, sensitivity, ceiling, effect registry — never the
// load instant, which made every reboot "a different law"), and the
// pin must be VALIDATED on the production decide/execute path — the
// domain validator existed but nothing wired it (acceptance theater,
// purged). A tool revoked from config NEVER executes.
// Reproduction-first contract.

package app

import (
	"path/filepath"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
)

// lawBaseCfg is the kernel config with a real agent block — the law
// covers the grant list and its cages, so the base must carry one.
func lawBaseCfg(t *testing.T, name string) *config.Config {
	t.Helper()
	cfg := kernelWiringConfig(filepath.Join(t.TempDir(), name+".db"))
	cfg.Brains[0].Agent = &config.AgentConfig{Tools: []string{"calc"}}
	return cfg
}

func twoBrainCfgs(t *testing.T) (*config.Config, *config.Config) {
	t.Helper()
	return lawBaseCfg(t, "a"), lawBaseCfg(t, "b")
}

func TestPolicyPin_stableAcrossBoots(t *testing.T) {
	t.Parallel()
	cfgA, cfgB := twoBrainCfgs(t)
	pinA, err := PolicyPinFor(cfgA, cfgA.Brains[0].Name)
	if err != nil {
		t.Fatalf("pin A: %v", err)
	}
	pinB, err := PolicyPinFor(cfgB, cfgB.Brains[0].Name)
	if err != nil {
		t.Fatalf("pin B: %v", err)
	}
	// Same effective content => the SAME law, whatever the boot instant.
	if pinA.Digest != pinB.Digest || pinA.Version != pinB.Version {
		t.Fatalf("the audit's finding: identical configs must pin the identical law: %+v vs %+v", pinA, pinB)
	}
}

func TestPolicyPin_coversWhatGovernsTheCage(t *testing.T) {
	t.Parallel()
	base := lawBaseCfg(t, "k")
	pin0, err := PolicyPinFor(base, base.Brains[0].Name)
	if err != nil {
		t.Fatalf("pin: %v", err)
	}
	mutations := []func(*config.Config){
		func(c *config.Config) { c.Brains[0].Agent.Tools = append(c.Brains[0].Agent.Tools, "time") },
		func(c *config.Config) { c.Brains[0].Sensitivity = "private" },
		func(c *config.Config) { c.Brains[0].Agent.EffectCeiling = "read_external" },
		func(c *config.Config) {
			c.Brains[0].Agent.WebhookCall = &config.WebhookCallToolConfig{AllowHosts: []string{"evil.example"}}
		},
		func(c *config.Config) {
			// R5 re-map: an EMPTY override is the same conduct (same
			// law); a REAL override moves the effective cage.
			c.Brains[0].Agent.ToolAttrs = map[string]config.ToolAttrsConfig{"calc": {Network: boolPtr(true)}}
		},
	}
	for i, mutate := range mutations {
		cfg := lawBaseCfg(t, "m")
		mutate(cfg)
		pin, err := PolicyPinFor(cfg, cfg.Brains[0].Name)
		if err != nil {
			t.Fatalf("mutation %d: %v", i, err)
		}
		if pin.Digest == pin0.Digest {
			t.Fatalf("mutation %d must change the law digest (the cage-governing content is IN the identity)", i)
		}
	}
	if _, err := PolicyPinFor(base, "ghost"); err == nil {
		t.Fatal("an unknown brain must refuse by name")
	}
}

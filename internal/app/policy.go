// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The policy pin (Trust Layer E3 FR-POL-1, rebuilt by the E5
// consolidation C1): every gate decision records the EXACT law that
// took it, and that identity must be STABLE — a digest of the
// EFFECTIVE content that governs the brain's cage (grant list, cages
// and allowlists, tool attrs, sensitivity, effect ceiling, effect
// registry). The first cut versioned the pin with the config-load
// instant, which made every reboot "a different law" and left the
// validator unwired — the second external audit's finding. The
// version is now the pin FORMAT (bump only when the digested content
// set changes shape); sameness of law is sameness of digest.
package app

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/tool"
)

// policyPinFormat identifies the SHAPE of the digested content, not an
// instant: two loads of the same effective config are the SAME law.
const policyPinFormat = 3

// effectiveCage resolves the ONE effective cage of a brain (R5): the
// same resolver feeds the law digest below AND BuildApprovalExecutor —
// one function, one verdict, so the deferred execution can never run a
// different jail than the one the law was pinned over. EFFECTIVE means
// conduct, not spelling: per-tool attrs come from effectiveToolAttrs
// (house defaults + operator overrides), cage bounds normalize their
// declared defaults (max_bytes 0 IS the default bound — same conduct,
// same law), and allow-lists are set-semantic (sorted). Fields that do
// not govern the cage — system_prompt, skills_dir, skills_body_budget,
// max_iterations — stay OUT (exclusion declared in the E5 spec).
// Governance keeps its declared order: order IS part of the law.
func effectiveCage(bc config.BrainConfig) (map[string]any, error) {
	cage := map[string]any{"sensitivity": bc.Sensitivity}
	a := bc.Agent
	if a == nil {
		return cage, nil
	}
	attrs, err := effectiveToolAttrs(a)
	if err != nil {
		return nil, err
	}
	cage["tools"] = a.Tools
	cage["governance"] = a.Governance
	cage["attrs"] = attrs
	cage["effect_ceiling"] = a.EffectCeiling
	if a.ReadFile != nil {
		cage["read_file"] = map[string]any{
			"root":      a.ReadFile.Root,
			"max_bytes": defaultedInt64(a.ReadFile.MaxBytes, tool.DefaultReadFileMaxBytes),
		}
	}
	if a.HTTPFetch != nil {
		cage["http_fetch"] = map[string]any{
			"allow_hosts":   sortedCopy(a.HTTPFetch.AllowHosts),
			"max_bytes":     defaultedInt64(a.HTTPFetch.MaxBytes, tool.DefaultFetchMaxBytes),
			"max_redirects": defaultedInt(a.HTTPFetch.MaxRedirects, tool.DefaultFetchMaxRedirects),
		}
	}
	if a.WebhookCall != nil {
		cage["webhook_call"] = map[string]any{
			"allow_hosts":     sortedCopy(a.WebhookCall.AllowHosts),
			"max_bytes":       defaultedInt64(a.WebhookCall.MaxBytes, tool.DefaultWebhookMaxBytes),
			"timeout_seconds": defaultedInt(a.WebhookCall.TimeoutSeconds, int(tool.DefaultWebhookTimeout/time.Second)),
		}
	}
	if a.Memory != nil {
		cage["memory"] = a.Memory.Settings()
	}
	return cage, nil
}

func defaultedInt64(v, def int64) int64 {
	if v == 0 {
		return def
	}
	return v
}

func defaultedInt(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// policyDigestFor computes the canonical law digest over the EFFECTIVE
// cage (R5) plus the effect-registry snapshot. Deterministic: maps
// sort by key under encoding/json.
func policyDigestFor(bc config.BrainConfig, effects map[string]action.EffectDescriptor) (string, error) {
	cage, err := effectiveCage(bc)
	if err != nil {
		return "", err
	}
	cage["effects"] = effects
	raw, err := json.Marshal(cage)
	if err != nil {
		// Unreachable for these plain types; kept for honesty.
		return "", fmt.Errorf("app: policy digest: %w", err)
	}
	return action.HashCanonical(string(raw)), nil
}

// effectSnapshot captures the declared registry for the digest: every
// safe-toolset name with its descriptor, deterministically.
func effectSnapshot() map[string]action.EffectDescriptor {
	names := []string{"calc", "echo", "http_fetch", "memory_note", "read_file", "time", "webhook_call"}
	sort.Strings(names)
	out := make(map[string]action.EffectDescriptor, len(names))
	for _, name := range names {
		if descriptor, ok := tool.BuiltinEffects(name); ok {
			out[name] = descriptor
		}
	}
	return out
}

// policyPin builds one brain's stable law pin.
func policyPin(bc config.BrainConfig) (actionsqlite.PolicyPin, error) {
	digest, err := policyDigestFor(bc, effectSnapshot())
	if err != nil {
		return actionsqlite.PolicyPin{}, err
	}
	return actionsqlite.PolicyPin{Version: policyPinFormat, Digest: digest}, nil
}

// PolicyPinFor computes the CURRENT law pin for one brain of a loaded
// config — the operator-side entry the C1 consolidation wires into
// decide and execute: the approval's pinned law must still BE the law.
func PolicyPinFor(cfg *config.Config, brainName string) (actionsqlite.PolicyPin, error) {
	for _, bc := range cfg.Brains {
		if bc.Name == brainName {
			return policyPin(bc)
		}
	}
	return actionsqlite.PolicyPin{}, fmt.Errorf("app: policy pin: brain %q is not in the current config", brainName)
}

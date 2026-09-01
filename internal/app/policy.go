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

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/tool"
)

// policyPinFormat identifies the SHAPE of the digested content, not an
// instant: two loads of the same effective config are the SAME law.
const policyPinFormat = 2

// policyDigestFor computes the canonical law digest over everything
// that governs one brain's cage. The whole agent block rides in (grant
// list in declared order — order IS part of the operator's config —
// governance, attrs, per-tool cages and allowlists, ceiling), plus the
// brain's sensitivity and the effect-registry snapshot. Deterministic:
// struct fields marshal in declared order, maps sort by key.
func policyDigestFor(bc config.BrainConfig, effects map[string]action.EffectDescriptor) string {
	raw, err := json.Marshal(map[string]any{
		"agent":       bc.Agent,
		"sensitivity": bc.Sensitivity,
		"effects":     effects,
	})
	if err != nil {
		// Unreachable for these plain types; kept for honesty.
		return "sha256:unmarshalable"
	}
	return action.HashCanonical(string(raw))
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
func policyPin(bc config.BrainConfig) actionsqlite.PolicyPin {
	return actionsqlite.PolicyPin{
		Version: policyPinFormat,
		Digest:  policyDigestFor(bc, effectSnapshot()),
	}
}

// PolicyPinFor computes the CURRENT law pin for one brain of a loaded
// config — the operator-side entry the C1 consolidation wires into
// decide and execute: the approval's pinned law must still BE the law.
func PolicyPinFor(cfg *config.Config, brainName string) (actionsqlite.PolicyPin, error) {
	for _, bc := range cfg.Brains {
		if bc.Name == brainName {
			return policyPin(bc), nil
		}
	}
	return actionsqlite.PolicyPin{}, fmt.Errorf("app: policy pin: brain %q is not in the current config", brainName)
}

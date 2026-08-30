// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The versioned policy pin (Trust Layer Etapa 3, lote 4, spec FR-POL-1):
// every gate decision records the EXACT law that took it. The law is the
// brain's effective governance PLUS the effect-registry snapshot — a
// change in either is a different law. The version is the config load
// instant (UnixNano): monotonic across loads with zero mutable globals.
package app

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/tool"
)

// policyDigestFrom computes the canonical law digest over one brain's
// governance and an effect-registry snapshot. Deterministic: governance
// rides in its declared order (order IS part of the operator's config),
// the snapshot as a sorted map via the canonical hasher.
func policyDigestFrom(gov []config.ToolGrantConfig, effects map[string]action.EffectDescriptor) string {
	raw, err := json.Marshal(map[string]any{
		"governance": gov,
		"effects":    effects,
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

// policyPin builds one brain's law pin at config-load time.
func policyPin(bc config.BrainConfig, loadedAt time.Time) actionsqlite.PolicyPin {
	var gov []config.ToolGrantConfig
	if bc.Agent != nil {
		gov = bc.Agent.Governance
	}
	return actionsqlite.PolicyPin{
		Version: loadedAt.UnixNano(),
		Digest:  policyDigestFrom(gov, effectSnapshot()),
	}
}

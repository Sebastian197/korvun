// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The validated effect registry (Trust Layer Etapa 3, lote 1, spec
// FR-REG, blueprint §9.7): every builtin DECLARES its Effect Descriptor
// here, beside BuiltinAttrs — the same single safe-toolset boundary. The
// Effect Engine classifies ONLY from these declarations, never from
// model text and never from parameters. Data sensitivity stays in Attrs:
// the two dimensions compose, never merge (§10.6 closing law).
package tool

import "github.com/Sebastian197/korvun/internal/action"

// BuiltinEffects returns the declared effect descriptor of a built-in
// tool by its protocol name, and ok=false for any name outside the safe
// toolset — a forgotten declaration is exactly what the registry
// tripwire test guards against. The semantics are the SEALED §7.10
// honest calls of the stage-3 spec:
//
//   - time/echo/calc: pure local computation.
//   - memory_note: write_reversible (sealed NC-1: a note that persists
//     IS a write — the ladder measures consequence, not network
//     distance; documented undo via /notes clear). Local state:
//     WritesExternalState stays false.
//   - read_file: read_external with data egress — what is read enters a
//     model prompt.
//   - http_fetch: read_external with data egress — the request travels
//     to an external host.
//   - webhook_call: write_irreversible — an arbitrary downstream POST
//     has no documented undo and no known compensation; honesty over
//     optimism (revisit per-connector in E7).
func BuiltinEffects(name string) (action.EffectDescriptor, bool) {
	switch name {
	case "time", "echo", "calc":
		return action.EffectDescriptor{Class: action.EffectPure}, true
	case "memory_note":
		return action.EffectDescriptor{
			Class:      action.EffectWriteReversible,
			Reversible: true,
		}, true
	case "read_file":
		return action.EffectDescriptor{
			Class:              action.EffectReadExternal,
			ReadsExternalState: true,
			DataEgress:         true,
		}, true
	case "http_fetch":
		return action.EffectDescriptor{
			Class:              action.EffectReadExternal,
			ReadsExternalState: true,
			DataEgress:         true,
		}, true
	case "webhook_call":
		return action.EffectDescriptor{
			Class:               action.EffectWriteIrreversible,
			WritesExternalState: true,
			DataEgress:          true,
		}, true
	default:
		return action.EffectDescriptor{}, false
	}
}

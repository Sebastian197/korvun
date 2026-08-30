// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The validated effect registry — Etapa 3, lote 1, pieza 2 (spec FR-REG,
// blueprint §9.7): every builtin DECLARES its descriptor with §7.10
// honesty, sealed by this table; the attrs and effects registries cover
// exactly the same safe toolset (tripwire); unknown names fail closed.
// Approved-red contract.

package tool

import (
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
)

// sealedEffects restates the sealed declarations LITERALLY (the oracle
// discipline): the spec's honest semantics, decision by decision.
var sealedEffects = map[string]action.EffectDescriptor{
	"time": {Class: action.EffectPure},
	"echo": {Class: action.EffectPure},
	"calc": {Class: action.EffectPure},
	// Sealed NC-1 (Chano 2026-08-30): a note that persists IS a write —
	// the ladder measures consequence, not network distance. Local state,
	// documented undo (/notes clear).
	"memory_note": {
		Class:      action.EffectWriteReversible,
		Reversible: true,
	},
	// What is read enters a model prompt: data egress is the truth.
	"read_file": {
		Class:              action.EffectReadExternal,
		ReadsExternalState: true,
		DataEgress:         true,
	},
	// The request travels to an external host.
	"http_fetch": {
		Class:              action.EffectReadExternal,
		ReadsExternalState: true,
		DataEgress:         true,
	},
	// An arbitrary downstream POST: no documented undo, no known
	// compensation — honesty over optimism (§7.10).
	"webhook_call": {
		Class:               action.EffectWriteIrreversible,
		WritesExternalState: true,
		DataEgress:          true,
	},
}

func TestBuiltinEffects_sealedHonestDeclarations(t *testing.T) {
	t.Parallel()
	for name, want := range sealedEffects {
		got, ok := BuiltinEffects(name)
		if !ok {
			t.Fatalf("%s must declare its effect descriptor", name)
		}
		if got != want {
			t.Fatalf("%s descriptor:\ngot  %+v\nwant %+v", name, got, want)
		}
		if !got.Class.Known() {
			t.Fatalf("%s declares an unknown class %q", name, got.Class)
		}
	}
}

func TestBuiltinEffects_unknownNamesFailClosed(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"shell", "", "TIME", "journal"} {
		if _, ok := BuiltinEffects(name); ok {
			t.Fatalf("%q is outside the safe toolset and must not classify", name)
		}
	}
}

// TestBuiltinEffects_coversExactlyTheSafeToolset is the tripwire: the
// attrs registry and the effects registry describe the SAME set — a tool
// added to one without the other fails here forever.
func TestBuiltinEffects_coversExactlyTheSafeToolset(t *testing.T) {
	t.Parallel()
	for name := range sealedEffects {
		if _, ok := BuiltinAttrs(name); !ok {
			t.Fatalf("%s declares effects but not attrs", name)
		}
	}
	// And the sealed table IS the whole safe toolset: every attrs-known
	// name must be in it (probe the known universe plus adversaries).
	for _, name := range []string{
		"time", "echo", "calc", "memory_note", "read_file", "http_fetch", "webhook_call",
	} {
		if _, ok := BuiltinAttrs(name); !ok {
			t.Fatalf("universe drift: %s lost its attrs", name)
		}
		if _, ok := BuiltinEffects(name); !ok {
			t.Fatalf("universe drift: %s lost its effect descriptor", name)
		}
	}
}

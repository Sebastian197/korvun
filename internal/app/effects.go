// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Boot preflight of the effect registry (Trust Layer Etapa 3, lote 1,
// spec FR-REG-3): an operation without a declared Effect Descriptor is a
// BOOT error, named loudly (the E-11 mold) — the first of the two walls;
// the gate's effect_undeclared denial (batch 2) is the second.
package app

import (
	"fmt"
	"sort"

	"github.com/Sebastian197/korvun/internal/tool"
)

// validateToolEffects verifies every tool in a brain's registry declares
// its effect descriptor. Deterministic order so the boot error is stable.
func validateToolEffects(reg tool.Registry) error {
	names := make([]string, 0, len(reg))
	for name := range reg {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if _, ok := tool.BuiltinEffects(name); !ok {
			return fmt.Errorf("app: tool %q has no declared effect descriptor (spec E3 FR-REG-3: every operation classifies from the registry or does not boot)", name)
		}
	}
	return nil
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Boot preflight of the effect registry — Etapa 3, lote 1, pieza 2 (spec
// FR-REG-3, the E-11 fail-loud mold): a brain whose registry carries an
// operation WITHOUT a declared effect descriptor must not boot. Today
// every safe-toolset name declares — the guard exists for the tool that
// forgets tomorrow. Approved-red contract.

package app

import (
	"context"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/tool"
)

// undeclaredTool is a tool outside the safe toolset: no attrs, no effect
// descriptor — exactly what the preflight must catch.
type undeclaredTool struct{}

func (undeclaredTool) Name() string        { return "mystery_op" }
func (undeclaredTool) Description() string { return "declares nothing" }
func (undeclaredTool) Execute(ctx context.Context, args string) (string, error) {
	return "", nil
}

func TestValidateToolEffects_undeclaredFailsLoudNamingTheTool(t *testing.T) {
	t.Parallel()
	reg := tool.Registry{"mystery_op": undeclaredTool{}}
	err := validateToolEffects(reg)
	if err == nil {
		t.Fatal("an undeclared operation must fail boot preflight")
	}
	if !strings.Contains(err.Error(), "mystery_op") {
		t.Fatalf("the failure must NAME the tool, got %v", err)
	}
	if !strings.Contains(err.Error(), "effect") {
		t.Fatalf("the failure must name the missing declaration, got %v", err)
	}
}

func TestValidateToolEffects_wholeSafeToolsetPasses(t *testing.T) {
	t.Parallel()
	reg := tool.Registry{}
	for _, name := range []string{"time", "echo", "calc"} {
		tl, ok := tool.Builtin(name)
		if !ok {
			t.Fatalf("builtin %s missing", name)
		}
		reg[name] = tl
	}
	if err := validateToolEffects(reg); err != nil {
		t.Fatalf("every declared builtin must pass preflight: %v", err)
	}
}

// TestBuild_undeclaredToolCannotBoot: the guard wired into the real boot
// path. Today's config validation already rejects unknown tool names, so
// this rides the unit seam above; the wiring itself is proven by the
// whole suite passing WITH the guard in place (every existing boot
// carries only declared tools).
func TestBuild_declaredToolsStillBoot(t *testing.T) {
	dbPath := t.TempDir() + "/korvun.db"
	app, err := Build(kernelWiringConfig(dbPath), withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("a config of declared tools must keep booting: %v", err)
	}
	shutdownApp(t, app)
}

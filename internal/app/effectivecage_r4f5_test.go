// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R4 Phase 5 (FR-R4F5): the effective cage becomes a TYPE, resolved
// ONCE, feeding the law digest AND every tool construction — boot and
// deferred executor alike. The house amendment pins digest stability
// byte-for-byte across the shape change against a GOLDEN value. The
// auditor's five conduct pairs ride as permanent tests.
// Reproduction-first contract.

package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/tool"
)

// goldenLawDigest was captured on the PRE-refactor shape (2026-09-02,
// pin format 3). The typed resolver must reproduce it byte-for-byte —
// or bump policyPinFormat with the reason written (the godoc's rule).
const goldenLawDigest = "sha256:aedb7f79ab17c86b6e74a305e146464019b39314cc0252aa95d75c57f6c1df2a"

func goldenCfg() *config.Config {
	return &config.Config{Brains: []config.BrainConfig{{
		Name: "g", Sensitivity: "private",
		Agent: &config.AgentConfig{
			Tools:         []string{"calc", "webhook_call", "read_file"},
			Governance:    []config.ToolGrantConfig{{Tool: "webhook_call", Mode: "allow"}},
			ToolAttrs:     map[string]config.ToolAttrsConfig{"calc": {Network: boolPtr(true)}},
			ReadFile:      &config.ReadFileToolConfig{Root: "/jail"},
			WebhookCall:   &config.WebhookCallToolConfig{AllowHosts: []string{"b.example", "a.example"}, MaxBytes: 1024},
			EffectCeiling: "write_reversible",
		},
	}}}
}

func TestPolicyPin_goldenDigestSurvivesTheShapeChange(t *testing.T) {
	t.Parallel()
	pin, err := PolicyPinFor(goldenCfg(), "g")
	if err != nil {
		t.Fatalf("pin: %v", err)
	}
	if pin.Version != 3 || pin.Digest != goldenLawDigest {
		t.Fatalf("HOUSE AMENDMENT: the typed resolver must reproduce the golden law byte-for-byte (or bump with a written reason): got v%d %s", pin.Version, pin.Digest)
	}
}

// Pair 1 + pair 4: same conduct is the same OBJECT — an explicit cage
// value equal to its default resolves DeepEqual to the absent one
// (max_bytes, timeouts, redirects), and allow-lists resolve sorted.
func TestResolveEffectiveCage_sameConductSameObject(t *testing.T) {
	t.Parallel()
	base := goldenCfg().Brains[0]
	explicit := goldenCfg().Brains[0]
	explicit.Agent.WebhookCall.TimeoutSeconds = int(tool.DefaultWebhookTimeout.Seconds())
	explicit.Agent.ReadFile.MaxBytes = tool.DefaultReadFileMaxBytes
	cageA, err := ResolveEffectiveCage(base)
	if err != nil {
		t.Fatalf("resolve base: %v", err)
	}
	cageB, err := ResolveEffectiveCage(explicit)
	if err != nil {
		t.Fatalf("resolve explicit: %v", err)
	}
	if !reflect.DeepEqual(cageA, cageB) {
		t.Fatalf("AUDIT R4-F5: same conduct must be the SAME OBJECT:\n%+v\nvs\n%+v", cageA, cageB)
	}
	if cageA.WebhookCall == nil || cageA.WebhookCall.AllowHosts[0] != "a.example" {
		t.Fatalf("allow-lists resolve SORTED: %+v", cageA.WebhookCall)
	}
	if cageA.WebhookCall.TimeoutSeconds != int(tool.DefaultWebhookTimeout.Seconds()) {
		t.Fatalf("timeout defaults resolve in the object: %+v", cageA.WebhookCall)
	}
	if cageA.ReadFile == nil || cageA.ReadFile.MaxBytes != tool.DefaultReadFileMaxBytes {
		t.Fatalf("max_bytes defaults resolve in the object: %+v", cageA.ReadFile)
	}
}

// Pairs 2+3 (the attrs half; the live-vs-deferred identity is
// structural — one resolver feeds both constructions): the operator's
// network override lands in the resolved attrs the shield consumes.
func TestResolveEffectiveCage_overridesLandInTheResolvedAttrs(t *testing.T) {
	t.Parallel()
	bc := goldenCfg().Brains[0]
	no := false
	bc.Agent.ToolAttrs["webhook_call"] = config.ToolAttrsConfig{Network: &no}
	cage, err := ResolveEffectiveCage(bc)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if cage.Attrs["webhook_call"].Network {
		t.Fatal("AUDIT R4-F5: network=false must land in the resolved attrs (live AND deferred consume THIS map)")
	}
	if !cage.Attrs["calc"].Network {
		t.Fatal("the calc override (network=true) resolves too")
	}
}

// Pair 5: a config the boot refuses is refused by the resolver — and
// therefore by BOTH constructions (they share it).
func TestResolveEffectiveCage_refusesWhatTheBootRefuses(t *testing.T) {
	t.Parallel()
	bc := goldenCfg().Brains[0]
	bc.Agent.ToolAttrs["ghost"] = config.ToolAttrsConfig{}
	_, err := ResolveEffectiveCage(bc)
	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("one resolver, one verdict: %v", err)
	}
}

// R5-S3/R6-X3: the single-object invariant, STRUCTURAL and judged by
// the AST (the text grep died — alias, parenthesized calls or an
// argument named like the function cannot bribe a parser). Production
// code may resolve the cage ONLY inside the allow-listed functions; a
// resolution appearing anywhere else forces adjudication.
func TestCageResolutionGuard_exactlyTheAllowedEntryPoints(t *testing.T) {
	t.Parallel()
	allowed := map[string]bool{
		"buildAgentBrain":    true, // the boot: one resolution feeds registry+pin+ceiling
		"policyDigestFor":    true, // PolicyPinFor's own single resolution (operator CLI)
		"ResolveApprovalLaw": true, // the operator execute/approve path (R6-X3)
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	found := map[string][]string{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if ok && fn.Body != nil {
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "ResolveEffectiveCage" {
						found[fn.Name.Name] = append(found[fn.Name.Name], fset.Position(call.Pos()).String())
					}
					return true
				})
			}
		}
	}
	for fnName, positions := range found {
		if !allowed[fnName] {
			t.Fatalf("R6-X3 GUARD: %s resolves the cage at %v — a new resolution point needs adjudication", fnName, positions)
		}
	}
	if len(found) != len(allowed) {
		t.Fatalf("R6-X3 GUARD: expected resolutions in exactly %d allowed functions, found %d: %v", len(allowed), len(found), found)
	}
}

// R5-S3: defensive copies — mutating the config after resolution must
// not reach into the resolved object (the aliasing died).
func TestResolveEffectiveCage_defensiveCopies(t *testing.T) {
	t.Parallel()
	bc := goldenCfg().Brains[0]
	bc.Agent.Governance[0].Channels = []string{"console"}
	cage, err := ResolveEffectiveCage(bc)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	bc.Agent.Tools[0] = "mutated"
	bc.Agent.Governance[0].Tool = "mutated"
	if cage.Tools[0] == "mutated" || cage.Governance[0].Tool == "mutated" {
		t.Fatal("R5-S3: the resolved object must not alias the config slices")
	}
	// R6-X3: the DEEP copy — inner Channels slices included.
	if len(bc.Agent.Governance[0].Channels) > 0 {
		bc.Agent.Governance[0].Channels[0] = "mutated"
		if cage.Governance[0].Channels[0] == "mutated" {
			t.Fatal("R6-X3: Governance.Channels must be deep-copied")
		}
	}
}

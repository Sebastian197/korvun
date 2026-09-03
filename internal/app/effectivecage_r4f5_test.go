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
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
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

// R5-S3/R6-X3/R7-Y4/R8-Z4: the single-object invariant, judged by
// the AST over the WHOLE tree (internal/..., cmd/..., web/... — the
// adjudicated line) with the allow-list keyed by SITE — exact package
// directory + top-level function — never by textual name: a briber
// method named policyDigestFor in another package, or a cross-package
// reference (pkg.ResolveEffectiveCage as Ident OR SelectorExpr),
// fails the guard.
// cageResolverRefs returns, per enclosing declaration, every
// reference to the resolver identifier in one source (Ident and
// selector Sel alike; package-level GenDecls swept as <package-level>).
func cageResolverRefs(filename string, src []byte) (map[string][]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return nil, err
	}
	found := map[string][]string{}
	note := func(scope string, pos token.Pos) {
		found[scope] = append(found[scope], fset.Position(pos).String())
	}
	scan := func(scope string, root ast.Node) {
		ast.Inspect(root, func(n ast.Node) bool {
			switch e := n.(type) {
			case *ast.SelectorExpr:
				if e.Sel.Name == "ResolveEffectiveCage" {
					note(scope, e.Sel.Pos())
					return false // do not double-count the Sel ident
				}
			case *ast.Ident:
				if e.Name == "ResolveEffectiveCage" {
					note(scope, e.Pos())
				}
			}
			return true
		})
	}
	for _, d := range f.Decls {
		switch decl := d.(type) {
		case *ast.FuncDecl:
			if decl.Body == nil || decl.Name.Name == "ResolveEffectiveCage" {
				continue
			}
			scan(decl.Name.Name, decl.Body)
		case *ast.GenDecl:
			scan("<package-level>", decl)
		}
	}
	return found, nil
}

func TestCageResolutionGuard_wholeTreeBySite(t *testing.T) {
	t.Parallel()
	type site struct{ pkg, fn string }
	allowed := map[site]bool{
		{"internal/app", "buildAgentBrain"}:    true,
		{"internal/app", "policyDigestFor"}:    true,
		{"internal/app", "ResolveApprovalLaw"}: true,
	}
	root := filepath.Join("..", "..")
	var offenders []string
	seen := map[site]bool{}
	for _, top := range []string{"internal", "cmd", "web"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "node_modules" {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			src, err := os.ReadFile(filepath.Clean(path))
			if err != nil {
				return err
			}
			refs, err := cageResolverRefs(path, src)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, filepath.Dir(path))
			for fn, positions := range refs {
				st := site{filepath.ToSlash(rel), fn}
				if allowed[st] {
					seen[st] = true
					continue
				}
				offenders = append(offenders, fmt.Sprintf("%s.%s at %v", st.pkg, fn, positions))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", top, err)
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("R8-Z4 GUARD: resolver references outside the allowed SITES: %v", offenders)
	}
	if len(seen) != len(allowed) {
		t.Fatalf("R8-Z4 GUARD: expected all %d allowed sites present, saw %d: %v", len(allowed), len(seen), seen)
	}
}

// The auditor's bribers, permanent fixtures of the detector.
func TestCageGuard_siteBribersCannotPass(t *testing.T) {
	t.Parallel()
	// A method NAMED like an allowed function, in another package —
	// the detector reports the reference; the SITE key rejects it.
	byName := `package cli
func policyDigestFor(bc BrainConfig) { _, _ = ResolveEffectiveCage(bc) }`
	refs, err := cageResolverRefs("cli.go", []byte(byName))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(refs["policyDigestFor"]) != 1 {
		t.Fatalf("the name-briber's reference must be seen (the SITE decides): %v", refs)
	}
	// The cross-package reference: app.ResolveEffectiveCage as a value.
	cross := `package cli
import "github.com/Sebastian197/korvun/internal/app"
func sneak() { f := app.ResolveEffectiveCage; _ = f }`
	refs, err = cageResolverRefs("cross.go", []byte(cross))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(refs["sneak"]) != 1 {
		t.Fatalf("AUDIT R8-Z4: the cross-package selector reference must be seen: %v", refs)
	}
}

// The earlier reference bribers stay pinned on the detector.
func TestCageGuard_referenceBribersCannotPass(t *testing.T) {
	t.Parallel()
	for name, src := range map[string]string{
		"value": `package app
func sneak(bc BrainConfig) { f := ResolveEffectiveCage; _, _ = f(bc) }`,
		"paren": `package app
func sneak(bc BrainConfig) { _, _ = (ResolveEffectiveCage)(bc) }`,
	} {
		refs, err := cageResolverRefs(name+".go", []byte(src))
		if err != nil {
			t.Fatalf("%s: parse: %v", name, err)
		}
		if len(refs["sneak"]) != 1 {
			t.Fatalf("the %s briber must be seen: %v", name, refs)
		}
	}
	alias := `package app
var sneaky = ResolveEffectiveCage
func sneak(bc BrainConfig) { _, _ = sneaky(bc) }`
	refs, err := cageResolverRefs("alias.go", []byte(alias))
	if err != nil {
		t.Fatalf("alias: parse: %v", err)
	}
	if len(refs["<package-level>"]) != 1 {
		t.Fatalf("the package-level alias must be seen: %v", refs)
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

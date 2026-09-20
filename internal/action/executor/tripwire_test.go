// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const (
	executorImportPath = "github.com/Sebastian197/korvun/internal/action/executor"
	brainImportPath    = "github.com/Sebastian197/korvun/internal/brain"
	appImportPath      = "github.com/Sebastian197/korvun/internal/app"
)

type listedPackage struct {
	Dir        string
	ImportPath string
	Export     string
	GoFiles    []string
	CgoFiles   []string
	Standard   bool
}

type typedPackage struct {
	path  string
	fset  *token.FileSet
	files []*ast.File
	info  *types.Info
}

type structuralViolation struct {
	kind string
	pos  token.Position
	note string
}

func (v structuralViolation) String() string {
	return fmt.Sprintf("%s %s: %s", v.kind, v.pos, v.note)
}

func TestExecutor_ASTRejectsEveryExternalDispatch(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	configs := []struct {
		goos string
		tags string
	}{
		{goos: "darwin"},
		{goos: "darwin", tags: "desktop"},
		{goos: "linux"},
		{goos: "linux", tags: "desktop"},
		{goos: "windows"},
		{goos: "windows", tags: "desktop"},
	}

	var violations []structuralViolation
	for _, cfg := range configs {
		cfg := cfg
		t.Run(cfg.goos+"/"+tagName(cfg.tags), func(t *testing.T) {
			packages := loadTypedProduction(t, root, cfg.goos, cfg.tags)
			for _, pkg := range packages {
				violations = append(violations, dispatchViolations(pkg)...)
				violations = append(violations, coordinatorViolations(pkg)...)
			}
		})
	}

	fixture := typeFixture(t)
	got := dispatchViolations(fixture)
	if len(got) != 8 {
		t.Fatalf("typed fixtures produced %d violations, want 8: %v", len(got), got)
	}
	for _, v := range got {
		if v.kind != "direct_tool_dispatch" {
			t.Fatalf("fixture violation kind = %q, want direct_tool_dispatch", v.kind)
		}
	}
	decoy := dispatchDecoyFixture(t)
	foundDecoy := false
	for _, v := range dispatchViolations(decoy) {
		if strings.Contains(v.pos.Filename, "decoy.go") {
			foundDecoy = true
		}
	}
	if !foundDecoy {
		t.Fatal("a different receiver's dispatch method escaped the exact executor guard")
	}
	for _, capture := range coordinatorCaptureFixtures(t) {
		if got := coordinatorViolations(capture); len(got) == 0 {
			t.Fatalf("coordinator method capture escaped the typed guard in %s", capture.path)
		}
	}

	if len(violations) != 0 {
		sort.Slice(violations, func(i, j int) bool {
			return violations[i].String() < violations[j].String()
		})
		var lines []string
		for _, v := range violations {
			lines = append(lines, v.String())
		}
		t.Fatalf("canonical execution structure violated:\n%s", strings.Join(lines, "\n"))
	}
}

func tagName(tags string) string {
	if tags == "" {
		return "default"
	}
	return tags
}

func loadTypedProduction(t *testing.T, root, goos, tags string) []typedPackage {
	t.Helper()
	args := []string{"list", "-deps", "-export", "-json"}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	args = append(args, "./internal/...", "./cmd/...", "./web/builder")
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH=amd64", "CGO_ENABLED=0")
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			t.Fatalf("go list (%s/%s): %v\n%s", goos, tagName(tags), err, exitErr.Stderr)
		}
		t.Fatalf("go list (%s/%s): %v", goos, tagName(tags), err)
	}

	dec := json.NewDecoder(bytes.NewReader(out))
	listed := make(map[string]listedPackage)
	for {
		var pkg listedPackage
		if err := dec.Decode(&pkg); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatalf("decode go list (%s/%s): %v", goos, tagName(tags), err)
		}
		listed[pkg.ImportPath] = pkg
	}

	exports := make(map[string]string, len(listed))
	for path, pkg := range listed {
		if pkg.Export != "" {
			exports[path] = pkg.Export
		}
	}
	lookup := func(path string) (io.ReadCloser, error) {
		file := exports[path]
		if file == "" {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		// #nosec G304 -- every path is compiler export data emitted by this
		// exact go list invocation, never user input.
		return os.Open(file)
	}

	var targets []listedPackage
	for _, pkg := range listed {
		if pkg.Standard || !strings.HasPrefix(pkg.ImportPath, "github.com/Sebastian197/korvun/") {
			continue
		}
		if len(pkg.GoFiles)+len(pkg.CgoFiles) == 0 {
			continue
		}
		targets = append(targets, pkg)
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].ImportPath < targets[j].ImportPath })

	result := make([]typedPackage, 0, len(targets))
	for _, pkg := range targets {
		fset := token.NewFileSet()
		var files []*ast.File
		goFiles := append(append([]string{}, pkg.GoFiles...), pkg.CgoFiles...)
		for _, name := range goFiles {
			file, err := parser.ParseFile(fset, filepath.Join(pkg.Dir, name), nil, parser.SkipObjectResolution)
			if err != nil {
				t.Fatalf("parse %s/%s: %v", pkg.ImportPath, name, err)
			}
			files = append(files, file)
		}
		info := &types.Info{
			Types:      make(map[ast.Expr]types.TypeAndValue),
			Defs:       make(map[*ast.Ident]types.Object),
			Uses:       make(map[*ast.Ident]types.Object),
			Selections: make(map[*ast.SelectorExpr]*types.Selection),
		}
		conf := types.Config{Importer: importer.ForCompiler(fset, "gc", lookup)}
		if _, err := conf.Check(pkg.ImportPath, fset, files, info); err != nil {
			t.Fatalf("type-check %s (%s/%s): %v", pkg.ImportPath, goos, tagName(tags), err)
		}
		result = append(result, typedPackage{path: pkg.ImportPath, fset: fset, files: files, info: info})
	}
	return result
}

func dispatchViolations(pkg typedPackage) []structuralViolation {
	var out []structuralViolation
	allowed := 0
	// judge decides ONE selector. fn is the enclosing function declaration, or
	// nil when the selector lives outside every function body — a package-level
	// `var x = func(...) { t.Execute(...) }`, a map of closures, any
	// initializer. Such a site has no adapter contract and can never be the
	// allowed dispatch, so it is always a violation.
	judge := func(sel *ast.SelectorExpr, fn *ast.FuncDecl) {
		if sel.Sel.Name != "Execute" && sel.Sel.Name != "ExecuteScoped" {
			return
		}
		obj := selectorObject(pkg.info, sel)
		if !isExecutionMethod(obj, sel.Sel.Name) {
			return
		}
		if fn != nil && isExecutorDispatch(pkg, fn) && selectorIsDirectCall(fn.Body, sel) {
			allowed++
			return
		}
		out = append(out, structuralViolation{
			kind: "direct_tool_dispatch",
			pos:  pkg.fset.Position(sel.Pos()),
			note: "Tool.Execute and ExecuteScoped are confined to executor.dispatch",
		})
	}
	// The walk covers EVERY declaration of every file, not only function
	// bodies. The earlier body-only walk skipped `*ast.GenDecl` entirely, and a
	// reachable exported bypass written as a package-level closure passed the
	// guard, `go build`, `go vet` and all four suites — the twenty-first pass's
	// P2-1, reproduced with `internal/brain/zz_backdoor.go`. The repository
	// already writes that shape in production (`internal/action/sqlite/store.go`,
	// `var migrationCopies = map[int]func(*sql.Tx) error{`), so it is idiomatic
	// Go, not an exotic attack.
	for _, file := range pkg.files {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				ast.Inspect(fn.Body, func(node ast.Node) bool {
					if sel, ok := node.(*ast.SelectorExpr); ok {
						judge(sel, fn)
					}
					return true
				})
				continue
			}
			ast.Inspect(decl, func(node ast.Node) bool {
				if sel, ok := node.(*ast.SelectorExpr); ok {
					judge(sel, nil)
				}
				return true
			})
		}
	}
	if pkg.path == executorImportPath && allowed != 2 {
		out = append(out, structuralViolation{
			kind: "direct_tool_dispatch",
			pos:  token.Position{Filename: pkg.path},
			note: fmt.Sprintf("executor.dispatch has %d physical dispatch sites, want 2", allowed),
		})
	}
	return out
}

func isExecutorDispatch(pkg typedPackage, fn *ast.FuncDecl) bool {
	if pkg.path != executorImportPath || fn.Name.Name != "dispatch" {
		return false
	}
	obj, ok := pkg.info.Defs[fn.Name].(*types.Func)
	return ok && isExecutorMethod(obj, "dispatch")
}

func selectorObject(info *types.Info, sel *ast.SelectorExpr) types.Object {
	if selection := info.Selections[sel]; selection != nil {
		return selection.Obj()
	}
	return info.Uses[sel.Sel]
}

func isExecutionMethod(obj types.Object, name string) bool {
	fn, ok := obj.(*types.Func)
	if !ok || fn.Name() != name {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Results().Len() != 2 || !isBuiltin(sig.Results().At(0).Type(), "string") || !isError(sig.Results().At(1).Type()) {
		return false
	}
	wantParams := 2
	if name == "ExecuteScoped" {
		wantParams = 3
	}
	if sig.Params().Len() != wantParams || !isNamed(sig.Params().At(0).Type(), "context", "Context") {
		return false
	}
	if name == "ExecuteScoped" && !isNamed(sig.Params().At(1).Type(), "github.com/Sebastian197/korvun/internal/tool", "Scope") {
		return false
	}
	return isBuiltin(sig.Params().At(wantParams-1).Type(), "string")
}

func isNamed(t types.Type, pkgPath, name string) bool {
	named, ok := types.Unalias(t).(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == pkgPath && named.Obj().Name() == name
}

func isBuiltin(t types.Type, name string) bool {
	basic, ok := types.Unalias(t).(*types.Basic)
	return ok && basic.Name() == name
}

func isError(t types.Type) bool {
	return types.Identical(types.Unalias(t), types.Universe.Lookup("error").Type())
}

func selectorIsDirectCall(root ast.Node, target *ast.SelectorExpr) bool {
	direct := false
	ast.Inspect(root, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if unparen(call.Fun) == target {
			direct = true
		}
		return true
	})
	return direct
}

func unparen(expr ast.Expr) ast.Expr {
	for {
		p, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = p.X
	}
}

func coordinatorViolations(pkg typedPackage) []structuralViolation {
	var out []structuralViolation
	for _, file := range pkg.files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				// Same hole as the dispatch walk (P2-1): a coordinator call
				// written outside every function body has no adapter contract,
				// so any reference to one from a package-level declaration is a
				// bypass by construction.
				ast.Inspect(decl, func(node ast.Node) bool {
					sel, ok := node.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					called, _ := selectorObject(pkg.info, sel).(*types.Func)
					if called == nil || !isCoordinatorSignature(called, called.Name()) {
						return true
					}
					out = append(out, structuralViolation{
						kind: "canonical_coordinator_bypass",
						pos:  pkg.fset.Position(sel.Pos()),
						note: fmt.Sprintf("package-level declaration uses coordinator method %s outside its direct adapter site", called.Name()),
					})
					return true
				})
				continue
			}
			required, forbidden := adapterContract(pkg, fn)
			counts := make(map[string]int)
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				switch target := node.(type) {
				case *ast.SelectorExpr:
					called, _ := selectorObject(pkg.info, target).(*types.Func)
					if called == nil {
						return true
					}
					name := called.Name()
					if isCoordinatorSignature(called, name) {
						exact := isExecutorMethod(called, name)
						direct := selectorIsDirectCall(fn.Body, target)
						if exact && direct && coordinatorSiteAllows(pkg, fn, name) {
							counts[name]++
						} else {
							out = append(out, structuralViolation{
								kind: "canonical_coordinator_bypass",
								pos:  pkg.fset.Position(target.Pos()),
								note: fmt.Sprintf("%s uses coordinator method %s outside its direct adapter site", fn.Name.Name, name),
							})
						}
					}
					if paths, ok := forbidden[name]; ok && paths[functionPackage(called)] {
						counts[name]++
					}
				case *ast.Ident:
					called, _ := pkg.info.Uses[target].(*types.Func)
					if called == nil || called.Pkg() == nil || called.Pkg().Path() != pkg.path {
						return true
					}
					if paths, ok := forbidden[called.Name()]; ok && paths[functionPackage(called)] {
						counts[called.Name()]++
					}
				}
				return true
			})
			for name, want := range required {
				if counts[name] != want {
					out = append(out, structuralViolation{
						kind: "canonical_coordinator_bypass",
						pos:  pkg.fset.Position(fn.Pos()),
						note: fmt.Sprintf("%s calls %s %d times, want %d", fn.Name.Name, name, counts[name], want),
					})
				}
			}
			for name := range forbidden {
				if counts[name] != 0 {
					out = append(out, structuralViolation{
						kind: "canonical_coordinator_bypass",
						pos:  pkg.fset.Position(fn.Pos()),
						note: fmt.Sprintf("%s retains legacy coordination call %s", fn.Name.Name, name),
					})
				}
			}
		}
	}
	return out
}

func adapterContract(pkg typedPackage, fn *ast.FuncDecl) (map[string]int, map[string]map[string]bool) {
	switch {
	case pkg.path == brainImportPath && fn.Name.Name == "runTool":
		return map[string]int{"Prepare": 1, "Submit": 1}, map[string]map[string]bool{
			"Run":                 stringSet(executorImportPath),
			"Has":                 stringSet(executorImportPath),
			"recordAttempt":       stringSet(brainImportPath),
			"recordAuthorized":    stringSet(brainImportPath),
			"finishAction":        stringSet(brainImportPath),
			"buildActionEnvelope": stringSet(brainImportPath),
			"effectGateRule":      stringSet(brainImportPath),
		}
	case pkg.path == appImportPath && fn.Name.Name == "ExecuteApprovedAction":
		return map[string]int{"ResumeApproved": 1}, map[string]map[string]bool{
			"Run":                            stringSet(executorImportPath),
			"GetApproval":                    stringSet("github.com/Sebastian197/korvun/internal/action/sqlite"),
			"Get":                            stringSet("github.com/Sebastian197/korvun/internal/action/sqlite"),
			"ClaimApprovalParamsUnderDigest": stringSet("github.com/Sebastian197/korvun/internal/action/sqlite"),
			"FinishWithResult":               stringSet("github.com/Sebastian197/korvun/internal/action/sqlite"),
			"CloseStateAfterRun":             stringSet("github.com/Sebastian197/korvun/internal/tool"),
		}
	default:
		return nil, nil
	}
}

func coordinatorSiteAllows(pkg typedPackage, fn *ast.FuncDecl, name string) bool {
	switch {
	case pkg.path == brainImportPath && fn.Name.Name == "runTool":
		return name == "Prepare" || name == "Submit"
	case pkg.path == appImportPath && fn.Name.Name == "ExecuteApprovedAction":
		return name == "ResumeApproved"
	default:
		return false
	}
}

func stringSet(values ...string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}

func functionPackage(fn *types.Func) string {
	if fn == nil || fn.Pkg() == nil {
		return ""
	}
	return fn.Pkg().Path()
}

func isExecutorMethod(fn *types.Func, name string) bool {
	if fn == nil || fn.Name() != name || functionPackage(fn) != executorImportPath {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return false
	}
	receiver := types.Unalias(sig.Recv().Type())
	if pointer, ok := receiver.(*types.Pointer); ok {
		receiver = types.Unalias(pointer.Elem())
	}
	named, ok := receiver.(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == executorImportPath && named.Obj().Name() == "Executor"
}

func isCoordinatorSignature(fn *types.Func, name string) bool {
	if fn == nil || fn.Name() != name {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return false
	}
	switch name {
	case "Prepare":
		return sig.Params().Len() == 1 && sig.Results().Len() == 2 &&
			isNamed(sig.Params().At(0).Type(), executorImportPath, "Submission") &&
			isPointerNamed(sig.Results().At(0).Type(), executorImportPath, "Request") &&
			isError(sig.Results().At(1).Type())
	case "Submit":
		return sig.Params().Len() == 2 && sig.Results().Len() == 2 &&
			isNamed(sig.Params().At(0).Type(), "context", "Context") &&
			isPointerNamed(sig.Params().At(1).Type(), executorImportPath, "Request") &&
			isNamed(sig.Results().At(0).Type(), executorImportPath, "Result") &&
			isError(sig.Results().At(1).Type())
	case "ResumeApproved":
		return sig.Params().Len() == 3 && sig.Results().Len() == 2 &&
			isNamed(sig.Params().At(0).Type(), "context", "Context") &&
			isNamed(sig.Params().At(1).Type(), executorImportPath, "ApprovalStore") &&
			isBuiltin(sig.Params().At(2).Type(), "string") &&
			isNamed(sig.Results().At(0).Type(), executorImportPath, "ApprovedResult") &&
			isError(sig.Results().At(1).Type())
	default:
		return false
	}
}

func isPointerNamed(t types.Type, pkgPath, name string) bool {
	pointer, ok := types.Unalias(t).(*types.Pointer)
	return ok && isNamed(pointer.Elem(), pkgPath, name)
}

func typeFixture(t *testing.T) typedPackage {
	t.Helper()
	const src = `package fixture
import (
	"context"
	"github.com/Sebastian197/korvun/internal/tool"
)
type direct interface { Execute(context.Context, string) (string, error) }
type scoped interface { ExecuteScoped(context.Context, tool.Scope, string) (string, error) }
type execContext = context.Context
type execString = string
type execScope = tool.Scope
type aliasDirect interface { Execute(execContext, execString) (execString, error) }
type aliasScoped interface { ExecuteScoped(execContext, execScope, execString) (execString, error) }
func a(ctx context.Context, t direct) { t.Execute(ctx, "a") }
func b(ctx context.Context, t direct) { (t.Execute)(ctx, "b") }
func c(ctx context.Context, t direct) { f := t.Execute; _, _ = f(ctx, "c") }
func d(ctx context.Context, t direct) { f := direct.Execute; _, _ = f(t, ctx, "d") }
func e(ctx context.Context, t scoped) { t.ExecuteScoped(ctx, tool.Scope{}, "e") }
func f(ctx context.Context, t scoped) { run := scoped.ExecuteScoped; _, _ = run(t, ctx, tool.Scope{}, "f") }
func g(ctx execContext, t aliasDirect) { t.Execute(ctx, "g") }
func h(ctx execContext, t aliasScoped) { t.ExecuteScoped(ctx, execScope{}, "h") }
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	conf := types.Config{Importer: fixtureImporter{base: importer.Default()}}
	if _, err := conf.Check("fixture", fset, []*ast.File{file}, info); err != nil {
		t.Fatalf("type-check fixtures: %v", err)
	}
	return typedPackage{path: "fixture", fset: fset, files: []*ast.File{file}, info: info}
}

func dispatchDecoyFixture(t *testing.T) typedPackage {
	t.Helper()
	const src = `package executor
import "context"
type direct interface { Execute(context.Context, string) (string, error) }
type decoy struct{}
func (decoy) dispatch(ctx context.Context, candidate direct) { candidate.Execute(ctx, "x") }
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "decoy.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	conf := types.Config{Importer: importer.Default()}
	if _, err := conf.Check(executorImportPath, fset, []*ast.File{file}, info); err != nil {
		t.Fatalf("type-check dispatch decoy: %v", err)
	}
	return typedPackage{path: executorImportPath, fset: fset, files: []*ast.File{file}, info: info}
}

func coordinatorCaptureFixtures(t *testing.T) []typedPackage {
	t.Helper()
	fixtures := []struct {
		path string
		name string
		src  string
	}{
		{
			path: brainImportPath,
			name: "brain_capture.go",
			src: `package brain
import (
	"context"
	executor "github.com/Sebastian197/korvun/internal/action/executor"
)
func runTool(ctx context.Context, e *executor.Executor, s executor.Submission, r *executor.Request) {
	prepare := e.Prepare
	_ = prepare
	e.Submit(ctx, r)
}
`,
		},
		{
			path: appImportPath,
			name: "app_capture.go",
			src: `package app
import (
	"context"
	executor "github.com/Sebastian197/korvun/internal/action/executor"
)
func ExecuteApprovedAction(ctx context.Context, e *executor.Executor, s executor.ApprovalStore) {
	resume := e.ResumeApproved
	_ = resume
}
`,
		},
		{
			path: brainImportPath,
			name: "brain_interface.go",
			src: `package brain
import (
	"context"
	executor "github.com/Sebastian197/korvun/internal/action/executor"
)
type coordinatorDoor interface {
	Prepare(executor.Submission) (*executor.Request, error)
	Submit(context.Context, *executor.Request) (executor.Result, error)
}
func runTool(ctx context.Context, e *executor.Executor, door coordinatorDoor, s executor.Submission, r *executor.Request) {
	if false {
		e.Prepare(s)
		e.Submit(ctx, r)
	}
	door.Prepare(s)
	door.Submit(ctx, r)
}
`,
		},
		{
			path: brainImportPath,
			name: "brain_helper.go",
			src: `package brain
import (
	"context"
	executor "github.com/Sebastian197/korvun/internal/action/executor"
)
type helperDoor interface {
	Prepare(executor.Submission) (*executor.Request, error)
	Submit(context.Context, *executor.Request) (executor.Result, error)
}
func coordinate(ctx context.Context, door helperDoor, s executor.Submission, r *executor.Request) {
	door.Prepare(s)
	door.Submit(ctx, r)
}
func runTool(ctx context.Context, e *executor.Executor, door helperDoor, s executor.Submission, r *executor.Request) {
	if false {
		e.Prepare(s)
		e.Submit(ctx, r)
	}
	coordinate(ctx, door, s, r)
}
`,
		},
		{
			path: brainImportPath,
			name: "brain_alias_helper.go",
			src: `package brain
import (
	"context"
	executor "github.com/Sebastian197/korvun/internal/action/executor"
)
type aliasContext = context.Context
type aliasSubmission = executor.Submission
type aliasRequest = executor.Request
type aliasResult = executor.Result
type aliasDoor interface {
	Prepare(aliasSubmission) (*aliasRequest, error)
	Submit(aliasContext, *aliasRequest) (aliasResult, error)
}
func aliasCoordinate(ctx aliasContext, door aliasDoor, s aliasSubmission, r *aliasRequest) {
	door.Prepare(s)
	door.Submit(ctx, r)
}
func runTool(ctx context.Context, e *executor.Executor, door aliasDoor, s executor.Submission, r *executor.Request) {
	if false {
		e.Prepare(s)
		e.Submit(ctx, r)
	}
	aliasCoordinate(ctx, door, s, r)
}
`,
		},
	}
	result := make([]typedPackage, 0, len(fixtures))
	for _, fixture := range fixtures {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, fixture.name, fixture.src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		info := &types.Info{
			Types:      make(map[ast.Expr]types.TypeAndValue),
			Defs:       make(map[*ast.Ident]types.Object),
			Uses:       make(map[*ast.Ident]types.Object),
			Selections: make(map[*ast.SelectorExpr]*types.Selection),
		}
		conf := types.Config{Importer: coordinatorFixtureImporter{base: importer.Default()}}
		if _, err := conf.Check(fixture.path, fset, []*ast.File{file}, info); err != nil {
			t.Fatalf("type-check coordinator capture fixture: %v", err)
		}
		result = append(result, typedPackage{path: fixture.path, fset: fset, files: []*ast.File{file}, info: info})
	}
	return result
}

type coordinatorFixtureImporter struct {
	base types.Importer
}

func (i coordinatorFixtureImporter) Import(path string) (*types.Package, error) {
	if path != executorImportPath {
		if i.base == nil {
			i.base = importer.Default()
		}
		return i.base.Import(path)
	}
	pkg := types.NewPackage(path, "executor")
	typeName := types.NewTypeName(token.NoPos, pkg, "Executor", nil)
	named := types.NewNamed(typeName, types.NewStruct(nil, nil), nil)
	pkg.Scope().Insert(typeName)
	addNamed := func(name string, underlying types.Type) *types.Named {
		obj := types.NewTypeName(token.NoPos, pkg, name, nil)
		typ := types.NewNamed(obj, underlying, nil)
		pkg.Scope().Insert(obj)
		return typ
	}
	submission := addNamed("Submission", types.NewStruct(nil, nil))
	request := addNamed("Request", types.NewStruct(nil, nil))
	result := addNamed("Result", types.NewStruct(nil, nil))
	approvalStore := addNamed("ApprovalStore", types.NewInterfaceType(nil, nil).Complete())
	approvedResult := addNamed("ApprovedResult", types.NewStruct(nil, nil))
	ctxPkg, err := i.Import("context")
	if err != nil {
		return nil, err
	}
	ctxType := ctxPkg.Scope().Lookup("Context").Type()
	errType := types.Universe.Lookup("error").Type()
	methods := []struct {
		name    string
		params  *types.Tuple
		results *types.Tuple
	}{
		{name: "Prepare",
			params:  types.NewTuple(types.NewVar(token.NoPos, pkg, "", submission)),
			results: types.NewTuple(types.NewVar(token.NoPos, pkg, "", types.NewPointer(request)), types.NewVar(token.NoPos, pkg, "", errType))},
		{name: "Submit",
			params:  types.NewTuple(types.NewVar(token.NoPos, pkg, "", ctxType), types.NewVar(token.NoPos, pkg, "", types.NewPointer(request))),
			results: types.NewTuple(types.NewVar(token.NoPos, pkg, "", result), types.NewVar(token.NoPos, pkg, "", errType))},
		{name: "ResumeApproved",
			params: types.NewTuple(types.NewVar(token.NoPos, pkg, "", ctxType), types.NewVar(token.NoPos, pkg, "", approvalStore),
				types.NewVar(token.NoPos, pkg, "", types.Typ[types.String])),
			results: types.NewTuple(types.NewVar(token.NoPos, pkg, "", approvedResult), types.NewVar(token.NoPos, pkg, "", errType))},
	}
	for _, method := range methods {
		sig := types.NewSignatureType(
			types.NewVar(token.NoPos, pkg, "", types.NewPointer(named)),
			nil,
			nil,
			method.params,
			method.results,
			false,
		)
		named.AddMethod(types.NewFunc(token.NoPos, pkg, method.name, sig))
	}
	pkg.MarkComplete()
	return pkg, nil
}

type fixtureImporter struct {
	base types.Importer
}

func (i fixtureImporter) Import(path string) (*types.Package, error) {
	if path != "github.com/Sebastian197/korvun/internal/tool" {
		return i.base.Import(path)
	}
	pkg := types.NewPackage(path, "tool")
	name := types.NewTypeName(token.NoPos, pkg, "Scope", nil)
	types.NewNamed(name, types.NewStruct(nil, nil), nil)
	pkg.Scope().Insert(name)
	pkg.MarkComplete()
	return pkg, nil
}

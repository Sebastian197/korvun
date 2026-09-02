// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R2 of the third Codex pass (adjudicated 2026-09-01): rotate-key was
// the surviving member of the killed class — a CLI act still walking
// through the FULL boot door (recovery + prune + migration), able to
// close a live server's in-flight work beside it. The cure sweeps the
// CLASS: every operator act goes through OpenOperator, the auditor's
// reproduction rides as a permanent test, and a CLASS GUARD fails the
// suite if any CLI command ever calls the full Open again.
// Reproduction-first contract.

package cli

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
)

func TestRotateKey_besideALiveServerTouchesNothing(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, _ := parkedRequest(t)
	// The live server's in-flight work: an AUTHORIZED action mid-run.
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	env := action.NewEnvelope("act_r2_live", "env-r2",
		action.Source{Kind: "agent_brain", Protocol: "text", Channel: "console"},
		action.Operation{Namespace: "tool", Name: "calc", Version: 1},
		`1+1`, time.Now().UTC())
	if err := store.RecordAttempt(context.Background(), env,
		actionsqlite.Decision{Outcome: "allow", Rule: "allow"}, action.StateAuthorized); err != nil {
		t.Fatalf("record: %v", err)
	}
	// The server stays OPEN while the operator rotates the key.
	defer func() { _ = store.Close() }()
	if code, _, stderr := runIntentCLI(t, "receipt", "rotate-key", "--config", cfgPath); code != 0 {
		t.Fatalf("rotate-key: %d %q", code, stderr)
	}
	rec, err := store.Get(context.Background(), "act_r2_live")
	if err != nil || rec.State != action.StateAuthorized || rec.RecoveryMarker != "" {
		t.Fatalf("AUDIT R2: rotate-key must never close the live server's work: %v %v %q",
			err, rec.State, rec.RecoveryMarker)
	}
}

// The class guard (R2, made unbribable by F2): no CLI source file may
// call the full boot door — resolved by AST, so an import ALIAS or a
// dot-import cannot smuggle the call past a literal grep. OpenOperator
// and OpenReadOnly are the only doors an operator command may walk;
// the full Open (prune+migration; the boot adds recovery) belongs to
// the server boot alone. Kill the class, not the bug.

// fullBootDoorCalls parses one Go source and returns the positions of
// every call to the action/sqlite package's Open — whatever local name
// the import wears (default, alias, or dot).
func fullBootDoorCalls(filename string, src []byte) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return nil, err
	}
	const doorPkg = "github.com/Sebastian197/korvun/internal/action/sqlite"
	names := map[string]bool{}
	dot := false
	for _, imp := range f.Imports {
		if strings.Trim(imp.Path.Value, `"`) != doorPkg {
			continue
		}
		switch {
		case imp.Name == nil:
			names["sqlite"] = true // the package's own name
		case imp.Name.Name == ".":
			dot = true
		default:
			names[imp.Name.Name] = true
		}
	}
	if len(names) == 0 && !dot {
		return nil, nil
	}
	var calls []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			if id, ok := fn.X.(*ast.Ident); ok && names[id.Name] && fn.Sel.Name == "Open" {
				calls = append(calls, fset.Position(call.Pos()).String())
			}
		case *ast.Ident:
			if dot && fn.Name == "Open" {
				calls = append(calls, fset.Position(call.Pos()).String())
			}
		}
		return true
	})
	return calls, nil
}

func TestClassGuard_noCLICommandUsesTheFullBootDoor(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		calls, err := fullBootDoorCalls(name, src)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		if len(calls) > 0 {
			t.Fatalf("CLASS GUARD (R2/F2): %s calls the full boot door action/sqlite Open at %v — operator commands walk OpenOperator or OpenReadOnly only", name, calls)
		}
	}
}

// The briber's fixtures: an alias and a dot-import must NOT slip past.
func TestClassGuard_aliasAndDotImportsCannotBribeIt(t *testing.T) {
	t.Parallel()
	for name, src := range map[string]string{
		"alias": `package cli
import asq "github.com/Sebastian197/korvun/internal/action/sqlite"
func x() { _, _ = asq.Open("p") }`,
		"dot": `package cli
import . "github.com/Sebastian197/korvun/internal/action/sqlite"
func x() { _, _ = Open("p") }`,
		"default": `package cli
import "github.com/Sebastian197/korvun/internal/action/sqlite"
func x() { _, _ = sqlite.Open("p") }`,
	} {
		calls, err := fullBootDoorCalls(name+".go", []byte(src))
		if err != nil {
			t.Fatalf("%s: parse: %v", name, err)
		}
		if len(calls) != 1 {
			t.Fatalf("AUDIT F2: the %s import must not bribe the guard: %v", name, calls)
		}
	}
	// And the legitimate doors stay untouched.
	ok := `package cli
import asq "github.com/Sebastian197/korvun/internal/action/sqlite"
func x() { _, _ = asq.OpenOperator("p"); _, _ = asq.OpenReadOnly("p") }`
	calls, err := fullBootDoorCalls("ok.go", []byte(ok))
	if err != nil || len(calls) != 0 {
		t.Fatalf("the operator doors are legitimate: %v %v", calls, err)
	}
}

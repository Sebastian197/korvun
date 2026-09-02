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
	"database/sql"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/app"
)

// CONTRACT INVERSION (R4 Phase 1, authorized by the mandate): the old
// TestRotateKey_besideALiveServerTouchesNothing asserted that a
// rotation BESIDE a live server succeeds and leaves the server's rows
// alone — measuring survival of the ACTION while missing the auditor's
// P1: the live server kept SEALING with the retired key, so every
// receipt after the rotation was born key_window_violated until a
// restart. That contract WAS the bug. The new contract: a rotation
// while the server's profile lock is held refuses with the stable rule
// signing_key_in_use and mutates NOTHING; once the server stops (lock
// released), the rotation proceeds and every era verifies.

func TestRotateKey_refusesWhileServerProfileLockIsHeld(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, _ := parkedRequest(t)
	// The "live server": the profile lock held, as Build holds it.
	lock, err := app.AcquireProfileLock(filepath.Dir(dbPath))
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer func() { _ = lock.Release() }()
	before := activeSigningKey(t, dbPath)
	code, _, stderr := runIntentCLI(t, "receipt", "rotate-key", "--config", cfgPath)
	if code == 0 {
		t.Fatal("AUDIT R4-F1: a rotation beside a live server must refuse")
	}
	if !strings.Contains(stderr, "signing_key_in_use") {
		t.Fatalf("the refusal must carry the stable rule signing_key_in_use: %q", stderr)
	}
	if after := activeSigningKey(t, dbPath); after != before {
		t.Fatalf("ZERO mutations on refusal: key %q became %q", before, after)
	}
}

func TestRotateKey_afterServerStopsRotatesAndReceiptsVerify(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, approvalID := parkedRequest(t)
	// An executed outcome BEFORE the rotation: its receipt seals with
	// the first era's key.
	if code, _, stderr := runIntentCLI(t, "approvals", "approve", "--config", cfgPath, approvalID); code != 0 {
		t.Fatalf("approve: %q", stderr)
	}
	// The server held the lock and STOPPED (released).
	lock, err := app.AcquireProfileLock(filepath.Dir(dbPath))
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	if code, _, stderr := runIntentCLI(t, "receipt", "rotate-key", "--config", cfgPath); code != 0 {
		t.Fatalf("a rotation after the server stops must proceed: %q", stderr)
	}
	// Every era verifies: the pre-rotation receipt under its era's key.
	if code, stdout, _ := runIntentCLI(t, "receipt", "verify", "--config", cfgPath, "act_inbox1"); code != 0 || !strings.Contains(stdout, "OK") {
		t.Fatalf("the first era's receipt verifies after rotation: %d %q", code, stdout)
	}
}

// activeSigningKey reads the currently active key id raw.
func activeSigningKey(t *testing.T, dbPath string) string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("raw: %v", err)
	}
	defer func() { _ = db.Close() }()
	var id string
	err = db.QueryRow(`SELECT key_id FROM signing_keys WHERE retired_at IS NULL OR retired_at = ''`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "" // a fresh profile has no key yet — a valid state to pin
	}
	if err != nil {
		t.Fatalf("active key: %v", err)
	}
	return id
}

// HOUSE AMENDMENT pin: the lock bounds THE ROTATION only — a decision
// act keeps working beside the live server.
func TestApprovalsDecide_worksWhileServerProfileLockIsHeld(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, approvalID := parkedRequest(t)
	lock, err := app.AcquireProfileLock(filepath.Dir(dbPath))
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer func() { _ = lock.Release() }()
	if code, stdout, stderr := runIntentCLI(t, "approvals", "approve", "--config", cfgPath, approvalID); code != 0 || !strings.Contains(stdout, "42") {
		t.Fatalf("HOUSE AMENDMENT: approve must work beside the live server: %d %q %q", code, stdout, stderr)
	}
}

// The class guard (R2/F2, widened by R4 Phase 1): no CLI or cmd
// source may REFERENCE the full boot door in any form — direct call,
// parenthesized call, function value, alias or dot-import — across
// internal/cli, its nested packages, and cmd/korvun. Detection works
// at SELECTOR level (a reference is caught even outside a call), so
// the value-reference briber the self-audit proved possible now dies
// too. OpenOperator and OpenReadOnly stay legitimate.

// fullDoorRefs parses one Go source and returns the positions of every
// REFERENCE to the action/sqlite package's Open.
func fullDoorRefs(filename string, src []byte) ([]string, error) {
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
	var refs []string
	ast.Inspect(f, func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.SelectorExpr:
			if id, ok := e.X.(*ast.Ident); ok && names[id.Name] && e.Sel.Name == "Open" {
				refs = append(refs, fset.Position(e.Pos()).String())
			}
		case *ast.Ident:
			// Dot-import: ANY use of the bare name is flagged —
			// conservative on purpose (fail closed).
			if dot && e.Name == "Open" {
				refs = append(refs, fset.Position(e.Pos()).String())
			}
		}
		return true
	})
	return refs, nil
}

// fullDoorScan walks every non-test Go file under the given roots and
// returns all full-door references found.
func fullDoorScan(roots ...string) (map[string][]string, error) {
	found := map[string][]string{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			src, err := os.ReadFile(filepath.Clean(path))
			if err != nil {
				return err
			}
			refs, err := fullDoorRefs(path, src)
			if err != nil {
				return err
			}
			if len(refs) > 0 {
				found[path] = refs
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return found, nil
}

func TestClassGuard_noCLICommandUsesTheFullBootDoor(t *testing.T) {
	t.Parallel()
	found, err := fullDoorScan(".", filepath.Join("..", "..", "cmd", "korvun"))
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(found) > 0 {
		t.Fatalf("CLASS GUARD (R4-F1): full-boot-door references in operator surfaces: %v — walk OpenOperator or OpenReadOnly only", found)
	}
}

// The briber's fixtures, one per reference form — permanent.
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
		refs, err := fullDoorRefs(name+".go", []byte(src))
		if err != nil {
			t.Fatalf("%s: parse: %v", name, err)
		}
		if len(refs) != 1 {
			t.Fatalf("AUDIT F2: the %s import must not bribe the guard: %v", name, refs)
		}
	}
	ok := `package cli
import asq "github.com/Sebastian197/korvun/internal/action/sqlite"
func x() { _, _ = asq.OpenOperator("p"); _, _ = asq.OpenReadOnly("p") }`
	refs, err := fullDoorRefs("ok.go", []byte(ok))
	if err != nil || len(refs) != 0 {
		t.Fatalf("the operator doors are legitimate: %v %v", refs, err)
	}
}

func TestClassGuard_functionValueCannotBribeIt(t *testing.T) {
	t.Parallel()
	src := `package cli
import asq "github.com/Sebastian197/korvun/internal/action/sqlite"
func x(p string) { f := asq.Open; _, _ = f(p) }`
	refs, err := fullDoorRefs("val.go", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("AUDIT R4-F1: the function-value briber (proven live by the self-audit) must die: %v", refs)
	}
}

func TestClassGuard_parenthesizedCallCannotBribeIt(t *testing.T) {
	t.Parallel()
	src := `package cli
import asq "github.com/Sebastian197/korvun/internal/action/sqlite"
func x(p string) { _, _ = (asq.Open)(p) }`
	refs, err := fullDoorRefs("paren.go", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("AUDIT R4-F1: the parenthesized-call briber must die: %v", refs)
	}
}

func TestClassGuard_scansNestedCLIPackages(t *testing.T) {
	t.Parallel()
	// A nested package under a scanned root must be reached by the walk.
	root := t.TempDir()
	nested := filepath.Join(root, "sub", "deeper")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	briber := `package deeper
import asq "github.com/Sebastian197/korvun/internal/action/sqlite"
func x(p string) { _, _ = asq.Open(p) }`
	if err := os.WriteFile(filepath.Join(nested, "bribe.go"), []byte(briber), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	found, err := fullDoorScan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("AUDIT R4-F1: the walk must reach nested packages: %v", found)
	}
}

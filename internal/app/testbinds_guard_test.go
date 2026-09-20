// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// allowedWideBindSites names the ONE test allowed to hand a non-loopback
// address to bootAdmin: the remote-peer mould, which cannot place a peer the
// server did not choose over a socket that only loopback can reach, and which
// is gated behind lanBindOptIn. The exception is named here, in the guard, so
// nobody has to infer it.
var allowedWideBindSites = map[string]bool{
	"TestV0151B_P2_7_noRouteServesOrDecidesForARemotePeer": true,
}

// TestV0151B_P2_7_everyAdminBindInThisPackageIsGuarded watches the CALL SITES,
// which a pure-function mould cannot see. Deleting bootAdmin's guard call left
// the whole package green (the twenty-third pass, P2-1), and three sibling
// helpers reach net.Listen through cfg.Observability.Addr without ever passing
// through the guard (P2-3). This scan judges both shapes over the package's own
// test sources.
//
// PERIMETER, said once: it reads THIS package's *_test.go files and judges (a)
// that bootAdmin still calls loopbackBindAllowed, (b) every literal address
// handed to bootAdmin, and (c) every literal Addr: field of a config struct in
// a test. It does NOT see an address built at runtime, read from a fixture file
// or passed through another helper's parameter — webhook_wiring_test.go's
// "0.0.0.0:0" is exactly that shape and is known: it is only ever BUILT, never
// started, and that test asserts the WARN the build emits. A scan is a
// discipline aid, not a proof that no other shape exists.
//
// Evidence: source-level, go/parser over this package; no socket is opened.
// Probing mutations executed: deleting the guard call from bootAdmin reddens
// (a); spelling any bootAdmin literal 0.0.0.0:0 outside the named site reddens
// (b); an Addr: "0.0.0.0:0" in a test reddens (c).
func TestV0151B_P2_7_everyAdminBindInThisPackageIsGuarded(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	guarded := false
	scanned := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, perr := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if perr != nil {
			t.Fatalf("parse %s: %v", name, perr)
		}
		scanned++
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if fn.Name.Name == "bootAdmin" && callsGuard(fn.Body) {
				guarded = true
			}
			enclosing := fn.Name.Name
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				switch n := node.(type) {
				case *ast.CallExpr:
					judgeBootAdminCall(t, fset, n, enclosing)
				case *ast.KeyValueExpr:
					judgeAddrField(t, fset, n)
				}
				return true
			})
		}
	}
	if scanned == 0 {
		t.Fatal("no test sources scanned; the guard would pass over nothing")
	}
	if !guarded {
		t.Fatal("bootAdmin no longer calls loopbackBindAllowed: the admin bind of every test in this package is ungoverned")
	}
}

// callsGuard reports whether a body calls loopbackBindAllowed.
func callsGuard(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "loopbackBindAllowed" {
			found = true
		}
		return true
	})
	return found
}

func judgeBootAdminCall(t *testing.T, fset *token.FileSet, call *ast.CallExpr, enclosing string) {
	t.Helper()
	ident, ok := call.Fun.(*ast.Ident)
	if !ok || ident.Name != "bootAdmin" || len(call.Args) < 2 {
		return
	}
	addr, ok := stringLiteral(call.Args[1])
	if !ok {
		return
	}
	if err := loopbackBindAllowed(addr, false); err != nil && !allowedWideBindSites[enclosing] {
		t.Errorf("%s: %s hands bootAdmin a bind that is not loopback: %v", fset.Position(call.Pos()), enclosing, err)
	}
}

func judgeAddrField(t *testing.T, fset *token.FileSet, kv *ast.KeyValueExpr) {
	t.Helper()
	key, ok := kv.Key.(*ast.Ident)
	if !ok || key.Name != "Addr" {
		return
	}
	addr, ok := stringLiteral(kv.Value)
	if !ok {
		return
	}
	if err := loopbackBindAllowed(addr, false); err != nil {
		t.Errorf("%s: a test sets Addr to a bind that is not loopback: %v", fset.Position(kv.Pos()), err)
	}
}

func stringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return value, true
}

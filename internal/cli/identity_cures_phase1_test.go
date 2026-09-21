// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	identityv2 "github.com/Sebastian197/korvun/internal/identity"
)

// TestIdentity_OperatorMintingRequiresTheOpenStore attacks the local
// operator's authentication cut. The function minted a capability from
// arguments alone — a namespace, a name and a parameter string — so nothing in
// its signature could tell an authenticated operator act from any other call,
// and the paper's claim that every door mints "at the point its own
// authentication succeeds" was false here by construction.
//
// The cut is now the OPEN SEALED STORE, passed in: holding it means the caller
// already crossed the local profile's own door. A nil store is refused as
// missing ingress, by name.
//
// The second half is the one that matters: a source scan over this package's
// production files, so no future caller can reach the minting function with a
// literal nil and restore the hole a behaviour test alone would not see.
//
// Evidence level: in-process for the refusal; source-level (go/parser) for the
// call sites.
// Probing mutation executed: delete the `store == nil` guard — the refusal
// half reddens with "minted with no store".
func TestIdentity_OperatorMintingRequiresTheOpenStore(t *testing.T) {
	t.Parallel()

	t.Run("no open store, no capability", func(t *testing.T) {
		env, evidence, err := operatorAuthenticatedEnvelope(nil, "intent", "adopt-root", "{}")
		if !errors.Is(err, identityv2.ErrIdentityEvidenceMissing) {
			t.Fatalf("minted with no store: env=%+v evidence=%+v err=%v", env, evidence, err)
		}
		if env.ActionID != "" || evidence.EvidenceID != "" {
			t.Fatalf("a refused mint still produced material: %+v / %+v", env, evidence)
		}
	})

	t.Run("no call site hands it a literal nil", func(t *testing.T) {
		fset := token.NewFileSet()
		entries, err := os.ReadDir(".")
		if err != nil {
			t.Fatalf("read package dir: %v", err)
		}
		scanned, callSites := 0, 0
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			file, perr := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
			if perr != nil {
				t.Fatalf("parse %s: %v", name, perr)
			}
			scanned++
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				ident, ok := call.Fun.(*ast.Ident)
				if !ok || ident.Name != "operatorAuthenticatedEnvelope" || len(call.Args) == 0 {
					return true
				}
				callSites++
				if arg, isIdent := call.Args[0].(*ast.Ident); isIdent && arg.Name == "nil" {
					t.Errorf("%s: an operator act mints its capability with no open store",
						fset.Position(call.Pos()))
				}
				return true
			})
		}
		if scanned == 0 {
			t.Fatal("no production sources scanned; the guard would pass over nothing")
		}
		if callSites == 0 {
			t.Fatal("no call site found: the scan is watching a name nothing calls")
		}
	})
}

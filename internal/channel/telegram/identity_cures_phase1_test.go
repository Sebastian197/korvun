// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/identity"
)

// TestIdentity_NoProductionPathDispatchesUnauthenticated watches the CALL
// SITES, which a behaviour test cannot see. This phase split one dispatch into
// an authenticated and an unauthenticated form and told them apart by a bool
// parameter and by a NAME — nothing structural stops a future production caller
// from reaching for the unauthenticated one and delivering an envelope with no
// ingress, whose refusal would only arrive much later, at the coordinator.
//
// PERIMETER, said once: it reads this package's NON-test .go files and judges
// (a) that no call to dispatchUpdate survives outside a test, and (b) that the
// only place spelling `false` into dispatchUpdateWithAuthentication is
// dispatchUpdate itself. It does NOT see a call built through a function value
// or an interface. A scan is a discipline aid, not a proof that no other shape
// exists.
//
// Evidence level: source-level, go/parser over this package; no socket, no bot.
// Probing mutation executed: point handleLibraryUpdate back at dispatchUpdate —
// (a) reddens, naming the file and the enclosing function.
func TestIdentity_NoProductionPathDispatchesUnauthenticated(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	scanned := 0
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
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			enclosing := fn.Name.Name
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				switch sel.Sel.Name {
				case "dispatchUpdate":
					t.Errorf("%s: %s calls the UNAUTHENTICATED dispatchUpdate; production doors mint at their own cut",
						fset.Position(call.Pos()), enclosing)
				case "dispatchUpdateWithAuthentication":
					if enclosing == "dispatchUpdate" || len(call.Args) < 3 {
						return true
					}
					if lit, isIdent := call.Args[2].(*ast.Ident); isIdent && lit.Name == "false" {
						t.Errorf("%s: %s dispatches with authentication=false outside dispatchUpdate",
							fset.Position(call.Pos()), enclosing)
					}
				}
				return true
			})
		}
	}
	if scanned == 0 {
		t.Fatal("no production sources scanned; the guard would pass over nothing")
	}
}

// TestIdentity_RefusedIssuanceIsCounted attacks the silent loss. A refused
// issuance DROPS the update, and this adapter returned without touching the
// counter the observability layer alerts on — the message vanished and nothing
// in the process said one had. Discord already counted its own; Telegram did
// not.
//
// Evidence level: in-process, through the real dispatch path with a zero-value
// issuer (nil seal), which is the only way Issue refuses.
// Probing mutation executed: delete a.dropped.Add(1) from the issuance-failure
// branch — the count stays 0 and this mould reddens.
func TestIdentity_RefusedIssuanceIsCounted(t *testing.T) {
	t.Parallel()
	adapter, err := New(
		WithToken("test-token"), WithMode(ModePolling),
		WithIngressIssuer(&identity.Issuer{}), withInjectedBotForTests(stubBotClient{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	before := adapter.DroppedCount()
	adapter.dispatchAuthenticatedUpdate(context.Background(), newTextUpdate(77, 222, "unissuable"))
	if got := adapter.DroppedCount(); got != before+1 {
		t.Fatalf("DroppedCount after a refused issuance = %d, want %d: the loss is invisible", got, before+1)
	}
	select {
	case env := <-adapter.inbound:
		t.Fatalf("an update with no ingress reached the queue: %+v", env)
	default:
	}
}

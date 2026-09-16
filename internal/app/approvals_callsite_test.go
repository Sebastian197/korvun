// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The production call site hands the claim its snapshot — held BY SITE.
//
// The adversary's first finding of 2026-09-16: the whole-row comparison was
// proved at the store's door and nowhere else. Replacing `&approval` with
// `nil` in the ONE production caller left every test in the repository green,
// because no mould watched the site that feeds the comparison — and that site
// is the one path that fires an irreversible effect.
//
// There is no seam to attack behaviourally: the prechecks read the row and the
// claim re-reads it inside its transaction, so a test cannot slip a write
// between them deterministically. The rule is therefore held where it lives,
// at the SITE, by reading this package's own AST — the shape
// TestWebhookCall_everyBranchAfterDo already uses for the delivery frontier.
//
// Evidence level, honest: a structural test over the source, not a behaviour.
// It proves the argument is passed, never that the comparison is right; that
// is TestClaim_refusesARowThatMovedInAnyColumn's job.
package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestExecuteApprovedAction_handsTheClaimItsSnapshot walks the calls to
// ClaimApprovalParamsUnderDigest in this package and requires every one of them
// to pass the approval the prechecks read, as an address-of expression, in the
// last argument.
//
// Probing mutation (executed, red, declared in the canto): change `&approval`
// to `nil` at the call site ⇒ this reddens.
func TestExecuteApprovedAction_handsTheClaimItsSnapshot(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "approvals.go", nil, 0)
	if err != nil {
		t.Fatalf("parse approvals.go: %v", err)
	}
	seen := 0
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "ClaimApprovalParamsUnderDigest" {
			return true
		}
		seen++
		if len(call.Args) != 5 {
			t.Fatalf("%s: the claim takes five arguments, this call passes %d",
				fset.Position(call.Pos()), len(call.Args))
		}
		snapshot, ok := call.Args[4].(*ast.UnaryExpr)
		if !ok || snapshot.Op != token.AND {
			t.Fatalf("%s: the snapshot argument is %T, not the address of the row the prechecks read — "+
				"a nil there disables the whole-row comparison and nothing else would notice",
				fset.Position(call.Pos()), call.Args[4])
		}
		ident, ok := snapshot.X.(*ast.Ident)
		if !ok || ident.Name != "approval" {
			t.Fatalf("%s: the snapshot must be the approval the prechecks read, got &%v",
				fset.Position(call.Pos()), snapshot.X)
		}
		return true
	})
	if seen != 1 {
		t.Fatalf("calls to the claim in this file = %d, want exactly 1 — a second caller needs its own guard", seen)
	}
}

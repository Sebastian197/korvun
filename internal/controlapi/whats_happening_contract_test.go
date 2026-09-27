// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.16.2 · «¿Qué pasa hoy?» — THE CONTRACT MOULD.
//
// The seventh law says no capability is done if the operator cannot see it. A
// law that lives only in a document decays the first time someone adds a switch
// in a hurry. This mould is the law with teeth: it enumerates, from the CODE,
// every dimension that decides what happens to an irreversible action, and
// reddens when the code grows one the screen does not carry.
//
// WHY IT WALKS THE SOURCE, AND WHAT THAT COST TO GET RIGHT. Two passes shaped
// this file and both found the guard weaker than its own prose.
//
// The first version compared `len(policy.EveryToolRule())` against a list of
// `policy.ToolRule` values typed here, claiming a seventh rule «reddens even if
// the author never opened this file». Executed: adding a constant without
// touching the enumeration left every mould green, because both sides were
// hand-maintained lists a hurried author misses together. Two hand-lists
// agreeing proves they agree, not that either is complete. (`EveryToolRule`
// itself is gone now: it had no production caller, only this test, and a public
// symbol whose only reader is a test is a surface with no reason to exist.)
//
// The second version parsed ONE named file for constants of the exact shape
// `T = "literal"`. The official pass executed three forms it skipped in silence —
// a constant in another file of the package, a constant with the type omitted,
// and a value written as a conversion — and all three shipped green. Those are
// idiomatic Go, not contrivances.
//
// So the guard is now by SITE in the sense the word demands: it walks every
// non-test file of the package DIRECTORY, reads the two value forms a rule can
// take, inherits a const block's declared type, and FAILS on any form it cannot
// read rather than skipping it. A filter skips what it does not understand; a
// guard refuses to.
//
// Evidence level, honest: in-process, structural, over the source text of two
// named files. It proves the SCREEN's contract covers the dimensions those files
// declare; it proves nothing about what the screen paints, which is what the
// sibling moulds are for.
//
// Plan: docs/superpowers/specs/2026-09-23-v0162-que-pasa-hoy-pretest.md.

package controlapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// toolRulePackage is the file that declares the capability rules. Named
// once so the failure message can point at it.
const toolRulePackage = "../policy"

// effectGateDeclarations is the file whose `effectGateRule` returns the effect
// rules.
const effectGateDeclarations = "../action/executor/executor.go"

// parkConditionDeclarations is the file that declares the park gate's
// conditions.
const parkConditionPackage = "."

// constStringsOfType parses EVERY non-test Go file of a package directory and
// returns the string values of every constant that belongs to the named type.
//
// WHAT IT READS, AND WHAT IT REFUSES TO GUESS. The official adversarial pass
// executed three forms the first version skipped in SILENCE, and its prose
// claimed all three were covered:
//
//   - a constant in ANOTHER FILE of the same package. The walk read one named
//     file, so `internal/policy/tools_probe.go` was invisible. Cured by walking
//     the directory.
//   - a constant in the same block with the TYPE OMITTED — `ParkNeedsSeventh =
//     "seventh"`. Idiomatic Go, usable as the type by implicit conversion, and
//     `vs.Type` is nil. Cured by inheriting the block's last declared type.
//   - a value that is a CONVERSION rather than a literal — `ToolRule("x")`. Not
//     a `*ast.BasicLit`. Cured by reading `T("lit")`.
//
// And the fourth form nobody has written yet: a spec that belongs to the type and
// whose value this walk CANNOT read FAILS the test, naming the file, the line and
// the form. That is the difference between a guard and a filter — the first
// version skipped what it did not understand, which is the fail-open shape a
// scanner must never have.
//
// Test files are excluded on purpose: a rule declared in a `_test.go` is not a
// dimension of production, and including them would redden on the audit probes
// this very file's history is made of.
func constStringsOfType(t *testing.T, dir, typeName string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("the contract cannot read the package %s: %v", dir, err)
	}
	var files []string
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		files = append(files, filepath.Join(dir, n))
	}
	if len(files) == 0 {
		t.Fatalf("the contract found no non-test Go file in %s: the walk would pass over nothing", dir)
	}
	var out []string
	for _, path := range files {
		out = append(out, constStringsOfTypeInFile(t, path, typeName)...)
	}
	if len(out) == 0 {
		t.Fatalf("the contract found NO %s constant anywhere in %s: the walk came back empty, so every assertion built on it would pass over nothing",
			typeName, dir)
	}
	sort.Strings(out)
	return out
}

func constStringsOfTypeInFile(t *testing.T, path, typeName string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("the contract cannot read %s: %v", path, err)
	}
	var out []string
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		// Inside one const block a spec with no type inherits the reader's
		// attention from the last spec that declared one. Go's own typing rules
		// make such a constant UNTYPED, but an untyped string constant is usable
		// wherever the named type is wanted, so for a guard's purposes it belongs
		// to the type and must be judged.
		blockType := ""
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			if id, ok := vs.Type.(*ast.Ident); ok {
				blockType = id.Name
			}
			if blockType != typeName || len(vs.Values) == 0 {
				continue
			}
			for _, v := range vs.Values {
				s, form := constStringValue(v, typeName)
				if form != "" {
					t.Fatalf("%s:%d declares a %s constant whose value this contract cannot read (%s). A form the guard does not understand must not be SKIPPED: extend the reader, or the next dimension ships without a screen row.",
						path, fset.Position(v.Pos()).Line, typeName, form)
				}
				if s != "" {
					out = append(out, s)
				}
			}
		}
	}
	return out
}

// constStringValue reads the two value forms a rule constant may take. It
// returns the string, or an empty string and a DESCRIPTION of the form it could
// not read — never an empty string and no description, which would be the silent
// skip this guard exists to forbid.
func constStringValue(v ast.Expr, typeName string) (value, unreadable string) {
	switch e := v.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", "a non-string literal"
		}
		// The literal carries its quotes; a rule name that needed escaping
		// would itself be a finding, so the plain trim is deliberate.
		return e.Value[1 : len(e.Value)-1], ""
	case *ast.CallExpr:
		// A conversion: `ToolRule("x")`.
		id, ok := e.Fun.(*ast.Ident)
		if !ok || id.Name != typeName || len(e.Args) != 1 {
			return "", "a call this contract does not recognise as a conversion"
		}
		lit, ok := e.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return "", "a conversion of something that is not a string literal"
		}
		return lit.Value[1 : len(lit.Value)-1], ""
	default:
		return "", "an expression that is neither a literal nor a conversion"
	}
}

// returnedStringsOf parses a file and returns every string literal the named
// function returns. `effectGateRule` decides by returning one of these names, so
// a fourth one added to it appears here by construction.
func returnedStringsOf(t *testing.T, path, funcName string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("the contract cannot read %s: %v", path, err)
	}
	var fn *ast.FuncDecl
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok && fd.Name.Name == funcName {
			fn = fd
			break
		}
	}
	if fn == nil {
		t.Fatalf("the contract cannot find %s in %s: it was renamed or moved, and the guard must follow it", funcName, path)
	}
	var out []string
	ast.Inspect(fn, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for _, r := range ret.Results {
			lit, ok := r.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			if s := lit.Value[1 : len(lit.Value)-1]; s != "" {
				out = append(out, s)
			}
		}
		return true
	})
	if len(out) == 0 {
		t.Fatalf("the contract found NO returned rule name in %s of %s: the walk came back empty", funcName, path)
	}
	sort.Strings(out)
	return out
}

// handPinnedRules are the two dimension names the walk above cannot derive,
// declared as hand-pinned rather than passed off as structural:
//
//   - "effect_undeclared" is `executor.BranchEffectUndeclared`, a Branch — and
//     the other Branch values (unknown, denied, shadowed, pending, executed) are
//     OUTCOMES, not dimensions, so walking that constant block would demand
//     screen rows for five things the operator does not configure;
//   - "require_approval" is written as a literal in
//     internal/action/sqlite/authority_v2.go, where the same string also appears
//     as a column value, so a literal walk there would not distinguish the
//     decision from its record.
//
// Naming them here with their sites is the honest form: the reader can check
// both, and nothing claims a guard that does not exist.
var handPinnedRules = []string{"effect_undeclared", "require_approval"}

// TestContract_everyToolRuleConstantHasAScreenRow walks the DECLARATION FILE, so
// a capability rule added to `policy` reaches this assertion whether or not the
// author remembered `EveryToolRule()` or this test.
//
// PROBING MUTATIONS (four, all executed — mutations.txt M-A…M-D). Each one is a
// form that shipped GREEN under an earlier version of this guard:
//   - the constant added to internal/policy/tools.go with its type and literal;
//   - the same constant in a DIFFERENT file of the package;
//   - the same constant written as `ToolRule("audit_seventh")`;
//   - a value the reader cannot parse at all — which must fail CLOSED, naming
//     the file, the line and the form.
func TestContract_everyToolRuleConstantHasAScreenRow(t *testing.T) {
	for _, rule := range constStringsOfType(t, toolRulePackage, "ToolRule") {
		if _, ok := screenRowForRule(rule); !ok {
			t.Errorf("the capability rule %q is declared in %s and decides what happens to an action, and the screen has no row for it — the seventh law forbids shipping it",
				rule, toolRulePackage)
		}
	}
}

// TestContract_everyEffectGateRuleHasAScreenRow walks `effectGateRule`'s own
// returns, plus the two hand-pinned names declared above.
//
// PROBING MUTATION: add a fourth `return "effect_audit"` branch to
// `effectGateRule`. This reddens naming `effect_audit`.
func TestContract_everyEffectGateRuleHasAScreenRow(t *testing.T) {
	rules := returnedStringsOf(t, effectGateDeclarations, "effectGateRule")
	rules = append(rules, handPinnedRules...)
	for _, rule := range rules {
		if _, ok := screenRowForRule(rule); !ok {
			t.Errorf("the effect rule %q decides what happens to an action and the screen has no row for it", rule)
		}
	}
}

// TestContract_everyParkConditionHasAScreenRow is the join the SCREEN walks when
// it paints a blocked brain. The gate's condition names and the code's rule
// names are different vocabularies — `ceiling` against `effect_ceiling`,
// `governance_denies` against `deny` — so joining them by string equality would
// drop three of the six blockers and leave the operator reading «something
// blocks this brain» with nothing under it.
//
// PROBING MUTATION (executed): add a seventh `ParkCondition` constant to
// internal/controlapi/approvals.go and change nothing else. This reddens naming
// it, because the walk reads the constant block rather than a list typed here.
func TestContract_everyParkConditionHasAScreenRow(t *testing.T) {
	for _, c := range constStringsOfType(t, parkConditionPackage, "ParkCondition") {
		if _, ok := screenRowForCondition(ParkCondition(c)); !ok {
			t.Errorf("the park condition %q blocks a brain from parking and the screen has no row that explains it: the operator would be told something blocks them with nothing under it", c)
		}
	}
}

// TestContract_everyScreenRowNamesItsProfileKey keeps the other half honest: a
// row the operator cannot act on must at least say WHERE to change it. A row
// with neither a button nor a key is a dead end on the screen.
//
// PROBING MUTATION: leave a row's key empty. This reddens naming it.
func TestContract_everyScreenRowNamesItsProfileKey(t *testing.T) {
	for _, row := range ScreenRows() {
		if row.ProfileKey == "" {
			t.Errorf("screen row %q names no profile key: an operator who cannot press a button must still be told where to change it", row.Rule)
		}
		if !row.HasButton && row.Why == "" {
			t.Errorf("screen row %q has no button and no reason: the operator is left with a dead end", row.Rule)
		}
	}
}

// TestContract_everyClaimedButtonHasARealDoor is the mould three rows of the
// contract needed and did not have. `not_granted`, `approval_unavailable` and
// the cage row each shipped `HasButton: true` while no door could make the
// change they pointed at — the screen would have drawn a button that refuses, or
// worse, one that changes something else.
//
// PROBING MUTATION: set `HasButton: true` on the `deny` row without a Door, or
// name a Door no handler mounts. Both redden.
func TestContract_everyClaimedButtonHasARealDoor(t *testing.T) {
	doors := writeDoorNames()
	if len(doors) == 0 {
		t.Fatal("no write doors are declared: the join this mould checks would pass over nothing")
	}
	mounted := map[string]bool{}
	for _, d := range doors {
		mounted[d] = true
	}
	for _, row := range ScreenRows() {
		switch {
		case row.HasButton && row.Door == "":
			t.Errorf("screen row %q claims a button and names no door: the screen would draw a control with nothing behind it", row.Rule)
		case row.HasButton && !mounted[row.Door]:
			t.Errorf("screen row %q claims the door %q, which is not one of the mounted write doors %v", row.Rule, row.Door, doors)
		case !row.HasButton && row.Door != "":
			t.Errorf("screen row %q names the door %q but claims no button: the screen would leave a working door unreachable", row.Rule, row.Door)
		}
	}
}

// TestContract_theHeaderArithmeticIsTrue recomputes the four numbers the
// package comment of internal/controlapi/whats_happening.go states.
//
// It exists because that sentence was FALSE when it was written: it said
// «ELEVEN dimensions», «four of the eleven have a button» and «the other
// seven», over a table holding sixteen rows. Documentary arithmetic not verified
// by execution is known-classes checklist item (h), and prose cannot be trusted
// to survive a row being added.
//
// PROBING MUTATION: add a row to `screenRows`, or flip one row's HasButton. This
// reddens with both numbers printed.
func TestContract_theHeaderArithmeticIsTrue(t *testing.T) {
	// 2026-09-24, the store's row gains its button (the director's decision on
	// the profile with no ledger): six rows with a button, ten as text, five
	// doors. The row count does not move — the row was already there.
	// 2026-09-24, later the same day: the durable mark adds the row of a ledger
	// founded by another profile, with its button «Adoptar libro» — seventeen
	// rows, seven with a button, ten as text, six doors.
	const (
		wantRows      = 17
		wantWithBtn   = 7
		wantTextOnly  = 10
		wantDoorCount = 6
	)
	rows := ScreenRows()
	withButton := 0
	for _, r := range rows {
		if r.HasButton {
			withButton++
		}
	}
	if len(rows) != wantRows {
		t.Errorf("the screen carries %d rows and its package comment says %d", len(rows), wantRows)
	}
	if withButton != wantWithBtn {
		t.Errorf("%d rows have a button and the package comment says %d", withButton, wantWithBtn)
	}
	if textOnly := len(rows) - withButton; textOnly != wantTextOnly {
		t.Errorf("%d rows are text-only and the package comment says %d", textOnly, wantTextOnly)
	}
	if got := len(writeDoorNames()); got != wantDoorCount {
		t.Errorf("there are %d write doors and the package comment says %d", got, wantDoorCount)
	}
	// The RELEASE NOTES repeat all four numbers in Spanish words, and a public
	// document that contradicts the code is a public-truth violation, not a
	// typo. The notes are checked against the SAME counts, so a row added
	// tomorrow reddens here instead of shipping a false sentence to the release
	// page.
	assertNotesCarryTheCounts(t, len(rows), withButton, len(rows)-withButton, wantDoorCount)
}

// spanishCount spells the small numbers these counts can take. It covers the
// range the screen can plausibly reach and FAILS LOUDLY outside it, rather than
// quietly skipping the check — a guard that stops guarding when a number grows
// is worse than none.
func spanishCount(t *testing.T, n int) string {
	t.Helper()
	words := map[int]string{
		4: "cuatro", 5: "cinco", 6: "seis", 7: "siete", 8: "ocho", 9: "nueve",
		10: "diez", 11: "once", 12: "doce", 13: "trece", 14: "catorce",
		15: "quince", 16: "dieciséis", 17: "diecisiete", 18: "dieciocho",
		19: "diecinueve", 20: "veinte",
	}
	w, ok := words[n]
	if !ok {
		t.Fatalf("the notes guard has no Spanish word for %d: extend it rather than leave the release notes unchecked", n)
	}
	return w
}

// assertNotesCarryTheCounts reads the release notes and demands each count in
// words. It matches case-insensitively because a count can open a sentence.
func assertNotesCarryTheCounts(t *testing.T, rows, withButton, textOnly, doors int) {
	t.Helper()
	const notes = "../../docs/releases/v0.16.2.md"
	b, err := os.ReadFile(notes)
	if err != nil {
		t.Fatalf("the contract cannot read %s: %v", notes, err)
	}
	lower := strings.ToLower(string(b))
	for _, c := range []struct {
		what string
		n    int
	}{
		{"rows", rows},
		{"rows with a button", withButton},
		{"text-only rows", textOnly},
		{"write doors", doors},
	} {
		if w := spanishCount(t, c.n); !strings.Contains(lower, w) {
			t.Errorf("%s says %d %s and the release notes do not carry the word %q",
				notes, c.n, c.what, w)
		}
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R13-G5b: the repair document's SQL binds the id with `.param` in the
// form CAPTURED from a real sqlite3 run (A13), and a guard BY SITE keeps
// every SQL statement line free of an interpolated id. Evidence level:
// in-process + the real sqlite3 shell (the CI forbids the skip,
// R12-H10). The doc guard's red lives in mutation m-n5d: an `'<apr>'`
// interpolated into a SQL statement line of the document reddens it.

package sqlite

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const repairDocPath = "../../../docs/operations/tombstone-manual-repair.md"

// A13 — an id carrying a single quote: the document's `.param set`
// form must bind it exactly. CAPTURED on sqlite3 3.39.5 (macOS): the
// single-quoted form with a doubled quote (`'it”s'`) is NOT parsed by
// the dot-command tokenizer — the shell prints the `.parameter` usage,
// exits 0 and leaves @apr UNBOUND (NULL): a silent miss; the
// double-quoted form (`"it's"`) binds the exact bytes. The document
// therefore shows the double-quoted form, and this test bites an id
// with a single quote through it.
func TestRepairDoc_paramBindsAQuotedIdExactly(t *testing.T) {
	t.Parallel()
	sqlite3bin, lookErr := exec.LookPath("sqlite3")
	if lookErr != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("R12-H10: sqlite3 is not on PATH on this CI runner (%v) — never skip under CI", lookErr)
		}
		t.Skip("SKIP NAMED (R12-A9): no sqlite3 binary on this runner")
	}
	hostile := "apr_it's_a_trap"
	out, err := exec.Command(sqlite3bin, ":memory:", `.param set @apr "`+hostile+`"`, "SELECT quote(@apr);").CombinedOutput() //nolint:gosec // G204: the DOCUMENTED operator form
	if err != nil || strings.TrimSpace(string(out)) != "'apr_it''s_a_trap'" {
		t.Fatalf("AUDIT R13-A13: the documented double-quoted .param form must bind the hostile id exactly: %v %q", err, out)
	}
	// The form the document must NOT show: captured behavior, not a
	// claim — the doubled single quote prints usage and binds NULL.
	out, err = exec.Command(sqlite3bin, ":memory:", ".param set @apr 'apr_it''s'", "SELECT quote(@apr);").CombinedOutput() //nolint:gosec // G204: the operator form the document warns about
	if err != nil || !strings.Contains(string(out), "NULL") {
		t.Fatalf("CAPTURE A13: the single-quoted doubled form binds NULL on this sqlite3 (declared in the document): %v %q", err, out)
	}
	doc, err := os.ReadFile(filepath.Clean(repairDocPath))
	if err != nil {
		t.Fatalf("read the document: %v", err)
	}
	// BY SITE: the shell lines of the code fences (`sqlite> .param set`),
	// not the prose that names the form the document warns about.
	shellLines := 0
	for _, line := range strings.Split(string(doc), "\n") {
		if !strings.Contains(line, "sqlite> .param set @apr") {
			continue
		}
		shellLines++
		if !strings.Contains(line, `sqlite> .param set @apr "`) {
			t.Fatalf("AUDIT R13-A13: the document's shell lines must show the double-quoted .param form (the captured one), got %q", line)
		}
	}
	if shellLines < 2 {
		t.Fatalf("the document's .param shell lines were not found (%d) — the document moved or the fences changed", shellLines)
	}
}

// The doc guard BY SITE: every SQL statement line of the document (a
// line inside a code fence that carries `approval_tombstones`) binds
// the id (`@apr`) and never interpolates one (`'apr_…'`). The `.param`
// lines are exempt — they are the ONE place an id is typed, and the
// document says "retype after visual inspection" governs them too.
func TestRepairDoc_sqlStatementsBindTheIdNeverInterpolate(t *testing.T) {
	t.Parallel()
	doc, err := os.ReadFile(filepath.Clean(repairDocPath))
	if err != nil {
		t.Fatalf("read the document: %v", err)
	}
	// BY STATEMENT, not by line: a statement inside a fence runs from its
	// first line to the line ending in `;` (the shell's continuation
	// prompt `...>` joins them), so a WHERE clause on a continuation line
	// is judged with the statement it belongs to.
	interpolated := regexp.MustCompile(`'apr_[^']*'`)
	inFence, sqlStatements := false, 0
	var stmt strings.Builder
	stmtStart := 0
	for n, line := range strings.Split(string(doc), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			stmt.Reset()
			continue
		}
		if !inFence || strings.Contains(line, ".param") || strings.Contains(line, ".backup") || strings.Contains(line, ".dump") {
			continue
		}
		if stmt.Len() == 0 {
			stmtStart = n + 1
		}
		stmt.WriteString(line)
		stmt.WriteString("\n")
		if !strings.HasSuffix(strings.TrimSpace(line), ";") {
			continue
		}
		s := stmt.String()
		stmt.Reset()
		if !strings.Contains(s, "approval_tombstones") {
			continue
		}
		sqlStatements++
		if interpolated.MatchString(s) || strings.Contains(s, "WHERE") && !strings.Contains(s, "@apr") {
			t.Fatalf("AUDIT R13-G5b doc guard, statement at line %d: a SQL statement over approval_tombstones must bind @apr, never interpolate an id: %q", stmtStart, s)
		}
	}
	if sqlStatements < 3 {
		t.Fatalf("the guard must see the document's SQL statements (found %d) — the document moved or the fences changed", sqlStatements)
	}
}

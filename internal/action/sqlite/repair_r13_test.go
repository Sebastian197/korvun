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

// The doc guard BY STATEMENT: every SQL statement inside a code fence
// of the document that names `approval_tombstones` — a shell-prompt
// statement joined across its `...>` continuation lines until `;`, an
// unterminated one judged at the fence close, or the SQL argument of a
// `sqlite3 "<db>" "…"` one-liner — binds the id (`@apr`) and never
// interpolates one (`'apr_…'` or `"apr_…"`). EXEMPT, by site: the
// dot-command lines (`sqlite> .param …`, and a `sqlite3 …` invocation
// whose arguments after the path are all dot-commands — `.dump
// approval_tombstones` is a dot-command, not a statement) — they are
// the ONE place an id is typed, and the document says "retype after
// visual inspection" governs them too — and the bare
// `sqlite3 "<profile>/korvun.db"` invocation (no argument after the
// path). The arguments are tokenized as shell words, never counted.
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
	// An interpolated id in either SQL quoting ('apr_…' or "apr_…" — SQLite
	// reads a double-quoted string as a literal when no such identifier
	// exists); WHERE matched case-insensitively.
	interpolated := regexp.MustCompile(`['"]apr_[^'"]*['"]`)
	whereClause := regexp.MustCompile(`(?i)\bwhere\b`)
	inFence, sqlStatements := false, 0
	var stmt strings.Builder
	stmtStart := 0
	judge := func() {
		s := stmt.String()
		stmt.Reset()
		if !strings.Contains(s, "approval_tombstones") {
			return
		}
		sqlStatements++
		if interpolated.MatchString(s) || whereClause.MatchString(s) && !strings.Contains(s, "@apr") {
			t.Fatalf("AUDIT R13-G5b doc guard, statement at line %d: a SQL statement over approval_tombstones must bind @apr, never interpolate an id: %q", stmtStart, s)
		}
	}
	// Exempt BY SITE: a dot-command — the shell prompt followed by a dot
	// (`sqlite> .param …`) or a bare dot-command — and a shell invocation
	// line whose ARGUMENTS after the database path are all dot-commands
	// (`".backup …"`, `".dump …"`) or absent; any other argument of a
	// `sqlite3 …` line is SQL and IS judged, whatever its quoting (the
	// fourth diff pass caught the prefix-only exemption, the fifth the
	// quote-count rule). The CONTINUATION prompt (`...>`) is NOT a
	// dot-command: it starts with a dot too, and the third diff pass
	// caught the guard skipping every continuation line — the WHERE of a
	// two-line statement — by that resemblance.
	// shellWords splits a shell invocation line into its words, honoring
	// single and double quotes (no escapes beyond `\"` inside double
	// quotes) — the ARGUMENT list is what is judged, never a quote count
	// (the fifth pass: an unquoted path, single-quoted arguments, or a
	// dot-command riding beside the SQL argument all fooled a count).
	shellWords := func(s string) []string {
		var words []string
		var cur strings.Builder
		inWord, quote := false, byte(0)
		for i := 0; i < len(s); i++ {
			c := s[i]
			switch {
			case quote != 0:
				if c == '\\' && quote == '"' && i+1 < len(s) {
					i++
					cur.WriteByte(s[i])
				} else if c == quote {
					quote = 0
				} else {
					cur.WriteByte(c)
				}
			case c == '"' || c == '\'':
				quote, inWord = c, true
			case c == ' ' || c == '\t':
				if inWord {
					words = append(words, cur.String())
					cur.Reset()
					inWord = false
				}
			default:
				cur.WriteByte(c)
				inWord = true
			}
		}
		if inWord {
			words = append(words, cur.String())
		}
		return words
	}
	// A `sqlite3 …` invocation is exempt only when NO argument after the
	// database path is SQL: every further argument is a dot-command.
	invocationSQL := func(line string) (sql string, exempt bool) {
		words := shellWords(strings.TrimSpace(line))
		if len(words) < 2 {
			return "", true
		}
		var parts []string
		for _, w := range words[2:] {
			if strings.HasPrefix(w, ".") {
				continue
			}
			parts = append(parts, w)
		}
		if len(parts) == 0 {
			return "", true
		}
		return strings.Join(parts, "\n"), false
	}
	isExemptSite := func(line string) bool {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "...>") {
			return false
		}
		if strings.HasPrefix(trimmed, "sqlite> .") || strings.HasPrefix(trimmed, ".") {
			return true
		}
		if strings.HasPrefix(trimmed, "sqlite3 ") {
			_, exempt := invocationSQL(trimmed)
			return exempt
		}
		return false
	}
	sqlText := func(line string) string {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "sqlite3 ") {
			sql, _ := invocationSQL(trimmed)
			return sql
		}
		trimmed = strings.TrimPrefix(trimmed, "sqlite> ")
		return strings.TrimPrefix(trimmed, "...>")
	}
	for n, line := range strings.Split(string(doc), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			// A statement left unterminated at the closing fence is
			// judged as it stands — never dropped.
			if inFence && stmt.Len() > 0 {
				judge()
			}
			inFence = !inFence
			continue
		}
		if !inFence || isExemptSite(line) {
			continue
		}
		if stmt.Len() == 0 {
			stmtStart = n + 1
		}
		text := sqlText(line)
		stmt.WriteString(text)
		stmt.WriteString("\n")
		// A one-liner's SQL arguments are a whole statement whatever
		// their terminator; a prompt line ends its statement at `;`.
		if strings.HasPrefix(strings.TrimSpace(line), "sqlite3 ") || strings.HasSuffix(strings.TrimSpace(line), ";") {
			judge()
		}
	}
	if sqlStatements < 3 {
		t.Fatalf("the guard must see the document's SQL statements (found %d) — the document moved or the fences changed", sqlStatements)
	}
}

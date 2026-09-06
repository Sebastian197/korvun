// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R13-G5b: the repair document's SQL binds the id with `.param` in the
// form CAPTURED from a real sqlite3 run (A13), and a guard BY STATEMENT
// keeps every SQL statement the document feeds sqlite3 free of an
// interpolated id and bound by @apr where it selects a row. Evidence level:
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

// The doc guard BY STATEMENT (rewritten after the eighth diff pass —
// the seventh cure's tokenizer and its exemptions kept opening holes).
// What it judges, exactly: every line of the document that is a SQL
// SITE — a shell-prompt line (`sqlite> …`), a continuation line
// (`...>`), or any line carrying the word `sqlite3` — with:
//   - a channel wall FIRST: a site line carrying a heredoc (`<<`), a
//     pipe (`|`), an input redirection (` <file`, `0<file`), `.read`,
//     `-init` or `-cmd` FAILS by name — the guard never sees the SQL
//     those channels carry, so the document may not use them until the
//     guard grows;
//   - an invocation line (`sqlite3 …`) judged WHOLE, no tokenizing, no
//     exemption: wherever a redirection sits, every byte of the line
//     is judged;
//   - prompt lines joined across continuations until `;` (or until a
//     non-site line closes them), the prompts stripped; a dot-command
//     prompt line (`sqlite> .param …`) is the ONE typing site and is
//     exempt — the document says "retype after visual inspection"
//     governs it;
//   - SQL comments of both forms stripped by a token scanner that never
//     enters a literal; then TWO checks: (a) NO interpolated id in ANY
//     statement — a literal beginning `apr_` in either quoting, case-
//     insensitively (`LIKE 'APR_…'` selects too) — and (b) a statement
//     naming `approval_tombstones` or `approval_id` that carries a
//     WHERE must carry the bind `@apr` as a whole TOKEN outside every
//     literal and comment (a literal `'@apr'`, a `@apr_typo` or a `--
//     @apr` never count);
//   - a floor: at least three statements naming the table exist.
//
// NOT judged, declared: an id hidden behind `hex()`/`CAST(X'…')` or a
// view alias without a literal — the guard watches interpolated
// LITERALS and the bind; the document's own text warns against pasting
// ids, and a hex-hidden id is a different act from a paste.
func TestRepairDoc_sqlStatementsBindTheIdNeverInterpolate(t *testing.T) {
	t.Parallel()
	doc, err := os.ReadFile(filepath.Clean(repairDocPath))
	if err != nil {
		t.Fatalf("read the document: %v", err)
	}
	interpolated := regexp.MustCompile(`(?i)['"]apr_[^'"]*['"]`)
	whereClause := regexp.MustCompile(`(?i)\bwhere\b`)
	bindToken := regexp.MustCompile(`(^|[^A-Za-z0-9_@])@apr([^A-Za-z0-9_]|$)`)
	invocation := regexp.MustCompile(`\bsqlite3\b`)
	channelWall := regexp.MustCompile(`<<|\s<\S|\d<|\||\.read\b|-init\b|-cmd\b`)
	// scan strips comments outside literals (both forms) and, when
	// blank is set, also empties every literal's interior — the bind is
	// looked for in the blanked text, the interpolation in the kept one.
	scan := func(s string, blank bool) string {
		var out strings.Builder
		quote := byte(0)
		for i := 0; i < len(s); i++ {
			c := s[i]
			switch {
			case quote != 0:
				if c == quote {
					quote = 0
					out.WriteByte(c)
				} else if !blank {
					out.WriteByte(c)
				}
			case c == '\'' || c == '"':
				quote = c
				out.WriteByte(c)
			case c == '-' && i+1 < len(s) && s[i+1] == '-':
				for i < len(s) && s[i] != '\n' {
					i++
				}
				out.WriteByte('\n')
			case c == '/' && i+1 < len(s) && s[i+1] == '*':
				end := strings.Index(s[i+2:], "*/")
				if end < 0 {
					i = len(s)
				} else {
					i += 2 + end + 1
				}
				out.WriteByte(' ')
			default:
				out.WriteByte(c)
			}
		}
		return out.String()
	}
	tableStatements := 0
	judge := func(startLine int, raw string) {
		kept := scan(raw, false)
		blanked := scan(raw, true)
		if interpolated.MatchString(kept) {
			t.Fatalf("AUDIT R13-G5b doc guard, statement at line %d: an interpolated id — bind it with @apr, never paste it: %q", startLine, raw)
		}
		namesTable := strings.Contains(blanked, "approval_tombstones") || strings.Contains(blanked, "approval_id")
		if strings.Contains(blanked, "approval_tombstones") {
			tableStatements++
		}
		if namesTable && whereClause.MatchString(blanked) && !bindToken.MatchString(blanked) {
			t.Fatalf("AUDIT R13-G5b doc guard, statement at line %d: a WHERE over the tombstones must bind @apr as a whole token outside literals and comments: %q", startLine, raw)
		}
	}
	isPrompt := func(trimmed string) bool {
		return strings.HasPrefix(trimmed, "sqlite> ") || strings.HasPrefix(trimmed, "...>")
	}
	var stmt strings.Builder
	stmtStart := 0
	closeOpen := func() {
		if stmt.Len() > 0 {
			s := stmt.String()
			stmt.Reset()
			judge(stmtStart, s)
		}
	}
	for n, line := range strings.Split(string(doc), "\n") {
		trimmed := strings.TrimSpace(line)
		site := isPrompt(trimmed) || invocation.MatchString(trimmed)
		if !site {
			closeOpen()
			continue
		}
		if channelWall.MatchString(trimmed) {
			t.Fatalf("AUDIT R13-G5b doc guard, line %d: SQL reaching sqlite3 through a heredoc, a pipe, a redirected file, .read, -init or -cmd is a channel this guard does not judge — grow the guard before the document uses it: %q", n+1, trimmed)
		}
		if !isPrompt(trimmed) {
			// An invocation line: judged WHOLE, on its own, at once.
			closeOpen()
			judge(n+1, trimmed)
			continue
		}
		if strings.HasPrefix(trimmed, "sqlite> .") {
			// The dot-command typing site, exempt by site.
			continue
		}
		if stmt.Len() == 0 {
			stmtStart = n + 1
		}
		text := strings.TrimPrefix(strings.TrimPrefix(trimmed, "sqlite> "), "...>")
		stmt.WriteString(text)
		stmt.WriteString("\n")
		if strings.HasSuffix(trimmed, ";") {
			closeOpen()
		}
	}
	closeOpen()
	if tableStatements < 3 {
		t.Fatalf("the guard must see the document's statements over approval_tombstones (found %d) — the document moved or its examples changed", tableStatements)
	}
}

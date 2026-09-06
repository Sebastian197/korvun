// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R13-G5b: the repair document's SQL binds the id with `.param` in the
// form CAPTURED from a real sqlite3 run (A13), and a guard BY STATEMENT
// keeps every SQL statement the document SHOWS at its sites — the
// prompt and the `sqlite3` invocation lines — free of an interpolated
// id and bound by @apr where a WHERE selects over the tombstones; the
// channels it cannot see are refused by the lexer, not by spelling. Evidence level:
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

// The doc guard BY STATEMENT, third form (the ninth diff pass demolished
// the second: the SQL check that demands the bind never ran on an
// invocation line because the shell's quotes were read as SQL literals;
// the channel wall was a spelling list; the dot-command exemption was
// a prefix). What it judges, exactly:
//   - SITES: a prompt line (`sqlite> `, one space), a continuation line
//     (`...>`), and any line carrying the word `sqlite3` (any case, a
//     backslash tolerated: `SQLITE3`, `sqlite\3`) OUTSIDE a markdown
//     code span — a backtick-wrapped mention is prose, not a command.
//     A prompt-LIKE line that is not one of the two exact forms
//     (`sqlite>SELECT`, a tab, `..>`, `....>`) FAILS by name: a
//     malformed prompt would otherwise close the open statement in
//     silence.
//   - An INVOCATION line is read by a shell lexer, not a tokenizer:
//     the shell's quotes are removed and their content kept as SQL; an
//     input redirection or heredoc (`<` outside quotes, wherever it
//     sits, glued or not), a pipe, a command substitution (`$(`, a
//     backtick, inside double quotes too), a variable expansion (`$x`,
//     `${`), an unterminated shell quote, or the `-init` flag FAILS by
//     name — those carry SQL this guard cannot see. The unwrapped text
//     is then judged as one statement.
//   - DOT-COMMANDS, at the prompt or as an argument: a closed
//     allowlist — `.param` (the typing site, exempt from the SQL
//     checks at the prompt), `.backup` and `.dump` (judged like a
//     statement); every other dot-command (`.read`, `.shell`,
//     `.system`, `.import`, `.restore`, `.once`, …) FAILS by name.
//   - Prompt lines are joined across continuations until `;` (or until
//     a non-site line closes them); SQL comments of both forms are
//     stripped by a token scanner that never enters a literal; then
//     TWO checks on every statement: (a) NO interpolated id — a
//     literal beginning `apr_` in either quoting, case-insensitively —
//     and (b) a statement naming `approval_tombstones` or
//     `approval_id` (in the text with literals KEPT, so a quoted
//     identifier counts) that carries a WHERE must carry the bind
//     `@apr` as a whole TOKEN outside every literal and comment.
//   - A floor: at least three statements naming the table exist.
//
// NOT judged, declared: an id hidden behind `hex()`/`CAST(X'…')` in a
// statement WITHOUT a WHERE over the table, a statement without a WHERE
// at all (`DELETE FROM approval_tombstones;` is destruction, not a
// paste — outside this guard's guarantee), a script the document would
// tell the operator to run (`bash repair.sh` names no site — the
// document names none), the word `sqlite3` split by a zero-width or
// fullwidth character, and the bind's PLACE inside the statement
// (`SELECT @apr, * … WHERE hex(…)` satisfies (b)).
func TestRepairDoc_sqlStatementsBindTheIdNeverInterpolate(t *testing.T) {
	t.Parallel()
	doc, err := os.ReadFile(filepath.Clean(repairDocPath))
	if err != nil {
		t.Fatalf("read the document: %v", err)
	}
	interpolated := regexp.MustCompile(`(?i)['"]apr_[^'"]*['"]`)
	whereClause := regexp.MustCompile(`(?i)\bwhere\b`)
	bindToken := regexp.MustCompile(`(^|[^A-Za-z0-9_@])@apr([^A-Za-z0-9_]|$)`)
	siteWord := regexp.MustCompile("(?i)(^|[^`A-Za-z0-9_])sqlite\\\\?3([^`A-Za-z0-9_]|$)")
	promptLike := regexp.MustCompile(`(?i)^(sqlite\s*>|\.{2,}\s*>)`)
	dotCommand := regexp.MustCompile(`(^|\s)\.([A-Za-z]+)`)
	allowedDot := map[string]bool{"param": true, "backup": true, "dump": true}
	initFlag := regexp.MustCompile(`(^|\s)-init\b`)
	isShellWord := func(c byte) bool {
		return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
	}
	// unwrap reads an invocation line as bash would: quotes removed,
	// their content kept; a channel the guard cannot see named.
	unwrap := func(s string) (string, string) {
		var out strings.Builder
		q := byte(0)
		for i := 0; i < len(s); i++ {
			c := s[i]
			switch {
			case q == '\'':
				if c == '\'' {
					q = 0
				} else {
					out.WriteByte(c)
				}
			case c == '`':
				return "", "a command substitution (backtick)"
			case c == '$' && i+1 < len(s) && (s[i+1] == '(' || s[i+1] == '{' || s[i+1] == '\'' || isShellWord(s[i+1])):
				return "", "a command substitution or a variable expansion"
			case c == '\\' && i+1 < len(s):
				if q == '"' && !strings.ContainsRune("\"\\$`", rune(s[i+1])) {
					out.WriteByte(c)
					continue
				}
				i++
				out.WriteByte(s[i])
			case q == '"':
				if c == '"' {
					q = 0
				} else {
					out.WriteByte(c)
				}
			case c == '\'' || c == '"':
				q = c
			case c == '<':
				return "", "an input redirection or a heredoc"
			case c == '|':
				return "", "a pipe"
			default:
				out.WriteByte(c)
			}
		}
		if q != 0 {
			return "", "an unterminated shell quote"
		}
		return out.String(), ""
	}
	// scan strips SQL comments outside literals (both forms) and, when
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
		// The dot-command allowlist FIRST: a channel is refused for what
		// it is, before anything it carries is read.
		for _, m := range dotCommand.FindAllStringSubmatch(blanked, -1) {
			if !allowedDot[strings.ToLower(m[2])] {
				t.Fatalf("AUDIT R13-G5b doc guard, line %d: the dot-command .%s is not one this guard judges (.param, .backup, .dump) — grow the guard before the document uses it: %q", startLine, m[2], raw)
			}
		}
		if interpolated.MatchString(kept) {
			t.Fatalf("AUDIT R13-G5b doc guard, statement at line %d: an interpolated id — bind it with @apr, never paste it: %q", startLine, raw)
		}
		namesTable := strings.Contains(kept, "approval_tombstones") || strings.Contains(kept, "approval_id")
		if strings.Contains(kept, "approval_tombstones") {
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
		if promptLike.MatchString(trimmed) && !isPrompt(trimmed) {
			t.Fatalf("AUDIT R13-G5b doc guard, line %d: a malformed prompt (`sqlite> ` with one space and `...>` are the forms) would close the open statement in silence: %q", n+1, trimmed)
		}
		if !isPrompt(trimmed) {
			closeOpen()
			if !siteWord.MatchString(trimmed) {
				continue
			}
			// An invocation line: read by the shell lexer, judged WHOLE.
			text, channel := unwrap(trimmed)
			if channel != "" {
				t.Fatalf("AUDIT R13-G5b doc guard, line %d: %s carries SQL this guard does not see — grow the guard before the document uses it: %q", n+1, channel, trimmed)
			}
			if initFlag.MatchString(text) {
				t.Fatalf("AUDIT R13-G5b doc guard, line %d: -init reads SQL from a file this guard does not see: %q", n+1, trimmed)
			}
			judge(n+1, text)
			continue
		}
		text := strings.TrimPrefix(strings.TrimPrefix(trimmed, "sqlite> "), "...>")
		if strings.HasPrefix(strings.TrimSpace(text), ".") {
			// A dot-command at the prompt: the allowlist, .param exempt.
			closeOpen()
			text = strings.TrimSpace(text)
			if strings.HasPrefix(text, ".param ") {
				continue
			}
			judge(n+1, text)
			continue
		}
		if stmt.Len() == 0 {
			stmtStart = n + 1
		}
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

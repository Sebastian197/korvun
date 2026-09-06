// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R13-G5b: the repair document's SQL binds the id with `.param` in the
// form CAPTURED from a real sqlite3 run (A13), and a guard BY STATEMENT
// keeps every SQL statement the document SHOWS at its sites — the
// prompt and the `sqlite3` invocation lines — free of an interpolated
// id and bound by @apr where a WHERE selects over the tombstones —
// statement by statement, each argument and each `;`-separated piece
// judged alone; the channels it cannot see are refused by a shell lexer
// for what they are, and by NAME for exactly two: the `-init`/`--init`
// flag and the closed dot-command allowlist. Evidence level:
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

// The doc guard BY STATEMENT, fifth form (the eleventh diff pass found
// the fourth deciding the SITE by a text regexp before the shell's
// quote removal, judging a line-GROUP where one bind masked a second
// unbound WHERE, reading the table name case-sensitively, and joining
// `\` continuations only forward). What it judges, exactly:
//   - LINES: a prompt line (`sqlite> `, one space) or a continuation
//     line (`...>`) is SQL typed at the shell; every other line ending
//     in `\` is FIRST joined with the next line(s) (an over-join:
//     `\ `, `\\` and a `\` inside single quotes join too — the joined
//     text is only ever MORE judged, never less); a prompt-LIKE line
//     that is not one of the two exact forms FAILS by name.
//   - SITES are decided AFTER the shell lexer, never by the raw text:
//     the text outside the line's inline code spans is read by the
//     lexer and, if its quote-removed words name `sqlite3` (any case;
//     `sqli\te3`, `sqlite”3`, `sqlite"3"` are the same word to bash
//     and to the lexer), the WHOLE raw line is a command line — a
//     backtick there is bash's substitution; otherwise, if any code
//     span's quote-removed text names `sqlite3`, EVERY span on the
//     line is read as a command line (a span beside it carrying the
//     SQL argument is judged too). A line that names it nowhere is
//     prose.
//   - The LEXER reads a command line as bash would: quotes removed,
//     their content kept; unquoted whitespace is an ARGUMENT boundary
//     (sqlite3 runs each argument as its own SQL); an input redirection
//     or heredoc (`<` outside quotes, wherever it sits), a pipe, a
//     command substitution (`$(`, a backtick, inside double quotes
//     too), any other `$` followed by a non-blank (`$'…'`, `$"…"`, a
//     parameter expansion), an unterminated shell quote, or the
//     `-init`/`--init` flag FAILS by name — those carry SQL this guard
//     cannot see. Prose naming `sqlite3` outside a code span is read the
//     same way: an apostrophe there reddens as an unterminated quote —
//     put the mention in a code span.
//   - STATEMENTS: the lexer's text and a prompt group are split at
//     every `;` outside a SQL literal and at every argument boundary;
//     each piece is judged ALONE, so a bound decoy cannot mask an
//     unbound WHERE beside it. On each piece, in this order: the
//     dot-command allowlist — `.param`, `.backup`, `.dump` pass, every
//     other dot-command (`.read`, `.shell`, `.system`, `.import`,
//     `.restore`, `.parameter`, …) FAILS by name; the `sqlite> .param`
//     prompt line is the ONE exempt typing site — a `.param` argument
//     on a command line is judged like any piece; then (a) NO
//     interpolated id — a literal beginning `apr_` in either quoting,
//     case-insensitively — and (b) a piece naming `approval_tombstones`
//     or `approval_id` (case-insensitively, literals KEPT so a quoted
//     identifier counts) that carries a WHERE must carry the bind
//     `@apr` as a whole TOKEN outside every literal and comment.
//   - A floor: at least three pieces naming the table exist.
//
// NOT judged, declared: an id hidden behind `hex()`/`CAST(X'…')` in a
// piece WITHOUT a WHERE over the table, a piece without a WHERE at all
// (`DELETE FROM approval_tombstones;` is destruction, not a paste —
// outside this guard's guarantee), a script or a feeder the document
// would tell the operator to run (`bash repair.sh`, `xargs -a
// repair.sql sqlite3 …`, `make repair` — the document names none), the
// word `sqlite3` split by a zero-width or fullwidth character, the
// bind's PLACE inside ONE piece (`SELECT @apr, * … WHERE hex(…)`
// satisfies (b)), an odd apostrophe inside a quoted argument BEFORE the
// SQL (`"/Users/it's/korvun.db"`: after unwrapping, the SQL scanner
// reads it as a literal's opening — the document's path is the
// placeholder `<profile>`, never a real one), and a `;` inside a SQL
// comment (it splits the piece there: over-splitting, never under). A
// fence info string ```` ```sqlite3 ```` or a double-backtick span would
// redden as a substitution: markdown this guard does not read.
func TestRepairDoc_sqlStatementsBindTheIdNeverInterpolate(t *testing.T) {
	t.Parallel()
	doc, err := os.ReadFile(filepath.Clean(repairDocPath))
	if err != nil {
		t.Fatalf("read the document: %v", err)
	}
	interpolated := regexp.MustCompile(`(?i)['"]apr_[^'"]*['"]`)
	whereClause := regexp.MustCompile(`(?i)\bwhere\b`)
	bindToken := regexp.MustCompile(`(^|[^A-Za-z0-9_@])@apr([^A-Za-z0-9_]|$)`)
	siteWord := regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_])sqlite3([^A-Za-z0-9_]|$)`)
	promptLike := regexp.MustCompile(`(?i)^(sqlite\s*>|\.{2,}\s*>)`)
	dotCommand := regexp.MustCompile(`(^|\s)\.([A-Za-z]+)`)
	allowedDot := map[string]bool{"param": true, "backup": true, "dump": true}
	initFlag := regexp.MustCompile(`(^|\s)--?init\b`)
	codeSpan := regexp.MustCompile("`[^`]*`")
	// unwrap reads a command line as bash would: quotes removed, their
	// content kept, an argument boundary marked by NUL; the FIRST
	// channel the guard cannot see is named, and the reading goes on
	// to the end so the site can be decided on the quote-removed words.
	unwrap := func(s string) (string, string) {
		var out strings.Builder
		q := byte(0)
		channel := ""
		note := func(c string) {
			if channel == "" {
				channel = c
			}
		}
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
				note("a command substitution")
			case c == '$' && i+1 < len(s) && s[i+1] != ' ' && s[i+1] != '\t':
				switch s[i+1] {
				case '(':
					note("a command substitution")
				case '\'':
					note("an ANSI-C quoted string")
				case '"':
					note("a locale-translated string")
				default:
					note("a parameter expansion")
				}
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
				note("an input redirection or a heredoc")
			case c == '|':
				note("a pipe")
			case c == ' ' || c == '\t':
				out.WriteByte(0)
			default:
				out.WriteByte(c)
			}
		}
		if q != 0 {
			note("an unterminated shell quote")
		}
		return out.String(), channel
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
	// pieces splits at every `;` outside a SQL literal and at every
	// argument boundary (NUL): each piece is one statement to judge.
	pieces := func(s string) []string {
		var parts []string
		var cur strings.Builder
		q := byte(0)
		for i := 0; i < len(s); i++ {
			c := s[i]
			switch {
			case q != 0:
				cur.WriteByte(c)
				if c == q {
					q = 0
				}
			case c == '\'' || c == '"':
				q = c
				cur.WriteByte(c)
			case c == ';' || c == 0:
				parts = append(parts, cur.String())
				cur.Reset()
			default:
				cur.WriteByte(c)
			}
		}
		return append(parts, cur.String())
	}
	tableStatements := 0
	judgeOne := func(startLine int, raw string) {
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
		lower := strings.ToLower(kept)
		namesTable := strings.Contains(lower, "approval_tombstones") || strings.Contains(lower, "approval_id")
		if strings.Contains(lower, "approval_tombstones") {
			tableStatements++
		}
		if namesTable && whereClause.MatchString(blanked) && !bindToken.MatchString(blanked) {
			t.Fatalf("AUDIT R13-G5b doc guard, statement at line %d: a WHERE over the tombstones must bind @apr as a whole token outside literals and comments: %q", startLine, raw)
		}
	}
	judge := func(startLine int, text string) {
		for _, p := range pieces(text) {
			if strings.TrimSpace(p) != "" {
				judgeOne(startLine, p)
			}
		}
	}
	judgeCommand := func(n int, cmd string) {
		text, channel := unwrap(cmd)
		if channel != "" {
			t.Fatalf("AUDIT R13-G5b doc guard, line %d: read as a sqlite3 command line, %s carries SQL this guard does not see — a prose mention belongs in a code span; a real channel needs the guard to grow first: %q", n, channel, cmd)
		}
		if initFlag.MatchString(strings.ReplaceAll(text, "\x00", " ")) {
			t.Fatalf("AUDIT R13-G5b doc guard, line %d: -init reads SQL from a file this guard does not see: %q", n, cmd)
		}
		judge(n, text)
	}
	namesSQLite := func(s string) bool {
		text, _ := unwrap(s)
		return siteWord.MatchString(text)
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
	lines := strings.Split(string(doc), "\n")
	for n := 0; n < len(lines); n++ {
		trimmed := strings.TrimSpace(lines[n])
		if promptLike.MatchString(trimmed) && !isPrompt(trimmed) {
			t.Fatalf("AUDIT R13-G5b doc guard, line %d: a malformed prompt (`sqlite> ` with one space and `...>` are the forms) would close the open statement in silence: %q", n+1, trimmed)
		}
		if !isPrompt(trimmed) {
			closeOpen()
			// `\` continuations joined BEFORE the site is decided, in
			// both directions (the channel may sit on the first physical
			// line and the word on the second).
			start := n + 1
			for strings.HasSuffix(trimmed, `\`) && n+1 < len(lines) {
				n++
				trimmed = strings.TrimSuffix(trimmed, `\`) + " " + strings.TrimSpace(lines[n])
			}
			if namesSQLite(codeSpan.ReplaceAllString(trimmed, " ")) {
				judgeCommand(start, trimmed)
				continue
			}
			spans := codeSpan.FindAllString(trimmed, -1)
			site := false
			for _, span := range spans {
				if namesSQLite(span[1 : len(span)-1]) {
					site = true
				}
			}
			if site {
				for _, span := range spans {
					judgeCommand(start, span[1:len(span)-1])
				}
			}
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

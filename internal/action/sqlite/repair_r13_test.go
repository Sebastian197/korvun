// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R13-G5b: the repair document's SQL binds the id with `.param` in the
// form CAPTURED from a real sqlite3 run (A13), and a guard BY STATEMENT
// keeps every SQL statement the document SHOWS at its sites — the
// prompt and the `sqlite3` invocation lines — free of an interpolated
// id and bound by @apr where a WHERE selects over the tombstones —
// statement by statement, each argument and each `;`-separated piece
// judged alone; whatever bash could rewrite or feed before sqlite3 sees
// it (an expansion, a substitution, a backslash, a redirection, a pipe,
// a glob, a brace, a continuation, a file-reading flag, an unknown
// dot-command) is REFUSED by name, never emulated. Evidence level:
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
// single-quoted form with a doubled quote inside (`it`, two single
// quotes, `s`, the SQL way of escaping one) is NOT parsed by
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

// The doc guard BY STATEMENT, seventh form. The thirteenth diff pass
// found the sixth still EMULATING bash — a `\` join that inserted a
// space bash does not insert, a `$name` consumer that read a positional
// parameter as a name, brace and glob expansions never modelled — and
// every emulation had opened the next hole. The seventh form emulates
// nothing it does not have to: whatever bash could rewrite before
// sqlite3 sees it is REFUSED by name, and the document is held to the
// forms it actually uses. What it judges, exactly:
//   - MARKDOWN, read structurally: a line starting with ``` or ~~~
//     toggles a fence and is skipped; outside a fence, backticks open
//     and close inline code spans and a span may continue across lines
//     (the running state carries); inside a fence there are no spans —
//     a backtick there is bash's.
//   - PROMPT lines (`sqlite> `, one space) and continuation lines
//     (`...>`) are SQL typed at the shell; a prompt-LIKE line that is
//     not one of the two exact forms FAILS by name.
//   - EVERY other line: one ending in `\` FAILS by name (a line
//     continuation is not a form this document may use — no join is
//     emulated); a `$` in its text outside code spans FAILS by name (a
//     shell expansion is not read, wherever it sits — a prose dollar
//     goes in a code span; `S=sqlite3` on one line and `$S …` on the
//     next is refused at the `$`).
//   - SITES: a line whose text outside its code spans carries the stem
//     `sqlite` (any case, as a substring — `$1sqlite3`, `sqlite{3..3}`,
//     `sqlite[3]`, `SQLite's` all carry it), raw or after the shell
//     lexer's quote removal (`sqlite"3"`, two single quotes inside the
//     word), is read as a
//     COMMAND LINE; prose naming SQLite outside a code span is read the
//     same way, and an apostrophe there reddens as an unterminated
//     quote — put the mention in a code span. A line whose spans carry
//     the stem has EVERY span read as a command line.
//   - The LEXER reads a command line as bash would read its quotes:
//     quotes removed, their content kept; unquoted whitespace is an
//     ARGUMENT boundary (sqlite3 runs each argument as its own SQL).
//     It REFUSES by name, outside single quotes: `$` (a shell
//     expansion), a backtick (a command substitution), a backslash (an
//     escape or a continuation), and, outside all quotes: `<` (an input
//     redirection or a heredoc), `|` (a pipe), and any of `{ } [ ] * ?
//     ~ & !` (a brace, a glob, a tilde, a background or a history
//     expansion); an unterminated shell quote; and the `-init`/`--init`
//     flag. The document's command lines use none of these; anything
//     the lexer does not model is refused, never guessed.
//   - STATEMENTS: the lexer's text is split at every argument
//     boundary, each argument (and a prompt group) has its SQL comments
//     stripped, and the result is split at every `;` outside a SQL
//     literal; each piece is judged ALONE. A prompt group closes when
//     the LINE's comment-stripped text ends in `;` (the group's own
//     comments are stripped again when it is split). On each piece, in
//     this order: the dot-command allowlist — `.param`, `.backup`,
//     `.dump` pass, every other dot-command FAILS by name; the
//     `sqlite> .param set @apr "…"` prompt line, in EXACTLY that form,
//     is the ONE exempt typing site; then (a) NO interpolated id — a
//     literal beginning `apr_` in either quoting, case-insensitively —
//     and (b) a piece naming `approval_tombstones` or `approval_id`
//     (case-insensitively, literals KEPT so a quoted identifier counts)
//     that carries a WHERE must carry the bind `@apr` as a whole TOKEN
//     outside every literal and comment.
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
// placeholder `<profile>`, never a real one), SQL given to sqlite3 as
// UNQUOTED words (each word its own argument, not a working
// invocation), and a zsh-only spelling bash does not share. A fence
// left unclosed makes every later line a fence line (prose there is
// read as shell).
func TestRepairDoc_sqlStatementsBindTheIdNeverInterpolate(t *testing.T) {
	t.Parallel()
	doc, err := os.ReadFile(filepath.Clean(repairDocPath))
	if err != nil {
		t.Fatalf("read the document: %v", err)
	}
	interpolated := regexp.MustCompile(`(?i)['"]apr_[^'"]*['"]`)
	whereClause := regexp.MustCompile(`(?i)\bwhere\b`)
	bindToken := regexp.MustCompile(`(^|[^A-Za-z0-9_@])@apr([^A-Za-z0-9_]|$)`)
	stem := regexp.MustCompile(`(?i)sqlite`)
	promptLike := regexp.MustCompile(`(?i)^(sqlite\s*>|\.{2,}\s*>)`)
	dotCommand := regexp.MustCompile(`(^|\s)\.([A-Za-z]+)`)
	allowedDot := map[string]bool{"param": true, "backup": true, "dump": true}
	initFlag := regexp.MustCompile(`(^|\s)--?init\b`)
	paramSite := regexp.MustCompile(`^\.param set @apr "[^"]*"$`)
	// unwrap reads a command line's quotes as bash would: quotes
	// removed, their content kept, an argument boundary marked by NUL;
	// the FIRST thing bash could rewrite or feed that the guard does not
	// model is named, and the reading goes on to the end so the site
	// can be decided on the quote-removed words.
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
			case c == '$':
				note("a shell expansion")
			case c == '`':
				note("a command substitution")
			case c == '\\':
				note("a backslash (an escape or a continuation)")
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
			case strings.IndexByte("{}[]*?~&!", c) >= 0:
				note("a shell character `" + string(c) + "` (a brace, a glob, a tilde, a background or a history expansion)")
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
	// pieces splits at every argument boundary (NUL), strips each
	// argument's SQL comments, then splits at every `;` outside a SQL
	// literal: each piece is one statement to judge, and a `;` inside a
	// comment splits nothing.
	pieces := func(s string) []string {
		var parts []string
		for _, arg := range strings.Split(s, "\x00") {
			arg = scan(arg, false)
			var cur strings.Builder
			q := byte(0)
			for i := 0; i < len(arg); i++ {
				c := arg[i]
				switch {
				case q != 0:
					cur.WriteByte(c)
					if c == q {
						q = 0
					}
				case c == '\'' || c == '"':
					q = c
					cur.WriteByte(c)
				case c == ';':
					parts = append(parts, cur.String())
					cur.Reset()
				default:
					cur.WriteByte(c)
				}
			}
			parts = append(parts, cur.String())
		}
		return parts
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
		if stem.MatchString(s) {
			return true
		}
		text, _ := unwrap(s)
		return stem.MatchString(text)
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
	inFence, inSpan := false, false
	for n, line := range strings.Split(string(doc), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			closeOpen()
			inFence = !inFence
			continue
		}
		if promptLike.MatchString(trimmed) && !isPrompt(trimmed) {
			t.Fatalf("AUDIT R13-G5b doc guard, line %d: a malformed prompt (`sqlite> ` with one space and `...>` are the forms) would close the open statement in silence: %q", n+1, trimmed)
		}
		if !isPrompt(trimmed) {
			closeOpen()
			if strings.HasSuffix(trimmed, `\`) {
				t.Fatalf("AUDIT R13-G5b doc guard, line %d: a `\\` line continuation is not a form this document may use — no join is emulated: %q", n+1, trimmed)
			}
			// The markdown split: the text outside code spans, and the
			// span segments on this line (a span may carry across lines).
			var outside, cur strings.Builder
			var spans []string
			if inFence {
				outside.WriteString(trimmed)
			} else {
				for i := 0; i < len(trimmed); i++ {
					c := trimmed[i]
					if c == '`' {
						if inSpan {
							spans = append(spans, cur.String())
							cur.Reset()
						}
						inSpan = !inSpan
						continue
					}
					if inSpan {
						cur.WriteByte(c)
					} else {
						outside.WriteByte(c)
					}
				}
				if inSpan {
					spans = append(spans, cur.String())
				}
			}
			if strings.Contains(outside.String(), "$") {
				t.Fatalf("AUDIT R13-G5b doc guard, line %d: a `$` outside a code span is a shell expansion this guard does not read, wherever it sits: %q", n+1, trimmed)
			}
			if namesSQLite(outside.String()) {
				judgeCommand(n+1, outside.String())
				continue
			}
			site := false
			for _, span := range spans {
				if namesSQLite(span) {
					site = true
				}
			}
			if site {
				for _, span := range spans {
					judgeCommand(n+1, span)
				}
			}
			continue
		}
		text := strings.TrimPrefix(strings.TrimPrefix(trimmed, "sqlite> "), "...>")
		if strings.HasPrefix(strings.TrimSpace(text), ".") {
			// A dot-command at the prompt: the allowlist, the exact
			// typing site exempt.
			closeOpen()
			text = strings.TrimSpace(text)
			if paramSite.MatchString(text) {
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
		if strings.HasSuffix(strings.TrimSpace(scan(text, false)), ";") {
			closeOpen()
		}
	}
	closeOpen()
	if tableStatements < 3 {
		t.Fatalf("the guard must see the document's statements over approval_tombstones (found %d) — the document moved or its examples changed", tableStatements)
	}
}

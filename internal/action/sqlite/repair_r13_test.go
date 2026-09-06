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
//   - MARKDOWN, read structurally: a fence is a COMMAND context —
//     every line inside it is read by the shell lexer, no stem asked
//     for, so a fence carries pasteable commands and nothing else (a
//     placeholder inside one is quoted, as a real command must quote
//     it). A line starting with ``` or ~~~ toggles a fence and is
//     skipped; outside a fence, backticks open and close inline code
//     spans and a span may continue onto the NEXT line only — a span
//     still open at the end of its second line, open when a fence
//     starts, open when a PROMPT line falls inside it, or open at the
//     end of the document FAILS by name (an
//     unbalanced backtick would otherwise invert what is span and what
//     is prose for every line after it), and its two halves are JOINED
//     into one command, as the reader sees it; inside a fence there
//     are no spans — a backtick there is bash's.
//   - PROMPT lines (`sqlite> `, one space) and continuation lines
//     (`...>`) are SQL typed at the shell; a prompt-LIKE line that is
//     not one of the two exact forms FAILS by name.
//   - EVERY other line: one ending in `\` FAILS by name (a line
//     continuation is not a form this document may use — no join is
//     emulated); a `$` ANYWHERE on it — in prose, in a code span, in a
//     fence — FAILS by name (a shell expansion is not read, wherever
//     it sits and whatever the span state; `S=sqlite3` on one line and
//     `$S …` on the next is refused at the `$`).
//   - THE PAYLOAD, on every such line whatever it is: an interpolated
//     id anywhere in its RAW text — the `apr_` prefix is a production
//     invariant, not a habit of this document: `NewApprovalID` in
//     `internal/action/approval.go` returns `"apr_"` and sixteen random
//     bytes in hex — or a WHERE over the tombstones without the bind
//     (the NAME read with the literals kept, so a quoted identifier
//     counts; the WHERE and the bind read blanked, so neither a literal
//     nor a comment can carry them), FAILS by name. Its COST, declared:
//     this document can no longer quote an `apr_` id ANYWHERE — not in
//     prose, not in a captured output block, not in a SQL comment —
//     except at the two exempt typing sites. This check asks nothing about the command word —
//     it is the guarantee itself, and it stands whether the line names
//     `sqlite3`, `sq{l..l}ite3`, `/usr/bin/sq?ite3`, a name bound by
//     `hash -p` or `alias`, or nothing at all. The command judgment
//     below adds the CHANNELS (what the guard cannot see) on top.
//   - SITES: a line whose text outside its code spans carries the stem
//     `sqlite` (any case, as a substring — `$1sqlite3`, `sqlite{3..3}`,
//     `sqlite[3]`, `SQLite's` all carry it), raw or after the shell
//     lexer's quote removal (`sqlite"3"`, two single quotes inside the
//     word), is read as a COMMAND LINE — the WHOLE raw line, backticks
//     included: on a command line a backtick is bash's substitution,
//     not markdown, wherever the line sits (a fence, an indented code
//     block, prose); prose naming SQLite outside a code span is read
//     the same way, and an apostrophe or a backtick there reddens —
//     put the whole mention in a code span or drop the name. A line
//     whose spans carry the stem has EVERY span read as a command line.
//   - The LEXER reads a command line as bash would read its quotes:
//     quotes removed, their content kept; unquoted whitespace is an
//     ARGUMENT boundary (sqlite3 runs each argument as its own SQL).
//     It REFUSES by name, outside single quotes: a backtick (a command
//     substitution) and a backslash (an escape or a continuation) —
//     a `$` never reaches it, the line-level wall above having refused
//     the line first, so the lexer carries no dead branch for one —
//     and, outside all quotes: `<` (an input
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
//     `sqlite> .param set @apr "…"` prompt line, in EXACTLY that form
//     and at the `sqlite> ` prompt only, is the ONE exempt typing site
//     — every other prompt line is judged on its RAW text too, so a
//     SQL comment cannot carry a quoted id past the strip; then (a) NO interpolated id — a
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
// invocation), and a zsh-only spelling bash does not share. SCOPE of
// the channel walls, declared: they read COMMAND lines — every line of
// a fence, and outside one a line whose visible text names the stem
// (or, failing that, its code spans when one of them does). On a prose
// line whose command word is disguised so that no stem survives
// (`sq{l..l}ite3`, `/usr/bin/sq?ite3`, a name bound by `hash -p` or
// `alias`), the guard judges the PAYLOAD — which is the guarantee —
// and does not read that line's channels. A fence
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
	inFence, inSpan, spanOpenedAt, carry := false, false, 0, ""
	for n, line := range strings.Split(string(doc), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			closeOpen()
			if inSpan {
				t.Fatalf("AUDIT R13-G5b doc guard, line %d: a fence starts while an inline code span opened on line %d is still open (an unbalanced backtick)", n+1, spanOpenedAt+1)
			}
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
			if strings.Contains(trimmed, "$") {
				t.Fatalf("AUDIT R13-G5b doc guard, line %d: a `$` on a line that is not a prompt line is a shell expansion this guard does not read, wherever it sits: %q", n+1, trimmed)
			}
			// THE PAYLOAD, judged on EVERY line whatever it is: an
			// interpolated id anywhere in its raw text, or a WHERE over
			// the tombstones without the bind in its comment-stripped,
			// literal-blanked text. This check asks nothing about the
			// command word — it is the guarantee itself, and it holds
			// for prose, fences and spans alike.
			if interpolated.MatchString(trimmed) {
				t.Fatalf("AUDIT R13-G5b doc guard, line %d: an interpolated id — bind it with @apr, never paste it: %q", n+1, trimmed)
			}
			if payload := scan(trimmed, true); whereClause.MatchString(payload) && !bindToken.MatchString(payload) {
				// The NAME is looked for with the literals KEPT (a quoted
				// identifier is a name, not a value — the seventeenth
				// pass: `"approval_tombstones"` had been blanked away);
				// the WHERE and the bind are looked for blanked, so
				// neither a literal nor a comment can carry them.
				lower := strings.ToLower(scan(trimmed, false))
				if strings.Contains(lower, "approval_tombstones") || strings.Contains(lower, "approval_id") {
					t.Fatalf("AUDIT R13-G5b doc guard, line %d: a WHERE over the tombstones must bind @apr as a whole token outside literals and comments: %q", n+1, trimmed)
				}
			}
			// The markdown split: the text outside code spans, and the
			// COMPLETE spans (a span may carry onto the next line only;
			// its halves are joined, as the reader sees them).
			var outside, cur strings.Builder
			var spans []string
			var spanLines []int
			if inFence {
				outside.WriteString(trimmed)
			} else {
				for i := 0; i < len(trimmed); i++ {
					c := trimmed[i]
					if c == '`' {
						if inSpan {
							spans = append(spans, carry+cur.String())
							spanLines = append(spanLines, spanOpenedAt+1)
							carry = ""
							cur.Reset()
						} else {
							spanOpenedAt = n
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
					if n > spanOpenedAt {
						t.Fatalf("AUDIT R13-G5b doc guard, line %d: an inline code span opened on line %d is still open at the end of this line — an unbalanced backtick would invert span and prose for every line after it: %q", n+1, spanOpenedAt+1, trimmed)
					}
					// Carried onto the next line, where it closes: the
					// reader sees ONE span, so the guard judges one (the
					// line ending folds to a space, as CommonMark folds it).
					carry = cur.String() + " "
				}
			}
			if inFence {
				// A fence is a COMMAND context: every line in it is a
				// command the reader may paste, so every line is read by
				// the lexer — no stem is asked for (the sixteenth pass:
				// a disguised command word is still a command).
				judgeCommand(n+1, trimmed)
				continue
			}
			if namesSQLite(outside.String()) {
				// A command line: read RAW, spans and all — a backtick on
				// it is bash's substitution, not markdown (the fourteenth
				// pass: an indented code block is runnable, and its spans
				// had been split off before the lexer could refuse them).
				judgeCommand(n+1, trimmed)
				continue
			}
			site := false
			for _, span := range spans {
				if namesSQLite(span) {
					site = true
				}
			}
			if site {
				for i, span := range spans {
					judgeCommand(spanLines[i], span)
				}
			}
			continue
		}
		if inSpan {
			t.Fatalf("AUDIT R13-G5b doc guard, line %d: a prompt line falls inside an inline code span opened on line %d — an unbalanced backtick would make one command out of lines the reader never sees together: %q", n+1, spanOpenedAt+1, trimmed)
		}
		text := strings.TrimPrefix(strings.TrimPrefix(trimmed, "sqlite> "), "...>")
		dot := strings.TrimSpace(text)
		if strings.HasPrefix(trimmed, "sqlite> ") && paramSite.MatchString(dot) {
			// The ONE exempt typing site, at the `sqlite> ` prompt only
			// (the eighteenth pass: a `...>` continuation had been
			// exempted too, which no sqlite3 session can produce).
			closeOpen()
			continue
		}
		// THE PAYLOAD's interpolation check on the RAW prompt line: a
		// SQL comment is stripped before `judgeOne` ever sees the line,
		// so `… = @apr; -- e.g. 'apr_9f3c…'` would show the reader a
		// pasted id the guard never read (the eighteenth pass, P2-1).
		if interpolated.MatchString(trimmed) {
			t.Fatalf("AUDIT R13-G5b doc guard, line %d: an interpolated id — bind it with @apr, never paste it: %q", n+1, trimmed)
		}
		if strings.HasPrefix(dot, ".") {
			// A dot-command at the prompt: the allowlist judges it.
			closeOpen()
			judge(n+1, dot)
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
	if inSpan {
		t.Fatalf("AUDIT R13-G5b doc guard: an inline code span opened on line %d is still open at the end of the document (an unbalanced backtick)", spanOpenedAt+1)
	}
	if tableStatements < 3 {
		t.Fatalf("the guard must see the document's statements over approval_tombstones (found %d) — the document moved or its examples changed", tableStatements)
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Train E, batch 5 (GE10) — TE54, plan §7's text inventory as a mould. Every
// sentence the train replaced is named with the reason it went. On the live
// surfaces — production code and its comments, the tests, the release notes,
// docs/operations, README, SECURITY, the top-level documents under docs (the
// handoff among them) and the website, in English and Spanish — none of them
// stands, save in the two moulds that name them to forbid them. In the
// historical papers under docs/superpowers/specs and docs/stages that keep
// one, the paragraph — or, in a table, the row — that holds it carries the
// superseded mark. The raw captures under docs/superpowers/specs/evidence
// are records, never edited, and are not read.
//
// Matching folds every run of whitespace, and the comment marker that opens
// a wrapped code line, into one space: a sentence wrapped across lines, in a
// comment or in a paragraph, is still one sentence.
//
// PROBING MUTATION (MU54): an old sentence put back into production code —
// the approvals 503, the screen's copy, a godoc — → reddens.
//
// Evidence level: an executable editorial check of the repository's own
// files, in-process. It proves where the sentences stand; what each current
// sentence describes is proved by the mould it names, not here.

// e5Old is one retired sentence and the reason it went.
type e5Old struct{ text, why string }

// e5OldText is the inventory.
func e5OldText() []e5Old {
	return []e5Old{
		{"No se pudo saber de qué perfil es el libro", "the old unreadable row of «¿Qué pasa hoy?»; D2 replaced it (batch 4, TE49/TE66)"},
		{"El núcleo no pudo leer la marca del libro de acciones", "the old unreadable row's caption; D2 replaced it (batch 4)"},
		{"Mientras no se lea, ninguna puerta aplica nada", "the old unreadable row's caption; D2 replaced it (batch 4)"},
		{"Libro ilegible", "the UX sheet's D2 title, never the approved copy (plan §7)"},
		{"Detén Korvun y restaura tu copia", "the UX sheet's D2 remedy, never the approved copy (plan §7)"},
		{"Mientras tanto no se registra ni se ejecuta nada", "the UX sheet's global D3 sentence; the director's replaced it (plan §7)"},
		{"only restoring the ledger lifts it", "the approvals 503's exclusive remedy; D2's no-copy path lifts it too (batch 5)"},
		{"only restoring the ledger from a copy", "the malformed mark's exclusive remedy (batch 5)"},
		{"nothing repairs it but restoring the ledger from a copy", "the CLI's universal restore advice (batch 4, TE42)"},
		{"solo restaurar el libro desde una copia lo levanta", "the release notes' exclusive remedy (batch 4)"},
		{"solo restaurar el libro lo levanta", "the approvals UX sheet's Spanish copy of the old 503 (batch 5)"},
		{"the row is missing while a marked receipt exists, or the read failed", "ErrLedgerUnreadable's godoc called any failed read a verdict; busy and the environment have their own names (batch 5)"},
		{"one query, never a cache", "Standing judges the shape and the identity rows, not one query (plan §7)"},
		{"CERRADO en la v0.16.2, REDISEÑADO", "the handoff called the identity redesign closed; it is pending until its real closure (plan §7)"},
		{"FALLO DE CONSULTA en el hook rehúsa la conexión (el pool abre otra), nunca es un veredicto", "a read that fails with a structural code is a verdict since batch 1"},
		{"convierte un fallo de consulta en veredicto", "a read that fails with a structural code is a verdict since batch 1"},
		{"FRESCO (ninguna tabla del almacén de actos;", "the handoff's fresh shape without the seed residue (batch 2)"},
		{"ACTUAL (versión del binario y las 34 tablas)", "the handoff's current shape without its three UNIQUE indexes (batch 2)"},
		{"El arranque de la app sobre un libro ilegible ARRANCA:", "the handoff's unconditional boot; a strict profile with an agent brain fails it by name (TE58)"},
		{"TODA otra forma", "train D's «every other shape is unreadable»; the seed residue is fresh since batch 2"},
		{"un índice ausente es rendimiento, no evidencia", "three UNIQUE indexes belong to the shape since batch 2"},
		{"TestShape_theHookRefusesTheConnectionOnAQueryError", "D07's old name; batch 1 renamed it TestShape_theHookRefusesANonVerdictFailure"},
		{"Tres consultas:", "train D's query count for judgeShape; the judgement reads more since train E"},
	}
}

// e5Superseded is the mark a historical paragraph or row carries.
const e5Superseded = "Superado (tren E"

// e5NegativeOracles are the moulds that name the old sentences to forbid them.
func e5NegativeOracles() map[string]bool {
	return map[string]bool{
		"internal/cli/ledger_e5_test.go":                                   true,
		"cmd/korvun-desktop/frontend/src/views/WhatsHappening.e4.test.tsx": true,
	}
}

// e5Fold folds whitespace, and the comment marker that opens a wrapped code
// line, into single spaces, byte for byte; line[i] is the source line of
// folded byte i.
func e5Fold(text string, code bool) (string, []int) {
	var b strings.Builder
	var line []int
	n := 1
	space := false
	blank := func() {
		if !space {
			b.WriteByte(' ')
			line = append(line, n)
			space = true
		}
	}
	for i := 0; i < len(text); {
		switch c := text[i]; {
		case c == '\n':
			n++
			i++
			for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
				i++
			}
			if code && strings.HasPrefix(text[i:], "//") {
				i += 2
			} else if code && strings.HasPrefix(text[i:], "* ") {
				i++
			}
			blank()
		case c == ' ' || c == '\t' || c == '\r':
			blank()
			i++
		default:
			b.WriteByte(c)
			line = append(line, n)
			space = false
			i++
		}
	}
	return b.String(), line
}

// e5Walk lists the files under root/dir, relative to root, whose name ends
// in one of exts, skipping dependency, build and capture directories.
func e5Walk(t *testing.T, root, dir string, exts ...string) []string {
	t.Helper()
	var out []string
	base := filepath.Join(root, dir)
	if _, err := os.Stat(base); err != nil {
		t.Fatalf("the surface %s cannot be read: %v", dir, err)
	}
	err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", "dist", "build", "coverage", ".docusaurus", "evidence", "test-results", "playwright-report":
				return filepath.SkipDir
			}
			return nil
		}
		for _, e := range exts {
			if strings.HasSuffix(p, e) {
				rel, rerr := filepath.Rel(root, p)
				if rerr != nil {
					return rerr
				}
				out = append(out, filepath.ToSlash(rel))
				return nil
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return out
}

// e5Paragraph is the block of text around line: the row itself when it is a
// table row; else the lines up to the blank lines around it, where a list
// item is a block of its own, so one mark never covers a whole list.
func e5Paragraph(lines []string, line int) string {
	i := line - 1
	if strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
		return lines[i]
	}
	item := func(l string) bool {
		l = strings.TrimSpace(l)
		return strings.HasPrefix(l, "- ") || strings.HasPrefix(l, "* ")
	}
	lo, hi := i, i
	for lo > 0 && strings.TrimSpace(lines[lo-1]) != "" && !item(lines[lo]) {
		lo--
	}
	for hi < len(lines)-1 && strings.TrimSpace(lines[hi+1]) != "" && !item(lines[hi+1]) {
		hi++
	}
	return strings.Join(lines[lo:hi+1], "\n")
}

// TE54 · the inventory holds: no retired sentence on a live surface, and
// every one a historical paper keeps is marked superseded where it stands.
func TestE5_TE54_theOldTextIsGoneAndTheHistoryIsMarked(t *testing.T) {
	root := filepath.Join("..", "..")
	code := []string{".go", ".ts", ".tsx", ".js", ".jsx"}
	text := []string{".md", ".mdx", ".json"}
	var live []string
	for _, dir := range []string{"internal", "cmd", "web/builder/src"} {
		live = append(live, e5Walk(t, root, dir, append(code, text...)...)...)
	}
	for _, dir := range []string{"docs/releases", "docs/operations", "website/docs", "website/i18n", "website/src", "website/e2e"} {
		live = append(live, e5Walk(t, root, dir, append(code, text...)...)...)
	}
	live = append(live, "README.md", "SECURITY.md", "website/docusaurus.config.ts", "website/sidebars.ts")
	tops, err := filepath.Glob(filepath.Join(root, "docs", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range tops {
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			t.Fatal(rerr)
		}
		live = append(live, filepath.ToSlash(rel))
	}
	var history []string
	for _, dir := range []string{"docs/superpowers/specs", "docs/stages"} {
		history = append(history, e5Walk(t, root, dir, ".md")...)
	}
	// A scan that read nothing would pass: the surfaces where the sentences
	// lived, and the papers that keep them, must have been read.
	read := map[string]bool{}
	for _, f := range append(append([]string{}, live...), history...) {
		read[f] = true
	}
	for _, must := range []string{
		"internal/controlapi/approvals.go",
		"internal/action/sqlite/profile_standing.go",
		"internal/action/sqlite/ledger_identity.go",
		"internal/cli/ledger.go",
		"cmd/korvun-desktop/frontend/src/views/WhatsHappening.tsx",
		"docs/releases/v0.16.2.md",
		"docs/HANDOFF.md",
		"website/docs/reference/operator-cli.md",
		"website/i18n/es/docusaurus-plugin-content-docs/current/reference/operator-cli.md",
		"docs/superpowers/specs/2026-09-24-v0162-el-marcador-rediseñado-pretest.md",
		"docs/superpowers/specs/2026-09-08-approvals-screen-ux.md",
	} {
		if !read[must] {
			t.Fatalf("the scan does not read %s", must)
		}
	}
	if len(live) < 300 {
		t.Fatalf("the scan read %d live files, want the whole surface (at least 300)", len(live))
	}
	oracles := e5NegativeOracles()
	var found []string
	for _, f := range live {
		if oracles[f] {
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(root, f)) //nolint:gosec // G304: the repository's own files
		if rerr != nil {
			t.Fatalf("read %s: %v", f, rerr)
		}
		isCode := false
		for _, e := range code {
			isCode = isCode || strings.HasSuffix(f, e)
		}
		folded, at := e5Fold(string(raw), isCode)
		for _, o := range e5OldText() {
			for k := strings.Index(folded, o.text); k >= 0; {
				found = append(found, fmt.Sprintf("%s:%d «%s» — %s", f, at[k], o.text, o.why))
				next := strings.Index(folded[k+1:], o.text)
				if next < 0 {
					break
				}
				k += 1 + next
			}
		}
	}
	for _, f := range history {
		raw, rerr := os.ReadFile(filepath.Join(root, f)) //nolint:gosec // G304: the repository's own files
		if rerr != nil {
			t.Fatalf("read %s: %v", f, rerr)
		}
		lines := strings.Split(string(raw), "\n")
		folded, at := e5Fold(string(raw), false)
		for _, o := range e5OldText() {
			for k := strings.Index(folded, o.text); k >= 0; {
				if !strings.Contains(e5Paragraph(lines, at[k]), e5Superseded) {
					found = append(found, fmt.Sprintf("%s:%d «%s» — unmarked: %s", f, at[k], o.text, o.why))
				}
				next := strings.Index(folded[k+1:], o.text)
				if next < 0 {
					break
				}
				k += 1 + next
			}
		}
	}
	if len(found) > 0 {
		t.Fatalf("%d retired sentence(s) still stand:\n%s", len(found), strings.Join(found, "\n"))
	}
}

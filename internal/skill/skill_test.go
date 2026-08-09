// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package skill

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The skills loader (ADR-0041 §6, spec FR-SKILL-1..4, mandate SP4): a leaf,
// stdlib-only reader of AgentSkills-compatible SKILL.md directories with an
// in-house FLAT frontmatter parser (R-3). Malformed skills are skipped with
// a structured warning naming the violation — never a boot failure (AS-5).

// writeSkill creates root/<dir>/SKILL.md with the given content.
func writeSkill(t *testing.T, root, dir, content string) {
	t.Helper()
	d := filepath.Join(root, dir)
	if err := os.MkdirAll(d, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(d, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

// captureLogger returns a logger writing to the returned buffer, so warning
// content is assertable.
func captureLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

const validSkill = `---
name: pdf-tools
description: Teaches when to use the http_fetch tool for PDF sources.
license: Apache-2.0
compatibility: Requires network access
allowed-tools: http_fetch read_file
metadata:
  author: example-org
  version: "1.0"
---

# Using PDF tools

Fetch the PDF first, then summarize it.
`

func TestLoadDir_parsesAValidSkill(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeSkill(t, root, "pdf-tools", validSkill)
	logger, _ := captureLogger()

	skills, err := LoadDir(root, logger)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("loaded %d skills, want 1: %+v", len(skills), skills)
	}
	s := skills[0]
	if s.Name != "pdf-tools" {
		t.Errorf("Name = %q", s.Name)
	}
	if s.Description != "Teaches when to use the http_fetch tool for PDF sources." {
		t.Errorf("Description = %q", s.Description)
	}
	if s.License != "Apache-2.0" {
		t.Errorf("License = %q", s.License)
	}
	if s.Compatibility != "Requires network access" {
		t.Errorf("Compatibility = %q", s.Compatibility)
	}
	// allowed-tools is RECORDED but never honored as a grant (D-4) — the
	// loader's job ends at the record.
	if len(s.AllowedTools) != 2 || s.AllowedTools[0] != "http_fetch" || s.AllowedTools[1] != "read_file" {
		t.Errorf("AllowedTools = %+v", s.AllowedTools)
	}
	if !strings.Contains(s.Body, "Fetch the PDF first") {
		t.Errorf("Body missing content: %q", s.Body)
	}
	// The nested metadata block was SKIPPED with tolerance, not parsed and
	// not smuggled into any field.
	if strings.Contains(s.Body, "example-org") {
		t.Errorf("nested metadata leaked into the body: %q", s.Body)
	}
}

// The flat parser skips nested blocks with tolerance and keeps parsing
// FIRST-LEVEL keys that follow them.
func TestLoadDir_nestedBlockThenFlatKeyStillParses(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeSkill(t, root, "nested-then-flat", `---
name: nested-then-flat
metadata:
  author: someone
  deep:
    deeper: x
description: Survives a nested block before it.
---
body
`)
	logger, _ := captureLogger()

	skills, err := LoadDir(root, logger)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("loaded %d skills, want 1", len(skills))
	}
	if skills[0].Description != "Survives a nested block before it." {
		t.Fatalf("Description = %q — the flat key after the nested block was lost", skills[0].Description)
	}
}

// Unknown first-level keys are tolerated (forward compatibility with the
// open format).
func TestLoadDir_unknownKeysAreTolerated(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeSkill(t, root, "future-keys", `---
name: future-keys
description: Carries keys this parser does not know.
future-field: whatever
---
body
`)
	logger, _ := captureLogger()

	skills, err := LoadDir(root, logger)
	if err != nil || len(skills) != 1 {
		t.Fatalf("LoadDir = (%d skills, %v), want 1 skill", len(skills), err)
	}
}

// AS-5: one valid + one malformed (frontmatter name ≠ directory name) → the
// valid one loads, the malformed one is SKIPPED with a structured warning
// naming the violation, and loading does NOT fail.
func TestLoadDir_AS5_malformedSkippedWithWarning(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeSkill(t, root, "good-skill", `---
name: good-skill
description: A valid skill.
---
body
`)
	writeSkill(t, root, "bad-dir", `---
name: other-name
description: Name does not match its directory.
---
body
`)
	logger, buf := captureLogger()

	skills, err := LoadDir(root, logger)
	if err != nil {
		t.Fatalf("LoadDir must not fail on a malformed skill: %v", err)
	}
	if len(skills) != 1 || skills[0].Name != "good-skill" {
		t.Fatalf("loaded %+v, want exactly the valid skill", skills)
	}
	warn := buf.String()
	if !strings.Contains(warn, "bad-dir") || !strings.Contains(warn, "name") {
		t.Fatalf("warning does not name the violation:\n%s", warn)
	}
}

func TestLoadDir_validationRejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		testName string
		dir      string
		content  string
	}{
		{"missing frontmatter", "no-front", "just a body\n"},
		{"unterminated frontmatter", "no-close", "---\nname: no-close\ndescription: x\n"},
		{"uppercase name", "upper", "---\nname: Upper\ndescription: x\n---\nbody\n"},
		{"consecutive hyphens", "double--hyphen", "---\nname: double--hyphen\ndescription: x\n---\nbody\n"},
		{"leading hyphen", "-lead", "---\nname: -lead\ndescription: x\n---\nbody\n"},
		{"empty description", "no-desc", "---\nname: no-desc\ndescription:   \n---\nbody\n"},
		{"missing name", "nameless", "---\ndescription: x\n---\nbody\n"},
		{"overlong name", "longname", "---\nname: " + strings.Repeat("a", 65) + "\ndescription: x\n---\nbody\n"},
		{"overlong description", "longdesc", "---\nname: longdesc\ndescription: " + strings.Repeat("d", 1025) + "\n---\nbody\n"},
	}
	for _, tc := range cases {
		t.Run(tc.testName, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeSkill(t, root, tc.dir, tc.content)
			logger, buf := captureLogger()

			skills, err := LoadDir(root, logger)
			if err != nil {
				t.Fatalf("LoadDir must degrade, not fail: %v", err)
			}
			if len(skills) != 0 {
				t.Fatalf("malformed skill loaded: %+v", skills)
			}
			if buf.Len() == 0 {
				t.Fatal("no structured warning emitted for the skipped skill")
			}
		})
	}
}

// The 64 KiB read cap: an oversize SKILL.md is skipped with a warning.
func TestLoadDir_oversizeFileIsSkipped(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	big := "---\nname: big-skill\ndescription: x\n---\n" + strings.Repeat("b", MaxSkillFileBytes)
	writeSkill(t, root, "big-skill", big)
	logger, buf := captureLogger()

	skills, err := LoadDir(root, logger)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(skills) != 0 {
		t.Fatal("oversize skill loaded")
	}
	if !strings.Contains(buf.String(), "big-skill") {
		t.Fatalf("warning does not name the oversize skill:\n%s", buf.String())
	}
}

// Non-skill directory entries (a dir without SKILL.md, a stray file) are
// ignored without noise.
func TestLoadDir_ignoresNonSkillEntries(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "not-a-skill"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	logger, _ := captureLogger()

	skills, err := LoadDir(root, logger)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("loaded %+v from non-skill entries", skills)
	}
}

func TestLoadDir_missingRootFailsLoud(t *testing.T) {
	t.Parallel()
	logger, _ := captureLogger()
	if _, err := LoadDir(filepath.Join(t.TempDir(), "missing"), logger); err == nil {
		t.Fatal("LoadDir on a missing root must fail (config error, the wiring decides)")
	}
}

// Deterministic order: skills come back sorted by name regardless of
// directory enumeration order.
func TestLoadDir_deterministicOrder(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, n := range []string{"zeta-skill", "alpha-skill", "mid-skill"} {
		writeSkill(t, root, n, "---\nname: "+n+"\ndescription: d\n---\nbody\n")
	}
	logger, _ := captureLogger()

	skills, err := LoadDir(root, logger)
	if err != nil || len(skills) != 3 {
		t.Fatalf("LoadDir = (%d, %v), want 3", len(skills), err)
	}
	if skills[0].Name != "alpha-skill" || skills[1].Name != "mid-skill" || skills[2].Name != "zeta-skill" {
		t.Fatalf("order not deterministic: %+v", []string{skills[0].Name, skills[1].Name, skills[2].Name})
	}
}

// PromptBlock (R-4): name+description always; bodies under the total rune
// budget — a body that would overflow is omitted (the skill stays listed)
// and reported, never a failure.
func TestPromptBlock_namesAlwaysBodiesUnderBudget(t *testing.T) {
	t.Parallel()
	skills := []Skill{
		{Name: "alpha", Description: "first skill", Body: strings.Repeat("a", 50)},
		{Name: "beta", Description: "second skill", Body: strings.Repeat("b", 500)},
		{Name: "gamma", Description: "third skill", Body: strings.Repeat("g", 30)},
	}

	block, omitted := PromptBlock(skills, 100)

	for _, want := range []string{"alpha: first skill", "beta: second skill", "gamma: third skill"} {
		if !strings.Contains(block, want) {
			t.Errorf("block missing catalog line %q:\n%s", want, block)
		}
	}
	// alpha (50) fits, beta (500) overflows the 100-rune budget and is
	// omitted, gamma (30) still fits in the remainder.
	if !strings.Contains(block, strings.Repeat("a", 50)) {
		t.Error("alpha body missing")
	}
	if strings.Contains(block, strings.Repeat("b", 500)) {
		t.Error("beta body included despite overflowing the budget")
	}
	if !strings.Contains(block, strings.Repeat("g", 30)) {
		t.Error("gamma body missing — greedy continuation after an omitted body")
	}
	if len(omitted) != 1 || omitted[0] != "beta" {
		t.Fatalf("omitted = %+v, want [beta]", omitted)
	}
}

func TestPromptBlock_emptyInputIsEmpty(t *testing.T) {
	t.Parallel()
	block, omitted := PromptBlock(nil, 100)
	if block != "" || len(omitted) != 0 {
		t.Fatalf("PromptBlock(nil) = (%q, %+v), want empty", block, omitted)
	}
}

func TestPromptBlock_zeroBudgetUsesDefault(t *testing.T) {
	t.Parallel()
	skills := []Skill{{Name: "solo", Description: "d", Body: "short body"}}
	block, omitted := PromptBlock(skills, 0)
	if !strings.Contains(block, "short body") {
		t.Fatalf("default budget did not admit a short body:\n%s", block)
	}
	if len(omitted) != 0 {
		t.Fatalf("omitted = %+v, want none", omitted)
	}
}

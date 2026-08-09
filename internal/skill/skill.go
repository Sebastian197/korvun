// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package skill is Korvun's read-only loader of AgentSkills-compatible
// markdown skills (ADR-0041 §6): directories holding a SKILL.md whose YAML
// frontmatter is parsed with an in-house FLAT-subset parser (R-3 — zero
// dependencies; the two REQUIRED AgentSkills fields are flat strings). It is
// a leaf: standard library only. Skills are DOCUMENTATION, never
// authorization (spec D-4): the frontmatter's experimental allowed-tools is
// recorded but no loader output ever widens a policy grant.
//
// The loader never executes anything and follows no references in this cut
// (zero levels ≤ the one-level bound of ADR-0041 §6): a skill's relative
// links ride verbatim inside its body text. A malformed skill is skipped
// with a structured warning naming the violation — degrading, never a boot
// failure (spec FR-SKILL-4, AS-5).
package skill

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// MaxSkillFileBytes is the SKILL.md read cap (ADR-0041 §6). A skill body is
// prompt-bound, so a larger file is a mistake, not a bigger skill.
const MaxSkillFileBytes = 64 * 1024

// DefaultBodyBudget is the total rune budget for injected skill bodies when
// the operator does not set one (R-4). Prompt-sized: bodies past it are
// omitted with a warning, names and descriptions always ride.
const DefaultBodyBudget = 8192

// Skill is one loaded, validated skill.
type Skill struct {
	// Name is the frontmatter name — validated to the AgentSkills
	// constraints and equal to the parent directory name.
	Name string
	// Description is the required what/when summary (1–1024).
	Description string
	// License is the optional license note.
	License string
	// Compatibility is the optional environment note.
	Compatibility string
	// AllowedTools is the frontmatter's experimental allowed-tools list,
	// RECORDED ONLY — never honored as a policy grant (spec D-4).
	AllowedTools []string
	// Body is the markdown after the frontmatter, verbatim.
	Body string
}

// nameRe pins the AgentSkills name constraints (verified at source
// 2026-08-09): lowercase alphanumerics and single hyphens, no
// leading/trailing/consecutive hyphen. Length is checked separately for a
// clearer violation message.
var nameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// LoadDir loads every skill directory under root: each subdirectory holding
// a SKILL.md is a candidate; anything else (stray files, dirs without a
// SKILL.md, symlinked entries) is ignored. Malformed candidates are SKIPPED
// with a structured warning naming the directory and the violation — LoadDir
// only fails on an unusable root (a config error the wiring surfaces). The
// result is sorted by name, deterministically.
func LoadDir(root string, logger *slog.Logger) ([]Skill, error) {
	if logger == nil {
		logger = slog.Default()
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("skill: read skills dir: %w", err)
	}

	var skills []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := e.Name()
		path := filepath.Join(root, dir, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			continue // not a skill directory — no noise
		}
		s, err := loadOne(path, dir)
		if err != nil {
			logger.Warn("skill: skipping malformed skill",
				"dir", dir, "violation", err.Error())
			continue
		}
		skills = append(skills, s)
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, nil
}

// loadOne reads and validates a single SKILL.md against the source-verified
// AgentSkills constraints.
func loadOne(path, dir string) (Skill, error) {
	// #nosec G304 -- path is <operator-configured skills root>/<enumerated
	// dir>/SKILL.md: both halves come from the operator's own filesystem
	// layout, and the read is capped and validated below.
	f, err := os.Open(path)
	if err != nil {
		return Skill{}, fmt.Errorf("open SKILL.md: %v", err)
	}
	defer f.Close() //nolint:errcheck // read-only descriptor
	raw, err := io.ReadAll(io.LimitReader(f, MaxSkillFileBytes+1))
	if err != nil {
		return Skill{}, fmt.Errorf("read SKILL.md: %v", err)
	}
	if len(raw) > MaxSkillFileBytes {
		return Skill{}, fmt.Errorf("SKILL.md exceeds the %d-byte cap", MaxSkillFileBytes)
	}

	s, err := parseSkillMD(string(raw))
	if err != nil {
		return Skill{}, err
	}
	if s.Name == "" {
		return Skill{}, fmt.Errorf("frontmatter name is required")
	}
	if len(s.Name) > 64 {
		return Skill{}, fmt.Errorf("frontmatter name exceeds 64 characters")
	}
	if !nameRe.MatchString(s.Name) {
		return Skill{}, fmt.Errorf("frontmatter name %q is not lowercase-kebab", s.Name)
	}
	if s.Name != dir {
		return Skill{}, fmt.Errorf("frontmatter name %q does not match the directory name %q", s.Name, dir)
	}
	if s.Description == "" {
		return Skill{}, fmt.Errorf("frontmatter description is required")
	}
	if len(s.Description) > 1024 {
		return Skill{}, fmt.Errorf("frontmatter description exceeds 1024 characters")
	}
	return s, nil
}

// parseSkillMD splits SKILL.md into the flat frontmatter and the body. The
// FLAT-subset contract (R-3): only first-level `key: value` lines are read;
// an indented line (a nested block's continuation, e.g. under `metadata:`)
// is skipped with tolerance; unknown first-level keys are skipped too
// (forward compatibility with the open format).
func parseSkillMD(content string) (Skill, error) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return Skill{}, fmt.Errorf("SKILL.md does not start with a --- frontmatter fence")
	}

	var s Skill
	closed := false
	bodyStart := 0
	for i := 1; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\r")
		if line == "---" {
			closed = true
			bodyStart = i + 1
			break
		}
		// An indented line continues a nested block — skipped, tolerant.
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue // not a key line — tolerated
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "name":
			s.Name = value
		case "description":
			s.Description = value
		case "license":
			s.License = value
		case "compatibility":
			s.Compatibility = value
		case "allowed-tools":
			if value != "" {
				s.AllowedTools = strings.Fields(value)
			}
		default:
			// Unknown first-level key (including a nested block opener like
			// "metadata:") — skipped with tolerance.
		}
	}
	if !closed {
		return Skill{}, fmt.Errorf("SKILL.md frontmatter fence is never closed")
	}
	s.Body = strings.TrimSpace(strings.Join(lines[bodyStart:], "\n"))
	return s, nil
}

// PromptBlock renders the skills section for the agent's seed system prompt
// (R-4): every skill's name+description ALWAYS; bodies greedily included in
// name order under the total rune budget (0 => DefaultBodyBudget). A body
// that would overflow is omitted — the skill stays listed — and its name is
// returned so the caller can warn. Empty input renders nothing.
func PromptBlock(skills []Skill, bodyBudget int) (string, []string) {
	if len(skills) == 0 {
		return "", nil
	}
	if bodyBudget <= 0 {
		bodyBudget = DefaultBodyBudget
	}

	var b strings.Builder
	b.WriteString("Skills (guidance on when to use your tools):\n")
	for _, s := range skills {
		b.WriteString("- ")
		b.WriteString(s.Name)
		b.WriteString(": ")
		b.WriteString(s.Description)
		b.WriteString("\n")
	}

	var omitted []string
	remaining := bodyBudget
	for _, s := range skills {
		if s.Body == "" {
			continue
		}
		runes := len([]rune(s.Body))
		if runes > remaining {
			omitted = append(omitted, s.Name)
			continue // greedy: a later, smaller body may still fit
		}
		remaining -= runes
		b.WriteString("\n")
		b.WriteString(s.Name)
		b.WriteString(" instructions:\n")
		b.WriteString(s.Body)
		b.WriteString("\n")
	}
	return b.String(), omitted
}

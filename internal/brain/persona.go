// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import "strings"

// Persona is a brain's optional personality (builder-canvas spec FR-PERSONA-2,
// NC-4 resolved), mirrored from config.PersonaConfig by internal/app — this
// package stays free of a config import. All fields are free text; empty
// fields contribute nothing to the composed prompt.
type Persona struct {
	DisplayName  string
	Tone         string
	Language     string
	Instructions string
}

// ComposePersona assembles a persona into a system-prompt fragment. It is PURE
// and deterministic: each field is TrimSpace'd, only present fields emit a
// section, sections ride a FIXED order (display name, tone, language,
// instructions) joined by "\n", and a nil/empty/whitespace-only persona
// composes to "" — zero noise in the prompt (NC-4). The fragment is the PREFIX
// of the brain's system prompt: the Orchestrator receives it via
// WithSystemPrompt, the AgentBrain via WithAgentPersona.
func ComposePersona(p *Persona) string {
	if p == nil {
		return ""
	}
	sections := make([]string, 0, 4)
	if name := strings.TrimSpace(p.DisplayName); name != "" {
		sections = append(sections, "You are "+name+".")
	}
	if tone := strings.TrimSpace(p.Tone); tone != "" {
		sections = append(sections, "Tone: "+tone)
	}
	if lang := strings.TrimSpace(p.Language); lang != "" {
		sections = append(sections, "Language: "+lang)
	}
	if instr := strings.TrimSpace(p.Instructions); instr != "" {
		sections = append(sections, instr)
	}
	return strings.Join(sections, "\n")
}

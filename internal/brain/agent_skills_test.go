// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"strings"
	"testing"
)

// Skills injection (ADR-0041 §6, spec FR-SKILL-2, mandate SP4): the
// pre-composed skills block joins the SAME seed system prompt as the tool
// catalog — ADDED after the ADR-0021 §3.1 protocol block (grammar, catalog,
// operator prompt), never reordering it. The persona precedent, on the other
// end of the prompt.

func TestAgentSkills_blockAppendedAfterProtocol(t *testing.T) {
	t.Parallel()
	spy := &spyTool{}
	m := &scriptedModel{name: "m", replies: []string{"done"}}
	a := NewAgentBrain(m, spyRegistry(spy),
		WithAgentLogger(quietLogger()),
		WithAgentSystemPrompt("operator instructions here"),
		WithAgentSkillsBlock("Skills:\n- pdf-tools: teaches PDF fetching"))

	if _, err := a.Handle(context.Background(), inboundText("telegram", "c", "go")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	prompt := systemPromptOf(t, m)

	// The §3.1 internal order is untouched: grammar, catalog, operator
	// prompt — and the skills block comes AFTER all of it.
	grammar := strings.Index(prompt, "You can use tools.")
	catalog := strings.Index(prompt, "- spy:")
	operator := strings.Index(prompt, "operator instructions here")
	skills := strings.Index(prompt, "pdf-tools: teaches PDF fetching")
	if grammar < 0 || catalog < 0 || operator < 0 || skills < 0 {
		t.Fatalf("prompt missing a section (grammar %d, catalog %d, operator %d, skills %d):\n%s",
			grammar, catalog, operator, skills, prompt)
	}
	if !(grammar < catalog && catalog < operator && operator < skills) {
		t.Fatalf("sections reordered (grammar %d, catalog %d, operator %d, skills %d):\n%s",
			grammar, catalog, operator, skills, prompt)
	}
}

// Persona prefix + skills suffix coexist around the intact protocol block.
func TestAgentSkills_coexistsWithPersona(t *testing.T) {
	t.Parallel()
	spy := &spyTool{}
	m := &scriptedModel{name: "m", replies: []string{"done"}}
	a := NewAgentBrain(m, spyRegistry(spy),
		WithAgentLogger(quietLogger()),
		WithAgentPersona("You are Korvo, terse and precise."),
		WithAgentSkillsBlock("Skills:\n- pdf-tools: teaches PDF fetching"))

	if _, err := a.Handle(context.Background(), inboundText("telegram", "c", "go")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	prompt := systemPromptOf(t, m)

	persona := strings.Index(prompt, "You are Korvo")
	grammar := strings.Index(prompt, "You can use tools.")
	skills := strings.Index(prompt, "pdf-tools")
	if persona < 0 || grammar < 0 || skills < 0 {
		t.Fatalf("prompt missing a section:\n%s", prompt)
	}
	if !(persona < grammar && grammar < skills) {
		t.Fatalf("expected persona < protocol < skills, got %d/%d/%d:\n%s", persona, grammar, skills, prompt)
	}
}

// AS-4 stands: no skills block → the seed system prompt is byte-identical to
// today's.
func TestAgentSkills_absentBlockIsByteIdentical(t *testing.T) {
	t.Parallel()
	spy := &spyTool{}
	m := &scriptedModel{name: "m", replies: []string{"done"}}
	a := NewAgentBrain(m, spyRegistry(spy), WithAgentLogger(quietLogger()))

	if _, err := a.Handle(context.Background(), inboundText("telegram", "c", "go")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	want := buildSystemPrompt(spyRegistry(spy), "")
	if got := systemPromptOf(t, m); got != want {
		t.Fatalf("prompt drifted without a skills block:\ngot:  %q\nwant: %q", got, want)
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/tool"
)

// TestParseReply_toolCalls covers how models ACTUALLY emit a tool request, not
// only the clean case (the prompt-protocol's fragility lives here, ADR-0021 §3.2).
// Accepted as a tool call: the first MEANINGFUL line (blank lines and code-fence
// delimiters skipped, surrounding/inline backticks stripped, leading/trailing
// whitespace trimmed) that matches "TOOL: name(args)" case-insensitively, with
// flexible spacing around the colon. args is captured VERBATIM (the tool parses
// it). The tool name is lower-cased for registry lookup.
func TestParseReply_toolCalls(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		content  string
		wantName string
		wantArgs string
	}{
		{"clean", "TOOL: calc(2+2)", "calc", "2+2"},
		{"surrounding whitespace", "   TOOL: echo(hi)   ", "echo", "hi"},
		{"blank initial lines", "\n\n\nTOOL: time()", "time", ""},
		{"fenced block", "```\nTOOL: calc(2+2)\n```", "calc", "2+2"},
		{"fenced block with lang", "```text\nTOOL: calc(1+1)\n```", "calc", "1+1"},
		{"inline backticks", "`TOOL: echo(x)`", "echo", "x"},
		{"lowercase keyword", "tool: calc(2+2)", "calc", "2+2"},
		{"mixed case keyword and name", "Tool: Calc(2+2)", "calc", "2+2"},
		{"no space after colon", "TOOL:calc(2+2)", "calc", "2+2"},
		{"space before colon", "TOOL : calc(2+2)", "calc", "2+2"},
		{"args with inner parens", "TOOL: calc((1+2)*3)", "calc", "(1+2)*3"},
		{"args preserved verbatim", "TOOL: echo( spaced )", "echo", " spaced "},
		{"trailing CRLF", "TOOL: calc(2+2)\r\n", "calc", "2+2"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			name, args, isCall := parseReply(tc.content)
			if !isCall {
				t.Fatalf("parseReply(%q) isCall=false, want a tool call", tc.content)
			}
			if name != tc.wantName || args != tc.wantArgs {
				t.Fatalf("parseReply(%q) = (%q, %q), want (%q, %q)",
					tc.content, name, args, tc.wantName, tc.wantArgs)
			}
		})
	}
}

// TestParseReply_finalAnswers covers what FALLS to "final answer" (ADR-0021 §3.2,
// §3.3): no TOOL line, malformed TOOL line (missing parens or missing name), and
// — the documented minimal-cut simplification — PROSE before the tool line. A
// model that explains itself before calling a tool is treated as a final answer;
// the system prompt asks for exactly one line, and the native path (§3.4) removes
// this ambiguity structurally.
func TestParseReply_finalAnswers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
	}{
		{"plain prose", "The answer is 4."},
		{"empty", ""},
		{"whitespace only", "   \n  \t\n"},
		{"prose before tool line", "Let me calculate that.\nTOOL: calc(2+2)"},
		{"malformed missing parens", "TOOL: calc"},
		{"malformed missing name", "TOOL: (2+2)"},
		{"keyword inside prose", "You could call TOOL: calc(2+2) here."},
		{"fence with prose", "```\njust some text\n```"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			name, args, isCall := parseReply(tc.content)
			if isCall {
				t.Fatalf("parseReply(%q) = tool call (%q,%q), want final answer",
					tc.content, name, args)
			}
		})
	}
}

// TestBuildSystemPrompt lists every tool deterministically (sorted by name) with
// its Name and Description, and embeds the protocol grammar so the model knows
// how to call a tool and how to signal a final answer (ADR-0021 §3.1). The
// operator's own system prompt is appended after the protocol block.
func TestBuildSystemPrompt(t *testing.T) {
	t.Parallel()
	reg := tool.Registry{
		"calc": tool.Calc(),
		"echo": tool.Echo(),
		"time": tool.Time(nil),
	}
	got := buildSystemPrompt(reg, "Be concise.")

	for _, want := range []string{
		"TOOL:", "OBSERVATION:",
		"calc:", "echo:", "time:",
		"Be concise.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("system prompt missing %q\n---\n%s", want, got)
		}
	}
	// Deterministic ordering: calc before echo before time (sorted by name).
	ci, ei, ti := strings.Index(got, "calc:"), strings.Index(got, "echo:"), strings.Index(got, "time:")
	if !(ci < ei && ei < ti) {
		t.Fatalf("tools not listed in sorted order: calc=%d echo=%d time=%d", ci, ei, ti)
	}
}

// TestBuildSystemPrompt_noOperatorPrompt proves an empty operator prompt is
// omitted cleanly (no dangling separator), the protocol block still present.
func TestBuildSystemPrompt_noOperatorPrompt(t *testing.T) {
	t.Parallel()
	got := buildSystemPrompt(tool.Registry{"echo": tool.Echo()}, "")
	if !strings.Contains(got, "TOOL:") || !strings.Contains(got, "echo:") {
		t.Fatalf("system prompt missing protocol or tool list:\n%s", got)
	}
}

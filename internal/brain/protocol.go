// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"regexp"
	"sort"
	"strings"

	"github.com/Sebastian197/korvun/internal/tool"
)

// The prompt-protocol (ADR-0021 §3) carries tool-use ENTIRELY inside
// model.Message.Content, over the existing model.Model interface — zero change to
// model.Model / Request / Response / the adapters. There is no Tool role:
// model.Role stays system|user|assistant, and observations ride as user messages
// (the loop in agent.go composes them). This file owns the two pure halves of the
// protocol: the system-prompt builder (loop → model) and the reply parser
// (model → loop). The deferred provider-native path (ToolCallingModel, ADR-0021
// §3.4) would replace this string protocol with a structured one.

// toolLineRe matches a single "TOOL: name(args)" call (ADR-0021 §3.2), tolerant
// of the formatting real models add around the same one-line answer:
//   - case-insensitive keyword ("(?i)") — TOOL / Tool / tool all match;
//   - flexible spacing around the colon and before the parens;
//   - name = a leading-letter identifier; args = everything between the OUTER
//     parens (greedy, so "(1+2)*3" survives), captured VERBATIM for the tool to
//     parse (the tool owns arg parsing, keeping the seam domain-agnostic).
//
// It is applied to a single already-cleaned line (see firstMeaningfulLine), so it
// anchors with ^...$ to reject prose that merely mentions the keyword.
var toolLineRe = regexp.MustCompile(`(?i)^tool\s*:\s*([a-zA-Z][a-zA-Z0-9_]*)\s*\((.*)\)$`)

// parseReply classifies a model reply as either a tool call or a final answer
// (ADR-0021 §3.2, §3.3).
//
// It inspects the FIRST MEANINGFUL line — blank lines and code-fence delimiters
// (```), surrounding/inline backticks, and leading/trailing whitespace are
// formatting noise and are stripped, because models wrap the same single-line
// answer in them. If that line matches the TOOL grammar, it is a tool call and
// the lower-cased name + verbatim args are returned. Anything else — no TOOL
// line, a malformed TOOL line (missing parens or name), or PROSE before the tool
// line — falls to a final answer (isToolCall=false). The prose-before-tool case
// is the documented minimal-cut simplification: the system prompt asks for
// exactly one line, and the native path (§3.4) removes the ambiguity
// structurally.
func parseReply(content string) (name, args string, isToolCall bool) {
	line, ok := firstMeaningfulLine(content)
	if !ok {
		return "", "", false
	}
	m := toolLineRe.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	return strings.ToLower(m[1]), m[2], true
}

// firstMeaningfulLine returns the first line of content that carries actual
// payload, skipping empty lines and code-fence delimiters and stripping
// surrounding backticks (the formatting noise models add). The bool is false when
// content has no meaningful line (empty or whitespace/fence-only).
func firstMeaningfulLine(content string) (string, bool) {
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "```") {
			continue // a code-fence delimiter (``` or ```lang) — formatting noise
		}
		line = strings.TrimSpace(strings.Trim(line, "`")) // strip inline-code backticks
		if line == "" {
			continue
		}
		return line, true
	}
	return "", false
}

// buildSystemPrompt assembles the protocol system message (ADR-0021 §3.1): the
// grammar (how to call a tool, how the result returns as an OBSERVATION, how to
// signal a final answer) followed by the tool catalog, listed deterministically
// (sorted by name) so the prompt is reproducible. The operator's own system
// prompt, if any, is appended after the protocol block.
func buildSystemPrompt(reg tool.Registry, operatorPrompt string) string {
	names := make([]string, 0, len(reg))
	for n := range reg {
		names = append(names, n)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("You can use tools. To call a tool, reply with EXACTLY one line and nothing else:\n")
	b.WriteString("TOOL: <name>(<args>)\n")
	b.WriteString("You will then receive a line starting with \"OBSERVATION:\" carrying the result.\n")
	b.WriteString("When you have the final answer, reply normally WITHOUT a TOOL: line.\n")
	b.WriteString("Available tools:\n")
	for _, n := range names {
		t := reg[n]
		b.WriteString("- ")
		b.WriteString(t.Name())
		b.WriteString(": ")
		b.WriteString(t.Description())
		b.WriteString("\n")
	}
	if strings.TrimSpace(operatorPrompt) != "" {
		b.WriteString("\n")
		b.WriteString(operatorPrompt)
	}
	return b.String()
}

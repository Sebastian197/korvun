// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"encoding/json"
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

// fenceDelimRe matches a PURE code-fence delimiter line: a run of backticks plus
// an optional bare language tag and nothing else (``` or ```text). Such lines are
// formatting noise and are skipped. A line that merely STARTS with backticks but
// carries real payload (a tool call wrapped in a single-line fence,
// "```TOOL: calc(2+2)```") does NOT match, so its backticks are stripped and the
// payload is kept rather than dropped.
var fenceDelimRe = regexp.MustCompile("^`{3,}[a-zA-Z0-9]*$")

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
		if fenceDelimRe.MatchString(line) {
			continue // a pure code-fence delimiter (``` or ```lang) — formatting noise
		}
		line = strings.TrimSpace(strings.Trim(line, "`")) // strip inline-code backticks / single-line fences
		if line == "" {
			continue
		}
		return line, true
	}
	return "", false
}

// protocolLeakReply is the honest user-facing error sent instead of a
// tool-call-shaped final answer (B13, spec FR-B13-2): the raw protocol JSON
// never reaches a channel, and the phantom tool's name — model-controlled —
// stays in the bounded local log, never on the user surface.
const protocolLeakReply = "Sorry, the model produced an internal tool request instead of an answer. Please try again."

// toolCallShape recognises content whose ENTIRE meaningful body is one
// tool-call-shaped JSON object — a string "name" plus at least one of
// "arguments"/"parameters"/"args" — and returns the name it carries (B13,
// spec FR-B13-1). Registry-FREE by design: the channel-exit guard blocks the
// shape whether or not the name exists (the 2026-08-23 fail-open), while the
// native rescue layers its registered-name check on top. Pure code-fence
// delimiter lines and surrounding whitespace are formatting noise (the
// firstMeaningfulLine classes); JSON amid prose, a JSON array, or an object
// without the key pair is NOT the shape — the guard inspects the whole body
// only (FR-B13-5, the pinned edge). The parsed object rides back so the
// native rescue can extract the call's arguments without re-parsing.
func toolCallShape(content string) (string, map[string]any, bool) {
	lines := strings.Split(content, "\n")
	start, end := 0, len(lines)
	for start < end && isFormattingNoise(lines[start]) {
		start++
	}
	for end > start && isFormattingNoise(lines[end-1]) {
		end--
	}
	body := strings.TrimSpace(strings.Join(lines[start:end], "\n"))
	if !strings.HasPrefix(body, "{") || !strings.HasSuffix(body, "}") {
		return "", nil, false
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(body), &obj); err != nil {
		return "", nil, false
	}
	name, ok := obj["name"].(string)
	if !ok {
		return "", nil, false
	}
	for _, key := range []string{"arguments", "parameters", "args"} {
		if _, present := obj[key]; present {
			return name, obj, true
		}
	}
	return "", nil, false
}

// isFormattingNoise reports whether a line is blank or a pure code-fence
// delimiter (``` or ```lang) — the same noise classes firstMeaningfulLine
// skips, applied at the body's edges only.
func isFormattingNoise(raw string) bool {
	line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
	return line == "" || fenceDelimRe.MatchString(line)
}

// buildSystemPrompt assembles the protocol system message (ADR-0021 §3.1): the
// grammar (how to call a tool, how the result returns as an OBSERVATION, how to
// signal a final answer) followed by the tool catalog, listed deterministically
// (sorted by name) so the prompt is reproducible. The operator's own system
// prompt, if any, is appended after the protocol block.
func buildSystemPrompt(reg tool.Registry, operatorPrompt string) string {
	// An EMPTY registry (deny-all governance, or the fail-closed gate) must
	// not teach the tool grammar — that would invite the model to hallucinate
	// calls that can only answer "not found" (estreno E-5 / adversarial H7).
	if len(reg) == 0 {
		return strings.TrimSpace(operatorPrompt)
	}
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

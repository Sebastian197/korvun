// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package tool

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// This file is the memory_note builtin (minimal-memory spec FR-TOOL-1/2,
// ADR-0043 §4-5): the governed writer of persistent notes. The tool package
// stays a LEAF — the app composes the writer closure over the conversation
// store and hands it in; the two sentinels below are the leaf-safe
// translation contract for the closure's failure modes.

// Scope carries ENVELOPE FACTS ONLY to a scope-aware tool (FR-TOOL-2):
// the brain's own name and the conversation id, possibly empty.
type Scope struct {
	Brain        string
	Conversation string
}

// ScopedTool is the OPTIONAL conversation-identity capability of a Tool
// (FR-TOOL-2, the ParamTool/ToolCallingModel precedent): the AgentBrain
// type-asserts it in runTool and fills Scope from its own name and the
// envelope; non-scoped tools are untouched.
type ScopedTool interface {
	Tool
	// ExecuteScoped runs the tool with the caller's scope facts. Same
	// error-becomes-observation contract as Execute.
	ExecuteScoped(ctx context.Context, scope Scope, args string) (string, error)
}

// ErrNoteBoxFull is what the app-composed writer closure returns when the
// store refused at the count cap (conversation.ErrNotesFull translated at
// the composition seam — the tool cannot import conversation).
var ErrNoteBoxFull = errors.New("tool: note box full")

// ErrNoteNeedsConversation is what the writer closure returns when scope
// derivation failed (no conversation identity on a conversation-scoped
// brain) — nothing was stored, never a global write.
var ErrNoteNeedsConversation = errors.New("tool: notes need a conversation here")

// MemoryNote is the memory_note builtin (FR-TOOL-1): app-constructed with
// the writer closure and the per-note rune cap; no network, no filesystem.
type MemoryNote struct {
	write        func(ctx context.Context, scope Scope, note string) error
	maxNoteRunes int
}

// NewMemoryNote builds the tool over the app-composed writer closure and
// the per-note rune cap from the brain's memory block.
func NewMemoryNote(write func(ctx context.Context, scope Scope, note string) error, maxNoteRunes int) *MemoryNote {
	return &MemoryNote{write: write, maxNoteRunes: maxNoteRunes}
}

// Name implements Tool.
func (m *MemoryNote) Name() string { return "memory_note" }

// Description implements Tool.
func (m *MemoryNote) Description() string {
	return "stores one short note the brain will remember in this scope. args = the note text."
}

// Execute implements Tool: the scope-less path delegates to ExecuteScoped
// with the zero scope (a conversation-scoped writer will refuse it
// honestly; the AgentBrain always prefers ExecuteScoped).
func (m *MemoryNote) Execute(ctx context.Context, args string) (string, error) {
	return m.ExecuteScoped(ctx, Scope{}, args)
}

// ExecuteScoped implements ScopedTool (FR-TOOL-1): single-line
// normalization, the rune cap named in the refusal, and the honest
// translation of the writer's failure modes — every error becomes the
// model's observation.
func (m *MemoryNote) ExecuteScoped(ctx context.Context, scope Scope, args string) (string, error) {
	note := strings.Join(strings.Fields(args), " ")
	if note == "" {
		return "", errors.New("the note is empty — nothing stored")
	}
	if n := utf8.RuneCountInString(note); m.maxNoteRunes > 0 && n > m.maxNoteRunes {
		return "", fmt.Errorf("the note exceeds the %d-rune cap (got %d) — nothing stored", m.maxNoteRunes, n)
	}
	if err := m.write(ctx, scope, note); err != nil {
		switch {
		case errors.Is(err, ErrNoteBoxFull):
			return "", fmt.Errorf("the note box is full — nothing stored; the operator can clear it with /notes clear: %w", err)
		case errors.Is(err, ErrNoteNeedsConversation):
			return "", fmt.Errorf("notes need a conversation here — nothing stored: %w", err)
		default:
			return "", fmt.Errorf("storing the note failed: %w", err)
		}
	}
	return "Note stored.", nil
}

// Params implements ParamTool: ONE required field `note` (the 2026-08-09
// lesson — small models fill separate fields reliably).
func (m *MemoryNote) Params() []ToolParam {
	return []ToolParam{{
		Name:        "note",
		Description: "the short note to remember, one line",
		Required:    true,
	}}
}

// ArgsFromCall implements ParamTool: tolerant reconstruction with a useful
// error naming the missing field.
func (m *MemoryNote) ArgsFromCall(fields map[string]any) (string, error) {
	note, _ := fields["note"].(string)
	if strings.TrimSpace(note) == "" {
		return "", errors.New(`the "note" field is required and must be a non-empty string`)
	}
	return note, nil
}

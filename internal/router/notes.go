// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"context"
	"fmt"
	"strings"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
)

// This file is the /notes command surface (minimal-memory spec FR-RECALL-2,
// ADR-0043 §7): channel-agnostic, zero model, wired as a WithToolsCommand-
// family option carrying the APP-COMPOSED list/clear closures keyed by
// (brainName, Key) — the router never learns the memory config. An
// unconfigured brain reports ok=false and the token FALLS THROUGH to the
// model like any unknown command. The fixed copy below IS the contract the
// SP-B red suite pins.

// NotesLister returns the scope's notes for the routed brain, oldest-first.
// ok=false means the brain has no memory configured — not handled.
type NotesLister func(ctx context.Context, brainName string, key conversation.Key) (notes []conversation.Note, ok bool, err error)

// NotesClearer clears the scope's notes for the routed brain. ok=false
// means the brain has no memory configured — not handled.
type NotesClearer func(ctx context.Context, brainName string, key conversation.Key) (ok bool, err error)

// WithNotesCommands mounts the /notes first-token commands over the
// app-composed closures (FR-RECALL-2). nil disables them entirely.
func WithNotesCommands(list NotesLister, clear NotesClearer) Option {
	return func(r *Router) {
		r.notesList = list
		r.notesClear = clear
	}
}

// NotesRenderCap caps how many notes the /notes report renders; the honest
// "+N more" suffix names the rest (FR-RECALL-2's render cap).
const NotesRenderCap = 20

// NotesReportHeader opens the fixed numbered /notes report.
const NotesReportHeader = "Stored notes:"

// NotesMoreSuffixFormat is the honest suffix when the report is render-
// capped (fmt verb: how many notes were not rendered).
const NotesMoreSuffixFormat = "… +%d more"

// NotesEmptyReply is the fixed reply when the scope holds no notes.
const NotesEmptyReply = "No notes stored."

// NotesClearedAck is the fixed acknowledgement after /notes clear.
const NotesClearedAck = "Notes cleared."

// NotesUsageReply is the fixed reply for any other /notes argument.
const NotesUsageReply = "Usage: /notes — list the stored notes; /notes clear — remove them."

// NotesErrorReply is the fixed honest reply when the notes store fails
// mid-command; never silence, never the model.
const NotesErrorReply = "Notes failed — the store returned an error."

// maybeHandleNotes applies the /notes first-token commands (FR-RECALL-2).
// It reports whether the envelope was fully handled here: an unconfigured
// brain (ok=false from the closures) falls through to the model like any
// unknown command; store failures answer the fixed honest reply plus a
// structured session error — never silence, never the model.
func (r *Router) maybeHandleNotes(ctx context.Context, key conversation.Key, env *envelope.Envelope, brainName string) bool {
	if r.notesList == nil || r.notesClear == nil {
		return false
	}
	first, rest, _ := strings.Cut(strings.TrimSpace(latestEnvelopeText(env.Parts)), " ")
	if first != "/notes" {
		return false
	}
	switch strings.TrimSpace(rest) {
	case "":
		notes, ok, err := r.notesList(ctx, brainName, key)
		if err != nil {
			r.notesError(env, brainName, err)
			return true
		}
		if !ok {
			return false
		}
		r.publishReceived(env, brainName)
		r.notesReply(env, renderNotesReport(notes))
		return true
	case "clear":
		ok, err := r.notesClear(ctx, brainName, key)
		if err != nil {
			r.notesError(env, brainName, err)
			return true
		}
		if !ok {
			return false
		}
		r.publishReceived(env, brainName)
		r.notesReply(env, NotesClearedAck)
		return true
	default:
		// The configured/unconfigured split must hold for garbage too: an
		// unconfigured brain's "/notes foo" falls through like the rest.
		_, ok, err := r.notesList(ctx, brainName, key)
		if err != nil {
			r.notesError(env, brainName, err)
			return true
		}
		if !ok {
			return false
		}
		r.publishReceived(env, brainName)
		r.notesReply(env, NotesUsageReply)
		return true
	}
}

// renderNotesReport is the fixed numbered report: the header, one line per
// note up to NotesRenderCap, and the honest "+N more" suffix beyond it.
func renderNotesReport(notes []conversation.Note) string {
	if len(notes) == 0 {
		return NotesEmptyReply
	}
	var b strings.Builder
	b.WriteString(NotesReportHeader)
	shown := len(notes)
	if shown > NotesRenderCap {
		shown = NotesRenderCap
	}
	for i := 0; i < shown; i++ {
		fmt.Fprintf(&b, "\n%d. %s", i+1, notes[i].Content)
	}
	if rest := len(notes) - shown; rest > 0 {
		b.WriteString("\n" + fmt.Sprintf(NotesMoreSuffixFormat, rest))
	}
	return b.String()
}

// notesReply sends one fixed /notes reply through the normal outbound
// funnel, marked with the MetaAck molde (AckNotesReport — sanctioned
// conversation content, the /tools precedent; FR-AUD-1).
func (r *Router) notesReply(env *envelope.Envelope, text string) {
	out := envelope.New(env.Channel, envelope.Outbound, korvun).AddText(text)
	for k, v := range env.Meta {
		out.Meta[k] = v
	}
	out.Meta[envelope.MetaAck] = envelope.AckNotesReport
	r.sendReply(out)
}

// notesError reports a mid-command store failure: the structured session
// error (%w-wrapped) plus the fixed honest reply.
func (r *Router) notesError(env *envelope.Envelope, brainName string, err error) {
	r.publishReceived(env, brainName)
	r.notifyError(RouterError{Kind: ErrKindSession, Envelope: env, Err: fmt.Errorf("notes: %w", err)})
	r.notesReply(env, NotesErrorReply)
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// SP-B red suite (minimal-memory spec 2026-08-16, FR-RECALL-2): the /notes
// and /notes clear first-token commands on the dispatch path — the numbered
// render-capped report with the honest "+N more" suffix, the fixed acks and
// usage, the unconfigured-brain FALLTHROUGH to the model, and honest store
// failures. Runs on the SP2/recall molde: fake channel/brain, MemStore, and
// APP-COMPOSED closures faked at the option seam (AS-B13).
package router_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/router"
)

// fakeNotesBackend is the app-composed closure pair at the option seam.
type fakeNotesBackend struct {
	mu       sync.Mutex
	notes    []conversation.Note
	ok       bool
	listErr  error
	clearErr error
	clears   int
	lists    int
}

func (f *fakeNotesBackend) list(_ context.Context, _ string, _ conversation.Key) ([]conversation.Note, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lists++
	return append([]conversation.Note(nil), f.notes...), f.ok, f.listErr
}

func (f *fakeNotesBackend) clear(_ context.Context, _ string, _ conversation.Key) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clears++
	if f.clearErr == nil {
		f.notes = nil
	}
	return f.ok, f.clearErr
}

func (f *fakeNotesBackend) counts() (lists, clears int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lists, f.clears
}

func notesOf(n int) []conversation.Note {
	out := make([]conversation.Note, n)
	for i := range out {
		out[i] = conversation.Note{Seq: i + 1, Content: fmt.Sprintf("nota-%02d", i+1), Timestamp: time.Unix(int64(100+i), 0)}
	}
	return out
}

func notesRouter(t *testing.T, backend *fakeNotesBackend) (*router.Router, *fakeChannel, *fakeBrain) {
	t.Helper()
	return sessionRouter(t, conversation.NewMemStore(),
		router.WithNotesCommands(backend.list, backend.clear))
}

// AS-B13: the numbered report, render-capped with the honest suffix.
func TestNotes_NumberedReportWithMoreSuffix(t *testing.T) {
	backend := &fakeNotesBackend{notes: notesOf(router.NotesRenderCap + 5), ok: true}
	r, ch, br := notesRouter(t, backend)

	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/notes")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "the notes report", func() bool { return len(ch.Sent()) == 1 })

	report := ch.Sent()[0]
	body := report.Parts[0].Content
	if !strings.HasPrefix(body, router.NotesReportHeader) {
		t.Fatalf("report does not open with %q:\n%q", router.NotesReportHeader, body)
	}
	if !strings.Contains(body, "1. nota-01") || !strings.Contains(body, fmt.Sprintf("%d. nota-%02d", router.NotesRenderCap, router.NotesRenderCap)) {
		t.Fatalf("report misses numbered entries up to the render cap:\n%q", body)
	}
	if strings.Contains(body, fmt.Sprintf("nota-%02d", router.NotesRenderCap+1)) {
		t.Fatalf("report rendered beyond NotesRenderCap:\n%q", body)
	}
	if !strings.Contains(body, fmt.Sprintf(router.NotesMoreSuffixFormat, 5)) {
		t.Fatalf("report misses the honest %q suffix:\n%q", fmt.Sprintf(router.NotesMoreSuffixFormat, 5), body)
	}
	if report.Meta[envelope.MetaAck] != envelope.AckNotesReport {
		t.Fatalf("report MetaAck = %q, want %q", report.Meta[envelope.MetaAck], envelope.AckNotesReport)
	}
	if n := len(br.Handled()); n != 0 {
		t.Fatalf("brain invoked %d times for /notes, want 0 (zero model)", n)
	}
}

func TestNotes_EmptyScopeReportsEmpty(t *testing.T) {
	backend := &fakeNotesBackend{ok: true}
	r, ch, _ := notesRouter(t, backend)

	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/notes")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "the empty reply", func() bool { return len(ch.Sent()) == 1 })
	if got := ch.Sent()[0].Parts[0].Content; got != router.NotesEmptyReply {
		t.Fatalf("reply = %q, want the fixed %q", got, router.NotesEmptyReply)
	}
}

// AS-B13: clear + a fresh /notes reports empty, zero model throughout.
func TestNotes_ClearThenFreshListEmpty(t *testing.T) {
	backend := &fakeNotesBackend{notes: notesOf(3), ok: true}
	r, ch, br := notesRouter(t, backend)

	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/notes clear")); err != nil {
		t.Fatalf("DispatchInbound clear: %v", err)
	}
	waitUntil(t, "the cleared ack", func() bool { return len(ch.Sent()) == 1 })
	if got := ch.Sent()[0].Parts[0].Content; got != router.NotesClearedAck {
		t.Fatalf("clear ack = %q, want %q", got, router.NotesClearedAck)
	}
	if _, clears := backend.counts(); clears != 1 {
		t.Fatalf("clear closure called %d times, want 1", clears)
	}

	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/notes")); err != nil {
		t.Fatalf("DispatchInbound list: %v", err)
	}
	waitUntil(t, "the fresh empty reply", func() bool { return len(ch.Sent()) == 2 })
	if got := ch.Sent()[1].Parts[0].Content; got != router.NotesEmptyReply {
		t.Fatalf("fresh /notes after clear = %q, want %q", got, router.NotesEmptyReply)
	}
	if n := len(br.Handled()); n != 0 {
		t.Fatalf("brain invoked %d times, want 0", n)
	}
}

func TestNotes_UnknownArgumentIsFixedUsage(t *testing.T) {
	backend := &fakeNotesBackend{notes: notesOf(2), ok: true}
	r, ch, _ := notesRouter(t, backend)

	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/notes foo")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "the usage reply", func() bool { return len(ch.Sent()) == 1 })
	if got := ch.Sent()[0].Parts[0].Content; got != router.NotesUsageReply {
		t.Fatalf("reply = %q, want %q", got, router.NotesUsageReply)
	}
	if _, clears := backend.counts(); clears != 0 {
		t.Fatalf("garbage argument reached the clear closure (%d), want 0", clears)
	}
}

// AS-B13: an unconfigured brain (ok=false) FALLS THROUGH to the model like
// any unknown command — documented behavior.
func TestNotes_UnconfiguredBrainFallsThrough(t *testing.T) {
	backend := &fakeNotesBackend{ok: false}
	r, _, br := notesRouter(t, backend)

	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/notes")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "the brain to receive the fallthrough", func() bool { return len(br.Handled()) == 1 })
	got := br.Handled()[0]
	if len(got.Parts) != 1 || got.Parts[0].Content != "/notes" {
		t.Fatalf("brain received %+v, want the untouched \"/notes\" text", got.Parts)
	}
}

// Store failures are honest: the fixed reply + a structured session error,
// never silence, never the model.
func TestNotes_StoreErrorHonestReply(t *testing.T) {
	backend := &fakeNotesBackend{ok: true, listErr: errors.New("notes store down")}
	var mu sync.Mutex
	var captured []router.RouterError
	r, ch, br := sessionRouter(t, conversation.NewMemStore(),
		router.WithNotesCommands(backend.list, backend.clear),
		router.WithErrorHandler(func(e router.RouterError) {
			mu.Lock()
			captured = append(captured, e)
			mu.Unlock()
		}))

	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/notes")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "the fixed error reply", func() bool { return len(ch.Sent()) == 1 })
	if got := ch.Sent()[0].Parts[0].Content; got != router.NotesErrorReply {
		t.Fatalf("reply = %q, want %q", got, router.NotesErrorReply)
	}
	if n := len(br.Handled()); n != 0 {
		t.Fatalf("store failure leaked /notes to the brain (%d), want 0", n)
	}
	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, e := range captured {
		if e.Kind == router.ErrKindSession && e.Err != nil && strings.Contains(e.Err.Error(), "notes store down") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no structured session error for the failed /notes; captured = %+v", captured)
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/conversation/sqlite"
)

// TestOrchestrator_persistTurns_survivesCancelledContext is the DECISION A guard
// (ADR-0019 §6 durability note): the router cancels its context on shutdown, so
// persistTurns must write on a DETACHED context — the final reply's turns must
// land even when the ctx it was handed is already cancelled. Proven against the
// durable store: persist under a cancelled ctx, then reopen the DB file and
// confirm the pair survived (a non-detached impl rolls the transaction back and
// the reopen finds nothing).
func TestOrchestrator_persistTurns_survivesCancelledContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "korvun.db")
	store, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	o := NewOrchestrator(nil, nil, nil, WithConversationStore(store, 10), WithLogger(quietLogger()))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // a graceful shutdown has already cancelled the router context

	const key = conversation.Key("telegram::shutdown")
	o.persistTurns(ctx, key, "last question", "last answer")

	// Reopen the DB file: the turns must have survived the cancelled context.
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	store2, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = store2.Close() }()

	got, err := store2.LoadRecent(context.Background(), key, 10)
	if err != nil {
		t.Fatalf("LoadRecent after reopen: %v", err)
	}
	if len(got) != 2 || got[0].Content != "last question" || got[1].Content != "last answer" {
		t.Fatalf("after cancelled-ctx persist + reopen got %+v, want both turns "+
			"(the shutdown cancellation aborted the durable write)", got)
	}
}

// TestOrchestrator_persistTurns_cancelledContext_memStoreUnaffected confirms the
// detach is benign for the in-memory store: MemStore has no transaction to abort,
// so the turns persist under a cancelled ctx exactly as before (no regression on
// the ADR-0018 path).
func TestOrchestrator_persistTurns_cancelledContext_memStoreUnaffected(t *testing.T) {
	store := conversation.NewMemStore()
	o := NewOrchestrator(nil, nil, nil, WithConversationStore(store, 10), WithLogger(quietLogger()))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	const key = conversation.Key("telegram::mem")
	o.persistTurns(ctx, key, "q", "a")

	got, err := store.LoadRecent(context.Background(), key, 10)
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	if len(got) != 2 || got[0].Content != "q" || got[1].Content != "a" {
		t.Fatalf("MemStore under cancelled-ctx persist got %+v, want both turns", got)
	}
}

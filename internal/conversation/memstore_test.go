// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package conversation_test

import (
	"context"
	"sync"
	"testing"

	"github.com/Sebastian197/korvun/internal/conversation"
)

func TestMemStore_AppendAssignsMonotonicSeqAndReturnsTurn(t *testing.T) {
	s := conversation.NewMemStore()
	ctx := context.Background()
	const key = conversation.Key("telegram::1")

	for i := 0; i < 3; i++ {
		got, err := s.Append(ctx, key, conversation.Turn{Role: conversation.RoleUser, Content: "hi"})
		if err != nil {
			t.Fatalf("Append: %v", err)
		}
		if got.Seq != i {
			t.Errorf("Seq = %d, want %d", got.Seq, i)
		}
		if got.Content != "hi" {
			t.Errorf("Content = %q, want %q", got.Content, "hi")
		}
	}
}

func TestMemStore_LoadRecent(t *testing.T) {
	s := conversation.NewMemStore()
	ctx := context.Background()
	const key = conversation.Key("telegram::1")
	for _, c := range []string{"a", "b", "c", "d"} {
		if _, err := s.Append(ctx, key, conversation.Turn{Role: conversation.RoleUser, Content: c}); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	t.Run("returns last n oldest-first", func(t *testing.T) {
		got, err := s.LoadRecent(ctx, key, 2)
		if err != nil {
			t.Fatalf("LoadRecent: %v", err)
		}
		if len(got) != 2 || got[0].Content != "c" || got[1].Content != "d" {
			t.Errorf("got %+v, want [c d]", got)
		}
	})

	t.Run("n larger than history returns all", func(t *testing.T) {
		got, err := s.LoadRecent(ctx, key, 100)
		if err != nil {
			t.Fatalf("LoadRecent: %v", err)
		}
		if len(got) != 4 {
			t.Errorf("len = %d, want 4", len(got))
		}
	})

	t.Run("n<=0 returns no turns", func(t *testing.T) {
		got, err := s.LoadRecent(ctx, key, 0)
		if err != nil {
			t.Fatalf("LoadRecent: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})

	t.Run("unknown key returns empty, no error", func(t *testing.T) {
		got, err := s.LoadRecent(ctx, conversation.Key("nope"), 5)
		if err != nil {
			t.Fatalf("LoadRecent: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})

	t.Run("returned slice is a copy", func(t *testing.T) {
		got, err := s.LoadRecent(ctx, key, 1)
		if err != nil {
			t.Fatalf("LoadRecent: %v", err)
		}
		got[0].Content = "mutated"
		again, _ := s.LoadRecent(ctx, key, 1)
		if again[0].Content == "mutated" {
			t.Error("mutating the returned slice mutated stored state")
		}
	})
}

// TestMemStore_ConcurrentAppendSameKey is the load-bearing contract test
// (ADR-0018 §7). The router does not serialize a conversation, so N goroutines
// may Append to the SAME key concurrently. The contract: no committed write is
// lost. After N concurrent Appends the key must hold exactly N turns with Seq
// values forming the contiguous set 0..N-1 (no gap, no duplicate). A non-locked
// map implementation fails this under -race and/or loses writes.
//
// Run with: go test -race -count=10 ./internal/conversation/
func TestMemStore_ConcurrentAppendSameKey(t *testing.T) {
	s := conversation.NewMemStore()
	ctx := context.Background()
	const key = conversation.Key("telegram::race")
	const n = 100

	var wg sync.WaitGroup
	wg.Add(n)
	returnedSeq := make([]int, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			got, err := s.Append(ctx, key, conversation.Turn{Role: conversation.RoleUser, Content: "x"})
			if err != nil {
				t.Errorf("Append: %v", err)
				return
			}
			returnedSeq[i] = got.Seq
		}(i)
	}
	wg.Wait()

	turns, err := s.LoadRecent(ctx, key, n*2)
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	if len(turns) != n {
		t.Fatalf("got %d turns, want %d (lost writes under concurrency)", len(turns), n)
	}

	// Stored Seqs must be exactly 0..n-1, each once.
	seen := make([]bool, n)
	for _, tr := range turns {
		if tr.Seq < 0 || tr.Seq >= n {
			t.Fatalf("Seq %d out of range [0,%d)", tr.Seq, n)
		}
		if seen[tr.Seq] {
			t.Fatalf("duplicate Seq %d", tr.Seq)
		}
		seen[tr.Seq] = true
	}
	for i, ok := range seen {
		if !ok {
			t.Fatalf("missing Seq %d (gap in sequence)", i)
		}
	}

	// Returned Seqs must also be unique (every Append got its own slot).
	seenRet := make([]bool, n)
	for _, sq := range returnedSeq {
		if sq < 0 || sq >= n || seenRet[sq] {
			t.Fatalf("returned Seq %d is out of range or duplicated", sq)
		}
		seenRet[sq] = true
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package conversation

import (
	"context"
	"sync"
)

// Compile-time assertion that *MemStore satisfies the Store seam.
var _ Store = (*MemStore)(nil)

// MemStore is the in-memory Store: a map of Key to its ordered turns, guarded by
// a single mutex. It is the permanent test double for the Orchestrator AND the
// enforcer of the Store concurrency contract under -race — not a discardable
// prototype. It holds no goroutines of its own; the only delicate thing is the
// lock discipline that makes Append atomic per key.
//
// Memory is unbounded in ADR-A (no eviction); durable storage and compaction
// arrive in ADR-B. Construct with NewMemStore.
type MemStore struct {
	mu sync.Mutex
	m  map[Key][]Turn
}

// NewMemStore returns an empty, ready-to-use in-memory Store.
func NewMemStore() *MemStore {
	return &MemStore{m: make(map[Key][]Turn)}
}

// Append atomically appends a single turn to key, assigning its Seq. It
// delegates to AppendTurns so the Seq-assignment logic lives in one place.
func (s *MemStore) Append(ctx context.Context, key Key, turn Turn) (Turn, error) {
	out, err := s.AppendTurns(ctx, key, turn)
	if err != nil {
		return Turn{}, err
	}
	return out[0], nil
}

// AppendTurns atomically appends a group of turns to key under a single lock,
// assigning consecutive Seq values (the next indices in that key's history) and
// returning the stored turns. Holding the lock across the whole group is what
// keeps the group contiguous: two concurrent AppendTurns to the same key cannot
// interleave their turns, so a user+assistant pair never gets split (ADR-0018
// §1, §7; reconciliation note). An empty group is a no-op.
func (s *MemStore) AppendTurns(_ context.Context, key Key, turns ...Turn) ([]Turn, error) {
	if len(turns) == 0 {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Turn, len(turns))
	base := len(s.m[key])
	for i, t := range turns {
		t.Seq = base + i
		s.m[key] = append(s.m[key], t)
		out[i] = t
	}
	return out, nil
}

// LoadRecent returns a copy of up to the last n turns for key, oldest first, so
// the caller cannot mutate stored state. n <= 0 or an unknown key returns no
// turns (ADR-0018 §1).
func (s *MemStore) LoadRecent(_ context.Context, key Key, n int) ([]Turn, error) {
	if n <= 0 {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	turns := s.m[key]
	if len(turns) == 0 {
		return nil, nil
	}
	start := len(turns) - n
	if start < 0 {
		start = 0
	}
	recent := turns[start:]
	out := make([]Turn, len(recent))
	copy(out, recent)
	return out, nil
}

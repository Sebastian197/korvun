// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package conversation

import (
	"context"
	"sort"
	"sync"
)

// Compile-time assertions: *MemStore satisfies the Store seam and its
// sessionful superset (operator-console spec SP1).
var (
	_ Store        = (*MemStore)(nil)
	_ SessionStore = (*MemStore)(nil)
)

// memSession is one session of one key: its id and its ordered turns.
type memSession struct {
	id    int
	turns []Turn
}

// MemStore is the in-memory SessionStore: a map of Key to its ordered
// sessions (the last one is ACTIVE), guarded by a single mutex. It is the
// permanent test double for the Orchestrator AND the enforcer of the Store
// concurrency contract under -race — not a discardable prototype. It holds no
// goroutines of its own; the only delicate thing is the lock discipline that
// makes Append atomic per key.
//
// Sessions (operator-console spec SP1): every key's turns live inside
// sessions with store-assigned monotonic ids; the Store methods operate on
// the active (highest) session, so a NewSession is a hard context cut for
// LoadRecent while old sessions stay readable via ListSessions/LoadSession.
//
// Memory is unbounded in ADR-A (no eviction); compaction is explicitly out of
// the operator-console spec. Construct with NewMemStore.
type MemStore struct {
	mu sync.Mutex
	m  map[Key][]*memSession
}

// NewMemStore returns an empty, ready-to-use in-memory SessionStore.
func NewMemStore() *MemStore {
	return &MemStore{m: make(map[Key][]*memSession)}
}

// activeLocked returns the key's active session, creating session 1 when the
// key has no history at all. Callers hold s.mu.
func (s *MemStore) activeLocked(key Key) *memSession {
	sessions := s.m[key]
	if len(sessions) == 0 {
		sess := &memSession{id: 1}
		s.m[key] = []*memSession{sess}
		return sess
	}
	return sessions[len(sessions)-1]
}

// Append atomically appends a single turn to key's active session, assigning
// its Seq. It delegates to AppendTurns so the Seq-assignment logic lives in
// one place.
func (s *MemStore) Append(ctx context.Context, key Key, turn Turn) (Turn, error) {
	out, err := s.AppendTurns(ctx, key, turn)
	if err != nil {
		return Turn{}, err
	}
	return out[0], nil
}

// AppendTurns atomically appends a group of turns to key's ACTIVE session
// under a single lock, assigning consecutive Seq values (the next indices in
// that session's history) and returning the stored turns. Holding the lock
// across the whole group is what keeps the group contiguous: two concurrent
// AppendTurns to the same key cannot interleave their turns, so a
// user+assistant pair never gets split (ADR-0018 §1, §7; reconciliation
// note). An empty group is a no-op.
func (s *MemStore) AppendTurns(_ context.Context, key Key, turns ...Turn) ([]Turn, error) {
	if len(turns) == 0 {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.activeLocked(key)
	out := make([]Turn, len(turns))
	base := len(sess.turns)
	for i, t := range turns {
		t.Seq = base + i
		sess.turns = append(sess.turns, t)
		out[i] = t
	}
	return out, nil
}

// LoadRecent returns a copy of up to the last n turns of key's ACTIVE session,
// oldest first, so the caller cannot mutate stored state. A session reset is
// therefore a hard context cut (FR-SESS-2): previous sessions never surface
// here. n <= 0 or an unknown key returns no turns (ADR-0018 §1).
func (s *MemStore) LoadRecent(_ context.Context, key Key, n int) ([]Turn, error) {
	if n <= 0 {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sessions := s.m[key]
	if len(sessions) == 0 {
		return nil, nil
	}
	turns := sessions[len(sessions)-1].turns
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

// NewSession opens a fresh session for key and returns its id (SessionStore).
// Idempotent on an empty active session; a key with no history returns 1.
func (s *MemStore) NewSession(_ context.Context, key Key) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sessions := s.m[key]
	if len(sessions) == 0 {
		s.m[key] = []*memSession{{id: 1}}
		return 1, nil
	}
	active := sessions[len(sessions)-1]
	if len(active.turns) == 0 {
		return active.id, nil
	}
	next := &memSession{id: active.id + 1}
	s.m[key] = append(sessions, next)
	return next.id, nil
}

// ListConversations lists up to limit conversations, most recent activity
// first (SessionStore). LastActivity/LastRole come from the key's most recent
// turn across sessions (zero values for a key holding only empty sessions).
// Ties order by key for determinism.
func (s *MemStore) ListConversations(_ context.Context, limit int) ([]ConversationInfo, error) {
	if limit <= 0 {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ConversationInfo, 0, len(s.m))
	for key, sessions := range s.m {
		info := ConversationInfo{
			Key:           key,
			ActiveSession: sessions[len(sessions)-1].id,
			SessionCount:  len(sessions),
		}
		// The most recent turn: sessions are ordered, so scan from the back
		// for the last non-empty one.
		for i := len(sessions) - 1; i >= 0; i-- {
			if n := len(sessions[i].turns); n > 0 {
				last := sessions[i].turns[n-1]
				info.LastActivity = last.Timestamp
				info.LastRole = last.Role
				break
			}
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].LastActivity.Equal(out[j].LastActivity) {
			return out[i].LastActivity.After(out[j].LastActivity)
		}
		return out[i].Key < out[j].Key
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ListSessions lists every session of key, oldest first (SessionStore). An
// unknown key returns an empty slice, not an error.
func (s *MemStore) ListSessions(_ context.Context, key Key) ([]SessionInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sessions := s.m[key]
	out := make([]SessionInfo, 0, len(sessions))
	for _, sess := range sessions {
		info := SessionInfo{ID: sess.id, TurnCount: len(sess.turns)}
		if n := len(sess.turns); n > 0 {
			info.First = sess.turns[0].Timestamp
			info.Last = sess.turns[n-1].Timestamp
		}
		out = append(out, info)
	}
	return out, nil
}

// LoadSession returns a copy of ALL turns of the given session of key, oldest
// first (SessionStore). An unknown key or session returns an empty slice, not
// an error.
func (s *MemStore) LoadSession(_ context.Context, key Key, session int) ([]Turn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sess := range s.m[key] {
		if sess.id == session {
			out := make([]Turn, len(sess.turns))
			copy(out, sess.turns)
			return out, nil
		}
	}
	return nil, nil
}

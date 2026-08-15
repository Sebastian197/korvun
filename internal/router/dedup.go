// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"container/list"
	"errors"
	"sync"
	"time"

	"github.com/Sebastian197/korvun/internal/envelope"
)

// Inbound deduplication (audit finding R-1). The at-least-once channels can
// hand the router the same event twice: Telegram re-delivers after a crash
// between receipt and offset advance, a Discord resume replays the gap, a
// webhook sender retries its POST. DispatchInbound therefore keeps a bounded
// LRU+TTL window keyed by channel + Meta[envelope.MetaProviderEventID] and
// drops the second delivery as a COUNTED, observable event — never silently,
// and never as an error to the caller (the channel did nothing wrong).
//
// The window is deliberately in-memory only: a restart forgets it, and one
// duplicate answered after a restart costs less than dragging a durable dedup
// store through every deployment. An envelope WITHOUT an event id is never
// deduplicated — fail-open, missing metadata must not discard a message. The
// capacity and TTL are house constants; they become configuration only if a
// real deployment asks for it.

// DedupCapacity is the maximum number of event ids the window remembers.
// When full, the least-recently-seen id is evicted.
const DedupCapacity = 4096

// DedupTTL is how long a seen event id keeps suppressing duplicates. An
// entry older than the TTL is treated as unseen (the window is a bounded
// memory, not a permanent ledger).
const DedupTTL = 10 * time.Minute

// ErrDuplicateEvent is the cause carried by the MessageDropped event a
// duplicate delivery produces. It never reaches DispatchInbound's caller.
var ErrDuplicateEvent = errors.New("router: duplicate event dropped")

// WithDedupCounter sets the per-channel counter the router invokes once per
// deduplicated (dropped) duplicate. The app wires this to the
// korvun_deduped_total metric; nil (the default) counts nothing. Kept as a
// plain func seam so the router does not grow a metrics dependency.
func WithDedupCounter(f func(channel string)) Option {
	return func(r *Router) { r.dedupCounter = f }
}

// dedupEntry is one remembered event id inside the window.
type dedupEntry struct {
	key  string
	seen time.Time
}

// dedupWindow is the bounded LRU+TTL memory of recently seen event ids. It
// is safe for concurrent use (DispatchInbound runs from N channel pumps).
type dedupWindow struct {
	mu    sync.Mutex
	cap   int
	ttl   time.Duration
	now   func() time.Time
	order *list.List               // front = most recently seen
	byKey map[string]*list.Element // key -> element whose Value is *dedupEntry
}

func newDedupWindow(capacity int, ttl time.Duration, now func() time.Time) *dedupWindow {
	return &dedupWindow{
		cap:   capacity,
		ttl:   ttl,
		now:   now,
		order: list.New(),
		byKey: make(map[string]*list.Element, capacity),
	}
}

// seen reports whether key was already recorded within the TTL, recording it
// as most-recently-seen either way. A fresh or expired key is (re)admitted
// and returns false; a live key returns true (duplicate).
func (w *dedupWindow) seen(key string) bool {
	now := w.now()
	w.mu.Lock()
	defer w.mu.Unlock()

	if el, ok := w.byKey[key]; ok {
		entry := el.Value.(*dedupEntry)
		if now.Sub(entry.seen) <= w.ttl {
			// Live duplicate: refresh recency so a replay storm cannot age
			// the id out mid-storm.
			entry.seen = now
			w.order.MoveToFront(el)
			return true
		}
		// Expired: readmit as fresh.
		entry.seen = now
		w.order.MoveToFront(el)
		return false
	}

	if w.order.Len() >= w.cap {
		oldest := w.order.Back()
		if oldest != nil {
			w.order.Remove(oldest)
			delete(w.byKey, oldest.Value.(*dedupEntry).key)
		}
	}
	w.byKey[key] = w.order.PushFront(&dedupEntry{key: key, seen: now})
	return false
}

// dedupKey builds the window key for an envelope, or "" when the envelope
// carries no provider event id (meaning: do not deduplicate).
func dedupKey(env *envelope.Envelope) string {
	id := env.Meta[envelope.MetaProviderEventID]
	if id == "" {
		return ""
	}
	// The channel is part of the key: the same numeric id on two channels is
	// two different events.
	return env.Channel + "\x00" + id
}

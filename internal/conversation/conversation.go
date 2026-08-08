// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package conversation owns the conversation-memory domain: the canonical
// conversation Key, the Turn record, the Role of a turn, and the Store seam the
// Brain reads from before a dispatch and writes to after a reply (ADR-0018,
// Stage 9 ADR-A).
//
// It is a leaf: it depends only on internal/envelope. The router and the brain
// both depend on conversation; nothing in conversation depends back, so the key
// composition lives here once and the router delegates to it (which is why
// router.ConversationKey and router.MetaConversationID are thin aliases).
package conversation

import (
	"context"
	"errors"
	"time"

	"github.com/Sebastian197/korvun/internal/envelope"
)

// MetaConversationID is the Envelope.Meta key under which a channel adapter
// records the conversation (chat) identifier. It is the canonical home for the
// constant; internal/router aliases it for backward compatibility.
const MetaConversationID = "conversation.id"

// ErrNoConversationID is returned by KeyFromEnvelope when the envelope is nil or
// carries no conversation id under MetaConversationID. internal/router aliases
// this value, so router.ErrNoConversationID and conversation.ErrNoConversationID
// are the same error and errors.Is treats them identically.
var ErrNoConversationID = errors.New(`conversation: envelope is missing Meta["conversation.id"]`)

// Key is the conversation identity: the channel name joined to the conversation
// id with "::". It is a named type so the Store seam cannot be called with an
// arbitrary string. Build it only via KeyFromEnvelope.
type Key string

// Role is the author of a Turn. It is kept dependency-free (a plain string, not
// model.Role) so conversation stays a leaf; the Orchestrator translates between
// Role and model roles.
type Role string

// The recognised turn authors.
const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
	// RoleOperator marks a turn written by a human operator from the console
	// (operator-console spec 2026-08-08, FR-STORE-2). It is persisted as-is —
	// the history stays honest about who spoke — and the brains' provider
	// translation maps it to the assistant role (brain.toModelRole).
	RoleOperator Role = "operator"
)

// Turn is one message in a conversation's history. Timestamp and Seq are carried
// from day one (even though ADR-A uses neither for retention) so a future
// compaction / retention query is an additive read, not a schema migration. Seq
// is assigned by the Store on Append; callers leave it zero.
//
// INVARIANT: Turn is value-only — every field is a value type (no pointer, slice,
// or map). MemStore.LoadRecent relies on this: it returns turns via a shallow
// copy, which fully detaches the caller from stored state only because there is
// nothing to share by reference. Do NOT add a pointer/slice/map field without
// making LoadRecent deep-copy it, or callers could mutate stored history.
type Turn struct {
	Role      Role
	Content   string
	Timestamp time.Time
	Seq       int
}

// Store persists and retrieves the turns of a conversation.
//
// Implementations MUST be safe for concurrent use by multiple goroutines,
// INCLUDING concurrent Append on the same Key. The router does not serialize a
// conversation (N workers, no per-conversation affinity), so two goroutines may
// Append to the same Key simultaneously; no committed turn may be lost. This is
// the same concurrency discipline model.Model and the fan-out carry.
type Store interface {
	// LoadRecent returns up to the last n turns for key, oldest first. It is a
	// best-effort snapshot: it MAY omit a turn a concurrent Append has not yet
	// committed (acceptable for building reply context), but it never loses a
	// committed write. n <= 0 returns no turns. An unknown key returns an empty
	// slice. Neither is an error.
	LoadRecent(ctx context.Context, key Key, n int) ([]Turn, error)

	// Append atomically adds one turn to key and returns it with its
	// store-assigned Seq filled in (callers never set Seq). Concurrent Appends
	// to the same key are serialized by the implementation. For a group of turns
	// that must stay together (a user+assistant pair), use AppendTurns — a single
	// Append per turn does NOT keep the pair contiguous under concurrency.
	Append(ctx context.Context, key Key, turn Turn) (Turn, error)

	// AppendTurns atomically appends a group of turns to key under a single
	// critical section, assigning them consecutive Seq values (the next indices
	// in the key's history) and returning them Seq-filled. It is the only way to
	// guarantee a group stays contiguous and ordered: when two messages of the
	// same conversation are persisted concurrently (the router does not serialize
	// a conversation — N workers, no per-conversation affinity), their groups do
	// not interleave, so a user+assistant pair never ends up split by another
	// message's turn (which would yield a non-alternating, provider-rejected
	// history). The order BETWEEN concurrent groups may vary; each group stays
	// intact. An empty group is a no-op returning (nil, nil).
	AppendTurns(ctx context.Context, key Key, turns ...Turn) ([]Turn, error)
}

// ConversationInfo is one row of the inbox listing (operator-console spec
// FR-STORE-1): a conversation key with its session bookkeeping. LastActivity
// is the timestamp of the key's most recent turn (zero when the conversation
// has no turns yet); LastRole is that turn's author ("" likewise).
type ConversationInfo struct {
	Key           Key
	ActiveSession int
	SessionCount  int
	LastActivity  time.Time
	LastRole      Role
}

// SessionInfo describes one session of a conversation (FR-SESS-6 /
// FR-API-1b): its id, how many turns it holds, and the timestamps of its
// first and last turns (zero for an empty session).
type SessionInfo struct {
	ID        int
	TurnCount int
	First     time.Time
	Last      time.Time
}

// SessionStore is the sessionful store seam (operator-console spec,
// FR-SESS-1/2/6 + FR-STORE-1). It EMBEDS Store — the additive-seam
// discipline: every existing Store consumer (the brains, the router, their
// fakes) keeps compiling untouched, while session-aware consumers (the
// dispatch layer's reset handling, the control API's inbox) ask for the
// superset.
//
// Sessions are store-assigned, monotonic per key (1, 2, 3…); the ACTIVE
// session of a key is the highest one, and the Store methods operate on it:
// Append/AppendTurns write to it, LoadRecent reads ONLY from it (a session
// reset is therefore a hard context cut — FR-SESS-2). Old sessions stay
// stored and readable via ListSessions/LoadSession, never fed to LoadRecent
// again.
type SessionStore interface {
	Store

	// NewSession closes the key's active session by opening a fresh one and
	// returns the new active session id. On a key whose active session holds
	// zero turns it is idempotent: it returns the current (still empty)
	// active session instead of stacking empty sessions. On a key with no
	// history at all it returns 1.
	NewSession(ctx context.Context, key Key) (int, error)

	// ListConversations lists up to limit conversations, most recent activity
	// first. limit <= 0 returns nothing; an empty store returns an empty
	// slice. Neither is an error.
	ListConversations(ctx context.Context, limit int) ([]ConversationInfo, error)

	// ListSessions lists every session of key, oldest first (ascending id).
	// An unknown key returns an empty slice, not an error.
	ListSessions(ctx context.Context, key Key) ([]SessionInfo, error)

	// LoadSession returns ALL turns of the given session of key, oldest
	// first. An unknown key or session returns an empty slice, not an error.
	LoadSession(ctx context.Context, key Key, session int) ([]Turn, error)
}

// KeyFromEnvelope derives the canonical conversation Key from an inbound
// envelope: Channel + "::" + Meta[MetaConversationID]. It returns
// ErrNoConversationID (and an empty Key) when the envelope is nil or the
// conversation id is absent or empty — the same condition the router rejects
// before dispatch. This is the single definition of the key composition.
func KeyFromEnvelope(env *envelope.Envelope) (Key, error) {
	if env == nil {
		return "", ErrNoConversationID
	}
	id := env.Meta[MetaConversationID]
	if id == "" {
		return "", ErrNoConversationID
	}
	return Key(env.Channel + "::" + id), nil
}

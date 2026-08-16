// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package conversation

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// This file is the persistent-notes domain (minimal-memory spec FR-STORE-1,
// ADR-0043 §3): an explicit scope enum, the NoteStore seam whose AppendNote
// enforces the count cap ATOMICALLY and rejects incoherent scope/key pairs,
// and the ONE pure scope-derivation function internal/app composes every
// memory closure from — write path, read path and the /notes commands can
// never drift.

// NoteScope is the declared scope of a brain's notes (spec D2). The zero
// value is deliberately invalid so an unconfigured scope fails loud — the
// Sensitivity discipline (ADR-0015).
type NoteScope int

const (
	// ScopeConversation binds notes to one conversation Key: they re-enter
	// only the conversation that produced them (the default).
	ScopeConversation NoteScope = iota + 1
	// ScopeBrainGlobal binds notes to the brain across conversations — the
	// explicit all-local opt-in (FR-PRIV-1).
	ScopeBrainGlobal
)

// Note is one stored note. Seq is store-assigned and monotonic per scope;
// Timestamp is stamped by the STORE (the NewSession precedent).
type Note struct {
	Seq       int
	Content   string
	Timestamp time.Time
}

// ErrNotesFull marks an AppendNote refused at the count cap: nothing was
// stored (refuse-when-full, never eviction — ADR-0043).
var ErrNotesFull = errors.New("conversation: note box full")

// ErrInvalidNoteScope marks an incoherent scope/key pair (ScopeConversation
// with an empty Key, ScopeBrainGlobal with a non-empty Key) or the invalid
// zero scope — an upstream derivation failure must never become a silent
// global write (FR-STORE-1).
var ErrInvalidNoteScope = errors.New("conversation: invalid note scope/key pair")

// NoteStore is the notes seam (FR-STORE-1). Both stores implement it; it is
// deliberately separate from Store/SessionStore (additive, zero blast radius
// on existing consumers).
type NoteStore interface {
	// AppendNote stores one note for (brain, scope, key). The count cap is
	// enforced ATOMICALLY inside (count+insert in one transaction on the
	// serialized writer): at maxNotes the typed ErrNotesFull comes back and
	// nothing is stored. The store stamps Timestamp. Incoherent scope/key
	// pairs are rejected with ErrInvalidNoteScope.
	AppendNote(ctx context.Context, brain string, scope NoteScope, key Key, content string, maxNotes int) (Note, error)
	// ListNotes returns the scope's notes oldest-first. An unknown scope
	// returns an empty slice, not an error.
	ListNotes(ctx context.Context, brain string, scope NoteScope, key Key) ([]Note, error)
	// ClearNotes removes every note of the scope. An unknown scope is a
	// no-op, not an error.
	ClearNotes(ctx context.Context, brain string, scope NoteScope, key Key) error
}

// EffectiveNoteScope is THE single pure scope derivation (FR-STORE-1,
// H2's resolution): the configured scope plus the envelope's conversation
// Key yield the effective (scope, key) pair every memory closure uses.
// ScopeConversation with a non-empty key passes through; ScopeBrainGlobal
// yields the empty key whatever the input; ScopeConversation with an empty
// key — no conversation identity — is ErrInvalidNoteScope, never a silent
// global write.
func EffectiveNoteScope(configured NoteScope, key Key) (NoteScope, Key, error) {
	switch configured {
	case ScopeConversation:
		if key == "" {
			return 0, "", fmt.Errorf("%w: conversation scope without a conversation key", ErrInvalidNoteScope)
		}
		return ScopeConversation, key, nil
	case ScopeBrainGlobal:
		return ScopeBrainGlobal, "", nil
	default:
		return 0, "", fmt.Errorf("%w: unknown scope %d", ErrInvalidNoteScope, int(configured))
	}
}

// CheckNotePair is the store-side coherence validation both NoteStore
// implementations share (FR-STORE-1): the pair must be EXACTLY what
// EffectiveNoteScope can produce — ScopeConversation with a non-empty key,
// or ScopeBrainGlobal with the empty key. Anything else is the typed
// ErrInvalidNoteScope.
func CheckNotePair(scope NoteScope, key Key) error {
	switch scope {
	case ScopeConversation:
		if key == "" {
			return fmt.Errorf("%w: conversation scope with an empty key", ErrInvalidNoteScope)
		}
	case ScopeBrainGlobal:
		if key != "" {
			return fmt.Errorf("%w: brain-global scope with a non-empty key %q", ErrInvalidNoteScope, key)
		}
	default:
		return fmt.Errorf("%w: unknown scope %d", ErrInvalidNoteScope, int(scope))
	}
	return nil
}

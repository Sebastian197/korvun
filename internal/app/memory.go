// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/router"
)

// This file is the app half of the minimal-memory composition (ADR-0043
// §3/§7): every memory closure — the tool's writer (agentTool), the brain's
// loader (buildAgentBrain) and the /notes list/clear below — is composed
// over the SAME pure derivation (conversation.EffectiveNoteScope) and the
// SAME NoteStore, so write path, read path and the operator commands can
// never drift (H2's resolution).

// noteScopeOf maps the resolved memory settings to the configured scope.
func noteScopeOf(s config.MemorySettings) conversation.NoteScope {
	if s.BrainGlobal {
		return conversation.ScopeBrainGlobal
	}
	return conversation.ScopeConversation
}

// notesClosures composes the /notes list/clear closures (FR-RECALL-2) over
// the config's memory brains and the shared NoteStore. ok=false for a brain
// without memory — the router lets the token fall through to the model.
// any=false when no brain has memory at all (the commands stay unmounted).
func notesClosures(cfg *config.Config, ns conversation.NoteStore) (router.NotesLister, router.NotesClearer, bool) {
	scopes := make(map[string]conversation.NoteScope)
	for _, bc := range cfg.Brains {
		if bc.Agent != nil && bc.Agent.Memory != nil {
			scopes[bc.Name] = noteScopeOf(bc.Agent.Memory.Settings())
		}
	}
	if len(scopes) == 0 {
		return nil, nil, false
	}
	list := func(ctx context.Context, brainName string, key conversation.Key) ([]conversation.Note, bool, error) {
		scopeCfg, ok := scopes[brainName]
		if !ok {
			return nil, false, nil
		}
		scope, ekey, err := conversation.EffectiveNoteScope(scopeCfg, key)
		if err != nil {
			return nil, true, err
		}
		notes, err := ns.ListNotes(ctx, brainName, scope, ekey)
		return notes, true, err
	}
	clear := func(ctx context.Context, brainName string, key conversation.Key) (bool, error) {
		scopeCfg, ok := scopes[brainName]
		if !ok {
			return false, nil
		}
		scope, ekey, err := conversation.EffectiveNoteScope(scopeCfg, key)
		if err != nil {
			return true, err
		}
		return true, ns.ClearNotes(ctx, brainName, scope, ekey)
	}
	return list, clear, true
}

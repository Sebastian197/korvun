// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package shell

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// EnsureChatBlocks upgrades an existing config at path for the chat piece
// (operator-console close, 2026-08-08): the Chat tab requires the storage
// and session blocks, and a config written before the piece lacks them, so
// the desktop runs this right after EnsureDefaultConfig on mount — fresh
// installs get the blocks from the template, existing installs get exactly
// the missing ones added here (as empty blocks: the DB path and the reset
// triggers resolve to their defaults at boot). The file is read as a raw
// JSON object so fields this build does not know survive verbatim; the
// rewrite normalizes layout (canonical MarshalIndent, keys sorted) but
// never drops or edits a value. When both blocks are already present the
// file is not touched at all — not even an identical rewrite — keeping
// FR-FIRST-2's steady-state guarantee. The write is atomic (temp file +
// rename in the same directory), the WriteConfigAtomic discipline.
func EnsureChatBlocks(path string) (changed bool, err error) {
	// #nosec G304 -- path is the desktop's own config location (resolved
	// from DefaultConfigPath), not user input.
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("shell: read config %q: %w", path, err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return false, fmt.Errorf("shell: parse config %q: %w", path, err)
	}
	if _, ok := raw["storage"]; !ok {
		raw["storage"] = json.RawMessage(`{}`)
		changed = true
	}
	if _, ok := raw["session"]; !ok {
		raw["session"] = json.RawMessage(`{}`)
		changed = true
	}
	if !changed {
		return false, nil
	}
	if err := writeRawConfigAtomic(path, raw); err != nil {
		return false, err
	}
	return true, nil
}

// EnsureAdminBlock upgrades an existing config at path for the builder's
// mutation surface (v0.9.1, app-audit finding A4): without an admin block
// the core mounts neither /api/config nor /builder, yet the desktop embeds
// the Builder unconditionally — the tab boots a 404 trap. A missing block
// gains token_env KORVUN_ADMIN_TOKEN (the shell-managed per-cycle bearer,
// controller.newAdminToken: generated in-process, never persisted); a
// present block — any token_env — is not touched, not even an identical
// rewrite. WHERE the admin server binds is untouched here: the shell's
// ephemeral loopback override (withEphemeralAdmin) stays the only bind
// authority. Same raw-object discipline as EnsureChatBlocks: unknown
// fields survive verbatim, the write is atomic.
func EnsureAdminBlock(path string) (changed bool, err error) {
	// #nosec G304 -- path is the desktop's own config location (resolved
	// from DefaultConfigPath), not user input.
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("shell: read config %q: %w", path, err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return false, fmt.Errorf("shell: parse config %q: %w", path, err)
	}
	if _, ok := raw["admin"]; ok {
		return false, nil
	}
	raw["admin"] = json.RawMessage(`{"token_env": "KORVUN_ADMIN_TOKEN"}`)
	if err := writeRawConfigAtomic(path, raw); err != nil {
		return false, err
	}
	return true, nil
}

// writeRawConfigAtomic rewrites path from the raw top-level object with the
// WriteConfigAtomic discipline: canonical MarshalIndent, temp file + rename
// in the same directory — a failure leaves no partial file.
func writeRawConfigAtomic(path string, raw map[string]json.RawMessage) error {
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("shell: marshal upgraded config: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".korvun-config-*.tmp")
	if err != nil {
		return fmt.Errorf("shell: create temp config: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op once renamed
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("shell: write temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("shell: close temp config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("shell: rename upgraded config into place: %w", err)
	}
	return nil
}

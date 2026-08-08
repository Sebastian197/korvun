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
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return false, fmt.Errorf("shell: marshal upgraded config: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".korvun-config-*.tmp")
	if err != nil {
		return false, fmt.Errorf("shell: create temp config: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op once renamed
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		return false, fmt.Errorf("shell: write temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return false, fmt.Errorf("shell: close temp config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return false, fmt.Errorf("shell: rename upgraded config into place: %w", err)
	}
	return true, nil
}

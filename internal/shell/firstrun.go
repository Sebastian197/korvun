// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package shell

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/supervisor"
)

// firstRunTemplate is the embedded first-run config (ADR-0035 §5, SP5 design
// spec FR-FIRST-3): derived from configs/edge.json with the mandatory admin
// block (token_env KORVUN_ADMIN_TOKEN — the shell-managed per-cycle bearer),
// observability enabled (the effective address is the shell's ephemeral
// override), one private ollama/llama3.2:1b brain, and — NC-1, option B —
// ZERO channels and zero routes: onboarding/builder adds the first real
// channel. No secret value ever appears here, only env-var NAMES. The bytes
// are authored in WriteConfigAtomic's canonical MarshalIndent form so what
// lands on disk is byte-identical to this embed (the fidelity test pins it).
//
//go:embed firstrun_template.json
var firstRunTemplate []byte

// DefaultConfigPath is the desktop's default config location:
// <os.UserConfigDir>/korvun/korvun.json — the exact pattern the core uses
// for the default korvun.db, so config and DB share one per-user directory
// (SP5 design spec FR-FIRST-1).
func DefaultConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("shell: resolve user config dir: %w", err)
	}
	return filepath.Join(dir, "korvun", "korvun.json"), nil
}

// EnsureDefaultConfig provisions the first-run config at path (FR-FIRST-2):
// if the file exists it is not touched (not even an identical rewrite) and
// the return is (false, nil); if absent, the parent directory is created
// 0o700 when it does not yet exist (an existing directory keeps its mode)
// and the embedded template is written ATOMICALLY via
// supervisor.WriteConfigAtomic (temp file + rename — a failure leaves no
// partial file), returning (true, nil). The never-overwrite guarantee is
// check-then-write, not a single atomic operation: a file created by
// another process inside the stat→rename window would be replaced (by this
// same template — two racing first-runs of the shell converge on identical
// bytes, so the window is benign for the only realistic racer). The two
// outcomes let the caller drive different first-run UI.
func EnsureDefaultConfig(path string) (created bool, err error) {
	if _, statErr := os.Stat(path); statErr == nil {
		return false, nil
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return false, fmt.Errorf("shell: stat config %q: %w", path, statErr)
	}
	var cfg config.Config
	if err := json.Unmarshal(firstRunTemplate, &cfg); err != nil {
		// Unreachable with a healthy embed; the fidelity test keeps it so.
		return false, fmt.Errorf("shell: parse embedded first-run template: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, fmt.Errorf("shell: create config dir for %q: %w", path, err)
	}
	if err := supervisor.WriteConfigAtomic(path, &cfg); err != nil {
		return false, fmt.Errorf("shell: write first-run config %q: %w", path, err)
	}
	return true, nil
}

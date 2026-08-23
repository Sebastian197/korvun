// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package logsink is the desktop's file log sink (v0.9.1, app-audit finding
// B3). A Finder-launched app has no terminal: macOS discards its stderr, so
// every slog line the shell and core emit was unreadable exactly when it was
// needed (a failed boot diagnosed blind). The desktop opens this sink at
// startup and tees slog to BOTH stderr (terminal launches keep today's
// behavior) and korvun-desktop.log in the profile directory — the same
// directory that holds korvun.json and korvun.db (shell.DefaultConfigPath's
// parent), so one folder carries the whole operational state.
//
// Rotation is deliberately simple and stdlib-only (no dependency, no ADR):
// when the current file exceeds MaxBytes at open time it is renamed to
// korvun-desktop.log.old (replacing the previous .old) and a fresh file
// starts. Two files, bounded size, no timers — rotation happens per app
// launch, which matches how the desktop is actually used.
package logsink

import (
	"fmt"
	"os"
	"path/filepath"
)

// FileName is the log file's name inside the profile directory. On macOS the
// full path is ~/Library/Application Support/korvun/korvun-desktop.log.
const FileName = "korvun-desktop.log"

// MaxBytes caps the current file: an oversized file rotates to .old on the
// next Open. 5 MiB keeps two generations under 10 MiB total.
const MaxBytes = 5 << 20

// Open returns the append-mode log file inside dir, rotating first when the
// existing file exceeds MaxBytes. The directory must exist (the desktop
// resolves it from the config path, which EnsureDefaultConfig has already
// created). Errors are returned, never fatal: the caller keeps stderr-only
// logging when the sink cannot open.
func Open(dir string) (*os.File, error) {
	path := filepath.Join(dir, FileName)
	if st, err := os.Stat(path); err == nil && st.Size() > MaxBytes {
		if err := os.Rename(path, path+".old"); err != nil {
			return nil, fmt.Errorf("logsink: rotate %q: %w", path, err)
		}
	}
	// #nosec G304 -- path is the desktop's own profile directory, not user input.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("logsink: open %q: %w", path, err)
	}
	return f, nil
}

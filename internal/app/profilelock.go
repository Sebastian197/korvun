// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The exclusive profile lock (R4 Phase 1, ADR-0045): an advisory OS
// file lock on <profile>/korvun.lock that the SERVER holds for its
// whole life and the OS releases when the process dies — no stale-
// lockfile heuristics. `korvun receipt rotate-key` takes it only
// around the rotation; a held lock refuses with the stable rule
// signing_key_in_use (the third audit's P1: rotating beside a live
// server left it sealing with the retired key). HOUSE AMENDMENT: no
// other operator act takes the lock — approve/reject/execute keep
// working beside a live server, pinned by test.

package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrProfileLocked reports that another process holds the profile.
var ErrProfileLocked = errors.New("app: profile lock held by another process")

// ProfileLock is a held exclusive lock on one profile directory.
type ProfileLock struct {
	f *os.File
}

// AcquireProfileLock takes the exclusive advisory lock, non-blocking:
// a held lock returns ErrProfileLocked immediately.
func AcquireProfileLock(profileDir string) (*ProfileLock, error) {
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		return nil, fmt.Errorf("app: profile lock dir: %w", err)
	}
	path := filepath.Join(profileDir, "korvun.lock")
	f, err := os.OpenFile(filepath.Clean(path), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("app: open profile lock %q: %w", path, err)
	}
	if err := tryLockFile(f); err != nil {
		_ = f.Close()
		if errors.Is(err, ErrProfileLocked) {
			return nil, fmt.Errorf("%w (%s)", ErrProfileLocked, path)
		}
		return nil, fmt.Errorf("app: lock profile %q: %w", path, err)
	}
	return &ProfileLock{f: f}, nil
}

// Release drops the lock (the OS also drops it on process death).
func (l *ProfileLock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	err := unlockFile(l.f)
	closeErr := l.f.Close()
	l.f = nil
	if err != nil {
		return err
	}
	return closeErr
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package app

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// tryLockFile takes an exclusive non-blocking flock (ADR-0045; the
// kernel releases it when the process exits).
func tryLockFile(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return ErrProfileLocked
	}
	return err
}

func unlockFile(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_UN)
}

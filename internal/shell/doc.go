// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package shell will hold the desktop shell's lifecycle logic (ADR-0035 §3a):
// plain Go functions — start/stop the in-process core, config selection,
// secret provisioning — with NO Wails import, so they test as ordinary Go.
// The thin Wails adapter in cmd/korvun-desktop wraps this package.
//
// SP1 deliberately leaves it empty: the logic arrives with SP2's TDD cycle.
package shell

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package shell is the desktop shell's lifecycle logic (ADR-0035 §3a): plain
// Go functions — load a config, start/stop the in-process core under the
// reload supervisor, the ephemeral-port policy, per-cycle admin-bearer
// provisioning, status — with NO Wails import, so everything tests as
// ordinary Go with -race. The thin Wails adapter in cmd/korvun-desktop wraps
// this package; secret provisioning from the OS keychain arrives in SP3.
package shell

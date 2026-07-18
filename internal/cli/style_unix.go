// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package cli

// vtCapable reports whether the terminal can render ANSI VT escape sequences. On
// Unix (Linux, macOS, the BSDs) terminals are ANSI-native, so this is an
// unconditional no-op true — styleEnabled's TTY and opt-out gates alone decide
// whether styling is emitted. It exists as the portable counterpart to the Windows
// build's real console-mode probe (style_windows.go), so styleEnabled can stay
// platform-agnostic. Keeping it a no-op here means the Unix behaviour — and every
// unit test, which runs on the host OS — is byte-identical to before the guard.
func vtCapable() bool { return true }

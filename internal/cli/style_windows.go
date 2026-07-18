// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package cli

import (
	"sync"
	"syscall"
	"unsafe"
)

// enableVirtualTerminalProcessing is the console-mode flag that makes the Windows
// console interpret ANSI VT escape sequences instead of printing them literally
// (Microsoft Learn: SetConsoleMode, ENABLE_VIRTUAL_TERMINAL_PROCESSING). Modern
// Windows Terminal sets it itself; legacy cmd.exe/conhost does not.
const enableVirtualTerminalProcessing = 0x0004

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode = kernel32.NewProc("SetConsoleMode")

	vtOnce   sync.Once
	vtResult bool
)

// vtCapable reports whether this process's console can render ANSI VT sequences,
// turning VT ON via a raw kernel32 SetConsoleMode syscall the first time it is
// asked and caching the result for the process (sync.Once). It probes both the
// stdout and stderr handles — the two streams styleEnabled may paint — and enables
// VT on whichever are real consoles; a redirected handle is skipped (styleEnabled's
// per-stream isTTY check already suppresses styling there). It returns true only if
// at least one real console was seen and every real console accepted the VT flag,
// so on an older console that rejects it (or when no console is attached at all)
// styleEnabled degrades to plain rather than emitting escapes that would print
// literally (design spec R4/FR-STY-9). Pure stdlib syscall — no dependency.
//
// The true (VT-enabled) branch requires a real Windows console and cannot be
// unit-tested, exactly like the interactive-TTY branch of isTerminal; it is covered
// by the windows-latest CI job and the ×6 cross-compile, which prove this file
// compiles, links, and vets on GOOS=windows.
func vtCapable() bool {
	vtOnce.Do(func() {
		sawConsole := false
		for _, stdHandle := range []int{syscall.STD_OUTPUT_HANDLE, syscall.STD_ERROR_HANDLE} {
			handle, err := syscall.GetStdHandle(stdHandle)
			if err != nil {
				continue
			}
			var mode uint32
			// #nosec G103 -- documented Windows console API: GetConsoleMode writes the
			// current mode through the pointer; the uintptr(unsafe.Pointer(...)) is
			// consumed inline in the same Call, as go vet requires.
			if ret, _, _ := procGetConsoleMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&mode))); ret == 0 {
				continue // not a console (redirected) — isTTY already gates this stream
			}
			sawConsole = true
			if ret, _, _ := procSetConsoleMode.Call(uintptr(handle), uintptr(mode|enableVirtualTerminalProcessing)); ret == 0 {
				return // a real console that refuses VT → degrade to plain everywhere
			}
		}
		vtResult = sawConsole
	})
	return vtResult
}

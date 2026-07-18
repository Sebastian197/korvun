// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"strings"
	"testing"
)

// TestPaint_colorDepth pins the color-depth contract (SP5 half-B fix): paint emits
// xterm-256 indexed color (38;5;n) BY DEFAULT — which every color terminal renders,
// including Apple Terminal, whose truecolor parsing is broken — and only escalates
// to 24-bit truecolor (38;2;r;g;b) when the terminal advertises it via COLORTERM.
// Both forms still contain an ESC, so the escape-presence assertions elsewhere hold.
func TestPaint_colorDepth(t *testing.T) {
	c, _, _, _ := newTestCLI(0)

	t.Run("default -> 256-indexed with each role's precomputed cube index", func(t *testing.T) {
		t.Setenv("COLORTERM", "") // no truecolor advertised (empty reads as unset)
		for _, tc := range []struct {
			name string
			rl   role
			idx  string
		}{
			{"success", roleSuccess, "78"},
			{"error", roleError, "203"},
			{"warn", roleWarn, "214"},
		} {
			got := c.paint(true, tc.rl, "X")
			want := "\x1b[38;5;" + tc.idx + "mX\x1b[0m"
			if got != want {
				t.Errorf("%s: paint = %q, want %q", tc.name, got, want)
			}
		}
	})

	t.Run("COLORTERM=truecolor -> 24-bit with the exact ADR-0030 hex", func(t *testing.T) {
		t.Setenv("COLORTERM", "truecolor")
		got := c.paint(true, roleSuccess, "X")
		want := "\x1b[38;2;34;197;94mX\x1b[0m" // #22C55E
		if got != want {
			t.Errorf("paint(success) = %q, want %q", got, want)
		}
	})

	t.Run("COLORTERM=24bit -> 24-bit too", func(t *testing.T) {
		t.Setenv("COLORTERM", "24bit")
		got := c.paint(true, roleError, "X")
		if !strings.Contains(got, "\x1b[38;2;239;68;68m") { // #EF4444
			t.Errorf("paint(error) under 24bit = %q, want 38;2 truecolor", got)
		}
	})

	t.Run("disabled -> plain regardless of COLORTERM", func(t *testing.T) {
		t.Setenv("COLORTERM", "truecolor")
		if got := c.paint(false, roleSuccess, "X"); got != "X" {
			t.Errorf("paint(disabled) = %q, want plain X", got)
		}
	})
}

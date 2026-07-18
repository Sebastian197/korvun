// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"io"
	"os"
)

// styleEnabled reports whether decorative output (the banner today; ANSI color as
// later sub-phases add it) may be emitted on w. It encodes the project's opt-out
// precedence (design spec R1+R2): styling is on ONLY when the target stream is an
// interactive terminal AND the user has not opted out via any of --plain,
// --no-color, or the NO_COLOR environment variable (no-color.org: any non-empty
// value disables color). This keeps machine-readable output (R3) escape-free off
// a TTY or under an explicit opt-out.
func (c *cli) styleEnabled(w io.Writer, plain, noColor bool) bool {
	if plain || noColor {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return c.isTTY(w)
}

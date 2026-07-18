// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

// parseStyled registers the display flags every styled subcommand shares
// (--plain/--no-color), buffers fs's output, and parses args — routing the two
// flag outcomes the whole CLI shares identically: -h/--help is a query (usage ->
// stdout, exit 0, matching top-level help) and a bad/unknown flag is a usage error
// (message -> stderr, exit 2), so stdout stays machine-clean (R3). The caller
// registers its command-specific flags on fs BEFORE calling this. When done is
// true the caller returns code immediately; otherwise plain/noColor carry the
// parsed opt-out state and fs.Args() holds the positionals. Sharing this keeps
// serve and config check in lockstep on the -h vs bad-flag contract.
func (c *cli) parseStyled(fs *flag.FlagSet, args []string) (plain, noColor bool, code int, done bool) {
	var usage bytes.Buffer
	fs.SetOutput(&usage)
	fs.BoolVar(&plain, "plain", false, "disable colored/decorative output")
	fs.BoolVar(&noColor, "no-color", false, "disable ANSI color")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, _ = io.Copy(c.stdout, &usage) // help is a query, not an error
			return false, false, 0, true
		}
		_, _ = io.Copy(c.stderr, &usage) // usage error: bad/unknown flag
		return false, false, 2, true
	}
	return plain, noColor, 0, false
}

// role is a semantic color from KORVUN's fixed palette (ADR-0030). The CLI uses
// only the two the config-check output needs so far — success and error — mapped
// to the event palette (sent #22C55E, failed #EF4444). More roles (info/warn) are
// added where a command's output first needs them, never speculatively.
type role struct{ r, g, b uint8 }

var (
	roleSuccess = role{0x22, 0xC5, 0x5E} // #22C55E — "sent"/OK/up
	roleError   = role{0xEF, 0x44, 0x44} // #EF4444 — "failed"/error/down
	roleWarn    = role{0xF5, 0x9E, 0x0B} // #F59E0B — "dropped"/warning
)

// paint wraps s in the role's ANSI truecolor SGR sequence when enabled, otherwise
// returns s untouched. The caller decides enabled by asking styleEnabled about the
// TARGET stream, so painting can never smuggle an escape into machine-clean output
// (R3). ADR-0030's invariant — color is never the only channel — is upheld by
// callers: the wrapped text (e.g. "OK", "FAILED") is itself a label, so stripping
// the color leaves the meaning intact.
func (c *cli) paint(enabled bool, rl role, s string) string {
	if !enabled {
		return s
	}
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm%s\x1b[0m", rl.r, rl.g, rl.b, s)
}

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
	if !c.isTTY(w) {
		return false
	}
	// The stream is an interactive terminal and the user has not opted out; emit
	// styling only if the terminal can actually render ANSI VT sequences. c.vt is a
	// seam over vtCapable (a no-op true on Unix; a live kernel32 console-mode probe
	// on Windows that enables VT once and reports false when impossible), so escapes
	// never print literally in a legacy conhost that would show them (R4/FR-STY-9),
	// and the gating stays deterministic under test on every OS.
	return c.vt()
}

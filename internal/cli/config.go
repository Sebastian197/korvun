// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/Sebastian197/korvun/internal/app"
	"github.com/Sebastian197/korvun/internal/config"
)

// configCmd dispatches the `config` noun's sub-verbs (sub-phase 3). Only `check`
// exists so far; a missing or unknown sub-verb is a usage error (exit 2) to stderr
// pointing at `config check` and `korvun help`. Keeping the noun/verb split here
// (not a flat flag surface) matches the framing's git/docker shape (FR-CLI-3) and
// leaves room for future verbs without touching the top-level dispatch.
func (c *cli) configCmd(args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(c.stderr, "korvun config: expected a subcommand: check\nRun 'korvun help' for usage.\n")
		return 2
	}
	switch args[0] {
	case "check":
		return c.configCheck(args[1:])
	default:
		_, _ = fmt.Fprintf(c.stderr, "korvun config: unknown subcommand %q\nRun 'korvun help' for usage.\n", args[0])
		return 2
	}
}

// configCheck implements `korvun config check [--preflight] [path]` (FR-CHK-1/2).
//
// Offline by default: config.Load validates structure/enums with no network and no
// secrets, so a malformed or invalid config surfaces its first field-path violation
// to stderr (exit 2) and a clean one prints an OK line to stdout (exit 0). With
// --preflight, the online app.Preflight seam additionally resolves secrets, runs the
// privacy selector, and does the channel getMe round-trip (ADR-0027 §5); a preflight
// failure is a runtime failure (exit 1, named error to stderr). The seam is injected
// (c.preflight) so tests force its outcome without a network or a port.
//
// Style (born here): the OK line is success-colored and errors are error-colored
// (ADR-0030 palette), always gated by styleEnabled on the TARGET stream so stdout
// stays escape-free under --plain / non-TTY (R1–R3). The colored token is itself a
// textual label ("OK"/"FAILED"), so color is never the only channel (ADR-0030).
func (c *cli) configCheck(args []string) int {
	// Permute path vs flags: Go's flag parser stops at the first positional, so
	// `config check korvun.json --preflight` (path first) would silently drop the
	// flag. Every config-check flag is boolean (none consumes a following value),
	// so any non-dash token is the path — separate them and parse the flags
	// regardless of the path's position. (Revisit if a value-taking flag like
	// --format is ever added, which would make a bare token ambiguous.)
	var positionals, flagArgs []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") && a != "-" {
			flagArgs = append(flagArgs, a)
		} else {
			positionals = append(positionals, a)
		}
	}

	fs := flag.NewFlagSet("korvun config check", flag.ContinueOnError)
	var preflight bool
	fs.BoolVar(&preflight, "preflight", false, "additionally run online checks (resolve secrets, privacy selector, channel getMe)")
	plain, noColor, code, done := c.parseStyled(fs, flagArgs)
	if done {
		return code // -h/--help (0) or a bad flag (2), already written to the right stream
	}

	// Close the same silence serve does: a residual positional the FlagSet itself
	// parsed (a token after a `--` terminator) must not be ignored — it is a usage
	// error, not a silently-dropped argument. (Non-dash tokens are already pulled
	// into `positionals` above; this catches dash-looking tokens after `--`.)
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(c.stderr, "korvun config check: unexpected argument %q\nRun 'korvun help' for usage.\n", fs.Arg(0))
		return 2
	}

	if len(positionals) > 1 {
		_, _ = fmt.Fprintf(c.stderr, "korvun config check: expected at most one path, got %d\nRun 'korvun help' for usage.\n", len(positionals))
		return 2
	}

	path := "korvun.json"
	if len(positionals) == 1 {
		path = positionals[0]
	}

	// Offline validate (structure + enums; no I/O beyond reading the file).
	cfg, err := config.Load(path)
	if err != nil {
		errStyled := c.styleEnabled(c.stderr, plain, noColor)
		_, _ = fmt.Fprintf(c.stderr, "%s  %v\n", c.paint(errStyled, roleError, "FAILED"), err)
		return 2
	}

	// Optional online preflight (secret resolution + privacy selector + getMe).
	if preflight {
		if err := c.preflight(cfg); err != nil {
			errStyled := c.styleEnabled(c.stderr, plain, noColor)
			_, _ = fmt.Fprintf(c.stderr, "%s  preflight failed: %v\n", c.paint(errStyled, roleError, "FAILED"), err)
			return 1
		}
	}

	okStyled := c.styleEnabled(c.stdout, plain, noColor)
	line := fmt.Sprintf("%s  config valid: %s", c.paint(okStyled, roleSuccess, "OK"), path)
	if preflight {
		line += " (preflight passed)"
	}
	_, _ = fmt.Fprintln(c.stdout, line)
	return 0
}

// runPreflight is the single call site of app.Preflight's option set (ADR-0027 §5),
// shared by the CLI's config-check seam (defaultPreflight) and the supervisor's
// reload preflight closure in bootServe, so both construct the config identically —
// only the logger sink differs. Centralizing it means a change to Preflight's
// options is made in one place, never drifting between the two callers.
func runPreflight(cfg *config.Config, logger *slog.Logger) error {
	return app.Preflight(cfg, app.WithLogger(logger))
}

// defaultPreflight is the real config-check preflight seam: it runs runPreflight
// with a discarding logger, so `config check --preflight` prints only its own
// OK/error summary — the per-model Info logs Preflight emits are noise for a check
// command. It is glue (tests inject a fake c.preflight), so the logger choice is
// invisible to the unit suite; the named error it returns is what the command
// surfaces on failure.
func defaultPreflight(cfg *config.Config) error {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return runPreflight(cfg, logger)
}

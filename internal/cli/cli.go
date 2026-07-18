// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package cli is the command-line surface of the korvun binary (Piece 3): it
// parses argv, dispatches the subcommands (serve / config / status / version /
// help), and owns the retrocompat shim so the pre-CLI invocation
// `korvun -config x.json` keeps behaving as an implicit `serve`. All logic lives
// here behind a single Run entry point so cmd/korvun/main stays a 3-line glue
// (ADR-0017) and every behaviour is unit-tested against injected writers rather
// than the real os.Std* streams.
//
// Sub-phase 1 landed the scaffold: dispatch, `version` (via internal/buildinfo),
// `help`, the TTY-gated logo banner, and the shim/serve seam wired to the
// existing boot. Sub-phase 2 gave `serve` its own flag surface (serveCmd: a
// pure, unit-testable parse/validate step over injected writers) split from the
// real boot behind the injectable c.boot seam, plus a gated pre-serve banner.
// Sub-phase 3 adds `config check [--preflight]` (offline config.Load/Validate by
// default; online app.Preflight behind the injectable c.preflight seam) and the
// first semantic color roles (success/error, ADR-0030), gated by styleEnabled.
// `status` is announced but lands in a later sub-phase; ANSI styling (KORVUN's
// violet identity, ADR-0030) is integrated where each command's output is born.
package cli

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"

	"github.com/Sebastian197/korvun/internal/buildinfo"
	"github.com/Sebastian197/korvun/internal/config"
)

// version is the build version of the korvun binary: "dev" for a local build, or
// the SemVer tag injected at release time via -ldflags "-X <pkg>.version=vX.Y.Z".
//
// NOTE (sub-phase follow-up, reported in the green review): Stage 15's
// .goreleaser.yaml still targets `main.version`. Now that the version subcommand
// lives here, that ldflags target must move to
// `github.com/Sebastian197/korvun/internal/cli.version` — a separate sub-phase
// (it edits .goreleaser.yaml). Until then a release build reports "dev"; no real
// release tag has been pushed yet, so nothing shipped regresses.
var version = "dev"

// cli holds the output writers plus the injectable seams a test overrides: isTTY
// (terminal detection, so the logo is emitted only on an interactive stderr) and
// boot (the real serve boot, faked in tests and real via bootServe). serve's flag
// surface (serveCmd) is pure and unit-tested against the injected writers; only
// the boot behind it opens ports, so tests drive routing without booting the app.
type cli struct {
	stdout    io.Writer
	stderr    io.Writer
	version   string
	isTTY     func(io.Writer) bool
	boot      func(configPath string) int
	preflight func(cfg *config.Config) error
	// vt reports whether the terminal can render ANSI VT sequences (enabling it on
	// Windows). It is a seam like isTTY/boot/preflight so styleEnabled's gating is
	// deterministic in tests on every OS: the real binary uses vtCapable (a no-op
	// true on Unix, a live console-mode probe on Windows), but the windows-latest
	// test process runs with redirected pipes where that probe would report false —
	// tests inject a fixed vt so the gating logic is exercised identically on all
	// three OSes.
	vt func() bool
}

// Run is the single entry point of the korvun command line. It dispatches args
// (argv without the program name) and writes to stdout/stderr, returning the
// process exit code: 0 success, 1 runtime failure, 2 usage error. main forwards
// os.Args[1:], os.Stdout and os.Stderr into it and exits with the result.
func Run(args []string, stdout, stderr io.Writer) int {
	return newCLI(stdout, stderr).run(args)
}

// newCLI builds a cli over the given writers with the real default seams:
// terminal detection via the OS file mode, and bootServe as the boot entry.
func newCLI(stdout, stderr io.Writer) *cli {
	return &cli{
		stdout:    stdout,
		stderr:    stderr,
		version:   version,
		isTTY:     isTerminal,
		boot:      bootServe,
		preflight: defaultPreflight,
		vt:        vtCapable,
	}
}

// run implements the dispatch table (see the design spec, FR-CLI-3..8). help and
// the version-flag forms are matched before the shim so a leading dash there is
// not mistaken for a serve flag.
func (c *cli) run(args []string) int {
	if len(args) == 0 {
		return c.help() // bare `korvun`: orient the user (a query, not an error)
	}

	switch args[0] {
	case "help", "-h", "--help":
		return c.help()
	case "version", "-version", "--version":
		return c.printVersion()
	case "serve":
		return c.serveCmd(args[1:])
	case "config":
		return c.configCmd(args[1:])
	case "status":
		_, _ = fmt.Fprintf(c.stderr, "korvun: %q is not available yet (coming in a later release).\nRun 'korvun help' for usage.\n", args[0])
		return 2
	}

	// Retrocompat shim: the pre-CLI `korvun -config x.json` invocation (a leading
	// -config/--config flag with no subcommand) routes to serve unchanged. Narrowed
	// to the config flag on purpose: a bare display flag like `korvun --plain` /
	// `--no-color` must NOT silently boot the server — those are serve-subcommand
	// flags, not top-level ones, so they fall through to the usage error below.
	if isServeShim(args[0]) {
		return c.serveCmd(args)
	}

	_, _ = fmt.Fprintf(c.stderr, "korvun: unknown command %q\nRun 'korvun help' for usage.\n", args[0])
	return 2
}

// isServeShim reports whether arg is the retrocompat serve invocation: a leading
// -config / --config flag (with either a following value or the `=value` form),
// the only pre-CLI shape the validated docs/systemd unit used. It is deliberately
// narrower than "any leading dash" so serve-only display flags (--plain,
// --no-color) do not slip through the shim and boot a server unasked.
func isServeShim(arg string) bool {
	return arg == "-config" || arg == "--config" ||
		strings.HasPrefix(arg, "-config=") || strings.HasPrefix(arg, "--config=")
}

// printVersion writes the one-line build identity to stdout (machine-clean, no
// styling) and returns 0. It reuses internal/buildinfo.Format — the same helper
// the pre-CLI `--version` flag used (ADR-0025 §2), so the two never diverge.
func (c *cli) printVersion() int {
	bi, _ := debug.ReadBuildInfo()
	_, _ = fmt.Fprintln(c.stdout, buildinfo.Format(c.version, bi))
	return 0
}

// help writes the usage screen to stdout (exit 0) and, only when styling is
// enabled on stderr, the logo banner to stderr. Banner gating goes through the
// shared styleEnabled helper (help has no flags of its own, so plain/no-color are
// false) so the NO_COLOR opt-out is honored identically to serve — the banner is
// decoration and never touches stdout, so `korvun help` piped to a file, or run
// under NO_COLOR, stays banner-free.
func (c *cli) help() int {
	if c.styleEnabled(c.stderr, false, false) {
		_, _ = fmt.Fprint(c.stderr, logo)
	}
	_, _ = fmt.Fprint(c.stdout, helpText)
	return 0
}

// isTerminal reports whether w is an interactive character device (a TTY). It is
// pure stdlib: only an *os.File can be a terminal, and a character-device mode
// bit distinguishes a console from a pipe/file/buffer. Any non-*os.File writer
// (e.g. a test bytes.Buffer) is treated as non-interactive.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// logo is the placeholder banner shown on an interactive terminal. The final art
// is Chano's brand decision (ADR-0030 reserves the teal→violet gradient for
// identity moments); this honest ASCII placeholder holds its place. It carries no
// ANSI styling yet — color is integrated in a later sub-phase.
const logo = `
   ██ ██  KORVUN
   ████    messaging gateway · multi-model router · multi-brain orchestrator
   ██ ██   (placeholder banner — final logo art TBD, ADR-0030)

`

// helpText is the usage screen. It names every subcommand with a one-line
// description (config/status flagged as upcoming) and points to the docs.
const helpText = `korvun — a single Go binary: messaging gateway + multi-model router + multi-brain orchestrator.

Usage:
  korvun <command> [flags]
  korvun -config <path>        shorthand for 'korvun serve -config <path>' (retrocompat)

Commands:
  serve         Load config, wire channels/brains, and serve until SIGINT/SIGTERM.
  config check  Validate a config file (offline; --preflight adds online checks).
  version       Print the binary version and exit.
  help          Show this help.

Coming in a later release:
  status        Show the live wiring of a running korvun via its admin API.

Examples:
  korvun serve --config korvun.json
  korvun config check --preflight korvun.json

See the docs in docs/ — QUICKSTART.md, packaging/INSTALL.md, CONFIGURATION.md.
`

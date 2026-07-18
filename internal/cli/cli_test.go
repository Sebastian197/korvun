// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package cli tests — Piece 3, dispatch + version + help + logo (sub-phase 1),
// serve's flag surface + injectable boot seam + banner gating (sub-phase 2), and
// `config check [--preflight]` + color roles (sub-phase 3).
//
// These pin the contract of internal/cli against the surface the implementation
// provides:
//
//	func Run(args []string, stdout, stderr io.Writer) int
//	type cli struct {
//	    stdout, stderr io.Writer
//	    version        string                       // ldflags-set default via a package var
//	    isTTY          func(io.Writer) bool          // TTY detection seam
//	    boot           func(configPath string) int   // real serve boot seam (sub-phase 2)
//	    preflight      func(*config.Config) error    // online preflight seam (sub-phase 3)
//	}
//	func newCLI(stdout, stderr io.Writer) *cli // real defaults
//	func (c *cli) run(args []string) int
//	func (c *cli) serveCmd(args []string) int  // pure serve flag surface (sub-phase 2)
//	func (c *cli) configCmd(args []string) int // config noun dispatch (sub-phase 3)
//
// Run is the ONLY public entry; main is a 3-line glue that forwards os.Args and
// os.Std* into it (ADR-0017: main stays un-unit-tested glue), so every behaviour
// is exercised here against injected buffers — never the real os.Stdout/os.Stderr.
// The seams (isTTY, boot, preflight) are injected so a test can drive TTY-gated
// styling, serve routing, and preflight outcomes without a terminal or a network.
package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
)

// bootRec records an invocation of the boot seam so a test can assert that the
// serve flag surface parsed argv and routed to boot with the exact config path,
// WITHOUT the real supervisor booting or a port opening (sub-phase 2: flag
// parsing is pure/testable, the boot lives behind this injectable seam).
type bootRec struct {
	called     bool
	configPath string
	ret        int // the exit code the fake boot returns (to prove serveCmd forwards it)
}

func (r *bootRec) fn(configPath string) int {
	r.called = true
	r.configPath = configPath
	return r.ret
}

// newTestCLI wires a cli over two buffers with the seams stubbed for a
// non-interactive run: isTTY=false (no terminal), a recording boot seam. Tests
// override the individual seams they exercise.
func newTestCLI(bootRet int) (c *cli, stdout, stderr *bytes.Buffer, rec *bootRec) {
	stdout = &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	rec = &bootRec{ret: bootRet}
	c = newCLI(stdout, stderr)
	c.isTTY = func(io.Writer) bool { return false }         // default: not a terminal
	c.boot = rec.fn                                         // faked serve boot
	c.preflight = func(*config.Config) error { return nil } // hermetic default: preflight passes, no network
	c.vt = func() bool { return true }                      // VT available: gating tested identically on all OSes
	return c, stdout, stderr, rec
}

const escape = "\x1b" // ANSI escape introducer; must never reach stdout off-TTY

// TestRun_dispatch pins the subcommand dispatch table: which token routes where,
// on which stream, with which exit code, and — for the serve paths — that the
// serve flag surface parsed argv and reached the boot seam with the right config
// path (sub-phase 2). The boot seam is faked, so no supervisor boots.
func TestRun_dispatch(t *testing.T) {
	const bootExit = 42 // a distinctive value to prove serveCmd forwards boot's return

	tests := []struct {
		name           string
		args           []string
		wantExit       int
		wantBootCalled bool
		wantConfigPath string // when routed to serve, the config path the seam receives
		stdoutContains []string
		stderrContains []string
		stdoutEmpty    bool
		stderrEmpty    bool
	}{
		{
			name:           "no args -> help to stdout, exit 0 (help is a query, not an error)",
			args:           nil,
			wantExit:       0,
			stdoutContains: []string{"Usage:", "Commands:"},
			stderrEmpty:    true, // non-TTY: no banner, nothing on stderr
		},
		{
			name:           "help subcommand -> help to stdout, exit 0",
			args:           []string{"help"},
			wantExit:       0,
			stdoutContains: []string{"Usage:", "Commands:"},
			stderrEmpty:    true,
		},
		{
			name:           "--help -> help to stdout, exit 0 (not routed to serve shim)",
			args:           []string{"--help"},
			wantExit:       0,
			stdoutContains: []string{"Usage:"},
			stderrEmpty:    true,
		},
		{
			name:           "-h -> help to stdout, exit 0",
			args:           []string{"-h"},
			wantExit:       0,
			stdoutContains: []string{"Usage:"},
			stderrEmpty:    true,
		},
		{
			name:           "unknown subcommand -> usage error to stderr, exit 2",
			args:           []string{"frobnicate"},
			wantExit:       2,
			stdoutEmpty:    true,
			stderrContains: []string{"frobnicate", "help"}, // names the bad token + points to help
		},
		{
			name:           "bare config (no sub-verb) -> usage error naming the subcommand, exit 2",
			args:           []string{"config"},
			wantExit:       2,
			stdoutEmpty:    true,
			stderrContains: []string{"config", "check", "help"}, // points at 'config check'
		},
		{
			name:           "config with an unknown sub-verb -> usage error, exit 2",
			args:           []string{"config", "frobnicate"},
			wantExit:       2,
			stdoutEmpty:    true,
			stderrContains: []string{"frobnicate", "help"},
		},
		{
			name:           "status --help -> routes to statusCmd, help query to stdout, exit 0 (no dial)",
			args:           []string{"status", "--help"},
			wantExit:       0,
			stdoutContains: []string{"addr"}, // status' own flag surface
			stderrEmpty:    true,
		},
		{
			name:           "serve subcommand routes to boot with the parsed --config path",
			args:           []string{"serve", "--config", "korvun.json"},
			wantExit:       bootExit,
			wantBootCalled: true,
			wantConfigPath: "korvun.json",
		},
		{
			name:           "serve with no flags -> boot with the default config path",
			args:           []string{"serve"},
			wantExit:       bootExit,
			wantBootCalled: true,
			wantConfigPath: "korvun.json",
		},
		{
			name:           "shim: -config <path> without a subcommand -> implicit serve, parsed path",
			args:           []string{"-config", "korvun.local.json"},
			wantExit:       bootExit,
			wantBootCalled: true,
			wantConfigPath: "korvun.local.json",
		},
		{
			name:           "shim: --config <path> -> implicit serve, parsed path",
			args:           []string{"--config", "x.json"},
			wantExit:       bootExit,
			wantBootCalled: true,
			wantConfigPath: "x.json",
		},
		{
			name:           "shim: -config=<path> (equals form) -> implicit serve, parsed path",
			args:           []string{"-config=cfg.json"},
			wantExit:       bootExit,
			wantBootCalled: true,
			wantConfigPath: "cfg.json",
		},
		{
			name:           "bare --plain (a serve display flag, no config) is NOT the shim -> usage error, exit 2",
			args:           []string{"--plain"},
			wantExit:       2,
			stdoutEmpty:    true,
			stderrContains: []string{"--plain", "help"}, // must NOT boot a server
		},
		{
			name:           "bare --no-color is NOT the shim -> usage error, exit 2 (never boots)",
			args:           []string{"--no-color"},
			wantExit:       2,
			stdoutEmpty:    true,
			stderrContains: []string{"no-color", "help"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, stdout, stderr, rec := newTestCLI(bootExit)

			got := c.run(tt.args)

			if got != tt.wantExit {
				t.Errorf("exit code = %d, want %d", got, tt.wantExit)
			}
			if rec.called != tt.wantBootCalled {
				t.Errorf("boot called = %v, want %v", rec.called, tt.wantBootCalled)
			}
			if tt.wantBootCalled && rec.configPath != tt.wantConfigPath {
				t.Errorf("boot config path = %q, want %q", rec.configPath, tt.wantConfigPath)
			}
			assertStream(t, "stdout", stdout.String(), tt.stdoutContains, tt.stdoutEmpty)
			assertStream(t, "stderr", stderr.String(), tt.stderrContains, tt.stderrEmpty)
		})
	}
}

// TestRun_version pins that every version form prints the identity to STDOUT
// (never stderr), exit 0, on a single clean line with no ANSI escapes, reusing
// internal/buildinfo (the injected version string surfaces verbatim).
func TestRun_version(t *testing.T) {
	for _, form := range [][]string{{"version"}, {"--version"}, {"-version"}} {
		t.Run(strings.Join(form, " "), func(t *testing.T) {
			c, stdout, stderr, _ := newTestCLI(0)
			c.version = "v9.9.9-test"

			got := c.run(form)

			if got != 0 {
				t.Errorf("exit = %d, want 0", got)
			}
			out := stdout.String()
			if !strings.HasPrefix(out, "korvun ") {
				t.Errorf("stdout = %q, want prefix %q (buildinfo.Format)", out, "korvun ")
			}
			if !strings.Contains(out, "v9.9.9-test") {
				t.Errorf("stdout = %q, want it to carry the injected version %q", out, "v9.9.9-test")
			}
			if strings.Contains(out, escape) {
				t.Errorf("stdout carries an ANSI escape; the version line must be machine-clean: %q", out)
			}
			if lines := strings.Count(strings.TrimRight(out, "\n"), "\n"); lines != 0 {
				t.Errorf("version stdout must be a single parseable line, got %d extra newline(s): %q", lines, out)
			}
			if stderr.Len() != 0 {
				t.Errorf("version must write nothing to stderr, got %q", stderr.String())
			}
		})
	}
}

// TestRun_versionOnTTY_noBanner pins that version stays scriptable even on a
// terminal: the logo/banner is a help-path decoration and must NOT precede a
// machine query like version, on either stream.
func TestRun_versionOnTTY_noBanner(t *testing.T) {
	c, stdout, stderr, _ := newTestCLI(0)
	c.isTTY = func(io.Writer) bool { return true } // pretend both streams are a TTY

	if got := c.run([]string{"version"}); got != 0 {
		t.Fatalf("exit = %d, want 0", got)
	}
	if stderr.Len() != 0 {
		t.Errorf("version on a TTY must not emit a banner to stderr, got %q", stderr.String())
	}
	if strings.Contains(stdout.String(), escape) {
		t.Errorf("version stdout must be escape-free even on a TTY: %q", stdout.String())
	}
}

// TestRun_exportedEntry pins the public Run(args, stdout, stderr) int surface —
// the exact signature main forwards into — using the real defaults (package
// version = "dev" in a test build).
func TestRun_exportedEntry(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if got := Run([]string{"version"}, &stdout, &stderr); got != 0 {
		t.Fatalf("Run(version) exit = %d, want 0", got)
	}
	if !strings.HasPrefix(stdout.String(), "korvun dev") {
		t.Errorf("Run(version) stdout = %q, want prefix %q", stdout.String(), "korvun dev")
	}
	if strings.Contains(stdout.String(), escape) {
		t.Errorf("Run(version) stdout must be machine-clean, got escapes: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("Run(version) stderr must be empty, got %q", stderr.String())
	}
}

// TestHelp_structure pins the help layout contract: a Usage line, every
// sub-command named with a description, and a pointer to where to find more.
// No color is asserted here (color arrives with the style logic in later
// sub-phases) — but help MUST be escape-free when the stream is not a TTY.
func TestHelp_structure(t *testing.T) {
	c, stdout, stderr, _ := newTestCLI(0)

	if got := c.run([]string{"help"}); got != 0 {
		t.Fatalf("help exit = %d, want 0", got)
	}
	out := stdout.String()

	for _, want := range []string{"Usage:", "Commands:", "serve", "config", "status", "version", "help", "docs"} {
		if !strings.Contains(out, want) {
			t.Errorf("help stdout missing %q; got:\n%s", want, out)
		}
	}
	if strings.Contains(out, escape) {
		t.Errorf("help stdout must be escape-free on a non-TTY stream: %q", out)
	}
	if stderr.Len() != 0 {
		t.Errorf("help on a non-TTY must leave stderr empty (no banner), got %q", stderr.String())
	}
}

// TestLogo_TTYGating pins the load-bearing styling invariant WITHOUT coupling to
// the concrete art: the logo/banner goes to stderr ONLY when stderr is an
// interactive TTY, and the stdout payload is byte-identical whether or not a TTY
// is present — i.e. the logo can never leak into machine-readable stdout.
func TestLogo_TTYGating(t *testing.T) {
	t.Setenv("NO_COLOR", "") // empty reads as unset: keep the TTY assertion env-independent
	run := func(tty bool) (stdout, stderr string) {
		c, out, errb, _ := newTestCLI(0)
		c.isTTY = func(io.Writer) bool { return tty }
		c.run([]string{"help"})
		return out.String(), errb.String()
	}

	stdoutOff, stderrOff := run(false)
	stdoutOn, stderrOn := run(true)

	if stderrOff != "" {
		t.Errorf("non-TTY: no banner expected on stderr, got %q", stderrOff)
	}
	if stderrOn == "" {
		t.Errorf("TTY: a banner is expected on stderr, got empty")
	}
	if stdoutOff != stdoutOn {
		t.Errorf("the stdout payload must not change with TTY state (the logo must never touch stdout):\n non-TTY: %q\n TTY:     %q", stdoutOff, stdoutOn)
	}
}

// TestIsTerminal pins the real (default) TTY-detection seam used by newCLI: a
// non-*os.File writer (a buffer) is never a terminal, and a regular file — an
// *os.File that is not a character device — is not a terminal either. The
// interactive-TTY true branch needs a real console and is exercised by running
// the binary in a terminal, not in the unit suite.
func TestIsTerminal(t *testing.T) {
	if isTerminal(&bytes.Buffer{}) {
		t.Error("a bytes.Buffer is not an *os.File and must not be reported as a terminal")
	}

	f, err := os.Create(filepath.Join(t.TempDir(), "regular.txt"))
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer func() { _ = f.Close() }()
	if isTerminal(f) {
		t.Error("a regular file is not a character device and must not be reported as a terminal")
	}
}

// TestHelp_bannerHonorsNoColor pins that help's banner obeys the SAME opt-out
// precedence as serve (the review's F3): NO_COLOR set suppresses the banner even
// on an interactive terminal, while the help text still prints to stdout. Without
// this, the opt-out the package introduced for serve would be silently ignored by
// help — an inconsistency across two commands in the same sub-phase.
func TestHelp_bannerHonorsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	c, stdout, stderr, _ := newTestCLI(0)
	c.isTTY = func(io.Writer) bool { return true } // interactive, but NO_COLOR opts out

	if got := c.run([]string{"help"}); got != 0 {
		t.Fatalf("help exit = %d, want 0", got)
	}
	if strings.Contains(stderr.String(), "KORVUN") {
		t.Errorf("NO_COLOR must suppress the help banner even on a TTY, got stderr %q", stderr.String())
	}
	if stdout.Len() == 0 {
		t.Error("help text must still print to stdout under NO_COLOR")
	}
}

// TestServeCmd_help pins that a help request on the serve subcommand (-h/--help)
// is a QUERY, not a usage error (the review's F2): exit 0 (matching top-level
// help, so a `set -e` wrapper does not abort), the serve usage prints to stdout,
// stderr stays empty, and the boot seam is never reached.
func TestServeCmd_help(t *testing.T) {
	for _, form := range [][]string{{"--help"}, {"-h"}} {
		t.Run(strings.Join(form, " "), func(t *testing.T) {
			c, stdout, stderr, rec := newTestCLI(0)

			got := c.serveCmd(form)

			if got != 0 {
				t.Errorf("serveCmd(%v) = %d, want 0 (help is a query, not a usage error)", form, got)
			}
			if rec.called {
				t.Error("serve help must not reach the boot seam")
			}
			if !strings.Contains(stdout.String(), "config") {
				t.Errorf("serve help must print its flag usage (incl. -config) to stdout; got %q", stdout.String())
			}
			if stderr.Len() != 0 {
				t.Errorf("serve help is a query: stderr must stay empty, got %q", stderr.String())
			}
		})
	}
}

// TestServeCmd_flagSurface pins serve's own flag surface (sub-phase 2, FR-CLI-4):
// an invalid serve flag is a usage error — exit 2, the message goes to the
// INJECTED stderr (never os.Stderr directly), stdout stays empty, and the boot
// seam is never reached (parse failed before any boot could start).
func TestServeCmd_flagSurface(t *testing.T) {
	c, stdout, stderr, rec := newTestCLI(0)

	got := c.serveCmd([]string{"--definitely-not-a-flag"})

	if got != 2 {
		t.Errorf("serveCmd(bad flag) exit = %d, want 2", got)
	}
	if rec.called {
		t.Error("boot must NOT be reached when serve flag parsing fails")
	}
	if stdout.Len() != 0 {
		t.Errorf("a serve usage error must keep stdout empty, got %q", stdout.String())
	}
	if stderr.Len() == 0 {
		t.Error("a serve usage error must be written to the injected stderr, got empty")
	}
	if !strings.Contains(stderr.String(), "definitely-not-a-flag") {
		t.Errorf("serve usage error should name the offending flag; got %q", stderr.String())
	}
}

// TestServeCmd_rejectsPositional pins the strictness parked in sub-phase 2: serve
// takes its config via the --config flag, so a stray positional argument (a user
// typing `korvun serve mycfg.json`, meaning `--config mycfg.json`) must NOT
// silently boot with the DEFAULT config — it is a usage error (exit 2) to stderr,
// and the boot seam is never reached. Without this, the typo boots korvun.json and
// serves the wrong config unnoticed.
func TestServeCmd_rejectsPositional(t *testing.T) {
	c, stdout, stderr, rec := newTestCLI(0)

	got := c.serveCmd([]string{"mycfg.json"})

	if got != 2 {
		t.Errorf("serveCmd(positional) = %d, want 2", got)
	}
	if rec.called {
		t.Error("a stray positional must NOT reach the boot seam (no silent default-config boot)")
	}
	if stdout.Len() != 0 {
		t.Errorf("serve usage error must keep stdout empty, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "mycfg.json") {
		t.Errorf("serve usage error should name the unexpected argument; got %q", stderr.String())
	}
}

// TestServeShim_rejectsTrailingToken pins that the positional-rejection strictness
// applies UNIFORMLY, including the retrocompat shim path: `korvun -config x.json
// extra` (a stray trailing token) is a usage error (exit 2), never a boot. This is
// intentional — a stray positional always signals user confusion, and no DOCUMENTED
// invocation (`korvun -config <path>`, systemd, the docs) carries a trailing token,
// so none of them regress. Pinned so the tightening is a decision, not an accident.
func TestServeShim_rejectsTrailingToken(t *testing.T) {
	c, stdout, stderr, rec := newTestCLI(0)

	got := c.run([]string{"-config", "x.json", "extra"})

	if got != 2 {
		t.Errorf("exit = %d, want 2 (stray trailing token on the shim)", got)
	}
	if rec.called {
		t.Error("a stray trailing token must NOT boot, even via the shim")
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout must stay empty, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "extra") {
		t.Errorf("usage error should name the unexpected token; got %q", stderr.String())
	}
}

// TestServeCmd_bannerGating pins the pre-serve banner contract (FR-STY-8, R1–R3):
// the placeholder banner is decoration on stderr, emitted ONLY on an interactive
// terminal with no opt-out, and NEVER when --plain / --no-color / NO_COLOR is set
// or the stream is not a TTY. In every case boot is still reached with the config
// path (the banner never gates the boot itself), and stdout stays untouched.
func TestServeCmd_bannerGating(t *testing.T) {
	tests := []struct {
		name       string
		tty        bool
		noColorEnv string
		args       []string
		wantBanner bool
	}{
		{name: "TTY, no opt-out -> banner", tty: true, args: []string{"--config", "x.json"}, wantBanner: true},
		{name: "non-TTY -> no banner", tty: false, args: []string{"--config", "x.json"}, wantBanner: false},
		{name: "TTY but --plain -> no banner", tty: true, args: []string{"--plain", "--config", "x.json"}, wantBanner: false},
		{name: "TTY but --no-color -> no banner", tty: true, args: []string{"--no-color", "--config", "x.json"}, wantBanner: false},
		{name: "TTY but NO_COLOR=1 -> no banner", tty: true, noColorEnv: "1", args: []string{"--config", "x.json"}, wantBanner: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make the banner-present cases independent of the ambient environment:
			// an empty value reads as unset under the NO_COLOR convention.
			t.Setenv("NO_COLOR", tt.noColorEnv)

			c, stdout, stderr, rec := newTestCLI(0)
			c.isTTY = func(io.Writer) bool { return tt.tty }

			if got := c.serveCmd(tt.args); got != 0 {
				t.Fatalf("serveCmd exit = %d, want 0", got)
			}
			if !rec.called || rec.configPath != "x.json" {
				t.Errorf("boot must be reached with the parsed config path; called=%v path=%q", rec.called, rec.configPath)
			}
			gotBanner := strings.Contains(stderr.String(), "KORVUN")
			if gotBanner != tt.wantBanner {
				t.Errorf("banner present = %v, want %v; stderr=%q", gotBanner, tt.wantBanner, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Errorf("the pre-serve banner must never touch stdout, got %q", stdout.String())
			}
		})
	}
}

// TestStyleEnabled_degradesWithoutVT pins the R4/FR-STY-9 degradation branch
// through the vt seam: on an interactive TTY with no opt-out, styling is on only
// when the terminal can render VT sequences; when it cannot (a legacy Windows
// conhost that rejects ENABLE_VIRTUAL_TERMINAL_PROCESSING), styleEnabled falls back
// to plain so escapes never print literally. Driven by the injected vt seam so the
// branch is exercised deterministically on every OS (the real Windows probe needs a
// console the CI test process does not have).
func TestStyleEnabled_degradesWithoutVT(t *testing.T) {
	t.Setenv("NO_COLOR", "") // keep the TTY assertion independent of the ambient env

	c, _, _, _ := newTestCLI(0)
	c.isTTY = func(io.Writer) bool { return true } // an interactive terminal
	var w bytes.Buffer

	c.vt = func() bool { return true }
	if !c.styleEnabled(&w, false, false) {
		t.Error("styleEnabled must be true on a TTY with VT available and no opt-out")
	}

	c.vt = func() bool { return false }
	if c.styleEnabled(&w, false, false) {
		t.Error("styleEnabled must degrade to plain when VT is unavailable, even on a TTY (R4)")
	}
}

// --- helpers ---

func assertStream(t *testing.T, name, got string, contains []string, wantEmpty bool) {
	t.Helper()
	if wantEmpty && got != "" {
		t.Errorf("%s: want empty, got %q", name, got)
	}
	for _, sub := range contains {
		if !strings.Contains(got, sub) {
			t.Errorf("%s: missing %q; got:\n%s", name, sub, got)
		}
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package cli tests — Piece 3, sub-phase 1 (scaffold + dispatch + version + logo).
//
// RED phase: this file defines the contract of internal/cli BEFORE the
// implementation exists. It compiles against a surface the green phase must
// provide:
//
//	func Run(args []string, stdout, stderr io.Writer) int
//	type cli struct {
//	    stdout, stderr io.Writer
//	    version        string             // ldflags-set default via a package var
//	    isTTY          func(io.Writer) bool
//	    serve          func([]string) int // serve seam; the real serve is sub-phase 2
//	}
//	func newCLI(stdout, stderr io.Writer) *cli // real defaults
//	func (c *cli) run(args []string) int
//
// Run is the ONLY public entry; main is a 3-line glue that forwards os.Args and
// os.Std* into it (ADR-0017: main stays un-unit-tested glue), so every behaviour
// is exercised here against injected buffers — never the real os.Stdout/os.Stderr.
// The seams (isTTY, serve) are injected so a test can drive TTY-gated styling and
// serve routing without a real terminal or a real boot.
package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// serveRec records an invocation of the serve seam so a test can assert the
// dispatch routed to serve with the exact args (including the config path),
// without serve actually existing yet (sub-phase 2).
type serveRec struct {
	called bool
	args   []string
	ret    int // the exit code the fake serve returns (to prove Run forwards it)
}

func (r *serveRec) fn(args []string) int {
	r.called = true
	r.args = args
	return r.ret
}

// newTestCLI wires a cli over two buffers with the seams stubbed for a
// non-interactive run: isTTY=false (no terminal), a recording serve seam. Tests
// override the individual seams they exercise.
func newTestCLI(serveRet int) (c *cli, stdout, stderr *bytes.Buffer, rec *serveRec) {
	stdout = &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	rec = &serveRec{ret: serveRet}
	c = newCLI(stdout, stderr)
	c.isTTY = func(io.Writer) bool { return false } // default: not a terminal
	c.serve = rec.fn
	return c, stdout, stderr, rec
}

const escape = "\x1b" // ANSI escape introducer; must never reach stdout off-TTY

// TestRun_dispatch pins the subcommand dispatch table: which token routes where,
// on which stream, with which exit code, and whether it reaches the serve seam.
func TestRun_dispatch(t *testing.T) {
	const serveExit = 42 // a distinctive value to prove Run forwards serve's return

	tests := []struct {
		name            string
		args            []string
		wantExit        int
		wantServeCalled bool
		wantServeArgs   []string // when routed to serve, the exact args passed
		stdoutContains  []string
		stderrContains  []string
		stdoutEmpty     bool
		stderrEmpty     bool
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
			name:           "config -> announced but not yet available, exit 2",
			args:           []string{"config"},
			wantExit:       2,
			stdoutEmpty:    true,
			stderrContains: []string{"config", "not available", "help"},
		},
		{
			name:           "status -> announced but not yet available, exit 2",
			args:           []string{"status"},
			wantExit:       2,
			stdoutEmpty:    true,
			stderrContains: []string{"status", "not available", "help"},
		},
		{
			name:            "serve subcommand routes to the serve seam with args after 'serve'",
			args:            []string{"serve", "--config", "korvun.json"},
			wantExit:        serveExit,
			wantServeCalled: true,
			wantServeArgs:   []string{"--config", "korvun.json"},
		},
		{
			name:            "shim: -config <path> without a subcommand -> implicit serve, full args",
			args:            []string{"-config", "korvun.local.json"},
			wantExit:        serveExit,
			wantServeCalled: true,
			wantServeArgs:   []string{"-config", "korvun.local.json"},
		},
		{
			name:            "shim: --config <path> -> implicit serve",
			args:            []string{"--config", "x.json"},
			wantExit:        serveExit,
			wantServeCalled: true,
			wantServeArgs:   []string{"--config", "x.json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, stdout, stderr, rec := newTestCLI(serveExit)

			got := c.run(tt.args)

			if got != tt.wantExit {
				t.Errorf("exit code = %d, want %d", got, tt.wantExit)
			}
			if rec.called != tt.wantServeCalled {
				t.Errorf("serve called = %v, want %v", rec.called, tt.wantServeCalled)
			}
			if tt.wantServeCalled && !equalArgs(rec.args, tt.wantServeArgs) {
				t.Errorf("serve args = %q, want %q", rec.args, tt.wantServeArgs)
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

func equalArgs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

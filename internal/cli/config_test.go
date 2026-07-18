// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package cli tests — Piece 3, sub-phase 3: `config check [--preflight] [path]`.
//
// These pin the config-noun dispatch, the offline validate default (config.Load,
// which validates and returns the first field-path violation), the online
// preflight behind an injectable seam (c.preflight, faked so tests never touch the
// network), and the first semantic color roles (success/error, ADR-0030) gated by
// styleEnabled with a textual label always present (color is never the only
// channel). All behaviour runs against injected buffers — never real os.Std*.
package cli

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
)

// validConfigJSON passes config.Load + Validate (structure/enums only; no secret
// resolution, no network): one Telegram channel, one local Ollama brain, one route.
const validConfigJSON = `{
  "channels": [{"type":"telegram","mode":"polling","token_env":"TELEGRAM_BOT_TOKEN"}],
  "brains": [{
    "name":"default","sensitivity":"private","dispatch":"fanout",
    "policy":{"kind":"priority","order":["ollama"]},
    "models":[{"provider":"ollama","model_id":"llama3.2","locality":"local","base_url":"http://localhost:11434"}]
  }],
  "routes": [{"channel":"telegram","brain":"default"}]
}`

// invalidConfigJSON fails Validate with a concrete field path: an unknown channel
// type surfaces as `channels[0].type: unknown channel type ...`.
const invalidConfigJSON = `{
  "channels": [{"type":"carrier-pigeon","mode":"polling","token_env":"X"}],
  "brains": [{
    "name":"d","sensitivity":"private","dispatch":"fanout",
    "policy":{"kind":"priority","order":["ollama"]},
    "models":[{"provider":"ollama","model_id":"m","locality":"local"}]
  }],
  "routes": [{"channel":"carrier-pigeon","brain":"d"}]
}`

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "korvun.json")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return p
}

// TestConfigCheck_offline pins FR-CHK-1: the offline default validates the config
// with no network and no secrets — a clean config prints an OK line to stdout
// (exit 0), a malformed/invalid one prints the first field-path violation to
// stderr (exit 2), and a missing file is likewise a usage error (exit 2).
func TestConfigCheck_offline(t *testing.T) {
	t.Run("valid config -> OK to stdout, exit 0, stderr empty", func(t *testing.T) {
		c, stdout, stderr, _ := newTestCLI(0)
		path := writeConfig(t, validConfigJSON)

		got := c.run([]string{"config", "check", path})

		if got != 0 {
			t.Errorf("exit = %d, want 0; stderr=%q", got, stderr.String())
		}
		out := stdout.String()
		if !strings.Contains(out, "OK") || !strings.Contains(out, path) {
			t.Errorf("stdout should carry an OK line naming the path; got %q", out)
		}
		if strings.Contains(out, "preflight") {
			t.Errorf("offline check must NOT claim preflight; got %q", out)
		}
		if stderr.Len() != 0 {
			t.Errorf("a clean check must leave stderr empty, got %q", stderr.String())
		}
	})

	t.Run("invalid config -> first field path to stderr, exit 2", func(t *testing.T) {
		c, stdout, stderr, _ := newTestCLI(0)
		path := writeConfig(t, invalidConfigJSON)

		got := c.run([]string{"config", "check", path})

		if got != 2 {
			t.Errorf("exit = %d, want 2", got)
		}
		if !strings.Contains(stderr.String(), "channels[0].type") {
			t.Errorf("stderr should report the offending field path; got %q", stderr.String())
		}
		if stdout.Len() != 0 {
			t.Errorf("a failed check must keep stdout empty, got %q", stdout.String())
		}
	})

	t.Run("missing file -> usage error, exit 2, no network", func(t *testing.T) {
		c, stdout, stderr, _ := newTestCLI(0)
		missing := filepath.Join(t.TempDir(), "nope.json")

		got := c.run([]string{"config", "check", missing})

		if got != 2 {
			t.Errorf("exit = %d, want 2", got)
		}
		if stderr.Len() == 0 {
			t.Error("a missing config must report an error to stderr")
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout must stay empty, got %q", stdout.String())
		}
	})

	t.Run("default path korvun.json when no positional given", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		if err := os.WriteFile(filepath.Join(dir, "korvun.json"), []byte(validConfigJSON), 0o600); err != nil {
			t.Fatalf("write default config: %v", err)
		}
		c, stdout, _, _ := newTestCLI(0)

		if got := c.run([]string{"config", "check"}); got != 0 {
			t.Fatalf("exit = %d, want 0 (default korvun.json)", got)
		}
		if !strings.Contains(stdout.String(), "korvun.json") {
			t.Errorf("OK line should name the default korvun.json; got %q", stdout.String())
		}
	})
}

// TestConfigCheck_preflight pins FR-CHK-2: --preflight runs the injected online
// seam. A preflight failure is a runtime failure (exit 1) with the named error to
// stderr; success extends the OK line to note preflight passed; and the seam is
// reached ONLY when --preflight is given.
func TestConfigCheck_preflight(t *testing.T) {
	t.Run("preflight passes -> OK line notes preflight, exit 0", func(t *testing.T) {
		c, stdout, stderr, _ := newTestCLI(0)
		called := false
		c.preflight = func(*config.Config) error { called = true; return nil }
		path := writeConfig(t, validConfigJSON)

		got := c.run([]string{"config", "check", "--preflight", path})

		if got != 0 {
			t.Errorf("exit = %d, want 0; stderr=%q", got, stderr.String())
		}
		if !called {
			t.Error("--preflight must reach the preflight seam")
		}
		if !strings.Contains(stdout.String(), "preflight") {
			t.Errorf("OK line should note preflight passed; got %q", stdout.String())
		}
	})

	t.Run("preflight fails -> named error to stderr, exit 1", func(t *testing.T) {
		c, stdout, stderr, _ := newTestCLI(0)
		c.preflight = func(*config.Config) error { return errors.New("bad token: unauthorized") }
		path := writeConfig(t, validConfigJSON)

		got := c.run([]string{"config", "check", "--preflight", path})

		if got != 1 {
			t.Errorf("exit = %d, want 1 (preflight is a runtime failure)", got)
		}
		if !strings.Contains(stderr.String(), "bad token: unauthorized") {
			t.Errorf("stderr should carry the named preflight error; got %q", stderr.String())
		}
		if stdout.Len() != 0 {
			t.Errorf("a failed preflight must keep stdout empty, got %q", stdout.String())
		}
	})

	t.Run("path-first: `config check <path> --preflight` still enables preflight", func(t *testing.T) {
		c, stdout, stderr, _ := newTestCLI(0)
		called := false
		c.preflight = func(*config.Config) error { called = true; return nil }
		path := writeConfig(t, validConfigJSON)

		// A flag AFTER the positional path must not be swallowed as a second path
		// (Go's flag parser stops at the first positional; configCheck permutes).
		got := c.run([]string{"config", "check", path, "--preflight"})

		if got != 0 {
			t.Errorf("exit = %d, want 0; stderr=%q", got, stderr.String())
		}
		if !called {
			t.Error("a flag placed after the path must still reach the preflight seam")
		}
		if !strings.Contains(stdout.String(), "preflight") {
			t.Errorf("OK line should note preflight passed; got %q", stdout.String())
		}
	})

	t.Run("seam NOT reached without --preflight", func(t *testing.T) {
		c, _, _, _ := newTestCLI(0)
		called := false
		c.preflight = func(*config.Config) error { called = true; return nil }
		path := writeConfig(t, validConfigJSON)

		if got := c.run([]string{"config", "check", path}); got != 0 {
			t.Fatalf("exit = %d, want 0", got)
		}
		if called {
			t.Error("preflight seam must NOT run for an offline check")
		}
	})
}

// TestConfigCheck_style pins the first color roles (ADR-0030) and R1–R3: the OK
// line is success-colored on an interactive stdout and the error is error-colored
// on an interactive stderr, but BOTH carry a textual label (color is never the
// only channel), and stdout is escape-free under --plain / non-TTY.
func TestConfigCheck_style(t *testing.T) {
	t.Setenv("NO_COLOR", "") // keep TTY assertions independent of the ambient env

	t.Run("OK line colored on a TTY stdout, label still present", func(t *testing.T) {
		c, stdout, _, _ := newTestCLI(0)
		c.isTTY = func(io.Writer) bool { return true }
		path := writeConfig(t, validConfigJSON)

		c.run([]string{"config", "check", path})

		out := stdout.String()
		if !strings.Contains(out, escape) {
			t.Errorf("on a TTY the OK line should be colored; got %q", out)
		}
		if !strings.Contains(out, "OK") {
			t.Errorf("color is never the only channel: the OK label must be present; got %q", out)
		}
	})

	t.Run("stdout escape-free under --plain even on a TTY (R3)", func(t *testing.T) {
		c, stdout, _, _ := newTestCLI(0)
		c.isTTY = func(io.Writer) bool { return true }
		path := writeConfig(t, validConfigJSON)

		c.run([]string{"config", "check", "--plain", path})

		if strings.Contains(stdout.String(), escape) {
			t.Errorf("--plain must strip every escape from stdout; got %q", stdout.String())
		}
		if !strings.Contains(stdout.String(), "OK") {
			t.Errorf("the OK label must still be present under --plain; got %q", stdout.String())
		}
	})

	t.Run("stdout escape-free on a non-TTY (R1)", func(t *testing.T) {
		c, stdout, _, _ := newTestCLI(0) // isTTY=false by default
		path := writeConfig(t, validConfigJSON)

		c.run([]string{"config", "check", path})

		if strings.Contains(stdout.String(), escape) {
			t.Errorf("non-TTY stdout must be escape-free; got %q", stdout.String())
		}
	})

	t.Run("error colored on a TTY stderr but plain under --plain", func(t *testing.T) {
		path := writeConfig(t, invalidConfigJSON)

		cTTY, _, stderrTTY, _ := newTestCLI(0)
		cTTY.isTTY = func(io.Writer) bool { return true }
		cTTY.run([]string{"config", "check", path})
		if !strings.Contains(stderrTTY.String(), escape) {
			t.Errorf("on a TTY the error line should be colored; got %q", stderrTTY.String())
		}

		cPlain, _, stderrPlain, _ := newTestCLI(0)
		cPlain.isTTY = func(io.Writer) bool { return true }
		cPlain.run([]string{"config", "check", "--plain", path})
		if strings.Contains(stderrPlain.String(), escape) {
			t.Errorf("--plain must strip escapes from the error line; got %q", stderrPlain.String())
		}
		if !strings.Contains(stderrPlain.String(), "channels[0].type") {
			t.Errorf("the field path must survive under --plain; got %q", stderrPlain.String())
		}
	})
}

// TestConfigCheck_usageErrors pins config check's own flag surface: an unknown
// flag and more than one positional path are both usage errors (exit 2) written to
// the injected stderr, leaving stdout clean and never touching config.Load.
func TestConfigCheck_usageErrors(t *testing.T) {
	t.Run("unknown flag -> usage error, exit 2", func(t *testing.T) {
		c, stdout, stderr, _ := newTestCLI(0)

		got := c.run([]string{"config", "check", "--definitely-not-a-flag"})

		if got != 2 {
			t.Errorf("exit = %d, want 2", got)
		}
		if !strings.Contains(stderr.String(), "definitely-not-a-flag") {
			t.Errorf("stderr should name the offending flag; got %q", stderr.String())
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout must stay empty, got %q", stdout.String())
		}
	})

	t.Run("more than one path -> usage error, exit 2", func(t *testing.T) {
		c, stdout, stderr, _ := newTestCLI(0)

		got := c.run([]string{"config", "check", "a.json", "b.json"})

		if got != 2 {
			t.Errorf("exit = %d, want 2", got)
		}
		if stderr.Len() == 0 {
			t.Error("too many paths must report a usage error to stderr")
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout must stay empty, got %q", stdout.String())
		}
	})

	t.Run("residual token after -- -> usage error, exit 2 (nothing silently ignored)", func(t *testing.T) {
		c, stdout, stderr, _ := newTestCLI(0)

		// A dash-looking token after the `--` terminator survives as a FlagSet
		// positional; config check must reject it, not silently drop it (symmetry
		// with serve's strictness).
		got := c.run([]string{"config", "check", "--", "-x"})

		if got != 2 {
			t.Errorf("exit = %d, want 2", got)
		}
		if !strings.Contains(stderr.String(), "-x") {
			t.Errorf("stderr should name the unexpected token; got %q", stderr.String())
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout must stay empty, got %q", stdout.String())
		}
	})
}

// TestConfigCheck_help pins that a help request on config check (-h/--help) is a
// query: exit 0, usage to stdout, stderr empty, and neither the config load nor the
// preflight seam runs.
func TestConfigCheck_help(t *testing.T) {
	for _, form := range [][]string{{"--help"}, {"-h"}} {
		t.Run(strings.Join(form, " "), func(t *testing.T) {
			c, stdout, stderr, _ := newTestCLI(0)
			preflighted := false
			c.preflight = func(*config.Config) error { preflighted = true; return nil }

			got := c.run(append([]string{"config", "check"}, form...))

			if got != 0 {
				t.Errorf("exit = %d, want 0", got)
			}
			if preflighted {
				t.Error("config check help must not run preflight")
			}
			if !strings.Contains(stdout.String(), "preflight") {
				t.Errorf("config check help should describe the --preflight flag on stdout; got %q", stdout.String())
			}
			if stderr.Len() != 0 {
				t.Errorf("config check help is a query: stderr must stay empty, got %q", stderr.String())
			}
		})
	}
}

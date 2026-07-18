// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package cli tests — Piece 3, sub-phase 4: `korvun status [--addr]`.
//
// status is a thin HTTP client of the already-serving admin API (ADR-0022): it
// GETs /healthz + /api/brains + /api/channels and renders an aligned table. These
// tests point --addr at an httptest.Server on an ephemeral port (never the real
// 2112), so the whole client path — reachability gate, JSON decode, table render,
// colored health, honest failure — is exercised hermetically. All output is checked
// against injected buffers, never real os.Std*.
package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	testBrainsJSON = `[
	  {"name":"default","sensitivity":"private","policy":"priority","dispatch":"fanout",
	   "models":[{"provider":"ollama","model_id":"llama3.2"}]},
	  {"name":"cloudy","sensitivity":"public","policy":"consensus","dispatch":"fanout",
	   "models":[{"provider":"groq","model_id":"llama-3.3-70b"},{"provider":"ollama","model_id":"llama3.2"}]}
	]`
	testChannelsJSON = `[{"type":"telegram","mode":"polling","name":"telegram","dropped":0}]`
)

// newAdminServer spins an httptest.Server that mimics the korvun admin API: a
// /healthz returning healthzStatus and the two JSON array endpoints. t.Cleanup
// closes it. Returns the host:port (no scheme) to pass as --addr.
func newAdminServer(t *testing.T, brains, channels string, healthzStatus int) (addr string) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(healthzStatus)
		_, _ = io.WriteString(w, "ok")
	})
	mux.HandleFunc("/api/brains", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, brains)
	})
	mux.HandleFunc("/api/channels", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, channels)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

// TestStatus_table pins FR-STA-1/3: against a live (test) admin API, status prints
// the resolved wiring — brains with their surviving models, channels with drop
// counts — as an aligned table to stdout, exit 0, escape-free under --plain.
func TestStatus_table(t *testing.T) {
	addr := newAdminServer(t, testBrainsJSON, testChannelsJSON, http.StatusOK)
	c, stdout, stderr, _ := newTestCLI(0)

	got := c.run([]string{"status", "--addr", addr, "--plain"})

	if got != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", got, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"HEALTH", "up",
		"BRAINS", "NAME", "SENSITIVITY", "POLICY", "DISPATCH", "MODELS",
		"default", "private", "priority", "fanout", "ollama/llama3.2",
		"cloudy", "consensus", "groq/llama-3.3-70b",
		"CHANNELS", "TYPE", "MODE", "DROPPED", "telegram", "polling",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status stdout missing %q; got:\n%s", want, out)
		}
	}
	if strings.Contains(out, escape) {
		t.Errorf("--plain stdout must be escape-free; got %q", out)
	}
	if stderr.Len() != 0 {
		t.Errorf("a healthy status must leave stderr empty, got %q", stderr.String())
	}
}

// TestStatus_addrWithScheme pins that a scheme-qualified --addr (a natural habit,
// since addresses are usually URLs) still reaches the admin API instead of building
// a malformed http://http://… base and misreporting a live server as unreachable.
func TestStatus_addrWithScheme(t *testing.T) {
	addr := newAdminServer(t, testBrainsJSON, testChannelsJSON, http.StatusOK)
	c, stdout, stderr, _ := newTestCLI(0)

	got := c.run([]string{"status", "--addr", "http://" + addr, "--plain"})

	if got != 0 {
		t.Fatalf("exit = %d, want 0 (scheme-qualified addr must still resolve); stderr=%q", got, stderr.String())
	}
	if !strings.Contains(stdout.String(), "HEALTH") {
		t.Errorf("status should render against a scheme-qualified addr; got %q", stdout.String())
	}
}

// TestStatus_healthColorGating pins FR-STY-6 + R1–R3: the health indicator is
// colored on an interactive stdout, plain otherwise, and the "up" label is always
// present (color is never the only channel); columns/text are otherwise identical.
func TestStatus_healthColorGating(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	addr := newAdminServer(t, testBrainsJSON, testChannelsJSON, http.StatusOK)

	t.Run("colored on a TTY", func(t *testing.T) {
		c, stdout, _, _ := newTestCLI(0)
		c.isTTY = func(io.Writer) bool { return true }
		if got := c.run([]string{"status", "--addr", addr}); got != 0 {
			t.Fatalf("exit = %d, want 0", got)
		}
		out := stdout.String()
		if !strings.Contains(out, escape) {
			t.Errorf("health should be colored on a TTY; got %q", out)
		}
		if !strings.Contains(out, "up") {
			t.Errorf("the 'up' label must be present even when colored; got %q", out)
		}
	})

	t.Run("plain under --plain even on a TTY (R3)", func(t *testing.T) {
		c, stdout, _, _ := newTestCLI(0)
		c.isTTY = func(io.Writer) bool { return true }
		if got := c.run([]string{"status", "--addr", addr, "--plain"}); got != 0 {
			t.Fatalf("exit = %d, want 0", got)
		}
		if strings.Contains(stdout.String(), escape) {
			t.Errorf("--plain must strip every escape from stdout; got %q", stdout.String())
		}
	})

	t.Run("plain on a non-TTY (R1)", func(t *testing.T) {
		c, stdout, _, _ := newTestCLI(0) // isTTY=false
		if got := c.run([]string{"status", "--addr", addr}); got != 0 {
			t.Fatalf("exit = %d, want 0", got)
		}
		if strings.Contains(stdout.String(), escape) {
			t.Errorf("non-TTY stdout must be escape-free; got %q", stdout.String())
		}
	})
}

// TestStatus_unreachable pins FR-STA-2: a dial failure or a non-200 /healthz is an
// honest failure — a clear message to stderr, exit 1, no panic, stdout empty.
func TestStatus_unreachable(t *testing.T) {
	t.Run("connection refused -> exit 1 honest message", func(t *testing.T) {
		// Start a server just to claim a real address, then close it so the port
		// refuses connections.
		srv := httptest.NewServer(http.NewServeMux())
		closedAddr := strings.TrimPrefix(srv.URL, "http://")
		srv.Close()

		c, stdout, stderr, _ := newTestCLI(0)
		got := c.run([]string{"status", "--addr", closedAddr})

		if got != 1 {
			t.Errorf("exit = %d, want 1", got)
		}
		if !strings.Contains(stderr.String(), "not reachable") || !strings.Contains(stderr.String(), closedAddr) {
			t.Errorf("stderr should carry an honest 'not reachable' line naming the addr; got %q", stderr.String())
		}
		if stdout.Len() != 0 {
			t.Errorf("a failed status must keep stdout empty, got %q", stdout.String())
		}
	})

	t.Run("non-200 /healthz -> exit 1", func(t *testing.T) {
		addr := newAdminServer(t, testBrainsJSON, testChannelsJSON, http.StatusServiceUnavailable)
		c, stdout, stderr, _ := newTestCLI(0)

		got := c.run([]string{"status", "--addr", addr})

		if got != 1 {
			t.Errorf("exit = %d, want 1", got)
		}
		if !strings.Contains(stderr.String(), "not reachable") {
			t.Errorf("a non-200 /healthz is unreachable; got %q", stderr.String())
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout must stay empty, got %q", stdout.String())
		}
	})
}

// TestStatus_dropsWarn pins the warn role (ADR-0030 #F59E0B): when a channel is
// dropping messages, a standalone warn line surfaces it (color on a TTY, textual
// label always present). Kept OUT of the tabwriter table so column widths are
// unaffected by escapes.
func TestStatus_dropsWarn(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	channels := `[{"type":"telegram","mode":"polling","name":"telegram","dropped":7}]`
	addr := newAdminServer(t, testBrainsJSON, channels, http.StatusOK)

	c, stdout, _, _ := newTestCLI(0)
	c.isTTY = func(io.Writer) bool { return true }

	if got := c.run([]string{"status", "--addr", addr}); got != 0 {
		t.Fatalf("exit = %d, want 0", got)
	}
	out := stdout.String()
	if !strings.Contains(out, "7") {
		t.Errorf("drop count should appear; got %q", out)
	}
	if !strings.Contains(strings.ToLower(out), "warn") {
		t.Errorf("a dropping channel should surface a warning line; got %q", out)
	}
	if !strings.Contains(out, escape) {
		t.Errorf("the warn line should be colored on a TTY; got %q", out)
	}
}

// TestStatus_dataEndpointErrors pins the reachable-but-misbehaving path: /healthz
// is OK but a data endpoint returns a non-200 or malformed JSON — status fails
// honestly (exit 1, message to stderr, stdout empty), never a panic.
func TestStatus_dataEndpointErrors(t *testing.T) {
	t.Run("non-200 on /api/brains -> exit 1", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
		mux.HandleFunc("/api/brains", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
		mux.HandleFunc("/api/channels", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, testChannelsJSON) })
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c, stdout, stderr, _ := newTestCLI(0)
		got := c.run([]string{"status", "--addr", strings.TrimPrefix(srv.URL, "http://")})

		if got != 1 {
			t.Errorf("exit = %d, want 1", got)
		}
		if !strings.Contains(stderr.String(), "unexpected") {
			t.Errorf("stderr should report the unexpected admin response; got %q", stderr.String())
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout must stay empty, got %q", stdout.String())
		}
	})

	t.Run("malformed JSON on /api/channels -> exit 1", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
		mux.HandleFunc("/api/brains", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, testBrainsJSON) })
		mux.HandleFunc("/api/channels", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "{not json") })
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c, _, stderr, _ := newTestCLI(0)
		if got := c.run([]string{"status", "--addr", strings.TrimPrefix(srv.URL, "http://")}); got != 1 {
			t.Errorf("exit = %d, want 1", got)
		}
		if !strings.Contains(stderr.String(), "unexpected") {
			t.Errorf("a decode failure should be reported honestly; got %q", stderr.String())
		}
	})
}

// TestStatus_brainWithNoModels pins the empty-models render: a Private brain whose
// cloud model the privacy selector dropped shows "-" in the MODELS column.
func TestStatus_brainWithNoModels(t *testing.T) {
	brains := `[{"name":"locked","sensitivity":"private","policy":"priority","dispatch":"fanout","models":[]}]`
	addr := newAdminServer(t, brains, testChannelsJSON, http.StatusOK)

	c, stdout, _, _ := newTestCLI(0)
	if got := c.run([]string{"status", "--addr", addr, "--plain"}); got != 0 {
		t.Fatalf("exit = %d, want 0", got)
	}
	out := stdout.String()
	if !strings.Contains(out, "locked") || !strings.Contains(out, "-") {
		t.Errorf("a brain with no surviving models should render '-' for MODELS; got:\n%s", out)
	}
}

// TestStatus_flagSurface pins status' own flag surface via the shared helper: a bad
// flag is a usage error (exit 2), a stray positional is a usage error (exit 2,
// uniform strictness), and -h/--help is a query (exit 0, usage to stdout) — none of
// which dial the admin API.
func TestStatus_flagSurface(t *testing.T) {
	t.Run("unknown flag -> exit 2", func(t *testing.T) {
		c, stdout, stderr, _ := newTestCLI(0)
		if got := c.run([]string{"status", "--definitely-not-a-flag"}); got != 2 {
			t.Errorf("exit = %d, want 2", got)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout must stay empty, got %q", stdout.String())
		}
		if !strings.Contains(stderr.String(), "definitely-not-a-flag") {
			t.Errorf("stderr should name the bad flag; got %q", stderr.String())
		}
	})

	t.Run("stray positional -> exit 2", func(t *testing.T) {
		c, _, stderr, _ := newTestCLI(0)
		if got := c.run([]string{"status", "extra"}); got != 2 {
			t.Errorf("exit = %d, want 2", got)
		}
		if !strings.Contains(stderr.String(), "extra") {
			t.Errorf("stderr should name the unexpected argument; got %q", stderr.String())
		}
	})

	t.Run("--help -> exit 0, usage to stdout", func(t *testing.T) {
		c, stdout, stderr, _ := newTestCLI(0)
		if got := c.run([]string{"status", "--help"}); got != 0 {
			t.Errorf("exit = %d, want 0", got)
		}
		if !strings.Contains(stdout.String(), "addr") {
			t.Errorf("status help should describe --addr on stdout; got %q", stdout.String())
		}
		if stderr.Len() != 0 {
			t.Errorf("status help is a query: stderr must stay empty, got %q", stderr.String())
		}
	})
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBootServe_configFatal covers the boot seam's only unit-testable path — a
// config that cannot be loaded returns a named fatal (exit 1) BEFORE the
// supervisor starts. The happy path (supervisor.Run serving until a real
// SIGINT/SIGTERM) is boot glue exercised by internal/app's lifecycle e2e, not
// here (ADR-0017): bootServe is the relocated pre-CLI boot body and still logs to
// os.Stderr verbatim, so this test emits one fatal JSON line to stderr by design.
// Flag parsing / validation is no longer here — sub-phase 2 moved it to serveCmd
// (unit-tested against injected writers), leaving bootServe as pure boot glue.
func TestBootServe_configFatal(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.json")
	if got := bootServe(missing); got != 1 {
		t.Errorf("bootServe(missing config) = %d, want 1", got)
	}
}

// TestBootServe_buildFatal covers the boot glue's second fatal path: a config
// that Loads/Validates cleanly but whose secret cannot be resolved makes
// app.Build fail, so the supervisor's initial start returns an error and bootServe
// exits 1 (serveFatal "run"). This exercises the whole supervisor-wiring block
// HERMETICALLY — the groq api-key env var is absent, so buildGroqModel returns
// ErrMissingSecret at a pure os.Getenv check, BEFORE any network dial or port
// bind. No app is served; only the boot-error path runs. The clean-stop happy
// tail (a real SIGINT drain) stays internal/app e2e territory (ADR-0017).
func TestBootServe_buildFatal(t *testing.T) {
	// Force the referenced secrets absent (empty reads as unset), independent of
	// the developer's ambient environment.
	t.Setenv("KORVUN_SP2_ABSENT_TOKEN", "")
	t.Setenv("KORVUN_SP2_ABSENT_KEY", "")

	const cfg = `{
  "channels": [{"type":"telegram","mode":"polling","token_env":"KORVUN_SP2_ABSENT_TOKEN"}],
  "brains": [{
    "name":"default","sensitivity":"public","dispatch":"fanout",
    "policy":{"kind":"priority","order":["groq"]},
    "models":[{"provider":"groq","model_id":"llama-3.3-70b-versatile","locality":"cloud","api_key_env":"KORVUN_SP2_ABSENT_KEY"}]
  }],
  "routes": [{"channel":"telegram","brain":"default"}]
}`
	path := filepath.Join(t.TempDir(), "korvun.json")
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	if got := bootServe(path); got != 1 {
		t.Errorf("bootServe(unresolvable secret) = %d, want 1", got)
	}
}

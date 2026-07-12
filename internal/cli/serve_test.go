// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"path/filepath"
	"testing"
)

// TestServeMain_earlyExits covers the serve seam's pre-boot paths — the ones that
// return WITHOUT starting the supervisor, so they are unit-testable. The happy
// path (supervisor.Run serving until a real SIGINT/SIGTERM) is boot glue exercised
// by internal/app's lifecycle e2e, not here (ADR-0017): serveMain is the relocated
// pre-CLI main body and still logs to os.Stderr verbatim, so these tests emit a
// flag-usage / a fatal JSON line to stderr by design.
func TestServeMain_earlyExits(t *testing.T) {
	t.Run("unknown flag -> usage error, exit 2", func(t *testing.T) {
		if got := serveMain([]string{"--definitely-not-a-flag"}); got != 2 {
			t.Errorf("serveMain(bad flag) = %d, want 2", got)
		}
	})

	t.Run("unreadable config -> named fatal, exit 1 (the retrocompat shim's contract)", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "does-not-exist.json")
		if got := serveMain([]string{"-config", missing}); got != 1 {
			t.Errorf("serveMain(missing config) = %d, want 1", got)
		}
	})
}

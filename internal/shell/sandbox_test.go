// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// sandboxUserDir redirects the per-platform user-config root to a fresh
// temp dir so anything that resolves <os.UserConfigDir>/korvun/… during
// the test (an empty storage.path does exactly that at boot) stays inside
// the sandbox instead of opening the developer's real profile. It returns
// the sandbox root, and fails on the spot if the redirect did not take —
// the resolved dir escaping the sandbox is the regression this helper
// exists to prevent.
func sandboxUserDir(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	switch runtime.GOOS {
	case "darwin", "linux":
		t.Setenv("HOME", tmp)
		t.Setenv("XDG_CONFIG_HOME", "") // linux: fall through to HOME/.config
	case "windows":
		t.Setenv("AppData", tmp)
	}
	resolved, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("sandboxUserDir: UserConfigDir after the redirect: %v", err)
	}
	if resolved != tmp && !strings.HasPrefix(resolved, tmp+string(os.PathSeparator)) {
		t.Fatalf("sandboxUserDir: resolved user dir %q escaped the sandbox %q", resolved, tmp)
	}
	return tmp
}

// TestMain is the structural tripwire for the whole package: it snapshots
// the developer's REAL <os.UserConfigDir>/korvun before any test runs and
// fails the run if a test created or modified it. A test that boots a
// config whose storage.path is empty without sandboxUserDir lands exactly
// there — this makes that pattern impossible to regress in silence.
func TestMain(m *testing.M) {
	real, err := os.UserConfigDir()
	if err != nil {
		// No resolvable user dir (bare CI env): nothing to protect.
		os.Exit(m.Run())
	}
	guarded := filepath.Join(real, "korvun")
	before := snapshotDir(guarded)

	code := m.Run()

	after := snapshotDir(guarded)
	if drift := diffSnapshots(before, after); drift != "" {
		fmt.Fprintf(os.Stderr,
			"FAIL: package shell touched the REAL user profile dir %q during the test run:\n%s"+
				"A test resolved storage outside its sandbox (use sandboxUserDir), or the real\n"+
				"Korvun app was writing during the run — rerun with it stopped to disambiguate.\n",
			guarded, drift)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

// snapshotDir records name → "size mtime" for the directory's entries;
// nil means the directory does not exist.
func snapshotDir(dir string) map[string]string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	snap := make(map[string]string, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		snap[e.Name()] = fmt.Sprintf("%d %s", info.Size(), info.ModTime().Format(time.RFC3339Nano))
	}
	return snap
}

func diffSnapshots(before, after map[string]string) string {
	if after == nil {
		return "" // dir absent after the run: nothing was created
	}
	var b strings.Builder
	if before == nil {
		fmt.Fprintf(&b, "  - the directory was CREATED during the run (%d entries)\n", len(after))
		return b.String()
	}
	for name, sig := range after {
		prev, ok := before[name]
		switch {
		case !ok:
			fmt.Fprintf(&b, "  - %s: CREATED during the run\n", name)
		case prev != sig:
			fmt.Fprintf(&b, "  - %s: MODIFIED during the run (%s -> %s)\n", name, prev, sig)
		}
	}
	return b.String()
}

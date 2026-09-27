// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// The harness's isolation (isolationEnv, applied by isolate in run): the
// source os.UserConfigDir reads on macOS, Linux and Windows — and on Windows
// the one os.UserCacheDir reads — points inside the harness's temp dir. Until PR #69
// run set only HOME and XDG_CONFIG_HOME, so on Windows the harness wrote its
// scripted config over the user's REAL %AppData%\korvun\korvun.json and its
// core opened the real ledger (found by reading in the adversary's pass over
// the cures; never run on Windows).
//
// PROBING MUTATIONS: AppData dropped from the Windows map → reddens;
// LocalAppData dropped → reddens.
//
// Evidence level: unit, in process; every platform's map is checked on any
// host, the GOOS being an argument.
func TestIsolationEnv_pointsTheUserDirSourcesIntoTheTempDir(t *testing.T) {
	dir := filepath.Join("tmp", "korvun-harness")
	for _, c := range []struct {
		goos string
		want map[string]string
	}{
		{"windows", map[string]string{
			"HOME":            dir,
			"XDG_CONFIG_HOME": filepath.Join(dir, ".config"),
			"AppData":         filepath.Join(dir, "AppData", "Roaming"),
			"LocalAppData":    filepath.Join(dir, "AppData", "Local"),
		}},
		{"linux", map[string]string{"HOME": dir, "XDG_CONFIG_HOME": filepath.Join(dir, ".config")}},
		{"darwin", map[string]string{"HOME": dir, "XDG_CONFIG_HOME": filepath.Join(dir, ".config")}},
	} {
		if got := isolationEnv(c.goos, dir); !reflect.DeepEqual(got, c.want) {
			t.Errorf("isolationEnv(%q, %q) = %v, want %v", c.goos, dir, got, c.want)
		}
	}
}

// On THIS host, once isolate has run, the user config dir — and on Windows the
// user cache dir — resolve inside the temp dir: the glue that
// TestIsolationEnv_pointsTheUserDirSourcesIntoTheTempDir does not reach
// (runtime.GOOS, os.Setenv). It proves isolate, not run: run's own isolation
// is TestHarnessBinary_neverTouchesTheRealUserConfigDir's, which starts the
// built harness as a separate process.
//
// PROBING MUTATION: isolate sets nothing → reddens on every host.
//
// Evidence level: in process, host OS only; it resolves paths and never
// writes under them.
func TestIsolate_theUserDirsResolveInsideTheTempDir(t *testing.T) {
	for k := range isolationEnv(runtime.GOOS, "x") {
		t.Setenv(k, os.Getenv(k)) // each variable isolate sets is restored when the test ends
	}
	dir := t.TempDir()
	if err := isolate(dir); err != nil {
		t.Fatalf("isolate: %v", err)
	}
	inside := func(p string) bool { return strings.HasPrefix(p, dir+string(os.PathSeparator)) }
	if cfg, err := os.UserConfigDir(); err != nil || !inside(cfg) {
		t.Fatalf("os.UserConfigDir() after isolate = %q (%v), want a path inside %q", cfg, err, dir)
	}
	if runtime.GOOS == "windows" {
		if cache, err := os.UserCacheDir(); err != nil || !inside(cache) {
			t.Fatalf("os.UserCacheDir() after isolate = %q (%v), want a path inside %q", cache, err, dir)
		}
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/app"
)

// e4SandboxUserConfigDir points os.UserConfigDir at a fresh temporary
// directory for the test and returns it: HOME and XDG_CONFIG_HOME for macOS
// and Linux, AppData for Windows — os.UserConfigDir's own sources on each.
// Redirecting HOME and XDG_CONFIG_HOME alone left Windows on the user's REAL
// AppData, where TE47 wrote its text at the default korvun.db (the first CI
// run of PR #69).
func e4SandboxUserConfigDir(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("AppData", home)
	return home
}

// Train E, batch 4 — the harness-only half of TE47 (plan v3, O10): the e2e
// harness's park door, whose store is not SQLite, answers its existing 503
// with the store's class and the native cause. It is a check of the test
// harness, not of the shipping desktop.
//
// The text goes to the default ledger path, so the user config dir is
// sandboxed first (e4SandboxUserConfigDir) and the path is asserted inside
// the sandbox before anything is written.
//
// PROBING MUTATIONS: the harness drops the opener's cause from its 503 →
// reddens (MU47); the sandbox removed → the path assertion reddens before any
// write.
//
// Evidence level: in process, the harness's own handler over a real file.
func TestE4_TE47_theHarnessParkDoorNamesTheOpenersClass(t *testing.T) {
	home := e4SandboxUserConfigDir(t)
	tc := testControl{approvals: true, cfgPath: filepath.Join(t.TempDir(), "korvun.json"), modelURL: "http://127.0.0.1:1"}
	path := app.StoragePath(harnessConfig(tc.modelURL, tc.approvals))
	if !strings.HasPrefix(path, home+string(os.PathSeparator)) {
		t.Fatalf("the harness's store resolves to %q, outside the isolated dir %q: refused before anything is written", path, home)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("this file is not a SQLite database; it is text.\n", 128)), 0o600); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/__test/park", strings.NewReader(`{}`))
	tc.park(w, r)
	body := w.Body.String()
	if w.Code != http.StatusServiceUnavailable || !strings.HasPrefix(body, "open the store: ") || !strings.Contains(body, "ledger_unreadable") || !strings.Contains(body, "not a database") {
		t.Fatalf("the park door over a text file = %d %q, want 503 «open the store:» with ledger_unreadable and the native cause", w.Code, body)
	}
}

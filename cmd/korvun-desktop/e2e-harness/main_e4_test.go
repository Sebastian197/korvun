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

// Train E, batch 4 — the harness-only half of TE47 (plan v3, O10): the e2e
// harness's park door, whose store is not SQLite, answers its existing 503
// with the store's class and the native cause. It is a check of the test
// harness, not of the shipping desktop.
//
// PROBING MUTATION (MU47): the harness drops the opener's cause from its 503
// → reddens.
//
// Evidence level: in process, the harness's own handler over a real file.
func TestE4_TE47_theHarnessParkDoorNamesTheOpenersClass(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	tc := testControl{approvals: true, cfgPath: filepath.Join(t.TempDir(), "korvun.json"), modelURL: "http://127.0.0.1:1"}
	path := app.StoragePath(harnessConfig(tc.modelURL, tc.approvals))
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

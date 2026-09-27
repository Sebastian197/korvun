// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.16.2 · the durable mark «ledger founded by this profile», end to end on
// the REAL supervisor (director's order, 2026-09-24): the ledger a profile
// founds from the screen knows that profile — the running app says `ok` without
// a restart, the operator's `ledger check` says `ok`, and the same ledger read
// from a profile copied elsewhere is named foreign.
//
// Evidence level, honest: in-process; the real shell.Controller and supervisor
// performing a real cutover; the CLI called in process. Not a packaged binary.
//
// Plan: docs/superpowers/specs/2026-09-24-v0162-el-libro-fundado-por-este-perfil-pretest.md, G6.

package shell

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/app"
	"github.com/Sebastian197/korvun/internal/cli"
)

// TestBootstrap_theFoundedLedgerKnowsItsProfile is G6.
//
// PROBING MUTATIONS: close the founding act with a plain Finish (the ledger
// reads legacy under both profiles; the `ok` legs redden); cache the standing
// at open (the running app keeps saying legacy after the close; the first leg
// reddens).
func TestBootstrap_theFoundedLedgerKnowsItsProfile(t *testing.T) {
	_, path, srv, _ := storelessController(t)
	code, out := pressDoor(t, srv, "enable-storage", `{}`)
	if code != http.StatusOK {
		t.Fatalf("enable-storage = %d %+v", code, out)
	}
	if out.Outcome == "applying" {
		if st := waitReloadThroughProxy(t, srv, out.Handle, firstRunBootTimeout); st["state"] != "succeeded" {
			t.Fatalf("founding ended %q", st["state"])
		}
	}
	ledger := defaultLedgerPath(t)
	if row := waitLedgerTerminal(t, ledger, out.Act.ActionID, 10*time.Second); row.State != "SUCCEEDED" {
		t.Fatalf("the founding act is %q", row.State)
	}

	// 1 · The running app, on the same cycle, no restart: the read door says ok.
	resp, body := get(t, srv.Client(), srv.URL+"/api/whats-happening", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/whats-happening = %d %q", resp.StatusCode, body)
	}
	var read struct {
		Ledger struct {
			Standing string `json:"standing"`
			Owner    string `json:"owner"`
		} `json:"ledger"`
	}
	if err := json.Unmarshal([]byte(body), &read); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if read.Ledger.Standing != "ok" || read.Ledger.Owner != app.ProfileIdentity(path) {
		t.Fatalf("the app that founded the ledger reads %+v, want ok owned by this profile %q", read.Ledger, app.ProfileIdentity(path))
	}

	// 2 · The operator's own tool, over the same profile: ok.
	var stdout, stderr strings.Builder
	if code := cli.Run([]string{"ledger", "check", "--config", path}, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "ledger standing: ok") {
		t.Fatalf("ledger check for the founder: %d %q %q", code, stdout.String(), stderr.String())
	}

	// 3 · The same profile document COPIED elsewhere, pointing at the same
	// ledger: foreign, named — and the chain still intact.
	raw, err := os.ReadFile(path) // #nosec G304 -- test-owned path
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	copy := filepath.Join(t.TempDir(), "elsewhere", "korvun.json")
	if err := os.MkdirAll(filepath.Dir(copy), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(copy, raw, 0o600); err != nil {
		t.Fatalf("write copy: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := cli.Run([]string{"ledger", "check", "--config", copy}, &stdout, &stderr); code != 0 ||
		!strings.Contains(stdout.String(), "ledger standing: ledger_foreign_profile") || !strings.Contains(stdout.String(), "chain intact") {
		t.Fatalf("ledger check for a copy elsewhere: %d %q %q, want foreign named and the chain intact", code, stdout.String(), stderr.String())
	}
}

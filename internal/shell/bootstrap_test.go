// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.16.2 · the operator act across a REAL supervisor cutover, and the store
// founded from the screen (the director's decision of 2026-09-24 on the profile
// with no ledger).
//
// These moulds exist because of a capture (evidence/v0.16.2/probe-real-cutover.txt):
// every mould of the act piece drove a FAKE reloader, and against the real
// supervisor the act ended OUTCOME_UNKNOWN — the app that sealed it was gone before
// the state turned terminal, and the next app's recovery took it for an orphan.
// Nothing below uses a fake reloader.
//
// Evidence level, honest: in-process, the REAL shell.Controller, the REAL
// supervisor performing a REAL cutover between two apps with real admin servers
// on loopback, real SQLite ledgers on disk read back through a read-only open.
// Not a packaged binary and not a separate OS process.
//
// Plan: docs/superpowers/specs/2026-09-24-v0162-el-almacen-y-el-cierre-del-acto-pretest.md,
// F2, F3, F4, F4b, F8, F9.

package shell

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/app"
	"github.com/Sebastian197/korvun/internal/config"
)

// ledgerRow reads an act and its receipts through a read-only open of the
// ledger at path — the operator's CLI's way, never the running app's handle.
func ledgerRow(t *testing.T, path, actionID string) (actionsqlite.Record, []action.Receipt) {
	t.Helper()
	ro, err := actionsqlite.OpenReadOnlyFor(path, testProfileIdentity)
	if err != nil {
		t.Fatalf("open ledger read-only: %v", err)
	}
	defer func() { _ = ro.Close() }()
	row, err := ro.Get(context.Background(), actionID)
	if err != nil {
		t.Fatalf("read act %s: %v", actionID, err)
	}
	receipts, err := ro.ReceiptsByAction(context.Background(), actionID)
	if err != nil {
		t.Fatalf("receipts of %s: %v", actionID, err)
	}
	return row, receipts
}

// waitLedgerTerminal polls the ledger (never the status door) until the act
// leaves AUTHORIZED, bounded.
func waitLedgerTerminal(t *testing.T, path, actionID string, budget time.Duration) actionsqlite.Record {
	t.Helper()
	deadline := time.Now().Add(budget)
	for {
		row, _ := ledgerRow(t, path, actionID)
		if row.State != action.StateAuthorized || time.Now().After(deadline) {
			return row
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// doorOutcome is what a screen button's POST answers, trimmed to what these
// moulds read.
type doorOutcome struct {
	Outcome string `json:"outcome"`
	Handle  string `json:"handle"`
	Detail  string `json:"detail"`
	Act     struct {
		ActionID  string `json:"action_id"`
		ReceiptID string `json:"receipt_id"`
	} `json:"act"`
}

// pressDoor POSTs a screen door through the shell proxy (which injects the
// cycle's bearer) and decodes the outcome.
func pressDoor(t *testing.T, srv *httptest.Server, door, body string) (int, doorOutcome) {
	t.Helper()
	resp, err := srv.Client().Post(srv.URL+"/api/whats-happening/"+door, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", door, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out doorOutcome
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// postConfigStatus POSTs a config through the proxy and returns the status and
// the decoded body, without asserting anything: the refusal IS the point for
// half of these moulds.
func postConfigStatus(t *testing.T, srv *httptest.Server, cfg *config.Config) (int, map[string]string) {
	t.Helper()
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	resp, err := srv.Client().Post(srv.URL+"/api/config", "application/json", strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("POST /api/config: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body
}

// waitReloadThroughProxy polls the status door through the proxy until a
// terminal state, tolerating the cutover window's 503s, and returns the body.
func waitReloadThroughProxy(t *testing.T, srv *httptest.Server, handle string, budget time.Duration) map[string]string {
	t.Helper()
	deadline := time.Now().Add(budget)
	var last map[string]string
	for time.Now().Before(deadline) {
		resp, err := srv.Client().Get(srv.URL + "/api/reload/" + handle)
		if err == nil {
			var body map[string]string
			_ = json.NewDecoder(resp.Body).Decode(&body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				last = body
				switch body["state"] {
				case "succeeded", "failed", "rolled-back", "persist-failed":
					return body
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("reload %s reached no terminal state within %s (last %v)", handle, budget, last)
	return nil
}

// storelessController boots the channel-less template WITHOUT a storage block
// under a sandboxed user dir, and serves the shell proxy over it.
func storelessController(t *testing.T) (*Controller, string, *httptest.Server, string) {
	t.Helper()
	root := sandboxUserDir(t)
	t.Setenv(adminTokenEnv, "")
	c := New(WithLogger(slog.New(slog.DiscardHandler)), WithBuildOptions(fakeFactory()))
	ollama := fakeOllama(t)
	tpl := templateCfg(ollama.URL)
	if tpl.Storage != nil {
		t.Fatal("the mould's premise is a template with NO storage")
	}
	path := writeCfg(t, tpl)
	if err := c.LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), firstRunBootTimeout)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if c.Status().Running {
			sctx, sc := context.WithTimeout(context.Background(), firstRunStopTimeout)
			defer sc()
			_ = c.Stop(sctx)
		}
	})
	srv := httptest.NewServer(c.ProxyHandler())
	t.Cleanup(srv.Close)
	return c, path, srv, root
}

// defaultLedgerPath is where a founded ledger lands: the same resolution the
// boot uses for an empty storage.path.
func defaultLedgerPath(t *testing.T) string {
	t.Helper()
	return app.StoragePath(&config.Config{})
}

// TestBootstrap_aRolledBackCutoverClosesTheFoundingActAsFailed is F3: the ledger
// is founded, the founding act sealed, and the cutover rolls back because the
// profile directory is locked by «another server» (this test holds the lock).
// The act says FAILED, the profile on disk still has no storage, the builder's
// door still refuses — and, once the lock is gone, a second press from the same
// process re-adopts the file it created and applies.
//
// PROBING MUTATIONS: close the founding act as applied whatever the state (the
// FAILED assertion reddens); forget what this process created (the retry reddens
// with ledger_exists).
func TestBootstrap_aRolledBackCutoverClosesTheFoundingActAsFailed(t *testing.T) {
	_, path, srv, _ := storelessController(t)
	ledger := defaultLedgerPath(t)
	lock, err := app.AcquireProfileLock(filepath.Dir(ledger))
	if err != nil {
		t.Fatalf("hold the profile lock: %v", err)
	}
	released := false
	t.Cleanup(func() {
		if !released {
			_ = lock.Release()
		}
	})

	code, out := pressDoor(t, srv, "enable-storage", `{}`)
	// «applying» or already «not_applied»: the POST answers while the real
	// supervisor is cutting over, and the state it reads may already be
	// terminal. This is a race of TIMING on an intermediate state, declared as
	// such — not a taxonomy hidden in an either/or: the terminal facts below
	// (state, act, receipt, profile) are exact.
	if code != http.StatusOK || out.Outcome != "applying" && out.Outcome != "not_applied" {
		t.Fatalf("enable-storage = %d %+v, want 200 applying/not_applied", code, out)
	}
	if out.Act.ActionID == "" {
		t.Fatal("no founding act answered")
	}
	if out.Outcome == "applying" {
		body := waitReloadThroughProxy(t, srv, out.Handle, 30*time.Second)
		if body["state"] != "rolled-back" {
			t.Fatalf("the cutover over a locked profile dir ended %q, want rolled-back", body["state"])
		}
	}
	row := waitLedgerTerminal(t, ledger, out.Act.ActionID, 10*time.Second)
	_, receipts := ledgerRow(t, ledger, out.Act.ActionID)
	if row.State != action.StateFailed || len(receipts) != 1 || receipts[0].Outcome != string(action.StateFailed) {
		t.Fatalf("the founding act of a rolled-back bootstrap is %q with receipts %+v, want FAILED with its receipt", row.State, receipts)
	}
	onDisk, err := config.Load(path)
	if err != nil {
		t.Fatalf("reload profile: %v", err)
	}
	if onDisk.Storage != nil {
		t.Fatalf("the profile on disk gained storage %+v over a rolled-back cutover", onDisk.Storage)
	}
	if code, body := postConfigStatus(t, srv, onDisk); code != http.StatusServiceUnavailable || body["error_code"] != "no_ledger" {
		t.Fatalf("after the rollback POST /api/config = %d %v, want 503 no_ledger", code, body)
	}

	// The repair: the lock is gone, the operator presses again.
	if err := lock.Release(); err != nil {
		t.Fatalf("release lock: %v", err)
	}
	released = true
	code, again := pressDoor(t, srv, "enable-storage", `{}`)
	if code != http.StatusOK || again.Outcome == "ledger_exists" {
		t.Fatalf("the retry over the file this process founded = %d %+v", code, again)
	}
	if again.Outcome == "applying" {
		if body := waitReloadThroughProxy(t, srv, again.Handle, 30*time.Second); body["state"] != "succeeded" {
			t.Fatalf("the retry ended %q, want succeeded", body["state"])
		}
	}
	second := waitLedgerTerminal(t, ledger, again.Act.ActionID, 10*time.Second)
	if second.State != action.StateSucceeded {
		t.Fatalf("the second founding act is %q, want SUCCEEDED", second.State)
	}
	first, _ := ledgerRow(t, ledger, out.Act.ActionID)
	if first.State != action.StateFailed {
		t.Fatalf("the first founding act was rewritten to %q: history must be kept", first.State)
	}
}

// TestBootstrap_anUnwritableProfileDirCreatesNoLedger is F4: no file, no act, no
// reload, profile untouched, and the builder's door still refuses.
//
// PROBING MUTATION: ask the reloader anyway. The profile gains storage over a
// ledger that does not exist and this reddens on the disk.
func TestBootstrap_anUnwritableProfileDirCreatesNoLedger(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission bits do not refuse writes here (windows, or root)")
	}
	_, path, srv, _ := storelessController(t)
	ledger := defaultLedgerPath(t)
	dir := filepath.Dir(ledger)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Chmod(dir, 0o500); err != nil { //nolint:gosec // an unwritable dir IS the attack; test-owned temp path
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) //nolint:gosec // restore the test-owned temp dir so TempDir can remove it

	code, out := pressDoor(t, srv, "enable-storage", `{}`)
	if code != http.StatusServiceUnavailable || out.Outcome != "ledger_not_created" {
		t.Fatalf("enable-storage over an unwritable dir = %d %+v, want 503 ledger_not_created", code, out)
	}
	if out.Act.ActionID != "" {
		t.Fatalf("an act %q was answered for a ledger that was never founded", out.Act.ActionID)
	}
	if _, err := os.Stat(ledger); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a ledger file exists after a refused founding: %v", err)
	}
	onDisk, err := config.Load(path)
	if err != nil {
		t.Fatalf("reload profile: %v", err)
	}
	if onDisk.Storage != nil {
		t.Fatalf("the profile gained storage %+v with no ledger", onDisk.Storage)
	}
	if code, body := postConfigStatus(t, srv, onDisk); code != http.StatusServiceUnavailable || body["error_code"] != "no_ledger" {
		t.Fatalf("POST /api/config = %d %v, want 503 no_ledger", code, body)
	}
}

// TestBootstrap_aLedgerAlreadyThereIsNeverAdopted is F4b: the default path holds
// a file this process did not create. Refused by name, bytes untouched, nothing
// asked of the supervisor.
//
// PROBING MUTATION: open whatever is there. The foreign bytes change and this
// reddens.
func TestBootstrap_aLedgerAlreadyThereIsNeverAdopted(t *testing.T) {
	_, path, srv, _ := storelessController(t)
	ledger := defaultLedgerPath(t)
	if err := os.MkdirAll(filepath.Dir(ledger), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	foreign := []byte("the desktop's own book, not this profile's")
	if err := os.WriteFile(ledger, foreign, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	code, out := pressDoor(t, srv, "enable-storage", `{}`)
	if code != http.StatusConflict || out.Outcome != "ledger_exists" {
		t.Fatalf("enable-storage over a foreign file = %d %+v, want 409 ledger_exists", code, out)
	}
	if !strings.Contains(out.Detail, ledger) {
		t.Fatalf("the detail %q does not name the path the operator must look at", out.Detail)
	}
	got, _ := os.ReadFile(ledger) // #nosec G304 -- sandboxed test path
	if string(got) != string(foreign) {
		t.Fatalf("the foreign file changed: %q", got)
	}
	onDisk, err := config.Load(path)
	if err != nil {
		t.Fatalf("reload profile: %v", err)
	}
	if onDisk.Storage != nil {
		t.Fatalf("the profile adopted %+v", onDisk.Storage)
	}
}

// storedController boots minimalCfg WITH a storage block in a temp dir and
// serves the shell proxy over it. It returns the ledger path too.
func storedController(t *testing.T) (*Controller, string, string, *httptest.Server, string) {
	t.Helper()
	srv := fakeOllama(t)
	t.Setenv(adminTokenEnv, "")
	c := testController(fakeFactory())
	cfg := minimalCfg(srv.URL)
	ledger := filepath.Join(t.TempDir(), "korvun.db")
	cfg.Storage = &config.StorageConfig{Path: ledger}
	path := writeCfg(t, cfg)
	if err := c.LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if c.Status().Running {
			sctx, sc := context.WithTimeout(context.Background(), 10*time.Second)
			defer sc()
			_ = c.Stop(sctx)
		}
	})
	proxy := httptest.NewServer(c.ProxyHandler())
	t.Cleanup(proxy.Close)
	return c, path, ledger, proxy, srv.URL
}

// TestReload_aRolledBackCutoverClosesTheActAsFailed is F8 on the REAL supervisor:
// the change moves storage.path to a directory whose profile lock this test
// holds, so Preflight passes (it opens no store) and the new app's Build
// refuses — rolled back, old ledger reopened by the rollback app. The act sealed
// in the OLD ledger must say FAILED, and the status door on the rollback app
// must answer that act's receipt.
//
// WHAT THIS MOULD PROVES, AND WHAT IT DOES NOT. It proves the OUTCOME: FAILED,
// with its receipt, through the real supervisor. It does NOT prove which hand
// closed it first: a close that fails on the dead app's store is retried by the
// next settler — the poll on the rollback app — and lands there, so the
// mutation «do not detach in Shutdown» stays green here (mutations.txt, M66,
// the method finding). That mutation's red lives in internal/app,
// `TestBuild_recoverySparesTheActsThisProcessOwns` (M66-bis).
//
// PROBING MUTATION (executed, M57): call RecoverPreviousLife with no keep in
// Build. The rollback app's boot closes the act OUTCOME_UNKNOWN and this
// reddens on the state.
func TestReload_aRolledBackCutoverClosesTheActAsFailed(t *testing.T) {
	_, _, ledger, srv, ollamaURL := storedController(t)
	lockedDir := t.TempDir()
	lock, err := app.AcquireProfileLock(lockedDir)
	if err != nil {
		t.Fatalf("hold the lock: %v", err)
	}
	t.Cleanup(func() { _ = lock.Release() })

	next := minimalCfg(ollamaURL)
	next.Storage = &config.StorageConfig{Path: filepath.Join(lockedDir, "korvun.db")}
	code, body := postConfigStatus(t, srv, next)
	if code != http.StatusAccepted {
		t.Fatalf("POST /api/config = %d %v, want 202", code, body)
	}
	actionID := body["action_id"]
	if actionID == "" {
		t.Fatal("the 202 carries no action_id")
	}
	terminal := waitReloadThroughProxy(t, srv, body["handle"], 30*time.Second)
	if terminal["state"] != "rolled-back" {
		t.Fatalf("the cutover ended %q, want rolled-back", terminal["state"])
	}
	row := waitLedgerTerminal(t, ledger, actionID, 10*time.Second)
	_, receipts := ledgerRow(t, ledger, actionID)
	if row.State != action.StateFailed || len(receipts) != 1 || receipts[0].Outcome != string(action.StateFailed) {
		t.Fatalf("the act of a rolled-back cutover is %q with receipts %+v, want FAILED with one receipt", row.State, receipts)
	}
	// And the status door on the rollback app answers THE receipt — not
	// nothing: the poll that sees `rolled-back` is served after the observer
	// closed the act, and a closed act always answers its receipt.
	if terminal["action_id"] != actionID || terminal["receipt_id"] != receipts[0].ReceiptID {
		t.Fatalf("the status door answered %v, want act %q with receipt %q", terminal, actionID, receipts[0].ReceiptID)
	}
}

// TestReload_theActClosesEvenIfNobodyPolls is F9: the desktop polls the status
// door once, right after the POST, long before the cutover ends. The process
// must close the act on its own. Nothing here touches /api/reload — the on-disk
// profile (persisted BEFORE the supervisor says succeeded) is the signal that
// the cutover happened.
//
// PROBING MUTATION: do not call the observer from setStatus. The act stays
// AUTHORIZED and this reddens.
func TestReload_theActClosesEvenIfNobodyPolls(t *testing.T) {
	_, path, ledger, srv, ollamaURL := storedController(t)
	next := minimalCfg(ollamaURL)
	next.Storage = &config.StorageConfig{Path: ledger}
	next.Brains[0].Models[0].ModelID = "llama3.2-unpolled"
	code, body := postConfigStatus(t, srv, next)
	if code != http.StatusAccepted {
		t.Fatalf("POST /api/config = %d %v, want 202", code, body)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		onDisk, _ := os.ReadFile(path) // #nosec G304 -- t.TempDir path
		if strings.Contains(string(onDisk), "llama3.2-unpolled") || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	row := waitLedgerTerminal(t, ledger, body["action_id"], 10*time.Second)
	_, receipts := ledgerRow(t, ledger, body["action_id"])
	if row.State != action.StateSucceeded || len(receipts) != 1 || receipts[0].Outcome != string(action.StateSucceeded) {
		t.Fatalf("with nobody polling, the act is %q with receipts %+v, want SUCCEEDED with its receipt", row.State, receipts)
	}
}

// TestReload_aRolledBackCutoverClosesEvenIfNobodyPolls is the official pass's
// instrument P3, made a mould: the rollback moulds above are satisfied by the
// POLL on the rollback app closing the act through its live recorder, so an
// observer that ignored rollbacks kept them green. Here nobody polls the status
// door after the POST. The rollback is emitted with no app alive, so the only
// closer is the process's observer, through a transient open of the old ledger;
// the ledger must say FAILED without a single GET /api/reload.
//
// PROBING MUTATION: make the observer return on any non-applied state. The act
// stays AUTHORIZED (the rollback app's recovery spares it — keep) and this
// reddens on the ledger.
func TestReload_aRolledBackCutoverClosesEvenIfNobodyPolls(t *testing.T) {
	_, path, ledger, srv, ollamaURL := storedController(t)
	lockedDir := t.TempDir()
	lock, err := app.AcquireProfileLock(lockedDir)
	if err != nil {
		t.Fatalf("hold the lock: %v", err)
	}
	t.Cleanup(func() { _ = lock.Release() })

	next := minimalCfg(ollamaURL)
	next.Storage = &config.StorageConfig{Path: filepath.Join(lockedDir, "korvun.db")}
	code, body := postConfigStatus(t, srv, next)
	if code != http.StatusAccepted {
		t.Fatalf("POST /api/config = %d %v, want 202", code, body)
	}
	// No poll. The ledger, read back through a read-only open, is the only
	// witness — and the profile on disk must not have moved.
	row := waitLedgerTerminal(t, ledger, body["action_id"], 15*time.Second)
	_, receipts := ledgerRow(t, ledger, body["action_id"])
	if row.State != action.StateFailed || len(receipts) != 1 || receipts[0].Outcome != string(action.StateFailed) {
		t.Fatalf("with nobody polling, the act of a rolled-back cutover is %q with receipts %d, want FAILED with its receipt", row.State, len(receipts))
	}
	// Closed by the OBSERVER, not by the rollback app's recovery: a recovery
	// close wears its marker, and a mould that accepted FAILED from either
	// hand would pass with keep neutralised and the observer deaf at once.
	if row.RecoveryMarker != "" {
		t.Fatalf("the act was closed by the boot's recovery (marker %q), not by the process's observer", row.RecoveryMarker)
	}
	onDisk, err := config.Load(path)
	if err != nil {
		t.Fatalf("reload profile: %v", err)
	}
	if onDisk.Storage == nil || onDisk.Storage.Path != ledger {
		t.Fatalf("the profile on disk names storage %+v after a rolled-back cutover, want the old ledger %q", onDisk.Storage, ledger)
	}
}

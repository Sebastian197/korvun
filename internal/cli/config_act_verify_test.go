// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.16.2 · A4 of the operator-act ruling: the receipt of a profile change made
// through the Control API VERIFIES with `korvun receipt verify`.
//
// This is the only mould that can say so, and it has to live here: the verify
// command is the CLI's, and `internal/app` cannot import `internal/cli` (the
// dependency runs the other way). So it boots a REAL app, presses a REAL door
// over loopback HTTP, and hands the receipt id the door answered to the real
// command.
//
// Evidence level, honest: in-process app with a REAL admin server on a real
// loopback listener, a REAL SQLite store on disk with the profile's own signing
// key and receipt sealer, and the verify command called IN PROCESS through
// `Run`. Not a compiled binary in a separate OS process, and the reloader is a
// stub — so nothing here proves a supervisor cutover, only that the act's
// receipt is one the operator's own tool accepts.

package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/app"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/controlapi"
	"github.com/Sebastian197/korvun/internal/supervisor"
)

// actStubReloader answers a handle whose state is already terminal, so the act
// closes inside the request and its receipt is born there. The cutover itself is
// not what this mould is about.
type actStubReloader struct{ cfg *config.Config }

func (r actStubReloader) RequestReload(*config.Config) (supervisor.Handle, error) {
	return "reload-verify", nil
}
func (r actStubReloader) Status(supervisor.Handle) supervisor.State {
	return supervisor.StateSucceeded
}
func (r actStubReloader) CurrentConfig() *config.Config { return r.cfg }

var _ controlapi.Reloader = actStubReloader{}

// TestConfigAct_theReceiptVerifies presses «Encender aprobaciones» on a running
// Korvun and verifies the receipt the door answered.
//
// PROBING MUTATION (executed): make the recorder return an empty ReceiptID from
// SettleAct. The door answers no receipt and this reddens before it can even
// reach the verify.
func TestConfigAct_theReceiptVerifies(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "korvun.db")
	// The NAME of an env var, never a credential: the value lives in the
	// environment. The identifier avoids the word gosec G101 matches on.
	const adminEnvName = "KORVUN_ACT_VERIFY_ENV"
	t.Setenv(adminEnvName, "tok-act-verify")

	cfg := map[string]any{
		"schema_version": 1,
		"storage":        map[string]any{"path": dbPath},
		"admin":          map[string]any{"token_env": adminEnvName},
		"observability":  map[string]any{"addr": "127.0.0.1:0"},
		"brains": []map[string]any{{
			"name": "default", "sensitivity": "public",
			"policy": map[string]any{"kind": "priority"},
			"models": []map[string]any{{"provider": "ollama", "model_id": "m", "locality": "local"}},
			"agent": map[string]any{
				"tools":        []any{"webhook_call"},
				"webhook_call": map[string]any{"allow_hosts": []any{"hooks.acme.io"}},
			},
		}},
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	cfgPath := filepath.Join(dir, "korvun.json")
	if err := os.WriteFile(cfgPath, raw, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	a, err := app.Build(loaded, app.WithReloader(actStubReloader{cfg: loaded}), app.WithProfilePath(cfgPath))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = a.Run(ctx) }()
	deadline := time.Now().Add(3 * time.Second)
	for a.AdminAddr() == "" && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if a.AdminAddr() == "" {
		t.Fatal("admin server never bound")
	}
	t.Cleanup(func() {
		cancel()
		sctx, sc := context.WithTimeout(context.Background(), 2*time.Second)
		defer sc()
		_ = a.Shutdown(sctx)
	})

	// The door, pressed over real HTTP with the real bearer.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"http://"+a.AdminAddr()+"/api/whats-happening/enable-approvals",
		strings.NewReader(`{"confirm":true}`))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer tok-act-verify")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("press the door: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the door answered %d, want 200", resp.StatusCode)
	}
	var out struct {
		Outcome string `json:"outcome"`
		Act     struct {
			ActionID  string `json:"action_id"`
			ReceiptID string `json:"receipt_id"`
		} `json:"act"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Act.ActionID == "" {
		t.Fatal("the door recorded no operator act")
	}
	if out.Act.ReceiptID == "" {
		t.Fatal("the act carries no receipt, so there is nothing for `receipt verify` to check")
	}

	// The operator's own tool, over the same profile.
	code, stdout, stderr := runIntentCLI(t, "receipt", "verify", "--config", cfgPath, out.Act.ReceiptID)
	if code != 0 {
		t.Fatalf("receipt verify exited %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "OK") {
		t.Fatalf("receipt verify said %q, which does not read as a pass", stdout)
	}
	// And the act it seals is the one the door answered.
	if !strings.Contains(stdout, out.Act.ActionID) && !strings.Contains(stdout, out.Act.ReceiptID) {
		t.Fatalf("the verify output names neither the act nor the receipt:\n%s", stdout)
	}
}

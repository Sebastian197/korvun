// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The operator's act, intents half — Etapa 2, lote 5, pieza 1 (sealed
// decision 3: limited intents arrive via CLI; the Console card comes
// later under the Sixth Law). Every mutation lands with its RECEIPT: an
// identified action row (the operator as principal, loopback_inprocess
// evidence) — the human's act leaves a trace too. Approved-red contract.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
)

// intentTestConfig writes a minimal valid config whose storage points at a
// temp db, returning the config path and the db path.
func intentTestConfig(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "korvun.db")
	cfg := map[string]any{
		"schema_version": 1,
		"storage":        map[string]any{"path": dbPath},
		"brains": []map[string]any{{
			"name": "a", "sensitivity": "public",
			"policy": map[string]any{"kind": "priority"},
			"models": []map[string]any{{"provider": "ollama", "model_id": "m", "locality": "local"}},
			"agent":  map[string]any{"tools": []any{"calc"}},
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
	return cfgPath, dbPath
}

func runIntentCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// receiptOf loads the single CLI receipt recorded for one verb.
func receiptOf(t *testing.T, dbPath, namespace, name string) (actionsqlite.Record, action.IdentityEvidence) {
	t.Helper()
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()
	recs, err := store.ListByOperation(context.Background(), namespace, name)
	if err != nil {
		t.Fatalf("list receipts: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("want exactly one %s.%s receipt, got %d", namespace, name, len(recs))
	}
	evidence, err := store.GetEvidence(context.Background(), recs[0].Envelope.ActionID)
	if err != nil {
		t.Fatalf("the receipt must carry evidence: %v", err)
	}
	return recs[0], evidence
}

func TestIntentCreate_persistsDraftWithReceipt(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath := intentTestConfig(t)
	code, stdout, stderr := runIntentCLI(t,
		"intent", "create", "--config", cfgPath,
		"--purpose", "read-only until friday",
		"--operations", "calc,time",
		"--max-actions", "25",
		"--expires", "2026-09-04T18:00:00Z")
	if code != 0 {
		t.Fatalf("create: exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "int_") || !strings.Contains(stdout, "DRAFT") {
		t.Fatalf("create must print the new id and DRAFT, got %q", stdout)
	}
	id := extractID(t, stdout, "int_")
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()
	stored, err := store.GetIntent(context.Background(), id)
	if err != nil {
		t.Fatalf("the intent must be persisted: %v", err)
	}
	if stored.Status != action.LifecycleDraft || stored.Purpose != "read-only until friday" {
		t.Fatalf("stored = %+v", stored)
	}
	if len(stored.AllowedOperations) != 2 || stored.Budgets.MaxActions != 25 {
		t.Fatalf("terms corrupted: %+v", stored)
	}
	if stored.ExpiresAt.IsZero() {
		t.Fatal("the expiry term must persist")
	}
	// The RECEIPT: an identified SUCCEEDED action by the operator with
	// loopback evidence — the human's act leaves a trace.
	rec, evidence := receiptOf(t, dbPath, "intent", "create")
	if rec.State != action.StateSucceeded {
		t.Fatalf("receipt state = %s", rec.State)
	}
	if rec.Identity == nil || rec.Identity.PrincipalID != action.OperatorPrincipal().PrincipalID {
		t.Fatalf("the act is the OPERATOR's: %+v", rec.Identity)
	}
	if evidence.Credential != action.CredentialLoopbackInProcess {
		t.Fatalf("CLI evidence is loopback_inprocess, got %s", evidence.Credential)
	}
}

func extractID(t *testing.T, out, prefix string) string {
	t.Helper()
	idx := strings.Index(out, prefix)
	if idx < 0 {
		t.Fatalf("no %s id in %q", prefix, out)
	}
	rest := out[idx:]
	if end := strings.IndexAny(rest, " \n\t"); end > 0 {
		return rest[:end]
	}
	return rest
}

func TestIntentLifecycle_activateRevokeWithHonestFailures(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath := intentTestConfig(t)
	code, stdout, _ := runIntentCLI(t, "intent", "create", "--config", cfgPath,
		"--purpose", "p", "--operations", "calc")
	if code != 0 {
		t.Fatalf("create: %d", code)
	}
	id := extractID(t, stdout, "int_")
	if code, out, _ := runIntentCLI(t, "intent", "activate", "--config", cfgPath, id); code != 0 || !strings.Contains(out, "ACTIVE") {
		t.Fatalf("activate: %d %q", code, out)
	}
	if code, out, _ := runIntentCLI(t, "intent", "revoke", "--config", cfgPath, id); code != 0 || !strings.Contains(out, "REVOKED") {
		t.Fatalf("revoke: %d %q", code, out)
	}
	// Terminal is terminal: re-activating a revoked intent fails honestly
	// (exit 1, the lifecycle error surfaced) and its receipt closes FAILED.
	code, _, stderr := runIntentCLI(t, "intent", "activate", "--config", cfgPath, id)
	if code != 1 || !strings.Contains(stderr, "lifecycle") {
		t.Fatalf("re-activate must fail honestly: %d %q", code, stderr)
	}
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	recs, err := store.ListByOperation(context.Background(), "intent", "activate")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("both activate attempts leave receipts, got %d", len(recs))
	}
	states := map[action.State]bool{}
	for _, r := range recs {
		states[r.State] = true
	}
	if !states[action.StateSucceeded] || !states[action.StateFailed] {
		t.Fatalf("one SUCCEEDED and one FAILED receipt, got %v", states)
	}
}

func TestIntentListAndShow(t *testing.T) {
	t.Parallel()
	cfgPath, _ := intentTestConfig(t)
	_, stdout, _ := runIntentCLI(t, "intent", "create", "--config", cfgPath,
		"--purpose", "first purpose", "--operations", "calc")
	id := extractID(t, stdout, "int_")
	code, listOut, _ := runIntentCLI(t, "intent", "list", "--config", cfgPath)
	if code != 0 || !strings.Contains(listOut, id) || !strings.Contains(listOut, "DRAFT") {
		t.Fatalf("list: %d %q", code, listOut)
	}
	code, showOut, _ := runIntentCLI(t, "intent", "show", "--config", cfgPath, id)
	if code != 0 || !strings.Contains(showOut, "first purpose") || !strings.Contains(showOut, "calc") {
		t.Fatalf("show: %d %q", code, showOut)
	}
	if !strings.Contains(showOut, "sha256:") {
		t.Fatalf("show surfaces the contract digest, got %q", showOut)
	}
}

func TestIntentUsageErrors(t *testing.T) {
	t.Parallel()
	cfgPath, _ := intentTestConfig(t)
	cases := [][]string{
		{"intent"},
		{"intent", "bogus", "--config", cfgPath},
		{"intent", "create", "--config", cfgPath},                      // no purpose
		{"intent", "create", "--config", cfgPath, "--purpose", "p"},    // no operations
		{"intent", "activate", "--config", cfgPath},                    // no id
		{"intent", "show", "--config", cfgPath},                        // no id
		{"intent", "create", "--purpose", "p", "--operations", "calc"}, // no config
	}
	for _, args := range cases {
		if code, _, _ := runIntentCLI(t, args...); code != 2 {
			t.Fatalf("%v must be a usage error (2), got %d", args, code)
		}
	}
	// An unknown intent id is a runtime failure (1), not usage.
	if code, _, _ := runIntentCLI(t, "intent", "show", "--config", cfgPath, "int_missing"); code != 1 {
		t.Fatal("unknown id must exit 1")
	}
}

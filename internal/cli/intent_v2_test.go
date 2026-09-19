// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
)

func writeIntentV2Fixture(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "intent-v2.json")
	raw := `{"intent_id":"int_cli_v2","schema_version":2,"version":1,"profile_id":"profile_cli","owner_principal_id":"principal_operator","purpose":"read reports","operations":[{"namespace":"tool","name":"read_file","version":1}],"allowed_resources":[{"kind":"cage","id":"reports"}],"denied_resources":[],"data_scope":["internal"],"output_destinations":["console"],"effect_classes":["read_external"],"budget":{"total":null,"per_operation":{}},"valid_from":"2026-09-19T12:00:00Z","expires_at":"2099-09-20T12:00:00Z","approval":{"required":false},"max_delegation_depth":0}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestIntentV2CLI_CreateActivateVerifyBind(t *testing.T) {
	cfg, dbPath := intentTestConfig(t)
	file := writeIntentV2Fixture(t, t.TempDir())
	commands := [][]string{
		{"intent", "create-v2", "--config", cfg, "--file", file},
		{"intent", "activate-v2", "--config", cfg, "int_cli_v2", "1"},
		{"intent", "verify-v2", "--config", cfg, "int_cli_v2", "1"},
		{"intent", "bind", "--config", cfg, "--actor", "principal_brain", "--channel", "console", "int_cli_v2", "1"},
	}
	for _, args := range commands {
		code, _, stderr := runIntentCLI(t, args...)
		if code != 0 {
			t.Fatalf("%s: code=%d stderr=%q", strings.Join(args[:2], " "), code, stderr)
		}
	}
	store, err := actionsqlite.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	resolved, err := store.ResolveExecutionBinding(context.Background(), "principal_brain", "console", "", time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC))
	if err != nil || resolved.Contract.IntentID != "int_cli_v2" {
		t.Fatalf("binding resolution = %q, %v", resolved.Contract.IntentID, err)
	}
	// SIX, not three: each of the three CLI verbs now leaves TWO receipts —
	// the intent mutation's sealed evidence, and the operator's own act, the
	// identified record this file's godoc promises for every mutation. The v2
	// verbs used to skip the second, so the phase about identity recorded no
	// principal for the human who acted (the twenty-second pass, P2-10).
	receipts, err := store.ListReceipts(context.Background(), "main")
	if err != nil || len(receipts) != 6 {
		t.Fatalf("receipts = %d, want 6 (three intent mutations and three operator acts), %v", len(receipts), err)
	}
}

func TestIntentV2CLI_ExplicitRootAdoption(t *testing.T) {
	cfg, dbPath := intentTestConfig(t)
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateIntent(context.Background(), action.RootIntent()); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	code, _, stderr := runIntentCLI(t, "intent", "adopt-root", "--config", cfg, "--profile", "profile_cli")
	if code != 0 {
		t.Fatalf("adopt-root code=%d stderr=%q", code, stderr)
	}
	reader, err := actionsqlite.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	got, err := reader.GetIntentAnyVersion(context.Background(), action.RootIntentID)
	if err != nil || got.Provenance != action.IntentProvenanceSignedV2 {
		t.Fatalf("root adoption = %+v, %v", got, err)
	}
}

// TestIntentV2CLI_LedgerIsCleanAfterIntentActs: a store whose only history is
// intent acts must verify CLEAN. The intent receipts used to name an action id
// in the ACTION namespace with no action row behind it, so the verifier
// explained the absence as retention's cascade — a prune that never happened —
// and `ledger check` reported every intent act as a degraded check on a
// newborn store (the twenty-second pass, P2-9).
func TestIntentV2CLI_LedgerIsCleanAfterIntentActs(t *testing.T) {
	cfg, dbPath := intentTestConfig(t)
	file := writeIntentV2Fixture(t, t.TempDir())
	for _, args := range [][]string{
		{"intent", "create-v2", "--config", cfg, "--file", file},
		{"intent", "activate-v2", "--config", cfg, "int_cli_v2", "1"},
		{"intent", "revoke-v2", "--config", cfg, "int_cli_v2"},
	} {
		if code, _, stderr := runIntentCLI(t, args...); code != 0 {
			t.Fatalf("%s: code=%d stderr=%q", strings.Join(args[:2], " "), code, stderr)
		}
	}
	code, stdout, stderr := runIntentCLI(t, "ledger", "check", "--config", cfg)
	if code != 0 {
		t.Fatalf("ledger check: code=%d stderr=%q", code, stderr)
	}
	if strings.Contains(stdout, "degraded") || strings.Contains(stdout, "action_row_absent") {
		t.Fatalf("a store of intent acts reports a degraded ledger: %q", stdout)
	}
	if !strings.Contains(stdout, "chain intact") {
		t.Fatalf("ledger check = %q, want an intact chain", stdout)
	}
	_ = dbPath
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R2 of the third Codex pass (adjudicated 2026-09-01): rotate-key was
// the surviving member of the killed class — a CLI act still walking
// through the FULL boot door (recovery + prune + migration), able to
// close a live server's in-flight work beside it. The cure sweeps the
// CLASS: every operator act goes through OpenOperator, the auditor's
// reproduction rides as a permanent test, and a CLASS GUARD fails the
// suite if any CLI command ever calls the full Open again.
// Reproduction-first contract.

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

func TestRotateKey_besideALiveServerTouchesNothing(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, _ := parkedRequest(t)
	// The live server's in-flight work: an AUTHORIZED action mid-run.
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	env := action.NewEnvelope("act_r2_live", "env-r2",
		action.Source{Kind: "agent_brain", Protocol: "text", Channel: "console"},
		action.Operation{Namespace: "tool", Name: "calc", Version: 1},
		`1+1`, time.Now().UTC())
	if err := store.RecordAttempt(context.Background(), env,
		actionsqlite.Decision{Outcome: "allow", Rule: "allow"}, action.StateAuthorized); err != nil {
		t.Fatalf("record: %v", err)
	}
	// The server stays OPEN while the operator rotates the key.
	defer func() { _ = store.Close() }()
	if code, _, stderr := runIntentCLI(t, "receipt", "rotate-key", "--config", cfgPath); code != 0 {
		t.Fatalf("rotate-key: %d %q", code, stderr)
	}
	rec, err := store.Get(context.Background(), "act_r2_live")
	if err != nil || rec.State != action.StateAuthorized || rec.RecoveryMarker != "" {
		t.Fatalf("AUDIT R2: rotate-key must never close the live server's work: %v %v %q",
			err, rec.State, rec.RecoveryMarker)
	}
}

// The class guard: no CLI source file may call the full boot door.
// OpenOperator and OpenReadOnly are the only doors an operator command
// may walk; the full Open (recovery+prune+migration) belongs to the
// server boot alone. Kill the class, not the bug.
func TestClassGuard_noCLICommandUsesTheFullBootDoor(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.Contains(string(src), "actionsqlite.Open(") {
			t.Fatalf("CLASS GUARD (R2): %s calls the full boot door actionsqlite.Open — operator commands walk OpenOperator or OpenReadOnly only", name)
		}
	}
}

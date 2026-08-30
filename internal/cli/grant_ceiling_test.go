// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The operator's ceiling — Etapa 3, lote 3, pieza 3 (spec FR-CEIL-1 CLI):
// --effect-ceiling on issue and delegate, validated against the finite
// ladder; delegation inherits the parent's ceiling unless narrowed (the
// expiry/budget mold); the wall names effect_ceiling on the terminal
// too, and a widening child never touches the disk. Approved-red
// contract.

package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
)

// issueCeilinged issues a parent grant with a read_external ceiling.
func issueCeilinged(t *testing.T, cfgPath, intentID string) string {
	t.Helper()
	code, stdout, stderr := runIntentCLI(t, "grant", "issue", "--config", cfgPath,
		"--intent", intentID, "--subject", "principal_brain_a",
		"--operations", "calc,time", "--max-actions", "50", "--depth", "2",
		"--effect-ceiling", "read_external")
	if code != 0 {
		t.Fatalf("issue ceilinged: %d %q", code, stderr)
	}
	return extractID(t, stdout, "grant_")
}

func TestGrantIssue_ceilingPersistsOnTheStoredGrant(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath := intentTestConfig(t)
	intentID := activeIntentID(t, cfgPath)
	grantID := issueCeilinged(t, cfgPath, intentID)
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	grant, err := store.GetGrant(context.Background(), grantID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if grant.EffectCeiling != action.EffectReadExternal {
		t.Fatalf("the ceiling must persist, got %q", grant.EffectCeiling)
	}
}

func TestGrantIssue_invalidCeilingIsAUsageError(t *testing.T) {
	t.Parallel()
	cfgPath, _ := intentTestConfig(t)
	intentID := activeIntentID(t, cfgPath)
	code, _, stderr := runIntentCLI(t, "grant", "issue", "--config", cfgPath,
		"--intent", intentID, "--subject", "s", "--operations", "calc",
		"--effect-ceiling", "super_safe")
	if code != 2 {
		t.Fatalf("an off-ladder ceiling is a usage error: %d %q", code, stderr)
	}
	if !strings.Contains(stderr, "pure") || !strings.Contains(stderr, "critical") {
		t.Fatalf("the error must name the ladder, got %q", stderr)
	}
}

func TestGrantDelegate_inheritsTheParentCeiling(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath := intentTestConfig(t)
	intentID := activeIntentID(t, cfgPath)
	parentID := issueCeilinged(t, cfgPath, intentID)
	// No --effect-ceiling: the child inherits the parent's (otherwise an
	// absent child ceiling under a limited parent would widen and the UX
	// would fight the operator — the expiry/budget inheritance mold).
	code, stdout, stderr := runIntentCLI(t, "grant", "delegate", "--config", cfgPath,
		"--parent", parentID, "--subject", "principal_ch_telegram",
		"--operations", "calc", "--max-actions", "5")
	if code != 0 {
		t.Fatalf("inheriting delegate must pass: %d %q", code, stderr)
	}
	childID := extractID(t, stdout, "grant_")
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	child, err := store.GetGrant(context.Background(), childID)
	if err != nil {
		t.Fatalf("get child: %v", err)
	}
	if child.EffectCeiling != action.EffectReadExternal {
		t.Fatalf("the child must inherit the parent's ceiling, got %q", child.EffectCeiling)
	}
}

func TestGrantDelegate_ceilingWallNamesTheDimensionOnTheTerminal(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath := intentTestConfig(t)
	intentID := activeIntentID(t, cfgPath)
	parentID := issueCeilinged(t, cfgPath, intentID)
	code, _, stderr := runIntentCLI(t, "grant", "delegate", "--config", cfgPath,
		"--parent", parentID, "--subject", "principal_ch_telegram",
		"--operations", "calc", "--max-actions", "5",
		"--effect-ceiling", "critical")
	if code != 1 || !strings.Contains(stderr, "effect_ceiling") {
		t.Fatalf("the wall must name the tenth dimension on the terminal: %d %q", code, stderr)
	}
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	recs, err := store.ListByOperation(context.Background(), "grant", "delegate")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(recs) != 1 || recs[0].Decision.Rule != "attenuation_violated" {
		t.Fatalf("the refusal leaves its receipt, got %+v", recs)
	}
}

func TestGrantDelegate_narrowedCeilingPersists(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath := intentTestConfig(t)
	intentID := activeIntentID(t, cfgPath)
	parentID := issueCeilinged(t, cfgPath, intentID)
	code, stdout, stderr := runIntentCLI(t, "grant", "delegate", "--config", cfgPath,
		"--parent", parentID, "--subject", "principal_ch_telegram",
		"--operations", "calc", "--max-actions", "5",
		"--effect-ceiling", "pure")
	if code != 0 {
		t.Fatalf("a narrowed ceiling must pass: %d %q", code, stderr)
	}
	childID := extractID(t, stdout, "grant_")
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	child, err := store.GetGrant(context.Background(), childID)
	if err != nil || child.EffectCeiling != action.EffectPure {
		t.Fatalf("narrowed ceiling round-trip: %v %q", err, child.EffectCeiling)
	}
}

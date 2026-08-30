// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The operator's COMPLETE act, end to end — Etapa 2, lote 5, pieza 3:
// create → activate → issue → delegate (attenuated) → revoke, with the
// honest failures along the way (a widening delegation denied naming
// its dimension; a revoked grant that no longer delegates; expired
// authority failing closed) — and every step leaving its identified
// receipt. This is the master plan's stage act, executable.
// Approved-red contract.

package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
)

func TestOperatorAct_endToEnd(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath := intentTestConfig(t)

	// 1. CREATE: a limited intent — "read-only until the window closes".
	code, stdout, stderr := runIntentCLI(t, "intent", "create", "--config", cfgPath,
		"--purpose", "read-only reporting",
		"--operations", "calc,time,read_file",
		"--max-actions", "100",
		"--expires", "2026-09-30T00:00:00Z")
	if code != 0 {
		t.Fatalf("create: %d %q", code, stderr)
	}
	intentID := extractID(t, stdout, "int_")

	// 2. ACTIVATE: the contract enters force.
	if code, _, stderr := runIntentCLI(t, "intent", "activate", "--config", cfgPath, intentID); code != 0 {
		t.Fatalf("activate: %d %q", code, stderr)
	}

	// 3. ISSUE: authority for a brain, bounded under the intent.
	code, stdout, stderr = runIntentCLI(t, "grant", "issue", "--config", cfgPath,
		"--intent", intentID, "--subject", "principal_brain_asistente",
		"--operations", "calc,time", "--max-actions", "40",
		"--expires", "2026-09-15T00:00:00Z", "--depth", "1")
	if code != 0 {
		t.Fatalf("issue: %d %q", code, stderr)
	}
	parentID := extractID(t, stdout, "grant_")

	// 4a. DELEGATE, widening: one extra operation the parent never had.
	// The wall denies NAMING the dimension; nothing lands.
	code, _, stderr = runIntentCLI(t, "grant", "delegate", "--config", cfgPath,
		"--parent", parentID, "--subject", "principal_ch_hooks",
		"--operations", "calc,read_file")
	if code != 1 || !strings.Contains(stderr, "operations") {
		t.Fatalf("widening delegation must be denied naming operations: %d %q", code, stderr)
	}

	// 4b. DELEGATE, attenuated: a strict subset passes and persists.
	code, stdout, stderr = runIntentCLI(t, "grant", "delegate", "--config", cfgPath,
		"--parent", parentID, "--subject", "principal_ch_hooks",
		"--operations", "calc", "--max-actions", "5")
	if code != 0 {
		t.Fatalf("attenuated delegation: %d %q", code, stderr)
	}
	childID := extractID(t, stdout, "grant_")

	// 5. REVOKE the child; a revoked grant delegates NOTHING anymore.
	if code, _, stderr := runIntentCLI(t, "grant", "revoke", "--config", cfgPath, childID); code != 0 {
		t.Fatalf("revoke child: %d %q", code, stderr)
	}
	code, _, stderr = runIntentCLI(t, "grant", "delegate", "--config", cfgPath,
		"--parent", childID, "--subject", "principal_ch_other",
		"--operations", "calc", "--max-actions", "1")
	if code != 1 || !strings.Contains(stderr, action.RuleAuthorityRevoked) {
		t.Fatalf("a revoked grant must not delegate: %d %q", code, stderr)
	}

	// 6. REVOKE the intent itself: the act closes; issuing under it dies
	// with the sealed rule.
	if code, _, stderr := runIntentCLI(t, "intent", "revoke", "--config", cfgPath, intentID); code != 0 {
		t.Fatalf("revoke intent: %d %q", code, stderr)
	}
	code, _, stderr = runIntentCLI(t, "grant", "issue", "--config", cfgPath,
		"--intent", intentID, "--subject", "principal_brain_asistente", "--operations", "calc")
	if code != 1 || !strings.Contains(stderr, action.RuleIntentInactive) {
		t.Fatalf("issuing under a revoked intent must fail closed: %d %q", code, stderr)
	}

	// THE TRAIL: every step above — including every refusal — left its
	// identified receipt under the operator with loopback evidence.
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	wantReceipts := map[[2]string]int{
		{"intent", "create"}:   1,
		{"intent", "activate"}: 1,
		{"intent", "revoke"}:   1,
		{"grant", "issue"}:     2, // one SUCCEEDED, one DENIED (revoked intent)
		{"grant", "delegate"}:  3, // widening DENIED, attenuated SUCCEEDED, revoked-parent DENIED
		{"grant", "revoke"}:    1,
	}
	for op, want := range wantReceipts {
		recs, err := store.ListByOperation(ctx, op[0], op[1])
		if err != nil {
			t.Fatalf("list %v: %v", op, err)
		}
		if len(recs) != want {
			t.Fatalf("%v receipts = %d, want %d", op, len(recs), want)
		}
		for _, rec := range recs {
			if rec.Identity == nil || rec.Identity.PrincipalID != action.OperatorPrincipal().PrincipalID {
				t.Fatalf("%v receipt without the operator: %+v", op, rec.Identity)
			}
			evidence, err := store.GetEvidence(ctx, rec.Envelope.ActionID)
			if err != nil {
				t.Fatalf("%v receipt without evidence: %v", op, err)
			}
			if evidence.Credential != action.CredentialLoopbackInProcess {
				t.Fatalf("%v evidence credential = %s", op, evidence.Credential)
			}
		}
	}
}

func TestHelp_namesTheOperatorActCommands(t *testing.T) {
	t.Parallel()
	code, stdout, _ := runIntentCLI(t, "help")
	if code != 0 {
		t.Fatalf("help: %d", code)
	}
	for _, want := range []string{"intent", "grant"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("help must name the %s commands, got %q", want, stdout)
		}
	}
}

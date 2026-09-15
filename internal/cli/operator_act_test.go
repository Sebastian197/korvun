// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The operator's COMPLETE act, end to end — Etapa 2, lote 5, pieza 3:
// create → activate → issue → delegate (attenuated) → revoke, with the
// honest failures along the way (a widening delegation denied naming
// its dimension; a revoked grant that no longer delegates; expired
// authority failing closed) — and every step leaving its identified
// receipt. This is the master plan's stage act, executable.
// Approved-red contract.
//
// ELEVATED 2026-09-15 — the date bomb. Every window here was a fixed
// calendar date, and on 2026-09-15 the parent grant's `--expires` went
// past: the widening delegation of step 4a was then refused as
// authority_expired, a refusal nobody attacked, and the act went red on
// every tree. The windows are now relative to the wall clock the CLI
// itself reads, and the «expired authority failing closed» the paragraph
// above promised — which no step exercised — is its own named attack.
//
// Evidence level, honest: in-process CLI over a real SQLite file. Not a
// compiled binary.

package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
)

func TestOperatorAct_endToEnd(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath := intentTestConfig(t)
	// The windows, relative to the wall clock the CLI reads. The intent opens
	// two days back so a grant whose window already closed still fits inside it.
	now := time.Now().UTC()
	stamp := func(d time.Duration) string { return now.Add(d).Format(time.RFC3339) }

	// 1. CREATE: a limited intent — "read-only until the window closes".
	code, stdout, stderr := runIntentCLI(t, "intent", "create", "--config", cfgPath,
		"--purpose", "read-only reporting",
		"--operations", "calc,time,read_file",
		"--max-actions", "100",
		"--valid-from", stamp(-48*time.Hour),
		"--expires", stamp(30*24*time.Hour))
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
		"--expires", stamp(7*24*time.Hour), "--depth", "1")
	if code != 0 {
		t.Fatalf("issue: %d %q", code, stderr)
	}
	parentID := extractID(t, stdout, "grant_")

	// 4a. DELEGATE, widening: one extra operation the parent never had.
	// The wall denies NAMING the dimension; nothing lands. An expired parent
	// also refuses with exit 1, so the refusal must be the widening one and
	// not the clock's — that disguise is exactly how the date bomb failed.
	code, _, stderr = runIntentCLI(t, "grant", "delegate", "--config", cfgPath,
		"--parent", parentID, "--subject", "principal_ch_hooks",
		"--operations", "calc,read_file")
	if code != 1 || !strings.Contains(stderr, "operations") {
		t.Fatalf("widening delegation must be denied naming operations: %d %q", code, stderr)
	}
	if strings.Contains(stderr, action.RuleAuthorityExpired) {
		t.Fatalf("the widening refusal came from the clock, not from the wall: %q", stderr)
	}

	// 4b. DELEGATE, attenuated: a strict subset passes and persists.
	code, stdout, stderr = runIntentCLI(t, "grant", "delegate", "--config", cfgPath,
		"--parent", parentID, "--subject", "principal_ch_hooks",
		"--operations", "calc", "--max-actions", "5")
	if code != 0 {
		t.Fatalf("attenuated delegation: %d %q", code, stderr)
	}
	childID := extractID(t, stdout, "grant_")

	// 4c. EXPIRED authority fails closed: a grant whose window closed an hour
	// ago delegates nothing, and the refusal names the clock.
	//
	// Probing mutation (executed, red, declared in the canto): drop the window
	// check `!at.Before(g.ExpiresAt)` from action.ValidateGrantAt ⇒ this step
	// reddens.
	code, stdout, stderr = runIntentCLI(t, "grant", "issue", "--config", cfgPath,
		"--intent", intentID, "--subject", "principal_brain_asistente",
		"--operations", "calc", "--max-actions", "1",
		"--valid-from", stamp(-2*time.Hour), "--expires", stamp(-time.Hour), "--depth", "1")
	if code != 0 {
		t.Fatalf("issue the lapsed grant: %d %q", code, stderr)
	}
	lapsedID := extractID(t, stdout, "grant_")
	code, _, stderr = runIntentCLI(t, "grant", "delegate", "--config", cfgPath,
		"--parent", lapsedID, "--subject", "principal_ch_lapsed",
		"--operations", "calc", "--max-actions", "1")
	if code != 1 || !strings.Contains(stderr, action.RuleAuthorityExpired) {
		t.Fatalf("a grant past its window must not delegate, naming %s: %d %q",
			action.RuleAuthorityExpired, code, stderr)
	}

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
		{"grant", "issue"}:     3, // parent SUCCEEDED, lapsed SUCCEEDED, one DENIED (revoked intent)
		{"grant", "delegate"}:  4, // widening DENIED, attenuated SUCCEEDED, lapsed-parent DENIED, revoked-parent DENIED
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

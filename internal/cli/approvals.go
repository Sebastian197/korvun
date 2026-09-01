// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The operator's approvals inbox — Etapa 5, lote 4 (spec FR-CLI,
// sealed NC-1b): `korvun approvals list|show` consult through the
// consolidation's READ-ONLY door (the audit's lesson applied to the
// new surface from birth — a consult never migrates, never recovers,
// never creates files, and its connection refuses every write);
// `approve|reject` are mutating operator acts through the sealed
// store (the rotate-key mold), each leaving its E4 ink. approve fires
// the lote-3 deferred execution — the claim, the digest belt, the one
// Executor path — and reports the REAL outcome. show renders the full
// §15.2 preview, the RAW parameters (the operator's loopback right,
// ADR-0024: they exist only here and in the parked store row) and THE
// DIGEST the human approves, prominently.
package cli

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/app"
	"github.com/Sebastian197/korvun/internal/config"
)

// approvalsCmd dispatches the `approvals` noun's verbs.
func (c *cli) approvalsCmd(args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(c.stderr, "korvun approvals: expected a subcommand: list | show | approve | reject\nRun 'korvun help' for usage.\n")
		return 2
	}
	switch args[0] {
	case "list":
		return c.approvalsList(args[1:])
	case "show":
		return c.approvalsShow(args[1:])
	case "approve":
		return c.approvalsDecide(args[1:], "approve")
	case "reject":
		return c.approvalsDecide(args[1:], "reject")
	default:
		_, _ = fmt.Fprintf(c.stderr, "korvun approvals: unknown subcommand %q\nRun 'korvun help' for usage.\n", args[0])
		return 2
	}
}

// approvalsList implements `korvun approvals list` (read-only door).
func (c *cli) approvalsList(args []string) int {
	fs := flag.NewFlagSet("approvals list", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	configPath := fs.String("config", "", "path to the korvun config (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *configPath == "" {
		_, _ = fmt.Fprint(c.stderr, "korvun approvals list: --config is required\n")
		return 2
	}
	store, err := openOperatorStore(*configPath)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun approvals list: %v\n", err)
		return 1
	}
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	total := 0
	for _, status := range []action.ApprovalStatus{
		action.ApprovalPending, action.ApprovalApproved,
		action.ApprovalRejected, action.ApprovalExpired, action.ApprovalCancelled,
	} {
		approvals, err := store.ListApprovals(ctx, status)
		if err != nil {
			_, _ = fmt.Fprintf(c.stderr, "korvun approvals list: %v\n", err)
			return 1
		}
		for _, a := range approvals {
			expiry := "never expires"
			if !a.ExpiresAt.IsZero() {
				expiry = "expires " + a.ExpiresAt.UTC().Format(time.RFC3339)
			}
			_, _ = fmt.Fprintf(c.stdout, "%-38s %-9s %-22s %s\n",
				a.ApprovalID, a.Status, a.RiskSummary, expiry)
			total++
		}
	}
	if total == 0 {
		_, _ = fmt.Fprintln(c.stdout, "no approval requests recorded")
	}
	return 0
}

// approvalsShow implements `korvun approvals show` (read-only door).
func (c *cli) approvalsShow(args []string) int {
	fs := flag.NewFlagSet("approvals show", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	configPath := fs.String("config", "", "path to the korvun config (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *configPath == "" || fs.NArg() != 1 {
		_, _ = fmt.Fprint(c.stderr, "korvun approvals show: usage: korvun approvals show --config <path> <apr_…>\n")
		return 2
	}
	store, err := openOperatorStore(*configPath)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun approvals show: %v\n", err)
		return 1
	}
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	a, p, err := store.GetApproval(ctx, fs.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun approvals show: %v\n", err)
		return 1
	}
	// THE DIGEST the human approves, first and prominent.
	_, _ = fmt.Fprintf(c.stdout, "APPROVING EXACTLY THIS — digest: %s\n\n", a.ActionDigest)
	_, _ = fmt.Fprintf(c.stdout,
		"request:       %s (%s)\npurpose:       %s\nactor:         %s (grant %s, depth %d)\noperation:     %s\nresources:     %v\ndata egress:   %s\ncost:          %s\neffect class:  %s\nreversibility: %s\ntool cage:     %s\npinned law:    v%d %s\nrequired by:   %s\nexpires:       %s\n",
		a.ApprovalID, a.Status, p.IntentPurpose, p.PrincipalID, p.GrantID, p.GrantDepth,
		p.Operation, p.Resources, p.DataEgress, p.CostLine, p.EffectClass,
		p.Reversibility, p.ToolCage, p.PolicyVersion, p.PolicyDigest, p.RequiredRule,
		a.ExpiresAt.UTC().Format(time.RFC3339))
	// The RAW parameters — the operator's loopback right (ADR-0024:
	// they appear ONLY on this surface and live only in the parked row).
	if params, err := store.ApprovalParams(ctx, a.ApprovalID); err == nil {
		_, _ = fmt.Fprintf(c.stdout, "\nraw parameters (loopback only):\n%s\n", string(params))
	} else {
		_, _ = fmt.Fprintln(c.stdout, "\nraw parameters: no longer held (decided or executed)")
	}
	if a.Decision != "" {
		_, _ = fmt.Fprintf(c.stdout, "\ndecision: %s by %s at %s (%s)\nproof receipt: %s\n",
			a.Decision, a.DecisionPrincipalID, a.DecisionAt.UTC().Format(time.RFC3339),
			a.Comment, a.DecisionReceiptID)
	}
	return 0
}

// approvalsDecide implements approve/reject: mutating operator acts by
// the sealed-store mold; approve fires the deferred execution.
func (c *cli) approvalsDecide(args []string, verb string) int {
	fs := flag.NewFlagSet("approvals "+verb, flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	configPath := fs.String("config", "", "path to the korvun config (required)")
	comment := fs.String("comment", "", "optional decision comment")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *configPath == "" || fs.NArg() != 1 {
		_, _ = fmt.Fprintf(c.stderr, "korvun approvals %s: usage: korvun approvals %s --config <path> <apr_…>\n", verb, verb)
		return 2
	}
	approvalID := fs.Arg(0)
	cfg, err := config.Load(*configPath)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun approvals %s: %v\n", verb, err)
		return 1
	}
	store, err := openOperatorStoreSealed(*configPath)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun approvals %s: %v\n", verb, err)
		return 1
	}
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	decision := "rejected"
	if verb == "approve" {
		decision = "approved"
	}
	env, ident, err := operatorEnvelope("approval", verb, `{"approval_id":"`+approvalID+`"}`)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun approvals %s: %v\n", verb, err)
		return 1
	}
	rule, err := store.DecideApproval(ctx, approvalID, decision, time.Now().UTC(),
		env, actionsqlite.AttemptIdentity{
			PrincipalID: ident.PrincipalID, IntentID: ident.IntentID, Evidence: ident.Evidence,
		}, *comment)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun approvals %s: %v\n", verb, err)
		return 1
	}
	if rule != "" {
		_, _ = fmt.Fprintf(c.stderr, "korvun approvals %s: %s\n", verb, rule)
		return 1
	}
	if verb == "reject" {
		_, _ = fmt.Fprintf(c.stdout, "approval %s rejected — the parked action closed with its receipt\n", approvalID)
		return 0
	}
	// approve: the lote-3 deferred execution of the EXACT object.
	a, p, err := store.GetApproval(ctx, approvalID)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun approvals approve: %v\n", err)
		return 1
	}
	exec, err := app.BuildApprovalExecutor(cfg, p)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun approvals approve: %v\n", err)
		return 1
	}
	result, err := app.ExecuteApprovedAction(ctx, store, exec, approvalID)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun approvals approve: execution: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(c.stdout, "approval %s approved — executed the exact approved object (digest %s)\noutcome: SUCCEEDED\nresult: %s\n",
		approvalID, a.ActionDigest, result)
	return 0
}

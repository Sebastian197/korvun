// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The approvals wiring (Trust Layer Etapa 5, lote 3 — spec FR-GATE-1,
// FR-PRV-1, sealed): when approvals.enabled, the brains' recorder
// adapter carries the RequestApproval extension — the gate's honest no
// becomes a request born WHOLE in the store (the lote-2 birth). The
// adapter owns the §15.2 preview assembly because it holds the store:
// the intent's purpose, the grant's budget line, the tool's declared
// egress and reversibility, and the brain's pinned law. Absent or OFF:
// the plain adapter, and the E3 denial stands byte-for-byte forever.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/action/executor"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/brain"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/policy"
	"github.com/Sebastian197/korvun/internal/tool"
)

// defaultApprovalTTL is the request expiry window when the config does
// not narrow it (spec FR-APR-1).
const defaultApprovalTTL = time.Hour

// approvalTTL resolves the configured TTL (strict: a malformed value
// dies at Build, the boot-fatal posture).
func approvalTTL(cfg *config.ApprovalsConfig) (time.Duration, error) {
	if cfg == nil || cfg.TTL == "" {
		return defaultApprovalTTL, nil
	}
	d, err := time.ParseDuration(cfg.TTL)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("app: approvals.ttl %q is not a positive duration", cfg.TTL)
	}
	return d, nil
}

// approvalRecorder is the extended adapter: the plain recorder plus
// the RequestApproval extension the brain's gate asks for.
type approvalRecorder struct {
	actionRecorder
	ttl time.Duration
}

// RequestApproval implements the brain's optional extension: it
// assembles the §15.2 preview and asks the store for the born-whole
// birth of the parked request. Returns the request id for the model's
// honest pending observation.
func (r approvalRecorder) RequestApproval(ctx context.Context, env action.Envelope, rule string, rawParams string) (string, error) {
	now := time.Now().UTC()
	preview := r.assemblePreview(ctx, env, rule)
	approval := action.Approval{
		ApprovalID:    action.NewApprovalID(),
		SchemaVersion: 1,
		ActionID:      env.ActionID,
		ActionDigest:  env.ParametersDigest,
		PreviewDigest: preview.Digest(),
		RequestedFrom: action.OperatorPrincipal().PrincipalID,
		Reason:        rule,
		RiskSummary:   preview.Reversibility,
		PolicyVersion: r.pin.Version,
		PolicyDigest:  r.pin.Digest,
		RequestedAt:   now,
		ExpiresAt:     now.Add(r.ttl),
		Status:        action.ApprovalPending,
	}
	if err := r.store.CreateApprovalRequest(ctx, env,
		actionsqlite.Decision{Outcome: rule, Rule: rule,
			PolicyVersion: r.pin.Version, PolicyDigest: r.pin.Digest},
		approval, preview, rawParams); err != nil {
		return "", err
	}
	return approval.ApprovalID, nil
}

// assemblePreview builds the §15.2 agent diff from what the adapter
// can honestly resolve: absent facts are stated as absent, never
// invented.
func (r approvalRecorder) assemblePreview(ctx context.Context, env action.Envelope, rule string) action.ActionPreview {
	purpose := "-"
	if env.IntentID != "" {
		if intent, err := r.store.GetIntent(ctx, env.IntentID); err == nil {
			purpose = intent.Purpose
		}
	}
	grantID, cost := "-", "unbudgeted"
	if len(env.AuthorityRefs) > 0 {
		grantID = env.AuthorityRefs[0]
		if grant, err := r.store.GetGrant(ctx, grantID); err == nil {
			if grant.Budgets.MaxActions > 0 {
				cost = fmt.Sprintf("budget %d actions under grant %s", grant.Budgets.MaxActions, grantID)
			}
		}
	}
	class := action.EffectClass(env.Effect.Class)
	egress := "no declared data egress"
	reversibility := "unclassified consequence"
	cage := env.Operation.Name
	if descriptor, declared := tool.BuiltinEffects(env.Operation.Name); declared {
		class = descriptor.Class
		if descriptor.DataEgress {
			egress = "request/result content LEAVES the kernel boundary"
		}
		switch {
		case descriptor.Reversible:
			reversibility = string(descriptor.Class) + " — reversible"
		case descriptor.Compensatable:
			reversibility = string(descriptor.Class) + " — compensatable, not reversible"
		default:
			reversibility = string(descriptor.Class) + " — irreversible, no documented undo"
		}
	} else if class != "" {
		reversibility = string(class)
	}
	return action.ActionPreview{
		ActionID:      env.ActionID,
		SchemaVersion: 1,
		IntentPurpose: purpose,
		PrincipalID:   env.Principal.PrincipalID,
		GrantID:       grantID,
		Operation:     env.Operation.Namespace + "/" + env.Operation.Name,
		Resources:     []string{strings.TrimSpace(env.Source.Channel)},
		DataEgress:    egress,
		ArgsDigest:    env.ParametersDigest,
		CostLine:      cost,
		EffectClass:   class,
		Reversibility: reversibility,
		ToolCage:      cage,
		PolicyVersion: r.pin.Version,
		PolicyDigest:  r.pin.Digest,
		RequiredRule:  rule,
	}
}

// newBrainRecorder builds the per-brain recorder adapter the wiring
// hands to brains: extended with the approval extension ONLY when the
// knob is on (the sacred pin lives in this fork).
func newBrainRecorder(store *actionsqlite.Store, pin actionsqlite.PolicyPin, approvals *config.ApprovalsConfig, ttl time.Duration) brain.ActionRecorder {
	base := actionRecorder{store: store, pin: pin}
	if approvals != nil && approvals.Enabled {
		return approvalRecorder{actionRecorder: base, ttl: ttl}
	}
	return base
}

// recorderForTest rebuilds the exact adapter shape wire() hands the
// brains — the same fork newBrainRecorder takes, driven by the built
// app's stored knob. Package-internal test seam.
func (a *App) recorderForTest() brain.ActionRecorder {
	pin := actionsqlite.PolicyPin{Version: 1, Digest: "sha256:test-seam"}
	return newBrainRecorder(a.actions.(*actionsqlite.Store), pin, a.approvalsCfg, a.approvalTTL)
}

// ExecuteApprovedAction runs the EXACT stored envelope of an APPROVED
// request through the one Executor Registry path (spec FR-EXEC, sealed
// NC-2: identity, never equivalence): claim the canonical params
// atomically (exactly one executor wins — racing calls cannot fire the
// effect twice), re-verify them against the approved digest as the
// belt, execute, and close the parked action with its era's E4 receipt
// and the on-the-fly result digest. A request that is not APPROVED —
// pending, rejected, cancelled or expired — never executes.
func ExecuteApprovedAction(ctx context.Context, store *actionsqlite.Store, exec *executor.Executor, approvalID string, law actionsqlite.PolicyPin) (string, error) {
	approval, _, err := store.GetApproval(ctx, approvalID)
	if err != nil {
		return "", err
	}
	if approval.Status != action.ApprovalApproved {
		return "", fmt.Errorf("app: approval %s is %s — only APPROVED requests execute", approvalID, approval.Status)
	}
	// C1: execution consumes authority too — the law is re-checked at
	// THIS touch, not only at decide (a config change between the two
	// must refuse here as well).
	if rule, dim := action.ValidateApprovalBinding(approval, approval.ActionDigest, law.Version, law.Digest); rule != "" {
		return "", fmt.Errorf("app: %s (%s): approval %s was parked under law v%d %s but the current law is v%d %s — refusing execution", rule, dim, approvalID, approval.PolicyVersion, approval.PolicyDigest, law.Version, law.Digest)
	}
	rec, err := store.Get(ctx, approval.ActionID)
	if err != nil {
		return "", fmt.Errorf("app: approved action %s: %w", approval.ActionID, err)
	}
	if rec.State != action.StateApproved {
		return "", fmt.Errorf("app: action %s is %s — already executed or closed", approval.ActionID, rec.State)
	}
	// The atomic claim: exactly one executor gets the params.
	params, err := store.ClaimApprovalParams(ctx, approvalID)
	if err != nil {
		return "", fmt.Errorf("app: claim execution of %s: %w", approvalID, err)
	}
	// The belt (NC-2): the claimed params must re-derive the EXACT
	// digest the human approved — identity, never equivalence.
	if got := action.Digest(rec.Envelope.Operation, string(params)); got != approval.ActionDigest {
		return "", fmt.Errorf("app: refusing execution of %s: stored params re-derive digest %s but the human approved %s", approvalID, got, approval.ActionDigest)
	}
	conv := ""
	result, _, execErr := exec.Run(ctx, rec.Envelope.Operation.Name,
		tool.Scope{Brain: "", Conversation: conv}, string(params))
	outcome := action.StateSucceeded
	resultDigest := action.HashCanonical(result)
	if execErr != nil {
		outcome = action.StateFailed
		resultDigest = ""
	}
	if err := store.FinishWithResult(ctx, approval.ActionID, outcome, time.Now().UTC(), resultDigest); err != nil {
		return "", fmt.Errorf("app: close executed action %s: %w", approval.ActionID, err)
	}
	if execErr != nil {
		return "", fmt.Errorf("app: approved execution of %s failed: %w", approvalID, execErr)
	}
	return result, nil
}

// BuildApprovalExecutor builds the executor for ONE approved action's
// deferred run (Etapa 5 FR-CLI): the acting brain is recovered from
// the preview's principal ("principal_brain_<name>" — the E2 identity
// mold), its config block found, and THE tool rebuilt with its real
// cage through the same constructor the boot uses. memory_note (the
// one tool needing the live app's note store) fails loud here — and
// honestly cannot appear: write_reversible never parks for approval.
func BuildApprovalExecutor(cfg *config.Config, preview action.ActionPreview) (*executor.Executor, error) {
	const brainPrefix = "principal_brain_"
	if !strings.HasPrefix(preview.PrincipalID, brainPrefix) {
		return nil, fmt.Errorf("app: approval executor: principal %q is not a brain principal", preview.PrincipalID)
	}
	brainName := strings.TrimPrefix(preview.PrincipalID, brainPrefix)
	toolName := preview.Operation
	if i := strings.LastIndex(toolName, "/"); i >= 0 {
		toolName = toolName[i+1:]
	}
	for _, bc := range cfg.Brains {
		if bc.Name != brainName {
			continue
		}
		// C1 depth check: rebuild ONLY from the CURRENT grant list — a
		// tool revoked from agent.tools after the park never executes,
		// even if every other gate were blind to the change.
		granted := false
		if bc.Agent != nil {
			for _, name := range bc.Agent.Tools {
				if name == toolName {
					granted = true
					break
				}
			}
		}
		if !granted {
			return nil, fmt.Errorf("app: approval executor: tool %q is not in brain %q's CURRENT grant list — a revoked tool never executes", toolName, brainName)
		}
		attrs := map[string]policy.ToolAttrs{}
		if a, ok := tool.BuiltinAttrs(toolName); ok {
			attrs[toolName] = policy.ToolAttrs{Sensitive: a.Sensitive, Network: a.Network}
		}
		sens, err := parseSensitivity(bc.Sensitivity)
		if err != nil {
			return nil, fmt.Errorf("app: approval executor: %w", err)
		}
		if _, pure := tool.Builtin(toolName); !pure && bc.Agent == nil {
			return nil, fmt.Errorf("app: approval executor: brain %q has no agent block — caged tool %q cannot be rebuilt", brainName, toolName)
		}
		b := &builder{logger: slog.New(slog.DiscardHandler)}
		t, err := b.agentTool(bc, toolName, attrs, sens)
		if err != nil {
			return nil, fmt.Errorf("app: approval executor for %s/%s: %w", brainName, toolName, err)
		}
		return executor.New(tool.Registry{toolName: t}, 0, time.Now), nil
	}
	return nil, fmt.Errorf("app: approval executor: brain %q not in the current config", brainName)
}

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
	// R4-F2: the adapter RESOLVES context facts (store lookups the
	// domain cannot do) and the action-package factory DERIVES the
	// whole story — narrated previews died with the bundle door.
	b, err := action.NewBoundApprovalRequest(env, rawParams, r.resolveApprovalContext(ctx, env, rule))
	if err != nil {
		return "", err
	}
	if err := r.store.CreateApprovalRequest(ctx, b); err != nil {
		return "", err
	}
	return b.Approval().ApprovalID, nil
}

// resolveApprovalContext gathers the facts the factory cannot derive
// itself: the intent's purpose, the grant identity and its budget line
// (absent facts are stated as absent, never invented), the declared
// effect descriptor, the pinned law and the injected clock.
func (r approvalRecorder) resolveApprovalContext(ctx context.Context, env action.Envelope, rule string) action.ApprovalContext {
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
	descriptor, declared := tool.BuiltinEffects(env.Operation.Name)
	return action.ApprovalContext{
		IntentPurpose: purpose,
		GrantID:       grantID,
		CostLine:      cost,
		ToolCage:      env.Operation.Name,
		Descriptor:    descriptor,
		HasDescriptor: declared,
		LawVersion:    r.pin.Version,
		LawDigest:     r.pin.Digest,
		Rule:          rule,
		Now:           time.Now().UTC(),
		TTL:           r.ttl,
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
// atomically (exactly one caller obtains them, so at most one executor
// START ever happens; C7 honesty: what a crashed start did to the
// external world is OUTCOME_UNKNOWN, not a claim this function makes),
// re-verify them against the approved digest as the
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
	// The atomic claim: exactly one caller gets the params — and the
	// law is judged inside the claiming transaction (R4-F2), over the
	// re-read row.
	params, err := store.ClaimApprovalParams(ctx, approvalID, &law)
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

// ResolveApprovalLaw resolves ONE brain's effective cage and its law
// pin in a SINGLE resolution (R6-X3): the operator CLI feeds BOTH the
// decision (the pin) and the deferred executor (the cage) from this
// one object.
func ResolveApprovalLaw(cfg *config.Config, brainName string) (*EffectiveCage, actionsqlite.PolicyPin, error) {
	for _, bc := range cfg.Brains {
		if bc.Name != brainName {
			continue
		}
		cage, err := ResolveEffectiveCage(bc)
		if err != nil {
			return nil, actionsqlite.PolicyPin{}, fmt.Errorf("app: %w", err)
		}
		pin, err := policyPinFromCage(cage)
		if err != nil {
			return nil, actionsqlite.PolicyPin{}, fmt.Errorf("app: brain %q: %w", brainName, err)
		}
		return cage, pin, nil
	}
	return nil, actionsqlite.PolicyPin{}, fmt.Errorf("app: policy pin: brain %q is not in the current config", brainName)
}

// BuildApprovalExecutorFromCage rebuilds the approved tool from an
// ALREADY-resolved cage (R6-X3: no second resolution on the operator
// path). The C1 depth check and the agent-block guard ride here.
func BuildApprovalExecutorFromCage(cage *EffectiveCage, preview action.ActionPreview) (*executor.Executor, error) {
	toolName := preview.Operation
	if i := strings.LastIndex(toolName, "/"); i >= 0 {
		toolName = toolName[i+1:]
	}
	granted := false
	for _, name := range cage.Tools {
		if name == toolName {
			granted = true
			break
		}
	}
	if !granted {
		return nil, fmt.Errorf("app: approval executor: tool %q is not in brain %q's CURRENT grant list — a revoked tool never executes", toolName, cage.BrainName)
	}
	if _, pure := tool.Builtin(toolName); !pure && !cage.HasAgent {
		return nil, fmt.Errorf("app: approval executor: brain %q has no agent block — caged tool %q cannot be rebuilt", cage.BrainName, toolName)
	}
	b := &builder{logger: slog.New(slog.DiscardHandler)}
	t, err := b.agentTool(cage, toolName)
	if err != nil {
		return nil, fmt.Errorf("app: approval executor for %s/%s: %w", cage.BrainName, toolName, err)
	}
	return executor.New(tool.Registry{toolName: t}, 0, time.Now), nil
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
	cage, _, err := ResolveApprovalLaw(cfg, brainName)
	if err != nil {
		return nil, fmt.Errorf("app: approval executor: %w", err)
	}
	return BuildApprovalExecutorFromCage(cage, preview)
}

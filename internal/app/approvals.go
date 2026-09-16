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
	"errors"
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
// approvedDigest is the digest the caller is standing behind, and it travels
// all the way into the claiming transaction to be compared THERE, against the
// row that transaction read. Comparing a re-read of the stored column against
// itself — which is what passing approval.ActionDigest did — proves the row
// agrees with itself and says nothing about what anybody approved.
//
// Its PROVENANCE differs by door, and this godoc is not entitled to flatten
// that: from the window it is the string the operator typed into the arming
// gate; from `korvun approvals approve|execute` there is no such string,
// because that command takes no digest flag, so the CLI stands behind the
// column it just read. The CLI's gap is FILED for v0.15.1 with its
// reproduction; what is NOT filed is a sentence claiming otherwise.
//
// An EMPTY digest is refused by the store's belt rather than silently replaced
// by the stored column. The replacement was here, and it made the degraded
// self-comparison the quiet default for every caller that passed nothing —
// class (a) of the known-classes checklist, on the one path that fires an
// irreversible effect.
func ExecuteApprovedAction(ctx context.Context, store *actionsqlite.Store, exec *executor.Executor, approvalID string, law actionsqlite.PolicyPin, approvedDigest string) (ApprovedExecution, error) {
	approval, _, err := store.GetApproval(ctx, approvalID)
	if err != nil {
		return ApprovedExecution{}, err
	}
	if approval.Status != action.ApprovalApproved {
		if approval.Status == action.ApprovalPending {
			return ApprovedExecution{}, fmt.Errorf("app: approval %s is %s — only APPROVED requests execute: %w", approvalID, approval.Status, ErrApprovalNotDecided)
		}
		return ApprovedExecution{}, fmt.Errorf("app: approval %s is %s: %w", approvalID, approval.Status, ErrApprovalAlreadyClosed)
	}
	rec, err := store.Get(ctx, approval.ActionID)
	if err != nil {
		// The decide has COMMITTED by the time this runs, and the actions row
		// is what the recovery pass would close. A failure here is evidence
		// that no longer reads, not «the execution simply did not start»: left
		// unnamed it was published as «the row still holds its parameters»,
		// which is a benign sentence over a permanently broken ledger.
		return ApprovedExecution{}, fmt.Errorf("app: approved action %s: %w: %w", approval.ActionID, ErrApprovalRecordUnreadable, err)
	}
	if rec.State != action.StateApproved {
		return ApprovedExecution{}, fmt.Errorf("app: action %s is %s: %w", approval.ActionID, rec.State, ErrApprovalAlreadyClosed)
	}
	// The atomic claim, and with it the WHOLE judgement: the law, the belts and
	// the digest all run inside the claiming transaction, over the row that
	// transaction read.
	//
	// The old shape read the operation triple here, claimed (committing the
	// purge), and only THEN compared the digest against that earlier read. An
	// external UPDATE of op_version in that window passed every belt and fired
	// an irreversible effect under an operation the row no longer declared. So
	// the claim hands back the triple it judged, and that is the one that runs.
	// The snapshot travels too: everything the prechecks above judged was read
	// OUTSIDE the claiming transaction, so the claim compares that whole row
	// against the row it reads itself and refuses if any column moved.
	params, op, err := store.ClaimApprovalParamsUnderDigest(ctx, approvalID, &law, approvedDigest, &approval)
	if err != nil {
		return ApprovedExecution{}, fmt.Errorf("app: claim execution of %s: %w", approvalID, err)
	}
	toolName := op.Name
	conv := ""
	result, _, execErr := exec.Run(ctx, toolName,
		tool.Scope{Brain: "", Conversation: conv}, string(params))
	// ONE function decides what this closes on, and the brain path calls the
	// same one. The rule itself is unchanged: a delivered request whose answer
	// was lost, and a context that ended, are uncertainty — closing them FAILED
	// would put in the ledger a definite claim nobody can support, what the
	// store's own C5 comment calls «a FAILED lie». Anything else is a tool that
	// refused before its effect left, and FAILED is honest for that.
	outcome := tool.CloseStateAfterRun(execErr)
	resultDigest := action.HashCanonical(result)
	if outcome != action.StateSucceeded {
		resultDigest = ""
	}
	// The close does NOT ride the caller's context. Between the record and this
	// line an irreversible effect happened; a context that ended meanwhile —
	// a cancelled request, a shutdown — would take the close down with it,
	// because database/sql refuses a query on a dead context before the driver
	// ever sees it. The effect would have happened and the ledger kept none of
	// What WithoutCancel drops is the cancellation AND the deadline: the close
	// and the read-back below run with no context bound at all, and the only
	// thing that bounds a wait for a lock is the store's own
	// `busy_timeout(5000)` in its DSN. That is the trade this line makes on
	// purpose — a bounded wait for the lock against an effect with no record —
	// and it is written here so nobody reads a surviving deadline into it.
	closeCtx := context.WithoutCancel(ctx)
	if err := store.FinishWithResult(closeCtx, approval.ActionID, outcome, time.Now().UTC(), resultDigest); err != nil {
		// The effect already happened or already failed; what could not be
		// written is the close. That is a KNOWN effect with an unwritten
		// record, never an unknown one.
		return ApprovedExecution{}, fmt.Errorf("app: close executed action %s: %w: %w", approval.ActionID, ErrApprovalCloseFailed, err)
	}
	// The receipt identifiers are re-read: neither the decide nor the close
	// returns them, and P4 and P5 print them. On the SAME uncancellable context
	// as the close: a read that dies with the caller would turn a landed close
	// into «the executed action could not be closed», which is a false sentence
	// about a ledger row that is right there.
	after, _, rerr := store.GetApproval(closeCtx, approvalID)
	if rerr != nil {
		return ApprovedExecution{}, fmt.Errorf("app: read back the receipt of %s: %w: %w", approvalID, ErrApprovalCloseFailed, rerr)
	}
	out := ApprovedExecution{
		Result:       result,
		ResultDigest: resultDigest,
		ReceiptID:    after.DecisionReceiptID,
		Operation:    op,
	}
	if execErr != nil {
		// Neither a deadline nor a delivered-and-unread answer is a refusal.
		// The call may well have gone out and its answer been lost, so the
		// effect's fate is genuinely unknown — which is what `unknown_outcome`
		// is for, and until the deadline cure nothing produced it.
		if outcome == action.StateOutcomeUnknown {
			out.Unknown = true
			out.FailureDetail = execErr.Error()
			return out, nil
		}
		// Anything else: the tool refused BEFORE anything left — a host off the
		// allow-list, a shield refusal at the dial, a malformed payload, a
		// failed dial. That is a DECIDED outcome with its receipt, and it
		// travels as one; knowing the attempt failed is a different fact from
		// not knowing what happened.
		//
		// This comment listed «an HTTP error status» among them, and that was
		// false: a status comes from a receiver that already read the body. So
		// did the cage's redirect refusal, raised over a response. Both wrap
		// tool.ErrEffectDelivered now and route above.
		//
		// It also cited a mould by a name that existed NOWHERE but in this
		// sentence. The rule is held by TestWebhookCall_everyBranchAfterDo,
		// which reads the tool's AST and requires the sentinel on every error
		// return past the point where a response exists.
		out.Failed = true
		out.FailureDetail = execErr.Error()
		return out, nil
	}
	return out, nil
}

// ApprovedExecution is what one approved run produced. It carries the receipt
// because neither DecideApprovalUnderLaw nor FinishWithResult returns it and
// both the CLI and the window print it.
//
// Failed is not an error: the tool ran, it said no, and the ledger closed it
// with its receipt. Reporting that through the error channel would make a
// KNOWN outcome indistinguishable from a failure to learn the outcome.
type ApprovedExecution struct {
	Result       string
	ResultDigest string
	ReceiptID    string
	Operation    action.Operation
	Failed       bool
	// Unknown is a deadline or a delivered request whose answer was lost: the call may have been delivered and the answer
	// lost, so nobody can say whether the effect happened.
	Unknown       bool
	FailureDetail string
}

// The named refusals of the ONE execution path. They exist so both callers —
// the operator CLI and the desktop window — can name what happened instead of
// matching the English of a sentence.
var (
	// ErrApprovalNotDecided is a request still awaiting a decision. Calling it
	// «already closed» would contradict what the row says.
	ErrApprovalNotDecided = errors.New("app: the approval is still awaiting a decision")
	// ErrApprovalAlreadyClosed is a request that is not awaiting execution.
	ErrApprovalAlreadyClosed = errors.New("app: the approval was already closed and is not awaiting execution")
	// ErrApprovalCloseFailed is the effect happening and its record not closing.
	ErrApprovalCloseFailed = errors.New("app: the executed action could not be closed")
	// ErrApprovalRecordUnreadable is the parked action's own row refusing to
	// read after the decision is sealed. It is evidence, not absence.
	ErrApprovalRecordUnreadable = errors.New("app: the parked action's record no longer reads")
)

// ResolveApprovalLaw resolves ONE brain's effective cage and its law
// pin in a SINGLE resolution (R6-X3): the operator CLI feeds BOTH the
// decision (the pin) and the deferred executor (the cage) from this
// one object.
// lawResolutionProbe is AS-121's counting seam. It is nil in production and a
// test arms it with setLawResolutionProbe.
//
// It lives HERE, on the one function every resolution goes through, and not on
// a caller's helper. The first version counted calls to the adapter's own
// private method, so the very mutation it published as its proof — calling
// BuildApprovalExecutor, which resolves again through a package function —
// left the mould green. An oracle that cannot see the branch it forbids is not
// an oracle.
var lawResolutionProbe func()

// setLawResolutionProbe arms the counting seam and returns it to nil. Test-only
// by construction: nothing in production assigns it.
func setLawResolutionProbe(f func()) func() {
	lawResolutionProbe = f
	return func() { lawResolutionProbe = nil }
}

func ResolveApprovalLaw(cfg *config.Config, brainName string) (*EffectiveCage, actionsqlite.PolicyPin, error) {
	if lawResolutionProbe != nil {
		lawResolutionProbe()
	}
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

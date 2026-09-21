// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package executor owns canonical action admission, approved claims, tool
// dispatch, and close. Immediate calls require an executor-owned decision plan
// and sealed-input action-bound request. Approved calls require a successful
// claim of the stored action.
package executor

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/identity"
	"github.com/Sebastian197/korvun/internal/policy"
	"github.com/Sebastian197/korvun/internal/tool"
)

var (
	// ErrUnknownTool reports a name absent from the executor registry.
	ErrUnknownTool = errors.New("executor: unknown tool")
	// ErrExecutionBindingMismatch reports a request, plan, or action that was
	// not prepared by this executor or no longer matches its sealed inputs.
	ErrExecutionBindingMismatch = errors.New("executor: execution binding mismatch")
	// ErrExecutionAlreadySubmitted reports reuse of a prepared request. All
	// copies of a Request share the same atomic single-use claim.
	ErrExecutionAlreadySubmitted = errors.New("executor: execution request already submitted")
)

// Recorder is the durable action-attempt seam used before and after effects.
// A nil Recorder keeps canonical admission in memory without claiming
// durability.
type Recorder interface {
	RecordAttempt(context.Context, action.Envelope, string, string, action.State) error
	Finish(context.Context, string, action.State, time.Time) error
}

// IdentifiedRecorder atomically records an attempt with its identity evidence.
type IdentifiedRecorder interface {
	RecordAttemptIdentified(context.Context, action.Envelope, string, string, action.State, action.IdentityEvidence) error
}

// AuthenticatedRecorder atomically records an attempt with Phase 1 identity
// evidence. A v2 request never falls back to a legacy recorder.
type AuthenticatedRecorder interface {
	RecordAttemptAuthenticated(context.Context, action.Envelope, string, string, action.State, identity.Evidence) error
}

// ResultRecorder closes an action together with the digest of its result.
type ResultRecorder interface {
	FinishWithResult(context.Context, string, action.State, time.Time, string) error
}

// ApprovalRequester parks an action and its exact raw parameters for approval.
type ApprovalRequester interface {
	RequestApproval(context.Context, action.Envelope, string, string) (string, error)
}

// AuthenticatedApprovalRequester parks an action and its Phase 1 evidence in
// one transaction.
type AuthenticatedApprovalRequester interface {
	RequestApprovalAuthenticated(context.Context, action.Envelope, string, string, identity.Evidence) (string, error)
}

// EffectClassifier resolves the declared effect of an operation name.
type EffectClassifier func(string) (action.EffectDescriptor, bool)

// Governance contains the fixed policy inputs used for per-channel selection.
type Governance struct {
	Grants      []policy.ToolGrant
	Attrs       map[string]policy.ToolAttrs
	Sensitivity policy.Sensitivity
	Locality    policy.Locality
}

// IdentityConfig contains the fixed provenance and authority facts used to
// bind action identity before recording.
type IdentityConfig struct {
	Registry      action.ProvenanceRegistry
	IntentID      string
	GrantID       string
	EffectCeiling action.EffectClass
}

// CoordinatorConfig fixes the non-request inputs of one executor.
type CoordinatorConfig struct {
	BrainName         string
	Recorder          Recorder
	Identity          *IdentityConfig
	PrincipalResolver *identity.Resolver
	EffectClassifier  EffectClassifier
	Governance        *Governance
	NewActionID       func() string
	BoundOperation    func(string) string
	InvocationProbe   func(action.Envelope)
	// CloseClock supplies terminal timestamps. Immediate execution wires the
	// brain clock; approved execution keeps its historical wall clock.
	CloseClock func() time.Time
	// BeforeClose observes an immediate invocation after dispatch and before
	// its terminal record. Approved execution leaves this hook nil.
	BeforeClose func(context.Context, *envelope.Envelope, Result)
	// BeforeRecord observes a non-executing immediate decision before its
	// best-effort terminal record. Approved execution leaves this hook nil.
	BeforeRecord func(context.Context, *envelope.Envelope, Result)
	// BeforeIdentityFallback observes unresolved provenance after the identified
	// path fails and before the legacy best-effort record is attempted.
	BeforeIdentityFallback func(context.Context, *envelope.Envelope, Result)
}

type executorOwner struct {
	marker byte
}

// Executor is the sole production owner of physical tool dispatch.
type Executor struct {
	tools   tool.Registry
	perTool time.Duration
	now     func() time.Time
	config  CoordinatorConfig
	owner   *executorOwner
}

// New builds an ungoverned executor. It remains the construction seam for
// approved execution, whose decision was already sealed by the store claim.
func New(tools tool.Registry, perTool time.Duration, now func() time.Time) *Executor {
	return NewCoordinator(tools, perTool, now, CoordinatorConfig{
		CloseClock: func() time.Time { return time.Now().UTC() },
	})
}

// Has reports whether the executor registry contains name.
func (e *Executor) Has(name string) bool {
	_, ok := e.tools[name]
	return ok
}

// NewCoordinator builds an executor with fixed policy, identity, recording,
// effect, and action-construction inputs.
func NewCoordinator(tools tool.Registry, perTool time.Duration, now func() time.Time, config CoordinatorConfig) *Executor {
	if config.NewActionID == nil {
		config.NewActionID = action.NewID
	}
	if config.BoundOperation == nil {
		config.BoundOperation = func(name string) string { return name }
	}
	if config.CloseClock == nil {
		config.CloseClock = now
	}
	return &Executor{
		tools:   tools,
		perTool: perTool,
		now:     now,
		config:  config,
		owner:   &executorOwner{marker: 1},
	}
}

// DecisionPlan is an opaque executor-owned snapshot of one capability
// selection. Its zero value is invalid.
type DecisionPlan struct {
	owner     *executorOwner
	channel   string
	governed  bool
	decisions map[string]policy.ToolDecision
}

// SelectTools selects the advertised registry once and returns the matching
// opaque execution plan. Policy errors return an empty advertisement and a
// valid deny-all plan alongside the original error.
func (e *Executor) SelectTools(ctx context.Context, channel string) (tool.Registry, *DecisionPlan, error) {
	decisions, governed := map[string]policy.ToolDecision(nil), false
	var selectErr error
	if e.config.Governance != nil {
		governed = true
		g := e.config.Governance
		decisions, selectErr = policy.SelectTools(g.Grants, g.Attrs, policy.ToolQuery{
			Channel:     channel,
			Sensitivity: g.Sensitivity,
			Locality:    g.Locality,
		})
		if selectErr != nil {
			decisions = map[string]policy.ToolDecision{}
		}
	}
	plan := &DecisionPlan{
		owner:     e.owner,
		channel:   channel,
		governed:  governed,
		decisions: cloneDecisions(decisions),
	}
	if !governed {
		return e.tools, plan, nil
	}
	advertised := make(tool.Registry)
	if selectErr == nil {
		for name, candidate := range e.tools {
			decision, ok := decisions[name]
			if ok && (decision.Mode == policy.ToolAllow || decision.Mode == policy.ToolShadow) {
				advertised[name] = candidate
			}
		}
	}
	return advertised, plan, selectErr
}

// Decisions returns a copy of the decisions sealed in a valid plan and
// reports whether the plan represents governed execution.
func (e *Executor) Decisions(plan *DecisionPlan) (map[string]policy.ToolDecision, bool, error) {
	if plan == nil || plan.owner != e.owner {
		return nil, false, ErrExecutionBindingMismatch
	}
	return cloneDecisions(plan.decisions), plan.governed, nil
}

func cloneDecisions(src map[string]policy.ToolDecision) map[string]policy.ToolDecision {
	if src == nil {
		return nil
	}
	dst := make(map[string]policy.ToolDecision, len(src))
	for name, decision := range src {
		dst[name] = decision
	}
	return dst
}

// Submission contains the caller-owned facts copied by Prepare. Scope and
// policy decisions are deliberately absent.
type Submission struct {
	Inbound   *envelope.Envelope
	Ingress   identity.AuthenticatedIngress
	Lane      string
	Name      string
	Arguments string
	Plan      *DecisionPlan
}

type requestUse struct {
	claimed atomic.Bool
}

// Request is an opaque, copy-safe, single-use execution capability prepared
// by one Executor. Its zero value is invalid.
type Request struct {
	owner          *executorOwner
	plan           *DecisionPlan
	action         action.Envelope
	rawName        string
	args           string
	lane           string
	channel        string
	senderID       string
	ingress        identity.AuthenticatedIngress
	evidence       *identity.Evidence
	scope          tool.Scope
	inbound        *envelope.Envelope
	known          bool
	effectDeclared bool
	use            *requestUse
}

// Prepare snapshots inbound facts and creates the request's one canonical
// action. It performs no recorder, approval, close, or tool call.
func (e *Executor) Prepare(submission Submission) (*Request, error) {
	if submission.Inbound == nil || submission.Plan == nil ||
		submission.Plan.owner != e.owner ||
		submission.Plan.channel != submission.Inbound.Channel {
		return nil, ErrExecutionBindingMismatch
	}
	_, known := e.tools[submission.Name]
	operationName := submission.Name
	if !known {
		operationName = e.config.BoundOperation(submission.Name)
	}
	effectClass := action.EffectClass("unclassified")
	declared := true
	if known && e.config.EffectClassifier != nil {
		descriptor, firstDeclared := e.config.EffectClassifier(submission.Name)
		declared = firstDeclared
		if firstDeclared {
			effectClass = descriptor.Class
		}
	}
	op := action.Operation{Namespace: "tool", Name: operationName, Version: 1}
	canonical := action.NewEnvelope(
		e.config.NewActionID(),
		submission.Inbound.ID,
		action.Source{Kind: "agent_brain", Protocol: submission.Lane, Channel: submission.Inbound.Channel},
		op,
		submission.Arguments,
		e.now(),
	)
	canonical.Effect = action.Effect{Class: string(effectClass)}
	conv := ""
	if key, err := conversation.KeyFromEnvelope(submission.Inbound); err == nil {
		conv = string(key)
	}
	return &Request{
		owner:    e.owner,
		plan:     submission.Plan,
		action:   canonical,
		rawName:  submission.Name,
		args:     submission.Arguments,
		lane:     submission.Lane,
		channel:  submission.Inbound.Channel,
		senderID: submission.Inbound.Sender.ID,
		ingress:  submission.Ingress,
		scope:    tool.Scope{Brain: e.config.BrainName, Conversation: conv},
		// The LIVE inbound envelope, not a copy: the bus event the brain
		// publishes carries this pointer, and before this phase every branch
		// handed subscribers the live message. A clone made the audit surface
		// observably different (the twenty-first pass, P2-2) — and half-deep at
		// that, since Keyboard and Operation stayed shared. Phase 0 changes
		// nothing observable, so the pointer stays the caller's.
		inbound:        submission.Inbound,
		known:          known,
		effectDeclared: declared,
		use:            &requestUse{},
	}, nil
}

// Branch is the finite coordinator outcome presented to an adapter.
type Branch string

const (
	// BranchUnknown is a request for a registry entry that does not exist.
	BranchUnknown Branch = "unknown"
	// BranchEffectUndeclared is a known tool absent from the effect registry.
	BranchEffectUndeclared Branch = "effect_undeclared"
	// BranchDenied is a terminal refusal, including record failure.
	BranchDenied Branch = "denied"
	// BranchShadowed is a rehearsal that never dispatches.
	BranchShadowed Branch = "shadowed"
	// BranchPending is a request parked for human approval.
	BranchPending Branch = "pending"
	// BranchExecuted reached the private physical dispatcher.
	BranchExecuted Branch = "executed"
)

// Result contains coordinator facts needed by a presentation adapter. It
// carries no user-facing prose.
type Result struct {
	Action                     action.Envelope
	Name                       string
	Arguments                  string
	Branch                     Branch
	Rule                       string
	Decision                   policy.ToolDecision
	Ungoverned                 bool
	Output                     string
	Latency                    time.Duration
	ApprovalID                 string
	ToolError                  error
	RecordError                error
	CloseError                 error
	IdentityFallback           bool
	AuthorizationIdentityError bool
	ApprovalIdentityError      bool
	ApprovalError              error
}

// Submit consumes a prepared request exactly once, decides it, records the
// decision when configured, and invokes only after admission succeeds.
func (e *Executor) Submit(ctx context.Context, request *Request) (Result, error) {
	if err := e.validateRequest(request); err != nil {
		return Result{}, err
	}
	if !request.use.claimed.CompareAndSwap(false, true) {
		return Result{}, ErrExecutionAlreadySubmitted
	}
	result := Result{
		Action:     request.action,
		Name:       request.rawName,
		Arguments:  request.args,
		Ungoverned: !request.plan.governed,
	}
	if e.config.PrincipalResolver != nil {
		evidence, err := e.config.PrincipalResolver.Resolve(request.ingress, identity.ResolveRequest{
			ActionID: request.action.ActionID, RequestID: request.inbound.ID,
			Channel: request.channel, Brain: e.config.BrainName,
		})
		if err != nil {
			return result, err
		}
		request.evidence = &evidence
		request.action.Principal = action.PrincipalRef{
			PrincipalID:        evidence.ActorPrincipalID,
			EvidenceID:         evidence.EvidenceID,
			ResponsibleHumanID: evidence.ResponsiblePrincipalID,
		}
		result.Action = request.action
	}
	if !request.known {
		result.Branch = BranchUnknown
		result.Rule = "unknown_tool"
		e.observeDecision(ctx, request, result)
		e.recordAttempt(ctx, request, &result, "deny", result.Rule, action.StateDenied)
		return result, ErrUnknownTool
	}
	if e.config.EffectClassifier != nil && !request.effectDeclared {
		result.Branch = BranchEffectUndeclared
		result.Rule = "effect_undeclared"
		e.observeDecision(ctx, request, result)
		e.recordAttempt(ctx, request, &result, "deny", result.Rule, action.StateDenied)
		return result, nil
	}
	if request.plan.governed {
		decision, decided := request.plan.decisions[request.rawName]
		result.Decision = decision
		switch {
		case decided && decision.Mode == policy.ToolShadow:
			result.Branch = BranchShadowed
			result.Rule = "shadow"
			e.observeDecision(ctx, request, result)
			e.recordAttempt(ctx, request, &result, "shadow", result.Rule, action.StateShadowed)
			return result, nil
		case !decided || decision.Mode != policy.ToolAllow:
			result.Branch = BranchDenied
			result.Rule = string(policy.ToolRuleNotGranted)
			if decided {
				result.Rule = string(decision.Rule)
			}
			e.observeDecision(ctx, request, result)
			e.recordAttempt(ctx, request, &result, "deny", result.Rule, action.StateDenied)
			return result, nil
		}
	}
	if e.config.Identity != nil && e.config.EffectClassifier != nil {
		descriptor, effectForGate := e.config.EffectClassifier(request.rawName)
		request.action.Effect.Class = "unclassified"
		if effectForGate {
			request.action.Effect.Class = string(descriptor.Class)
		}
		result.Action = request.action
		if rule := effectGateRule(descriptor.Class, e.config.Identity.EffectCeiling, false); effectForGate && rule != "" {
			if rule == "approval_unavailable" {
				if request.evidence != nil {
					if requester, ok := e.config.Recorder.(AuthenticatedApprovalRequester); ok {
						e.refreshDurableEffect(request)
						approved := request.action
						e.bindAuthority(&approved, "require_approval")
						if approvalID, err := requester.RequestApprovalAuthenticated(ctx, approved, "require_approval", request.args, *request.evidence); err != nil {
							result.ApprovalError = err
						} else {
							result.Action = approved
							result.Branch = BranchPending
							result.Rule = "require_approval"
							result.ApprovalID = approvalID
							return result, nil
						}
					} else {
						result.ApprovalIdentityError = true
					}
				} else if requester, ok := e.config.Recorder.(ApprovalRequester); ok {
					e.refreshDurableEffect(request)
					approved := request.action
					evidence, resolved := e.bindIdentity(request, &approved, "require_approval")
					_ = evidence
					if !resolved {
						result.ApprovalIdentityError = true
					} else if approvalID, err := requester.RequestApproval(ctx, approved, "require_approval", request.args); err != nil {
						result.ApprovalError = err
					} else {
						result.Action = approved
						result.Branch = BranchPending
						result.Rule = "require_approval"
						result.ApprovalID = approvalID
						return result, nil
					}
				}
			}
			result.Branch = BranchDenied
			result.Rule = rule
			e.observeDecision(ctx, request, result)
			e.recordAttempt(ctx, request, &result, "deny", rule, action.StateDenied)
			return result, nil
		}
	}
	grantRule := "granted"
	if !request.plan.governed {
		grantRule = "ungoverned"
	}
	if !e.recordAuthorized(ctx, request, &result, grantRule) {
		result.Branch = BranchDenied
		result.Rule = "record_failed"
		e.observeDecision(ctx, request, result)
		return result, nil
	}
	capability := invocationCapability{
		action:   result.Action,
		toolName: request.rawName,
		args:     request.args,
		scope:    request.scope,
	}
	result.Branch = BranchExecuted
	result.Rule = grantRule
	execution := e.invokeAndClose(ctx, capability, e.immediateCloser(), func(execution executionResult) {
		result.Output = execution.output
		result.Latency = execution.latency
		result.ToolError = execution.toolErr
		if e.config.BeforeClose != nil {
			e.config.BeforeClose(ctx, request.inbound, result)
		}
	})
	result.CloseError = execution.closeErr
	return result, execution.toolErr
}

func (e *Executor) observeDecision(ctx context.Context, request *Request, result Result) {
	if e.config.BeforeRecord != nil {
		e.config.BeforeRecord(ctx, request.inbound, result)
	}
}

func (e *Executor) validateRequest(request *Request) error {
	if request == nil || request.use == nil || request.owner != e.owner ||
		request.plan == nil || request.plan.owner != e.owner ||
		request.plan.channel != request.channel {
		return ErrExecutionBindingMismatch
	}
	wantName := request.rawName
	if !request.known {
		wantName = e.config.BoundOperation(request.rawName)
	}
	wantOperation := action.Operation{Namespace: "tool", Name: wantName, Version: 1}
	if request.action.ActionID == "" || request.action.Operation != wantOperation ||
		request.action.Source != (action.Source{Kind: "agent_brain", Protocol: request.lane, Channel: request.channel}) ||
		request.action.ParametersDigest != action.Digest(request.action.Operation, request.args) {
		return ErrExecutionBindingMismatch
	}
	return nil
}

func (e *Executor) recordAttempt(ctx context.Context, request *Request, result *Result, outcome, rule string, state action.State) {
	recorder := e.config.Recorder
	if recorder == nil {
		return
	}
	e.refreshDurableEffect(request)
	canonical := request.action
	result.Action = canonical
	if request.evidence != nil {
		e.bindAuthority(&canonical, rule)
		result.Action = canonical
		authenticated, ok := recorder.(AuthenticatedRecorder)
		if !ok {
			result.RecordError = identity.ErrIdentityEvidenceMissing
			return
		}
		result.RecordError = authenticated.RecordAttemptAuthenticated(
			ctx, canonical, outcome, rule, state, *request.evidence)
		return
	}
	if e.config.Identity != nil {
		if identified, ok := recorder.(IdentifiedRecorder); ok {
			evidence, resolved := e.bindIdentity(request, &canonical, rule)
			if resolved {
				result.Action = canonical
				result.RecordError = identified.RecordAttemptIdentified(ctx, canonical, outcome, rule, state, evidence)
				return
			}
			result.IdentityFallback = true
			if e.config.BeforeIdentityFallback != nil {
				e.config.BeforeIdentityFallback(ctx, request.inbound, *result)
			}
		}
	}
	result.RecordError = recorder.RecordAttempt(ctx, canonical, outcome, rule, state)
}

func (e *Executor) refreshDurableEffect(request *Request) {
	if e.config.EffectClassifier == nil || e.config.Recorder == nil {
		return
	}
	request.action.Effect.Class = "unclassified"
	if descriptor, declared := e.config.EffectClassifier(request.action.Operation.Name); declared {
		request.action.Effect.Class = string(descriptor.Class)
	}
}

func (e *Executor) recordAuthorized(ctx context.Context, request *Request, result *Result, rule string) bool {
	recorder := e.config.Recorder
	if recorder == nil {
		return true
	}
	e.refreshDurableEffect(request)
	canonical := request.action
	result.Action = canonical
	if request.evidence != nil {
		e.bindAuthority(&canonical, rule)
		result.Action = canonical
		authenticated, ok := recorder.(AuthenticatedRecorder)
		if !ok {
			result.AuthorizationIdentityError = true
			result.RecordError = identity.ErrIdentityEvidenceMissing
			return false
		}
		if err := authenticated.RecordAttemptAuthenticated(
			ctx, canonical, "allow", rule, action.StateAuthorized, *request.evidence); err != nil {
			result.RecordError = err
			return false
		}
		return true
	}
	if e.config.Identity != nil {
		if identified, ok := recorder.(IdentifiedRecorder); ok {
			evidence, resolved := e.bindIdentity(request, &canonical, rule)
			if !resolved {
				result.AuthorizationIdentityError = true
				return false
			}
			result.Action = canonical
			if err := identified.RecordAttemptIdentified(ctx, canonical, "allow", rule, action.StateAuthorized, evidence); err != nil {
				result.RecordError = err
				return false
			}
			return true
		}
	}
	if err := recorder.RecordAttempt(ctx, canonical, "allow", rule, action.StateAuthorized); err != nil {
		result.RecordError = err
		return false
	}
	return true
}

func (e *Executor) bindIdentity(request *Request, canonical *action.Envelope, rule string) (action.IdentityEvidence, bool) {
	identity := e.config.Identity
	if identity == nil {
		return action.IdentityEvidence{}, true
	}
	_, evidence, err := action.ResolvePrincipal(identity.Registry, request.channel, request.senderID, e.now())
	if err != nil {
		return action.IdentityEvidence{}, false
	}
	brain := action.BrainPrincipal(e.config.BrainName)
	canonical.Principal = action.PrincipalRef{
		PrincipalID:        brain.PrincipalID,
		EvidenceID:         evidence.EvidenceID,
		ResponsibleHumanID: brain.ResponsibleHumanID,
	}
	canonical.IntentID = identity.IntentID
	if rule == "granted" && identity.GrantID != "" {
		canonical.AuthorityRefs = []string{identity.GrantID}
	}
	return evidence, true
}

func (e *Executor) bindAuthority(canonical *action.Envelope, rule string) {
	if e.config.Identity == nil {
		return
	}
	canonical.IntentID = e.config.Identity.IntentID
	if rule == "granted" && e.config.Identity.GrantID != "" {
		canonical.AuthorityRefs = []string{e.config.Identity.GrantID}
	}
}

func effectGateRule(class, ceiling action.EffectClass, requiresPrepare bool) string {
	if ceiling != "" {
		if class.Rank() > ceiling.Rank() {
			return "effect_ceiling"
		}
		if class == action.EffectWriteIrreversible || class == action.EffectCritical {
			return "approval_unavailable"
		}
	}
	if requiresPrepare {
		return "prepare_unavailable"
	}
	return ""
}

type invocationCapability struct {
	action   action.Envelope
	toolName string
	args     string
	scope    tool.Scope
}

type executionResult struct {
	output    string
	latency   time.Duration
	toolErr   error
	outcome   action.State
	digest    string
	closeErr  error
	closeTime time.Time
}

type closeExecution func(context.Context, string, action.State, time.Time, string) error

func (e *Executor) invokeAndClose(
	ctx context.Context,
	capability invocationCapability,
	close closeExecution,
	beforeClose func(executionResult),
) executionResult {
	if e.config.InvocationProbe != nil {
		e.config.InvocationProbe(capability.action)
	}
	output, latency, toolErr := e.dispatch(ctx, capability.toolName, capability.scope, capability.args)
	outcome := tool.CloseStateAfterRun(toolErr)
	digest := ""
	if outcome == action.StateSucceeded {
		digest = action.HashCanonical(output)
	}
	result := executionResult{
		output: output, latency: latency, toolErr: toolErr,
		outcome: outcome, digest: digest,
	}
	if beforeClose != nil {
		beforeClose(result)
	}
	if close != nil {
		closeCtx := context.WithoutCancel(ctx)
		closedAt := e.config.CloseClock()
		result.closeErr = close(closeCtx, capability.action.ActionID, outcome, closedAt, digest)
		result.closeTime = closedAt
	}
	return result
}

func (e *Executor) immediateCloser() closeExecution {
	if e.config.Recorder == nil {
		return nil
	}
	return func(ctx context.Context, actionID string, state action.State, at time.Time, digest string) error {
		if actionID == "" {
			return nil
		}
		if recorder, ok := e.config.Recorder.(ResultRecorder); ok {
			return recorder.FinishWithResult(ctx, actionID, state, at, digest)
		}
		return e.config.Recorder.Finish(ctx, actionID, state, at)
	}
}

// dispatch is the only production function that physically invokes tools.
func (e *Executor) dispatch(ctx context.Context, name string, scope tool.Scope, args string) (string, time.Duration, error) {
	candidate, ok := e.tools[name]
	if !ok {
		return "", 0, ErrUnknownTool
	}
	toolCtx := ctx
	if e.perTool > 0 {
		var cancel context.CancelFunc
		toolCtx, cancel = context.WithTimeout(ctx, e.perTool)
		defer cancel()
	}
	start := e.now()
	var output string
	var err error
	if scoped, ok := candidate.(tool.ScopedTool); ok {
		output, err = scoped.ExecuteScoped(toolCtx, scope, args)
	} else {
		output, err = candidate.Execute(toolCtx, args)
	}
	return output, e.now().Sub(start), err
}

// ApprovalStore is the executor-owned adapter for the existing approval store
// transactions. Claim must return only the operation and bytes it judged.
type ApprovalStore interface {
	ReadApproval(context.Context, string) (action.Approval, error)
	ReadActionState(context.Context, string) (action.State, error)
	Claim(context.Context, string, *action.Approval) ([]byte, action.Operation, error)
	Close(context.Context, string, action.State, time.Time, string) error
	ReadReceipt(context.Context, string) (string, error)
}

// ResumeStage names the exact coordinator stage that prevented an approved
// execution. Presentation adapters retain their existing public errors.
type ResumeStage string

const (
	// ResumeReadApproval failed before an approval row could be judged.
	ResumeReadApproval ResumeStage = "read_approval"
	// ResumeApprovalPending found a request still awaiting a decision.
	ResumeApprovalPending ResumeStage = "approval_pending"
	// ResumeApprovalClosed found a terminal approval status.
	ResumeApprovalClosed ResumeStage = "approval_closed"
	// ResumeReadAction failed to read the parked action.
	ResumeReadAction ResumeStage = "read_action"
	// ResumeActionClosed found a parked action outside APPROVED.
	ResumeActionClosed ResumeStage = "action_closed"
	// ResumeClaim failed the atomic approval claim.
	ResumeClaim ResumeStage = "claim"
	// ResumeClose failed after the tool returned.
	ResumeClose ResumeStage = "close"
	// ResumeReadReceipt failed after the close landed.
	ResumeReadReceipt ResumeStage = "read_receipt"
)

// ResumeError reports one failed approved-execution stage without rewriting
// its underlying store sentinel.
type ResumeError struct {
	Stage      ResumeStage
	ApprovalID string
	ActionID   string
	Status     action.ApprovalStatus
	State      action.State
	Err        error
}

func (e *ResumeError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("executor: resume approved at %s: %v", e.Stage, e.Err)
	}
	return fmt.Sprintf("executor: resume approved at %s", e.Stage)
}

// Unwrap exposes the original store or claim error.
func (e *ResumeError) Unwrap() error { return e.Err }

// ApprovedResult is the execution and receipt material returned to the app
// adapter after one approved claim.
type ApprovedResult struct {
	Result       string
	ResultDigest string
	ReceiptID    string
	Operation    action.Operation
	Outcome      action.State
	ToolError    error
}

// ResumeApproved performs the current prechecks and claim through ApprovalStore,
// then invokes and closes through the same private routine as Submit.
func (e *Executor) ResumeApproved(ctx context.Context, store ApprovalStore, approvalID string) (ApprovedResult, error) {
	if store == nil {
		return ApprovedResult{}, ErrExecutionBindingMismatch
	}
	approval, err := store.ReadApproval(ctx, approvalID)
	if err != nil {
		return ApprovedResult{}, &ResumeError{Stage: ResumeReadApproval, ApprovalID: approvalID, Err: err}
	}
	if approval.Status == action.ApprovalPending {
		return ApprovedResult{}, &ResumeError{Stage: ResumeApprovalPending, ApprovalID: approvalID, ActionID: approval.ActionID, Status: approval.Status}
	}
	if approval.Status != action.ApprovalApproved {
		return ApprovedResult{}, &ResumeError{Stage: ResumeApprovalClosed, ApprovalID: approvalID, ActionID: approval.ActionID, Status: approval.Status}
	}
	state, err := store.ReadActionState(ctx, approval.ActionID)
	if err != nil {
		return ApprovedResult{}, &ResumeError{Stage: ResumeReadAction, ApprovalID: approvalID, ActionID: approval.ActionID, Err: err}
	}
	if state != action.StateApproved {
		return ApprovedResult{}, &ResumeError{Stage: ResumeActionClosed, ApprovalID: approvalID, ActionID: approval.ActionID, State: state}
	}
	params, operation, err := store.Claim(ctx, approvalID, &approval)
	if err != nil {
		return ApprovedResult{}, &ResumeError{Stage: ResumeClaim, ApprovalID: approvalID, ActionID: approval.ActionID, Err: err}
	}
	capability := invocationCapability{
		action: action.Envelope{
			ActionID:         approval.ActionID,
			Operation:        operation,
			ParametersDigest: action.Digest(operation, string(params)),
		},
		toolName: operation.Name,
		args:     string(params),
		scope:    tool.Scope{},
	}
	execution := e.invokeAndClose(ctx, capability, store.Close, nil)
	if execution.closeErr != nil {
		return ApprovedResult{}, &ResumeError{Stage: ResumeClose, ApprovalID: approvalID, ActionID: approval.ActionID, Err: execution.closeErr}
	}
	receiptID, err := store.ReadReceipt(context.WithoutCancel(ctx), approvalID)
	if err != nil {
		return ApprovedResult{}, &ResumeError{Stage: ResumeReadReceipt, ApprovalID: approvalID, ActionID: approval.ActionID, Err: err}
	}
	return ApprovedResult{
		Result:       execution.output,
		ResultDigest: execution.digest,
		ReceiptID:    receiptID,
		Operation:    operation,
		Outcome:      execution.outcome,
		ToolError:    execution.toolErr,
	}, nil
}

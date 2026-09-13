// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The approvals ADAPTER: the seam controlapi.Approvals implemented against
// the real store and the live profile.
//
// Its whole job is NAMING. The screen renders by error name, so every refusal
// that reaches it has to carry the name its literal belongs to — and the one
// thing this file may never do is answer a shrug. Two rules run through it:
//
//   - the name comes from a TYPED sentinel or from a column that was READ,
//     never from the text of an error;
//   - the frontier between "nothing was decided, retry" and "the decision
//     exists and the effect is unknown" is the COMMIT of the decide, not the
//     HTTP verb.
//
// Anchors: docs/superpowers/specs/2026-09-08-approvals-screen-ux.md §11 and
// §12, and 2026-09-12-approvals-adapter-pre-test-review.md §0-sexies.

package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/controlapi"
	"github.com/Sebastian197/korvun/internal/tool"
)

// ErrBrainNotInProfile is the brain a parked request names and the current
// profile no longer has.
//
// It exists because the screen paints `brain_gone`, and until now the only
// source of that fact was a plain fmt.Errorf — so the only way to name it was
// to match its English sentence, which FR-API-15 calls a finding rather than
// an implementation. A name the screen paints and the server can never emit
// is dead interface.
var ErrBrainNotInProfile = errors.New("app: the brain is no longer in the profile")

// ApprovalsAdapter implements controlapi.Approvals.
type ApprovalsAdapter struct {
	cfg   *config.Config
	store *actionsqlite.Store
}

// ApprovalsAdapterOption configures the adapter.
type ApprovalsAdapterOption func(*ApprovalsAdapter)

// NewApprovalsAdapter wires the seam to a real store and a live profile.
func NewApprovalsAdapter(cfg *config.Config, store *actionsqlite.Store, opts ...ApprovalsAdapterOption) *ApprovalsAdapter {
	a := &ApprovalsAdapter{cfg: cfg, store: store}
	for _, o := range opts {
		o(a)
	}
	return a
}

func (a *ApprovalsAdapter) enabled() bool {
	return a.cfg.Approvals != nil && a.cfg.Approvals.Enabled
}

// ListPending answers the page plus the gate block.
//
// With the switch OFF it refuses by name instead of answering an empty page:
// "nothing parked" and "nothing can park" have to be different answers, or the
// screen cannot tell a quiet tray from a hole in the guarantee. And the switch
// only forks the RECORDER — requests parked before it was flipped are still in
// the store and still decidable from the CLI — so the refusal never asserts
// emptiness.
func (a *ApprovalsAdapter) ListPending(ctx context.Context) (controlapi.ApprovalList, error) {
	if !a.enabled() {
		return controlapi.ApprovalList{}, controlapi.ErrApprovalsDisabled
	}
	listing, err := a.store.ListPendingApprovals(ctx, controlapi.ApprovalsPageLimit)
	if err != nil {
		return controlapi.ApprovalList{}, a.nameRead(err)
	}
	out := controlapi.ApprovalList{
		Gate: controlapi.ApprovalGate{
			ApprovalsEnabled: true,
			BrainsTotal:      len(a.cfg.Brains),
			BrainsCanPark:    a.brainsThatCanPark(),
			RowsSkipped:      len(listing.Skipped),
		},
		Rows: make([]controlapi.ApprovalRow, 0, len(listing.Rows)),
	}
	for _, r := range listing.Rows {
		out.Rows = append(out.Rows, controlapi.ApprovalRow{
			ID:       r.Approval.ApprovalID,
			ActionID: r.Approval.ActionID,
			// A preview that did not parse leaves operation and class EMPTY
			// rather than guessed: the screen has a banner for exactly that
			// and nothing here may presume a class the store did not read.
			Operation:   r.Preview.Operation,
			EffectClass: string(r.Preview.EffectClass),
			ExpiresAt:   rfc3339(r.Approval.ExpiresAt),
			Digest:      r.Approval.ActionDigest,
			Origin:      r.Channel,
		})
	}
	return out, nil
}

// Detail serves one parked request from ONE snapshot.
func (a *ApprovalsAdapter) Detail(ctx context.Context, id string) (controlapi.ApprovalDetail, error) {
	if !a.enabled() {
		return controlapi.ApprovalDetail{}, controlapi.ErrApprovalsDisabled
	}
	cage, pin, lawErr := a.resolveLawFor(ctx, id)
	var d actionsqlite.ApprovalDetailRow
	var err error
	if lawErr == nil {
		d, err = a.store.ApprovalDetailUnderLaw(ctx, id, pin)
	} else {
		d, err = a.store.ApprovalDetail(ctx, id)
	}
	if err != nil {
		return controlapi.ApprovalDetail{}, a.nameRead(err)
	}
	// Precedence by STATE beats any belt's name (FR-API-18). An unknown
	// status is not a document either: the column has no CHECK, so anything
	// outside the game fails closed the way the domain already does.
	switch d.Approval.Status {
	case action.ApprovalPending:
	case action.ApprovalExpired:
		return controlapi.ApprovalDetail{}, controlapi.ErrApprovalExpired
	default:
		return controlapi.ApprovalDetail{}, controlapi.ErrApprovalAlreadyDecided
	}
	out := controlapi.ApprovalDetail{
		ApprovalRow: controlapi.ApprovalRow{
			ID:          d.Approval.ApprovalID,
			ActionID:    d.Approval.ActionID,
			Operation:   d.Preview.Operation,
			EffectClass: string(d.Preview.EffectClass),
			ExpiresAt:   rfc3339(d.Approval.ExpiresAt),
			Digest:      d.Approval.ActionDigest,
			Origin:      channelOfPreview(d.Preview),
		},
		Purpose:         d.Preview.IntentPurpose,
		PrincipalID:     d.Preview.PrincipalID,
		Reversibility:   d.Preview.Reversibility,
		ToolCage:        d.Preview.ToolCage,
		RequiredRule:    d.Preview.RequiredRule,
		LawDigest:       d.Approval.PolicyDigest,
		Parameters:      string(d.Params),
		ParametersState: string(d.ParamsState),
		BrainGone:       lawErr != nil && errors.Is(lawErr, ErrBrainNotInProfile),
	}
	_ = cage
	return out, nil
}

// rfc3339 renders a time column; the zero time stays an EMPTY string, which is
// the only case FR-API-3 allows.
func rfc3339(t interface{ IsZero() bool }) string {
	type formatter interface{ Format(string) string }
	if t.IsZero() {
		return ""
	}
	if f, ok := t.(formatter); ok {
		return f.Format("2006-01-02T15:04:05.999999999Z07:00")
	}
	return ""
}

// channelOfPreview reads the origin out of the sealed resources set, and only
// when the set has exactly ONE element. The canonical form SORTS it, so an
// index over any other shape is a position guard over bytes an external hand
// wrote — the class of mistake that names a value by where it sits.
func channelOfPreview(p action.ActionPreview) string {
	if len(p.Resources) != 1 {
		return ""
	}
	return p.Resources[0]
}

// nameRead maps a store refusal on a READ door. Nothing has been decided here,
// so a driver failure is `unavailable` — the honest "retry", not a sentence
// about an execution that never began.
func (a *ApprovalsAdapter) nameRead(err error) error {
	switch {
	case errors.Is(err, actionsqlite.ErrApprovalInvalidated):
		return controlapi.LawMoved(a.currentLawDigest())
	case errors.Is(err, actionsqlite.ErrApprovalEvidenceCorrupt):
		return controlapi.ErrApprovalEvidenceCorrupt
	case errors.Is(err, actionsqlite.ErrApprovalParamsDigestMismatch):
		return controlapi.ErrApprovalParamsDigestMismatch
	case errors.Is(err, actionsqlite.ErrApprovalNotFound):
		return controlapi.ErrApprovalNotFound
	case errors.Is(err, actionsqlite.ErrApprovalUnreadable):
		return controlapi.ErrApprovalsUnavailable
	default:
		return controlapi.ErrApprovalsUnavailable
	}
}

func (a *ApprovalsAdapter) currentLawDigest() string {
	if len(a.cfg.Brains) == 0 {
		return ""
	}
	_, pin, err := ResolveApprovalLaw(a.cfg, a.cfg.Brains[0].Name)
	if err != nil {
		return ""
	}
	return pin.Digest
}

// brainsThatCanPark counts the brains that meet the FIVE conditions the gate
// really demands, over the channels the profile configures.
//
// It is a declared UPPER BOUND, not a promise. The fifth condition is judged
// PER CHANNEL — policy.SelectTools receives the channel on every message — so
// no channel-independent boolean exists, and V1 and E4 print this number with
// that scope.
func (a *ApprovalsAdapter) brainsThatCanPark() int {
	// 1 · without the action store nothing can be parked at all.
	if a.cfg.Storage == nil {
		return 0
	}
	n := 0
	for i := range a.cfg.Brains {
		if a.brainCanPark(&a.cfg.Brains[i]) {
			n++
		}
	}
	return n
}

func (a *ApprovalsAdapter) brainCanPark(bc *config.BrainConfig) bool {
	// 2 · only an AGENT brain runs tools.
	if bc.Agent == nil {
		return false
	}
	// 3 · the ceiling has to be ON the ladder and reach write_irreversible.
	// An unknown class ranks ABOVE critical, which is fail-closed for the
	// shield and would be fail-OPEN here, so it is refused by name.
	ceiling := action.EffectClass(bc.Agent.EffectCeiling)
	if !ceiling.OnLadder() || ceiling.Rank() < action.EffectWriteIrreversible.Rank() {
		return false
	}
	for _, name := range bc.Agent.Tools {
		// 4 · at least one tool whose DECLARED class is parkable and whose
		// rank does not exceed the ceiling. A ceiling of write_irreversible
		// over a lone critical tool is denied by effect_ceiling and parks
		// nothing — counting that brain would publish a number the gate
		// cannot honour.
		d, ok := tool.BuiltinEffects(name)
		if !ok {
			continue
		}
		if d.Class != action.EffectWriteIrreversible && d.Class != action.EffectCritical {
			continue
		}
		if d.Class.Rank() > ceiling.Rank() {
			continue
		}
		// 5 · and governance has to let it through. The capability gate runs
		// BEFORE the effect gate, so a deny, a shadow or a channel-restricted
		// grant kills the tool without ever reaching the effect rule.
		if a.governanceAllows(bc, name) {
			return true
		}
	}
	return false
}

// governanceAllows judges one tool against the brain's grants over the
// channels the profile configures. A brain with no governance block is
// ungoverned and passes.
func (a *ApprovalsAdapter) governanceAllows(bc *config.BrainConfig, toolName string) bool {
	if len(bc.Agent.Governance) == 0 {
		return true
	}
	for _, g := range bc.Agent.Governance {
		if g.Tool != toolName {
			continue
		}
		if g.Mode != "allow" {
			return false
		}
		if len(g.Channels) == 0 {
			return true
		}
		for _, want := range g.Channels {
			for _, ch := range a.cfg.Channels {
				// A channel is identified by its TYPE in the profile
				// ("telegram", "discord", "webhook"), which is the same key
				// the grants use.
				if ch.Type == want {
					return true
				}
			}
		}
		return false
	}
	// Listed in the cage and absent from the grants: the tri-state treats an
	// ungranted tool as not advertised.
	return false
}

// resolveLawFor resolves the law ONCE for one parked request and hands back
// both halves — the cage the executor is rebuilt from and the pin the decide
// is judged under. FR-API-24: one resolution per decision, so editing the
// profile halfway cannot move a pin already captured.
func (a *ApprovalsAdapter) resolveLawFor(ctx context.Context, approvalID string) (*EffectiveCage, actionsqlite.PolicyPin, error) {
	approval, preview, err := a.store.GetApproval(ctx, approvalID)
	if err != nil {
		return nil, actionsqlite.PolicyPin{}, err
	}
	_ = approval
	const brainPrefix = "principal_brain_"
	name := preview.PrincipalID
	if len(name) > len(brainPrefix) && name[:len(brainPrefix)] == brainPrefix {
		name = name[len(brainPrefix):]
	}
	cage, pin, err := ResolveApprovalLaw(a.cfg, name)
	if err != nil {
		return nil, actionsqlite.PolicyPin{}, fmt.Errorf("%w: %w", ErrBrainNotInProfile, err)
	}
	return cage, pin, nil
}

// Approve decides and then executes, under ONE law resolution.
//
// The digest comes first and on purpose: FR-API-14 compares what the operator
// saw against what the row carries BEFORE anything is consumed, so a stale
// screen refuses without spending the approval.
func (a *ApprovalsAdapter) Approve(ctx context.Context, id, digest string) (controlapi.ApprovalOutcome, error) {
	if !a.enabled() {
		return controlapi.ApprovalOutcome{}, controlapi.ErrApprovalsDisabled
	}
	// The status comes from a read that runs NO belt, because it is what
	// decides how a belt's refusal gets named: the same corruption is
	// `evidence_corrupt` before a decision and `decided_evidence_corrupt`
	// after one. Asking GetApproval first would be circular — its refusal is
	// exactly what needs naming.
	status, err := a.store.ApprovalStatusOf(ctx, id)
	if err != nil {
		return controlapi.ApprovalOutcome{}, a.nameTouch(err, false)
	}
	sealed := status != action.ApprovalPending
	approval, _, err := a.store.GetApproval(ctx, id)
	if err != nil {
		return controlapi.ApprovalOutcome{}, a.nameTouch(err, sealed)
	}
	if !sealed && approval.ActionDigest != digest {
		return controlapi.ApprovalOutcome{}, controlapi.ErrApprovalDigestMismatch
	}
	cage, pin, err := a.resolveLawFor(ctx, id)
	if err != nil {
		if errors.Is(err, ErrBrainNotInProfile) {
			return controlapi.ApprovalOutcome{}, fmt.Errorf("%w: %w", controlapi.ErrApprovalBrainGone, err)
		}
		return controlapi.ApprovalOutcome{}, a.nameTouch(err, sealed)
	}
	opEnv, opIdent, err := approvalsOperator(id)
	if err != nil {
		return controlapi.ApprovalOutcome{}, controlapi.ErrApprovalsUnavailable
	}
	rule, err := a.store.DecideApprovalUnderLaw(ctx, id, action.DecisionApproved,
		nowUTC(), opEnv, opIdent, "", pin)
	if err != nil {
		return controlapi.ApprovalOutcome{}, a.nameTouch(err, sealed)
	}
	// The store signals IN BAND: it hands back a RULE with a nil error. An
	// adapter that read only the error would publish a receipt for a decision
	// that never happened.
	if rule != "" {
		return controlapi.ApprovalOutcome{}, nameInBandRule(rule)
	}
	return a.runApproved(ctx, id, cage, pin, approval.ActionDigest)
}

// Reject seals the no and re-reads the receipt the DTO promises.
//
// Neither DecideApprovalUnderLaw nor FinishWithResult returns the receipt
// identifiers P4 and P5 print, so the adapter reads them back — and that
// re-read is its own window (cure 25).
func (a *ApprovalsAdapter) Reject(ctx context.Context, id, comment string) (controlapi.ApprovalOutcome, error) {
	if !a.enabled() {
		return controlapi.ApprovalOutcome{}, controlapi.ErrApprovalsDisabled
	}
	if _, _, err := a.store.GetApproval(ctx, id); err != nil {
		return controlapi.ApprovalOutcome{}, a.nameTouch(err, false)
	}
	opEnv, opIdent, err := approvalsOperator(id)
	if err != nil {
		return controlapi.ApprovalOutcome{}, controlapi.ErrApprovalsUnavailable
	}
	rule, err := a.store.DecideApprovalUnderLaw(ctx, id, action.DecisionRejected,
		nowUTC(), opEnv, opIdent, comment, actionsqlite.PolicyPin{})
	if err != nil {
		return controlapi.ApprovalOutcome{}, a.nameTouch(err, false)
	}
	if rule != "" {
		return controlapi.ApprovalOutcome{}, nameInBandRule(rule)
	}
	after, _, err := a.store.GetApproval(ctx, id)
	if err != nil {
		// The decision IS sealed; what failed is reading back its identifier.
		// `params_unreadable` would open with «this execution did not start»,
		// and on the rejection path there is no execution at all — a sentence
		// nobody could have observed. The honest existing name says the record
		// could not be closed, which is exactly what happened.
		return controlapi.ApprovalOutcome{Outcome: string(controlapi.OutcomeCloseFailed)},
			fmt.Errorf("%w: %w", controlapi.ErrApprovalCloseFailed, err)
	}
	return controlapi.ApprovalOutcome{
		Outcome:   "rejected",
		ReceiptID: after.DecisionReceiptID,
	}, nil
}

// nameInBandRule turns the store's in-band rule into the outcome it means.
func nameInBandRule(rule string) error {
	switch rule {
	case action.RuleApprovalAlreadyDecided:
		return controlapi.ErrApprovalAlreadyDecided
	case action.RuleApprovalExpired:
		return controlapi.ErrApprovalExpired
	default:
		return fmt.Errorf("%w: %s", controlapi.ErrApprovalUnknownOutcome, rule)
	}
}

// nameTouch maps a store refusal on a TOUCH. `sealed` says whether a decide
// was already committed for this request, and it is the frontier: before the
// commit a driver failure means nothing was decided, after it the decision
// exists and only the effect is unknown.
func (a *ApprovalsAdapter) nameTouch(err error, sealed bool) error {
	switch {
	case errors.Is(err, actionsqlite.ErrApprovalActionNotPending):
		// The whole transaction rolled back: nothing decided, nothing run.
		// Saying «we do not know whether the effect happened» here would be
		// lying out of caution, which is still lying.
		return controlapi.ErrApprovalAlreadyClosed
	case errors.Is(err, actionsqlite.ErrApprovalInvalidated):
		return controlapi.LawMoved(a.currentLawDigest())
	case errors.Is(err, actionsqlite.ErrApprovalEvidenceCorrupt):
		if sealed {
			return controlapi.ErrApprovalDecidedEvidenceBad
		}
		return controlapi.ErrApprovalEvidenceCorrupt
	case errors.Is(err, actionsqlite.ErrApprovalParamsDigestMismatch):
		return controlapi.ErrApprovalParamsDigestMismatch
	case errors.Is(err, actionsqlite.ErrApprovalNotFound):
		return controlapi.ErrApprovalNotFound
	case errors.Is(err, actionsqlite.ErrApprovalUnreadable):
		if sealed {
			return controlapi.ErrApprovalParamsUnreadable
		}
		return controlapi.ErrApprovalsUnavailable
	default:
		if sealed {
			return controlapi.ErrApprovalDecidedEvidenceBad
		}
		return controlapi.ErrApprovalsUnavailable
	}
}

func nowUTC() time.Time { return time.Now().UTC() }

// approvalsOperator builds the desktop operator's envelope and attempt
// identity. The window is a LOOPBACK, in-process caller behind the admin
// bearer, so its provenance class says exactly that — the decision act is
// signed by a principal whose credential is what it really is, never a
// borrowed one.
func approvalsOperator(approvalID string) (action.Envelope, actionsqlite.AttemptIdentity, error) {
	registry := action.ProvenanceRegistry{
		"desktop": {Class: "console", Credential: action.CredentialLoopbackInProcess},
	}
	principal, evidence, err := action.ResolvePrincipal(registry, "desktop", "operator", nowUTC())
	if err != nil {
		return action.Envelope{}, actionsqlite.AttemptIdentity{}, err
	}
	env := action.NewEnvelope(action.NewID(), "desktop",
		action.Source{Kind: "operator", Protocol: "desktop", Channel: "desktop"},
		action.Operation{Namespace: "approval", Name: "decide", Version: 1},
		`{"approval_id":"`+approvalID+`"}`, nowUTC())
	env.Principal = action.PrincipalRef{
		PrincipalID:        principal.PrincipalID,
		EvidenceID:         evidence.EvidenceID,
		ResponsibleHumanID: principal.ResponsibleHumanID,
	}
	env.IntentID = action.RootIntentID
	return env, actionsqlite.AttemptIdentity{
		PrincipalID: principal.PrincipalID,
		IntentID:    action.RootIntentID,
		Evidence:    evidence,
	}, nil
}

// runApproved delegates to the ONE execution path — the same function the
// operator CLI runs. Two implementations of an irreversible effect is exactly
// the class this house forbids, and the previous shape had them: a cure wired
// only to the window while the CLI kept the defect.
func (a *ApprovalsAdapter) runApproved(ctx context.Context, id string, cage *EffectiveCage, pin actionsqlite.PolicyPin, digest string) (controlapi.ApprovalOutcome, error) {
	_, preview, err := a.store.GetApproval(ctx, id)
	if err != nil {
		return controlapi.ApprovalOutcome{}, a.nameTouch(err, true)
	}
	// FROM CAGE, never the resolving variant: the executor is rebuilt from the
	// cage this decision already resolved, so editing the profile halfway
	// cannot run the tool under a new cage with the old pin.
	exec, err := BuildApprovalExecutorFromCage(cage, preview)
	if err != nil {
		return controlapi.ApprovalOutcome{}, fmt.Errorf("%w: %w", controlapi.ErrApprovalBrainGone, err)
	}
	run, err := ExecuteApprovedAction(ctx, a.store, exec, id, pin)
	if err != nil {
		return controlapi.ApprovalOutcome{}, a.nameExecution(ctx, id, err)
	}
	if run.Failed {
		// The tool ran and said no. It reaches the window as the `failed`
		// outcome WITH its receipt — never as a bare 500, which is the worst
		// possible message over an irreversible effect that already left.
		return controlapi.ApprovalOutcome{
			Outcome:   "failed",
			Digest:    digest,
			Result:    run.FailureDetail,
			ReceiptID: run.ReceiptID,
		}, nil
	}
	return controlapi.ApprovalOutcome{
		Outcome:   "executed",
		Digest:    digest,
		Result:    run.Result,
		ReceiptID: run.ReceiptID,
	}, nil
}

// nameExecution names what the one execution path refused. The claim's own
// refusals go through the re-read ladder; the rest are named by sentinel.
func (a *ApprovalsAdapter) nameExecution(ctx context.Context, id string, err error) error {
	switch {
	case errors.Is(err, ErrApprovalNotDecided):
		return controlapi.ErrApprovalNotDecided
	case errors.Is(err, ErrApprovalAlreadyClosed):
		return controlapi.ErrApprovalAlreadyClosed
	case errors.Is(err, ErrApprovalCloseFailed):
		return controlapi.ErrApprovalCloseFailed
	}
	return a.nameClaim(ctx, id, err)
}

// nameClaim decides between the four «did it start?» names by RE-READING the
// row, never by inferring from the fact that our transaction rolled back — an
// inference that is false in at least two verified branches.
//
// The re-read RE-DERIVES where the answer depends on it: an empty column is
// two different facts, and only the digest tells them apart. Answering `gone`
// for both printed «the row was born without parameters, so no execution could
// have started» over a request a competitor had just claimed and was running.
func (a *ApprovalsAdapter) nameClaim(ctx context.Context, id string, err error) error {
	// A belt or a parse refusing is named by its SENTINEL, and what a sentinel
	// names is not re-read: its literal may not assert state it never looked at.
	switch {
	case errors.Is(err, actionsqlite.ErrApprovalEvidenceCorrupt),
		errors.Is(err, actionsqlite.ErrApprovalParamsDigestMismatch),
		errors.Is(err, actionsqlite.ErrApprovalInvalidated):
		return a.nameTouch(err, true)
	}
	params, state, rerr := a.store.ReReadParams(ctx, id)
	switch {
	case rerr == nil && state == actionsqlite.ParamsPresent && len(params) > 0:
		return controlapi.ErrApprovalNotStartedParamsHeld
	case rerr == nil && state == actionsqlite.ParamsEmpty:
		return controlapi.ErrApprovalNotStartedParamsGone
	case errors.Is(rerr, actionsqlite.ErrApprovalParamsUnaccounted),
		errors.Is(rerr, actionsqlite.ErrApprovalNotFound):
		return controlapi.ErrApprovalParamsUnaccounted
	default:
		return controlapi.ErrApprovalParamsUnreadable
	}
}

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
	"encoding/json"
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
		return controlapi.ApprovalList{}, a.nameRead(ctx, "", err)
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

// Detail serves one parked request.
//
// TWO reads, said plainly because the sentence that said «ONE snapshot» was
// false: resolveLawFor reads the row to find which brain parked it, and the
// store's detail door then opens its own transaction. The SECOND is the one the
// guarantee is about — state, triple, params and belts all from one snapshot,
// so a legitimate CLI rejection landing mid-read can never be painted as
// corruption. The first read only answers «whose law?», and a row that moved
// between the two changes which law is judged, never which document is served.
//
// Collapsing them into one transaction would mean the store door resolving the
// profile, which is not its job. Filed, with the promise scoped to what the
// wire does.
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
		return controlapi.ApprovalDetail{}, a.nameRead(ctx, id, err)
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
func (a *ApprovalsAdapter) nameRead(ctx context.Context, id string, err error) error {
	switch {
	case errors.Is(err, actionsqlite.ErrApprovalInvalidated):
		return controlapi.LawMoved(a.currentLawDigest(ctx, id))
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

// currentLawDigest is the CURRENT law of the brain that parked THIS request
// (v0.15.1 block B, P2-8) — never cfg.Brains[0], which published another
// brain's law for a request parked by any brain but the first. "" when the
// request or its brain cannot be resolved: an empty field, never a guess.
func (a *ApprovalsAdapter) currentLawDigest(ctx context.Context, id string) string {
	if id == "" {
		return ""
	}
	_, pin, err := a.resolveLawFor(ctx, id)
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
		return controlapi.ApprovalOutcome{}, a.nameTouch(ctx, id, err, false)
	}
	sealed := decideWasCommitted(status)
	approval, _, err := a.store.GetApproval(ctx, id)
	if err != nil {
		return controlapi.ApprovalOutcome{}, a.nameTouch(ctx, id, err, sealed)
	}
	if !sealed && approval.ActionDigest != digest {
		return controlapi.ApprovalOutcome{}, controlapi.ErrApprovalDigestMismatch
	}
	cage, pin, err := a.resolveLawFor(ctx, id)
	if err != nil {
		if errors.Is(err, ErrBrainNotInProfile) {
			return controlapi.ApprovalOutcome{}, fmt.Errorf("%w: %w", controlapi.ErrApprovalBrainGone, err)
		}
		return controlapi.ApprovalOutcome{}, a.nameTouch(ctx, id, err, sealed)
	}
	opEnv, opIdent, err := approvalsOperator(id)
	if err != nil {
		return controlapi.ApprovalOutcome{}, controlapi.ErrApprovalsUnavailable
	}
	rule, err := a.store.DecideApprovalUnderLaw(ctx, id, action.DecisionApproved,
		nowUTC(), opEnv, opIdent, "", pin)
	if err != nil {
		return controlapi.ApprovalOutcome{}, a.nameTouch(ctx, id, err, sealed)
	}
	// The store signals IN BAND: it hands back a RULE with a nil error. An
	// adapter that read only the error would publish a receipt for a decision
	// that never happened.
	if rule != "" {
		return controlapi.ApprovalOutcome{}, nameInBandRule(rule)
	}
	// The OPERATOR's digest, not the column re-read. The two are proven equal
	// three statements up, so this changes no value today — it changes the
	// provenance the code can be read to carry, which is the whole content of
	// FR-API-14. Handing `approval.ActionDigest` on meant the sentence «the
	// digest the human re-typed travels all the way there» was true only by
	// coincidence of an earlier comparison.
	return a.runApproved(ctx, id, cage, pin, digest)
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
		return controlapi.ApprovalOutcome{}, a.nameTouch(ctx, id, err, false)
	}
	opEnv, opIdent, err := approvalsOperator(id)
	if err != nil {
		return controlapi.ApprovalOutcome{}, controlapi.ErrApprovalsUnavailable
	}
	rule, err := a.store.DecideApprovalUnderLaw(ctx, id, action.DecisionRejected,
		nowUTC(), opEnv, opIdent, comment, actionsqlite.PolicyPin{})
	if err != nil {
		return controlapi.ApprovalOutcome{}, a.nameTouch(ctx, id, err, false)
	}
	if rule != "" {
		return controlapi.ApprovalOutcome{}, nameInBandRule(rule)
	}
	after, _, err := a.store.GetApproval(ctx, id)
	if err != nil {
		// The decision IS sealed; what failed is reading back its identifier.
		// Neither `params_unreadable` nor `close_failed` fits: the first opens
		// with «this execution did not start» and the second says «whether the
		// effect happened is unknown» — and on the REJECT path no execution is
		// ever attempted, so both invent a fact. The previous cure swapped one
		// fabricated sentence for another; this name says only what happened.
		return controlapi.ApprovalOutcome{}, fmt.Errorf("%w: %w", controlapi.ErrApprovalReceiptUnreadable, err)
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

// decideWasCommitted answers the ONLY question `sealed` is allowed to ask:
// did an operator's decide act commit for this request?
//
// It is a closed set, and it is judged by naming its members rather than by
// negating PENDING. `status != PENDING` reads as the same thing and is not:
// EXPIRED is written by the CLOCK at a consume touch, so under the old
// predicate a clock-closed row that then failed a belt answered
// `decided_evidence_corrupt`, whose operator literal opens «La decisión quedó
// registrada y sellada, con su recibo». There was no decision and there was no
// receipt — a fabricated fact on the screen that governs an irreversible
// effect. A status this file has never seen is NOT a decision either: the
// default is the safe half here, because claiming a receipt that may not exist
// is the damage.
//
// CANCELLED sits on the false side too, and the reason is NOT action's phrase
// «withdrawn before any decision» — that describes the intent, while the only
// writer of the status (consumeApprovalTx) records a decision act and stores
// its proof in decision_receipt_id in the same UPDATE. No production caller
// passes DecisionCancelled today, so the arm is unreachable and no mould can
// force it; whoever wires it decides which side it belongs on. Until then the
// safe half holds it.
//
// TestAdapter_sealedNamesOnlyTheTwoDecidedStatuses walks every member of
// action's status set in both directions, so a sixth status cannot be added
// without this function answering for it.
func decideWasCommitted(status action.ApprovalStatus) bool {
	switch status {
	case action.ApprovalApproved, action.ApprovalRejected:
		return true
	default:
		return false
	}
}

// nameTouch maps a store refusal on a TOUCH. `sealed` says whether a decide
// was already committed for this request — decideWasCommitted is the judge —
// and it is the frontier: before the commit a driver failure means nothing was
// decided, after it the decision exists and only the effect is unknown.
func (a *ApprovalsAdapter) nameTouch(ctx context.Context, id string, err error, sealed bool) error {
	switch {
	case errors.Is(err, actionsqlite.ErrApprovalWriteFailed):
		// A driver failure while the decide wrote the transition: the
		// transaction rolled back and nothing was decided (v0.15.1 block B,
		// P2-9). Transient, retry — never already_closed.
		return controlapi.ErrApprovalsUnavailable
	case errors.Is(err, actionsqlite.ErrApprovalActionNotPending):
		// The whole transaction rolled back: nothing decided, nothing run.
		// Saying «we do not know whether the effect happened» here would be
		// lying out of caution, which is still lying.
		return controlapi.ErrApprovalAlreadyClosed
	case errors.Is(err, actionsqlite.ErrApprovalInvalidated):
		return controlapi.LawMoved(a.currentLawDigest(ctx, id))
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

// approvalIDParams renders {"approval_id": id} through encoding/json. The
// string concatenation it replaced built a sealed document from bytes nobody
// validated. The control API's detail, approve and reject routes refuse a
// malformed id by shape before they call this adapter (P2-1); the adapter
// itself accepts any string, so this is defence in depth, declared without a
// mould (v0.15.1 block B).
func approvalIDParams(approvalID string) string {
	b, err := json.Marshal(struct {
		ApprovalID string `json:"approval_id"`
	}{approvalID})
	if err != nil {
		// Unreachable: a struct of one string always marshals.
		return `{}`
	}
	return string(b)
}

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
		approvalIDParams(approvalID), nowUTC())
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
		return controlapi.ApprovalOutcome{}, a.nameTouch(ctx, id, err, true)
	}
	// FROM CAGE, never the resolving variant: the executor is rebuilt from the
	// cage this decision already resolved, so editing the profile halfway
	// cannot run the tool under a new cage with the old pin.
	exec, err := BuildApprovalExecutorFromCage(cage, preview)
	if err != nil {
		return controlapi.ApprovalOutcome{}, fmt.Errorf("%w: %w", controlapi.ErrApprovalBrainGone, err)
	}
	run, err := ExecuteApprovedAction(ctx, a.store, exec, id, pin, digest)
	if err != nil {
		return controlapi.ApprovalOutcome{}, a.nameExecution(ctx, id, err)
	}
	if run.Unknown {
		// Every outcome ExecuteApprovedAction closes OUTCOME_UNKNOWN (the table
		// in tool.CloseStateAfterRun): a response that is not a usable success,
		// a connection obtained with no answer read, or a context that ended
		// in a tool that does not observe the wire. The effect may have left
		// and nobody can account for it. A deadline that ended BEFORE any
		// connection existed is not here: it closes FAILED.
		// It is the ONE outcome that says «we do not know», and saying it is
		// the whole reason the name exists.
		return controlapi.ApprovalOutcome{}, fmt.Errorf("%w: %s", controlapi.ErrApprovalUnknownOutcome, run.FailureDetail)
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
	case errors.Is(err, ErrApprovalRecordUnreadable):
		return controlapi.ErrApprovalDecidedEvidenceBad
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
		return a.nameTouch(ctx, id, err, true)
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
	case errors.Is(rerr, actionsqlite.ErrApprovalEvidenceCorrupt):
		// The re-read RAN and answered corrupt bytes. Letting this fall to the
		// residual said «the store could not be read» about a store that was
		// read perfectly well and told us its evidence no longer verifies.
		//
		// DEFENCE IN DEPTH, declared: through the endpoint this branch is not
		// reachable today, because the claim reads the approval row before the
		// params and catches the same corruption first. It is here so the next
		// caller — or a claim that stops reading that row — does not reopen the
		// hole, and no mould claims to cover it.
		return controlapi.ErrApprovalDecidedEvidenceBad
	case errors.Is(rerr, actionsqlite.ErrApprovalUnreadable):
		return controlapi.ErrApprovalParamsUnreadable
	default:
		// Nothing named it. That is the only honest residual, and it says it
		// does not know rather than guessing.
		return controlapi.ErrApprovalParamsUnreadable
	}
}

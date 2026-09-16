// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.15.0 — the two doors the approvals screen needs, and the typed
// sentinels that let a caller NAME what refused.
//
// Why this file exists. The screen renders BY NAME: every refusal it can
// paint has its own literal, and the one thing it may never do is degrade an
// unknown answer into an empty list or a generic shrug. That contract is only
// keepable if the store distinguishes what its names claim — so the belts stop
// refusing with a bare fmt.Errorf, sql.ErrNoRows stops sharing a return with a
// driver failure, and the claim stops answering ErrNotFound to three different
// questions.
//
// Anchors: docs/superpowers/specs/2026-09-08-approvals-screen-ux.md §11
// (FR-API-1, 19, 21, 22, 25) and §17 F rows 16-21.

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// The typed sentinels. Every one of them answers a question a caller has to
// be able to ask, and the reason they are types and not sentences is that a
// mapping by strings.Contains is a finding, not an implementation (FR-API-15).
//
// The three that mean "you will not get the params" WRAP ErrNotFound, so every
// caller written before this file keeps working unchanged while a new caller
// can tell the three apart.
var (
	// ErrApprovalNotFound is the row that is NOT THERE. It is never a
	// driver failure, and the difference is the whole point: an absent row
	// is permanent and a driver failure is worth retrying.
	ErrApprovalNotFound = fmt.Errorf("action/sqlite: approval row absent: %w", ErrNotFound)

	// ErrApprovalParamsEmpty is the row that IS there with an empty
	// parameters column — born without arguments, or purged by a close.
	// Collapsing it into "not found" is what made `gone` and `unaccounted`
	// indistinguishable.
	ErrApprovalParamsEmpty = fmt.Errorf("action/sqlite: approval parameters column empty: %w", ErrNotFound)

	// ErrApprovalClaimSkipped is the claim whose UPDATE affected NO rows.
	// It says nothing about where the params ended up: the caller re-reads.
	ErrApprovalClaimSkipped = fmt.Errorf("action/sqlite: approval claim affected no rows: %w", ErrNotFound)

	// ErrApprovalInvalidated is the law that moved under a parked request.
	// Repairable from the profile, which is why it may never share a name
	// with corruption.
	ErrApprovalInvalidated = errors.New("action/sqlite: the law moved under this approval")

	// ErrApprovalEvidenceCorrupt is any belt or evidence parse refusing.
	// Permanent: no profile repairs it.
	ErrApprovalEvidenceCorrupt = errors.New("action/sqlite: the approval's stored evidence no longer verifies")

	// ErrApprovalUnreadable is a DRIVER failure, and only that. It is the
	// single transient class of this file; everything else fails closed.
	ErrApprovalUnreadable = errors.New("action/sqlite: the approval could not be read")

	// ErrApprovalParamsDigestMismatch is the stored terna and params no
	// longer re-deriving the digest the human approved.
	ErrApprovalParamsDigestMismatch = errors.New("action/sqlite: the stored parameters no longer re-derive the approved digest")

	// ErrApprovalParamsUnaccounted is a row whose column is empty and whose
	// empty body does NOT re-derive the approved digest. It was born WITH
	// arguments and something took them, which is a different fact from a row
	// parked without any — and the two used to share a name, so a literal that
	// says «no execution could have started» was printed over a request whose
	// params a competitor had just claimed.
	ErrApprovalParamsUnaccounted = errors.New("action/sqlite: the parameters are no longer where they were")

	// ErrApprovalActionNotPending is the parked action that moved out from
	// under a decision. Nothing was decided and nothing ran — the whole
	// transaction rolls back — so its name says exactly that and not a word
	// about effects.
	ErrApprovalActionNotPending = errors.New("action/sqlite: the parked action was no longer pending")

	// ErrApprovalNoLongerApproved is a claim that found the approval or its
	// parked action out of APPROVED inside its own transaction. The claim rolls
	// back whole, so nothing was consumed and nothing was handed to an executor.
	ErrApprovalNoLongerApproved = errors.New("action/sqlite: the approval or its action was not APPROVED inside the claim")

	// ErrApprovalMovedUnderTheClaim is a claim whose transaction read an
	// approval row that differs, in any column action.Approval carries, from
	// the row the caller read before it. Two columns were checked before this;
	// the rest of that struct — the digests, the law pin, the window, the
	// decision fields — was read outside the transaction and never compared
	// inside it.
	//
	// The two columns OUTSIDE the struct, canonical_preview and
	// canonical_params, are not compared here and do not need to be: the same
	// transaction re-parses the preview, re-validates its binding and
	// re-derives the digest over the params, so a move in either fails closed
	// with its own sentinel (ErrApprovalEvidenceCorrupt,
	// ErrApprovalParamsDigestMismatch).
	ErrApprovalMovedUnderTheClaim = errors.New("action/sqlite: the approval row moved between the caller's read and the claim")
)

// ParamsState is FR-UI-16's four values. `present` is the only one that lets
// the screen offer the yes.
type ParamsState string

const (
	// ParamsPresent is a readable body that re-derives the approved digest.
	ParamsPresent ParamsState = "present"
	// ParamsEmpty is a row parked WITHOUT arguments. It is not a purge: the
	// digest re-derives over the empty body, which is how the two are told
	// apart.
	ParamsEmpty ParamsState = "empty"
	// ParamsUnavailable is a body that could not be read at this instant.
	ParamsUnavailable ParamsState = "unavailable"
	// ParamsTooLarge is a body past the screen's bound.
	ParamsTooLarge ParamsState = "too_large"
)

// approvalColumns is the row shape scanApproval expects, in its order. Named
// once so the two doors below cannot drift from it.
const approvalColumns = `approval_id, schema_version, action_id, action_digest, preview_digest,
	requested_from, reason, risk_summary, policy_version, policy_digest,
	requested_at, expires_at, status, decision_principal_id, decision,
	decision_at, comment, decision_receipt_id`

// ApprovalListRow is one row of the LIST door: the approval, the preview as
// STORED, and the channel the preview seals.
//
// PreviewReadable false means the preview did not parse. The row still comes
// out — with its identifier and its digest — because a row the operator cannot
// read is exactly the row he most needs to see (FR-API-1).
//
// SCOPE, as of 2026-09-13: both flags are computed here and NO production
// consumer reads either. internal/app's adapter copies Preview.Operation and
// Channel straight onto the wire, so a corrupt preview and an empty channel
// reach the screen as the same empty fields — the exact conflation the
// ChannelKnown comment used to say this struct prevents. The screen does print
// «SIN CLASE LEGIBLE» for an empty effect class, but it INFERS that from
// another column instead of reading the fact this door already established,
// and the `origin` column has no such banner at all. Carrying the flags to the
// wire is filed for v0.15.1 with its reproduction; the claim they already do
// is not shipped.
type ApprovalListRow struct {
	Approval        action.Approval
	Preview         action.ActionPreview
	PreviewReadable bool
	// Channel is the origin, read from the preview's sealed resources set.
	Channel string
	// ChannelKnown distinguishes a channel that IS empty from one that could
	// not be determined — at THIS door. Nothing downstream consumes it yet
	// (see the scope note on this type).
	ChannelKnown bool
}

// SkippedApproval is a row the list could not serve whole, named so the
// operator learns it exists instead of losing it in silence.
type SkippedApproval struct {
	ApprovalID string
	Reason     string
}

// ApprovalListing is what the LIST door answers.
type ApprovalListing struct {
	Rows    []ApprovalListRow
	Skipped []SkippedApproval
}

// ListPendingApprovals is the LIST door (§17 F row 19). It reads and it does
// NOT run the verification belt: the belt is the DETAIL's job, and running it
// here would make a row whose story no longer verifies disappear instead of
// being refused where the operator can read why.
//
// One row that cannot be scanned does NOT take the listing down. It is
// skipped, counted and named — the old `return nil, err` let a single mutated
// timestamp erase the healthy rows too.
func (s *Store) ListPendingApprovals(ctx context.Context, limit int) (ApprovalListing, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+approvalColumns+`, canonical_preview
		   FROM approvals WHERE status = ?
		  ORDER BY requested_at DESC LIMIT ?`,
		string(action.ApprovalPending), limit)
	if err != nil {
		return ApprovalListing{}, fmt.Errorf("action/sqlite: list pending approvals: %w: %w", ErrApprovalUnreadable, err)
	}
	defer func() { _ = rows.Close() }()

	var out ApprovalListing
	for rows.Next() {
		a, rawPreview, id, err := scanApprovalAndPreview(rows)
		if err != nil {
			out.Skipped = append(out.Skipped, SkippedApproval{ApprovalID: id, Reason: err.Error()})
			continue
		}
		row := ApprovalListRow{Approval: a}
		if p, perr := action.ParseCanonicalPreview([]byte(rawPreview)); perr == nil {
			row.Preview = p
			row.PreviewReadable = true
			row.Channel, row.ChannelKnown = channelOf(p)
		}
		out.Rows = append(out.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return ApprovalListing{}, fmt.Errorf("action/sqlite: iterate pending approvals: %w: %w", ErrApprovalUnreadable, err)
	}
	return out, nil
}

// channelOf reads the origin out of the preview's sealed resources set.
//
// The set is SORTED by the canonical form, so an index is only meaningful when
// there is exactly one element — which is what the factory writes. Any other
// shape is a preview an external hand rewrote: the channel is not known, and
// saying so is not the same as saying it is empty.
func channelOf(p action.ActionPreview) (string, bool) {
	if len(p.Resources) != 1 {
		return "", false
	}
	return p.Resources[0], true
}

// scanApprovalAndPreview scans the list's row shape. Unlike scanApproval it
// returns the approval id even when a later column fails to parse, because a
// row that cannot be served still has to be NAMED.
func scanApprovalAndPreview(row scanner) (action.Approval, string, string, error) {
	var (
		a           action.Approval
		status      string
		requestedAt string
		expiresAt   sql.NullString
		decisionAt  sql.NullString
		rawPreview  string
	)
	if err := row.Scan(&a.ApprovalID, &a.SchemaVersion, &a.ActionID, &a.ActionDigest,
		&a.PreviewDigest, &a.RequestedFrom, &a.Reason, &a.RiskSummary,
		&a.PolicyVersion, &a.PolicyDigest, &requestedAt, &expiresAt, &status,
		&a.DecisionPrincipalID, &a.Decision, &decisionAt, &a.Comment,
		&a.DecisionReceiptID, &rawPreview); err != nil {
		return action.Approval{}, "", "", err
	}
	a.Status = action.ApprovalStatus(status)
	var err error
	if a.RequestedAt, err = time.Parse(time.RFC3339Nano, requestedAt); err != nil {
		return a, rawPreview, a.ApprovalID, fmt.Errorf("requested_at does not parse: %w", err)
	}
	if a.ExpiresAt, err = parseNullTime(expiresAt); err != nil {
		return a, rawPreview, a.ApprovalID, fmt.Errorf("expires_at does not parse: %w", err)
	}
	if a.DecisionAt, err = parseNullTime(decisionAt); err != nil {
		return a, rawPreview, a.ApprovalID, fmt.Errorf("decision_at does not parse: %w", err)
	}
	return a, rawPreview, a.ApprovalID, nil
}

// maxDetailParamsBytes is the bound past which the screen refuses to show a
// body whole. Beyond it the document cannot be read in one sitting, so it does
// not offer the yes either.
const maxDetailParamsBytes = 64 << 10

// ApprovalDetailRow is the DETAIL door's answer: the state, the TERNA, the
// params and the belts, all from ONE snapshot.
type ApprovalDetailRow struct {
	Approval action.Approval
	Preview  action.ActionPreview
	// Operation is the terna from the ACTIONS row — namespace, name and
	// VERSION. The canonical preview stores only "ns/name", so without this
	// the digest cannot be re-derived at all.
	Operation   action.Operation
	Params      []byte
	ParamsState ParamsState
	ActionState action.State
}

// ApprovalDetail reads one parked request in a SINGLE transaction (§17 F row
// 20, FR-API-22).
//
// What the single transaction promises is CONSISTENCY, not freshness. SQLite in
// WAL hands the transaction a snapshot taken at its first read, so a commit
// that lands halfway is not seen — and that is exactly what is wanted: the old
// four loose reads let a legitimate CLI rejection land between two of them, and
// the belt then saw an emptied column beside a digest read before it and
// painted "this is not transient, look at the book" over a CORRECT decision.
//
// A row whose status is not PENDING comes back whole and unjudged: the belts do
// not run on it, because FR-API-18's precedence by state wins over any belt's
// name and the caller is the one that applies it.
func (s *Store) ApprovalDetail(ctx context.Context, approvalID string) (ApprovalDetailRow, error) {
	return s.approvalDetail(ctx, approvalID, nil)
}

// ApprovalDetailUnderLaw is ApprovalDetail with the law judged over the row the
// transaction read.
func (s *Store) ApprovalDetailUnderLaw(ctx context.Context, approvalID string, law PolicyPin) (ApprovalDetailRow, error) {
	return s.approvalDetail(ctx, approvalID, &law)
}

func (s *Store) approvalDetail(ctx context.Context, approvalID string, law *PolicyPin) (ApprovalDetailRow, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ApprovalDetailRow{}, fmt.Errorf("action/sqlite: begin approval detail: %w: %w", ErrApprovalUnreadable, err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		rawPreview string
		rawParams  string
	)
	row := tx.QueryRowContext(ctx,
		`SELECT `+approvalColumns+`, canonical_preview, canonical_params
		   FROM approvals WHERE approval_id = ?`, approvalID)
	a, rawPreview, rawParams, err := scanApprovalDetail(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ApprovalDetailRow{}, fmt.Errorf("action/sqlite: approval %q: %w", approvalID, ErrApprovalNotFound)
	}
	if err != nil {
		// A column that will not parse is corruption of the evidence, not a
		// disk that did not answer. Sending it to the transient residual is
		// how a permanent fault gets told "retry".
		return ApprovalDetailRow{}, fmt.Errorf("action/sqlite: approval %q: %w: %w", approvalID, ErrApprovalEvidenceCorrupt, err)
	}

	out := ApprovalDetailRow{Approval: a, Params: []byte(rawParams)}
	if a.Status != action.ApprovalPending {
		// Precedence by state (FR-API-18) belongs to the caller: a decided
		// row is served unjudged rather than refused by a belt that would
		// name the purge instead of the decision.
		return out, nil
	}

	p, err := action.ParseCanonicalPreview([]byte(rawPreview))
	if err != nil {
		return ApprovalDetailRow{}, fmt.Errorf("action/sqlite: approval %q preview: %w: %w", approvalID, ErrApprovalEvidenceCorrupt, err)
	}
	out.Preview = p
	if err := action.ValidatePreviewBinding(a, p); err != nil {
		return ApprovalDetailRow{}, fmt.Errorf("action/sqlite: approval %q: %w: %w", approvalID, ErrApprovalEvidenceCorrupt, err)
	}
	if err := verifyApprovalStoryTyped(ctx, tx, a, p); err != nil {
		return ApprovalDetailRow{}, fmt.Errorf("action/sqlite: approval %q: %w", approvalID, err)
	}
	if law != nil {
		if rule, dim := action.ValidateApprovalBinding(a, a.ActionDigest, law.Version, law.Digest); rule != "" {
			return ApprovalDetailRow{}, fmt.Errorf(
				"action/sqlite: approval %q was parked under law v%d %s but the current law is v%d %s (%s): %w",
				approvalID, a.PolicyVersion, a.PolicyDigest, law.Version, law.Digest, dim, ErrApprovalInvalidated)
		}
	}

	// The terna and the params from the SAME snapshot. The join is LEFT on
	// purpose: an inner one would turn the orphan row into "does not exist",
	// the fail-open R15 already caught.
	op, state, err := ternaOf(ctx, tx, a.ActionID)
	if err != nil {
		return ApprovalDetailRow{}, fmt.Errorf("action/sqlite: approval %q: %w", approvalID, err)
	}
	out.Operation = op
	out.ActionState = state

	// The belt runs BEFORE the classification, so a mismatch precedes `empty`
	// and `too_large` — FR-API-19. And it runs over the terna, not the
	// preview: op_version is in the digest and the preview does not store it.
	if got := action.Digest(op, rawParams); got != a.ActionDigest {
		return ApprovalDetailRow{}, fmt.Errorf(
			"action/sqlite: approval %q: stored parameters re-derive %s but the approved digest is %s: %w",
			approvalID, got, a.ActionDigest, ErrApprovalParamsDigestMismatch)
	}
	switch {
	case rawParams == "":
		out.ParamsState = ParamsEmpty
	case len(rawParams) > maxDetailParamsBytes:
		out.ParamsState = ParamsTooLarge
	default:
		out.ParamsState = ParamsPresent
	}
	if err := tx.Commit(); err != nil {
		return ApprovalDetailRow{}, fmt.Errorf("action/sqlite: commit approval detail: %w: %w", ErrApprovalUnreadable, err)
	}
	return out, nil
}

func scanApprovalDetail(row scanner) (action.Approval, string, string, error) {
	var (
		a           action.Approval
		status      string
		requestedAt string
		expiresAt   sql.NullString
		decisionAt  sql.NullString
		rawPreview  string
		rawParams   string
	)
	if err := row.Scan(&a.ApprovalID, &a.SchemaVersion, &a.ActionID, &a.ActionDigest,
		&a.PreviewDigest, &a.RequestedFrom, &a.Reason, &a.RiskSummary,
		&a.PolicyVersion, &a.PolicyDigest, &requestedAt, &expiresAt, &status,
		&a.DecisionPrincipalID, &a.Decision, &decisionAt, &a.Comment,
		&a.DecisionReceiptID, &rawPreview, &rawParams); err != nil {
		return action.Approval{}, "", "", err
	}
	a.Status = action.ApprovalStatus(status)
	var err error
	if a.RequestedAt, err = time.Parse(time.RFC3339Nano, requestedAt); err != nil {
		return action.Approval{}, "", "", fmt.Errorf("parse approval requested_at: %w", err)
	}
	if a.ExpiresAt, err = parseNullTime(expiresAt); err != nil {
		return action.Approval{}, "", "", fmt.Errorf("parse approval expires_at: %w", err)
	}
	if a.DecisionAt, err = parseNullTime(decisionAt); err != nil {
		return action.Approval{}, "", "", fmt.Errorf("parse approval decision_at: %w", err)
	}
	return a, rawPreview, rawParams, nil
}

// ternaOf brings the operation triple and the action's state through a LEFT
// JOIN, so an approvals row whose actions row was destroyed reads as CORRUPT
// and never as absent.
func ternaOf(ctx context.Context, q rowQuerier, actionID string) (action.Operation, action.State, error) {
	var (
		ns      sql.NullString
		name    sql.NullString
		version sql.NullInt64
		state   sql.NullString
	)
	err := q.QueryRowContext(ctx,
		`SELECT a.op_namespace, a.op_name, a.op_version, a.state
		   FROM actions a WHERE a.action_id = ?`, actionID).
		Scan(&ns, &name, &version, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return action.Operation{}, "", fmt.Errorf("the actions row for %q is gone: %w", actionID, ErrApprovalEvidenceCorrupt)
	}
	if err != nil {
		return action.Operation{}, "", fmt.Errorf("read the actions row for %q: %w: %w", actionID, ErrApprovalUnreadable, err)
	}
	if !ns.Valid || !name.Valid || !version.Valid {
		return action.Operation{}, "", fmt.Errorf("the actions row for %q is incomplete: %w", actionID, ErrApprovalEvidenceCorrupt)
	}
	return action.Operation{
		Namespace: ns.String, Name: name.String, Version: int(version.Int64),
	}, action.State(state.String), nil
}

// verifyApprovalStoryTyped is verifyApprovalStory with its refusals NAMED.
//
// The old one wrapped the absent row and the driver failure in the same
// return, so a DESTROYED row and a disk that did not answer left by one door
// and the caller had to guess. Here sql.ErrNoRows is corruption — the row is
// gone and nothing repairs it — and only a driver error is transient.
func verifyApprovalStoryTyped(ctx context.Context, q rowQuerier, a action.Approval, p action.ActionPreview) error {
	var (
		effectClass, opNS, opName string
		principal                 sql.NullString
	)
	err := q.QueryRowContext(ctx,
		`SELECT effect_class, op_namespace, op_name, principal_id
		   FROM actions WHERE action_id = ?`, a.ActionID).
		Scan(&effectClass, &opNS, &opName, &principal)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("the actions row for %q is gone: %w", a.ActionID, ErrApprovalEvidenceCorrupt)
	}
	if err != nil {
		return fmt.Errorf("read the story of %q: %w: %w", a.ActionID, ErrApprovalUnreadable, err)
	}
	if effectClass != string(p.EffectClass) {
		return fmt.Errorf("preview_effect_mismatch: the preview shows %s but the action row carries %s: %w", p.EffectClass, effectClass, ErrApprovalEvidenceCorrupt)
	}
	if op := opNS + "/" + opName; op != p.Operation {
		return fmt.Errorf("preview_operation_mismatch: the preview shows %s but the action row carries %s: %w", p.Operation, op, ErrApprovalEvidenceCorrupt)
	}
	if principal.String != p.PrincipalID {
		return fmt.Errorf("preview_principal_mismatch: the preview shows %q but the action row carries %q: %w", p.PrincipalID, principal.String, ErrApprovalEvidenceCorrupt)
	}
	var (
		outcome, rule, polDigest string
		polVersion               int64
	)
	err = q.QueryRowContext(ctx,
		`SELECT outcome, rule, policy_version, policy_digest
		   FROM action_decisions WHERE action_id = ?`, a.ActionID).
		Scan(&outcome, &rule, &polVersion, &polDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("the decision row for %q is gone: %w", a.ActionID, ErrApprovalEvidenceCorrupt)
	}
	if err != nil {
		return fmt.Errorf("read the decision of %q: %w: %w", a.ActionID, ErrApprovalUnreadable, err)
	}
	if outcome != a.Reason || rule != a.Reason {
		return fmt.Errorf("decision_outcome_mismatch: the request was born from %q but the decision row says outcome %q rule %q: %w", a.Reason, outcome, rule, ErrApprovalEvidenceCorrupt)
	}
	if polVersion != a.PolicyVersion || polDigest != a.PolicyDigest {
		return fmt.Errorf("decision_policy_mismatch: the request pinned law v%d %s but the decision row carries v%d %s: %w", a.PolicyVersion, a.PolicyDigest, polVersion, polDigest, ErrApprovalEvidenceCorrupt)
	}
	return nil
}

// Path is the file this store was opened on. The approvals moulds need it to
// open a SECOND real connection and attack from outside the component, which
// is what the cross-verification law asks for.
func (s *Store) Path() string { return s.path }

// ClaimApprovalParamsUnderDigest is the execution claim with the belt INSIDE
// the claiming transaction.
//
// Why it exists. The old path read the operation triple with store.Get, then
// claimed (committing the consume and emptying the column), and only THEN
// compared the digest using the triple from that earlier read. An external
// UPDATE of op_version in that window passed every belt and executed an
// irreversible effect under an operation the row no longer declared — the
// comparison judged a RECOMPUTED value while a stored one sat right there.
//
// Here the triple, the params and the digest all come from the row THIS
// transaction read, and a mismatch refuses by name with nothing consumed.
// It returns the OPERATION it judged alongside the params, so the caller runs
// the triple this transaction read and not one it fetched somewhere else. That
// is the whole cure: handing back only the params would leave the caller free
// to execute under a stale operation, which is the defect this function exists
// to close.
// approvalRowMoved names the FIRST column in which two reads of one approval
// row differ, or "" when they are the same row. Times are compared by instant,
// not by representation.
//
// It exists so the claim can judge every column action.Approval carries rather
// than the two an external review happened to name: a comparison that
// enumerates what to check is a list, and the next column added to the struct
// is outside it. This one has to be extended when action.Approval grows, and
// TestApprovalRowMoved_coversEveryColumn is the mould that makes that failure
// loud instead of silent.
//
// It does NOT cover the two columns the struct does not carry —
// canonical_preview and canonical_params — because the claim's own belts
// re-parse and re-derive them inside the same transaction.
func approvalRowMoved(seen, now action.Approval) string {
	switch {
	case seen.ApprovalID != now.ApprovalID:
		return "approval_id"
	case seen.SchemaVersion != now.SchemaVersion:
		return "schema_version"
	case seen.ActionID != now.ActionID:
		return "action_id"
	case seen.ActionDigest != now.ActionDigest:
		return "action_digest"
	case seen.PreviewDigest != now.PreviewDigest:
		return "preview_digest"
	case seen.RequestedFrom != now.RequestedFrom:
		return "requested_from"
	case seen.Reason != now.Reason:
		return "reason"
	case seen.RiskSummary != now.RiskSummary:
		return "risk_summary"
	case seen.PolicyVersion != now.PolicyVersion:
		return "policy_version"
	case seen.PolicyDigest != now.PolicyDigest:
		return "policy_digest"
	case !seen.RequestedAt.Equal(now.RequestedAt):
		return "requested_at"
	case !seen.ExpiresAt.Equal(now.ExpiresAt):
		return "expires_at"
	case seen.Status != now.Status:
		return "status"
	case seen.DecisionPrincipalID != now.DecisionPrincipalID:
		return "decision_principal_id"
	case seen.Decision != now.Decision:
		return "decision"
	case !seen.DecisionAt.Equal(now.DecisionAt):
		return "decision_at"
	case seen.Comment != now.Comment:
		return "comment"
	case seen.DecisionReceiptID != now.DecisionReceiptID:
		return "decision_receipt_id"
	}
	return ""
}

func (s *Store) ClaimApprovalParamsUnderDigest(ctx context.Context, approvalID string, law *PolicyPin, wantDigest string, seen *action.Approval) ([]byte, action.Operation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, action.Operation{}, fmt.Errorf("action/sqlite: begin claim: %w: %w", ErrApprovalUnreadable, err)
	}
	defer func() { _ = tx.Rollback() }()

	a, err := s.approvalTx(ctx, tx, approvalID)
	if errors.Is(err, ErrApprovalNotFound) {
		// Propagated, not re-mapped: re-wrapping here would make the door's own
		// sentinel decorative — any door could go back to the generic error and
		// nothing would notice.
		return nil, action.Operation{}, err
	}
	if err != nil {
		return nil, action.Operation{}, fmt.Errorf("action/sqlite: approval %q: %w: %w", approvalID, ErrApprovalEvidenceCorrupt, err)
	}
	if law != nil {
		if rule, dim := action.ValidateApprovalBinding(a, a.ActionDigest, law.Version, law.Digest); rule != "" {
			return nil, action.Operation{}, fmt.Errorf(
				"action/sqlite: approval %q was parked under law v%d %s but the current law is v%d %s (%s): %w",
				approvalID, a.PolicyVersion, a.PolicyDigest, law.Version, law.Digest, dim, ErrApprovalInvalidated)
		}
	}
	// The snapshot the caller read, against the row THIS transaction read. The
	// caller's prechecks happen outside this transaction, so every column they
	// judged is a stale read until it is compared here.
	if seen != nil {
		if column := approvalRowMoved(*seen, a); column != "" {
			return nil, action.Operation{}, fmt.Errorf(
				"action/sqlite: approval %q moved under the claim (%s): %w",
				approvalID, column, ErrApprovalMovedUnderTheClaim)
		}
	}
	var rawPreview, rawParams string
	if err := tx.QueryRowContext(ctx,
		`SELECT canonical_preview, canonical_params FROM approvals WHERE approval_id = ?`,
		approvalID).Scan(&rawPreview, &rawParams); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, action.Operation{}, fmt.Errorf("action/sqlite: approval %q: %w", approvalID, ErrApprovalNotFound)
		}
		return nil, action.Operation{}, fmt.Errorf("action/sqlite: claim read %q: %w: %w", approvalID, ErrApprovalUnreadable, err)
	}
	p, err := action.ParseCanonicalPreview([]byte(rawPreview))
	if err != nil {
		return nil, action.Operation{}, fmt.Errorf("action/sqlite: approval %q preview: %w: %w", approvalID, ErrApprovalEvidenceCorrupt, err)
	}
	if err := action.ValidatePreviewBinding(a, p); err != nil {
		return nil, action.Operation{}, fmt.Errorf("action/sqlite: approval %q: %w: %w", approvalID, ErrApprovalEvidenceCorrupt, err)
	}
	if err := verifyApprovalStoryTyped(ctx, tx, a, p); err != nil {
		return nil, action.Operation{}, fmt.Errorf("action/sqlite: approval %q: %w", approvalID, err)
	}

	op, _, err := ternaOf(ctx, tx, a.ActionID)
	if err != nil {
		return nil, action.Operation{}, fmt.Errorf("action/sqlite: approval %q: %w", approvalID, err)
	}
	// The EMPTY column is judged FIRST, and the order is the whole point. An
	// empty body almost never re-derives a digest taken over a non-empty one,
	// so running the belt first swallowed every emptied row into
	// `params_digest_mismatch` — whose literal says «this is permanent and
	// nothing was executed» while the competitor that emptied it may be inside
	// its own run. Emptiness is a fact about the ROW; the mismatch is a fact
	// about the BYTES, and only the first one can be told apart by re-reading.
	if rawParams == "" {
		return nil, action.Operation{}, fmt.Errorf("action/sqlite: approval %q: %w", approvalID, ErrApprovalParamsEmpty)
	}
	if got := action.Digest(op, rawParams); got != wantDigest {
		return nil, action.Operation{}, fmt.Errorf(
			"action/sqlite: approval %q: the row this transaction read re-derives %s but the caller approved %s: %w",
			approvalID, got, wantDigest, ErrApprovalParamsDigestMismatch)
	}

	// The purge is conditioned on the AUTHORITY, not only on the parameters
	// still being there. ExecuteApprovedAction checks approval=APPROVED and
	// action=APPROVED before this transaction opens; a state that moved after
	// those prechecks used to be consumed here and handed to the executor.
	res, err := tx.ExecContext(ctx,
		`UPDATE approvals SET canonical_params = ''
		  WHERE approval_id = ? AND canonical_params != '' AND status = ?
		    AND EXISTS (SELECT 1 FROM actions WHERE action_id = ? AND state = ?)`,
		approvalID, string(action.ApprovalApproved), a.ActionID, string(action.StateApproved))
	if err != nil {
		return nil, action.Operation{}, fmt.Errorf("action/sqlite: purge claim %q: %w: %w", approvalID, ErrApprovalUnreadable, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		// A count that was never obtained is not a count of zero. Reading it
		// as "already claimed" would name an outcome over a fact nobody has.
		return nil, action.Operation{}, fmt.Errorf("action/sqlite: claim %q: rows affected unavailable: %w: %w", approvalID, ErrApprovalUnreadable, err)
	}
	// The authority is re-read AFTER the purge and before the commit, in this
	// transaction. The conditioned WHERE only sees the state as it stood when
	// the purge evaluated it; a trigger on the purge itself can move it later
	// in the same statement, and only this read observes that. It also names
	// the zero-row purge: a moved authority is not «already claimed».
	if err := claimAuthorityTx(ctx, tx, approvalID, a.ActionID); err != nil {
		return nil, action.Operation{}, err
	}
	if n == 0 {
		return nil, action.Operation{}, fmt.Errorf("action/sqlite: approval %q: %w", approvalID, ErrApprovalClaimSkipped)
	}
	if err := tx.Commit(); err != nil {
		return nil, action.Operation{}, fmt.Errorf("action/sqlite: commit claim: %w: %w", ErrApprovalUnreadable, err)
	}
	return []byte(rawParams), op, nil
}

// claimAuthorityTx refuses, by name, a claim whose approval or parked action is
// not APPROVED as its own transaction sees them. An unreadable row fails
// closed with its own name; it is never read as a moved state.
func claimAuthorityTx(ctx context.Context, tx *sql.Tx, approvalID, actionID string) error {
	var status string
	err := tx.QueryRowContext(ctx,
		`SELECT status FROM approvals WHERE approval_id = ?`, approvalID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("action/sqlite: approval %q is gone inside its claim: %w", approvalID, ErrApprovalEvidenceCorrupt)
	}
	if err != nil {
		return fmt.Errorf("action/sqlite: claim authority %q: %w: %w", approvalID, ErrApprovalUnreadable, err)
	}
	_, state, err := ternaOf(ctx, tx, actionID)
	if err != nil {
		return fmt.Errorf("action/sqlite: approval %q: %w", approvalID, err)
	}
	if status != string(action.ApprovalApproved) || state != action.StateApproved {
		return fmt.Errorf("action/sqlite: approval %q is %s and its action is %s inside the claim: %w",
			approvalID, status, state, ErrApprovalNoLongerApproved)
	}
	return nil
}

// ApprovalStatusOf reads one column and runs NO belt.
//
// A caller has to know whether a decide was already committed BEFORE it can
// name what a belt refused: the same corruption is `evidence_corrupt` on a
// pending row and `decided_evidence_corrupt` once a decision is sealed. Asking
// GetApproval would be circular — it is the call whose refusal we are trying
// to name.
func (s *Store) ApprovalStatusOf(ctx context.Context, approvalID string) (action.ApprovalStatus, error) {
	var status string
	err := s.db.QueryRowContext(ctx,
		`SELECT status FROM approvals WHERE approval_id = ?`, approvalID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("action/sqlite: approval %q: %w", approvalID, ErrApprovalNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("action/sqlite: approval status %q: %w: %w", approvalID, ErrApprovalUnreadable, err)
	}
	return action.ApprovalStatus(status), nil
}

// ReReadParams is the eje-2 re-read: it looks at the row AGAIN and answers what
// it finds NOW, re-deriving where the answer depends on it.
//
// It exists because «the column is empty» is two different facts. A row parked
// WITHOUT arguments re-derives its digest over the empty body — nothing was
// ever there to take. A row parked WITH arguments whose column is now empty
// does not — something took them. Collapsing the two let a literal that says
// «the row was born without parameters, so no execution could have started»
// be printed over a request a competitor had just claimed and was executing.
//
// What it does NOT do is claim exclusivity: the store is multi-process and a
// reader here can prove nothing about who else holds what.
func (s *Store) ReReadParams(ctx context.Context, approvalID string) ([]byte, ParamsState, error) {
	var rawParams string
	err := s.db.QueryRowContext(ctx,
		`SELECT canonical_params FROM approvals WHERE approval_id = ?`, approvalID).Scan(&rawParams)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", fmt.Errorf("action/sqlite: approval %q: %w", approvalID, ErrApprovalNotFound)
	}
	if err != nil {
		return nil, "", fmt.Errorf("action/sqlite: re-read params %q: %w: %w", approvalID, ErrApprovalUnreadable, err)
	}
	if rawParams != "" {
		return []byte(rawParams), ParamsPresent, nil
	}
	a, err := s.approvalTx0(ctx, approvalID)
	if err != nil {
		return nil, "", err
	}
	op, _, err := ternaOf(ctx, s.db, a.ActionID)
	if err != nil {
		return nil, "", fmt.Errorf("action/sqlite: approval %q: %w", approvalID, err)
	}
	if action.Digest(op, "") == a.ActionDigest {
		// Born without arguments: the empty body IS what was approved.
		return nil, ParamsEmpty, nil
	}
	return nil, "", fmt.Errorf("action/sqlite: approval %q: %w", approvalID, ErrApprovalParamsUnaccounted)
}

// approvalTx0 reads the approval row outside any transaction, for the re-read.
func (s *Store) approvalTx0(ctx context.Context, approvalID string) (action.Approval, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+approvalColumns+` FROM approvals WHERE approval_id = ?`, approvalID)
	a, err := scanApproval(row)
	if errors.Is(err, sql.ErrNoRows) {
		return action.Approval{}, fmt.Errorf("action/sqlite: approval %q: %w", approvalID, ErrApprovalNotFound)
	}
	if err != nil {
		return action.Approval{}, fmt.Errorf("action/sqlite: approval %q: %w: %w", approvalID, ErrApprovalEvidenceCorrupt, err)
	}
	return a, nil
}

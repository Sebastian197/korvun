// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The persisted approval (Trust Layer Etapa 5, lote 2 — spec FR-APR,
// FR-STATE-2, sealed NC-2): the request is BORN WHOLE — the action
// parks PENDING_APPROVAL with its decision, its §10.8 approval row,
// its sealed preview and its canonical parameters in ONE transaction,
// integrity-pinned (the recovered params must re-derive the EXACT
// digest the human approves, or the request does not come to exist).
// The decision is one-shot atomic: the single PENDING->decided UPDATE
// is the consumption point, and the operator's decision act leaves its
// own E4 signed receipt in the SAME transaction as the state
// transition — together or nothing. Expiry is judged at the consume
// touch (the E2 clock mold): no sweeper, no goroutine. There are NO
// update or delete paths beyond the one transition.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// ErrUnknownDecision reports a decision verb outside the finite set.
var ErrUnknownDecision = errors.New("action/sqlite: unknown decision verb")

// maxApprovalParamsBytes caps the raw params a request may park (C6,
// the resource-bound invariant): 64 KiB holds any reasonable tool call
// and refuses the unbounded blob.
const maxApprovalParamsBytes = 64 << 10

// CreateApprovalRequest parks env as PENDING_APPROVAL and persists the
// approval request, its sealed preview and its canonical parameters in
// one transaction. The integrity pin runs BEFORE anything lands: the
// canonical params must re-derive env's exact digest, and the approval
// must bind that same digest — otherwise the request does not born.
func (s *Store) CreateApprovalRequest(ctx context.Context, env action.Envelope, d Decision, a action.Approval, p action.ActionPreview, rawParams string) error {
	// C6: the resource-bound invariant at the door — parked params are
	// the one user-driven blob this table holds; cap them at birth.
	if len(rawParams) > maxApprovalParamsBytes {
		return fmt.Errorf("action/sqlite: approval request %q: params of %d bytes exceed the %d-byte cap — an approval request is a decision record, not a payload store", a.ApprovalID, len(rawParams), maxApprovalParamsBytes)
	}
	canonical := string(action.CanonicalParams(rawParams))
	if got := action.Digest(env.Operation, rawParams); got != env.ParametersDigest {
		return fmt.Errorf("action/sqlite: approval request %q: params do not derive the envelope digest (%s vs %s)", a.ApprovalID, got, env.ParametersDigest)
	}
	if a.ActionDigest != env.ParametersDigest {
		return fmt.Errorf("action/sqlite: approval request %q: approval binds %s, envelope is %s", a.ApprovalID, a.ActionDigest, env.ParametersDigest)
	}
	if a.ActionID != env.ActionID || p.ActionID != env.ActionID {
		return fmt.Errorf("action/sqlite: approval request %q: id mismatch across request parts", a.ApprovalID)
	}
	// R1: born-whole includes the CROSS LINKS — the approval, its
	// preview and the gate decision must tell ONE story at birth.
	if err := action.ValidatePreviewBinding(a, p); err != nil {
		return fmt.Errorf("action/sqlite: approval request %q: %w", a.ApprovalID, err)
	}
	if d.Rule != a.Reason {
		return fmt.Errorf("action/sqlite: approval request %q: preview_rule_mismatch: the gate decided by rule %q but the request claims %q", a.ApprovalID, d.Rule, a.Reason)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("action/sqlite: begin approval request: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO actions (action_id, schema_version, correlation_id,
		    source_kind, source_protocol, source_channel,
		    op_namespace, op_name, op_version,
		    parameters_digest, effect_class, state, requested_at,
		    principal_id, intent_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		env.ActionID, env.SchemaVersion, env.CorrelationID,
		env.Source.Kind, env.Source.Protocol, env.Source.Channel,
		env.Operation.Namespace, env.Operation.Name, env.Operation.Version,
		env.ParametersDigest, env.Effect.Class, string(action.StatePendingApproval),
		env.RequestedAt.UTC().Format(time.RFC3339Nano),
		nullable(env.Principal.PrincipalID), nullable(env.IntentID),
	); err != nil {
		return fmt.Errorf("action/sqlite: park action %q: %w", env.ActionID, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO action_decisions (action_id, outcome, rule, decided_at, policy_version, policy_digest)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		env.ActionID, d.Outcome, d.Rule, env.RequestedAt.UTC().Format(time.RFC3339Nano),
		d.PolicyVersion, d.PolicyDigest,
	); err != nil {
		return fmt.Errorf("action/sqlite: insert decision %q: %w", env.ActionID, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO approvals (approval_id, schema_version, action_id, action_digest,
		    preview_digest, canonical_preview, canonical_params,
		    requested_from, reason, risk_summary, policy_version, policy_digest,
		    requested_at, expires_at, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ApprovalID, a.SchemaVersion, a.ActionID, a.ActionDigest,
		a.PreviewDigest, string(action.CanonicalPreview(p)), canonical,
		a.RequestedFrom, a.Reason, a.RiskSummary, a.PolicyVersion, a.PolicyDigest,
		a.RequestedAt.UTC().Format(time.RFC3339Nano), timeCol(a.ExpiresAt), string(action.ApprovalPending),
	); err != nil {
		return fmt.Errorf("action/sqlite: insert approval %q: %w", a.ApprovalID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("action/sqlite: commit approval request: %w", err)
	}
	return nil
}

// DecideApproval consumes the approval ONE-SHOT and, in the SAME
// transaction, transitions the parked action and records the
// operator's decision act with its E4 receipt — together or nothing.
// decision ∈ {"approved", "rejected", "cancelled"} (finite, fail
// closed). Returned rules: "" on success; approval_expired when the
// touch lands past the window (the approval closes EXPIRED and the
// action REJECTED, receipted); approval_already_decided when another
// operator won the race.
// DecideApprovalUnderLaw is the ONLY exported decision surface (E5
// consolidation C1): an approve consumes authority, so it must happen
// under the SAME law that parked the request — the caller passes the
// CURRENT pin (app.PolicyPinFor) and a moved law refuses by name,
// mutating nothing: the request stays PENDING for an explicit human
// act. reject and cancelled stay safe under ANY law (withdrawing
// authority never needs one), so the pin is not consulted for them.
func (s *Store) DecideApprovalUnderLaw(ctx context.Context, approvalID, decision string, at time.Time, operatorEnv action.Envelope, ident AttemptIdentity, comment string, law PolicyPin) (string, error) {
	if decision == "approved" {
		a, _, err := s.GetApproval(ctx, approvalID)
		if err != nil {
			return "", err
		}
		// F1: the CLOCK is consulted FIRST — an expired or already-
		// decided request falls through to the mechanics, whose touch
		// closes it by the E2 mold (FR-APR-4: no zombie PENDING rows).
		// Only a still-consumable approve must happen under the law.
		if rule := action.ApprovalConsumableAt(a, at); rule == "" {
			if rule, dim := action.ValidateApprovalBinding(a, a.ActionDigest, law.Version, law.Digest); rule != "" {
				return "", fmt.Errorf("action/sqlite: %s (%s): the request was parked under law v%d %s but the current law is v%d %s — nothing was decided; re-request under the current law or reject this one", rule, dim, a.PolicyVersion, a.PolicyDigest, law.Version, law.Digest)
			}
		}
	}
	return s.decideApproval(ctx, approvalID, decision, at, operatorEnv, ident, comment)
}

// decideApproval is the decision mechanics: the one-shot consume, the
// operator's proof-of-decision act, the parked action's transition.
// Unexported on purpose — the law-validating surface above is the only
// door out of the package.
func (s *Store) decideApproval(ctx context.Context, approvalID, decision string, at time.Time, operatorEnv action.Envelope, ident AttemptIdentity, comment string) (string, error) {
	switch decision {
	case "approved", "rejected", "cancelled":
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownDecision, decision)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("action/sqlite: begin decision: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	a, err := s.approvalTx(ctx, tx, approvalID)
	if err != nil {
		return "", err
	}
	// C2+R1: the DECISION touch re-verifies the sealed preview and its
	// WHOLE cross binding inside the same transaction — no decision is
	// recorded over a preview that lies about the digest, the law, the
	// args or the rule.
	var rawPreview string
	if err := tx.QueryRowContext(ctx,
		`SELECT canonical_preview FROM approvals WHERE approval_id = ?`, approvalID).Scan(&rawPreview); err != nil {
		return "", fmt.Errorf("action/sqlite: approval preview %q: %w", approvalID, err)
	}
	if p, err := action.ParseCanonicalPreview([]byte(rawPreview)); err != nil {
		return "", fmt.Errorf("action/sqlite: approval preview %q: %w", approvalID, err)
	} else if err := action.ValidatePreviewBinding(a, p); err != nil {
		return "", fmt.Errorf("action/sqlite: approval %q: %w; refusing the decision", approvalID, err)
	}
	if rule := action.ApprovalConsumableAt(a, at); rule != "" {
		if rule == action.RuleApprovalAlreadyDecided {
			return rule, nil
		}
		// approval_expired: the touch closes it — EXPIRED approval,
		// REJECTED action with its terminal receipt, params purged.
		if err := s.closeApprovalTx(ctx, tx, a, action.ApprovalExpired, "", "clock", at, "", ""); err != nil {
			return "", err
		}
		if err := s.rejectParkedActionTx(ctx, tx, a.ActionID, at); err != nil {
			return "", err
		}
		if err := tx.Commit(); err != nil {
			return "", fmt.Errorf("action/sqlite: commit expiry close: %w", err)
		}
		return action.RuleApprovalExpired, nil
	}

	// The operator's decision act: a real action of its own, terminal
	// SUCCEEDED, with its signed receipt — the §10.8 proof of decision.
	proofID, err := s.recordDecisionActTx(ctx, tx, operatorEnv, ident, decision, at)
	if err != nil {
		return "", err
	}

	// ONE-SHOT consumption: the single PENDING->decided UPDATE. Zero
	// rows means another operator won between our read and here.
	status := action.ApprovalRejected
	if decision == "approved" {
		status = action.ApprovalApproved
	} else if decision == "cancelled" {
		status = action.ApprovalCancelled
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE approvals SET status = ?, decision_principal_id = ?, decision = ?,
		        decision_at = ?, comment = ?, decision_receipt_id = ?
		  WHERE approval_id = ? AND status = ?`,
		string(status), ident.PrincipalID, decision,
		at.UTC().Format(time.RFC3339Nano), comment, proofID,
		approvalID, string(action.ApprovalPending))
	if err != nil {
		return "", fmt.Errorf("action/sqlite: consume approval %q: %w", approvalID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return action.RuleApprovalAlreadyDecided, nil
	}
	if decision == "approved" {
		if err := transitionTx(ctx, tx, a.ActionID, action.StatePendingApproval, action.StateApproved); err != nil {
			return "", err
		}
	} else {
		if err := s.rejectParkedActionTx(ctx, tx, a.ActionID, at); err != nil {
			return "", err
		}
		// A close without execution purges the raw canonical params.
		if _, err := tx.ExecContext(ctx,
			`UPDATE approvals SET canonical_params = '' WHERE approval_id = ?`, approvalID); err != nil {
			return "", fmt.Errorf("action/sqlite: purge params %q: %w", approvalID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("action/sqlite: commit decision: %w", err)
	}
	return "", nil
}

// approvalTx loads one approval inside the transaction.
func (s *Store) approvalTx(ctx context.Context, tx *sql.Tx, approvalID string) (action.Approval, error) {
	row := tx.QueryRowContext(ctx,
		`SELECT approval_id, schema_version, action_id, action_digest, preview_digest,
		        requested_from, reason, risk_summary, policy_version, policy_digest,
		        requested_at, expires_at, status, decision_principal_id, decision,
		        decision_at, comment, decision_receipt_id
		   FROM approvals WHERE approval_id = ?`, approvalID)
	a, err := scanApproval(row)
	if errors.Is(err, sql.ErrNoRows) {
		return action.Approval{}, fmt.Errorf("action/sqlite: approval %q: %w", approvalID, ErrNotFound)
	}
	return a, err
}

// closeApprovalTx closes an approval into a terminal status (the
// expiry path; the normal decision path closes via the one-shot
// UPDATE). Params are purged here too: an expired request never runs.
func (s *Store) closeApprovalTx(ctx context.Context, tx *sql.Tx, a action.Approval, status action.ApprovalStatus, decider, decision string, at time.Time, comment, proofID string) error {
	if _, err := tx.ExecContext(ctx,
		`UPDATE approvals SET status = ?, decision_principal_id = ?, decision = ?,
		        decision_at = ?, comment = ?, decision_receipt_id = ?,
		        canonical_params = ''
		  WHERE approval_id = ? AND status = ?`,
		string(status), decider, decision, at.UTC().Format(time.RFC3339Nano),
		comment, proofID, a.ApprovalID, string(action.ApprovalPending)); err != nil {
		return fmt.Errorf("action/sqlite: close approval %q: %w", a.ApprovalID, err)
	}
	return nil
}

// rejectParkedActionTx moves the parked action to its terminal
// REJECTED and births its receipt in the same transaction (the E4 law:
// every terminal outcome receipts or the whole close fails).
func (s *Store) rejectParkedActionTx(ctx context.Context, tx *sql.Tx, actionID string, at time.Time) error {
	if err := transitionTx(ctx, tx, actionID, action.StatePendingApproval, action.StateRejected); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE actions SET finished_at = ? WHERE action_id = ?`,
		at.UTC().Format(time.RFC3339Nano), actionID); err != nil {
		return fmt.Errorf("action/sqlite: stamp finish %q: %w", actionID, err)
	}
	r, err := s.receiptForFinish(ctx, tx, actionID, action.StateRejected, at, "")
	if err != nil {
		return err
	}
	return s.appendReceiptTx(ctx, tx, r)
}

// recordDecisionActTx records the operator's decision act — a real
// action, terminal SUCCEEDED, receipted — inside the shared
// transaction, and returns the receipt id (the proof of decision).
func (s *Store) recordDecisionActTx(ctx context.Context, tx *sql.Tx, env action.Envelope, ident AttemptIdentity, decision string, at time.Time) (string, error) {
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO actions (action_id, schema_version, correlation_id,
		    source_kind, source_protocol, source_channel,
		    op_namespace, op_name, op_version,
		    parameters_digest, effect_class, state, requested_at, finished_at,
		    principal_id, intent_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		env.ActionID, env.SchemaVersion, env.CorrelationID,
		env.Source.Kind, env.Source.Protocol, env.Source.Channel,
		env.Operation.Namespace, env.Operation.Name, env.Operation.Version,
		env.ParametersDigest, env.Effect.Class, string(action.StateSucceeded),
		at.UTC().Format(time.RFC3339Nano), at.UTC().Format(time.RFC3339Nano),
		nullable(ident.PrincipalID), nullable(ident.IntentID),
	); err != nil {
		return "", fmt.Errorf("action/sqlite: record decision act %q: %w", env.ActionID, err)
	}
	d := Decision{Outcome: "allow", Rule: "operator"}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO action_decisions (action_id, outcome, rule, decided_at, policy_version, policy_digest)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		env.ActionID, d.Outcome, d.Rule, at.UTC().Format(time.RFC3339Nano),
		d.PolicyVersion, d.PolicyDigest,
	); err != nil {
		return "", fmt.Errorf("action/sqlite: decision-act decision %q: %w", env.ActionID, err)
	}
	r := s.receiptForRecord(ctx, tx, env, d, action.StateSucceeded)
	if err := s.appendReceiptTx(ctx, tx, r); err != nil {
		return "", err
	}
	return r.ReceiptID, nil
}

// transitionTx validates the domain edge and applies the single-state
// UPDATE — the only mutation path the approvals flow owns.
func transitionTx(ctx context.Context, tx *sql.Tx, actionID string, from, to action.State) error {
	if err := action.Transition(from, to); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE actions SET state = ? WHERE action_id = ? AND state = ?`,
		string(to), actionID, string(from))
	if err != nil {
		return fmt.Errorf("action/sqlite: transition %q %s->%s: %w", actionID, from, to, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("action/sqlite: transition %q %s->%s: row not in expected state", actionID, from, to)
	}
	return nil
}

// ApprovalParams returns the parked request's canonical parameters —
// present only while the request awaits execution; purged (ErrNotFound)
// at any close without execution.
func (s *Store) ApprovalParams(ctx context.Context, approvalID string) ([]byte, error) {
	var params string
	err := s.db.QueryRowContext(ctx,
		`SELECT canonical_params FROM approvals WHERE approval_id = ?`, approvalID).Scan(&params)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("action/sqlite: approval %q: %w", approvalID, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("action/sqlite: approval params %q: %w", approvalID, err)
	}
	if params == "" {
		return nil, fmt.Errorf("action/sqlite: approval %q params purged: %w", approvalID, ErrNotFound)
	}
	return []byte(params), nil
}

// GetApproval returns one approval and its sealed preview.
func (s *Store) GetApproval(ctx context.Context, approvalID string) (action.Approval, action.ActionPreview, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT approval_id, schema_version, action_id, action_digest, preview_digest,
		        requested_from, reason, risk_summary, policy_version, policy_digest,
		        requested_at, expires_at, status, decision_principal_id, decision,
		        decision_at, comment, decision_receipt_id
		   FROM approvals WHERE approval_id = ?`, approvalID)
	a, err := scanApproval(row)
	if errors.Is(err, sql.ErrNoRows) {
		return action.Approval{}, action.ActionPreview{}, fmt.Errorf("action/sqlite: approval %q: %w", approvalID, ErrNotFound)
	}
	if err != nil {
		return action.Approval{}, action.ActionPreview{}, err
	}
	var rawPreview string
	if err := s.db.QueryRowContext(ctx,
		`SELECT canonical_preview FROM approvals WHERE approval_id = ?`, approvalID).Scan(&rawPreview); err != nil {
		return action.Approval{}, action.ActionPreview{}, fmt.Errorf("action/sqlite: approval preview %q: %w", approvalID, err)
	}
	p, err := action.ParseCanonicalPreview([]byte(rawPreview))
	if err != nil {
		return action.Approval{}, action.ActionPreview{}, fmt.Errorf("action/sqlite: approval preview %q: %w", approvalID, err)
	}
	// C2: the sealed preview must still re-derive the pinned digest —
	// a stored preview that parses fine but tells a different story is
	// refused at the READ, by name.
	if got := p.Digest(); got != a.PreviewDigest {
		return action.Approval{}, action.ActionPreview{}, fmt.Errorf(
			"action/sqlite: approval %q: preview_digest_mismatch — the stored preview re-derives %s but the request pinned %s", approvalID, got, a.PreviewDigest)
	}
	return a, p, nil
}

// ListApprovals returns every approval in one status, newest first.
func (s *Store) ListApprovals(ctx context.Context, status action.ApprovalStatus) ([]action.Approval, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT approval_id, schema_version, action_id, action_digest, preview_digest,
		        requested_from, reason, risk_summary, policy_version, policy_digest,
		        requested_at, expires_at, status, decision_principal_id, decision,
		        decision_at, comment, decision_receipt_id
		   FROM approvals WHERE status = ? ORDER BY requested_at DESC`, string(status))
	if err != nil {
		return nil, fmt.Errorf("action/sqlite: list approvals: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []action.Approval
	for rows.Next() {
		a, err := scanApproval(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("action/sqlite: iterate approvals: %w", err)
	}
	return out, nil
}

// scanner covers *sql.Row and *sql.Rows.
type scanner interface{ Scan(dest ...any) error }

// scanApproval shares the row shape across every approval read.
func scanApproval(row scanner) (action.Approval, error) {
	var (
		a           action.Approval
		status      string
		requestedAt string
		expiresAt   sql.NullString
		decisionAt  sql.NullString
	)
	if err := row.Scan(&a.ApprovalID, &a.SchemaVersion, &a.ActionID, &a.ActionDigest,
		&a.PreviewDigest, &a.RequestedFrom, &a.Reason, &a.RiskSummary,
		&a.PolicyVersion, &a.PolicyDigest, &requestedAt, &expiresAt, &status,
		&a.DecisionPrincipalID, &a.Decision, &decisionAt, &a.Comment,
		&a.DecisionReceiptID); err != nil {
		return action.Approval{}, err
	}
	a.Status = action.ApprovalStatus(status)
	var err error
	if a.RequestedAt, err = time.Parse(time.RFC3339Nano, requestedAt); err != nil {
		return action.Approval{}, fmt.Errorf("action/sqlite: parse approval requested_at: %w", err)
	}
	if a.ExpiresAt, err = parseNullTime(expiresAt); err != nil {
		return action.Approval{}, fmt.Errorf("action/sqlite: parse approval expires_at: %w", err)
	}
	if a.DecisionAt, err = parseNullTime(decisionAt); err != nil {
		return action.Approval{}, fmt.Errorf("action/sqlite: parse approval decision_at: %w", err)
	}
	return a, nil
}

// timeCol renders a nullable time column ("" stays NULL).
func timeCol(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// nullable renders an optional identity column ("" stays NULL, the
// E2 SQL-NULL honesty for identity-less rows).
func nullable(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// ClaimApprovalParams atomically reads AND purges the stored canonical
// params — the execution claim (Etapa 5 FR-EXEC): exactly one caller
// wins the params; every later caller gets ErrNotFound. The claim
// happens BEFORE the effect, so racing executors cannot fire twice; a
// crash between claim and terminal close leaves a non-terminal action
// the E1 recovery pass closes honestly on the next lifecycle open.
func (s *Store) ClaimApprovalParams(ctx context.Context, approvalID string) ([]byte, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("action/sqlite: begin claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var params string
	err = tx.QueryRowContext(ctx,
		`SELECT canonical_params FROM approvals WHERE approval_id = ?`, approvalID).Scan(&params)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("action/sqlite: approval %q: %w", approvalID, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("action/sqlite: claim params %q: %w", approvalID, err)
	}
	if params == "" {
		return nil, fmt.Errorf("action/sqlite: approval %q already claimed or closed: %w", approvalID, ErrNotFound)
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE approvals SET canonical_params = '' WHERE approval_id = ? AND canonical_params != ''`, approvalID)
	if err != nil {
		return nil, fmt.Errorf("action/sqlite: purge claim %q: %w", approvalID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, fmt.Errorf("action/sqlite: approval %q already claimed: %w", approvalID, ErrNotFound)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("action/sqlite: commit claim: %w", err)
	}
	return []byte(params), nil
}

// GetApprovalByAction returns the approval bound to one action (the
// verifier's approval-coherence lookup).
func (s *Store) GetApprovalByAction(ctx context.Context, actionID string) (action.Approval, action.ActionPreview, error) {
	var approvalID string
	err := s.db.QueryRowContext(ctx,
		`SELECT approval_id FROM approvals WHERE action_id = ?`, actionID).Scan(&approvalID)
	if errors.Is(err, sql.ErrNoRows) {
		return action.Approval{}, action.ActionPreview{}, fmt.Errorf("action/sqlite: approval for action %q: %w", actionID, ErrNotFound)
	}
	if err != nil {
		return action.Approval{}, action.ActionPreview{}, fmt.Errorf("action/sqlite: approval for action %q: %w", actionID, err)
	}
	return s.GetApproval(ctx, approvalID)
}

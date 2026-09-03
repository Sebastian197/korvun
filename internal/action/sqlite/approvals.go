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
// touch (the E2 clock mold) AND swept server-side (R4: at boot and on
// the prune cadence — synchronous passes, no goroutine), each close
// owning its row atomically: the one-shot PENDING->decided UPDATE is
// the ownership test, and losing a clean race is a skip, never an
// error. There are NO other update or delete paths.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// ErrUnknownDecision reports a decision verb outside the finite set.
var ErrUnknownDecision = errors.New("action/sqlite: unknown decision verb")

// maxApprovalParamsBytes caps the raw params a request may park (C6,
// the resource-bound invariant): 64 KiB holds any reasonable tool call
// and refuses the unbounded blob.
const maxApprovalParamsBytes = 64 << 10

// CreateApprovalRequest is the ONLY exported birth door (R4 Phase 2,
// FR-R4F2-2): it accepts the born-whole bundle the action-package
// factory derived — four loose narrated structures can no longer
// enter. The R1 cross-link belt below stays as in-store defense in
// depth behind the factory.
func (s *Store) CreateApprovalRequest(ctx context.Context, b action.BoundApprovalRequest) error {
	a := b.Approval()
	return s.createApprovalParts(ctx, b.Envelope(),
		Decision{Outcome: a.Reason, Rule: a.Reason,
			PolicyVersion: a.PolicyVersion, PolicyDigest: a.PolicyDigest},
		a, b.Preview(), b.RawParams())
}

// createApprovalParts is the birth mechanics behind the bundle door:
// park the action PENDING_APPROVAL with its decision, approval row,
// sealed preview and canonical params in one transaction, the whole
// R1 cross-link belt enforced by name. Unexported on purpose — the
// factory-validated bundle is the only way in from outside.
func (s *Store) createApprovalParts(ctx context.Context, env action.Envelope, d Decision, a action.Approval, p action.ActionPreview, rawParams string) error {
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
	// F3: the park itself pays the retention cadence — a server whose
	// only traffic is parking requests still sweeps its expired ones.
	return s.noteWrite(ctx)
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
	// R4-F2 (FR-R4F2-4): the non-atomic pre-read died — the law is
	// judged inside decideApprovalWithLaw's transaction over the
	// RE-READ row, after the clock (F1: expiry wins the cross).
	if decision == "approved" {
		return s.decideApprovalWithLaw(ctx, approvalID, decision, at, operatorEnv, ident, comment, &law)
	}
	return s.decideApprovalWithLaw(ctx, approvalID, decision, at, operatorEnv, ident, comment, nil)
}

// decideApproval keeps the law-less mechanics signature for the
// package's own tests.
func (s *Store) decideApproval(ctx context.Context, approvalID, decision string, at time.Time, operatorEnv action.Envelope, ident AttemptIdentity, comment string) (string, error) {
	return s.decideApprovalWithLaw(ctx, approvalID, decision, at, operatorEnv, ident, comment, nil)
}

// decideApprovalWithLaw is the decision mechanics: the one-shot
// consume, the operator's proof-of-decision act, the parked action's
// transition — and, for an approve, the law judged INSIDE this same
// transaction over the re-read row (R4-F2). Unexported on purpose.
func (s *Store) decideApprovalWithLaw(ctx context.Context, approvalID, decision string, at time.Time, operatorEnv action.Envelope, ident AttemptIdentity, comment string, law *PolicyPin) (string, error) {
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
	} else if err := s.verifyApprovalStory(ctx, tx, a, p); err != nil {
		// R5-S2: the WHOLE story judged inside THIS transaction — a
		// saboteur moving actions/action_decisions between any earlier
		// read and this consume refuses by name, approval intact.
		return "", fmt.Errorf("action/sqlite: approval %q: %w; refusing the decision", approvalID, err)
	}
	if rule := action.ApprovalConsumableAt(a, at); rule != "" {
		if rule == action.RuleApprovalAlreadyDecided {
			return rule, nil
		}
		// approval_expired: the touch closes it — EXPIRED approval,
		// REJECTED action with its terminal receipt, params purged.
		// R4-F3: a lost race here means someone else already decided.
		won, err := s.closeApprovalTx(ctx, tx, a, action.ApprovalExpired, "", "clock", at, "", "")
		if err != nil {
			return "", err
		}
		if !won {
			return action.RuleApprovalAlreadyDecided, nil
		}
		if err := s.rejectParkedActionTx(ctx, tx, a.ActionID, at); err != nil {
			return "", err
		}
		if err := tx.Commit(); err != nil {
			return "", fmt.Errorf("action/sqlite: commit expiry close: %w", err)
		}
		return action.RuleApprovalExpired, nil
	}
	// R4-F2: the law, judged over THE ROW THIS TRANSACTION READ — a
	// law change between any earlier read and this consume is caught
	// here, atomically.
	if law != nil {
		if rule, dim := action.ValidateApprovalBinding(a, a.ActionDigest, law.Version, law.Digest); rule != "" {
			return "", fmt.Errorf("action/sqlite: %s (%s): the request was parked under law v%d %s but the current law is v%d %s — nothing was decided; re-request under the current law or reject this one", rule, dim, a.PolicyVersion, a.PolicyDigest, law.Version, law.Digest)
		}
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
	// R6-X2: the decided preimage rides in the SAME transaction.
	if err := s.tombstoneTx(ctx, tx, a, ident.PrincipalID, decision, at); err != nil {
		return "", err
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
// R4-F3: the one-shot WHERE status='PENDING' is the ownership test —
// the return says whether THIS transaction won the transition; a loser
// mutated nothing and must not receipt anything.
func (s *Store) closeApprovalTx(ctx context.Context, tx *sql.Tx, a action.Approval, status action.ApprovalStatus, decider, decision string, at time.Time, comment, proofID string) (bool, error) {
	res, err := tx.ExecContext(ctx,
		`UPDATE approvals SET status = ?, decision_principal_id = ?, decision = ?,
		        decision_at = ?, comment = ?, decision_receipt_id = ?,
		        canonical_params = ''
		  WHERE approval_id = ? AND status = ?`,
		string(status), decider, decision, at.UTC().Format(time.RFC3339Nano),
		comment, proofID, a.ApprovalID, string(action.ApprovalPending))
	if err != nil {
		return false, fmt.Errorf("action/sqlite: close approval %q: %w", a.ApprovalID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("action/sqlite: close approval %q: %w", a.ApprovalID, err)
	}
	if n > 0 {
		// R6-X2: the winner writes the decided preimage in its tx.
		if err := s.tombstoneTx(ctx, tx, a, decider, decision, at); err != nil {
			return false, err
		}
	}
	return n > 0, nil
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

// tombstoneTx writes the decided approval's digest PREIMAGE (R6-X2)
// inside the closing transaction: the bounded scalar facts — who
// decided what, when, under which law, over which preview — that
// reconstruct and prove the sealed ApprovalDigest after retention
// takes the rows. Idempotent per action (INSERT OR REPLACE: the
// preimage of a one-shot decision never changes).
func (s *Store) tombstoneTx(ctx context.Context, tx *sql.Tx, a action.Approval, decider, decision string, at time.Time) error {
	sealed := a
	sealed.DecisionPrincipalID = decider
	sealed.Decision = decision
	sealed.DecisionAt = at.UTC()
	_, err := tx.ExecContext(ctx,
		`INSERT INTO approval_tombstones
		    (approval_id, approval_digest, action_id, action_digest, preview_digest,
		     policy_version, policy_digest, decision_principal_id, decision, decision_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ApprovalID, sealed.Digest(), a.ActionID, a.ActionDigest, a.PreviewDigest,
		a.PolicyVersion, a.PolicyDigest, decider, decision,
		at.UTC().Format(time.RFC3339Nano))
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "UNIQUE constraint failed") {
		// R7-Y2: history is immutable. An IDENTICAL row is harmless
		// idempotence; anything else is a named conflict.
		existing, gerr := s.approvalTombstoneRowTx(ctx, tx, a.ApprovalID)
		if gerr == nil && existing.Digest() == sealed.Digest() {
			return nil
		}
		return fmt.Errorf("action/sqlite: tombstone_conflict: approval %q already has an immutable tombstone with a different story — history is never rewritten", a.ApprovalID)
	}
	return fmt.Errorf("action/sqlite: tombstone for %q: %w", a.ActionID, err)
}

// approvalTombstoneRowTx reads one tombstone by approval id in-tx.
func (s *Store) approvalTombstoneRowTx(ctx context.Context, tx *sql.Tx, approvalID string) (action.Approval, error) {
	return scanTombstone(tx.QueryRowContext(ctx,
		`SELECT approval_id, action_id, action_digest, preview_digest, policy_version,
		        policy_digest, decision_principal_id, decision, decision_at
		   FROM approval_tombstones WHERE approval_id = ?`, approvalID))
}

// scanTombstone scans one tombstone row into its preimage.
func scanTombstone(row *sql.Row) (action.Approval, error) {
	var a action.Approval
	var decisionAt sql.NullString
	err := row.Scan(&a.ApprovalID, &a.ActionID, &a.ActionDigest, &a.PreviewDigest,
		&a.PolicyVersion, &a.PolicyDigest, &a.DecisionPrincipalID,
		&a.Decision, &decisionAt)
	if err != nil {
		return action.Approval{}, err
	}
	if t, err := parseNullTime(decisionAt); err == nil {
		a.DecisionAt = t
	}
	return a, nil
}

// ApprovalTombstone reconstructs the decided approval's digest
// preimage for one action (R6-X2; with reuse, the LATEST decision).
// ErrNotFound when no decided close ever wrote one.
func (s *Store) ApprovalTombstone(ctx context.Context, actionID string) (action.Approval, error) {
	a, err := scanTombstone(s.db.QueryRowContext(ctx,
		`SELECT approval_id, action_id, action_digest, preview_digest, policy_version,
		        policy_digest, decision_principal_id, decision, decision_at
		   FROM approval_tombstones WHERE action_id = ?
		   ORDER BY decision_at DESC LIMIT 1`, actionID))
	if errors.Is(err, sql.ErrNoRows) {
		return action.Approval{}, fmt.Errorf("action/sqlite: tombstone of %q: %w", actionID, ErrNotFound)
	}
	if err != nil {
		return action.Approval{}, fmt.Errorf("action/sqlite: tombstone of %q: %w", actionID, err)
	}
	return a, nil
}

// ApprovalTombstoneByDigest reconstructs by the APPROVAL's own sealed
// identity (R7-Y2) — the verifier's natural lookup: each receipt
// reconstructs ITS story even when an action_id was reused.
func (s *Store) ApprovalTombstoneByDigest(ctx context.Context, digest string) (action.Approval, error) {
	a, err := scanTombstone(s.db.QueryRowContext(ctx,
		`SELECT approval_id, action_id, action_digest, preview_digest, policy_version,
		        policy_digest, decision_principal_id, decision, decision_at
		   FROM approval_tombstones WHERE approval_digest = ?`, digest))
	if errors.Is(err, sql.ErrNoRows) {
		return action.Approval{}, fmt.Errorf("action/sqlite: tombstone with digest %q: %w", digest, ErrNotFound)
	}
	if err != nil {
		return action.Approval{}, fmt.Errorf("action/sqlite: tombstone with digest %q: %w", digest, err)
	}
	return a, nil
}

// SweepExpiredApprovals closes every PENDING approval whose window is
// past at the injected instant (R4): EXPIRED approval, REJECTED action
// WITH its signed receipt, params purged — the same close the consume
// touch performs, now server-owned so an untouched expired request
// cannot outlive its window forever. Runs at boot (after the sealer is
// wired) and on the existing prune cadence. Returns how many swept
// and how many were skipped (decided concurrently — R4-F3).
//
// Retention (R4 Phase 4, ADR-0046 — SUPERSEDES the earlier exemption):
// approval rows CASCADE with their action's retention prune (real FK,
// ON DELETE CASCADE since v9). The surviving evidence of a consumed
// approval is its SIGNED receipt — the sealed approval_digest carries
// the decision terms, the preview digest and the law pin. Growth is
// therefore bounded by the actions cap itself: at most one approval
// per surviving action row.
func (s *Store) SweepExpiredApprovals(ctx context.Context, at time.Time) (swept, skipped int, err error) {
	ids, err := s.collectIDs(ctx,
		`SELECT approval_id FROM approvals
		  WHERE status = ? AND expires_at IS NOT NULL AND expires_at != ''
		    AND expires_at <= ?`,
		string(action.ApprovalPending), at.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, 0, fmt.Errorf("action/sqlite: expiry sweep: %w", err)
	}
	for _, id := range ids {
		// R4-F3: real errors (a dead context included) abort; a row an
		// operator decided between the SELECT and this close is SKIPPED
		// with a note — never a boot-fatal for losing a clean race.
		if err := ctx.Err(); err != nil {
			return swept, skipped, fmt.Errorf("action/sqlite: expiry sweep: %w", err)
		}
		won, err := s.sweepExpiredOne(ctx, id, at)
		if err != nil {
			if isBusyClass(err) {
				// R6-X4 (found by the two-connection wild form): a busy
				// row is another connection's clean win — skip with the
				// note, never a boot-fatal; the next cadence owns it.
				skipped++
				continue
			}
			return swept, skipped, err
		}
		if won {
			swept++
		} else {
			skipped++
		}
	}
	return swept, skipped, nil
}

// sweepExpiredOne closes ONE expired approval in its own transaction.
// Ownership is decided by closeApprovalTx's one-shot UPDATE (R4-F3):
// only the winner rejects the action, purges params and receipts; a
// loser rolls back untouched and reports won=false.
func (s *Store) sweepExpiredOne(ctx context.Context, approvalID string, at time.Time) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("action/sqlite: begin expiry sweep %q: %w", approvalID, err)
	}
	defer func() { _ = tx.Rollback() }()
	a, err := s.approvalTx(ctx, tx, approvalID)
	if err != nil {
		return false, err
	}
	won, err := s.closeApprovalTx(ctx, tx, a, action.ApprovalExpired, "", "clock", at, "", "")
	if err != nil {
		return false, err
	}
	if !won {
		return false, nil
	}
	if err := s.rejectParkedActionTx(ctx, tx, a.ActionID, at); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("action/sqlite: commit expiry sweep %q: %w", approvalID, err)
	}
	return true, nil
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
	// C2+F1: the READ runs the WHOLE binding, like the decision — a
	// stored preview that parses fine but lies about the digest, the
	// law, the args or the rule is refused by name at the read; the
	// human never reads a lie.
	if err := action.ValidatePreviewBinding(a, p); err != nil {
		return action.Approval{}, action.ActionPreview{}, fmt.Errorf("action/sqlite: approval %q: %w", approvalID, err)
	}
	// R4-F2 (FR-R4F2-3): the read also re-verifies the persisted STORY
	// against the actions row and the action_decisions row — a preview
	// whose effect, operation or principal no longer match the action,
	// or a decision whose outcome/rule or law no longer match the
	// request, refuses BY NAME.
	if err := s.verifyApprovalStory(ctx, s.db, a, p); err != nil {
		return action.Approval{}, action.ActionPreview{}, fmt.Errorf("action/sqlite: approval %q: %w", approvalID, err)
	}
	return a, p, nil
}

// rowQuerier is the shared seam: *sql.DB and *sql.Tx both satisfy it,
// so the STORY check runs identically at the read and INSIDE the
// consuming transactions (R5-S2).
type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// verifyApprovalStory checks the approval+preview pair against the
// actions and action_decisions rows they claim to describe (R4-F2;
// R5-S2 runs it INSIDE the decide and claim transactions too, over
// the rows THOSE transactions read).
func (s *Store) verifyApprovalStory(ctx context.Context, q rowQuerier, a action.Approval, p action.ActionPreview) error {
	var (
		effectClass, opNS, opName string
		principal                 sql.NullString
	)
	err := q.QueryRowContext(ctx,
		`SELECT effect_class, op_namespace, op_name, principal_id
		   FROM actions WHERE action_id = ?`, a.ActionID).
		Scan(&effectClass, &opNS, &opName, &principal)
	if err != nil {
		return fmt.Errorf("action/sqlite: story of %q: %w", a.ActionID, err)
	}
	if effectClass != string(p.EffectClass) {
		return fmt.Errorf("preview_effect_mismatch: the preview shows %s but the action row carries %s", p.EffectClass, effectClass)
	}
	if op := opNS + "/" + opName; op != p.Operation {
		return fmt.Errorf("preview_operation_mismatch: the preview shows %s but the action row carries %s", p.Operation, op)
	}
	if principal.String != p.PrincipalID {
		return fmt.Errorf("preview_principal_mismatch: the preview shows %q but the action row carries %q", p.PrincipalID, principal.String)
	}
	var (
		outcome, rule, polDigest string
		polVersion               int64
	)
	err = q.QueryRowContext(ctx,
		`SELECT outcome, rule, policy_version, policy_digest
		   FROM action_decisions WHERE action_id = ?`, a.ActionID).
		Scan(&outcome, &rule, &polVersion, &polDigest)
	if err != nil {
		return fmt.Errorf("action/sqlite: decision of %q: %w", a.ActionID, err)
	}
	if outcome != a.Reason || rule != a.Reason {
		return fmt.Errorf("decision_outcome_mismatch: the request was born from %q but the decision row says outcome %q rule %q", a.Reason, outcome, rule)
	}
	if polVersion != a.PolicyVersion || polDigest != a.PolicyDigest {
		return fmt.Errorf("decision_policy_mismatch: the request pinned law v%d %s but the decision row carries v%d %s", a.PolicyVersion, a.PolicyDigest, polVersion, polDigest)
	}
	return nil
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
func (s *Store) ClaimApprovalParams(ctx context.Context, approvalID string, law *PolicyPin) ([]byte, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("action/sqlite: begin claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// R4-F2 (FR-R4F2-4): the claim is execute's consume point — the law
	// is judged HERE, inside the claiming transaction, over the re-read
	// row. nil law skips (package-internal mechanics and tests).
	if law != nil {
		a, err := s.approvalTx(ctx, tx, approvalID)
		if err != nil {
			return nil, err
		}
		if rule, dim := action.ValidateApprovalBinding(a, a.ActionDigest, law.Version, law.Digest); rule != "" {
			return nil, fmt.Errorf("action/sqlite: %s (%s): approval %q was parked under law v%d %s but the current law is v%d %s — refusing the claim", rule, dim, approvalID, a.PolicyVersion, a.PolicyDigest, law.Version, law.Digest)
		}
		// R5-S2: the claim is execute's consume — the whole story is
		// judged inside THIS transaction over re-read rows too.
		var rawPreview string
		if err := tx.QueryRowContext(ctx,
			`SELECT canonical_preview FROM approvals WHERE approval_id = ?`, approvalID).Scan(&rawPreview); err != nil {
			return nil, fmt.Errorf("action/sqlite: claim preview %q: %w", approvalID, err)
		}
		p, err := action.ParseCanonicalPreview([]byte(rawPreview))
		if err != nil {
			return nil, fmt.Errorf("action/sqlite: claim preview %q: %w", approvalID, err)
		}
		if err := action.ValidatePreviewBinding(a, p); err != nil {
			return nil, fmt.Errorf("action/sqlite: approval %q: %w; refusing the claim", approvalID, err)
		}
		if err := s.verifyApprovalStory(ctx, tx, a, p); err != nil {
			return nil, fmt.Errorf("action/sqlite: approval %q: %w; refusing the claim", approvalID, err)
		}
	}
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

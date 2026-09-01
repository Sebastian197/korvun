// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The persisted ledger (Trust Layer Etapa 4, lote 3, spec FR-LED): every
// TERMINAL outcome appends its canonicalized, SIGNED receipt in the SAME
// transaction as the row it reifies — DENIED and SHADOWED at record time
// (the sealed NC-1/NC-2 yeses: the noes are the valuable half of the
// evidence), SUCCEEDED/FAILED at the terminal close. The domain API
// offers NO update and NO delete on receipts: append-only by
// construction, and the chain — monotonic sequence per partition, each
// receipt carrying its predecessor's hash from the documented genesis
// link — makes out-of-band edits DETECTABLE. Tamper-evident, never
// immutable: the operator controls storage and keys (§19.3). Receipts
// are EXEMPT from the E1 actions prune (the sealed exemption): the
// evidence outlives operational pruning; growth stays one bounded row
// per outcome, observable via the verifier's counts.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// ledgerPartition is the v1 single partition (E10 shards without a
// schema break — the field is real).
const ledgerPartition = "main"

// SetReceiptSealer wires the signing seam: the app injects the active
// key's signer (the store never touches key material). A nil sealer
// keeps the pre-stage behavior byte-for-byte — no receipts.
func (s *Store) SetReceiptSealer(seal func(action.Receipt) action.Receipt) {
	s.sealer = seal
}

// appendReceiptTx builds, links, seals and appends one receipt INSIDE
// the caller's transaction — together with its row or not at all.
func (s *Store) appendReceiptTx(ctx context.Context, tx *sql.Tx, r action.Receipt) error {
	if s.sealer == nil {
		return nil
	}
	// The chain head, read inside the SAME transaction: the single
	// serialized writer makes the sequence race-free; the UNIQUE
	// (partition, chain_seq) index is the belt under the suspenders.
	var lastSeq sql.NullInt64
	var lastHash sql.NullString
	err := tx.QueryRowContext(ctx,
		`SELECT chain_seq, receipt_hash FROM receipts
		  WHERE partition = ? ORDER BY chain_seq DESC LIMIT 1`, ledgerPartition,
	).Scan(&lastSeq, &lastHash)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		r.Partition = ledgerPartition
		r.ChainSeq = 0
		r.PreviousReceiptHash = action.GenesisPreviousHash
	case err != nil:
		return fmt.Errorf("action/sqlite: read chain head: %w", err)
	default:
		r.Partition = ledgerPartition
		r.ChainSeq = lastSeq.Int64 + 1
		r.PreviousReceiptHash = lastHash.String
	}
	r.ReceiptHash = action.ComputeReceiptHash(r)
	r = s.sealer(r)
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO receipts (receipt_id, action_id, intent_digest, principal_id,
		    authority_digest, decision_digest, action_digest, effect_class, attempt,
		    outcome, result_digest, started_at, finished_at, partition, chain_seq,
		    previous_receipt_hash, receipt_hash, signing_key_id, signature,
		    schema_version, approval_digest)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ReceiptID, r.ActionID, r.IntentDigest, r.PrincipalID,
		r.AuthorityDigest, r.DecisionDigest, r.ActionDigest, string(r.EffectClass), r.Attempt,
		r.Outcome, r.ResultDigest, timeOrNull(r.StartedAt), timeOrNull(r.FinishedAt),
		r.Partition, r.ChainSeq, r.PreviousReceiptHash, r.ReceiptHash,
		r.SigningKeyID, r.Signature, r.SchemaVersion, r.ApprovalDigest,
	); err != nil {
		return fmt.Errorf("action/sqlite: append receipt for %q: %w", r.ActionID, err)
	}
	return nil
}

// receiptForRecord reifies one just-recorded terminal outcome. The
// intent digest resolves from the stored contract inside the same
// transaction ("" when the intent is not in the table); the authority
// digest resolves only for grants the table knows (operator-issued) —
// in-memory derived grants keep their reference on the action row, the
// digest honestly absent (declared in the spec).
func (s *Store) receiptForRecord(ctx context.Context, tx *sql.Tx, env action.Envelope, d Decision, state action.State) action.Receipt {
	return action.Receipt{
		SchemaVersion:  2,
		ReceiptID:      action.NewReceiptID(),
		ActionID:       env.ActionID,
		IntentDigest:   s.intentDigestTx(ctx, tx, env.IntentID),
		PrincipalID:    env.Principal.PrincipalID,
		DecisionDigest: decisionDigest(d),
		ActionDigest:   env.ParametersDigest,
		EffectClass:    action.EffectClass(env.Effect.Class),
		Attempt:        1,
		Outcome:        string(state),
		StartedAt:      env.RequestedAt.UTC(),
		FinishedAt:     env.RequestedAt.UTC(),
	}
}

// approvalDigestTx resolves the CONSUMED approval's decision digest for
// one action ("" when no approved request exists — the honest empty of
// every unapproved outcome, FR-RCPT-2).
func (s *Store) approvalDigestTx(ctx context.Context, tx *sql.Tx, actionID string) string {
	// C2: the decision digest also seals what the human READ and the
	// law it was read under — preview_digest and the policy pin ride in.
	row := tx.QueryRowContext(ctx,
		`SELECT approval_id, action_digest, preview_digest, policy_version,
		        policy_digest, decision_principal_id, decision, decision_at
		   FROM approvals WHERE action_id = ? AND status = ?`,
		actionID, string(action.ApprovalApproved))
	var a action.Approval
	var decisionAt sql.NullString
	if err := row.Scan(&a.ApprovalID, &a.ActionDigest, &a.PreviewDigest,
		&a.PolicyVersion, &a.PolicyDigest, &a.DecisionPrincipalID,
		&a.Decision, &decisionAt); err != nil {
		return ""
	}
	if t, err := parseNullTime(decisionAt); err == nil {
		a.DecisionAt = t
	}
	return a.Digest()
}

// intentDigestTx resolves the stored intent's term digest ("" if absent).
func (s *Store) intentDigestTx(ctx context.Context, tx *sql.Tx, intentID string) string {
	if intentID == "" {
		return ""
	}
	var digest string
	if err := tx.QueryRowContext(ctx,
		`SELECT digest FROM intents WHERE intent_id = ?`, intentID).Scan(&digest); err != nil {
		return ""
	}
	return digest
}

// decisionDigest is the canonical digest over the decision terms.
func decisionDigest(d Decision) string {
	return action.HashCanonical(fmt.Sprintf(
		`{"outcome":%q,"policy_digest":%q,"policy_version":%d,"rule":%q}`,
		d.Outcome, d.PolicyDigest, d.PolicyVersion, d.Rule))
}

// Finish closes an action into a terminal state (the E1 seam,
// unchanged): FinishWithResult with no result digest.
func (s *Store) Finish(ctx context.Context, actionID string, to action.State, finishedAt time.Time) error {
	return s.FinishWithResult(ctx, actionID, to, finishedAt, "")
}

// FinishWithResult closes an action into a terminal state AND births its
// receipt in the same transaction, carrying the on-the-fly result digest
// (sealed NC-3: the digest travels, raw content never touches disk).
func (s *Store) FinishWithResult(ctx context.Context, actionID string, to action.State, finishedAt time.Time, resultDigest string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("action/sqlite: begin finish: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var current string
	err = tx.QueryRowContext(ctx, `SELECT state FROM actions WHERE action_id = ?`, actionID).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %q", ErrNotFound, actionID)
	}
	if err != nil {
		return fmt.Errorf("action/sqlite: read state %q: %w", actionID, err)
	}
	if err := action.Transition(action.State(current), to); err != nil {
		return fmt.Errorf("action/sqlite: finish %q: %w", actionID, err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE actions SET state = ?, finished_at = ? WHERE action_id = ?`,
		string(to), finishedAt.UTC().Format(time.RFC3339Nano), actionID,
	); err != nil {
		return fmt.Errorf("action/sqlite: finish %q: %w", actionID, err)
	}
	if s.sealer != nil {
		receipt, err := s.receiptForFinish(ctx, tx, actionID, to, finishedAt, resultDigest)
		if err != nil {
			return err
		}
		if err := s.appendReceiptTx(ctx, tx, receipt); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("action/sqlite: commit finish %q: %w", actionID, err)
	}
	return nil
}

// receiptForFinish reifies one executed outcome from its stored row,
// inside the closing transaction.
func (s *Store) receiptForFinish(ctx context.Context, tx *sql.Tx, actionID string, to action.State, finishedAt time.Time, resultDigest string) (action.Receipt, error) {
	var (
		paramsDigest string
		effectClass  string
		requestedAt  string
		principalID  sql.NullString
		intentID     sql.NullString
		outcome      string
		rule         string
		policyVer    int64
		policyDig    string
	)
	err := tx.QueryRowContext(ctx,
		`SELECT a.parameters_digest, a.effect_class, a.requested_at,
		        a.principal_id, a.intent_id, d.outcome, d.rule,
		        d.policy_version, d.policy_digest
		   FROM actions a JOIN action_decisions d ON d.action_id = a.action_id
		  WHERE a.action_id = ?`, actionID,
	).Scan(&paramsDigest, &effectClass, &requestedAt, &principalID, &intentID,
		&outcome, &rule, &policyVer, &policyDig)
	if err != nil {
		return action.Receipt{}, fmt.Errorf("action/sqlite: reify %q for its receipt: %w", actionID, err)
	}
	started, err := time.Parse(time.RFC3339Nano, requestedAt)
	if err != nil {
		return action.Receipt{}, fmt.Errorf("action/sqlite: parse requested_at of %q: %w", actionID, err)
	}
	return action.Receipt{
		SchemaVersion:  2,
		ApprovalDigest: s.approvalDigestTx(ctx, tx, actionID),
		ReceiptID:      action.NewReceiptID(),
		ActionID:       actionID,
		IntentDigest:   s.intentDigestTx(ctx, tx, intentID.String),
		PrincipalID:    principalID.String,
		DecisionDigest: decisionDigest(Decision{
			Outcome: outcome, Rule: rule,
			PolicyVersion: policyVer, PolicyDigest: policyDig,
		}),
		ActionDigest: paramsDigest,
		EffectClass:  action.EffectClass(effectClass),
		Attempt:      1,
		Outcome:      string(to),
		ResultDigest: resultDigest,
		StartedAt:    started,
		FinishedAt:   finishedAt.UTC(),
	}, nil
}

// ReceiptsByAction returns the receipts of one action, chain order.
func (s *Store) ReceiptsByAction(ctx context.Context, actionID string) ([]action.Receipt, error) {
	return s.queryReceipts(ctx,
		`SELECT receipt_id, action_id, intent_digest, principal_id, authority_digest,
		        decision_digest, action_digest, effect_class, attempt, outcome,
		        result_digest, started_at, finished_at, partition, chain_seq,
		        previous_receipt_hash, receipt_hash, signing_key_id, signature,
		        schema_version, approval_digest
		   FROM receipts WHERE action_id = ? ORDER BY chain_seq ASC`, actionID)
}

// ListReceipts returns one partition's whole chain, sequence order.
func (s *Store) ListReceipts(ctx context.Context, partition string) ([]action.Receipt, error) {
	return s.queryReceipts(ctx,
		`SELECT receipt_id, action_id, intent_digest, principal_id, authority_digest,
		        decision_digest, action_digest, effect_class, attempt, outcome,
		        result_digest, started_at, finished_at, partition, chain_seq,
		        previous_receipt_hash, receipt_hash, signing_key_id, signature,
		        schema_version, approval_digest
		   FROM receipts WHERE partition = ? ORDER BY chain_seq ASC`, partition)
}

// queryReceipts shares the scan across every receipt read.
func (s *Store) queryReceipts(ctx context.Context, query string, args ...any) ([]action.Receipt, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("action/sqlite: query receipts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []action.Receipt
	for rows.Next() {
		var (
			r          action.Receipt
			class      string
			startedAt  sql.NullString
			finishedAt sql.NullString
		)
		if err := rows.Scan(&r.ReceiptID, &r.ActionID, &r.IntentDigest, &r.PrincipalID,
			&r.AuthorityDigest, &r.DecisionDigest, &r.ActionDigest, &class, &r.Attempt,
			&r.Outcome, &r.ResultDigest, &startedAt, &finishedAt, &r.Partition,
			&r.ChainSeq, &r.PreviousReceiptHash, &r.ReceiptHash,
			&r.SigningKeyID, &r.Signature, &r.SchemaVersion, &r.ApprovalDigest); err != nil {
			return nil, fmt.Errorf("action/sqlite: scan receipt: %w", err)
		}
		r.EffectClass = action.EffectClass(class)
		if r.StartedAt, err = parseNullTime(startedAt); err != nil {
			return nil, fmt.Errorf("action/sqlite: parse receipt started_at: %w", err)
		}
		if r.FinishedAt, err = parseNullTime(finishedAt); err != nil {
			return nil, fmt.Errorf("action/sqlite: parse receipt finished_at: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("action/sqlite: iterate receipts: %w", err)
	}
	return out, nil
}

// GetReceipt returns one receipt by id (ErrNotFound if absent) — the
// verifier's entry point (Etapa 4 FR-VER).
func (s *Store) GetReceipt(ctx context.Context, receiptID string) (action.Receipt, error) {
	receipts, err := s.queryReceipts(ctx,
		`SELECT receipt_id, action_id, intent_digest, principal_id, authority_digest,
		        decision_digest, action_digest, effect_class, attempt, outcome,
		        result_digest, started_at, finished_at, partition, chain_seq,
		        previous_receipt_hash, receipt_hash, signing_key_id, signature,
		        schema_version, approval_digest
		   FROM receipts WHERE receipt_id = ?`, receiptID)
	if err != nil {
		return action.Receipt{}, err
	}
	if len(receipts) == 0 {
		return action.Receipt{}, fmt.Errorf("action/sqlite: receipt %q: %w", receiptID, ErrNotFound)
	}
	return receipts[0], nil
}

// ReceiptAt returns the receipt at one chain position (ErrNotFound if
// absent) — the verifier's predecessor lookup.
func (s *Store) ReceiptAt(ctx context.Context, partition string, seq int64) (action.Receipt, error) {
	receipts, err := s.queryReceipts(ctx,
		`SELECT receipt_id, action_id, intent_digest, principal_id, authority_digest,
		        decision_digest, action_digest, effect_class, attempt, outcome,
		        result_digest, started_at, finished_at, partition, chain_seq,
		        previous_receipt_hash, receipt_hash, signing_key_id, signature,
		        schema_version, approval_digest
		   FROM receipts WHERE partition = ? AND chain_seq = ?`, partition, seq)
	if err != nil {
		return action.Receipt{}, err
	}
	if len(receipts) == 0 {
		return action.Receipt{}, fmt.Errorf("action/sqlite: receipt at %s/%d: %w", partition, seq, ErrNotFound)
	}
	return receipts[0], nil
}

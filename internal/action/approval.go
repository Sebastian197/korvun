// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Approval domain (Trust Layer Etapa 5, lote 1, spec FR-APR — sealed
// NC-2/NC-3): the §10.8 approval request as a pure type. The
// INVALIDATION LAW (§15.3) is structural: the approval binds the E1
// action digest — operation, resource, protected parameters, amount,
// recipient and effect class all live UNDER that digest by
// construction, so any change is simply a different digest — plus the
// policy pin captured at request time. Expiry is judged at the
// injected instant (the E2 window mold; no sweeper, no goroutine).
// The proof of decision is the operator's own E4 ledger receipt — no
// second signing path exists.
package action

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// ApprovalStatus is the finite §10.8 lifecycle. Anything outside the
// set is treated as already-decided by the consume rule — fail closed.
type ApprovalStatus string

const (
	// ApprovalPending awaits the operator's decision.
	ApprovalPending ApprovalStatus = "PENDING"
	// ApprovalApproved was consumed by an approve act.
	ApprovalApproved ApprovalStatus = "APPROVED"
	// ApprovalRejected was closed by a reject act.
	ApprovalRejected ApprovalStatus = "REJECTED"
	// ApprovalExpired was closed by the clock at a consume touch.
	ApprovalExpired ApprovalStatus = "EXPIRED"
	// ApprovalCancelled was withdrawn before any decision.
	ApprovalCancelled ApprovalStatus = "CANCELLED"
)

// The finite invalidation/consume rules (spec FR-APR-2/3/4).
const (
	// RuleApprovalInvalidated reports a binding that no longer matches —
	// the dimension (digest | policy) rides beside it.
	RuleApprovalInvalidated = "approval_invalidated"
	// RuleApprovalExpired reports a consume touch past the window.
	RuleApprovalExpired = "approval_expired"
	// RuleApprovalAlreadyDecided reports a non-PENDING approval — the
	// one-shot law's loser message, and the fail-closed answer for any
	// unknown status.
	RuleApprovalAlreadyDecided = "approval_already_decided"
)

// Approval is the §10.8 v1 subset, honest today: the plan digest stays
// RESERVED with Etapa 6, and the signature field of the blueprint is
// the DecisionReceiptID — the operator's decision act leaves its own
// Ed25519-signed receipt in the E4 ledger.
type Approval struct {
	// ApprovalID is the request identity ("apr_" namespace).
	ApprovalID string
	// SchemaVersion pins the approval wire form (1 in Etapa 5).
	SchemaVersion int
	// ActionID names the parked action this request guards.
	ActionID string
	// ActionDigest is THE anchor (E1 canonical digest): the exact
	// version shown to the human. plan_digest: RESERVED→E6.
	ActionDigest string
	// PreviewDigest seals the persisted agent diff the human saw.
	PreviewDigest string
	// RequestedFrom is the principal asked to decide (the operator).
	RequestedFrom string
	// Reason is the finite gate rule that demanded approval.
	Reason string
	// RiskSummary is the one-line class + reversibility statement.
	RiskSummary string
	// PolicyVersion / PolicyDigest pin the law at request time — the
	// §15.3 policy half of the invalidation law.
	PolicyVersion int64
	PolicyDigest  string
	// RequestedAt / ExpiresAt bound the window (E2 mold: half-open,
	// zero ExpiresAt = no expiry; the creator sets the TTL).
	RequestedAt time.Time
	ExpiresAt   time.Time
	// Status is the finite lifecycle above.
	Status ApprovalStatus
	// DecisionPrincipalID, Decision, DecisionAt and Comment record the
	// human's verdict; empty until decided.
	DecisionPrincipalID string
	Decision            string
	DecisionAt          time.Time
	Comment             string
	// DecisionReceiptID references the decision act's own signed
	// receipt in the E4 ledger — §10.8's proof of decision.
	DecisionReceiptID string
}

// NewApprovalID generates a fresh approval identity ("apr_" + 16
// random bytes hex, the NewID mold).
func NewApprovalID() string {
	b := make([]byte, 16)
	// crypto/rand.Read is documented (Go ≥1.24) to always succeed.
	_, _ = rand.Read(b)
	return "apr_" + hex.EncodeToString(b)
}

// Digest returns the deterministic digest of the CONSUMED DECISION
// terms (spec FR-RCPT-1, the value receipt canonical v2 seals): the
// request identity, the exact action it bound, who decided, what they
// decided and when. Comment and presentation fields are deliberately
// outside — words do not change a decision's identity.
func (a Approval) Digest() string {
	// Widened by the C2 consolidation: what the human READ
	// (preview_digest) and the law it was read under (policy pin) are
	// decision terms too — the v2 receipt seals the whole chain.
	return HashCanonical(fmt.Sprintf(
		`{"action_digest":%q,"approval_id":%q,"decided_at":%q,"decider":%q,"decision":%q,"policy_digest":%q,"policy_version":%d,"preview_digest":%q}`,
		a.ActionDigest, a.ApprovalID, timeTerm(a.DecisionAt),
		a.DecisionPrincipalID, a.Decision,
		a.PolicyDigest, a.PolicyVersion, a.PreviewDigest))
}

// approvalWire is the canonical JSON shape (field order fixed by the
// struct; encoding/json emits keys in declaration order).
type approvalWire struct {
	ApprovalID          string `json:"approval_id"`
	SchemaVersion       int    `json:"schema_version"`
	ActionID            string `json:"action_id"`
	ActionDigest        string `json:"action_digest"`
	PreviewDigest       string `json:"preview_digest"`
	RequestedFrom       string `json:"requested_from"`
	Reason              string `json:"reason"`
	RiskSummary         string `json:"risk_summary"`
	PolicyVersion       int64  `json:"policy_version"`
	PolicyDigest        string `json:"policy_digest"`
	RequestedAt         string `json:"requested_at"`
	ExpiresAt           string `json:"expires_at"`
	Status              string `json:"status"`
	DecisionPrincipalID string `json:"decision_principal_id"`
	Decision            string `json:"decision"`
	DecisionAt          string `json:"decision_at"`
	Comment             string `json:"comment"`
	DecisionReceiptID   string `json:"decision_receipt_id"`
}

// CanonicalApproval renders the deterministic wire form of an approval.
func CanonicalApproval(a Approval) []byte {
	raw, err := json.Marshal(approvalWire{
		ApprovalID:          a.ApprovalID,
		SchemaVersion:       a.SchemaVersion,
		ActionID:            a.ActionID,
		ActionDigest:        a.ActionDigest,
		PreviewDigest:       a.PreviewDigest,
		RequestedFrom:       a.RequestedFrom,
		Reason:              a.Reason,
		RiskSummary:         a.RiskSummary,
		PolicyVersion:       a.PolicyVersion,
		PolicyDigest:        a.PolicyDigest,
		RequestedAt:         timeTerm(a.RequestedAt),
		ExpiresAt:           timeTerm(a.ExpiresAt),
		Status:              string(a.Status),
		DecisionPrincipalID: a.DecisionPrincipalID,
		Decision:            a.Decision,
		DecisionAt:          timeTerm(a.DecisionAt),
		Comment:             a.Comment,
		DecisionReceiptID:   a.DecisionReceiptID,
	})
	if err != nil {
		// Unreachable: the wire form is plain strings and ints.
		panic("action: canonical approval encoding failed: " + err.Error())
	}
	return raw
}

// ParseCanonicalApproval parses the canonical wire form STRICTLY:
// unknown fields, malformed times and trailing bytes are refused.
func ParseCanonicalApproval(raw []byte) (Approval, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var w approvalWire
	if err := dec.Decode(&w); err != nil {
		return Approval{}, fmt.Errorf("action: parse canonical approval: %w", err)
	}
	if dec.More() {
		return Approval{}, fmt.Errorf("action: parse canonical approval: trailing data")
	}
	requestedAt, err := parseTimeTerm(w.RequestedAt)
	if err != nil {
		return Approval{}, fmt.Errorf("action: parse approval requested_at: %w", err)
	}
	expiresAt, err := parseTimeTerm(w.ExpiresAt)
	if err != nil {
		return Approval{}, fmt.Errorf("action: parse approval expires_at: %w", err)
	}
	decisionAt, err := parseTimeTerm(w.DecisionAt)
	if err != nil {
		return Approval{}, fmt.Errorf("action: parse approval decision_at: %w", err)
	}
	return Approval{
		ApprovalID:          w.ApprovalID,
		SchemaVersion:       w.SchemaVersion,
		ActionID:            w.ActionID,
		ActionDigest:        w.ActionDigest,
		PreviewDigest:       w.PreviewDigest,
		RequestedFrom:       w.RequestedFrom,
		Reason:              w.Reason,
		RiskSummary:         w.RiskSummary,
		PolicyVersion:       w.PolicyVersion,
		PolicyDigest:        w.PolicyDigest,
		RequestedAt:         requestedAt,
		ExpiresAt:           expiresAt,
		Status:              ApprovalStatus(w.Status),
		DecisionPrincipalID: w.DecisionPrincipalID,
		Decision:            w.Decision,
		DecisionAt:          decisionAt,
		Comment:             w.Comment,
		DecisionReceiptID:   w.DecisionReceiptID,
	}, nil
}

// ValidateApprovalBinding enforces the §15.3 invalidation law at the
// consume touch: "" when the approval still binds; otherwise
// RuleApprovalInvalidated with the dimension that moved ("digest" —
// the structural half: ANY change to operation, resource, protected
// parameters, amount, recipient or effect class IS a different E1
// digest — or "policy": a different law than the one pinned at
// request time). Plan/dependency dimensions: RESERVED→E6.
func ValidateApprovalBinding(a Approval, actionDigest string, policyVersion int64, policyDigest string) (rule string, dimension string) {
	if a.ActionDigest != actionDigest {
		return RuleApprovalInvalidated, "digest"
	}
	if a.PolicyVersion != policyVersion || a.PolicyDigest != policyDigest {
		return RuleApprovalInvalidated, "policy"
	}
	return "", ""
}

// ApprovalConsumableAt reports whether the approval can be consumed at
// the injected instant: "" when it can, the finite rule when it cannot.
// The E2 clock discipline verbatim: half-open window (the expiry
// instant is OUT), zero ExpiresAt means no expiry, and any status but
// PENDING — unknown garbage included — is already decided, fail closed.
func ApprovalConsumableAt(a Approval, at time.Time) string {
	if a.Status != ApprovalPending {
		return RuleApprovalAlreadyDecided
	}
	if !a.ExpiresAt.IsZero() && !at.Before(a.ExpiresAt) {
		return RuleApprovalExpired
	}
	return ""
}

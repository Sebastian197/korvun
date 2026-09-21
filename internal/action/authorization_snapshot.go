// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package action

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// AuthorizationSnapshotV1Domain separates snapshot signatures from grants,
// lifecycle events, debits, and start proofs signed by the same profile key.
const AuthorizationSnapshotV1Domain = "korvun.authorization-snapshot.v1"

// AuthorizationSnapshotKind distinguishes a parked preview from a confirmed
// start. A pending snapshot never consumes budget.
type AuthorizationSnapshotKind string

const (
	AuthorizationSnapshotPending AuthorizationSnapshotKind = "pending"
	AuthorizationSnapshotStart   AuthorizationSnapshotKind = "start"
)

// AuthorizationBudgetKind is the finite wire vocabulary shown by approvals.
type AuthorizationBudgetKind string

const (
	AuthorizationBudgetFinite    AuthorizationBudgetKind = "finite"
	AuthorizationBudgetUnlimited AuthorizationBudgetKind = "unlimited"
)

// ErrAuthorizationSnapshotMalformed reports an incomplete display snapshot.
var ErrAuthorizationSnapshotMalformed = errors.New("action: authorization snapshot malformed")

// AuthorizationSnapshotV1 is immutable evidence captured while authority is
// checked. PrincipalChain is stored in root-to-leaf order. BudgetRemaining is
// the minimum capacity before this attempt; pending snapshots do not debit it.
type AuthorizationSnapshotV1 struct {
	Kind       AuthorizationSnapshotKind
	ActionID   string
	ApprovalID string
	// ConversationID is the conversation KEY the authority was resolved under
	// when this snapshot was taken ("" for a conversation-less request). It is
	// inside the signed bytes because the approved resume must re-resolve
	// authority under the SAME scope: without it the resume selected whatever
	// binding the empty conversation matched, and a request parked under a
	// conversation-scoped binding could never start — refused, falsely, as
	// "authority revoked".
	ConversationID         string
	RequesterPrincipalID   string
	ActorPrincipalID       string
	IdentityEvidenceDigest string
	IntentID               string
	IntentVersion          int
	IntentDigest           string
	IntentPurpose          string
	PrincipalChain         []string
	BudgetKind             AuthorizationBudgetKind
	BudgetRemaining        *int64
	RecordedAt             time.Time
}

type authorizationSnapshotWireV1 struct {
	SchemaVersion          int                       `json:"schema_version"`
	Kind                   AuthorizationSnapshotKind `json:"snapshot_kind"`
	ActionID               string                    `json:"action_id"`
	ApprovalID             string                    `json:"approval_id,omitempty"`
	ConversationID         string                    `json:"conversation_id,omitempty"`
	RequesterPrincipalID   string                    `json:"requester_principal_id"`
	ActorPrincipalID       string                    `json:"actor_principal_id"`
	IdentityEvidenceDigest string                    `json:"identity_evidence_digest"`
	IntentID               string                    `json:"intent_id"`
	IntentVersion          int                       `json:"intent_version"`
	IntentDigest           string                    `json:"intent_digest"`
	IntentPurpose          string                    `json:"intent_purpose"`
	PrincipalChain         []string                  `json:"principal_chain"`
	BudgetKind             AuthorizationBudgetKind   `json:"budget_kind"`
	BudgetRemaining        *int64                    `json:"budget_remaining,omitempty"`
	RecordedAt             string                    `json:"recorded_at"`
}

// Validate checks the closed snapshot shape without consulting current state.
func (s AuthorizationSnapshotV1) Validate() error {
	if s.ActionID == "" || s.RequesterPrincipalID == "" || s.ActorPrincipalID == "" ||
		s.IntentID == "" || s.IntentVersion <= 0 || s.IntentDigest == "" ||
		s.IntentPurpose == "" || s.RecordedAt.IsZero() || len(s.PrincipalChain) == 0 {
		return ErrAuthorizationSnapshotMalformed
	}
	for _, principal := range s.PrincipalChain {
		if principal == "" {
			return ErrAuthorizationSnapshotMalformed
		}
	}
	switch s.Kind {
	case AuthorizationSnapshotPending:
		if s.ApprovalID == "" {
			return ErrAuthorizationSnapshotMalformed
		}
	case AuthorizationSnapshotStart:
	default:
		return ErrAuthorizationSnapshotMalformed
	}
	switch s.BudgetKind {
	case AuthorizationBudgetFinite:
		if s.BudgetRemaining == nil || *s.BudgetRemaining < 0 {
			return ErrAuthorizationSnapshotMalformed
		}
	case AuthorizationBudgetUnlimited:
		if s.BudgetRemaining != nil {
			return ErrAuthorizationSnapshotMalformed
		}
	default:
		return ErrAuthorizationSnapshotMalformed
	}
	return nil
}

// CanonicalBytes returns the deterministic signed snapshot encoding.
func (s AuthorizationSnapshotV1) CanonicalBytes() []byte {
	raw, err := json.Marshal(authorizationSnapshotWireV1{
		SchemaVersion: 1, Kind: s.Kind, ActionID: s.ActionID, ApprovalID: s.ApprovalID,
		ConversationID:       s.ConversationID,
		RequesterPrincipalID: s.RequesterPrincipalID, ActorPrincipalID: s.ActorPrincipalID,
		IdentityEvidenceDigest: s.IdentityEvidenceDigest,
		IntentID:               s.IntentID, IntentVersion: s.IntentVersion, IntentDigest: s.IntentDigest,
		IntentPurpose: s.IntentPurpose, PrincipalChain: s.PrincipalChain,
		BudgetKind: s.BudgetKind, BudgetRemaining: s.BudgetRemaining,
		RecordedAt: s.RecordedAt.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		panic(err)
	}
	return raw
}

// ParseAuthorizationSnapshotV1 parses exact canonical snapshot bytes.
func ParseAuthorizationSnapshotV1(raw []byte) (AuthorizationSnapshotV1, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var wire authorizationSnapshotWireV1
	if err := dec.Decode(&wire); err != nil || wire.SchemaVersion != 1 {
		return AuthorizationSnapshotV1{}, ErrAuthorizationSnapshotMalformed
	}
	if err := ensureJSONEOF(dec); err != nil {
		return AuthorizationSnapshotV1{}, fmt.Errorf("%w: %w", ErrAuthorizationSnapshotMalformed, err)
	}
	recordedAt, err := time.Parse(time.RFC3339Nano, wire.RecordedAt)
	if err != nil {
		return AuthorizationSnapshotV1{}, ErrAuthorizationSnapshotMalformed
	}
	snapshot := AuthorizationSnapshotV1{
		Kind: wire.Kind, ActionID: wire.ActionID, ApprovalID: wire.ApprovalID,
		ConversationID:       wire.ConversationID,
		RequesterPrincipalID: wire.RequesterPrincipalID, ActorPrincipalID: wire.ActorPrincipalID,
		IdentityEvidenceDigest: wire.IdentityEvidenceDigest,
		IntentID:               wire.IntentID, IntentVersion: wire.IntentVersion, IntentDigest: wire.IntentDigest,
		IntentPurpose: wire.IntentPurpose, PrincipalChain: wire.PrincipalChain,
		BudgetKind: wire.BudgetKind, BudgetRemaining: wire.BudgetRemaining, RecordedAt: recordedAt.UTC(),
	}
	if err := snapshot.Validate(); err != nil {
		return AuthorizationSnapshotV1{}, err
	}
	// Its own refusal, with its own error: folded into the line above it
	// returned the zero snapshot under a NIL error.
	if snapshot.IdentityEvidenceDigest == "" {
		return AuthorizationSnapshotV1{}, fmt.Errorf("%w: identity evidence digest is empty", ErrAuthorizationSnapshotMalformed)
	}
	if !bytes.Equal(raw, snapshot.CanonicalBytes()) {
		return AuthorizationSnapshotV1{}, fmt.Errorf("%w: non-canonical bytes", ErrAuthorizationSnapshotMalformed)
	}
	return snapshot, nil
}

// SignAuthorizationSnapshotV1 seals one exact snapshot.
func SignAuthorizationSnapshotV1(priv ed25519.PrivateKey, snapshot AuthorizationSnapshotV1) AuthoritySignature {
	return SignAuthorityBytes(priv, AuthorizationSnapshotV1Domain, snapshot.CanonicalBytes())
}

// VerifyAuthorizationSnapshotV1 verifies a snapshot under the profile key.
func VerifyAuthorizationSnapshotV1(pub ed25519.PublicKey, snapshot AuthorizationSnapshotV1, signature AuthoritySignature) error {
	if err := snapshot.Validate(); err != nil {
		return ErrAuthorityEvidenceCorrupt
	}
	return VerifyAuthorityBytes(pub, AuthorizationSnapshotV1Domain, snapshot.CanonicalBytes(), signature)
}

// VerifyAuthorizationSnapshotBytesV1 parses and verifies stored bytes.
func VerifyAuthorizationSnapshotBytesV1(pub ed25519.PublicKey, raw []byte, signature AuthoritySignature) (AuthorizationSnapshotV1, error) {
	snapshot, err := ParseAuthorizationSnapshotV1(raw)
	if err != nil {
		return AuthorizationSnapshotV1{}, ErrAuthorityEvidenceCorrupt
	}
	if err := VerifyAuthorizationSnapshotV1(pub, snapshot, signature); err != nil {
		return AuthorizationSnapshotV1{}, err
	}
	return snapshot, nil
}

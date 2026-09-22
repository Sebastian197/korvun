// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/identity"
)

const (
	authorityEventDomain      = "korvun.authority-event.v1"
	authorityDebitDomain      = "korvun.authority-debit.v1"
	authorityBudgetHeadDomain = "korvun.authority-budget-head.v1"
	authorityStartDomain      = "korvun.authorization-start.v1"
	authorityActivationDomain = "korvun.authority-activation.v1"
	authorityBirthDomain      = "korvun.approval-birth.v1"
	authorityBirthHeadDomain  = "korvun.approval-birth-head.v1"
)

var (
	// ErrBudgetExhausted is a trusted finite-limit refusal.
	ErrBudgetExhausted = errors.New("action/sqlite: authority budget exhausted")
	// ErrAuthorityStoreBusy is an infrastructure refusal, never policy.
	ErrAuthorityStoreBusy = errors.New("action/sqlite: authority store busy")
	// ErrAuthorityRevoked reports a revoked leaf or ancestor.
	ErrAuthorityRevoked = errors.New("action/sqlite: authority revoked")
	// ErrAuthorityInactive reports a non-active grant.
	ErrAuthorityInactive = errors.New("action/sqlite: authority inactive")
	// ErrAuthorityExpired reports a grant outside its half-open window.
	ErrAuthorityExpired = errors.New("action/sqlite: authority expired")
	// ErrAuthorityMissing reports that no store-owned leaf applies.
	ErrAuthorityMissing = errors.New("action/sqlite: authority missing")
	// ErrAuthoritySignerUnavailable reports a store asked to seal authority
	// evidence with no authority signer wired.
	ErrAuthoritySignerUnavailable = errors.New("action/sqlite: authority signer unavailable")
	// ErrAuthorityAmbiguous reports more than one equally applicable leaf.
	ErrAuthorityAmbiguous = errors.New("action/sqlite: authority ambiguous")
	// ErrAuthorityApprovalRequired reports an immediate start whose verified
	// intent or grant chain requires a prior human approval.
	ErrAuthorityApprovalRequired = errors.New("action/sqlite: authority approval required")
	// ErrIssuerMismatch reports that authenticated actor and claimed issuer differ.
	ErrIssuerMismatch = errors.New("action/sqlite: authority issuer mismatch")
	// ErrActionAlreadyStarted reports durable replay.
	ErrActionAlreadyStarted = errors.New("action/sqlite: action already started")
	// ErrBudgetEvidenceCorrupt reports a mutable projection inconsistent with
	// its signed debit tail.
	ErrBudgetEvidenceCorrupt = errors.New("action/sqlite: budget evidence corrupt")
	// ErrAuthorizationSnapshotCorrupt reports missing, malformed, mismatched,
	// or unsigned strict authorization evidence.
	ErrAuthorizationSnapshotCorrupt = errors.New("action/sqlite: authorization snapshot corrupt")
)

// AuthorityStartProbe identifies the two destructive crash boundaries.
type AuthorityStartProbe string

const (
	AuthorityProbeAfterDebitBeforeCommit  AuthorityStartProbe = "after_debit_before_commit"
	AuthorityProbeBeforeCommit            AuthorityStartProbe = "before_commit"
	AuthorityProbeAfterCommitBeforeReturn AuthorityStartProbe = "after_commit_before_return"
)

// AuthorityStartRequest is the store-owned start input. ActionID is never
// caller supplied: the store mints it after write ownership.
type AuthorityStartRequest struct {
	ActorPrincipalID string
	CorrelationID    string
	SourceProtocol   string
	Channel          string
	ConversationID   string
	Operation        action.Operation
	Arguments        string
	EffectClass      action.EffectClass
	At               time.Time
	ApprovalID       string
	PolicyVersion    int64
	PolicyDigest     string
	ResolveEvidence  func(string) (identity.Evidence, error)
	Probe            func(AuthorityStartProbe) error
}

// AuthorityStartResult is the durable start capability returned after commit.
type AuthorityStartResult struct {
	ActionID        string
	IntentID        string
	AuthorityRefs   []string
	RemainingBefore *int64
	Evidence        *identity.Evidence
}

// AuthorityPendingRequest carries the final facts of a strict, non-consuming
// approval birth. Both ids are store-minted after writer ownership.
type AuthorityPendingRequest struct {
	ActorPrincipalID string
	CorrelationID    string
	SourceProtocol   string
	Channel          string
	ConversationID   string
	Operation        action.Operation
	Arguments        string
	EffectClass      action.EffectClass
	At               time.Time
	ApprovalContext  action.ApprovalContext
	ResolveEvidence  func(string) (identity.Evidence, error)
}

// AuthorityPendingResult is the committed parked capability.
type AuthorityPendingResult struct {
	ActionID      string
	ApprovalID    string
	IntentID      string
	AuthorityRefs []string
	Evidence      identity.Evidence
}

// AuthorityApprovedStartResult is the one-shot capability recovered from a
// strict parked action. It always reuses the parked action identity.
type AuthorityApprovedStartResult struct {
	ActionID  string
	Params    []byte
	Operation action.Operation
}

type storedGrantV2 struct {
	signed          action.SignedAuthorityGrantV2
	status          action.LifecycleStatus
	revision        int
	lastEventDigest string
}

type resolvedAuthority struct {
	intent           action.IntentContractV2
	chain            []storedGrantV2
	refs             []string
	principalChain   []string
	accounts         []string
	operationKey     string
	configGeneration int64
	remainingBefore  *int64
}

type authorityDebitRecord struct {
	ActionID, AccountID, Operation string
	Sequence, Cumulative           int64
	Previous, At                   string
}

type authorityDebitRef struct {
	AccountID    string `json:"account_id"`
	OperationKey string `json:"operation_key"`
	Sequence     int64  `json:"sequence"`
	Digest       string `json:"digest"`
}

type authorityBudgetHead struct {
	AccountID    string `json:"account_id"`
	OperationKey string `json:"operation_key"`
	Spent        int64  `json:"spent"`
	Sequence     int64  `json:"sequence"`
	TailDigest   string `json:"tail_digest"`
}

type authorityGrantStartRef struct {
	GrantID  string `json:"grant_id"`
	Version  int    `json:"version"`
	Digest   string `json:"digest"`
	Revision int    `json:"revision"`
}

type authorityStartRecord struct {
	ActionID               string `json:"action_id"`
	Generation             int64  `json:"generation"`
	IntentID               string `json:"intent_id"`
	IntentVersion          int    `json:"intent_version"`
	IntentDigest           string `json:"intent_digest"`
	GrantChain             string `json:"grant_chain"`
	DebitSetDigest         string `json:"debit_set_digest"`
	IdentityEvidenceDigest string `json:"identity_evidence_digest"`
	ConfigGeneration       int64  `json:"config_generation"`
	DecisionDigest         string `json:"decision_digest"`
	PolicyVersion          int64  `json:"policy_version"`
	PolicyDigest           string `json:"policy_digest"`
	AuthorizationTime      string `json:"authorization_time"`
}

func canonicalAuthorityDebit(actionID, accountID, operation string, sequence,
	cumulative int64, previous string, at time.Time) []byte {
	raw, _ := json.Marshal(authorityDebitRecord{
		ActionID: actionID, AccountID: accountID, Operation: operation,
		Sequence: sequence, Cumulative: cumulative, Previous: previous,
		At: at.UTC().Format(time.RFC3339Nano),
	})
	return raw
}

// SetAuthoritySigner wires the active profile key. Every returned signature is
// verified against the registered public key before insertion.
func (s *Store) SetAuthoritySigner(signer func(string, []byte) action.AuthoritySignature) {
	s.authoritySigner = signer
}

type authorityBirthHead struct {
	ProfileID        string `json:"profile_id"`
	ActivationDigest string `json:"activation_digest"`
	Sequence         int64  `json:"sequence"`
	LastEventDigest  string `json:"last_event_digest"`
}

type authorityBirthEvent struct {
	ProfileID      string `json:"profile_id"`
	Sequence       int64  `json:"sequence"`
	ApprovalID     string `json:"approval_id"`
	ActionID       string `json:"action_id"`
	ActionDigest   string `json:"action_digest"`
	StrictRequired bool   `json:"strict_required"`
	SnapshotDigest string `json:"snapshot_digest"`
	PreviousDigest string `json:"previous_event_digest"`
}

type authorityActivationRoot struct {
	ProfileID, ManifestDigest, ActorActionID, Actor, Reason, At string
}

// CanonicalAuthorityActivation binds the explicit operator act to the exact
// pre-existing approval manifest it adopts as legacy history.
func CanonicalAuthorityActivation(profileID, manifestDigest, reason string) []byte {
	raw, _ := json.Marshal(struct {
		ProfileID      string `json:"profile_id"`
		ManifestDigest string `json:"manifest_digest"`
		Reason         string `json:"reason"`
	}{profileID, manifestDigest, reason})
	return raw
}

// AuthorityActivationManifest returns the exact ordered legacy approval
// manifest digest an operator must bind in the separate activation act.
func (s *Store) AuthorityActivationManifest(ctx context.Context) (string, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	digest, _, err := authorityActivationManifestTx(ctx, tx)
	return digest, err
}

func authorityActivationManifestTx(ctx context.Context, tx *sql.Tx) (string, []authorityBirthEvent, error) {
	rows, err := tx.QueryContext(ctx, `SELECT approval_id,action_id,action_digest,authority_snapshot_required FROM approvals ORDER BY approval_id`)
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = rows.Close() }()
	var events []authorityBirthEvent
	for rows.Next() {
		var event authorityBirthEvent
		var strict int
		if err := rows.Scan(&event.ApprovalID, &event.ActionID, &event.ActionDigest, &strict); err != nil {
			return "", nil, err
		}
		if strict != 0 {
			return "", nil, ErrAuthorizationSnapshotCorrupt
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return "", nil, err
	}
	raw, _ := json.Marshal(events)
	return action.HashCanonical(string(raw)), events, nil
}

// ActivateAuthority creates the pinned approval-birth root once. Boot never
// calls this method; it only verifies the returned digest.
func (s *Store) ActivateAuthority(ctx context.Context, profileID, actorActionID,
	reason string, at time.Time) (string, error) {
	if profileID == "" || strings.TrimSpace(reason) == "" {
		return "", action.ErrAuthorityMalformed
	}
	tx, err := s.beginAuthorityWrite(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM approval_birth_heads WHERE profile_id=?`, profileID).Scan(&exists); err != nil {
		return "", err
	}
	if exists != 0 {
		return "", ErrAuthorizationSnapshotCorrupt
	}
	manifestDigest, legacy, err := authorityActivationManifestTx(ctx, tx)
	if err != nil {
		return "", err
	}
	actor, err := s.validateAuthorityActorTx(ctx, tx, actorActionID, "activate",
		CanonicalAuthorityActivation(profileID, manifestDigest, reason), true, at)
	if err != nil {
		return "", err
	}
	rootCanonical := mustAuthorityJSON(authorityActivationRoot{
		ProfileID: profileID, ManifestDigest: manifestDigest,
		ActorActionID: actorActionID, Actor: actor, Reason: reason,
		At: at.UTC().Format(time.RFC3339Nano),
	})
	rootSeal, err := s.signAuthorityTx(ctx, tx, authorityActivationDomain, rootCanonical)
	if err != nil {
		return "", err
	}
	activationID := "activation:" + profileID
	if _, err := tx.ExecContext(ctx, `INSERT INTO approval_birth_events(profile_id,sequence,approval_id,action_id,action_digest,strict_required,snapshot_digest,previous_event_digest,canonical_event,digest,signing_key_id,signature) VALUES(?,0,?,?,?,?,?,?,?,?,?,?)`,
		profileID, activationID, actorActionID, manifestDigest, 0, "activation",
		"", rootCanonical, rootSeal.Digest, rootSeal.SigningKeyID, rootSeal.Signature); err != nil {
		return "", err
	}
	sequence := int64(0)
	last := rootSeal.Digest
	for i := range legacy {
		sequence++
		event := legacy[i]
		event.ProfileID = profileID
		event.Sequence = sequence
		event.PreviousDigest = last
		canonical, _ := json.Marshal(event)
		seal, err := s.signAuthorityTx(ctx, tx, authorityBirthDomain, canonical)
		if err != nil {
			return "", err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO approval_birth_events(profile_id,sequence,approval_id,action_id,action_digest,strict_required,snapshot_digest,previous_event_digest,canonical_event,digest,signing_key_id,signature) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
			profileID, sequence, event.ApprovalID, event.ActionID, event.ActionDigest,
			0, "", last, canonical, seal.Digest, seal.SigningKeyID, seal.Signature); err != nil {
			return "", err
		}
		last = seal.Digest
	}
	head := authorityBirthHead{profileID, rootSeal.Digest, sequence, last}
	headCanonical, _ := json.Marshal(head)
	headSeal, err := s.signAuthorityTx(ctx, tx, authorityBirthHeadDomain, headCanonical)
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO approval_birth_heads(profile_id,activation_digest,sequence,last_event_digest,actor_action_id,canonical_head,digest,signing_key_id,signature) VALUES(?,?,?,?,?,?,?,?,?)`,
		profileID, rootSeal.Digest, sequence, last, actorActionID, headCanonical,
		headSeal.Digest, headSeal.SigningKeyID, headSeal.Signature); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", mapAuthorityStoreError(err)
	}
	return rootSeal.Digest, nil
}

// RequireAuthorityActivation verifies the externally pinned root and every
// current approval cross-link, then arms strict checks on this store instance.
func (s *Store) RequireAuthorityActivation(ctx context.Context, profileID, activationDigest string) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if profileID == "" {
		rows, err := tx.QueryContext(ctx, `SELECT profile_id FROM approval_birth_heads WHERE activation_digest=? ORDER BY profile_id`, activationDigest)
		if err != nil {
			return authorityReadFailure(ctx, err, ErrAuthorizationSnapshotCorrupt)
		}
		var profiles []string
		for rows.Next() {
			var profile string
			if err := rows.Scan(&profile); err != nil {
				_ = rows.Close()
				return authorityReadFailure(ctx, err, ErrAuthorizationSnapshotCorrupt)
			}
			profiles = append(profiles, profile)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return authorityReadFailure(ctx, err, ErrAuthorizationSnapshotCorrupt)
		}
		_ = rows.Close()
		if len(profiles) != 1 {
			return ErrAuthorizationSnapshotCorrupt
		}
		profileID = profiles[0]
	}
	if err := s.verifyAuthorityActivationTx(ctx, tx, profileID, activationDigest); err != nil {
		return err
	}
	if err := s.verifyAuthorizationStartsTx(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.authorityProfileID = profileID
	s.authorityActivationDigest = activationDigest
	return nil
}

func (s *Store) verifyAuthorityActivationTx(ctx context.Context, tx *sql.Tx, profileID, activationDigest string) error {
	var stored authorityBirthHead
	var actorActionID string
	var canonical []byte
	var digest, keyID, signature string
	if err := tx.QueryRowContext(ctx, `SELECT profile_id,activation_digest,sequence,last_event_digest,actor_action_id,canonical_head,digest,signing_key_id,signature FROM approval_birth_heads WHERE profile_id=?`, profileID).
		Scan(&stored.ProfileID, &stored.ActivationDigest, &stored.Sequence,
			&stored.LastEventDigest, &actorActionID, &canonical, &digest, &keyID,
			&signature); err != nil {
		return authorityReadFailure(ctx, err, ErrAuthorizationSnapshotCorrupt)
	}
	if stored.ProfileID != profileID || stored.ActivationDigest != activationDigest ||
		!bytes.Equal(canonical, mustAuthorityJSON(stored)) {
		return ErrAuthorizationSnapshotCorrupt
	}
	pub, _, err := publicKeyTx(ctx, tx, keyID)
	if err != nil {
		return authorityReadFailure(ctx, err, ErrAuthorizationSnapshotCorrupt)
	}
	if action.VerifyAuthorityBytes(pub, authorityBirthHeadDomain, canonical,
		action.AuthoritySignature{Digest: digest, SigningKeyID: keyID, Signature: signature}) != nil {
		return ErrAuthorizationSnapshotCorrupt
	}
	rows, err := tx.QueryContext(ctx, `SELECT sequence,approval_id,action_id,action_digest,strict_required,snapshot_digest,previous_event_digest,canonical_event,digest,signing_key_id,signature FROM approval_birth_events WHERE profile_id=? ORDER BY sequence`, profileID)
	if err != nil {
		return authorityReadFailure(ctx, err, ErrAuthorizationSnapshotCorrupt)
	}
	defer func() { _ = rows.Close() }()
	expected := int64(0)
	previous := ""
	seen := map[string]authorityBirthEvent{}
	for rows.Next() {
		var sequence int64
		var event authorityBirthEvent
		var strict int
		var raw []byte
		var eventDigest, eventKey, eventSignature string
		if err := rows.Scan(&sequence, &event.ApprovalID, &event.ActionID,
			&event.ActionDigest, &strict, &event.SnapshotDigest, &event.PreviousDigest,
			&raw, &eventDigest, &eventKey, &eventSignature); err != nil {
			return authorityReadFailure(ctx, err, ErrAuthorizationSnapshotCorrupt)
		}
		if sequence != expected || event.PreviousDigest != previous {
			return ErrAuthorizationSnapshotCorrupt
		}
		domain := authorityBirthDomain
		if sequence == 0 {
			domain = authorityActivationDomain
			if eventDigest != activationDigest || event.ApprovalID != "activation:"+profileID {
				return ErrAuthorizationSnapshotCorrupt
			}
			var root authorityActivationRoot
			if err := json.Unmarshal(raw, &root); err != nil ||
				root.ProfileID != profileID || root.ManifestDigest != event.ActionDigest ||
				root.ActorActionID != event.ActionID || root.Actor == "" ||
				strings.TrimSpace(root.Reason) == "" || root.At == "" ||
				!bytes.Equal(raw, mustAuthorityJSON(root)) {
				return ErrAuthorizationSnapshotCorrupt
			}
			activatedAt, err := time.Parse(time.RFC3339Nano, root.At)
			if err != nil {
				return ErrAuthorizationSnapshotCorrupt
			}
			operator, err := principalTx(ctx, tx, root.Actor)
			if err != nil {
				return authorityReadFailure(ctx, err, ErrAuthorizationSnapshotCorrupt)
			}
			// The actor is judged AT THE ACTIVATION, which is what it signed.
			// Demanding it enabled NOW made disabling that human later — an
			// ordinary act — read as a corrupt ledger on every start. A disable
			// dated at or before the activation is the other thing: the
			// recorded actor could not have activated anything.
			if operator.Kind != identity.PrincipalHuman ||
				(!operator.DisabledAt.IsZero() && !operator.DisabledAt.After(activatedAt)) {
				return ErrAuthorizationSnapshotCorrupt
			}
			if err := verifyPrincipalProjectionTx(ctx, tx, operator); err != nil {
				return authorityReadFailure(ctx, err, ErrAuthorizationSnapshotCorrupt)
			}
		} else {
			event.ProfileID, event.Sequence, event.StrictRequired = profileID, sequence, strict == 1
			if !bytes.Equal(raw, mustAuthorityJSON(event)) {
				return ErrAuthorizationSnapshotCorrupt
			}
			seen[event.ApprovalID] = event
		}
		pub, _, err := publicKeyTx(ctx, tx, eventKey)
		if err != nil {
			return authorityReadFailure(ctx, err, ErrAuthorizationSnapshotCorrupt)
		}
		if action.VerifyAuthorityBytes(pub, domain, raw,
			action.AuthoritySignature{Digest: eventDigest, SigningKeyID: eventKey, Signature: eventSignature}) != nil {
			return ErrAuthorizationSnapshotCorrupt
		}
		previous = eventDigest
		expected++
	}
	if err := rows.Err(); err != nil {
		return authorityReadFailure(ctx, err, ErrAuthorizationSnapshotCorrupt)
	}
	if expected-1 != stored.Sequence || previous != stored.LastEventDigest {
		return ErrAuthorizationSnapshotCorrupt
	}
	approvalRows, err := tx.QueryContext(ctx, `SELECT approval_id,action_id,action_digest,authority_snapshot_required FROM approvals`)
	if err != nil {
		return authorityReadFailure(ctx, err, ErrAuthorizationSnapshotCorrupt)
	}
	defer func() { _ = approvalRows.Close() }()
	for approvalRows.Next() {
		var id, actionID, actionDigest string
		var strict int
		if err := approvalRows.Scan(&id, &actionID, &actionDigest, &strict); err != nil {
			return authorityReadFailure(ctx, err, ErrAuthorizationSnapshotCorrupt)
		}
		event, ok := seen[id]
		if !ok || event.ActionID != actionID || event.ActionDigest != actionDigest ||
			event.StrictRequired != (strict == 1) {
			return ErrAuthorizationSnapshotCorrupt
		}
		if strict == 1 {
			if !strings.HasPrefix(id, "apr3_") {
				return ErrAuthorizationSnapshotCorrupt
			}
			var snapshotDigest string
			if err := tx.QueryRowContext(ctx, `SELECT authorization_digest FROM authorization_snapshots WHERE action_id=? AND approval_id=?`, actionID, id).Scan(&snapshotDigest); err != nil {
				return authorityReadFailure(ctx, err, ErrAuthorizationSnapshotCorrupt)
			}
			if snapshotDigest != event.SnapshotDigest {
				return ErrAuthorizationSnapshotCorrupt
			}
		} else if !strings.HasPrefix(id, "apr_") || event.SnapshotDigest != "" {
			return ErrAuthorizationSnapshotCorrupt
		}
	}
	if err := approvalRows.Err(); err != nil {
		return authorityReadFailure(ctx, err, ErrAuthorizationSnapshotCorrupt)
	}
	return nil
}

func mustAuthorityJSON(value any) []byte {
	raw, _ := json.Marshal(value)
	return raw
}

func (s *Store) appendApprovalBirthTx(ctx context.Context, tx *sql.Tx, approval action.Approval, snapshotDigest string) error {
	if s.authorityActivationDigest == "" {
		return nil
	}
	if err := s.verifyAuthorityActivationTx(ctx, tx, s.authorityProfileID, s.authorityActivationDigest); err != nil {
		return err
	}
	var sequence int64
	var previous string
	if err := tx.QueryRowContext(ctx, `SELECT sequence,last_event_digest FROM approval_birth_heads WHERE profile_id=?`, s.authorityProfileID).Scan(&sequence, &previous); err != nil {
		return ErrAuthorizationSnapshotCorrupt
	}
	sequence++
	event := authorityBirthEvent{
		ProfileID: s.authorityProfileID, Sequence: sequence,
		ApprovalID: approval.ApprovalID, ActionID: approval.ActionID,
		ActionDigest: approval.ActionDigest, StrictRequired: true,
		SnapshotDigest: snapshotDigest, PreviousDigest: previous,
	}
	canonical := mustAuthorityJSON(event)
	seal, err := s.signAuthorityTx(ctx, tx, authorityBirthDomain, canonical)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO approval_birth_events(profile_id,sequence,approval_id,action_id,action_digest,strict_required,snapshot_digest,previous_event_digest,canonical_event,digest,signing_key_id,signature) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		s.authorityProfileID, sequence, approval.ApprovalID, approval.ActionID,
		approval.ActionDigest, 1, snapshotDigest, previous, canonical, seal.Digest,
		seal.SigningKeyID, seal.Signature); err != nil {
		return err
	}
	head := authorityBirthHead{s.authorityProfileID, s.authorityActivationDigest, sequence, seal.Digest}
	headCanonical := mustAuthorityJSON(head)
	headSeal, err := s.signAuthorityTx(ctx, tx, authorityBirthHeadDomain, headCanonical)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE approval_birth_heads SET sequence=?,last_event_digest=?,canonical_head=?,digest=?,signing_key_id=?,signature=? WHERE profile_id=?`,
		sequence, seal.Digest, headCanonical, headSeal.Digest, headSeal.SigningKeyID,
		headSeal.Signature, s.authorityProfileID)
	return err
}

func newAuthorityActionID(generation int64) string {
	raw := make([]byte, 16)
	_, _ = rand.Read(raw)
	return fmt.Sprintf("act3_%d_%s", generation, hex.EncodeToString(raw))
}

func newAuthorityEvidenceID(prefix string) string {
	raw := make([]byte, 16)
	_, _ = rand.Read(raw)
	return prefix + hex.EncodeToString(raw)
}

// CanonicalAuthorityRevoke returns the exact parameters the administrative
// action and the mutation door both bind.
func CanonicalAuthorityRevoke(grantID, reason string) []byte {
	raw, _ := json.Marshal(struct {
		GrantID string `json:"grant_id"`
		Reason  string `json:"reason"`
	}{grantID, reason})
	return raw
}

// CanonicalAdminAuthorityGrant binds an administrative issue/delegation act
// to both exact terms and its operator reason.
func CanonicalAdminAuthorityGrant(grant action.AuthorityGrantV2, reason string) []byte {
	raw, _ := json.Marshal(struct {
		Terms  json.RawMessage `json:"terms"`
		Reason string          `json:"reason"`
	}{json.RawMessage(grant.CanonicalBytes()), reason})
	return raw
}

// CanonicalLegacyAuthorityImport binds an explicit v1-to-v2 import act.
func CanonicalLegacyAuthorityImport(legacyGrantID string, grant action.AuthorityGrantV2, reason string) []byte {
	raw, _ := json.Marshal(struct {
		LegacyGrantID string          `json:"legacy_grant_id"`
		Terms         json.RawMessage `json:"terms"`
		Reason        string          `json:"reason"`
	}{legacyGrantID, json.RawMessage(grant.CanonicalBytes()), reason})
	return raw
}

func (s *Store) beginAuthorityWrite(ctx context.Context) (*sql.Tx, error) {
	if s.authorityBeforeWriter != nil {
		s.authorityBeforeWriter()
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, mapAuthorityStoreError(err)
		}
		var revision int64
		err = tx.QueryRowContext(ctx,
			`UPDATE authority_write_lock SET revision=revision+1 WHERE singleton=1 RETURNING revision`).
			Scan(&revision)
		if err == nil {
			if revision <= 0 {
				_ = tx.Rollback()
				return nil, action.ErrAuthorityEvidenceCorrupt
			}
			return tx, nil
		}
		_ = tx.Rollback()
		mapped := mapAuthorityStoreError(err)
		if !errors.Is(mapped, ErrAuthorityStoreBusy) || ctx.Err() != nil ||
			time.Now().After(deadline) {
			return nil, mapped
		}
		select {
		case <-ctx.Done():
			return nil, mapAuthorityStoreError(ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
}

// authorityAbsent names an authority row a door was pointed at and that is not
// there: the grant to revoke, the parent to delegate under, the legacy grant to
// import. Those doors returned the driver's bare sql.ErrNoRows, a class with no
// name of its own at a door an operator reads the error of. Every other error
// keeps the name it came with.
func authorityAbsent(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %w", ErrAuthorityMissing, err)
	}
	return err
}

func mapAuthorityStoreError(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || strings.Contains(message, "busy") || strings.Contains(message, "locked") || strings.Contains(message, "interrupted") {
		return fmt.Errorf("%w: %w", ErrAuthorityStoreBusy, err)
	}
	return err
}

// authorityReadFailure names a failed READ of authority evidence with exactly
// one class. A store that did not answer — the context is over, or the error
// belongs to the SQLite busy family — is ErrAuthorityStoreBusy and keeps its
// cause in the chain. Everything else (the row that must exist is absent, a
// column will not convert) is the corruption sentinel the caller names. Calling
// an intact book corrupt because a deadline landed one statement after the
// write lock sends the operator after tampering that is not there (the
// adversary's pass over piece 3 phase 3, F5; the house precedent is
// classifyApprovalRead). Declared limit: a driver I/O failure that is neither
// of the two still reads as corruption, which fails closed.
func authorityReadFailure(ctx context.Context, err error, corrupt error) error {
	if mapped := mapAuthorityStoreError(err); errors.Is(mapped, ErrAuthorityStoreBusy) {
		return mapped
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return mapAuthorityStoreError(ctxErr)
	}
	return corrupt
}

// IssueAuthority persists a root grant only after actor, intent and terms are
// checked under the transaction that owns the write.
func (s *Store) IssueAuthority(ctx context.Context, grant action.AuthorityGrantV2, actorActionID string, at time.Time) error {
	return s.issueAuthority(ctx, grant, actorActionID, "", false, at)
}

// AdminIssueAuthority is a distinct operator-attributed door. It never
// rewrites the requested issuer; the signed event records the authenticated
// operator and reason as an administrative act.
func (s *Store) AdminIssueAuthority(ctx context.Context, grant action.AuthorityGrantV2,
	actorActionID, reason string, at time.Time) error {
	if strings.TrimSpace(reason) == "" {
		return action.ErrAuthorityMalformed
	}
	return s.issueAuthority(ctx, grant, actorActionID, reason, true, at)
}

func (s *Store) issueAuthority(ctx context.Context, grant action.AuthorityGrantV2,
	actorActionID, reason string, administrative bool, at time.Time) error {
	if err := grant.Validate(); err != nil {
		return err
	}
	tx, err := s.beginAuthorityWrite(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	parameters := grant.CanonicalBytes()
	if administrative {
		parameters = CanonicalAdminAuthorityGrant(grant, reason)
	}
	actor, err := s.validateAuthorityActorTx(ctx, tx, actorActionID, "issue", parameters, administrative, at)
	if err != nil {
		return err
	}
	intent, err := s.activeIntentTx(ctx, tx, grant.IntentID, grant.IntentVersion, grant.IntentDigest, at)
	if err != nil {
		return err
	}
	if !administrative && (actor != intent.OwnerPrincipalID || grant.IssuerPrincipalID != actor) {
		return ErrIssuerMismatch
	}
	grant, err = normalizeRootAuthority(grant, intent)
	if err != nil {
		return err
	}
	if err := s.insertGrantTx(ctx, tx, grant, actorActionID, actor, administrative, reason, at); err != nil {
		return err
	}
	if err := s.ensureBudgetAccountTx(ctx, tx, intent.ProfileID, "intent", intent.IntentID, intent.Budget, at); err != nil {
		return err
	}
	if err := s.ensureBudgetAccountTx(ctx, tx, grant.ProfileID, "grant", grant.GrantID, grant.Budget, at); err != nil {
		return err
	}
	return mapAuthorityStoreError(tx.Commit())
}

// DelegateAuthority validates stored remaining balance, chain and actor inside
// the child-persisting writer.
func (s *Store) DelegateAuthority(ctx context.Context, child action.AuthorityGrantV2, actorActionID string, at time.Time) error {
	return s.delegateAuthority(ctx, child, actorActionID, "", false, at)
}

// AdminDelegateAuthority is the operator-attributed counterpart of ordinary
// delegation. The chain and remaining balance are still checked under the
// child-persisting writer.
func (s *Store) AdminDelegateAuthority(ctx context.Context, child action.AuthorityGrantV2,
	actorActionID, reason string, at time.Time) error {
	if strings.TrimSpace(reason) == "" {
		return action.ErrAuthorityMalformed
	}
	return s.delegateAuthority(ctx, child, actorActionID, reason, true, at)
}

func (s *Store) delegateAuthority(ctx context.Context, child action.AuthorityGrantV2,
	actorActionID, reason string, administrative bool, at time.Time) error {
	if err := child.Validate(); err != nil {
		return err
	}
	tx, err := s.beginAuthorityWrite(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	parameters := child.CanonicalBytes()
	if administrative {
		parameters = CanonicalAdminAuthorityGrant(child, reason)
	}
	actor, err := s.validateAuthorityActorTx(ctx, tx, actorActionID, "delegate", parameters, administrative, at)
	if err != nil {
		return err
	}
	parents, err := s.authorityChainTx(ctx, tx, child.ParentGrantID, child.ParentGrantVersion, at)
	if err != nil {
		return authorityAbsent(err)
	}
	parent := parents[len(parents)-1]
	if !administrative && (actor != parent.signed.Grant.SubjectPrincipalID || child.IssuerPrincipalID != actor) {
		return ErrIssuerMismatch
	}
	intent, err := s.activeIntentTx(ctx, tx, child.IntentID, child.IntentVersion, child.IntentDigest, at)
	if err != nil {
		return err
	}
	if err := validateStoredAuthorityChain(parents, intent); err != nil {
		return err
	}
	remaining, err := s.remainingForGrantTx(ctx, tx, parent.signed.Grant)
	if err != nil {
		return err
	}
	normalized, err := action.NormalizeAuthorityDelegation(parent.signed.Grant, child, intent, remaining, defaultAuthorityMatchers())
	if err != nil {
		var attenuation *action.AttenuationError
		if errors.As(err, &attenuation) && strings.HasPrefix(attenuation.Dimension, "budget_") {
			return fmt.Errorf("%w: %v", ErrBudgetExhausted, err)
		}
		return err
	}
	if err := s.insertGrantTx(ctx, tx, normalized, actorActionID, actor, administrative, reason, at); err != nil {
		return err
	}
	if err := s.ensureBudgetAccountTx(ctx, tx, normalized.ProfileID, "grant", normalized.GrantID, normalized.Budget, at); err != nil {
		return err
	}
	return mapAuthorityStoreError(tx.Commit())
}

// RevokeAuthority appends the revocation event and moves the head atomically.
func (s *Store) RevokeAuthority(ctx context.Context, grantID, actorActionID, reason string, at time.Time) error {
	return s.revokeAuthority(ctx, grantID, actorActionID, reason, false, at)
}

// AdminRevokeAuthority records an authenticated operator rather than
// pretending the operator was the delegated subject or issuer.
func (s *Store) AdminRevokeAuthority(ctx context.Context, grantID, actorActionID, reason string, at time.Time) error {
	if strings.TrimSpace(reason) == "" {
		return action.ErrAuthorityMalformed
	}
	return s.revokeAuthority(ctx, grantID, actorActionID, reason, true, at)
}

func (s *Store) revokeAuthority(ctx context.Context, grantID, actorActionID, reason string,
	administrative bool, at time.Time) error {
	tx, err := s.beginAuthorityWrite(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stored, err := s.readGrantHeadTx(ctx, tx, grantID)
	if err != nil {
		return authorityAbsent(err)
	}
	actor, err := s.validateAuthorityActorTx(ctx, tx, actorActionID, "revoke", CanonicalAuthorityRevoke(grantID, reason), administrative, at)
	if err != nil {
		return err
	}
	if !administrative && actor != stored.signed.Grant.SubjectPrincipalID && actor != stored.signed.Grant.IssuerPrincipalID {
		return ErrIssuerMismatch
	}
	if stored.status == action.LifecycleRevoked {
		return ErrAuthorityRevoked
	}
	eventCanonical := canonicalGrantEvent(grantID, stored.signed.Grant.Version, stored.revision+1, stored.status, action.LifecycleRevoked, actorActionID, actor, administrative, reason, at, stored.lastEventDigest)
	seal, err := s.signAuthorityTx(ctx, tx, authorityEventDomain, eventCanonical)
	if err != nil {
		return err
	}
	admin := 0
	if administrative {
		admin = 1
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO grant_events(event_id,grant_id,grant_version,revision,from_status,to_status,actor_action_id,actor_principal_id,administrative,reason,occurred_at,previous_event_digest,canonical_event,digest,signing_key_id,signature) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, newAuthorityEvidenceID("gev_"), grantID, stored.signed.Grant.Version, stored.revision+1, string(stored.status), string(action.LifecycleRevoked), actorActionID, actor, admin, reason, at.UTC().Format(time.RFC3339Nano), stored.lastEventDigest, eventCanonical, seal.Digest, seal.SigningKeyID, seal.Signature); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE grant_heads SET revision=?,last_event_digest=?,status=? WHERE grant_id=?`, stored.revision+1, seal.Digest, string(action.LifecycleRevoked), grantID); err != nil {
		return err
	}
	return mapAuthorityStoreError(tx.Commit())
}

// ImportLegacyAuthority is the only v1-to-v2 grant door. The legacy row stays
// readable and unchanged; the new signed grant starts with the exact durable
// v1 consumption as its signed baseline.
func (s *Store) ImportLegacyAuthority(ctx context.Context, legacyGrantID string,
	grant action.AuthorityGrantV2, actorActionID, reason string, at time.Time) error {
	if err := grant.Validate(); err != nil {
		return err
	}
	if legacyGrantID == "" || strings.TrimSpace(reason) == "" {
		return action.ErrAuthorityMalformed
	}
	tx, err := s.beginAuthorityWrite(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	actor, err := s.validateAuthorityActorTx(ctx, tx, actorActionID, "import",
		CanonicalLegacyAuthorityImport(legacyGrantID, grant, reason), true, at)
	if err != nil {
		return err
	}
	legacy, err := legacyGrantTx(ctx, tx, legacyGrantID)
	if err != nil {
		return authorityAbsent(err)
	}
	if legacy.Status != action.LifecycleActive {
		return ErrAuthorityInactive
	}
	intent, err := s.activeIntentTx(ctx, tx, grant.IntentID, grant.IntentVersion, grant.IntentDigest, at)
	if err != nil {
		return err
	}
	if err := validateLegacyImport(legacy, grant, intent); err != nil {
		return err
	}
	if grant.ParentGrantID == "" {
		grant, err = normalizeRootAuthority(grant, intent)
		if err != nil {
			return err
		}
	} else {
		parents, err := s.authorityChainTx(ctx, tx, grant.ParentGrantID, grant.ParentGrantVersion, at)
		if err != nil {
			return err
		}
		if err := validateStoredAuthorityChain(parents, intent); err != nil {
			return err
		}
		parent := parents[len(parents)-1].signed.Grant
		remaining, err := s.remainingForGrantTx(ctx, tx, parent)
		if err != nil {
			return err
		}
		if _, err := action.NormalizeAuthorityDelegation(parent, grant, intent, remaining, defaultAuthorityMatchers()); err != nil {
			return err
		}
	}
	if err := s.insertGrantTx(ctx, tx, grant, actorActionID, actor, true, reason, at); err != nil {
		return err
	}
	if err := s.ensureBudgetAccountTx(ctx, tx, intent.ProfileID, "intent", intent.IntentID, intent.Budget, at); err != nil {
		return err
	}
	if err := s.ensureBudgetAccountTx(ctx, tx, grant.ProfileID, "grant", grant.GrantID, grant.Budget, at); err != nil {
		return err
	}
	account := budgetAccountID(grant.ProfileID, "grant", grant.GrantID)
	rows, err := tx.QueryContext(ctx, `SELECT operation,spent FROM budget_spent WHERE scope_id=? AND spent>0 ORDER BY operation`, legacyGrantID)
	if err != nil {
		return err
	}
	type baseline struct {
		operation string
		spent     int64
	}
	var baselines []baseline
	var total int64
	for rows.Next() {
		var item baseline
		if err := rows.Scan(&item.operation, &item.spent); err != nil {
			_ = rows.Close()
			return err
		}
		if item.operation == "" {
			item.operation = "*"
			total = item.spent
		} else {
			item.operation = "legacy/" + item.operation + "@1"
		}
		baselines = append(baselines, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range baselines {
		if err := s.insertLegacyBaselineTx(ctx, tx, actorActionID, account, item.operation, item.spent, at); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO legacy_authority_imports(grant_id,imported_version,actor_action_id,baseline_spent,imported_at) VALUES(?,?,?,?,?)`,
		legacyGrantID, grant.Version, actorActionID, total, at.UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return mapAuthorityStoreError(tx.Commit())
}

func legacyGrantTx(ctx context.Context, tx *sql.Tx, grantID string) (action.AuthorityGrant, error) {
	var grant action.AuthorityGrant
	var operations, resources, perOperation, validFrom, status, ceiling string
	var expiresAt sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT grant_id,intent_id,issuer_principal_id,subject_principal_id,
		parent_grant_id,operations,resources,max_actions,per_operation,valid_from,expires_at,status,
		depth_remaining,effect_ceiling FROM grants WHERE grant_id=?`, grantID).
		Scan(&grant.GrantID, &grant.IntentID, &grant.IssuerPrincipalID, &grant.SubjectPrincipalID,
			&grant.ParentGrantID, &operations, &resources, &grant.Budgets.MaxActions,
			&perOperation, &validFrom, &expiresAt, &status, &grant.DelegationDepthRemaining,
			&ceiling); err != nil {
		return action.AuthorityGrant{}, err
	}
	var err error
	if grant.Operations, err = parseSet(operations); err != nil {
		return action.AuthorityGrant{}, err
	}
	if grant.ResourceScope, err = parseSet(resources); err != nil {
		return action.AuthorityGrant{}, err
	}
	if grant.Budgets.MaxActionsPerOperation, err = parsePerOp(perOperation); err != nil {
		return action.AuthorityGrant{}, err
	}
	if grant.ValidFrom, err = time.Parse(time.RFC3339Nano, validFrom); err != nil {
		return action.AuthorityGrant{}, err
	}
	if grant.ExpiresAt, err = parseNullTime(expiresAt); err != nil {
		return action.AuthorityGrant{}, err
	}
	grant.Status = action.LifecycleStatus(status)
	grant.EffectCeiling = action.EffectClass(ceiling)
	return grant, nil
}

func validateLegacyImport(legacy action.AuthorityGrant, grant action.AuthorityGrantV2, intent action.IntentContractV2) error {
	if grant.IntentID != legacy.IntentID || grant.SubjectPrincipalID != legacy.SubjectPrincipalID ||
		grant.IssuerPrincipalID != legacy.IssuerPrincipalID || grant.ProfileID != intent.ProfileID {
		return action.ErrAttenuationViolated
	}
	wantOperations := make([]action.OperationRef, 0, len(legacy.Operations))
	for _, name := range legacy.Operations {
		wantOperations = append(wantOperations, action.OperationRef{Namespace: "legacy", Name: name, Version: 1})
	}
	if !operationSubsetSQLite(wantOperations, grant.Operations) || !operationSubsetSQLite(grant.Operations, wantOperations) {
		return action.ErrAttenuationViolated
	}
	wantResources := make([]action.ResourceRef, 0, len(legacy.ResourceScope))
	for _, resource := range legacy.ResourceScope {
		wantResources = append(wantResources, action.ResourceRef{Kind: "legacy", ID: resource})
	}
	if !sameResourceSet(grant.AllowedResources, wantResources) || len(grant.DeniedResources) != 0 ||
		len(grant.Channels) == 0 || len(grant.EffectClasses) == 0 {
		return action.ErrAttenuationViolated
	}
	if legacy.Budgets.MaxActions == 0 {
		if grant.Budget.Total != nil {
			return action.ErrAttenuationViolated
		}
	} else if grant.Budget.Total == nil || *grant.Budget.Total != int64(legacy.Budgets.MaxActions) {
		return action.ErrAttenuationViolated
	}
	if len(grant.Budget.PerOperation) != len(legacy.Budgets.MaxActionsPerOperation) {
		return action.ErrAttenuationViolated
	}
	for name, maximum := range legacy.Budgets.MaxActionsPerOperation {
		if grant.Budget.PerOperation["legacy/"+name+"@1"] != int64(maximum) {
			return action.ErrAttenuationViolated
		}
	}
	legacyExpires := legacy.ExpiresAt
	if legacyExpires.IsZero() {
		legacyExpires = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
	}
	if !grant.ValidFrom.Equal(legacy.ValidFrom) || !grant.ExpiresAt.Equal(legacyExpires) ||
		grant.DelegationDepthRemaining != legacy.DelegationDepthRemaining || grant.EffectCeiling != legacy.EffectCeiling {
		return action.ErrAttenuationViolated
	}
	return nil
}

func sameResourceSet(left, right []action.ResourceRef) bool {
	if len(left) != len(right) {
		return false
	}
	for _, item := range left {
		found := false
		for _, candidate := range right {
			if item == candidate {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (s *Store) insertLegacyBaselineTx(ctx context.Context, tx *sql.Tx, actorActionID,
	account, operation string, spent int64, at time.Time) error {
	if spent <= 0 {
		return nil
	}
	remaining, err := s.remainingBeforeAccountTx(ctx, tx, account, operation)
	if err != nil {
		return err
	}
	if remaining != nil && spent > *remaining {
		return ErrBudgetExhausted
	}
	canonical := canonicalAuthorityDebit(actorActionID, account, operation, 1, spent, "", at)
	seal, err := s.signAuthorityTx(ctx, tx, authorityDebitDomain, canonical)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO budget_debits(action_id,account_id,operation_key,sequence,previous_digest,cumulative_spent,canonical_debit,digest,signing_key_id,signature) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		actorActionID, account, operation, 1, "", spent, canonical, seal.Digest,
		seal.SigningKeyID, seal.Signature); err != nil {
		return err
	}
	return s.upsertBudgetCounterTx(ctx, tx, account, operation, spent, 1, seal.Digest)
}

func (s *Store) validateAuthorityActorTx(ctx context.Context, tx *sql.Tx, actionID, operation string,
	parameters []byte, administrative bool, at time.Time) (string, error) {
	if actionID == "" {
		return "", identity.ErrIdentityEvidenceMissing
	}
	evidence, err := validateActionIdentityV2Tx(ctx, tx, actionID, at)
	if err != nil {
		return "", err
	}
	var namespace, name, digest, principal string
	if err := tx.QueryRowContext(ctx, `SELECT op_namespace,op_name,parameters_digest,principal_id FROM actions WHERE action_id=?`, actionID).Scan(&namespace, &name, &digest, &principal); err != nil {
		return "", err
	}
	op := action.Operation{Namespace: "authority", Name: operation, Version: 1}
	if namespace != "authority" || name != operation || digest != action.Digest(op, string(parameters)) {
		return "", identity.ErrIdentityBindingMismatch
	}
	if principal != evidence.ActorPrincipalID {
		return "", identity.ErrIdentityEvidenceCorrupt
	}
	var used int
	if err := tx.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM grant_events WHERE actor_action_id=?) +
		(SELECT COUNT(*) FROM approval_birth_heads WHERE actor_action_id=?) +
		(SELECT COUNT(*) FROM legacy_authority_imports WHERE actor_action_id=?)`,
		actionID, actionID, actionID).Scan(&used); err != nil {
		return "", err
	}
	if used != 0 {
		return "", identity.ErrIdentityBindingMismatch
	}
	if administrative {
		operator, err := principalTx(ctx, tx, evidence.ResponsiblePrincipalID)
		if err != nil || operator.Kind != identity.PrincipalHuman {
			return "", ErrIssuerMismatch
		}
		return operator.ID, nil
	}
	return principal, nil
}

func normalizeRootAuthority(grant action.AuthorityGrantV2, intent action.IntentContractV2) (action.AuthorityGrantV2, error) {
	fail := func(dimension, detail string) (action.AuthorityGrantV2, error) {
		return action.AuthorityGrantV2{}, &action.AttenuationError{Dimension: dimension, Detail: detail}
	}
	if grant.ParentGrantID != "" || grant.ParentGrantVersion != 0 {
		return fail("parent", "root grant has a parent")
	}
	if grant.ProfileID != intent.ProfileID || grant.IntentID != intent.IntentID || grant.IntentVersion != intent.Version || grant.IntentDigest != intent.Digest() {
		return fail("intent", "root intent differs")
	}
	if !operationSubsetSQLite(grant.Operations, intent.Operations) || !effectSubsetSQLite(grant.EffectClasses, intent.EffectClasses) {
		return fail("operations", "operation or effect set widens")
	}
	if !resourcesIncludedSQLite(grant.AllowedResources, intent.AllowedResources, defaultAuthorityMatchers()) {
		return fail("allowed_resources", "allowed resource widens")
	}
	if !resourceSupersetSQLite(grant.DeniedResources, intent.DeniedResources) {
		return fail("denied_resources", "intent denial disappeared")
	}
	if !stringSubsetSQLite(grant.AllowedData, intent.DataScope) {
		return fail("allowed_data", "data set widens")
	}
	if !stringSubsetSQLite(grant.OutputDestinations, intent.OutputDestinations) {
		return fail("destinations", "destination set widens")
	}
	if grant.ValidFrom.Before(intent.ValidFrom) || grant.ExpiresAt.After(intent.ExpiresAt) || grant.DelegationDepthRemaining > intent.MaxDelegationDepth {
		return fail("validity", "root window or depth widens")
	}
	if intent.Budget.Total != nil && (grant.Budget.Total == nil || *grant.Budget.Total > *intent.Budget.Total) {
		return fail("budget_total_remaining", "root total exceeds intent")
	}
	if grant.Budget.PerOperation == nil {
		grant.Budget.PerOperation = map[string]int64{}
	} else {
		copied := make(map[string]int64, len(grant.Budget.PerOperation))
		for key, value := range grant.Budget.PerOperation {
			copied[key] = value
		}
		grant.Budget.PerOperation = copied
	}
	operations := make(map[string]bool, len(grant.Operations))
	for _, operation := range grant.Operations {
		operations[fmt.Sprintf("%s/%s@%d", operation.Namespace, operation.Name, operation.Version)] = true
	}
	for operation, maximum := range intent.Budget.PerOperation {
		if !operations[operation] {
			continue
		}
		childMaximum, exists := grant.Budget.PerOperation[operation]
		if !exists {
			return fail("budget_operation_remaining", "root operation limit disappeared")
		}
		if childMaximum > maximum {
			return fail("budget_operation_remaining", "root operation exceeds intent")
		}
	}
	if intent.Approval.Required && !grant.Approval.Required {
		return fail("approval", "required approval disappeared")
	}
	return grant, nil
}

func resourcesIncludedSQLite(children, parents []action.ResourceRef, matchers action.ResourceMatchers) bool {
	for _, child := range children {
		covered := false
		for _, parent := range parents {
			if child.Kind == parent.Kind && matchers[child.Kind] != nil && matchers[child.Kind](parent.ID, child.ID) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

func resourceSupersetSQLite(child, parent []action.ResourceRef) bool {
	for _, required := range parent {
		found := false
		for _, candidate := range child {
			if candidate == required {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func stringSubsetSQLite(child, parent []string) bool {
	for _, value := range child {
		if !containsAuthorityString(parent, value) {
			return false
		}
	}
	return true
}
func operationSubsetSQLite(child, parent []action.OperationRef) bool {
	for _, c := range child {
		ok := false
		for _, p := range parent {
			if c == p {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}
func effectSubsetSQLite(child, parent []action.EffectClass) bool {
	for _, c := range child {
		ok := false
		for _, p := range parent {
			if c == p {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}
func defaultAuthorityMatchers() action.ResourceMatchers {
	return action.ResourceMatchers{"path": action.PathResourceIncludes, "url": action.URLResourceIncludes, "cage": func(p, c string) bool { return p == c || p == "*" }, "legacy": func(p, c string) bool { return p == c || p == "*" }}
}

func (s *Store) insertGrantTx(ctx context.Context, tx *sql.Tx, grant action.AuthorityGrantV2, actorActionID, actor string, administrative bool, reason string, at time.Time) error {
	seal, err := s.signAuthorityTx(ctx, tx, "korvun.authority-grant.v2", grant.CanonicalBytes())
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO grant_versions(grant_id,version,schema_version,profile_id,intent_id,intent_version,intent_digest,parent_grant_id,parent_version,issuer_principal_id,subject_principal_id,canonical_terms,digest,signing_key_id,signature,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, grant.GrantID, grant.Version, 2, grant.ProfileID, grant.IntentID, grant.IntentVersion, grant.IntentDigest, grant.ParentGrantID, grant.ParentGrantVersion, grant.IssuerPrincipalID, grant.SubjectPrincipalID, grant.CanonicalBytes(), seal.Digest, seal.SigningKeyID, seal.Signature, at.UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	eventCanonical := canonicalGrantEvent(grant.GrantID, grant.Version, 1, "", action.LifecycleActive, actorActionID, actor, administrative, reason, at, "")
	eventSeal, err := s.signAuthorityTx(ctx, tx, authorityEventDomain, eventCanonical)
	if err != nil {
		return err
	}
	admin := 0
	if administrative {
		admin = 1
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO grant_events(event_id,grant_id,grant_version,revision,from_status,to_status,actor_action_id,actor_principal_id,administrative,reason,occurred_at,previous_event_digest,canonical_event,digest,signing_key_id,signature) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, newAuthorityEvidenceID("gev_"), grant.GrantID, grant.Version, 1, "", string(action.LifecycleActive), actorActionID, actor, admin, reason, at.UTC().Format(time.RFC3339Nano), "", eventCanonical, eventSeal.Digest, eventSeal.SigningKeyID, eventSeal.Signature); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO grant_heads(grant_id,active_version,revision,last_event_digest,status) VALUES(?,?,?,?,?)`, grant.GrantID, grant.Version, 1, eventSeal.Digest, string(action.LifecycleActive))
	return err
}

func canonicalGrantEvent(grantID string, version, revision int, from, to action.LifecycleStatus, actorActionID, actor string, administrative bool, reason string, at time.Time, previous string) []byte {
	raw, _ := json.Marshal(struct {
		GrantID        string                 `json:"grant_id"`
		Version        int                    `json:"version"`
		Revision       int                    `json:"revision"`
		From           action.LifecycleStatus `json:"from"`
		To             action.LifecycleStatus `json:"to"`
		ActorActionID  string                 `json:"actor_action_id"`
		Actor          string                 `json:"actor_principal_id"`
		Administrative bool                   `json:"administrative"`
		Reason         string                 `json:"reason"`
		OccurredAt     string                 `json:"occurred_at"`
		Previous       string                 `json:"previous_event_digest"`
	}{grantID, version, revision, from, to, actorActionID, actor, administrative, reason, at.UTC().Format(time.RFC3339Nano), previous})
	return raw
}

func (s *Store) signAuthorityTx(ctx context.Context, tx *sql.Tx, domain string, canonical []byte) (action.AuthoritySignature, error) {
	if s.authoritySigner == nil {
		return action.AuthoritySignature{}, ErrAuthoritySignerUnavailable
	}
	seal := s.authoritySigner(domain, canonical)
	pub, retired, err := publicKeyTx(ctx, tx, seal.SigningKeyID)
	if err != nil {
		return action.AuthoritySignature{}, err
	}
	if retired {
		return action.AuthoritySignature{}, action.ErrSigningKeyRetired
	}
	if err := action.VerifyAuthorityBytes(pub, domain, canonical, seal); err != nil {
		return action.AuthoritySignature{}, err
	}
	return seal, nil
}

func (s *Store) activeIntentTx(ctx context.Context, tx *sql.Tx, id string, version int, digest string, at time.Time) (action.IntentContractV2, error) {
	c, err := readIntentTermsTx(ctx, tx, id, version)
	if err != nil {
		return c, err
	}
	if c.Digest() != digest {
		return c, action.ErrIntentEvidenceCorrupt
	}
	head, err := intentHeadTx(ctx, tx, id)
	if err != nil {
		return c, err
	}
	if err := verifyIntentHistoryOn(ctx, tx, head); err != nil {
		return c, err
	}
	switch head.Status {
	case action.LifecycleRevoked:
		return c, action.ErrIntentRevoked
	case action.LifecycleExpired:
		return c, action.ErrIntentExpired
	case action.LifecycleActive:
	default:
		return c, action.ErrIntentInactive
	}
	var key, sig string
	if err := tx.QueryRowContext(ctx, `SELECT signing_key_id,signature FROM intent_versions WHERE intent_id=? AND version=?`, id, version).Scan(&key, &sig); err != nil {
		return c, err
	}
	pub, _, err := publicKeyTx(ctx, tx, key)
	if err != nil {
		return c, err
	}
	if err := action.VerifyIntentContractV2(pub, action.SignedIntentContractV2{Contract: c, Digest: digest, SigningKeyID: key, Signature: sig}); err != nil {
		return c, err
	}
	if at.Before(c.ValidFrom) || !at.Before(c.ExpiresAt) {
		return c, action.ErrIntentExpired
	}
	return c, nil
}

func (s *Store) readGrantHeadTx(ctx context.Context, tx *sql.Tx, id string) (storedGrantV2, error) {
	var version int
	if err := tx.QueryRowContext(ctx, `SELECT active_version FROM grant_heads WHERE grant_id=?`, id).Scan(&version); err != nil {
		return storedGrantV2{}, err
	}
	return s.readGrantTx(ctx, tx, id, version)
}
func (s *Store) readGrantTx(ctx context.Context, tx *sql.Tx, id string, version int) (storedGrantV2, error) {
	var raw []byte
	var digest, key, sig, status, last string
	var revision int
	if err := tx.QueryRowContext(ctx, `SELECT v.canonical_terms,v.digest,v.signing_key_id,v.signature,h.status,h.revision,h.last_event_digest FROM grant_versions v JOIN grant_heads h ON h.grant_id=v.grant_id WHERE v.grant_id=? AND v.version=? AND h.active_version=v.version`, id, version).Scan(&raw, &digest, &key, &sig, &status, &revision, &last); err != nil {
		return storedGrantV2{}, err
	}
	grant, err := action.ParseAuthorityGrantV2(raw)
	if err != nil {
		return storedGrantV2{}, action.ErrAuthorityEvidenceCorrupt
	}
	pub, _, err := publicKeyTx(ctx, tx, key)
	if err != nil {
		return storedGrantV2{}, err
	}
	signed := action.SignedAuthorityGrantV2{Grant: grant, AuthoritySignature: action.AuthoritySignature{Digest: digest, SigningKeyID: key, Signature: sig}}
	if err := action.VerifyAuthorityGrantV2(pub, signed); err != nil {
		return storedGrantV2{}, err
	}
	if err := s.verifyGrantHistoryTx(ctx, tx, id, version, revision, action.LifecycleStatus(status), last); err != nil {
		return storedGrantV2{}, err
	}
	return storedGrantV2{signed: signed, status: action.LifecycleStatus(status), revision: revision, lastEventDigest: last}, nil
}

func (s *Store) verifyGrantHistoryTx(ctx context.Context, tx *sql.Tx, grantID string,
	version, headRevision int, headStatus action.LifecycleStatus, headDigest string) error {
	rows, err := tx.QueryContext(ctx, `SELECT grant_version,revision,from_status,to_status,actor_action_id,actor_principal_id,administrative,reason,occurred_at,previous_event_digest,canonical_event,digest,signing_key_id,signature FROM grant_events WHERE grant_id=? ORDER BY revision`, grantID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	revision := 0
	previous := ""
	status := action.LifecycleStatus("")
	for rows.Next() {
		var eventVersion, eventRevision, administrative int
		var fromRaw, toRaw, actorActionID, actor, reason, occurredRaw, prior string
		var canonical []byte
		var digest, keyID, signature string
		if err := rows.Scan(&eventVersion, &eventRevision, &fromRaw, &toRaw,
			&actorActionID, &actor, &administrative, &reason, &occurredRaw, &prior,
			&canonical, &digest, &keyID, &signature); err != nil {
			return err
		}
		occurred, err := time.Parse(time.RFC3339Nano, occurredRaw)
		if err != nil || eventVersion != version || eventRevision != revision+1 ||
			prior != previous || action.LifecycleStatus(fromRaw) != status {
			return action.ErrAuthorityEvidenceCorrupt
		}
		to := action.LifecycleStatus(toRaw)
		if eventRevision == 1 {
			if fromRaw != "" || to != action.LifecycleActive {
				return action.ErrAuthorityEvidenceCorrupt
			}
		} else if to != action.LifecycleRevoked || status != action.LifecycleActive {
			return action.ErrAuthorityEvidenceCorrupt
		}
		want := canonicalGrantEvent(grantID, eventVersion, eventRevision,
			action.LifecycleStatus(fromRaw), to, actorActionID, actor,
			administrative == 1, reason, occurred, prior)
		pub, _, err := publicKeyTx(ctx, tx, keyID)
		if err != nil || !bytes.Equal(canonical, want) ||
			action.VerifyAuthorityBytes(pub, authorityEventDomain, canonical,
				action.AuthoritySignature{Digest: digest, SigningKeyID: keyID, Signature: signature}) != nil {
			return action.ErrAuthorityEvidenceCorrupt
		}
		revision, previous, status = eventRevision, digest, to
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if revision != headRevision || previous != headDigest || status != headStatus {
		return action.ErrAuthorityEvidenceCorrupt
	}
	return nil
}

func budgetAccountID(profile, kind, scope string) string {
	return action.HashCanonical(profile + "\x00" + kind + "\x00" + scope)
}
func perOperationJSON(m map[string]int64) string { raw, _ := json.Marshal(m); return string(raw) }
func (s *Store) ensureBudgetAccountTx(ctx context.Context, tx *sql.Tx, profile, kind, scope string, budget action.IntentBudgetV2, at time.Time) error {
	var total any
	if budget.Total != nil {
		total = *budget.Total
	}
	accountID := budgetAccountID(profile, kind, scope)
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO budget_accounts(account_id,profile_id,scope_kind,stable_scope_id,max_total,per_operation,created_at) VALUES(?,?,?,?,?,?,?)`, accountID, profile, kind, scope, total, perOperationJSON(budget.PerOperation), at.UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	var storedProfile, storedKind, storedScope, storedPerOperation string
	var storedTotal sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT profile_id,scope_kind,stable_scope_id,max_total,per_operation FROM budget_accounts WHERE account_id=?`, accountID).
		Scan(&storedProfile, &storedKind, &storedScope, &storedTotal, &storedPerOperation); err != nil {
		return err
	}
	expectedTotal, hasTotal := int64(0), budget.Total != nil
	if hasTotal {
		expectedTotal = *budget.Total
	}
	if storedProfile != profile || storedKind != kind || storedScope != scope ||
		storedTotal.Valid != hasTotal || hasTotal && storedTotal.Int64 != expectedTotal ||
		storedPerOperation != perOperationJSON(budget.PerOperation) {
		return ErrBudgetEvidenceCorrupt
	}
	return nil
}
func (s *Store) remainingForGrantTx(ctx context.Context, tx *sql.Tx, grant action.AuthorityGrantV2) (action.AuthorityBudgetRemaining, error) {
	account := budgetAccountID(grant.ProfileID, "grant", grant.GrantID)
	remaining := action.AuthorityBudgetRemaining{PerOperation: map[string]int64{}}
	spent, sequence, tail, err := counterTx(ctx, tx, account, "*")
	if err != nil {
		return remaining, err
	}
	if err := s.verifyDebitTailTx(ctx, tx, account, "*", spent, sequence, tail); err != nil {
		return remaining, err
	}
	if grant.Budget.Total != nil {
		v := *grant.Budget.Total - spent
		if v < 0 {
			return remaining, ErrBudgetEvidenceCorrupt
		}
		remaining.Total = &v
	}
	for op, max := range grant.Budget.PerOperation {
		spent, sequence, tail, err := counterTx(ctx, tx, account, op)
		if err != nil {
			return remaining, err
		}
		if err := s.verifyDebitTailTx(ctx, tx, account, op, spent, sequence, tail); err != nil {
			return remaining, err
		}
		value := max - spent
		if value < 0 {
			return remaining, ErrBudgetEvidenceCorrupt
		}
		remaining.PerOperation[op] = value
	}
	return remaining, nil
}

// ParkAuthorization verifies current identity, intent, authority, scope and
// balance without consuming them, then births the strict pending action and
// its signed display snapshot in the same writer transaction.
func (s *Store) ParkAuthorization(ctx context.Context, request AuthorityPendingRequest) (AuthorityPendingResult, error) {
	if request.ResolveEvidence == nil {
		return AuthorityPendingResult{}, identity.ErrIdentityEvidenceMissing
	}
	tx, err := s.beginAuthorityWrite(ctx)
	if err != nil {
		return AuthorityPendingResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	actionID := s.authorityNewActionID(authorityEvidenceEpoch)
	evidence, err := request.ResolveEvidence(actionID)
	if err != nil {
		return AuthorityPendingResult{}, err
	}
	if request.ActorPrincipalID != "" && request.ActorPrincipalID != evidence.ActorPrincipalID {
		return AuthorityPendingResult{}, ErrIssuerMismatch
	}
	resolved, err := s.resolveAuthorityTx(ctx, tx, evidence.ActorPrincipalID, request.Channel,
		request.ConversationID, request.Operation, request.Arguments, request.EffectClass,
		request.At, true)
	if err != nil {
		return AuthorityPendingResult{}, err
	}
	if resolved.remainingBefore != nil && *resolved.remainingBefore <= 0 {
		return AuthorityPendingResult{}, ErrBudgetExhausted
	}
	correlationID := request.CorrelationID
	if correlationID == "" {
		correlationID = actionID
	}
	protocol := request.SourceProtocol
	if protocol == "" {
		protocol = "authority"
	}
	env := action.NewEnvelope(actionID, correlationID,
		action.Source{Kind: "agent_brain", Protocol: protocol, Channel: request.Channel},
		request.Operation, request.Arguments, request.At)
	env.Effect = action.Effect{Class: string(request.EffectClass)}
	env.Principal = action.PrincipalRef{
		PrincipalID: evidence.ActorPrincipalID, EvidenceID: evidence.EvidenceID,
		ResponsibleHumanID: evidence.ResponsiblePrincipalID,
	}
	env.IntentID = resolved.intent.IntentID
	env.AuthorityRefs = append([]string(nil), resolved.refs...)
	approvalContext := request.ApprovalContext
	approvalContext.IntentPurpose = resolved.intent.Purpose
	approvalContext.Rule = "require_approval"
	approvalContext.Now = request.At
	if len(resolved.refs) > 0 {
		approvalContext.GrantID = resolved.refs[len(resolved.refs)-1]
		approvalContext.GrantDepth = len(resolved.refs)
	}
	if resolved.remainingBefore == nil {
		approvalContext.CostLine = "maximum unlimited starts"
	} else {
		approvalContext.CostLine = fmt.Sprintf("maximum %d starts before this attempt", *resolved.remainingBefore)
	}
	approvalID := action.NewStrictApprovalID()
	bound, err := action.NewBoundApprovalRequestWithID(env, request.Arguments, approvalContext, approvalID)
	if err != nil {
		return AuthorityPendingResult{}, err
	}
	a := bound.Approval()
	p := bound.Preview()
	if len(request.Arguments) > maxApprovalParamsBytes {
		return AuthorityPendingResult{}, fmt.Errorf("action/sqlite: approval request %q: params exceed the %d-byte cap", approvalID, maxApprovalParamsBytes)
	}
	if err := validateResolvedEvidenceTx(ctx, tx, evidence, env, request.At.UTC()); err != nil {
		return AuthorityPendingResult{}, err
	}
	signed, err := s.signEvidenceTx(ctx, tx, evidence)
	if err != nil {
		return AuthorityPendingResult{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO actions(action_id,schema_version,correlation_id,
		source_kind,source_protocol,source_channel,op_namespace,op_name,op_version,
		parameters_digest,effect_class,state,requested_at,principal_id,intent_id,authority_refs,
		identity_version,identity_evidence_digest,identity_canonical_evidence,identity_signing_key_id,identity_signature)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,2,?,?,?,?)`, env.ActionID, env.SchemaVersion,
		env.CorrelationID, env.Source.Kind, env.Source.Protocol, env.Source.Channel,
		env.Operation.Namespace, env.Operation.Name, env.Operation.Version, env.ParametersDigest,
		env.Effect.Class, string(action.StatePendingApproval), env.RequestedAt.UTC().Format(time.RFC3339Nano),
		evidence.ActorPrincipalID, env.IntentID, authorityRefsValue(resolved.refs), signed.Digest,
		signed.Canonical, signed.SigningKeyID, signed.Signature); err != nil {
		return AuthorityPendingResult{}, err
	}
	if err := insertEvidenceTx(ctx, tx, signed); err != nil {
		return AuthorityPendingResult{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO action_decisions(action_id,outcome,rule,decided_at,policy_version,policy_digest) VALUES(?,?,?,?,?,?)`,
		env.ActionID, "require_approval", "require_approval", request.At.UTC().Format(time.RFC3339Nano),
		a.PolicyVersion, a.PolicyDigest); err != nil {
		return AuthorityPendingResult{}, err
	}
	budgetKind := action.AuthorizationBudgetUnlimited
	if resolved.remainingBefore != nil {
		budgetKind = action.AuthorizationBudgetFinite
	}
	snapshot := action.AuthorizationSnapshotV1{
		Kind: action.AuthorizationSnapshotPending, ActionID: actionID, ApprovalID: approvalID,
		ConversationID:       request.ConversationID,
		RequesterPrincipalID: evidence.RequesterPrincipalID, ActorPrincipalID: evidence.ActorPrincipalID,
		IdentityEvidenceDigest: signed.Digest, IntentID: resolved.intent.IntentID,
		IntentVersion: resolved.intent.Version, IntentDigest: resolved.intent.Digest(),
		IntentPurpose: resolved.intent.Purpose, PrincipalChain: resolved.principalChain,
		BudgetKind: budgetKind, BudgetRemaining: resolved.remainingBefore, RecordedAt: request.At,
	}
	if err := s.insertAuthorizationSnapshotTx(ctx, tx, snapshot, signed.Digest); err != nil {
		return AuthorityPendingResult{}, err
	}
	var snapshotDigest string
	if err := tx.QueryRowContext(ctx, `SELECT authorization_digest FROM authorization_snapshots WHERE action_id=?`, actionID).Scan(&snapshotDigest); err != nil {
		return AuthorityPendingResult{}, err
	}
	if err := s.appendApprovalBirthTx(ctx, tx, a, snapshotDigest); err != nil {
		return AuthorityPendingResult{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO approvals(approval_id,schema_version,action_id,action_digest,
		preview_digest,canonical_preview,canonical_params,requested_from,reason,risk_summary,
		policy_version,policy_digest,requested_at,expires_at,status,authority_snapshot_required)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,1)`, a.ApprovalID, a.SchemaVersion, a.ActionID,
		a.ActionDigest, a.PreviewDigest, string(action.CanonicalPreview(p)), string(action.CanonicalParams(request.Arguments)),
		a.RequestedFrom, a.Reason, a.RiskSummary, a.PolicyVersion, a.PolicyDigest,
		a.RequestedAt.UTC().Format(time.RFC3339Nano), timeCol(a.ExpiresAt), string(action.ApprovalPending)); err != nil {
		return AuthorityPendingResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return AuthorityPendingResult{}, mapAuthorityStoreError(err)
	}
	return AuthorityPendingResult{
		ActionID: actionID, ApprovalID: approvalID, IntentID: resolved.intent.IntentID,
		AuthorityRefs: append([]string(nil), resolved.refs...), Evidence: evidence,
	}, nil
}

// StartApprovedAuthorization re-verifies and spends a strict parked request
// under one writer transaction. It reuses the parked action id and stored
// phase-1 evidence, and purges parameters only after every authority write has
// succeeded in that same transaction.
func (s *Store) StartApprovedAuthorization(ctx context.Context, approvalID string,
	law PolicyPin, wantDigest string, at time.Time) (AuthorityApprovedStartResult, error) {
	return s.startApprovedAuthorization(ctx, approvalID, law, wantDigest, at, nil)
}

func (s *Store) startApprovedAuthorization(ctx context.Context, approvalID string,
	law PolicyPin, wantDigest string, at time.Time,
	probe func(AuthorityStartProbe) error) (AuthorityApprovedStartResult, error) {
	tx, err := s.beginAuthorityWrite(ctx)
	if err != nil {
		return AuthorityApprovedStartResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	a, err := s.approvalTx(ctx, tx, approvalID)
	if err != nil {
		return AuthorityApprovedStartResult{}, err
	}
	// A repeated start is named FIRST, as the precedence list puts it. Its
	// first life purged the parked parameters and moved the action on, so any
	// belt judged before this one names that purge or that state instead —
	// «parameters column empty … action not found», about an action that
	// exists and has started.
	var already int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM authorization_starts WHERE action_id=?`, a.ActionID).Scan(&already); err != nil {
		return AuthorityApprovedStartResult{}, authorityReadFailure(ctx, err, ErrAuthorizationSnapshotCorrupt)
	}
	if already != 0 {
		return AuthorityApprovedStartResult{}, ErrActionAlreadyStarted
	}
	if a.Status != action.ApprovalApproved {
		return AuthorityApprovedStartResult{}, ErrApprovalNoLongerApproved
	}
	if rule, dimension := action.ValidateApprovalBinding(a, wantDigest, law.Version, law.Digest); rule != "" {
		return AuthorityApprovedStartResult{}, fmt.Errorf("action/sqlite: strict approval %q moved under current law (%s): %w", approvalID, dimension, ErrApprovalInvalidated)
	}
	var previewCell, paramsCell sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT canonical_preview,canonical_params FROM approvals WHERE approval_id=?`, approvalID).Scan(&previewCell, &paramsCell); err != nil {
		return AuthorityApprovedStartResult{}, err
	}
	if !previewCell.Valid || !paramsCell.Valid {
		return AuthorityApprovedStartResult{}, ErrApprovalEvidenceCorrupt
	}
	if paramsCell.String == "" {
		return AuthorityApprovedStartResult{}, ErrApprovalParamsEmpty
	}
	preview, err := action.ParseCanonicalPreview([]byte(previewCell.String))
	if err != nil || action.ValidatePreviewBinding(a, preview) != nil {
		return AuthorityApprovedStartResult{}, ErrApprovalEvidenceCorrupt
	}
	if err := verifyApprovalStoryTyped(ctx, tx, a, preview); err != nil {
		return AuthorityApprovedStartResult{}, err
	}
	pending, err := s.approvalAuthoritySnapshotTx(ctx, tx, a)
	if err != nil {
		return AuthorityApprovedStartResult{}, err
	}
	if pending == nil || pending.Kind != action.AuthorizationSnapshotPending {
		return AuthorityApprovedStartResult{}, ErrAuthorizationSnapshotCorrupt
	}
	// WHO ASKED was answered when the request was parked, and `pending` — the
	// snapshot this transaction has just verified against its signature — is
	// where that instant is recorded, so the ingress capability's expiry is
	// judged there. What may have CHANGED while the human decided is judged
	// here and now: a disabled principal, a revoked or advanced binding.
	if _, err := validateActionIdentityV2AtTx(ctx, tx, a.ActionID, pending.RecordedAt, at); err != nil {
		return AuthorityApprovedStartResult{}, err
	}
	operation, state, err := ternaOf(ctx, tx, a.ActionID)
	if err != nil {
		return AuthorityApprovedStartResult{}, err
	}
	if state != action.StateApproved {
		return AuthorityApprovedStartResult{}, ErrApprovalNoLongerApproved
	}
	if action.Digest(operation, paramsCell.String) != wantDigest {
		return AuthorityApprovedStartResult{}, ErrApprovalParamsDigestMismatch
	}
	var actor, channel, effectRaw, storedIntent, refsRaw, evidenceDigest string
	if err := tx.QueryRowContext(ctx, `SELECT principal_id,source_channel,effect_class,intent_id,authority_refs,identity_evidence_digest FROM actions WHERE action_id=?`, a.ActionID).
		Scan(&actor, &channel, &effectRaw, &storedIntent, &refsRaw, &evidenceDigest); err != nil {
		return AuthorityApprovedStartResult{}, err
	}
	if actor != pending.ActorPrincipalID || storedIntent != pending.IntentID || evidenceDigest != pending.IdentityEvidenceDigest {
		return AuthorityApprovedStartResult{}, ErrAuthorizationSnapshotCorrupt
	}
	// The SAME conversation scope the pending birth resolved under, read from
	// the signed pending snapshot — never the empty conversation, which selects
	// another binding.
	resolved, err := s.resolveAuthorityTx(ctx, tx, actor, channel, pending.ConversationID, operation,
		paramsCell.String, action.EffectClass(effectRaw), at, true)
	if err != nil {
		return AuthorityApprovedStartResult{}, err
	}
	wantRefs, _ := json.Marshal(resolved.refs)
	if storedIntent != resolved.intent.IntentID || refsRaw != string(wantRefs) ||
		!sameStringSlice(pending.PrincipalChain, resolved.principalChain) {
		return AuthorityApprovedStartResult{}, ErrAuthorityRevoked
	}
	for _, account := range resolved.accounts {
		if _, err := s.debitAccountTx(ctx, tx, a.ActionID, account, resolved.operationKey, at); err != nil {
			return AuthorityApprovedStartResult{}, err
		}
	}
	if probe != nil {
		if err := probe(AuthorityProbeAfterDebitBeforeCommit); err != nil {
			return AuthorityApprovedStartResult{}, err
		}
	}
	if err := s.insertAuthorizationStartTx(ctx, tx, a.ActionID, authorityEvidenceEpoch, resolved,
		evidenceDigest, law.Version, law.Digest, at); err != nil {
		return AuthorityApprovedStartResult{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE approvals SET canonical_params='' WHERE approval_id=? AND status=? AND canonical_params!='' AND EXISTS(SELECT 1 FROM actions WHERE action_id=? AND state=?)`,
		approvalID, string(action.ApprovalApproved), a.ActionID, string(action.StateApproved))
	if err != nil {
		return AuthorityApprovedStartResult{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return AuthorityApprovedStartResult{}, ErrApprovalClaimSkipped
	}
	var after sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT canonical_params FROM approvals WHERE approval_id=?`, approvalID).Scan(&after); err != nil || !after.Valid || after.String != "" {
		return AuthorityApprovedStartResult{}, ErrApprovalEvidenceCorrupt
	}
	updated, err := tx.ExecContext(ctx, `UPDATE actions SET state=? WHERE action_id=? AND state=?`,
		string(action.StateAuthorized), a.ActionID, string(action.StateApproved))
	if err != nil {
		return AuthorityApprovedStartResult{}, err
	}
	if rows, err := updated.RowsAffected(); err != nil || rows != 1 {
		return AuthorityApprovedStartResult{}, ErrApprovalClaimSkipped
	}
	if probe != nil {
		if err := probe(AuthorityProbeBeforeCommit); err != nil {
			return AuthorityApprovedStartResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return AuthorityApprovedStartResult{}, mapAuthorityStoreError(err)
	}
	if probe != nil {
		if err := probe(AuthorityProbeAfterCommitBeforeReturn); err != nil {
			return AuthorityApprovedStartResult{}, err
		}
	}
	return AuthorityApprovedStartResult{ActionID: a.ActionID, Params: []byte(paramsCell.String), Operation: operation}, nil
}

func sameStringSlice(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// StartAuthorization atomically verifies the current chain, debits the intent
// and every grant, records the durable start and returns only after commit.
func (s *Store) StartAuthorization(ctx context.Context, request AuthorityStartRequest) (AuthorityStartResult, error) {
	if request.ResolveEvidence == nil {
		return AuthorityStartResult{}, identity.ErrIdentityEvidenceMissing
	}
	if request.ApprovalID != "" {
		return AuthorityStartResult{}, ErrAuthorizationSnapshotCorrupt
	}
	tx, err := s.beginAuthorityWrite(ctx)
	if err != nil {
		return AuthorityStartResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	actionID := s.authorityNewActionID(authorityEvidenceEpoch)
	actorPrincipalID := request.ActorPrincipalID
	evidence, resolveErr := request.ResolveEvidence(actionID)
	if resolveErr != nil {
		return AuthorityStartResult{}, resolveErr
	}
	if actorPrincipalID != "" && actorPrincipalID != evidence.ActorPrincipalID {
		return AuthorityStartResult{}, ErrIssuerMismatch
	}
	actorPrincipalID = evidence.ActorPrincipalID
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM authorization_starts WHERE action_id=?`, actionID).Scan(&exists); err != nil {
		return AuthorityStartResult{}, err
	}
	if exists != 0 {
		return AuthorityStartResult{}, ErrActionAlreadyStarted
	}
	resolved, err := s.resolveAuthorityTx(ctx, tx, actorPrincipalID, request.Channel,
		request.ConversationID, request.Operation, request.Arguments, request.EffectClass,
		request.At, false)
	if err != nil {
		return AuthorityStartResult{}, err
	}
	for _, account := range resolved.accounts {
		before, err := s.debitAccountTx(ctx, tx, actionID, account, resolved.operationKey, request.At)
		if err != nil {
			return AuthorityStartResult{}, err
		}
		if before != nil && (resolved.remainingBefore == nil || *before < *resolved.remainingBefore) {
			v := *before
			resolved.remainingBefore = &v
		}
	}
	if request.Probe != nil {
		if err := request.Probe(AuthorityProbeAfterDebitBeforeCommit); err != nil {
			return AuthorityStartResult{}, err
		}
	}
	correlationID := request.CorrelationID
	if correlationID == "" {
		correlationID = actionID
	}
	protocol := request.SourceProtocol
	if protocol == "" {
		protocol = "authority"
	}
	env := action.NewEnvelope(actionID, correlationID,
		action.Source{Kind: "agent_brain", Protocol: protocol, Channel: request.Channel},
		request.Operation, request.Arguments, request.At)
	env.Effect = action.Effect{Class: string(request.EffectClass)}
	env.Principal = action.PrincipalRef{PrincipalID: actorPrincipalID}
	env.IntentID = resolved.intent.IntentID
	env.AuthorityRefs = append([]string(nil), resolved.refs...)
	env.Principal.EvidenceID = evidence.EvidenceID
	env.Principal.ResponsibleHumanID = evidence.ResponsiblePrincipalID
	if err := validateResolvedEvidenceTx(ctx, tx, evidence, env, request.At.UTC()); err != nil {
		return AuthorityStartResult{}, err
	}
	signed, err := s.signEvidenceTx(ctx, tx, evidence)
	if err != nil {
		return AuthorityStartResult{}, err
	}
	identityDigest := signed.Digest
	if _, err = tx.ExecContext(ctx, `INSERT INTO actions(action_id,schema_version,correlation_id,source_kind,source_protocol,source_channel,op_namespace,op_name,op_version,parameters_digest,effect_class,state,requested_at,principal_id,intent_id,authority_refs,identity_version,identity_evidence_digest,identity_canonical_evidence,identity_signing_key_id,identity_signature) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,2,?,?,?,?)`, env.ActionID, env.SchemaVersion, env.CorrelationID, env.Source.Kind, env.Source.Protocol, env.Source.Channel, env.Operation.Namespace, env.Operation.Name, env.Operation.Version, env.ParametersDigest, env.Effect.Class, string(action.StateAuthorized), env.RequestedAt.UTC().Format(time.RFC3339Nano), actorPrincipalID, resolved.intent.IntentID, authorityRefsValue(resolved.refs), signed.Digest, signed.Canonical, signed.SigningKeyID, signed.Signature); err != nil {
		return AuthorityStartResult{}, err
	}
	if err := insertEvidenceTx(ctx, tx, signed); err != nil {
		return AuthorityStartResult{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO action_decisions(action_id,outcome,rule,decided_at,policy_version,policy_digest) VALUES(?, 'allow','authority',?,?,?)`, actionID, request.At.UTC().Format(time.RFC3339Nano), request.PolicyVersion, request.PolicyDigest); err != nil {
		return AuthorityStartResult{}, err
	}
	if err := s.insertAuthorizationStartTx(ctx, tx, actionID, authorityEvidenceEpoch, resolved,
		identityDigest, request.PolicyVersion, request.PolicyDigest, request.At); err != nil {
		return AuthorityStartResult{}, err
	}
	budgetKind := action.AuthorizationBudgetUnlimited
	if resolved.remainingBefore != nil {
		budgetKind = action.AuthorizationBudgetFinite
	}
	snapshot := action.AuthorizationSnapshotV1{
		Kind: action.AuthorizationSnapshotStart, ActionID: actionID,
		ConversationID:       request.ConversationID,
		RequesterPrincipalID: evidence.RequesterPrincipalID,
		ActorPrincipalID:     actorPrincipalID, IdentityEvidenceDigest: identityDigest,
		IntentID: resolved.intent.IntentID, IntentVersion: resolved.intent.Version, IntentDigest: resolved.intent.Digest(),
		IntentPurpose: resolved.intent.Purpose, PrincipalChain: resolved.principalChain,
		BudgetKind: budgetKind, BudgetRemaining: resolved.remainingBefore, RecordedAt: request.At,
	}
	if err := s.insertAuthorizationSnapshotTx(ctx, tx, snapshot, identityDigest); err != nil {
		return AuthorityStartResult{}, err
	}
	if request.Probe != nil {
		if err := request.Probe(AuthorityProbeBeforeCommit); err != nil {
			return AuthorityStartResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return AuthorityStartResult{}, mapAuthorityStoreError(err)
	}
	if request.Probe != nil {
		if err := request.Probe(AuthorityProbeAfterCommitBeforeReturn); err != nil {
			return AuthorityStartResult{}, err
		}
	}
	return AuthorityStartResult{ActionID: actionID, IntentID: resolved.intent.IntentID, AuthorityRefs: resolved.refs, RemainingBefore: resolved.remainingBefore, Evidence: &evidence}, nil
}

// authorityEvidenceEpoch is the "1" inside every strict action id
// (`act3_1_<random>`) and the `generation` column of authorization_starts.
//
// It is a FIXED EPOCH in this phase, and it is a constant so that nothing can
// read it as more: it is NOT a database generation, it is not read inside the
// writer transaction, and it never moves. The earlier shape of this code was a
// function taking the context and the transaction and returning 1 with an error
// that could not happen — a transactional read in costume.
//
// FILED for the next phase, by name — "the mutable authority generation read
// inside the writer transaction": a generation row that rotation advances, read
// under write ownership before the id is minted, so that a closed generation
// can refuse an id minted under it. Until that lands, no comment, spec or canto
// may describe this value as read, current, or transactional.
const authorityEvidenceEpoch int64 = 1

func (s *Store) insertAuthorizationSnapshotTx(ctx context.Context, tx *sql.Tx, snapshot action.AuthorizationSnapshotV1, evidenceDigest string) error {
	if err := snapshot.Validate(); err != nil || evidenceDigest == "" || snapshot.IdentityEvidenceDigest != evidenceDigest {
		return ErrAuthorizationSnapshotCorrupt
	}
	canonical := snapshot.CanonicalBytes()
	seal, err := s.signAuthorityTx(ctx, tx, action.AuthorizationSnapshotV1Domain, canonical)
	if err != nil {
		return err
	}
	chain, err := json.Marshal(snapshot.PrincipalChain)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO authorization_snapshots(
		action_id,context_version,requester_principal_id,actor_principal_id,evidence_digest,
		intent_id,intent_version,intent_digest,canonical_context,authorization_digest,
		snapshot_kind,approval_id,intent_purpose,principal_chain,budget_kind,budget_remaining,
		recorded_at,signing_key_id,signature) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		snapshot.ActionID, 1, snapshot.RequesterPrincipalID, snapshot.ActorPrincipalID,
		evidenceDigest, snapshot.IntentID, snapshot.IntentVersion, snapshot.IntentDigest,
		canonical, seal.Digest, string(snapshot.Kind), snapshot.ApprovalID, snapshot.IntentPurpose,
		string(chain), string(snapshot.BudgetKind), snapshot.BudgetRemaining,
		snapshot.RecordedAt.UTC().Format(time.RFC3339Nano), seal.SigningKeyID, seal.Signature)
	return err
}

// resolveAuthorityTx reads every protected input through the transaction that
// already owns the SQLite writer. It does not debit; pending approval birth
// uses it for a non-consuming snapshot and StartAuthorization debits the
// returned stable accounts before it commits a capability.
func (s *Store) resolveAuthorityTx(ctx context.Context, tx *sql.Tx, actor, channel,
	conversation string, operation action.Operation, arguments string,
	effect action.EffectClass, at time.Time, approved bool) (resolvedAuthority, error) {
	binding, err := bindingAuthorityTx(ctx, tx, actor, channel, conversation)
	if err != nil {
		return resolvedAuthority{}, err
	}
	intent, err := s.activeIntentTx(ctx, tx, binding.IntentID, binding.IntentVersion, binding.IntentDigest, at)
	if err != nil {
		return resolvedAuthority{}, err
	}
	if s.authorityActivationDigest != "" {
		if intent.ProfileID != s.authorityProfileID {
			return resolvedAuthority{}, ErrAuthorizationSnapshotCorrupt
		}
		if err := s.verifyAuthorityActivationTx(ctx, tx, s.authorityProfileID, s.authorityActivationDigest); err != nil {
			return resolvedAuthority{}, err
		}
	}
	opRef := action.OperationRef(operation)
	opKey := fmt.Sprintf("%s/%s@%d", opRef.Namespace, opRef.Name, opRef.Version)
	if !operationSubsetSQLite([]action.OperationRef{opRef}, intent.Operations) ||
		!containsAuthorityEffect(intent.EffectClasses, effect) {
		return resolvedAuthority{}, action.ErrAttenuationViolated
	}
	if intent.Approval.Required && !approved {
		return resolvedAuthority{}, ErrAuthorityApprovalRequired
	}
	resolved := resolvedAuthority{
		intent: intent, operationKey: opKey,
		accounts: []string{budgetAccountID(intent.ProfileID, "intent", intent.IntentID)},
	}
	if err := s.ensureBudgetAccountTx(ctx, tx, intent.ProfileID, "intent", intent.IntentID, intent.Budget, at); err != nil {
		return resolvedAuthority{}, err
	}
	if binding.GrantID == "" {
		if s.authorityActivationDigest == "" || operation.Namespace != "tool" {
			return resolvedAuthority{}, ErrAuthorityMissing
		}
		clause, err := s.configAuthorityClauseTx(ctx, tx, intent.ProfileID, actor, operation.Name, channel)
		if err != nil {
			return resolvedAuthority{}, err
		}
		resolved.refs = []string{clause.ClauseID}
		if err := tx.QueryRowContext(ctx, `SELECT generation FROM config_authority_heads
			WHERE profile_id=? AND brain_principal_id=?`, intent.ProfileID, actor).
			Scan(&resolved.configGeneration); err != nil {
			return resolvedAuthority{}, authorityReadFailure(ctx, err, action.ErrAuthorityEvidenceCorrupt)
		}
		resolved.principalChain = []string{intent.OwnerPrincipalID, actor}
		if err := s.ensureBudgetAccountTx(ctx, tx, intent.ProfileID, "config",
			configAccountScope(actor, operation.Name), action.IntentBudgetV2{}, at); err != nil {
			return resolvedAuthority{}, err
		}
		resolved.accounts = append(resolved.accounts,
			budgetAccountID(intent.ProfileID, "config", configAccountScope(actor, operation.Name)))
	} else {
		chain, err := s.authorityChainTx(ctx, tx, binding.GrantID, binding.GrantVersion, at)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return resolvedAuthority{}, ErrAuthorityMissing
			}
			return resolvedAuthority{}, err
		}
		if len(chain) == 0 || chain[len(chain)-1].signed.Grant.Digest() != binding.GrantDigest {
			return resolvedAuthority{}, action.ErrAuthorityEvidenceCorrupt
		}
		if err := validateStoredAuthorityChain(chain, intent); err != nil {
			return resolvedAuthority{}, err
		}
		resolved.chain = chain
		originActor, administrative, err := grantOriginActorTx(ctx, tx,
			chain[0].signed.Grant.GrantID, chain[0].signed.Grant.Version)
		if err != nil {
			return resolvedAuthority{}, err
		}
		if administrative {
			resolved.principalChain = append(resolved.principalChain, originActor)
		}
		for _, stored := range chain {
			grant := stored.signed.Grant
			if err := s.ensureBudgetAccountTx(ctx, tx, grant.ProfileID, "grant", grant.GrantID, grant.Budget, at); err != nil {
				return resolvedAuthority{}, err
			}
			if !operationSubsetSQLite([]action.OperationRef{opRef}, grant.Operations) ||
				!containsAuthorityString(grant.Channels, channel) ||
				!containsAuthorityEffect(grant.EffectClasses, effect) ||
				(grant.EffectCeiling != "" && effect.Rank() > grant.EffectCeiling.Rank()) {
				return resolvedAuthority{}, action.ErrAttenuationViolated
			}
			if grant.Approval.Required && !approved {
				return resolvedAuthority{}, ErrAuthorityApprovalRequired
			}
			resolved.refs = append(resolved.refs, grant.GrantID)
			if len(resolved.principalChain) == 0 ||
				resolved.principalChain[len(resolved.principalChain)-1] != grant.IssuerPrincipalID {
				resolved.principalChain = append(resolved.principalChain, grant.IssuerPrincipalID)
			}
			if resolved.principalChain[len(resolved.principalChain)-1] != grant.SubjectPrincipalID {
				resolved.principalChain = append(resolved.principalChain, grant.SubjectPrincipalID)
			}
			resolved.accounts = append(resolved.accounts, budgetAccountID(grant.ProfileID, "grant", grant.GrantID))
		}
		if chain[len(chain)-1].signed.Grant.SubjectPrincipalID != actor {
			return resolvedAuthority{}, ErrIssuerMismatch
		}
	}
	if err := validateAuthorityActualUse(intent, resolved.chain, operation.Name, arguments); err != nil {
		return resolvedAuthority{}, err
	}
	for _, account := range resolved.accounts {
		remaining, err := s.remainingBeforeAccountTx(ctx, tx, account, opKey)
		if err != nil {
			return resolvedAuthority{}, err
		}
		if remaining != nil && (resolved.remainingBefore == nil || *remaining < *resolved.remainingBefore) {
			value := *remaining
			resolved.remainingBefore = &value
		}
	}
	return resolved, nil
}

func grantOriginActorTx(ctx context.Context, tx *sql.Tx, grantID string,
	version int) (string, bool, error) {
	var actor string
	var administrative int
	if err := tx.QueryRowContext(ctx, `SELECT actor_principal_id,administrative
		FROM grant_events WHERE grant_id=? AND grant_version=? AND revision=1`,
		grantID, version).Scan(&actor, &administrative); err != nil {
		return "", false, authorityReadFailure(ctx, err, action.ErrAuthorityEvidenceCorrupt)
	}
	return actor, administrative == 1, nil
}

func validateStoredAuthorityChain(chain []storedGrantV2, intent action.IntentContractV2) error {
	if len(chain) == 0 {
		return action.ErrAuthorityEvidenceCorrupt
	}
	root, err := normalizeRootAuthority(chain[0].signed.Grant, intent)
	if err != nil {
		return err
	}
	if !bytes.Equal(root.CanonicalBytes(), chain[0].signed.Grant.CanonicalBytes()) {
		return action.ErrAuthorityEvidenceCorrupt
	}
	for i := 1; i < len(chain); i++ {
		parent, child := chain[i-1].signed.Grant, chain[i].signed.Grant
		normalized, err := action.NormalizeAuthorityDelegation(parent, child, intent,
			action.AuthorityBudgetRemaining{
				Total: parent.Budget.Total, PerOperation: parent.Budget.PerOperation,
			}, defaultAuthorityMatchers())
		if err != nil {
			return err
		}
		if !bytes.Equal(normalized.CanonicalBytes(), child.CanonicalBytes()) {
			return action.ErrAuthorityEvidenceCorrupt
		}
	}
	return nil
}

func validateAuthorityActualUse(intent action.IntentContractV2, chain []storedGrantV2, operation, arguments string) error {
	registry := action.NewOperationUseRegistry()
	if err := action.RegisterBuiltInOperationUse(registry); err != nil {
		return err
	}
	use, err := registry.Analyze(operation, arguments)
	if err != nil && !errors.Is(err, action.ErrOperationUseNoAnalyzer) {
		// An analyzer IS registered and could not resolve these arguments:
		// they cannot be proved inside any grant, whatever the terms say.
		return action.ErrAuthorityUseUnresolved
	}
	if err != nil {
		restricted := len(intent.AllowedResources) != 0 || len(intent.DeniedResources) != 0 ||
			len(intent.DataScope) != 0 || len(intent.OutputDestinations) != 0
		for _, stored := range chain {
			grant := stored.signed.Grant
			restricted = restricted || len(grant.AllowedResources) != 0 || len(grant.DeniedResources) != 0 ||
				len(grant.AllowedData) != 0 || len(grant.DeniedData) != 0 || len(grant.OutputDestinations) != 0
		}
		if restricted {
			return action.ErrAuthorityUseUnresolved
		}
		use = action.OperationUse{}
	}
	matchers := defaultAuthorityMatchers()
	intentTerms := action.AuthorityGrantV2{
		AllowedResources: intent.AllowedResources, DeniedResources: intent.DeniedResources,
		AllowedData: intent.DataScope, OutputDestinations: intent.OutputDestinations,
	}
	if err := action.ValidateAuthorityUse(intentTerms, use, matchers); err != nil {
		return err
	}
	for _, stored := range chain {
		if err := action.ValidateAuthorityUse(stored.signed.Grant, use, matchers); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) remainingBeforeAccountTx(ctx context.Context, tx *sql.Tx, account, operation string) (*int64, error) {
	var maxTotal sql.NullInt64
	var rawPer string
	if err := tx.QueryRowContext(ctx, `SELECT max_total,per_operation FROM budget_accounts WHERE account_id=?`, account).Scan(&maxTotal, &rawPer); err != nil {
		return nil, err
	}
	var per map[string]int64
	if err := json.Unmarshal([]byte(rawPer), &per); err != nil {
		return nil, ErrBudgetEvidenceCorrupt
	}
	var remaining *int64
	if maxTotal.Valid {
		spent, sequence, tail, err := counterTx(ctx, tx, account, "*")
		if err != nil {
			return nil, err
		}
		if err := s.verifyDebitTailTx(ctx, tx, account, "*", spent, sequence, tail); err != nil {
			return nil, err
		}
		value := maxTotal.Int64 - spent
		if value < 0 {
			return nil, ErrBudgetEvidenceCorrupt
		}
		remaining = &value
	}
	if maximum, ok := per[operation]; ok {
		spent, sequence, tail, err := counterTx(ctx, tx, account, operation)
		if err != nil {
			return nil, err
		}
		if err := s.verifyDebitTailTx(ctx, tx, account, operation, spent, sequence, tail); err != nil {
			return nil, err
		}
		value := maximum - spent
		if value < 0 {
			return nil, ErrBudgetEvidenceCorrupt
		}
		if remaining == nil || value < *remaining {
			remaining = &value
		}
	}
	return remaining, nil
}

type bindingAuthority struct {
	IntentID, IntentDigest, GrantID, GrantDigest string
	IntentVersion, GrantVersion                  int
}

func bindingAuthorityTx(ctx context.Context, tx *sql.Tx, actor, channel, conversation string) (bindingAuthority, error) {
	var b bindingAuthority
	err := tx.QueryRowContext(ctx, `SELECT intent_id,intent_version,intent_digest,COALESCE(grant_id,''),COALESCE(grant_version,0),COALESCE(grant_digest,'') FROM execution_bindings WHERE actor_principal_id=? AND channel=? AND status='ACTIVE' AND (conversation_id=? OR conversation_id IS NULL) ORDER BY conversation_id IS NOT NULL DESC LIMIT 1`, actor, channel, conversation).Scan(&b.IntentID, &b.IntentVersion, &b.IntentDigest, &b.GrantID, &b.GrantVersion, &b.GrantDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return bindingAuthority{}, ErrAuthorityMissing
	}
	if err == nil {
		empty := b.GrantID == "" && b.GrantVersion == 0 && b.GrantDigest == ""
		complete := b.GrantID != "" && b.GrantVersion > 0 && b.GrantDigest != ""
		if !empty && !complete {
			return bindingAuthority{}, action.ErrAuthorityEvidenceCorrupt
		}
	}
	return b, err
}
func (s *Store) authorityChainTx(ctx context.Context, tx *sql.Tx, id string, version int, at time.Time) ([]storedGrantV2, error) {
	if id == "" {
		return nil, action.ErrAuthorityEvidenceCorrupt
	}
	var reversed []storedGrantV2
	seen := map[string]bool{}
	for id != "" {
		if seen[id] || len(reversed) >= 16 {
			return nil, action.ErrAuthorityEvidenceCorrupt
		}
		seen[id] = true
		g, err := s.readGrantTx(ctx, tx, id, version)
		if err != nil {
			// The FIRST link absent means the named authority does not exist,
			// and callers name that. An ANCESTOR absent is a different fact: a
			// signed leaf exists and points at a parent version that does not,
			// which is a broken chain. Reporting it as "missing" would send an
			// operator to issue authority when the evidence is what is wrong.
			if errors.Is(err, sql.ErrNoRows) && len(reversed) > 0 {
				return nil, action.ErrAuthorityEvidenceCorrupt
			}
			return nil, err
		}
		if g.status == action.LifecycleRevoked {
			return nil, ErrAuthorityRevoked
		}
		if g.status != action.LifecycleActive {
			return nil, ErrAuthorityInactive
		}
		if at.Before(g.signed.Grant.ValidFrom) || !at.Before(g.signed.Grant.ExpiresAt) {
			return nil, ErrAuthorityExpired
		}
		reversed = append(reversed, g)
		id, version = g.signed.Grant.ParentGrantID, g.signed.Grant.ParentGrantVersion
	}
	chain := make([]storedGrantV2, len(reversed))
	for i := range reversed {
		chain[len(reversed)-1-i] = reversed[i]
	}
	return chain, nil
}
func containsAuthorityString(values []string, want string) bool {
	for _, v := range values {
		if v == want || v == "*" {
			return true
		}
	}
	return false
}
func containsAuthorityEffect(values []action.EffectClass, want action.EffectClass) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func counterTx(ctx context.Context, tx *sql.Tx, account, operation string) (spent, sequence int64, tail string, err error) {
	err = tx.QueryRowContext(ctx, `SELECT spent,sequence,tail_digest FROM budget_counters WHERE account_id=? AND operation_key=?`, account, operation).Scan(&spent, &sequence, &tail)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, "", nil
	}
	return
}
func (s *Store) debitAccountTx(ctx context.Context, tx *sql.Tx, actionID, account, opKey string, at time.Time) (*int64, error) {
	var maxTotal sql.NullInt64
	var rawPer string
	if err := tx.QueryRowContext(ctx, `SELECT max_total,per_operation FROM budget_accounts WHERE account_id=?`, account).Scan(&maxTotal, &rawPer); err != nil {
		return nil, err
	}
	var per map[string]int64
	if err := json.Unmarshal([]byte(rawPer), &per); err != nil {
		return nil, ErrBudgetEvidenceCorrupt
	}
	totalSpent, totalSeq, totalTail, err := counterTx(ctx, tx, account, "*")
	if err != nil {
		return nil, err
	}
	if err := s.verifyDebitTailTx(ctx, tx, account, "*", totalSpent, totalSeq, totalTail); err != nil {
		return nil, err
	}
	var remaining *int64
	if maxTotal.Valid {
		v := maxTotal.Int64 - totalSpent
		if v <= 0 {
			return nil, ErrBudgetExhausted
		}
		remaining = &v
	}
	if err := s.appendDebitTx(ctx, tx, actionID, account, "*", totalSpent, totalSeq, totalTail, at); err != nil {
		return nil, err
	}
	if maximum, ok := per[opKey]; ok {
		spent, seq, tail, err := counterTx(ctx, tx, account, opKey)
		if err != nil {
			return nil, err
		}
		if err := s.verifyDebitTailTx(ctx, tx, account, opKey, spent, seq, tail); err != nil {
			return nil, err
		}
		if spent >= maximum {
			return nil, ErrBudgetExhausted
		}
		operationRemaining := maximum - spent
		if remaining == nil || operationRemaining < *remaining {
			value := operationRemaining
			remaining = &value
		}
		if err := s.appendDebitTx(ctx, tx, actionID, account, opKey, spent, seq, tail, at); err != nil {
			return nil, err
		}
	}
	return remaining, nil
}
func (s *Store) verifyDebitTailTx(ctx context.Context, tx *sql.Tx, account, operation string,
	spent, sequence int64, tail string) error {
	if sequence == 0 {
		var count int
		if spent != 0 || tail != "" {
			return ErrBudgetEvidenceCorrupt
		}
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM budget_debits WHERE account_id=? AND operation_key=?`,
			account, operation).Scan(&count); err != nil {
			return authorityReadFailure(ctx, err, ErrBudgetEvidenceCorrupt)
		}
		if count != 0 {
			return ErrBudgetEvidenceCorrupt
		}
		return nil
	}
	var actionID, prior, digest, keyID, signature string
	var rowSequence, cumulative int64
	var canonical []byte
	if err := tx.QueryRowContext(ctx, `SELECT action_id,sequence,previous_digest,
		cumulative_spent,canonical_debit,digest,signing_key_id,signature
		FROM budget_debits WHERE account_id=? AND operation_key=?
		ORDER BY sequence DESC LIMIT 1`, account, operation).
		Scan(&actionID, &rowSequence, &prior, &cumulative, &canonical, &digest,
			&keyID, &signature); err != nil {
		return authorityReadFailure(ctx, err, ErrBudgetEvidenceCorrupt)
	}
	var record authorityDebitRecord
	if err := json.Unmarshal(canonical, &record); err != nil ||
		record.ActionID != actionID || record.AccountID != account ||
		record.Operation != operation || record.Sequence != rowSequence ||
		record.Cumulative != cumulative || record.Previous != prior ||
		rowSequence != sequence || cumulative != spent || digest != tail ||
		!bytes.Equal(canonical, mustAuthorityJSON(record)) {
		return ErrBudgetEvidenceCorrupt
	}
	if _, err := time.Parse(time.RFC3339Nano, record.At); err != nil {
		return ErrBudgetEvidenceCorrupt
	}
	pub, _, err := publicKeyTx(ctx, tx, keyID)
	if err != nil {
		return authorityReadFailure(ctx, err, ErrBudgetEvidenceCorrupt)
	}
	if action.VerifyAuthorityBytes(pub, authorityDebitDomain, canonical,
		action.AuthoritySignature{Digest: digest, SigningKeyID: keyID,
			Signature: signature}) != nil {
		return ErrBudgetEvidenceCorrupt
	}
	var headCanonical []byte
	var headDigest, headKeyID, headSignature string
	if err := tx.QueryRowContext(ctx, `SELECT canonical_counter,digest,signing_key_id,signature
		FROM budget_counters WHERE account_id=? AND operation_key=?`, account, operation).
		Scan(&headCanonical, &headDigest, &headKeyID, &headSignature); err != nil {
		return authorityReadFailure(ctx, err, ErrBudgetEvidenceCorrupt)
	}
	head := authorityBudgetHead{AccountID: account, OperationKey: operation,
		Spent: spent, Sequence: sequence, TailDigest: tail}
	pub, _, err = publicKeyTx(ctx, tx, headKeyID)
	if err != nil {
		return authorityReadFailure(ctx, err, ErrBudgetEvidenceCorrupt)
	}
	if !bytes.Equal(headCanonical, mustAuthorityJSON(head)) ||
		action.VerifyAuthorityBytes(pub, authorityBudgetHeadDomain, headCanonical,
			action.AuthoritySignature{Digest: headDigest, SigningKeyID: headKeyID,
				Signature: headSignature}) != nil {
		return ErrBudgetEvidenceCorrupt
	}
	return nil
}
func (s *Store) appendDebitTx(ctx context.Context, tx *sql.Tx, actionID, account, operation string, spent, sequence int64, tail string, at time.Time) error {
	canonical := canonicalAuthorityDebit(actionID, account, operation, sequence+1, spent+1, tail, at)
	seal, err := s.signAuthorityTx(ctx, tx, authorityDebitDomain, canonical)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO budget_debits(action_id,account_id,operation_key,sequence,previous_digest,cumulative_spent,canonical_debit,digest,signing_key_id,signature) VALUES(?,?,?,?,?,?,?,?,?,?)`, actionID, account, operation, sequence+1, tail, spent+1, canonical, seal.Digest, seal.SigningKeyID, seal.Signature); err != nil {
		return err
	}
	return s.upsertBudgetCounterTx(ctx, tx, account, operation, spent+1, sequence+1, seal.Digest)
}

func (s *Store) upsertBudgetCounterTx(ctx context.Context, tx *sql.Tx, account,
	operation string, spent, sequence int64, tail string) error {
	head := authorityBudgetHead{AccountID: account, OperationKey: operation,
		Spent: spent, Sequence: sequence, TailDigest: tail}
	canonical := mustAuthorityJSON(head)
	seal, err := s.signAuthorityTx(ctx, tx, authorityBudgetHeadDomain, canonical)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO budget_counters(account_id,operation_key,
		spent,sequence,tail_digest,canonical_counter,digest,signing_key_id,signature)
		VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(account_id,operation_key) DO UPDATE SET
		spent=excluded.spent,sequence=excluded.sequence,tail_digest=excluded.tail_digest,
		canonical_counter=excluded.canonical_counter,digest=excluded.digest,
		signing_key_id=excluded.signing_key_id,signature=excluded.signature`,
		account, operation, spent, sequence, tail, canonical, seal.Digest,
		seal.SigningKeyID, seal.Signature)
	return err
}
func resolvedGrantChain(resolved resolvedAuthority) string {
	refs := make([]authorityGrantStartRef, 0, len(resolved.refs))
	if len(resolved.chain) == 0 {
		for _, ref := range resolved.refs {
			refs = append(refs, authorityGrantStartRef{
				GrantID: ref, Version: 1, Digest: ref, Revision: 1,
			})
		}
	} else {
		for _, stored := range resolved.chain {
			refs = append(refs, authorityGrantStartRef{
				GrantID:  stored.signed.Grant.GrantID,
				Version:  stored.signed.Grant.Version,
				Digest:   stored.signed.Digest,
				Revision: stored.revision,
			})
		}
	}
	raw, _ := json.Marshal(refs)
	return string(raw)
}

func debitSetDigestTx(ctx context.Context, tx *sql.Tx, actionID string) (string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT account_id,operation_key,sequence,digest
		FROM budget_debits WHERE action_id=? ORDER BY account_id,operation_key`, actionID)
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	refs := make([]authorityDebitRef, 0)
	for rows.Next() {
		var ref authorityDebitRef
		if err := rows.Scan(&ref.AccountID, &ref.OperationKey, &ref.Sequence, &ref.Digest); err != nil {
			return "", err
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil || len(refs) == 0 {
		return "", ErrBudgetEvidenceCorrupt
	}
	raw, _ := json.Marshal(refs)
	return action.HashCanonical(string(raw)), nil
}

func (s *Store) insertAuthorizationStartTx(ctx context.Context, tx *sql.Tx,
	actionID string, generation int64, resolved resolvedAuthority, identityDigest string,
	policyVersion int64, policyDigest string, at time.Time) error {
	debitSetDigest, err := debitSetDigestTx(ctx, tx, actionID)
	if err != nil {
		return err
	}
	grantChain := resolvedGrantChain(resolved)
	var outcome, rule, decidedAt, storedPolicyDigest string
	var storedPolicyVersion int64
	if err := tx.QueryRowContext(ctx, `SELECT outcome,rule,decided_at,policy_version,
		policy_digest FROM action_decisions WHERE action_id=?`, actionID).
		Scan(&outcome, &rule, &decidedAt, &storedPolicyVersion,
			&storedPolicyDigest); err != nil {
		return ErrAuthorizationSnapshotCorrupt
	}
	if storedPolicyVersion != policyVersion || storedPolicyDigest != policyDigest {
		return ErrAuthorizationSnapshotCorrupt
	}
	decisionCanonical, _ := json.Marshal(struct {
		ActionID      string `json:"action_id"`
		Outcome       string `json:"outcome"`
		Rule          string `json:"rule"`
		DecidedAt     string `json:"decided_at"`
		PolicyVersion int64  `json:"policy_version"`
		PolicyDigest  string `json:"policy_digest"`
	}{actionID, outcome, rule, decidedAt, storedPolicyVersion, storedPolicyDigest})
	record := authorityStartRecord{
		ActionID: actionID, Generation: generation,
		IntentID: resolved.intent.IntentID, IntentVersion: resolved.intent.Version,
		IntentDigest: resolved.intent.Digest(), GrantChain: grantChain,
		DebitSetDigest: debitSetDigest, IdentityEvidenceDigest: identityDigest,
		ConfigGeneration: resolved.configGeneration,
		DecisionDigest:   action.HashCanonical(string(decisionCanonical)),
		PolicyVersion:    policyVersion, PolicyDigest: policyDigest,
		AuthorizationTime: at.UTC().Format(time.RFC3339Nano),
	}
	canonical := mustAuthorityJSON(record)
	seal, err := s.signAuthorityTx(ctx, tx, authorityStartDomain, canonical)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO authorization_starts(action_id,generation,
		intent_id,intent_version,intent_digest,grant_chain,debit_set_digest,
		authorization_time,canonical_start,digest,signing_key_id,signature)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, actionID, generation, record.IntentID,
		record.IntentVersion, record.IntentDigest, grantChain, debitSetDigest,
		record.AuthorizationTime, canonical, seal.Digest, seal.SigningKeyID,
		seal.Signature); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return ErrActionAlreadyStarted
		}
		return err
	}
	return nil
}

func (s *Store) verifyAuthorizationStartsTx(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT action_id,generation,intent_id,intent_version,
		intent_digest,grant_chain,debit_set_digest,authorization_time,canonical_start,
		digest,signing_key_id,signature FROM authorization_starts ORDER BY action_id`)
	if err != nil {
		return authorityReadFailure(ctx, err, ErrAuthorizationSnapshotCorrupt)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var record authorityStartRecord
		var canonical []byte
		var digest, keyID, signature string
		if err := rows.Scan(&record.ActionID, &record.Generation, &record.IntentID,
			&record.IntentVersion, &record.IntentDigest, &record.GrantChain,
			&record.DebitSetDigest, &record.AuthorizationTime, &canonical,
			&digest, &keyID, &signature); err != nil {
			return authorityReadFailure(ctx, err, ErrAuthorizationSnapshotCorrupt)
		}
		var signedRecord authorityStartRecord
		if err := json.Unmarshal(canonical, &signedRecord); err != nil ||
			signedRecord.ActionID != record.ActionID ||
			signedRecord.Generation != record.Generation ||
			signedRecord.IntentID != record.IntentID ||
			signedRecord.IntentVersion != record.IntentVersion ||
			signedRecord.IntentDigest != record.IntentDigest ||
			signedRecord.GrantChain != record.GrantChain ||
			signedRecord.DebitSetDigest != record.DebitSetDigest ||
			signedRecord.AuthorizationTime != record.AuthorizationTime ||
			signedRecord.IdentityEvidenceDigest == "" || signedRecord.DecisionDigest == "" ||
			!bytes.Equal(canonical, mustAuthorityJSON(signedRecord)) {
			return ErrAuthorizationSnapshotCorrupt
		}
		if _, err := time.Parse(time.RFC3339Nano, record.AuthorizationTime); err != nil {
			return ErrAuthorizationSnapshotCorrupt
		}
		debitDigest, err := debitSetDigestTx(ctx, tx, record.ActionID)
		if err != nil {
			return authorityReadFailure(ctx, err, ErrAuthorizationSnapshotCorrupt)
		}
		if debitDigest != record.DebitSetDigest {
			return ErrAuthorizationSnapshotCorrupt
		}
		pub, _, err := publicKeyTx(ctx, tx, keyID)
		if err != nil {
			return authorityReadFailure(ctx, err, ErrAuthorizationSnapshotCorrupt)
		}
		if action.VerifyAuthorityBytes(pub, authorityStartDomain, canonical,
			action.AuthoritySignature{Digest: digest, SigningKeyID: keyID,
				Signature: signature}) != nil {
			return ErrAuthorizationSnapshotCorrupt
		}
	}
	if err := rows.Err(); err != nil {
		return authorityReadFailure(ctx, err, ErrAuthorizationSnapshotCorrupt)
	}
	return nil
}

// Ensure the stored grant canonical bytes are not signer-mutated.
func sameAuthorityBytes(a, b []byte) bool { return bytes.Equal(a, b) }

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
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/identity"
)

// SetIdentitySigners wires the profile-key signers used for new evidence and
// principal events. The store verifies every returned object before insert.
func (s *Store) SetIdentitySigners(
	evidence func(identity.Evidence) identity.SignedEvidence,
	event func(identity.PrincipalEvent) identity.SignedPrincipalEvent,
) {
	s.identityEvidenceSigner = evidence
	s.principalEventSigner = event
}

// RegisterIdentity idempotently materializes configured principals and
// bindings. Incompatible existing rows fail without overwrite.
func (s *Store) RegisterIdentity(ctx context.Context, registry identity.Registry, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("action/sqlite: begin identity registration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, configured := range registry.Principals {
		stored, err := principalTx(ctx, tx, configured.ID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			created := configured
			created.CreatedAt = at.UTC()
			created.DisabledAt = time.Time{}
			created.Revision = 1
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO principals(principal_id,kind,display_name,created_at,disabled_at,revision)
				 VALUES(?,?,?,?,NULL,1)`, created.ID, string(created.Kind), created.DisplayName,
				created.CreatedAt.Format(time.RFC3339Nano)); err != nil {
				return fmt.Errorf("action/sqlite: insert principal %q: %w", created.ID, err)
			}
			event := identity.PrincipalEvent{
				EventID: newIdentityID("pev_"), Principal: created, Revision: 1,
				Kind: "created", OccurredAt: at.UTC(),
			}
			if err := s.insertPrincipalEventTx(ctx, tx, event); err != nil {
				return err
			}
		case err != nil:
			return fmt.Errorf("action/sqlite: read principal %q: %w", configured.ID, err)
		default:
			// KIND is identity and is boot-fatal. DisplayName is NOT: the type's
			// own godoc calls it decoration, and the stored row is the one the
			// signed lifecycle event covers, so a renamed label must never stop
			// a boot (the twentieth pass, P2-5). The stored decoration wins and
			// is never overwritten.
			if stored.Kind != configured.Kind {
				return fmt.Errorf("action/sqlite: principal %q conflicts with configured identity", configured.ID)
			}
			if err := verifyPrincipalProjectionTx(ctx, tx, stored); err != nil {
				return err
			}
		}
	}
	for _, configured := range registry.Bindings {
		var stored identity.Binding
		err := tx.QueryRowContext(ctx,
			`SELECT binding_id,provider,channel,credential_ref,subject_namespace,verified_subject,
			        principal_id,generation,status
			   FROM principal_bindings WHERE binding_id=?`, configured.ID).
			Scan(&stored.ID, &stored.Provider, &stored.Channel, &stored.CredentialRef,
				&stored.SubjectNamespace, &stored.VerifiedSubject, &stored.PrincipalID,
				&stored.Generation, &stored.Status)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO principal_bindings(binding_id,provider,channel,credential_ref,
				 subject_namespace,verified_subject,principal_id,generation,status)
				 VALUES(?,?,?,?,?,?,?,?,?)`,
				configured.ID, configured.Provider, configured.Channel, configured.CredentialRef,
				configured.SubjectNamespace, configured.VerifiedSubject, configured.PrincipalID,
				configured.Generation, string(configured.Status)); err != nil {
				return fmt.Errorf("action/sqlite: insert identity binding %q: %w", configured.ID, err)
			}
		case err != nil:
			return fmt.Errorf("action/sqlite: read identity binding %q: %w", configured.ID, err)
		default:
			// CredentialRef is a configuration REFERENCE — the NAME of an
			// environment variable, which an operator renames without touching
			// who anyone is. Comparing it made `token_env: FOO` -> `token_env:
			// FOO_V2` permanently boot-fatal on an existing store, with no
			// documented remedy (the twentieth pass, P2-5). Everything that
			// BINDS an identity — provider, channel, subject namespace, verified
			// subject, principal, generation and status — stays boot-fatal and
			// is never overwritten, and none of it is the reference. The stored
			// reference wins; no row is rewritten here.
			identityBearing := stored
			identityBearing.CredentialRef = configured.CredentialRef
			if identityBearing != configured {
				return fmt.Errorf("action/sqlite: binding %q conflicts with configured identity", configured.ID)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("action/sqlite: commit identity registration: %w", err)
	}
	return nil
}

// DisablePrincipal appends a signed disable event and changes its projection
// in the same transaction. Repeated disable is an idempotent no-op.
func (s *Store) DisablePrincipal(ctx context.Context, principalID string, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("action/sqlite: begin principal disable: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	principal, err := principalTx(ctx, tx, principalID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("action/sqlite: principal %q: %w", principalID, ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("action/sqlite: read principal %q: %w", principalID, err)
	}
	if err := verifyPrincipalProjectionTx(ctx, tx, principal); err != nil {
		return err
	}
	if !principal.DisabledAt.IsZero() {
		return nil
	}
	principal.DisabledAt = at.UTC()
	principal.Revision++
	event := identity.PrincipalEvent{
		EventID: newIdentityID("pev_"), Principal: principal,
		Revision: principal.Revision, Kind: "disabled", OccurredAt: at.UTC(),
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE principals SET disabled_at=?,revision=? WHERE principal_id=?`,
		principal.DisabledAt.Format(time.RFC3339Nano), principal.Revision, principal.ID); err != nil {
		return fmt.Errorf("action/sqlite: disable principal %q: %w", principalID, err)
	}
	if err := s.insertPrincipalEventTx(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("action/sqlite: commit principal disable %q: %w", principalID, err)
	}
	return nil
}

// RecordAttemptAuthenticated persists a Phase 1 action, decision, signed birth
// snapshot, and full identity evidence in one transaction.
func (s *Store) RecordAttemptAuthenticated(ctx context.Context, env action.Envelope, d Decision, state action.State, evidence identity.Evidence) error {
	switch state {
	case action.StateDenied, action.StateShadowed, action.StateAuthorized:
	default:
		return fmt.Errorf("%w: %s", ErrNotADecisionState, state)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("action/sqlite: begin authenticated record: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := validateResolvedEvidenceTx(ctx, tx, evidence, env, s.identityNow().UTC()); err != nil {
		return err
	}
	signed, err := s.signEvidenceTx(ctx, tx, evidence)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO actions (action_id,schema_version,correlation_id,
		 source_kind,source_protocol,source_channel,op_namespace,op_name,op_version,
		 parameters_digest,effect_class,state,requested_at,principal_id,intent_id,authority_refs,
		 identity_version,identity_evidence_digest,identity_canonical_evidence,
		 identity_signing_key_id,identity_signature)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,2,?,?,?,?)`,
		env.ActionID, env.SchemaVersion, env.CorrelationID,
		env.Source.Kind, env.Source.Protocol, env.Source.Channel,
		env.Operation.Namespace, env.Operation.Name, env.Operation.Version,
		env.ParametersDigest, env.Effect.Class, string(state),
		env.RequestedAt.UTC().Format(time.RFC3339Nano), evidence.ActorPrincipalID,
		nullable(env.IntentID), authorityRefsValue(env.AuthorityRefs),
		signed.Digest, signed.Canonical, signed.SigningKeyID, signed.Signature); err != nil {
		return fmt.Errorf("action/sqlite: insert authenticated action %q: %w", env.ActionID, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO action_decisions(action_id,outcome,rule,decided_at,policy_version,policy_digest)
		 VALUES(?,?,?,?,?,?)`, env.ActionID, d.Outcome, d.Rule,
		env.RequestedAt.UTC().Format(time.RFC3339Nano), d.PolicyVersion, d.PolicyDigest); err != nil {
		return fmt.Errorf("action/sqlite: insert authenticated decision %q: %w", env.ActionID, err)
	}
	if err := insertEvidenceTx(ctx, tx, signed); err != nil {
		return err
	}
	if state != action.StateAuthorized {
		receipt, err := s.receiptForRecord(ctx, tx, env, d, state)
		if err != nil {
			return err
		}
		if err := s.appendReceiptTx(ctx, tx, receipt); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("action/sqlite: commit authenticated record %q: %w", env.ActionID, err)
	}
	return s.noteWrite(ctx)
}

// GetIdentityEvidence returns and verifies the signed v2 evidence of one action.
func (s *Store) GetIdentityEvidence(ctx context.Context, actionID string) (identity.SignedEvidence, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return identity.SignedEvidence{}, fmt.Errorf("%w: %v", identity.ErrIdentityEvidenceUnreadable, err)
	}
	defer func() { _ = tx.Rollback() }()
	signed, err := readEvidenceTx(ctx, tx, actionID)
	if err != nil {
		return identity.SignedEvidence{}, err
	}
	pub, _, err := publicKeyTx(ctx, tx, signed.SigningKeyID)
	if err != nil || identity.VerifyEvidence(pub, signed) != nil {
		return identity.SignedEvidence{}, identity.ErrIdentityEvidenceCorrupt
	}
	return signed, nil
}

func (s *Store) signEvidenceTx(ctx context.Context, tx *sql.Tx, evidence identity.Evidence) (identity.SignedEvidence, error) {
	if s.identityEvidenceSigner == nil {
		return identity.SignedEvidence{}, identity.ErrIdentityEvidenceMissing
	}
	signed := s.identityEvidenceSigner(evidence)
	if signed.Evidence != evidence || !bytes.Equal(signed.Canonical, identity.CanonicalEvidence(evidence)) {
		return identity.SignedEvidence{}, identity.ErrSignedObjectMutated
	}
	pub, retired, err := publicKeyTx(ctx, tx, signed.SigningKeyID)
	if err != nil {
		return identity.SignedEvidence{}, identity.ErrIdentityEvidenceCorrupt
	}
	if retired {
		return identity.SignedEvidence{}, identity.ErrSigningKeyRetired
	}
	if err := identity.VerifyEvidence(pub, signed); err != nil {
		return identity.SignedEvidence{}, err
	}
	return signed, nil
}

func (s *Store) insertPrincipalEventTx(ctx context.Context, tx *sql.Tx, event identity.PrincipalEvent) error {
	if s.principalEventSigner == nil {
		return identity.ErrIdentityEvidenceMissing
	}
	signed := s.principalEventSigner(event)
	if signed.Event != event || !bytes.Equal(signed.Canonical, identity.CanonicalPrincipalEvent(event)) {
		return identity.ErrSignedObjectMutated
	}
	pub, retired, err := publicKeyTx(ctx, tx, signed.SigningKeyID)
	if err != nil {
		return identity.ErrIdentityEvidenceCorrupt
	}
	if retired {
		return identity.ErrSigningKeyRetired
	}
	if err := identity.VerifyPrincipalEvent(pub, signed); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO principal_events(event_id,principal_id,revision,kind,occurred_at,
		 canonical_event,digest,signing_key_id,signature) VALUES(?,?,?,?,?,?,?,?,?)`,
		event.EventID, event.Principal.ID, event.Revision, event.Kind,
		event.OccurredAt.UTC().Format(time.RFC3339Nano), signed.Canonical,
		signed.Digest, signed.SigningKeyID, signed.Signature); err != nil {
		return fmt.Errorf("action/sqlite: insert principal event %q: %w", event.EventID, err)
	}
	return nil
}

func validateResolvedEvidenceTx(ctx context.Context, tx *sql.Tx, evidence identity.Evidence, env action.Envelope, at time.Time) error {
	if evidence.ActionID != env.ActionID || evidence.RequestID != env.CorrelationID ||
		evidence.ActorPrincipalID != env.Principal.PrincipalID ||
		evidence.ResponsiblePrincipalID != env.Principal.ResponsibleHumanID ||
		evidence.Provider != env.Source.Channel {
		return identity.ErrIdentityBindingMismatch
	}
	if !evidence.ExpiresAt.IsZero() && !at.UTC().Before(evidence.ExpiresAt) {
		return identity.ErrIdentityEvidenceExpired
	}
	for _, id := range []string{evidence.RequesterPrincipalID, evidence.ActorPrincipalID, evidence.ResponsiblePrincipalID} {
		if id == "" {
			continue
		}
		principal, err := principalTx(ctx, tx, id)
		if err != nil {
			return identity.ErrIdentityEvidenceUnreadable
		}
		if err := verifyPrincipalProjectionTx(ctx, tx, principal); err != nil {
			return err
		}
		if !principal.DisabledAt.IsZero() && !principal.DisabledAt.After(at.UTC()) {
			return identity.ErrPrincipalDisabled
		}
	}
	var binding identity.Binding
	if err := tx.QueryRowContext(ctx,
		`SELECT binding_id,provider,channel,credential_ref,subject_namespace,verified_subject,
		 principal_id,generation,status FROM principal_bindings WHERE binding_id=?`, evidence.BindingID).
		Scan(&binding.ID, &binding.Provider, &binding.Channel, &binding.CredentialRef,
			&binding.SubjectNamespace, &binding.VerifiedSubject, &binding.PrincipalID,
			&binding.Generation, &binding.Status); err != nil {
		return identity.ErrIdentityEvidenceUnreadable
	}
	if binding.Status != identity.BindingActive || binding.Generation != evidence.BindingGeneration ||
		binding.PrincipalID != evidence.RequesterPrincipalID || binding.Provider != evidence.Provider ||
		binding.Channel != evidence.Issuer || binding.SubjectNamespace != evidence.SubjectNamespace ||
		binding.VerifiedSubject != evidence.VerifiedSubject {
		return identity.ErrIdentityBindingMismatch
	}
	return nil
}

func validateActionIdentityTx(ctx context.Context, tx *sql.Tx, actionID string, at time.Time) error {
	stored, err := actionSnapshotTx(ctx, tx, actionID)
	if err != nil {
		return err
	}
	snapshot, principalID := stored.Snapshot, stored.PrincipalID
	if identity.SnapshotIsLegacy(snapshot) {
		return nil
	}
	if snapshot.Version != 2 || snapshot.Digest == "" || len(snapshot.Canonical) == 0 ||
		snapshot.SigningKeyID == "" || snapshot.Signature == "" {
		return identity.ErrIdentityEvidenceCorrupt
	}
	evidence, err := identity.ParseCanonicalEvidence(snapshot.Canonical)
	if err != nil || evidence.ActionID != actionID || evidence.ActorPrincipalID != principalID {
		return identity.ErrIdentityEvidenceCorrupt
	}
	signed := identity.SignedEvidence{Evidence: evidence, Canonical: snapshot.Canonical,
		Digest: snapshot.Digest, SigningKeyID: snapshot.SigningKeyID, Signature: snapshot.Signature}
	pub, _, err := publicKeyTx(ctx, tx, signed.SigningKeyID)
	if err != nil || identity.VerifyEvidence(pub, signed) != nil {
		return identity.ErrIdentityEvidenceCorrupt
	}
	full, err := readEvidenceTx(ctx, tx, actionID)
	if err != nil {
		if errors.Is(err, identity.ErrIdentityEvidenceUnreadable) {
			return err
		}
		return identity.ErrIdentityEvidenceCorrupt
	}
	if full.Evidence != evidence || full.Digest != signed.Digest ||
		full.SigningKeyID != signed.SigningKeyID || full.Signature != signed.Signature ||
		!bytes.Equal(full.Canonical, signed.Canonical) {
		return identity.ErrIdentityEvidenceCorrupt
	}
	if !evidence.ExpiresAt.IsZero() && !at.UTC().Before(evidence.ExpiresAt) {
		return identity.ErrIdentityEvidenceExpired
	}
	// The envelope handed to the shared check is built from the STORED row:
	// its correlation id, its source channel and its principal id, which
	// `RecordAttemptAuthenticated` wrote in the same transaction that signed
	// this evidence. The responsible principal has no second copy on `actions`
	// — it is pinned by the signature over the canonical evidence and by the
	// `full.Evidence != evidence` comparison above, and that is all this line
	// claims about it.
	return validateResolvedEvidenceTx(ctx, tx, evidence, action.Envelope{
		ActionID: actionID, CorrelationID: stored.CorrelationID,
		Source: action.Source{Channel: stored.SourceChannel},
		Principal: action.PrincipalRef{PrincipalID: stored.PrincipalID,
			ResponsibleHumanID: evidence.ResponsiblePrincipalID},
	}, at)
}

func readEvidenceTx(ctx context.Context, tx *sql.Tx, actionID string) (identity.SignedEvidence, error) {
	var (
		e        identity.Evidence
		observed string
		expires  sql.NullString
		signed   identity.SignedEvidence
	)
	err := tx.QueryRowContext(ctx,
		`SELECT evidence_id,action_id,request_id,requester_principal_id,
		 actor_principal_id,coalesce(responsible_principal_id,''),binding_id,binding_generation,
		 adapter_instance_id,provider,method,credential_class,issuer,subject_namespace,
		 verified_subject,subject_claim,observed_at,expires_at,claims_digest,canonical_evidence,
		 evidence_digest,signing_key_id,signature FROM identity_evidence_v2 WHERE action_id=?`, actionID).
		Scan(&e.EvidenceID, &e.ActionID, &e.RequestID, &e.RequesterPrincipalID,
			&e.ActorPrincipalID, &e.ResponsiblePrincipalID, &e.BindingID, &e.BindingGeneration,
			&e.AdapterInstanceID, &e.Provider, &e.Method, &e.CredentialClass, &e.Issuer,
			&e.SubjectNamespace, &e.VerifiedSubject, &e.SubjectClaim, &observed, &expires, &e.ClaimsDigest,
			&signed.Canonical, &signed.Digest, &signed.SigningKeyID, &signed.Signature)
	if errors.Is(err, sql.ErrNoRows) {
		return identity.SignedEvidence{}, identity.ErrIdentityEvidenceCorrupt
	}
	if err != nil {
		return identity.SignedEvidence{}, fmt.Errorf("%w: %v", identity.ErrIdentityEvidenceUnreadable, err)
	}
	if e.ObservedAt, err = time.Parse(time.RFC3339Nano, observed); err != nil {
		return identity.SignedEvidence{}, identity.ErrIdentityEvidenceCorrupt
	}
	if expires.Valid {
		if e.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires.String); err != nil {
			return identity.SignedEvidence{}, identity.ErrIdentityEvidenceCorrupt
		}
	}
	signed.Evidence = e
	return signed, nil
}

func insertEvidenceTx(ctx context.Context, tx *sql.Tx, signed identity.SignedEvidence) error {
	e := signed.Evidence
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO identity_evidence_v2(evidence_id,action_id,request_id,
		 requester_principal_id,actor_principal_id,responsible_principal_id,binding_id,
		 binding_generation,adapter_instance_id,provider,method,credential_class,issuer,
		 subject_namespace,verified_subject,subject_claim,observed_at,expires_at,claims_digest,
		 canonical_evidence,evidence_digest,signing_key_id,signature)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.EvidenceID, e.ActionID, e.RequestID, e.RequesterPrincipalID,
		e.ActorPrincipalID, nullable(e.ResponsiblePrincipalID), e.BindingID,
		e.BindingGeneration, e.AdapterInstanceID, e.Provider, e.Method,
		e.CredentialClass, e.Issuer, e.SubjectNamespace, e.VerifiedSubject, e.SubjectClaim,
		e.ObservedAt.UTC().Format(time.RFC3339Nano), timeCol(e.ExpiresAt),
		e.ClaimsDigest, signed.Canonical, signed.Digest, signed.SigningKeyID,
		signed.Signature); err != nil {
		return fmt.Errorf("action/sqlite: insert identity evidence for %q: %w", e.ActionID, err)
	}
	return nil
}

// storedAction is what the `actions` row itself says about one action's birth:
// the signed identity snapshot plus the THREE fields the evidence must agree
// with. They are read from storage rather than recomputed from the evidence —
// judging `evidence.RequestID` against a request id copied out of that same
// evidence proved nothing, and a rewritten `correlation_id` or `source_channel`
// claimed clean (the twentieth pass, P2-1).
type storedAction struct {
	Snapshot      identity.BirthSnapshot
	PrincipalID   string
	CorrelationID string
	SourceChannel string
}

func actionSnapshotTx(ctx context.Context, tx *sql.Tx, actionID string) (storedAction, error) {
	var stored storedAction
	var principal sql.NullString
	err := tx.QueryRowContext(ctx,
		`SELECT identity_version,identity_evidence_digest,identity_canonical_evidence,
		 identity_signing_key_id,identity_signature,principal_id,correlation_id,source_channel
		   FROM actions WHERE action_id=?`, actionID).
		Scan(&stored.Snapshot.Version, &stored.Snapshot.Digest, &stored.Snapshot.Canonical,
			&stored.Snapshot.SigningKeyID, &stored.Snapshot.Signature, &principal,
			&stored.CorrelationID, &stored.SourceChannel)
	if errors.Is(err, sql.ErrNoRows) {
		return storedAction{}, ErrNotFound
	}
	if err != nil {
		return storedAction{}, fmt.Errorf("%w: %v", identity.ErrIdentityEvidenceUnreadable, err)
	}
	stored.PrincipalID = principal.String
	return stored, nil
}

func principalTx(ctx context.Context, tx *sql.Tx, id string) (identity.Principal, error) {
	var (
		principal identity.Principal
		kind      string
		created   string
		disabled  sql.NullString
	)
	err := tx.QueryRowContext(ctx,
		`SELECT principal_id,kind,display_name,created_at,disabled_at,revision
		 FROM principals WHERE principal_id=?`, id).
		Scan(&principal.ID, &kind, &principal.DisplayName, &created, &disabled, &principal.Revision)
	if err != nil {
		return identity.Principal{}, err
	}
	principal.Kind = identity.PrincipalKind(kind)
	if principal.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return identity.Principal{}, err
	}
	if disabled.Valid {
		if principal.DisabledAt, err = time.Parse(time.RFC3339Nano, disabled.String); err != nil {
			return identity.Principal{}, err
		}
	}
	return principal, nil
}

func verifyPrincipalProjectionTx(ctx context.Context, tx *sql.Tx, principal identity.Principal) error {
	var (
		canonical []byte
		digest    string
		keyID     string
		signature string
		revision  int64
	)
	err := tx.QueryRowContext(ctx,
		`SELECT canonical_event,digest,signing_key_id,signature,revision
		 FROM principal_events WHERE principal_id=? ORDER BY revision DESC LIMIT 1`, principal.ID).
		Scan(&canonical, &digest, &keyID, &signature, &revision)
	if err != nil {
		return identity.ErrPrincipalEvidenceCorrupt
	}
	event, err := identity.ParseCanonicalPrincipalEvent(canonical)
	if err != nil || revision != principal.Revision || event.Revision != principal.Revision ||
		event.Principal != principal {
		return identity.ErrPrincipalEvidenceCorrupt
	}
	pub, _, err := publicKeyTx(ctx, tx, keyID)
	if err != nil {
		return identity.ErrPrincipalEvidenceCorrupt
	}
	if err := identity.VerifyPrincipalEvent(pub, identity.SignedPrincipalEvent{
		Event: event, Canonical: canonical, Digest: digest,
		SigningKeyID: keyID, Signature: signature,
	}); err != nil {
		return identity.ErrPrincipalEvidenceCorrupt
	}
	return nil
}

type receiptIdentityTerms struct {
	present      bool
	status       string
	requester    string
	actor        string
	responsible  string
	evidenceHash string
	snapshotHash string
}

func identityReceiptTermsTx(ctx context.Context, tx *sql.Tx, actionID string, allowCorrupt bool) (receiptIdentityTerms, error) {
	stored, err := actionSnapshotTx(ctx, tx, actionID)
	if err != nil {
		return receiptIdentityTerms{}, err
	}
	snapshot, principalID := stored.Snapshot, stored.PrincipalID
	if identity.SnapshotIsLegacy(snapshot) {
		return receiptIdentityTerms{}, nil
	}
	corrupt := func() (receiptIdentityTerms, error) {
		if !allowCorrupt {
			return receiptIdentityTerms{}, identity.ErrIdentityEvidenceCorrupt
		}
		return receiptIdentityTerms{
			present: true, status: "corrupt",
			snapshotHash: identity.SnapshotDigest(snapshot),
		}, nil
	}
	if snapshot.Version != 2 || snapshot.Digest == "" || len(snapshot.Canonical) == 0 ||
		snapshot.SigningKeyID == "" || snapshot.Signature == "" {
		return corrupt()
	}
	evidence, err := identity.ParseCanonicalEvidence(snapshot.Canonical)
	if err != nil || evidence.ActionID != actionID || evidence.ActorPrincipalID != principalID {
		return corrupt()
	}
	pub, _, err := publicKeyTx(ctx, tx, snapshot.SigningKeyID)
	if err != nil || identity.VerifyEvidence(pub, identity.SignedEvidence{
		Evidence: evidence, Canonical: snapshot.Canonical, Digest: snapshot.Digest,
		SigningKeyID: snapshot.SigningKeyID, Signature: snapshot.Signature,
	}) != nil {
		return corrupt()
	}
	return receiptIdentityTerms{
		present: true, status: "verified", requester: evidence.RequesterPrincipalID,
		actor: evidence.ActorPrincipalID, responsible: evidence.ResponsiblePrincipalID,
		evidenceHash: snapshot.Digest, snapshotHash: identity.SnapshotDigest(snapshot),
	}, nil
}

func authorityRefsValue(refs []string) any {
	if len(refs) == 0 {
		return nil
	}
	raw, _ := json.Marshal(refs)
	return string(raw)
}

func newIdentityID(prefix string) string {
	raw := make([]byte, 16)
	// See identity.newID: crypto/rand.Read does not return an error here.
	_, _ = rand.Read(raw)
	return prefix + hex.EncodeToString(raw)
}

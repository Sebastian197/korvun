// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// IntentHead is the checked lifecycle projection for one intent identity.
type IntentHead struct {
	IntentID                string
	ActiveVersion, Revision int
	LastEventDigest         string
	Status                  action.LifecycleStatus
}

// SetIntentV2Signer wires the active ledger key for terms and event acts.
func (s *Store) SetIntentV2Signer(contract func(action.IntentContractV2) action.SignedIntentContractV2, event func(action.IntentEventV1) action.SignedIntentEventV1) {
	s.intentContractSigner = contract
	s.intentEventSigner = event
}

// CreateIntentV2 persists validated DRAFT terms and their signed birth event.
func (s *Store) CreateIntentV2(ctx context.Context, c action.IntentContractV2, actor string, at time.Time) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if s.intentEventSigner == nil {
		return errors.New("action/sqlite: intent event signer unavailable")
	}
	e := action.IntentEventV1{EventID: action.NewIntentEventID(), IntentID: c.IntentID, Version: c.Version, Revision: 1, From: "", To: action.LifecycleDraft, ActorPrincipalID: actor, OccurredAt: at.UTC()}
	signed := s.intentEventSigner(e)
	if signed.Event != e {
		return action.ErrSignedObjectMutated
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.verifyNewEventTx(ctx, tx, signed); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO intent_versions(intent_id,version,schema_version,profile_id,owner_principal_id,canonical_terms,digest,created_at) VALUES(?,?,?,?,?,?,?,?)`, c.IntentID, c.Version, 2, c.ProfileID, c.OwnerPrincipalID, c.CanonicalBytes(), c.Digest(), at.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("action/sqlite: create intent v2: %w", err)
	}
	if err = s.insertIntentEventTx(ctx, tx, signed); err != nil {
		return err
	}
	if err = s.appendIntentMutationReceiptTx(ctx, tx, signed, c.Digest()); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO intent_heads(intent_id,active_version,revision,last_event_digest,status) VALUES(?,?,?,?,?)`, c.IntentID, c.Version, 1, signed.Digest, string(action.LifecycleDraft)); err != nil {
		return err
	}
	return tx.Commit()
}

// ActivateIntentV2 validates and signs exact stored terms before activation.
func (s *Store) ActivateIntentV2(ctx context.Context, id string, version int, actor string, at time.Time) error {
	if s.intentContractSigner == nil || s.intentEventSigner == nil {
		return errors.New("action/sqlite: intent signer unavailable")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	c, err := readIntentTermsTx(ctx, tx, id, version)
	if err != nil {
		return err
	}
	if err = c.Validate(); err != nil {
		return err
	}
	head, err := intentHeadTx(ctx, tx, id)
	if err != nil {
		return err
	}
	if err = action.IntentV2LifecycleTransition(head.Status, action.LifecycleActive); err != nil {
		return err
	}
	// The canonical bytes are taken BEFORE the signer sees the terms. Comparing
	// the signed object against `c` alone was blind to a signer that mutates a
	// slice-backed field IN PLACE: both sides then read the same mutated array
	// and the comparison passed, rows committed and the intent was left
	// unusable (the twenty-second pass, P2-6). Two comparisons now: what came
	// back, and what the caller still holds.
	before := c.CanonicalBytes()
	signedC := s.intentContractSigner(c)
	if !bytes.Equal(signedC.Contract.CanonicalBytes(), before) || !bytes.Equal(c.CanonicalBytes(), before) {
		return action.ErrSignedObjectMutated
	}
	if err = s.verifyNewContractTx(ctx, tx, signedC); err != nil {
		return err
	}
	e := action.IntentEventV1{EventID: action.NewIntentEventID(), IntentID: id, Version: version, Revision: head.Revision + 1, From: head.Status, To: action.LifecycleActive, ActorPrincipalID: actor, OccurredAt: at.UTC(), PreviousEventDigest: head.LastEventDigest}
	signedE := s.intentEventSigner(e)
	if signedE.Event != e {
		return action.ErrSignedObjectMutated
	}
	if err = s.verifyNewEventTx(ctx, tx, signedE); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE intent_versions SET signing_key_id=?,signature=? WHERE intent_id=? AND version=?`, signedC.SigningKeyID, signedC.Signature, id, version); err != nil {
		return err
	}
	if err = s.insertIntentEventTx(ctx, tx, signedE); err != nil {
		return err
	}
	if err = s.appendIntentMutationReceiptTx(ctx, tx, signedE, c.Digest()); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE intent_heads SET active_version=?,revision=?,last_event_digest=?,status=? WHERE intent_id=?`, version, e.Revision, signedE.Digest, string(action.LifecycleActive), id); err != nil {
		return err
	}
	return tx.Commit()
}

// RevokeIntentV2 appends a signed revocation and advances the checked head.
func (s *Store) RevokeIntentV2(ctx context.Context, id, actor string, at time.Time) error {
	return s.transitionIntentV2(ctx, id, actor, action.LifecycleRevoked, at)
}

// ExpireIntentV2 appends a signed expiry and advances the checked head.
func (s *Store) ExpireIntentV2(ctx context.Context, id, actor string, at time.Time) error {
	return s.transitionIntentV2(ctx, id, actor, action.LifecycleExpired, at)
}
func (s *Store) transitionIntentV2(ctx context.Context, id, actor string, to action.LifecycleStatus, at time.Time) error {
	if s.intentEventSigner == nil {
		return errors.New("action/sqlite: intent event signer unavailable")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	head, err := intentHeadTx(ctx, tx, id)
	if err != nil {
		return err
	}
	if err = action.IntentV2LifecycleTransition(head.Status, to); err != nil {
		return err
	}
	e := action.IntentEventV1{EventID: action.NewIntentEventID(), IntentID: id, Version: head.ActiveVersion, Revision: head.Revision + 1, From: head.Status, To: to, ActorPrincipalID: actor, OccurredAt: at.UTC(), PreviousEventDigest: head.LastEventDigest}
	signed := s.intentEventSigner(e)
	if signed.Event != e {
		return action.ErrSignedObjectMutated
	}
	if err = s.verifyNewEventTx(ctx, tx, signed); err != nil {
		return err
	}
	if err = s.insertIntentEventTx(ctx, tx, signed); err != nil {
		return err
	}
	var intentDigest string
	if err = tx.QueryRowContext(ctx, `SELECT digest FROM intent_versions WHERE intent_id=? AND version=?`, id, head.ActiveVersion).Scan(&intentDigest); err != nil {
		return err
	}
	if err = s.appendIntentMutationReceiptTx(ctx, tx, signed, intentDigest); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE intent_heads SET revision=?,last_event_digest=?,status=? WHERE intent_id=?`, e.Revision, signed.Digest, string(to), id); err != nil {
		return err
	}
	return tx.Commit()
}

// ResolveIntentV2 verifies exact terms and current lifecycle at use time.
func (s *Store) ResolveIntentV2(ctx context.Context, id string, version int, digest string, at time.Time) (action.SignedIntentContractV2, error) {
	var raw []byte
	var storedDigest, keyID, signature string
	err := s.db.QueryRowContext(ctx, `SELECT canonical_terms,digest,coalesce(signing_key_id,''),coalesce(signature,'') FROM intent_versions WHERE intent_id=? AND version=?`, id, version).Scan(&raw, &storedDigest, &keyID, &signature)
	if errors.Is(err, sql.ErrNoRows) {
		return action.SignedIntentContractV2{}, ErrNotFound
	}
	if err != nil {
		return action.SignedIntentContractV2{}, err
	}
	c, err := action.ParseIntentContractV2(raw)
	if err != nil {
		return action.SignedIntentContractV2{}, fmt.Errorf("%w: %v", action.ErrIntentEvidenceCorrupt, err)
	}
	signed := action.SignedIntentContractV2{Contract: c, Digest: storedDigest, SigningKeyID: keyID, Signature: signature}
	if digest != c.Digest() || storedDigest != c.Digest() {
		return action.SignedIntentContractV2{}, action.ErrIntentEvidenceCorrupt
	}
	// The LIFECYCLE is judged before the key is looked up. A DRAFT carries no
	// signing key, so the old order asked for the empty key first and the
	// operator was told `action not found` about an intent that exists and was
	// simply never activated — and ErrIntentInactive, which names exactly that,
	// was unreachable (the twenty-second pass, P2-8).
	// ONE snapshot for the head and the chain that must agree with it. Two
	// statements on the pool let a legitimate revocation committed in between
	// make the second disagree with the first, and the door then shouted
	// CORRUPT about a benign race — the alarm that triggers manual repair (the
	// twenty-second pass, P2-9). A read transaction gives both reads one view.
	head, err := s.headAndHistory(ctx, id)
	if err != nil {
		return action.SignedIntentContractV2{}, err
	}
	switch head.Status {
	case action.LifecycleDraft:
		return action.SignedIntentContractV2{}, action.ErrIntentInactive
	case action.LifecycleRevoked:
		return action.SignedIntentContractV2{}, action.ErrIntentRevoked
	case action.LifecycleExpired:
		return action.SignedIntentContractV2{}, action.ErrIntentExpired
	}
	pub, _, err := s.publicKey(ctx, keyID)
	if err != nil {
		return action.SignedIntentContractV2{}, err
	}
	if err = action.VerifyIntentContractV2(pub, signed); err != nil {
		return action.SignedIntentContractV2{}, err
	}
	if at.Before(c.ValidFrom) || !at.Before(c.ExpiresAt) {
		return action.SignedIntentContractV2{}, action.ErrIntentExpired
	}
	return signed, nil
}

// IntentHead reads the lifecycle projection for an intent.
func (s *Store) IntentHead(ctx context.Context, id string) (IntentHead, error) {
	return scanIntentHead(s.db.QueryRowContext(ctx, `SELECT intent_id,active_version,revision,last_event_digest,status FROM intent_heads WHERE intent_id=?`, id))
}
func intentHeadTx(ctx context.Context, tx *sql.Tx, id string) (IntentHead, error) {
	return scanIntentHead(tx.QueryRowContext(ctx, `SELECT intent_id,active_version,revision,last_event_digest,status FROM intent_heads WHERE intent_id=?`, id))
}
func scanIntentHead(row *sql.Row) (IntentHead, error) {
	var h IntentHead
	var status string
	if err := row.Scan(&h.IntentID, &h.ActiveVersion, &h.Revision, &h.LastEventDigest, &status); errors.Is(err, sql.ErrNoRows) {
		return h, ErrNotFound
	} else if err != nil {
		return h, err
	}
	h.Status = action.LifecycleStatus(status)
	return h, nil
}

// PutExecutionBinding stores an operator-selected exact intent binding.
func (s *Store) PutExecutionBinding(ctx context.Context, b action.ExecutionBinding) error {
	if b.Status != action.BindingActive && b.Status != action.BindingRevoked {
		return errors.New("action/sqlite: invalid binding status")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `INSERT INTO execution_bindings(binding_id,actor_principal_id,channel,conversation_id,intent_id,intent_version,intent_digest,revision,status) VALUES(?,?,?,?,?,?,?,?,?)`, b.BindingID, b.ActorPrincipalID, b.Channel, nullString(b.ConversationID), b.IntentID, b.IntentVersion, b.IntentDigest, b.Revision, string(b.Status)); err != nil {
		return err
	}
	at := time.Now().UTC()
	if err = s.appendReceiptTx(ctx, tx, action.Receipt{SchemaVersion: 2, ReceiptID: action.NewReceiptID(), ActionID: b.BindingID, IntentDigest: b.IntentDigest, PrincipalID: b.ActorPrincipalID, DecisionDigest: action.HashCanonical("execution_binding"), ActionDigest: action.HashCanonical(fmt.Sprintf("%s\x00%s\x00%s\x00%d", b.ActorPrincipalID, b.Channel, b.ConversationID, b.Revision)), EffectClass: action.EffectPure, Attempt: 1, Outcome: string(action.StateSucceeded), StartedAt: at, FinishedAt: at}); err != nil {
		return err
	}
	return tx.Commit()
}

// ResolveExecutionBinding selects the most specific binding and verifies it.
func (s *Store) ResolveExecutionBinding(ctx context.Context, actor, channel, conversation string, at time.Time) (action.SignedIntentContractV2, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT intent_id,intent_version,intent_digest,conversation_id FROM execution_bindings WHERE actor_principal_id=? AND channel=? AND status='ACTIVE' AND (conversation_id=? OR conversation_id IS NULL) ORDER BY conversation_id IS NOT NULL DESC`, actor, channel, conversation)
	if err != nil {
		return action.SignedIntentContractV2{}, err
	}
	defer func() { _ = rows.Close() }()
	type hit struct {
		id, digest string
		v          int
		conv       sql.NullString
	}
	var hits []hit
	for rows.Next() {
		var h hit
		if err = rows.Scan(&h.id, &h.v, &h.digest, &h.conv); err != nil {
			return action.SignedIntentContractV2{}, err
		}
		hits = append(hits, h)
	}
	if len(hits) == 0 {
		return action.SignedIntentContractV2{}, ErrNotFound
	}
	if len(hits) > 1 && hits[0].conv.Valid == hits[1].conv.Valid {
		return action.SignedIntentContractV2{}, action.ErrAmbiguousIntentBinding
	}
	return s.ResolveIntentV2(ctx, hits[0].id, hits[0].v, hits[0].digest, at)
}

// GetIntentAnyVersion reads signed version 2 first, then legacy unsigned v1.
func (s *Store) GetIntentAnyVersion(ctx context.Context, id string) (action.IntentAnyVersion, error) {
	var raw []byte
	var digest, key, sig string
	err := s.db.QueryRowContext(ctx, `SELECT canonical_terms,digest,coalesce(signing_key_id,''),coalesce(signature,'') FROM intent_versions WHERE intent_id=? ORDER BY version DESC LIMIT 1`, id).Scan(&raw, &digest, &key, &sig)
	if err == nil {
		c, e := action.ParseIntentContractV2(raw)
		if e != nil {
			return action.IntentAnyVersion{}, e
		}
		signed := action.SignedIntentContractV2{Contract: c, Digest: digest, SigningKeyID: key, Signature: sig}
		if verr := s.verifyStoredTerms(ctx, signed); verr != nil {
			return action.IntentAnyVersion{}, verr
		}
		// A v2 row with no signature is a DRAFT that was never activated. It
		// used to be reported as `signed_v2`, which named evidence that does
		// not exist (the twenty-second pass, P2-7): the taxonomy now has a term
		// for it.
		provenance := action.IntentProvenanceSignedV2
		if key == "" && sig == "" {
			provenance = action.IntentProvenanceUnsignedV2
		}
		return action.IntentAnyVersion{Provenance: provenance, SignedV2: &signed}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return action.IntentAnyVersion{}, err
	}
	legacy, e := s.GetIntent(ctx, id)
	if e != nil {
		return action.IntentAnyVersion{}, e
	}
	return action.IntentAnyVersion{Provenance: action.IntentProvenanceLegacyUnsigned, Legacy: &legacy}, nil
}

// GetIntentV2 reads one exact version without applying lifecycle or time rules.
func (s *Store) GetIntentV2(ctx context.Context, id string, version int) (action.SignedIntentContractV2, error) {
	var raw []byte
	var signed action.SignedIntentContractV2
	err := s.db.QueryRowContext(ctx, `SELECT canonical_terms,digest,coalesce(signing_key_id,''),coalesce(signature,'') FROM intent_versions WHERE intent_id=? AND version=?`, id, version).Scan(&raw, &signed.Digest, &signed.SigningKeyID, &signed.Signature)
	if errors.Is(err, sql.ErrNoRows) {
		return signed, ErrNotFound
	}
	if err != nil {
		return signed, err
	}
	if signed.Contract, err = action.ParseIntentContractV2(raw); err != nil {
		return action.SignedIntentContractV2{}, fmt.Errorf("%w: %v", action.ErrIntentEvidenceCorrupt, err)
	}
	// INTEGRITY is not lifecycle and is not time: this door skips those two and
	// still refuses bytes that do not re-derive their own stored digest, and a
	// signature that does not verify over them. It used to return TAMPERED
	// terms with err=nil, next to the digest and signature of the ORIGINAL text
	// — a valid signature over invalid surrounding data (the twenty-second
	// pass, P2-7).
	if err := s.verifyStoredTerms(ctx, signed); err != nil {
		return action.SignedIntentContractV2{}, err
	}
	return signed, nil
}

// verifyStoredTerms judges one row read from intent_versions: the bytes must
// re-derive the stored digest, and a row that carries a signature must verify
// under its own key. A row with neither key nor signature is an unsigned DRAFT
// and is NOT called corrupt — it is reported for what it is by its caller.
func (s *Store) verifyStoredTerms(ctx context.Context, signed action.SignedIntentContractV2) error {
	if signed.Digest != signed.Contract.Digest() {
		return action.ErrIntentEvidenceCorrupt
	}
	if signed.SigningKeyID == "" && signed.Signature == "" {
		return nil
	}
	pub, _, err := s.publicKey(ctx, signed.SigningKeyID)
	if err != nil {
		return err
	}
	return action.VerifyIntentContractV2(pub, signed)
}

func readIntentTermsTx(ctx context.Context, tx *sql.Tx, id string, version int) (action.IntentContractV2, error) {
	var raw []byte
	if err := tx.QueryRowContext(ctx, `SELECT canonical_terms FROM intent_versions WHERE intent_id=? AND version=?`, id, version).Scan(&raw); errors.Is(err, sql.ErrNoRows) {
		return action.IntentContractV2{}, ErrNotFound
	} else if err != nil {
		return action.IntentContractV2{}, err
	}
	return action.ParseIntentContractV2(raw)
}
func (s *Store) verifyNewContractTx(ctx context.Context, tx *sql.Tx, signed action.SignedIntentContractV2) error {
	pub, retired, err := publicKeyTx(ctx, tx, signed.SigningKeyID)
	if err != nil {
		return err
	}
	if retired {
		return action.ErrSigningKeyRetired
	}
	return action.VerifyIntentContractV2(pub, signed)
}
func (s *Store) verifyNewEventTx(ctx context.Context, tx *sql.Tx, signed action.SignedIntentEventV1) error {
	pub, retired, err := publicKeyTx(ctx, tx, signed.SigningKeyID)
	if err != nil {
		return err
	}
	if retired {
		return action.ErrSigningKeyRetired
	}
	return action.VerifyIntentEventV1(pub, signed)
}
func publicKeyTx(ctx context.Context, tx *sql.Tx, id string) (ed25519.PublicKey, bool, error) {
	var raw string
	var retired sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT public_key,retired_at FROM signing_keys WHERE key_id=?`, id).Scan(&raw, &retired); errors.Is(err, sql.ErrNoRows) {
		return nil, false, ErrNotFound
	} else if err != nil {
		return nil, false, err
	}
	b, err := hex.DecodeString(raw)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, false, action.ErrIntentEvidenceCorrupt
	}
	return ed25519.PublicKey(b), retired.Valid && retired.String != "", nil
}
func (s *Store) publicKey(ctx context.Context, id string) (ed25519.PublicKey, bool, error) {
	var raw string
	var retired sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT public_key,retired_at FROM signing_keys WHERE key_id=?`, id).Scan(&raw, &retired); errors.Is(err, sql.ErrNoRows) {
		return nil, false, ErrNotFound
	} else if err != nil {
		return nil, false, err
	}
	b, err := hex.DecodeString(raw)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, false, action.ErrIntentEvidenceCorrupt
	}
	return ed25519.PublicKey(b), retired.Valid && retired.String != "", nil
}
func (s *Store) insertIntentEventTx(ctx context.Context, tx *sql.Tx, signed action.SignedIntentEventV1) error {
	e := signed.Event
	_, err := tx.ExecContext(ctx, `INSERT INTO intent_events(event_id,intent_id,intent_version,revision,from_status,to_status,actor_principal_id,evidence_digest,occurred_at,previous_event_digest,canonical_event,digest,signing_key_id,signature) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, e.EventID, e.IntentID, e.Version, e.Revision, string(e.From), string(e.To), e.ActorPrincipalID, e.EvidenceDigest, e.OccurredAt.UTC().Format(time.RFC3339Nano), e.PreviousEventDigest, e.CanonicalBytes(), signed.Digest, signed.SigningKeyID, signed.Signature)
	return err
}

func (s *Store) appendIntentMutationReceiptTx(ctx context.Context, tx *sql.Tx, signed action.SignedIntentEventV1, intentDigest string) error {
	e := signed.Event
	return s.appendReceiptTx(ctx, tx, action.Receipt{
		SchemaVersion: 2, ReceiptID: action.NewReceiptID(), ActionID: e.EventID,
		IntentDigest: intentDigest, PrincipalID: e.ActorPrincipalID,
		DecisionDigest: action.HashCanonical(signed.Digest), ActionDigest: signed.Digest,
		EffectClass: action.EffectPure, Attempt: 1, Outcome: string(action.StateSucceeded),
		StartedAt: e.OccurredAt, FinishedAt: e.OccurredAt,
	})
}

// rowsQuerier is the multi-row sibling of the package's rowQuerier: *sql.DB and
// *sql.Tx both satisfy it, so the history walk can run inside the SAME snapshot
// as the head read it judges.
type rowsQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func verifyIntentHistoryOn(ctx context.Context, q rowsQuerier, head IntentHead) error {
	rows, err := q.QueryContext(ctx, `SELECT e.event_id,e.intent_version,e.revision,e.from_status,e.to_status,e.actor_principal_id,e.evidence_digest,e.occurred_at,e.previous_event_digest,e.digest,e.signing_key_id,e.signature,k.public_key FROM intent_events e JOIN signing_keys k ON k.key_id=e.signing_key_id WHERE e.intent_id=? ORDER BY e.revision`, head.IntentID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	previous := ""
	revision := 0
	status := action.LifecycleStatus("")
	for rows.Next() {
		var event action.IntentEventV1
		var from, to, occurred string
		var publicKeyHex string
		var signed action.SignedIntentEventV1
		if err := rows.Scan(&event.EventID, &event.Version, &event.Revision, &from, &to, &event.ActorPrincipalID, &event.EvidenceDigest, &occurred, &event.PreviousEventDigest, &signed.Digest, &signed.SigningKeyID, &signed.Signature, &publicKeyHex); err != nil {
			return err
		}
		event.IntentID = head.IntentID
		event.From = action.LifecycleStatus(from)
		event.To = action.LifecycleStatus(to)
		event.OccurredAt, err = time.Parse(time.RFC3339Nano, occurred)
		if err != nil {
			return action.ErrIntentEvidenceCorrupt
		}
		signed.Event = event
		if event.Revision != revision+1 || event.PreviousEventDigest != previous {
			return action.ErrIntentEvidenceCorrupt
		}
		if event.Revision == 1 {
			if event.From != "" || event.To != action.LifecycleDraft {
				return action.ErrIntentEvidenceCorrupt
			}
		} else if event.From != status || action.IntentV2LifecycleTransition(event.From, event.To) != nil {
			return action.ErrIntentEvidenceCorrupt
		}
		publicKeyBytes, decodeErr := hex.DecodeString(publicKeyHex)
		if decodeErr != nil || len(publicKeyBytes) != ed25519.PublicKeySize {
			return action.ErrIntentEvidenceCorrupt
		}
		pub := ed25519.PublicKey(publicKeyBytes)
		if action.VerifyIntentEventV1(pub, signed) != nil {
			return action.ErrIntentEvidenceCorrupt
		}
		previous, revision, status = signed.Digest, event.Revision, event.To
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if revision != head.Revision || previous != head.LastEventDigest || status != head.Status {
		return action.ErrIntentEvidenceCorrupt
	}
	return nil
}
func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// headAndHistory reads the lifecycle projection and verifies the signed event
// chain against it inside ONE read transaction, so a concurrent, legitimate
// transition cannot make the two disagree.
func (s *Store) headAndHistory(ctx context.Context, id string) (IntentHead, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return IntentHead{}, err
	}
	defer func() { _ = tx.Rollback() }()
	head, err := intentHeadTx(ctx, tx, id)
	if err != nil {
		return IntentHead{}, err
	}
	if err := verifyIntentHistoryOn(ctx, tx, head); err != nil {
		return IntentHead{}, err
	}
	return head, nil
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

const (
	evidenceDomain = "korvun.identity-evidence.v2"
	eventDomain    = "korvun.principal-event.v1"
	snapshotDomain = "korvun.identity-birth-snapshot.v1"
)

// SignedEvidence is a canonical identity statement with its digest and seal.
type SignedEvidence struct {
	Evidence     Evidence
	Canonical    []byte
	Digest       string
	SigningKeyID string
	Signature    string
}

// PrincipalEvent is the signed lifecycle projection of one principal.
type PrincipalEvent struct {
	EventID    string    `json:"event_id"`
	Principal  Principal `json:"principal"`
	Revision   int64     `json:"revision"`
	Kind       string    `json:"kind"`
	OccurredAt time.Time `json:"occurred_at"`
}

// SignedPrincipalEvent is one canonical principal event and its seal.
type SignedPrincipalEvent struct {
	Event        PrincipalEvent
	Canonical    []byte
	Digest       string
	SigningKeyID string
	Signature    string
}

// BirthSnapshot is the durable signed identity tuple copied onto an action.
type BirthSnapshot struct {
	Version      int
	Digest       string
	Canonical    []byte
	SigningKeyID string
	Signature    string
}

// CanonicalEvidence returns the fixed JSON wire signed for one action.
func CanonicalEvidence(e Evidence) []byte {
	raw, err := json.Marshal(e)
	if err != nil {
		panic("identity: canonical evidence encoding: " + err.Error())
	}
	return raw
}

// ParseCanonicalEvidence parses exactly one fixed evidence object.
func ParseCanonicalEvidence(raw []byte) (Evidence, error) {
	return parseEvidence(raw)
}

// SignEvidence seals evidence in the identity-evidence domain.
func SignEvidence(priv ed25519.PrivateKey, e Evidence) SignedEvidence {
	canonical := CanonicalEvidence(e)
	return SignedEvidence{
		Evidence: e, Canonical: canonical, Digest: hash(canonical),
		SigningKeyID: signingKeyID(priv.Public().(ed25519.PublicKey)),
		Signature:    signDomain(priv, evidenceDomain, canonical),
	}
}

// VerifyEvidence verifies canonical bytes, digest, key id, and signature.
func VerifyEvidence(pub ed25519.PublicKey, signed SignedEvidence) error {
	parsed, err := parseEvidence(signed.Canonical)
	if err != nil || parsed != signed.Evidence || signed.Digest != hash(signed.Canonical) ||
		signed.SigningKeyID != signingKeyID(pub) ||
		!verifyDomain(pub, evidenceDomain, signed.Canonical, signed.Signature) {
		return ErrIdentityEvidenceCorrupt
	}
	return nil
}

// CanonicalPrincipalEvent returns the fixed event wire.
func CanonicalPrincipalEvent(event PrincipalEvent) []byte {
	raw, err := json.Marshal(event)
	if err != nil {
		panic("identity: canonical principal event encoding: " + err.Error())
	}
	return raw
}

// ParseCanonicalPrincipalEvent parses exactly one fixed principal event.
func ParseCanonicalPrincipalEvent(raw []byte) (PrincipalEvent, error) {
	var event PrincipalEvent
	if err := strictJSON(raw, &event); err != nil {
		return PrincipalEvent{}, err
	}
	return event, nil
}

// SignPrincipalEvent seals one lifecycle event.
func SignPrincipalEvent(priv ed25519.PrivateKey, event PrincipalEvent) SignedPrincipalEvent {
	canonical := CanonicalPrincipalEvent(event)
	return SignedPrincipalEvent{
		Event: event, Canonical: canonical, Digest: hash(canonical),
		SigningKeyID: signingKeyID(priv.Public().(ed25519.PublicKey)),
		Signature:    signDomain(priv, eventDomain, canonical),
	}
}

// VerifyPrincipalEvent verifies an event against registered historical ink.
func VerifyPrincipalEvent(pub ed25519.PublicKey, signed SignedPrincipalEvent) error {
	var parsed PrincipalEvent
	if err := strictJSON(signed.Canonical, &parsed); err != nil || parsed != signed.Event ||
		signed.Digest != hash(signed.Canonical) || signed.SigningKeyID != signingKeyID(pub) ||
		!verifyDomain(pub, eventDomain, signed.Canonical, signed.Signature) {
		return ErrIdentityEvidenceCorrupt
	}
	return nil
}

// SnapshotDigest hashes every stored birth-snapshot term with unambiguous
// length prefixes. Changing the era, key id, or signature changes the digest.
func SnapshotDigest(snapshot BirthSnapshot) string {
	var buf bytes.Buffer
	writeTerm(&buf, []byte(snapshotDomain))
	writeTerm(&buf, []byte(strconv.Itoa(snapshot.Version)))
	writeTerm(&buf, []byte(snapshot.Digest))
	writeTerm(&buf, snapshot.Canonical)
	writeTerm(&buf, []byte(snapshot.SigningKeyID))
	writeTerm(&buf, []byte(snapshot.Signature))
	return hash(buf.Bytes())
}

// SnapshotIsLegacy reports the only legacy tuple: zero era and every signed
// snapshot field empty.
func SnapshotIsLegacy(snapshot BirthSnapshot) bool {
	return snapshot.Version == 0 && snapshot.Digest == "" && len(snapshot.Canonical) == 0 &&
		snapshot.SigningKeyID == "" && snapshot.Signature == ""
}

func parseEvidence(raw []byte) (Evidence, error) {
	var evidence Evidence
	if err := strictJSON(raw, &evidence); err != nil {
		return Evidence{}, err
	}
	return evidence, nil
}

func strictJSON(raw []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("identity: trailing canonical content")
		}
		return err
	}
	return nil
}

func signingKeyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return "ed25519:" + hex.EncodeToString(sum[:])[:16]
}

func signDomain(priv ed25519.PrivateKey, domain string, canonical []byte) string {
	digest := sha256.Sum256(append(append([]byte(domain), 0), canonical...))
	return hex.EncodeToString(ed25519.Sign(priv, digest[:]))
}

func verifyDomain(pub ed25519.PublicKey, domain string, canonical []byte, signatureHex string) bool {
	signature, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false
	}
	digest := sha256.Sum256(append(append([]byte(domain), 0), canonical...))
	return ed25519.Verify(pub, digest[:], signature)
}

func writeTerm(buf *bytes.Buffer, term []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(term)))
	buf.Write(size[:])
	buf.Write(term)
}

// RegisteredKeyID validates the algorithm prefix and returns the key id
// derived from pub. It is used by durable verification without importing the
// action package.
func RegisteredKeyID(pub ed25519.PublicKey) (string, error) {
	id := signingKeyID(pub)
	if !strings.HasPrefix(id, "ed25519:") {
		return "", ErrIdentityEvidenceCorrupt
	}
	return id, nil
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Signing primitives (Trust Layer Etapa 4, lote 2, spec FR-KEY, sealed
// §30-1): Ed25519 from the standard library — deterministic signatures,
// 32-byte keys, no nonce failure mode — over the sha256 of the
// receipt's canonical signable form. The algorithm name lives INSIDE
// the signing key id ("ed25519:" + hex(sha256(pub))[:16], the E1
// digest-prefix precedent), so a future algorithm coexists without
// ambiguity and an UNKNOWN prefix fails CLOSED at verification. The
// chain this ink seals provides TAMPER EVIDENCE — never describe it as
// immutability: the operator controls storage and keys (§19.3).
package action

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// ErrUnknownSignatureAlgorithm reports a signing_key_id whose algorithm
// prefix this binary does not know: verification fails CLOSED — an
// unknown algorithm is never assumed valid.
var ErrUnknownSignatureAlgorithm = errors.New("action: unknown signature algorithm")

// signingAlgorithm is the sealed v1 algorithm prefix.
const signingAlgorithm = "ed25519"

// SigningKeyID derives the sealed key identity: the algorithm INSIDE
// the id, then the first 16 hex chars of the public key's sha256.
func SigningKeyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return signingAlgorithm + ":" + hex.EncodeToString(sum[:])[:16]
}

// SignReceipt seals a receipt: signature = ed25519 over
// sha256(canonical signable form), hex-encoded, with the signer's key
// id stamped. The signable form excludes hash/key/signature by design.
func SignReceipt(priv ed25519.PrivateKey, r Receipt) Receipt {
	digest := sha256.Sum256(CanonicalReceipt(r))
	r.SigningKeyID = SigningKeyID(priv.Public().(ed25519.PublicKey))
	r.Signature = hex.EncodeToString(ed25519.Sign(priv, digest[:]))
	return r
}

// VerifyReceiptSignature verifies a sealed receipt against a public
// key: the algorithm prefix must be known, the key id must match the
// key, and the signature must verify over the RECOMPUTED canonical
// digest — stored bytes are never trusted.
func VerifyReceiptSignature(pub ed25519.PublicKey, r Receipt) error {
	algorithm, _, found := strings.Cut(r.SigningKeyID, ":")
	if !found || algorithm != signingAlgorithm {
		return fmt.Errorf("%w: %q", ErrUnknownSignatureAlgorithm, r.SigningKeyID)
	}
	if r.SigningKeyID != SigningKeyID(pub) {
		return fmt.Errorf("action: signing key id %q does not match the verification key", r.SigningKeyID)
	}
	signature, err := hex.DecodeString(r.Signature)
	if err != nil {
		return fmt.Errorf("action: decode signature: %w", err)
	}
	digest := sha256.Sum256(CanonicalReceipt(r))
	if !ed25519.Verify(pub, digest[:], signature) {
		return errors.New("action: receipt signature does not verify")
	}
	return nil
}

// EncodeSigningKeySeed renders a private key's 32-byte seed as lowercase
// hex — the on-disk form of the profile key file (0600).
func EncodeSigningKeySeed(priv ed25519.PrivateKey) []byte {
	return []byte(hex.EncodeToString(priv.Seed()))
}

// ParseSigningKeySeed parses the key file's UNTRUSTED bytes strictly:
// exactly 64 lowercase-or-uppercase hex chars (one trailing newline
// tolerated — the file-editor reality), nothing else. Anything short,
// long, non-hex or trailing fails closed.
func ParseSigningKeySeed(raw []byte) (ed25519.PrivateKey, error) {
	trimmed := raw
	if n := len(trimmed); n > 0 && trimmed[n-1] == '\n' {
		trimmed = trimmed[:n-1]
	}
	if bytes.ContainsAny(trimmed, "\n\r \t") {
		return nil, errors.New("action: signing key seed carries stray content")
	}
	if len(trimmed) != ed25519.SeedSize*2 {
		return nil, fmt.Errorf("action: signing key seed must be %d hex chars, got %d", ed25519.SeedSize*2, len(trimmed))
	}
	seed, err := hex.DecodeString(string(trimmed))
	if err != nil {
		return nil, fmt.Errorf("action: decode signing key seed: %w", err)
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

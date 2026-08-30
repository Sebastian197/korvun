// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Signing primitives contract — Etapa 4, lote 2, pieza 1 (spec FR-KEY,
// sealed §30-1): Ed25519 over sha256 of the canonical receipt, hex; the
// algorithm INSIDE the key id (the digest-prefix precedent); pure
// sign/verify primitives with the SABOTAGE TABLE extended — one flipped
// byte in ANY sealed field breaks verification, a wrong key fails, an
// unknown algorithm prefix fails CLOSED; and the key-seed file parser is
// strict and birth-fuzzed. Tamper-evident, never immutable.
// Approved-red contract.

package action

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
)

func testKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return pub, priv
}

func TestSigningKeyID_sealedFormat(t *testing.T) {
	t.Parallel()
	pub, _ := testKeyPair(t)
	id := SigningKeyID(pub)
	if !strings.HasPrefix(id, "ed25519:") {
		t.Fatalf("the algorithm lives INSIDE the id, got %q", id)
	}
	sum := sha256.Sum256(pub)
	want := "ed25519:" + hex.EncodeToString(sum[:])[:16]
	if id != want {
		t.Fatalf("sealed format: got %q want %q", id, want)
	}
	if SigningKeyID(pub) != id {
		t.Fatal("the id must be deterministic")
	}
}

func TestSignReceipt_sealsAndVerifies(t *testing.T) {
	t.Parallel()
	pub, priv := testKeyPair(t)
	r := testReceipt()
	r.ReceiptHash = ComputeReceiptHash(r)
	signed := SignReceipt(priv, r)
	if signed.SigningKeyID != SigningKeyID(pub) {
		t.Fatalf("the receipt carries its key id, got %q", signed.SigningKeyID)
	}
	if signed.Signature == "" {
		t.Fatal("the signature must be present")
	}
	if _, err := hex.DecodeString(signed.Signature); err != nil {
		t.Fatalf("the signature travels hex-encoded: %v", err)
	}
	if err := VerifyReceiptSignature(pub, signed); err != nil {
		t.Fatalf("a sealed receipt must verify: %v", err)
	}
}

// TestVerify_sabotageTable is the lote-1 mold extended to signatures:
// one flipped byte in ANY sealed field breaks verification.
func TestVerify_sabotageTable(t *testing.T) {
	t.Parallel()
	pub, priv := testKeyPair(t)
	base := testReceipt()
	base.ReceiptHash = ComputeReceiptHash(base)
	sealed := SignReceipt(priv, base)
	sabotage := map[string]func(*Receipt){
		"receipt_id":       func(r *Receipt) { r.ReceiptID = "rcpt_forged" },
		"action_id":        func(r *Receipt) { r.ActionID = "act_forged" },
		"intent_digest":    func(r *Receipt) { r.IntentDigest = "sha256:forged" },
		"principal_id":     func(r *Receipt) { r.PrincipalID = "principal_forged" },
		"authority_digest": func(r *Receipt) { r.AuthorityDigest = "sha256:forged" },
		"decision_digest":  func(r *Receipt) { r.DecisionDigest = "sha256:forged" },
		"action_digest":    func(r *Receipt) { r.ActionDigest = "sha256:forged" },
		"effect_class":     func(r *Receipt) { r.EffectClass = EffectCritical },
		"attempt":          func(r *Receipt) { r.Attempt = 99 },
		"outcome":          func(r *Receipt) { r.Outcome = string(StateFailed) },
		"result_digest":    func(r *Receipt) { r.ResultDigest = "sha256:forged" },
		"started_at":       func(r *Receipt) { r.StartedAt = r.StartedAt.Add(time.Second) },
		"finished_at":      func(r *Receipt) { r.FinishedAt = r.FinishedAt.Add(time.Second) },
		"partition":        func(r *Receipt) { r.Partition = "forged" },
		"chain_seq":        func(r *Receipt) { r.ChainSeq = 999 },
		"previous_hash":    func(r *Receipt) { r.PreviousReceiptHash = "sha256:forged" },
		"signature_byte": func(r *Receipt) {
			raw := []byte(r.Signature)
			if raw[0] == 'a' {
				raw[0] = 'b'
			} else {
				raw[0] = 'a'
			}
			r.Signature = string(raw)
		},
	}
	for name, tamper := range sabotage {
		forged := sealed
		tamper(&forged)
		if err := VerifyReceiptSignature(pub, forged); err == nil {
			t.Fatalf("sabotaging %s must break verification", name)
		}
	}
}

func TestVerify_wrongKeyFails(t *testing.T) {
	t.Parallel()
	_, priv := testKeyPair(t)
	otherPub, _ := testKeyPair(t)
	r := testReceipt()
	sealed := SignReceipt(priv, r)
	if err := VerifyReceiptSignature(otherPub, sealed); err == nil {
		t.Fatal("a wrong key must fail verification")
	}
}

func TestVerify_unknownAlgorithmPrefixFailsClosed(t *testing.T) {
	t.Parallel()
	pub, priv := testKeyPair(t)
	sealed := SignReceipt(priv, testReceipt())
	sealed.SigningKeyID = "quantum9000:" + strings.TrimPrefix(sealed.SigningKeyID, "ed25519:")
	err := VerifyReceiptSignature(pub, sealed)
	if !errors.Is(err, ErrUnknownSignatureAlgorithm) {
		t.Fatalf("an unknown algorithm prefix must fail CLOSED with the sentinel, got %v", err)
	}
	sealed.SigningKeyID = "no-colon-at-all"
	if err := VerifyReceiptSignature(pub, sealed); !errors.Is(err, ErrUnknownSignatureAlgorithm) {
		t.Fatalf("a malformed key id must fail closed, got %v", err)
	}
}

func TestParseSigningKeySeed_strictAndRoundTrips(t *testing.T) {
	t.Parallel()
	pub, priv := testKeyPair(t)
	encoded := EncodeSigningKeySeed(priv)
	parsed, err := ParseSigningKeySeed(encoded)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !parsed.Public().(ed25519.PublicKey).Equal(pub) {
		t.Fatal("the seed must round-trip to the same key pair")
	}
	// Strict: garbage, short, long, non-hex, trailing content fail closed.
	for _, garbage := range [][]byte{
		nil, []byte(""), []byte("zz"), []byte(strings.Repeat("ab", 16)),
		[]byte(strings.Repeat("ab", 33)), []byte(strings.Repeat("g", 64)),
		append(append([]byte{}, encoded...), '\n', 'x'),
	} {
		if _, err := ParseSigningKeySeed(garbage); err == nil {
			t.Fatalf("garbage seed %q must fail closed", garbage)
		}
	}
	// A trailing newline (the file-editor reality) is tolerated: exactly
	// one, nothing else.
	withNewline := append(append([]byte{}, encoded...), '\n')
	if _, err := ParseSigningKeySeed(withNewline); err != nil {
		t.Fatalf("one trailing newline is tolerated: %v", err)
	}
}

// FuzzParseSigningKeySeed: the key file is read at every boot — never a
// panic, and only exact 32-byte hex seeds parse.
func FuzzParseSigningKeySeed(f *testing.F) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	f.Add([]byte(""))
	f.Add(EncodeSigningKeySeed(priv))
	f.Add([]byte(strings.Repeat("ff", 32)))
	f.Add([]byte("\x00\xffgarbage"))
	f.Fuzz(func(t *testing.T, raw []byte) {
		key, err := ParseSigningKeySeed(raw)
		if err != nil {
			return // failing closed is correct
		}
		if len(key) != ed25519.PrivateKeySize {
			t.Fatalf("a parsed key must be a full private key, got %d bytes", len(key))
		}
	})
}

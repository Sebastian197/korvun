// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The receipt canonical v2 — Etapa 5, lote 4, pieza 2 (spec FR-RCP,
// sealed NC-3α): approval_digest INSIDE the seal, version-aware
// canonicalization — v1 receipts keep their EXACT historical bytes
// (they verify FOREVER), v2 adds schema_version and approval_digest to
// the sealed form; the strict parser dispatches on the version; mixed
// eras coexist like the signing-key eras. Approved-red contract.

package action

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func testReceiptV1() Receipt {
	return Receipt{
		ReceiptID: "rcpt_00000000000000000000000000000001",
		ActionID:  "act_1", IntentDigest: "sha256:i", PrincipalID: "p",
		DecisionDigest: "sha256:d", ActionDigest: "sha256:a",
		EffectClass: EffectPure, Attempt: 1, Outcome: "DENIED",
		StartedAt:  time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC),
		FinishedAt: time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC),
		Partition:  "main", ChainSeq: 0,
		PreviousReceiptHash: GenesisPreviousHash,
	}
}

func TestReceiptV1_bytesAreFrozenForever(t *testing.T) {
	t.Parallel()
	r := testReceiptV1() // SchemaVersion zero-value = the v1 era
	raw := CanonicalReceipt(r)
	// The v1 wire must NOT contain the v2 fields — historical receipts
	// keep their exact bytes, so their stored hashes recompute forever.
	if bytes.Contains(raw, []byte("schema_version")) || bytes.Contains(raw, []byte("approval_digest")) {
		t.Fatalf("v1 canonical bytes must stay frozen: %s", raw)
	}
	parsed, err := ParseCanonicalReceipt(raw)
	if err != nil {
		t.Fatalf("v1 form must parse forever: %v", err)
	}
	if ComputeReceiptHash(parsed) != ComputeReceiptHash(r) {
		t.Fatal("v1 round trip must preserve the hash")
	}
}

func TestReceiptV2_sealsTheApprovalDigest(t *testing.T) {
	t.Parallel()
	r := testReceiptV1()
	r.SchemaVersion = 2
	r.ApprovalDigest = "sha256:approval"
	raw := CanonicalReceipt(r)
	if !bytes.Contains(raw, []byte(`"schema_version":2`)) || !bytes.Contains(raw, []byte("approval_digest")) {
		t.Fatalf("v2 canonical bytes must carry the new sealed fields: %s", raw)
	}
	parsed, err := ParseCanonicalReceipt(raw)
	if err != nil {
		t.Fatalf("v2 form must parse: %v", err)
	}
	if parsed.ApprovalDigest != "sha256:approval" || parsed.SchemaVersion != 2 {
		t.Fatalf("v2 round trip: %+v", parsed)
	}
	// The approval digest is INSIDE the seal: changing it changes the hash.
	tampered := r
	tampered.ApprovalDigest = "sha256:other"
	if ComputeReceiptHash(tampered) == ComputeReceiptHash(r) {
		t.Fatal("NC-3α: approval_digest must move the receipt hash")
	}
	// And v1 vs v2 of otherwise-equal receipts hash differently (the
	// version is part of the identity).
	v1 := testReceiptV1()
	v2 := v1
	v2.SchemaVersion = 2
	if ComputeReceiptHash(v1) == ComputeReceiptHash(v2) {
		t.Fatal("the era is part of the sealed identity")
	}
}

func TestReceiptV2_signatureCoversTheApproval(t *testing.T) {
	t.Parallel()
	pub, priv := testKeyPair(t)
	r := testReceiptV1()
	r.SchemaVersion = 2
	r.ApprovalDigest = "sha256:approval"
	r.ReceiptHash = ComputeReceiptHash(r)
	signed := SignReceipt(priv, r)
	if err := VerifyReceiptSignature(pub, signed); err != nil {
		t.Fatalf("v2 must sign and verify: %v", err)
	}
	forged := signed
	forged.ApprovalDigest = "sha256:forged"
	if err := VerifyReceiptSignature(pub, forged); err == nil {
		t.Fatal("a re-pointed approval reference must break the signature")
	}
}

func TestReceipt_unknownVersionFailsClosed(t *testing.T) {
	t.Parallel()
	raw := strings.Replace(string(CanonicalReceipt(func() Receipt {
		r := testReceiptV1()
		r.SchemaVersion = 2
		return r
	}())), `"schema_version":2`, `"schema_version":9`, 1)
	if _, err := ParseCanonicalReceipt([]byte(raw)); err == nil {
		t.Fatal("an unknown receipt schema version must fail closed")
	}
}

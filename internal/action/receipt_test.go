// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The receipt domain contract — Etapa 4, lote 1 (spec FR-REC, blueprint
// §10.10 subset v1): a canonicalized Execution Receipt, DETERMINISTIC on
// the fuzzed E1 canonicalizer — the same receipt, byte for byte — with
// its hash excluding hash+signature, the documented genesis link, the
// chain-link helper, and a birth-fuzzed parser (the verifier will read
// untrusted bytes). Tamper-EVIDENT, never immutable. Approved-red
// contract.

package action

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func testReceipt() Receipt {
	return Receipt{
		ReceiptID:           "rcpt_0123456789abcdef",
		ActionID:            "act_aaaa",
		IntentDigest:        "sha256:intent",
		PrincipalID:         "principal_brain_default",
		AuthorityDigest:     "sha256:grant",
		DecisionDigest:      "sha256:decision",
		ActionDigest:        "sha256:params",
		EffectClass:         EffectPure,
		Attempt:             1,
		Outcome:             string(StateSucceeded),
		ResultDigest:        "sha256:result",
		StartedAt:           time.Date(2026, 8, 30, 14, 0, 0, 0, time.UTC),
		FinishedAt:          time.Date(2026, 8, 30, 14, 0, 1, 0, time.UTC),
		Partition:           "main",
		ChainSeq:            7,
		PreviousReceiptHash: "sha256:prev",
		ReceiptHash:         "",
		SigningKeyID:        "",
		Signature:           "",
	}
}

func TestReceiptCanonical_deterministicByteForByte(t *testing.T) {
	t.Parallel()
	r := testReceipt()
	first := CanonicalReceipt(r)
	second := CanonicalReceipt(r)
	if !bytes.Equal(first, second) {
		t.Fatal("the canonical form must be byte-for-byte deterministic")
	}
	if len(first) == 0 {
		t.Fatal("the canonical form must not be empty")
	}
	// Signable form: hash and signature fields never participate.
	signed := r
	signed.ReceiptHash = "sha256:whatever"
	signed.SigningKeyID = "ed25519:abcd"
	signed.Signature = "deadbeef"
	if !bytes.Equal(CanonicalReceipt(signed), first) {
		t.Fatal("hash, key id and signature must be EXCLUDED from the signable form")
	}
}

func TestReceiptHash_coversEveryV1Field(t *testing.T) {
	t.Parallel()
	base := ComputeReceiptHash(testReceipt())
	if !strings.HasPrefix(base, "sha256:") {
		t.Fatalf("receipt hash carries the pinned algorithm, got %q", base)
	}
	mutations := map[string]func(*Receipt){
		"receipt_id":       func(r *Receipt) { r.ReceiptID = "rcpt_other" },
		"action_id":        func(r *Receipt) { r.ActionID = "act_bbbb" },
		"intent_digest":    func(r *Receipt) { r.IntentDigest = "sha256:x" },
		"principal_id":     func(r *Receipt) { r.PrincipalID = "principal_operator" },
		"authority_digest": func(r *Receipt) { r.AuthorityDigest = "sha256:y" },
		"decision_digest":  func(r *Receipt) { r.DecisionDigest = "sha256:z" },
		"action_digest":    func(r *Receipt) { r.ActionDigest = "sha256:w" },
		"effect_class":     func(r *Receipt) { r.EffectClass = EffectCritical },
		"attempt":          func(r *Receipt) { r.Attempt = 2 },
		"outcome":          func(r *Receipt) { r.Outcome = string(StateFailed) },
		"result_digest":    func(r *Receipt) { r.ResultDigest = "sha256:r2" },
		"started_at":       func(r *Receipt) { r.StartedAt = r.StartedAt.Add(time.Millisecond) },
		"finished_at":      func(r *Receipt) { r.FinishedAt = r.FinishedAt.Add(time.Millisecond) },
		"partition":        func(r *Receipt) { r.Partition = "other" },
		"chain_seq":        func(r *Receipt) { r.ChainSeq = 8 },
		"previous_hash":    func(r *Receipt) { r.PreviousReceiptHash = "sha256:p2" },
	}
	for name, mutate := range mutations {
		mutated := testReceipt()
		mutate(&mutated)
		if ComputeReceiptHash(mutated) == base {
			t.Fatalf("changing %s must change the receipt hash", name)
		}
	}
	// And the excluded trio does NOT move it.
	sealed := testReceipt()
	sealed.ReceiptHash = "sha256:h"
	sealed.SigningKeyID = "ed25519:k"
	sealed.Signature = "sig"
	if ComputeReceiptHash(sealed) != base {
		t.Fatal("hash/key/signature must never move the recomputed hash")
	}
}

func TestGenesisPreviousHash_isTheDocumentedEmptyHash(t *testing.T) {
	t.Parallel()
	empty := sha256.Sum256(nil)
	want := "sha256:" + hex.EncodeToString(empty[:])
	if GenesisPreviousHash != want {
		t.Fatalf("the genesis link is the sha256 of empty, documented: got %q want %q", GenesisPreviousHash, want)
	}
}

func TestReceiptChain_linkHelper(t *testing.T) {
	t.Parallel()
	first := testReceipt()
	first.ChainSeq = 0
	first.PreviousReceiptHash = GenesisPreviousHash
	first.ReceiptHash = ComputeReceiptHash(first)
	second := testReceipt()
	second.ReceiptID = "rcpt_second"
	LinkReceipt(&second, first)
	if second.ChainSeq != 1 {
		t.Fatalf("the chain sequence must increment, got %d", second.ChainSeq)
	}
	if second.PreviousReceiptHash != first.ReceiptHash {
		t.Fatal("the link must carry the previous receipt's hash")
	}
	if second.Partition != first.Partition {
		t.Fatal("a link never crosses partitions")
	}
}

func TestReceiptCanonical_roundTripsThroughTheParser(t *testing.T) {
	t.Parallel()
	r := testReceipt()
	r.ReceiptHash = ComputeReceiptHash(r)
	parsed, err := ParseCanonicalReceipt(CanonicalReceipt(r))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// The signable fields round-trip whole (hash/key/signature travel
	// separately — they are not in the signable form by design).
	r.ReceiptHash, r.SigningKeyID, r.Signature = "", "", ""
	if parsed != r {
		t.Fatalf("round-trip:\ngot  %+v\nwant %+v", parsed, r)
	}
}

func TestParseCanonicalReceipt_failsClosedOnGarbage(t *testing.T) {
	t.Parallel()
	for _, garbage := range [][]byte{
		nil, []byte(""), []byte("{"), []byte(`"just a string"`), []byte(`[1,2]`),
		[]byte(`{"receipt_id": 42}`),
	} {
		if _, err := ParseCanonicalReceipt(garbage); err == nil {
			t.Fatalf("garbage %q must fail closed", garbage)
		}
	}
}

// FuzzReceiptCanonical: the verifier reads UNTRUSTED bytes — never a
// panic, and whatever parses re-canonicalizes idempotently (the fixed
// point law of the E1 canonicalizer, inherited).
func FuzzReceiptCanonical(f *testing.F) {
	seed := testReceipt()
	f.Add(CanonicalReceipt(seed))
	// The v2 era seed (Etapa 5, NC-3α): the fuzzer walks both wires.
	seedV2 := testReceipt()
	seedV2.SchemaVersion = 2
	seedV2.ApprovalDigest = "sha256:approval"
	f.Add(CanonicalReceipt(seedV2))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"receipt_id":"rcpt_x","chain_seq":-1}`))
	f.Add([]byte("\x00\xff garbage"))
	f.Fuzz(func(t *testing.T, raw []byte) {
		parsed, err := ParseCanonicalReceipt(raw)
		if err != nil {
			return // failing closed on garbage is correct
		}
		once := CanonicalReceipt(parsed)
		reparsed, err := ParseCanonicalReceipt(once)
		if err != nil {
			t.Fatalf("a parsed receipt must re-parse: %v", err)
		}
		if !bytes.Equal(CanonicalReceipt(reparsed), once) {
			t.Fatal("canonicalization must be idempotent (fixed point)")
		}
	})
}

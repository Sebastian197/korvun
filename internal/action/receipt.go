// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Receipt domain (Trust Layer Etapa 4, lote 1, spec FR-REC, blueprint
// §10.10 subset v1): the canonicalized Execution Receipt — the action ledger's
// row reified into verifiable evidence. Canonicalization is
// DETERMINISTIC on the fuzzed E1 canonicalizer (sorted keys,
// RFC3339Nano UTC times, the contract-digest mold): the same receipt is
// the same bytes, on any machine. The hash chain this feeds provides
// TAMPER EVIDENCE — it must never be described as immutability: the
// operator controls storage and keys (§19.3, the honesty sentence).
//
// RESERVED §10.10 fields and the stages that wake them (the E2
// discipline — an unreachable field is a field nobody can misuse):
// transaction_id (E6), approval_digest (E5), executor_id/target_system/
// external_reference (E7 — today there is exactly ONE in-process
// executor; a constant would be decoration), tenant (E10),
// protected_params_ref (§30-2: nothing raw is ever persisted — the
// encryption decision re-opens when a stage stores protected content).
package action

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// Receipt is the §10.10 v1 subset: one terminal outcome, reified.
// ReceiptHash, SigningKeyID and Signature SEAL the receipt — they are
// computed over (and therefore excluded from) the signable form.
type Receipt struct {
	// ReceiptID is the receipt identity ("rcpt_" namespace).
	ReceiptID string
	// SchemaVersion pins the sealed wire form: 0/1 = the frozen v1 era
	// (historical bytes verify FOREVER), 2 = the Etapa-5 era with the
	// approval reference inside the seal (sealed NC-3α).
	SchemaVersion int
	// ActionID ties the receipt to its action row.
	ActionID string
	// IntentDigest / AuthorityDigest / DecisionDigest / ActionDigest are
	// the term digests of what was asked, under which contract, by which
	// law — digests only, never content.
	IntentDigest    string
	PrincipalID     string
	AuthorityDigest string
	DecisionDigest  string
	ActionDigest    string
	// EffectClass is the woken consequence class (E3).
	EffectClass EffectClass
	// Attempt is pinned at 1 in v1 (retries arrive with E6).
	Attempt int
	// Outcome is the terminal state; ResultDigest is computed on the fly
	// over the observation (sealed NC-3) — raw content never persisted.
	Outcome      string
	ResultDigest string
	// ApprovalDigest references the CONSUMED approval's decision digest
	// (v2 seal, §10.10). R5-S1: every approval-DECIDED outcome seals —
	// approved and refused alike; "" is honest only where no approval
	// ever existed for the action.
	ApprovalDigest string
	// StartedAt / FinishedAt bound the attempt, UTC.
	StartedAt  time.Time
	FinishedAt time.Time
	// Partition and ChainSeq place the receipt on its hash chain; v1
	// ships the single "main" partition (E10 shards without a break).
	Partition string
	ChainSeq  int64
	// PreviousReceiptHash links the chain (GenesisPreviousHash first).
	PreviousReceiptHash string
	// ReceiptHash / SigningKeyID / Signature seal the receipt (lote 2
	// signs; this lote computes the hash).
	ReceiptHash  string
	SigningKeyID string
	Signature    string
}

// GenesisPreviousHash is the documented chain origin: the pinned-form
// sha256 of empty input. The first receipt of a partition links here.
const GenesisPreviousHash = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// receiptWire is the canonical wire form of the SIGNABLE fields — one
// struct, one JSON shape, sorted keys by construction (encoding/json
// emits struct fields in declaration order; the canonicalizer re-sorts
// object keys, making declaration order irrelevant to the bytes).
type receiptWire struct {
	ReceiptID           string `json:"receipt_id"`
	ActionID            string `json:"action_id"`
	IntentDigest        string `json:"intent_digest"`
	PrincipalID         string `json:"principal_id"`
	AuthorityDigest     string `json:"authority_digest"`
	DecisionDigest      string `json:"decision_digest"`
	ActionDigest        string `json:"action_digest"`
	EffectClass         string `json:"effect_class"`
	Attempt             int    `json:"attempt"`
	Outcome             string `json:"outcome"`
	ResultDigest        string `json:"result_digest"`
	StartedAt           string `json:"started_at"`
	FinishedAt          string `json:"finished_at"`
	Partition           string `json:"partition"`
	ChainSeq            int64  `json:"chain_seq"`
	PreviousReceiptHash string `json:"previous_receipt_hash"`
}

// receiptWireV2 is the Etapa-5 sealed form: the v1 fields PLUS the
// schema version and the approval reference — inside the seal, so a
// re-pointed approval breaks hash and signature alike (NC-3α).
type receiptWireV2 struct {
	SchemaVersion       int    `json:"schema_version"`
	ReceiptID           string `json:"receipt_id"`
	ActionID            string `json:"action_id"`
	IntentDigest        string `json:"intent_digest"`
	PrincipalID         string `json:"principal_id"`
	AuthorityDigest     string `json:"authority_digest"`
	DecisionDigest      string `json:"decision_digest"`
	ActionDigest        string `json:"action_digest"`
	ApprovalDigest      string `json:"approval_digest"`
	EffectClass         string `json:"effect_class"`
	Attempt             int    `json:"attempt"`
	Outcome             string `json:"outcome"`
	ResultDigest        string `json:"result_digest"`
	StartedAt           string `json:"started_at"`
	FinishedAt          string `json:"finished_at"`
	Partition           string `json:"partition"`
	ChainSeq            int64  `json:"chain_seq"`
	PreviousReceiptHash string `json:"previous_receipt_hash"`
}

// CanonicalReceipt returns the deterministic signable byte form of a
// receipt, dispatching on its era: SchemaVersion 0/1 renders the
// FROZEN v1 wire byte-for-byte (historical hashes recompute forever);
// 2 renders the v2 wire with the approval reference inside the seal.
// ReceiptHash, SigningKeyID and Signature stay EXCLUDED (they seal
// these bytes). Rides the fuzzed E1 canonicalizer.
func CanonicalReceipt(r Receipt) []byte {
	if r.SchemaVersion >= 2 {
		wire := receiptWireV2{
			SchemaVersion:       r.SchemaVersion,
			ReceiptID:           r.ReceiptID,
			ActionID:            r.ActionID,
			IntentDigest:        r.IntentDigest,
			PrincipalID:         r.PrincipalID,
			AuthorityDigest:     r.AuthorityDigest,
			DecisionDigest:      r.DecisionDigest,
			ActionDigest:        r.ActionDigest,
			ApprovalDigest:      r.ApprovalDigest,
			EffectClass:         string(r.EffectClass),
			Attempt:             r.Attempt,
			Outcome:             r.Outcome,
			ResultDigest:        r.ResultDigest,
			StartedAt:           timeTerm(r.StartedAt),
			FinishedAt:          timeTerm(r.FinishedAt),
			Partition:           r.Partition,
			ChainSeq:            r.ChainSeq,
			PreviousReceiptHash: r.PreviousReceiptHash,
		}
		raw, err := json.Marshal(wire)
		if err != nil {
			// Unreachable: plain strings and ints.
			panic("action: canonical receipt v2 encoding failed: " + err.Error())
		}
		return raw
	}
	wire := receiptWire{
		ReceiptID:           r.ReceiptID,
		ActionID:            r.ActionID,
		IntentDigest:        r.IntentDigest,
		PrincipalID:         r.PrincipalID,
		AuthorityDigest:     r.AuthorityDigest,
		DecisionDigest:      r.DecisionDigest,
		ActionDigest:        r.ActionDigest,
		EffectClass:         string(r.EffectClass),
		Attempt:             r.Attempt,
		Outcome:             r.Outcome,
		ResultDigest:        r.ResultDigest,
		StartedAt:           timeTerm(r.StartedAt),
		FinishedAt:          timeTerm(r.FinishedAt),
		Partition:           r.Partition,
		ChainSeq:            r.ChainSeq,
		PreviousReceiptHash: r.PreviousReceiptHash,
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		// Unreachable for the plain types above; kept for honesty.
		return []byte("unmarshalable")
	}
	return CanonicalParams(string(raw))
}

// ComputeReceiptHash returns the pinned-form hash over the signable
// bytes. Verification NEVER trusts a stored hash: it recomputes.
func ComputeReceiptHash(r Receipt) string {
	sum := sha256.Sum256(CanonicalReceipt(r))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// LinkReceipt places next on the chain after prev: same partition,
// incremented sequence, previous hash carried. A link never crosses
// partitions.
func LinkReceipt(next *Receipt, prev Receipt) {
	next.Partition = prev.Partition
	next.ChainSeq = prev.ChainSeq + 1
	next.PreviousReceiptHash = prev.ReceiptHash
}

// ParseCanonicalReceipt parses UNTRUSTED canonical bytes back into the
// signable fields, failing closed on anything that is not exactly one
// JSON object of the wire shape. Times parse from their canonical form;
// a zero-time term ("") stays zero.
func ParseCanonicalReceipt(raw []byte) (Receipt, error) {
	// Version sniff (lax, fields only), then the STRICT parse of the
	// matching era's wire. Unknown versions fail closed.
	var sniff struct {
		SchemaVersion int `json:"schema_version"`
	}
	_ = json.Unmarshal(raw, &sniff)
	switch sniff.SchemaVersion {
	case 0:
		// The frozen v1 era (no schema_version field on the wire).
	case 2:
		var w2 receiptWireV2
		if err := strictUnmarshal(raw, &w2); err != nil {
			return Receipt{}, fmt.Errorf("action: parse canonical receipt v2: %w", err)
		}
		startedAt, err := parseTimeTerm(w2.StartedAt)
		if err != nil {
			return Receipt{}, fmt.Errorf("action: parse receipt v2 started_at: %w", err)
		}
		finishedAt, err := parseTimeTerm(w2.FinishedAt)
		if err != nil {
			return Receipt{}, fmt.Errorf("action: parse receipt v2 finished_at: %w", err)
		}
		return Receipt{
			SchemaVersion:       w2.SchemaVersion,
			ReceiptID:           w2.ReceiptID,
			ActionID:            w2.ActionID,
			IntentDigest:        w2.IntentDigest,
			PrincipalID:         w2.PrincipalID,
			AuthorityDigest:     w2.AuthorityDigest,
			DecisionDigest:      w2.DecisionDigest,
			ActionDigest:        w2.ActionDigest,
			ApprovalDigest:      w2.ApprovalDigest,
			EffectClass:         EffectClass(w2.EffectClass),
			Attempt:             w2.Attempt,
			Outcome:             w2.Outcome,
			ResultDigest:        w2.ResultDigest,
			StartedAt:           startedAt,
			FinishedAt:          finishedAt,
			Partition:           w2.Partition,
			ChainSeq:            w2.ChainSeq,
			PreviousReceiptHash: w2.PreviousReceiptHash,
		}, nil
	default:
		return Receipt{}, fmt.Errorf("action: parse canonical receipt: unknown schema version %d", sniff.SchemaVersion)
	}
	var wire receiptWire
	if err := strictUnmarshal(raw, &wire); err != nil {
		return Receipt{}, fmt.Errorf("action: parse canonical receipt: %w", err)
	}
	r := Receipt{
		ReceiptID:           wire.ReceiptID,
		ActionID:            wire.ActionID,
		IntentDigest:        wire.IntentDigest,
		PrincipalID:         wire.PrincipalID,
		AuthorityDigest:     wire.AuthorityDigest,
		DecisionDigest:      wire.DecisionDigest,
		ActionDigest:        wire.ActionDigest,
		EffectClass:         EffectClass(wire.EffectClass),
		Attempt:             wire.Attempt,
		Outcome:             wire.Outcome,
		ResultDigest:        wire.ResultDigest,
		Partition:           wire.Partition,
		ChainSeq:            wire.ChainSeq,
		PreviousReceiptHash: wire.PreviousReceiptHash,
	}
	var err error
	if r.StartedAt, err = parseTimeTerm(wire.StartedAt); err != nil {
		return Receipt{}, fmt.Errorf("action: parse receipt started_at: %w", err)
	}
	if r.FinishedAt, err = parseTimeTerm(wire.FinishedAt); err != nil {
		return Receipt{}, fmt.Errorf("action: parse receipt finished_at: %w", err)
	}
	return r, nil
}

// strictUnmarshal decodes exactly one JSON object with known fields —
// unknown fields and trailing garbage fail closed (untrusted input).
func strictUnmarshal(raw []byte, into any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("trailing content after the receipt object")
	}
	return nil
}

// parseTimeTerm reads a canonical time term ("" = zero time).
func parseTimeTerm(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, raw)
}

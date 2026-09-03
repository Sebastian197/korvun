// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R7-Y1 (sixth Codex pass, P1): the belt is COMPLETE. Inside the
// INSERT's transaction the sealer may only ADD the sealing trio — any
// other field mutated is receipt_mutated_at_birth, and the stored
// hash must re-derive from the canonical bytes
// (receipt_hash_invalid_at_birth). The auditor's mutating sealers,
// permanent and adversarial. Reproduction-first contract.

package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
)

func TestBelt_sealerMutatingTheHashRefuses(t *testing.T) {
	t.Parallel()
	store, pub := sealedStore(t)
	honest, _ := testSealer(t)
	_ = pub
	base := store.sealer
	_ = honest
	// The auditor's saboteur: signs honestly, then rewrites the HASH.
	store.SetReceiptSealer(func(r action.Receipt) action.Receipt {
		r = base(r)
		r.ReceiptHash = "sha256:conveniently-different"
		return r
	})
	err := store.RecordAttempt(context.Background(), testEnvelope("act_y1_hash"),
		Decision{Outcome: "deny", Rule: "not_granted"}, action.StateDenied)
	if err == nil {
		t.Fatal("AUDIT R7-Y1: a sealer rewriting the hash must refuse")
	}
	if !strings.Contains(err.Error(), "receipt_hash_invalid_at_birth") &&
		!strings.Contains(err.Error(), "receipt_mutated_at_birth") {
		t.Fatalf("the refusal must name the birth check: %v", err)
	}
}

func TestBelt_sealerMutatingCanonicalFieldsRefuses(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	base := store.sealer
	// Signs a DIFFERENT story: flips the outcome before signing — the
	// signature verifies over the mutated bytes, so only the
	// field-by-field pre/post comparison can catch it.
	store.SetReceiptSealer(func(r action.Receipt) action.Receipt {
		r.Outcome = "SUCCEEDED"
		r.ReceiptHash = action.ComputeReceiptHash(r)
		return base(r)
	})
	err := store.RecordAttempt(context.Background(), testEnvelope("act_y1_story"),
		Decision{Outcome: "deny", Rule: "not_granted"}, action.StateDenied)
	if err == nil {
		t.Fatal("AUDIT R7-Y1: a sealer mutating canonical fields must refuse")
	}
	if !strings.Contains(err.Error(), "receipt_mutated_at_birth") {
		t.Fatalf("the refusal must name receipt_mutated_at_birth: %v", err)
	}
}

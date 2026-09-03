// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R6-X1 (fifth Codex pass, P1): the belt verifies THE SIGNATURE. The
// old guard checked registry state but skipped everything when the
// sealer stamped no key id — a degenerate sealer (identity, or
// active-key-with-empty-signature) produced unverifiable receipts
// silently. Now, inside the INSERT's transaction: no key id =
// receipt_unsigned; an empty or non-verifying signature against the
// REGISTERED public key = signature_invalid_at_birth. The auditor's
// two degenerate sealers, permanent and adversarial.
// Reproduction-first contract.

package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
)

func TestBelt_identitySealerRefusesReceiptUnsigned(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	t.Cleanup(func() { _ = store.Close() })
	// The auditor's degenerate sealer #1: identity — no key, no ink.
	store.SetReceiptSealer(func(r action.Receipt) action.Receipt { return r })
	err := store.RecordAttempt(context.Background(), testEnvelope("act_x1_id"),
		Decision{Outcome: "deny", Rule: "not_granted"}, action.StateDenied)
	if err == nil {
		t.Fatal("AUDIT R6-X1: an identity sealer must refuse — the skip-if-empty guard is dead")
	}
	if !strings.Contains(err.Error(), "receipt_unsigned") {
		t.Fatalf("the refusal must carry receipt_unsigned: %v", err)
	}
}

func TestBelt_activeKeyEmptySignatureRefusesAtBirth(t *testing.T) {
	t.Parallel()
	store, pub := sealedStore(t)
	// The auditor's degenerate sealer #2: stamps the ACTIVE key id but
	// an empty signature.
	store.SetReceiptSealer(func(r action.Receipt) action.Receipt {
		r.SigningKeyID = action.SigningKeyID(pub)
		r.Signature = ""
		return r
	})
	err := store.RecordAttempt(context.Background(), testEnvelope("act_x1_empty"),
		Decision{Outcome: "deny", Rule: "not_granted"}, action.StateDenied)
	if err == nil {
		t.Fatal("AUDIT R6-X1: an empty signature must refuse at birth")
	}
	if !strings.Contains(err.Error(), "signature_invalid_at_birth") {
		t.Fatalf("the refusal must carry signature_invalid_at_birth: %v", err)
	}
}

func TestBelt_wrongKeySignatureRefusesAtBirth(t *testing.T) {
	t.Parallel()
	store, pub := sealedStore(t)
	// A signature made by ANOTHER key while stamping the active id.
	otherSealer, _ := testSealer(t)
	store.SetReceiptSealer(func(r action.Receipt) action.Receipt {
		r = otherSealer(r)
		r.SigningKeyID = action.SigningKeyID(pub) // lies about the era
		return r
	})
	err := store.RecordAttempt(context.Background(), testEnvelope("act_x1_wrong"),
		Decision{Outcome: "deny", Rule: "not_granted"}, action.StateDenied)
	if err == nil {
		t.Fatal("AUDIT R6-X1: a non-verifying signature must refuse at birth")
	}
	if !strings.Contains(err.Error(), "signature_invalid_at_birth") {
		t.Fatalf("the refusal must carry signature_invalid_at_birth: %v", err)
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R4 Phase 1 belt (FR-R4F1-3): sealing a receipt with a key the
// registry marks RETIRED refuses BY NAME inside the receipt's own
// transaction — never a silent invalid signature. Keys the registry
// does not know stay un-vetoed here (test seams sign with unregistered
// keys; the offline verifier's key_unknown check owns that axis).
// Reproduction-first contract.

package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

func TestSealerBelt_retiredKeyRefusesByName(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	// Learn the key id the sealer stamps, from a first honest receipt.
	mustRecord(t, store, "act_belt_probe", action.StateDenied)
	probe, err := store.ReceiptsByAction(ctx, "act_belt_probe")
	if err != nil || len(probe) != 1 {
		t.Fatalf("probe receipt: %v %d", err, len(probe))
	}
	keyID := probe[0].SigningKeyID
	// RETIRE the seam-registered key out of band (R5-S4: sealedStore
	// registers it, so this is an UPDATE now).
	if _, err := store.db.Exec(
		`UPDATE signing_keys SET retired_at = ? WHERE key_id = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), keyID); err != nil {
		t.Fatalf("retire: %v", err)
	}
	err = store.RecordAttempt(ctx, testEnvelope("act_belt"),
		Decision{Outcome: "deny", Rule: "not_granted"}, action.StateDenied)
	if err == nil {
		t.Fatal("AUDIT R4-F1: a retired key must never seal a new receipt")
	}
	if !strings.Contains(err.Error(), "signing_key_retired") {
		t.Fatalf("the refusal must carry the stable rule signing_key_retired: %v", err)
	}
}

// R5-S4 (fourth Codex pass, P2): the belt fails CLOSED. An absent
// registry row or a registry read error at seal time is a NAMED
// refusal — never a silently unverifiable receipt.
func TestSealerBelt_unregisteredKeyRefusesByName(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	t.Cleanup(func() { _ = store.Close() })
	sealer, _ := testSealer(t)
	store.SetReceiptSealer(sealer) // deliberately NOT registered
	err := store.RecordAttempt(context.Background(), testEnvelope("act_s4_unreg"),
		Decision{Outcome: "deny", Rule: "not_granted"}, action.StateDenied)
	if err == nil {
		t.Fatal("AUDIT R5-S4: an unregistered key must never seal")
	}
	if !strings.Contains(err.Error(), "signing_key_unregistered") {
		t.Fatalf("the refusal must carry signing_key_unregistered: %v", err)
	}
}

func TestSealerBelt_registryErrorRefusesByName(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	// The registry becomes unreadable (renamed out from under the belt).
	if _, err := store.db.Exec(`ALTER TABLE signing_keys RENAME TO signing_keys_gone`); err != nil {
		t.Fatalf("sabotage: %v", err)
	}
	err := store.RecordAttempt(context.Background(), testEnvelope("act_s4_err"),
		Decision{Outcome: "deny", Rule: "not_granted"}, action.StateDenied)
	if err == nil {
		t.Fatal("AUDIT R5-S4: a registry error must refuse, never limp on")
	}
	if !strings.Contains(err.Error(), "key_registry_unavailable") {
		t.Fatalf("the refusal must carry key_registry_unavailable: %v", err)
	}
}

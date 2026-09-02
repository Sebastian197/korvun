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
	// Register the key and RETIRE it out of band.
	if _, err := store.db.Exec(
		`INSERT INTO signing_keys (key_id, public_key, created_at, retired_at)
		 VALUES (?, 'deadbeef', ?, ?)`,
		keyID, time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano),
		time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
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

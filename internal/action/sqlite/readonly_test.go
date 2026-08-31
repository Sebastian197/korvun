// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The read-only door — consolidation R1 (external audit, CRITICAL):
// verification and consultation must NEVER mutate the store. The old
// single door (Open) ran migrations, the crash-recovery pass and the
// retention prune on EVERY open — so a `korvun receipt verify` while
// the server had an action IN FLIGHT closed it as crash_recovered
// behind the server's back (the reviewer's reproducible scenario; the
// ceremony's own CLI-migrated-the-profile precedent). OpenReadOnly is
// the new door: no bootstrap, no migration, no recovery, no prune, and
// the CONNECTION ITSELF refuses every write (PRAGMA query_only) — the
// whole gate pinned, not one command. Reproduction-first contract.

package sqlite

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

func TestOpenReadOnly_theReviewerScenario_inFlightActionSurvives(t *testing.T) {
	t.Parallel()
	// The WRITER (the server) is alive with an action IN FLIGHT.
	writer, path := openTemp(t)
	ctx := context.Background()
	if err := writer.RecordAttempt(ctx, testEnvelope("act_inflight"),
		Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized); err != nil {
		t.Fatalf("record in-flight: %v", err)
	}
	// The READER (verify/check/consultation) opens the SAME live file.
	reader, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("OpenReadOnly on a live store: %v", err)
	}
	defer func() { _ = reader.Close() }()
	// THE PIN: the in-flight action is INTACT — no crash_recovered, no
	// forced close, no recovery marker.
	rec, err := reader.Get(ctx, "act_inflight")
	if err != nil {
		t.Fatalf("read through the RO door: %v", err)
	}
	if rec.State != action.StateAuthorized || rec.RecoveryMarker != "" {
		t.Fatalf("REGRESSION (the audit's critical finding): the read-only open touched the in-flight action: state=%s marker=%q", rec.State, rec.RecoveryMarker)
	}
	// And the original writer closes its action successfully.
	if err := writer.Finish(ctx, "act_inflight", action.StateSucceeded, time.Now().UTC()); err != nil {
		t.Fatalf("the writer must still own its lifecycle: %v", err)
	}
}

func TestOpenReadOnly_theConnectionRefusesEveryWrite(t *testing.T) {
	t.Parallel()
	writer, path := openTemp(t)
	ctx := context.Background()
	if err := writer.RecordAttempt(ctx, testEnvelope("act_ro"),
		Decision{Outcome: "deny", Rule: "not_granted"}, action.StateDenied); err != nil {
		t.Fatalf("seed: %v", err)
	}
	reader, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer func() { _ = reader.Close() }()
	// Belt over the design: even a hand-written mutation through the RO
	// handle dies at the connection.
	if _, err := reader.db.ExecContext(ctx, `UPDATE actions SET state = 'FORGED'`); err == nil {
		t.Fatal("the RO connection must refuse writes at the SQLite level")
	}
	// The domain mutation paths refuse too.
	if err := reader.Finish(ctx, "act_ro", action.StateSucceeded, time.Now().UTC()); err == nil {
		t.Fatal("Finish through the RO door must fail")
	}
}

func TestOpenReadOnly_neverMigratesAnOldProfile(t *testing.T) {
	t.Parallel()
	// The ceremony precedent, reproduced: a hand-built OLD profile
	// (v4 — no receipts) must NOT be migrated by a read-only consult.
	path := buildV4File(t)
	_, err := OpenReadOnly(path)
	if err == nil {
		t.Fatal("a pre-receipts schema must be refused by the RO door, never migrated")
	}
	if !strings.Contains(err.Error(), "schema") {
		t.Fatalf("the refusal names the schema mismatch: %v", err)
	}
	if v := inspect(t, path, `SELECT version FROM action_schema`); v != 4 {
		t.Fatalf("the RO door must leave the old profile UNTOUCHED at v4, got v%d", v)
	}
}

func TestOpenReadOnly_missingFileFailsHonest(t *testing.T) {
	t.Parallel()
	if _, err := OpenReadOnly(t.TempDir() + "/nope.db"); err == nil {
		t.Fatal("a missing store must fail honest — the RO door never creates files")
	}
}

func TestOpenReadOnly_readSurfacesWork(t *testing.T) {
	t.Parallel()
	writer, path := openTemp(t)
	t.Cleanup(func() { _ = writer.Close() })
	sealer, pub := testSealer(t)
	writer.SetReceiptSealer(sealer)
	ctx := context.Background()
	if err := writer.PutSigningKey(ctx, action.SigningKeyID(pub), hex.EncodeToString(pub), time.Now().UTC()); err != nil {
		t.Fatalf("register test key: %v", err)
	}
	if err := writer.RecordAttempt(ctx, testEnvelope("act_read"),
		Decision{Outcome: "deny", Rule: "not_granted"}, action.StateDenied); err != nil {
		t.Fatalf("seed: %v", err)
	}
	reader, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer func() { _ = reader.Close() }()
	receipts, err := reader.ReceiptsByAction(ctx, "act_read")
	if err != nil || len(receipts) != 1 {
		t.Fatalf("the verifier's reads must work through the RO door: %v %d", err, len(receipts))
	}
	if err := action.VerifyReceiptSignature(pub, receipts[0]); err != nil {
		t.Fatalf("verify through the RO door: %v", err)
	}
	if _, err := reader.ListReceipts(ctx, "main"); err != nil {
		t.Fatalf("ledger check reads: %v", err)
	}
	if _, err := reader.ActiveSigningKey(ctx); err != nil {
		t.Fatalf("key registry reads: %v", err)
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R3 of the third Codex pass (adjudicated 2026-09-01): the recovery
// pass ran INSIDE Open, before the keystore/sealer existed — so its
// terminal closes (FAILED/crash_recovered, OUTCOME_UNKNOWN) were the
// only terminals born WITHOUT a receipt, silently exempt from the
// ledger. The boot reorders: open → keystore → sealer → recovery, and
// the recovery closes per row through the store's domain API WITH its
// era's signed receipt. Pinned here on the REAL boot: no terminal is
// born unsigned, recovery's included. Reproduction-first contract.

package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
)

func TestBootRecovery_closesWithSignedReceipts(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "korvun.db")
	// A previous life leaves an in-flight action behind (no sealer —
	// the crash predates any ledger ink for it).
	store, err := actionsqlite.OpenOperator(dbPath)
	if err != nil {
		t.Fatalf("seed open: %v", err)
	}
	env := action.NewEnvelope("act_r3_crash", "env-r3",
		action.Source{Kind: "agent_brain", Protocol: "text", Channel: "console"},
		action.Operation{Namespace: "tool", Name: "calc", Version: 1},
		`1+1`, time.Now().UTC())
	if err := store.RecordAttempt(context.Background(), env,
		actionsqlite.Decision{Outcome: "allow", Rule: "allow"}, action.StateAuthorized); err != nil {
		t.Fatalf("record: %v", err)
	}
	_ = store.Close()

	// The REAL boot: Build wires keystore and sealer, THEN recovers.
	app, err := Build(kernelWiringConfig(dbPath), withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	shutdownApp(t, app)

	ro, err := actionsqlite.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = ro.Close() }()
	ctx := context.Background()
	rec, err := ro.Get(ctx, "act_r3_crash")
	if err != nil || rec.State != action.StateFailed || rec.RecoveryMarker != "crash_recovered" {
		t.Fatalf("the crash orphan closes honestly: %v %v %q", err, rec.State, rec.RecoveryMarker)
	}
	// AUDIT R3: the close is IN THE LEDGER — a signed receipt, like
	// every other terminal.
	receipts, err := ro.ReceiptsByAction(ctx, "act_r3_crash")
	if err != nil || len(receipts) != 1 {
		t.Fatalf("no terminal is born unsigned — recovery's included: %v %d", err, len(receipts))
	}
	r := receipts[0]
	if r.Outcome != string(action.StateFailed) || r.SigningKeyID == "" || r.Signature == "" {
		t.Fatalf("the recovery receipt carries the outcome and the era's ink: %+v", r)
	}
}

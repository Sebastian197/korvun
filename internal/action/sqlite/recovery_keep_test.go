// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.16.2 · the recovery pass spares what the LIVING process still owns (D5).
//
// Captured before this file existed (evidence/v0.16.2/probe-real-cutover.txt):
// after a real cutover that SUCCEEDED, the operator act sealed by the old app
// ended OUTCOME_UNKNOWN — the new app's boot recovery closed it as an orphan of a
// «previous life» while the supervisor that owned the change was alive and about
// to learn the outcome. A cutover is not a previous life.
//
// `keep` names the actions the living process governs. It is NOT an exemption
// by state or by namespace: an act nobody names is recovered exactly as before,
// and that leg is the CONTROL of this mould.
//
// Evidence level, honest: in-process, real store on a real file. «Another process
// died» is simulated by an empty keep over the same file, not by a separate OS
// process.

package sqlite

import (
	"context"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
)

// TestRecoverPreviousLife_keepsOnlyWhatTheLivingProcessOwns: with keep naming
// act_1, act_1 stays AUTHORIZED and act_2 closes OUTCOME_UNKNOWN with its
// receipt; with an empty keep, act_1 closes too.
//
// PROBING MUTATIONS (both to be executed): (1) drop the keep filter — act_1
// closes and the first leg reddens; (2) with a non-empty keep skip EVERY
// AUTHORIZED row — act_2 survives and the control leg reddens.
func TestRecoverPreviousLife_keepsOnlyWhatTheLivingProcessOwns(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// The boot wires a SIGNING receipt sealer BEFORE the recovery pass (R3),
	// and appendReceiptTx writes no receipt without one — so the mould uses the
	// suite's sealed store, or its receipt assertion would be measuring the
	// missing sealer rather than the pass.
	store, _ := sealedStore(t)
	mustRecord(t, store, "act_1", action.StateAuthorized) // owned by the living process
	mustRecord(t, store, "act_2", action.StateAuthorized) // nobody's: a true orphan

	if _, err := store.RecoverPreviousLife(ctx, "act_1"); err != nil {
		t.Fatalf("recovery with keep: %v", err)
	}
	kept, err := store.Get(ctx, "act_1")
	if err != nil {
		t.Fatalf("read act_1: %v", err)
	}
	if kept.State != action.StateAuthorized || kept.RecoveryMarker != "" {
		t.Fatalf("the act the living process owns was recovered: state=%q marker=%q", kept.State, kept.RecoveryMarker)
	}
	orphan, err := store.Get(ctx, "act_2")
	if err != nil {
		t.Fatalf("read act_2: %v", err)
	}
	if orphan.State != action.StateOutcomeUnknown || orphan.RecoveryMarker != recoveryMarkerOutcomeUnknown {
		t.Fatalf("the orphan nobody named was not recovered: state=%q marker=%q — keep must not widen into an exemption", orphan.State, orphan.RecoveryMarker)
	}
	receipts, err := store.ReceiptsByAction(ctx, "act_2")
	if err != nil || len(receipts) != 1 {
		t.Fatalf("the recovered orphan has %d receipts (err %v), want its terminal receipt", len(receipts), err)
	}

	// The next life of the process: the registry is empty, so nothing is kept,
	// and what the dead process left open is recovered as before.
	if _, err := store.RecoverPreviousLife(ctx); err != nil {
		t.Fatalf("recovery without keep: %v", err)
	}
	after, err := store.Get(ctx, "act_1")
	if err != nil {
		t.Fatalf("read act_1 again: %v", err)
	}
	if after.State != action.StateOutcomeUnknown {
		t.Fatalf("with nobody owning it, act_1 stayed %q: an empty keep must recover everything", after.State)
	}
}

// TestRecoverPreviousLife_keepNamingNothingRealChangesNothing: a keep entry for
// an id that is not in the store is not an error and protects nothing — the
// registry may remember an act of another ledger.
func TestRecoverPreviousLife_keepNamingNothingRealChangesNothing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, _ := openTemp(t)
	mustRecord(t, store, "act_7", action.StateAuthorized)
	if _, err := store.RecoverPreviousLife(ctx, "act_of_another_ledger"); err != nil {
		t.Fatalf("recovery with a foreign keep: %v", err)
	}
	got, _ := store.Get(ctx, "act_7")
	if got.State != action.StateOutcomeUnknown {
		t.Fatalf("act_7 stayed %q under a keep that names nothing here", got.State)
	}
}

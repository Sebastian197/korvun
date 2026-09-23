// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Ficha in `docs/HANDOFF.md` — `GetApprovalByAction` returned its read failure
// with no class.
//
// THE FAULT. Every approval read door answers one of three named classes
// through `classifyApprovalRead`: not-found, corrupt evidence, unreadable. This
// one returned the driver's raw error, so a caller deciding «retry or not» had
// nothing to decide on — and it is production code: the receipt verifier at
// internal/cli/receipt.go is its only non-test caller.
//
// Evidence level, honest: in-process, one real SQLite file, a real cancelled
// context. The read really is refused by the driver, not by a fake.
func TestApprovalByAction_aFailedReadCarriesItsClass(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	approval := boundPark(t, store, "act_class")

	// The CONTROL first: on a live context the door answers, so the refusal
	// below is the cancellation's and not a door that never worked.
	if _, _, err := store.GetApprovalByAction(context.Background(), approval.ActionID); err != nil {
		t.Fatalf("control: the door refuses a healthy read: %v", err)
	}

	dead, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := store.GetApprovalByAction(dead, approval.ActionID)
	if err == nil {
		t.Fatal("a cancelled read was answered")
	}
	if !errors.Is(err, ErrApprovalUnreadable) {
		t.Fatalf("the read failed as %v, which carries no class — every sibling "+
			"door answers %v for this", err, ErrApprovalUnreadable)
	}
	// And the class is the RIGHT one: a cancelled context is not corruption
	// and it is not an absent row.
	if errors.Is(err, ErrApprovalEvidenceCorrupt) || errors.Is(err, ErrApprovalNotFound) {
		t.Fatalf("a cancelled read was classified as corruption or absence: %v", err)
	}
}

// TestApprovalByAction_anAbsentRowKeepsItsOwnName is the CONTROL for the cure:
// the not-found arm must not be swept into the new classification, because
// `classifyApprovalRead` words its message around an APPROVAL id and this door
// only has an ACTION id when the lookup finds nothing.
func TestApprovalByAction_anAbsentRowKeepsItsOwnName(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	_, _, err := store.GetApprovalByAction(context.Background(), "act_never_parked")
	if !errors.Is(err, ErrApprovalNotFound) {
		t.Fatalf("an absent row answered %v, want %v", err, ErrApprovalNotFound)
	}
	if errors.Is(err, ErrApprovalUnreadable) {
		t.Fatalf("an absent row was also called unreadable: %v", err)
	}
}

// Ficha in `docs/HANDOFF.md` — the pending listing named a skipped row with an
// empty id.
//
// THE FAULT. `scanApprovalAndPreview`'s godoc promises the approval id «even
// when a later column fails to parse, because a row that cannot be served still
// has to be NAMED». Its three time arms kept that promise; the `row.Scan` arm
// threw the id away, so a wrongly-typed cell in any later column produced a
// counted-but-nameless skip. An operator saw «one row skipped» and had no id to
// go and look at.
//
// Evidence level, honest: in-process, one real SQLite file, the cell rewritten
// through the store's own helper — a real typed column holding text a real
// driver refuses to convert.
func TestListPendingApprovals_aSkippedRowIsAlwaysNamed(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	good := boundPark(t, store, "act_named_good")
	bad := boundPark(t, store, "act_named_bad")

	// The CONTROL: both rows list cleanly before the cell is broken.
	before, err := store.ListPendingApprovals(context.Background(), 10)
	if err != nil {
		t.Fatalf("control listing: %v", err)
	}
	if len(before.Rows) != 2 || len(before.Skipped) != 0 {
		t.Fatalf("control: %d rows, %d skipped, want 2 and 0", len(before.Rows), len(before.Skipped))
	}

	// policy_version is an INTEGER destination; text the driver cannot convert
	// makes row.Scan itself fail, which is the arm that used to lose the id.
	corruptCell(t, store, "approvals", "policy_version", "approval_id", bad.ApprovalID, "siete")

	after, err := store.ListPendingApprovals(context.Background(), 10)
	if err != nil {
		t.Fatalf("the whole listing fell over on one bad cell: %v", err)
	}
	if len(after.Rows) != 1 || after.Rows[0].Approval.ApprovalID != good.ApprovalID {
		t.Fatalf("the healthy row did not survive: %d rows", len(after.Rows))
	}
	if len(after.Skipped) != 1 {
		t.Fatalf("%d rows skipped, want exactly 1", len(after.Skipped))
	}
	if after.Skipped[0].ApprovalID != bad.ApprovalID {
		t.Fatalf("the skipped row is named %q, want %q — a row that cannot be "+
			"served still has to be NAMED, and an operator has nowhere to look "+
			"without the id", after.Skipped[0].ApprovalID, bad.ApprovalID)
	}
	if after.Skipped[0].Reason == "" {
		t.Fatal("the skipped row carries no reason")
	}
}

// Ficha «El error determinista dentro de la purga se publica "unreadable"».
//
// THE FAULT. The claim's purge is a WRITE, and every failure of it was
// published as ErrApprovalUnreadable — the package's single TRANSIENT class,
// «the store did not answer». A trigger's RAISE(ABORT) or a NOT NULL constraint
// is DETERMINISTIC: it will fail identically forever. A caller branching on
// «retry or not» was told to loop, and an operator was sent away from evidence
// that had been tampered with. The gap was declared in the v0.15.1 block A
// canto and shipped with no mould and no capture.
//
// Evidence level, honest: in-process, one real SQLite file, and the failure
// forced by a REAL trigger that aborts the purge's own UPDATE from a SECOND
// real connection — an oracle by impossibility, not a fake returning an error.
func TestClaim_aDeterministicPurgeFailureIsCorruptionNotWeather(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	approval := boundPark(t, store, "act_purge_class")

	envD, identD := operatorDecisionEnv("approve", approval.ApprovalID)
	if _, err := store.decideApproval(ctx, approval.ApprovalID, "approved",
		approval.RequestedAt.Add(time.Minute), envD, identD, ""); err != nil {
		t.Fatalf("approve: %v", err)
	}

	// The CONTROL, taken first: with nothing blocked the claim succeeds. Without
	// it the assertion below could pass because the claim never worked.
	control, _ := sealedStore(t)
	cApproval := boundPark(t, control, "act_purge_ok")
	cEnv, cIdent := operatorDecisionEnv("approve", cApproval.ApprovalID)
	if _, err := control.decideApproval(ctx, cApproval.ApprovalID, "approved",
		cApproval.RequestedAt.Add(time.Minute), cEnv, cIdent, ""); err != nil {
		t.Fatalf("control approve: %v", err)
	}
	if _, _, err := control.ClaimApprovalParamsUnderDigest(ctx, cApproval.ApprovalID,
		nil, cApproval.ActionDigest, nil); err != nil {
		t.Fatalf("control claim: %v", err)
	}

	hand := claimAttackConn(t, store)
	if _, err := hand.Exec(`CREATE TRIGGER purge_class_probe AFTER UPDATE OF canonical_params ON approvals
		BEGIN SELECT RAISE(ABORT, 'deterministic and permanent'); END`); err != nil {
		t.Fatalf("install the probe: %v", err)
	}

	params, _, err := store.ClaimApprovalParamsUnderDigest(ctx, approval.ApprovalID,
		nil, approval.ActionDigest, nil)
	if err == nil {
		t.Fatal("an aborted purge was answered as a successful claim")
	}
	if params != nil {
		t.Fatalf("a refused claim handed back parameters: %q", params)
	}
	if !errors.Is(err, ErrApprovalEvidenceCorrupt) {
		t.Fatalf("a deterministic purge failure was published as %v, want %v — "+
			"retrying a trigger's RAISE changes nothing, and calling it «the store "+
			"did not answer» sends the operator away from the evidence", err,
			ErrApprovalEvidenceCorrupt)
	}
	if errors.Is(err, ErrApprovalUnreadable) {
		t.Fatalf("it still carries the transient class too: %v", err)
	}
}

// TestClaim_anOperationalPurgeFailureStaysTransient is the CONTROL the first
// shape of this cure did not have, and it is the adversary's own reproduction.
//
// The ficha asks that a DETERMINISTIC purge failure stop being published as
// transient. It asks nothing about the rest, and the first classifier moved the
// rest anyway: a full disk and a read-only database became «la evidencia
// guardada ya no verifica · esto es permanente», over failures a `rm` or a
// remount repairs — and, because the corrupt class short-circuits `nameClaim`
// in the adapter, the cure also deleted the re-read that tells `params_held`
// from `params_gone`.
//
// Evidence level, honest: in-process, one real SQLite file, a REAL write
// refusal from the driver (`PRAGMA query_only`), not a fake returning an error.
// The store pins SetMaxOpenConns(1), so the pragma binds the claim's own
// connection.
func TestClaim_anOperationalPurgeFailureStaysTransient(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	approval := boundPark(t, store, "act_purge_readonly")
	envD, identD := operatorDecisionEnv("approve", approval.ApprovalID)
	if _, err := store.decideApproval(ctx, approval.ApprovalID, "approved",
		approval.RequestedAt.Add(time.Minute), envD, identD, ""); err != nil {
		t.Fatalf("approve: %v", err)
	}

	if _, err := store.db.ExecContext(ctx, `PRAGMA query_only = ON`); err != nil {
		t.Fatalf("make the connection read-only: %v", err)
	}

	_, _, err := store.ClaimApprovalParamsUnderDigest(ctx, approval.ApprovalID,
		nil, approval.ActionDigest, nil)
	if err == nil {
		t.Fatal("a claim against a read-only database reported success")
	}
	if !errors.Is(err, ErrApprovalUnreadable) {
		t.Fatalf("a read-only database was published as %v, want %v — a remount "+
			"repairs it, and calling it permanent evidence corruption sends the "+
			"operator after tampering that is not there", err, ErrApprovalUnreadable)
	}
	if errors.Is(err, ErrApprovalEvidenceCorrupt) {
		t.Fatalf("it carries the permanent class too: %v", err)
	}
}

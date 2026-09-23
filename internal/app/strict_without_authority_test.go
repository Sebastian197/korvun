// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/action/executor"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/tool"
)

// writeThroughASecondConnection runs one statement over a SEPARATE real
// connection to the same file, the way the fixture's reader already does. The
// store exposes no handle, and that is the honest shape anyway: the state this
// mould builds is one another writer could leave behind.
func writeThroughASecondConnection(t *testing.T, dbPath, statement string, args ...any) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(statement, args...); err != nil {
		t.Fatalf("%s: %v", statement, err)
	}
}

// TestStrictWithoutAuthority_theReaderRefusesTheIncoherentPair is the cure's
// mould, and it carries its own pre-cure reproduction.
//
// The state is NOT «the block is missing from the screen». It is an incoherent
// pair the store used to serve without complaint: an approval id carrying the
// strict prefix `apr3_` while its own `authority_snapshot_required` marker says
// 0. Of the twelve returns in `approvalAuthoritySnapshotTx`, exactly one left
// without an error — marker 0 with no snapshot row — so the detail was served,
// the screen painted a strict request as a plainly decidable document, and
// nothing anywhere compared the id against the marker.
//
// BEFORE the cure this same fixture served that detail with `Authority == nil`
// and `err == nil`, and the reject below was accepted — captured on
// 2026-09-22, which is what established that no screen state was needed: once
// the reader names the pair, no production door can serve it, and a state with
// no emitter does not enter a closed registry.
//
// Evidence level, honest: in-process, one real SQLite file, TWO real
// connections — the store's own and the separate writer that builds the state.
// The production park door, and the production read and decide doors. NOT the
// compiled binary and NOT the HTTP surface: `ApprovalsAdapter.Reject` adds only
// an operator envelope over the two store doors exercised here.
func TestStrictWithoutAuthority_theReaderRefusesTheIncoherentPair(t *testing.T) {
	f := newStrictDoorFixture(t)
	launch := &strictDoorTool{name: "launch"}
	exec := f.coordinator(tool.Registry{"launch": launch})
	ctx := context.Background()

	parked := f.submit(t, exec, "launch")
	if parked.Branch != executor.BranchPending || parked.ApprovalID == "" {
		t.Fatalf("park: branch=%q approval=%q err=%v", parked.Branch, parked.ApprovalID, parked.ApprovalError)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM approvals WHERE approval_id=? AND authority_snapshot_required=1`, parked.ApprovalID); n != 1 {
		t.Fatalf("the fixture did not park a strict row: marker rows = %d, want 1", n)
	}

	// The control, taken BEFORE the state is built: a coherent strict row does
	// serve its authority object. Without this, the assertion below could pass
	// because the door never serves one at all.
	before, err := f.store.ApprovalDetail(ctx, parked.ApprovalID)
	if err != nil {
		t.Fatalf("coherent strict detail: %v", err)
	}
	if before.Authority == nil {
		t.Fatalf("the coherent strict row served no authority object; the control is broken, not the door")
	}

	// Build the pair: the marker drops to 0 and the snapshot row goes with it,
	// which is the ONLY shape `approvalAuthoritySnapshotTx` answers (nil, nil)
	// for. The id keeps its `apr3_` prefix, untouched.
	writeThroughASecondConnection(t, f.dbPath,
		`UPDATE approvals SET authority_snapshot_required=0 WHERE approval_id=?`, parked.ApprovalID)
	writeThroughASecondConnection(t, f.dbPath,
		`DELETE FROM authorization_snapshots WHERE approval_id=?`, parked.ApprovalID)

	// 1 · The READ door now NAMES it. Before the cure this served the document.
	if _, err := f.store.ApprovalDetail(ctx, parked.ApprovalID); err == nil {
		t.Fatalf("the detail served a strict id against marker 0; the reader's pair check is gone")
	} else if !errors.Is(err, actionsqlite.ErrAuthorizationSnapshotCorrupt) {
		t.Fatalf("the pair was refused as %v, want %v — an incoherent pair is corrupt evidence, "+
			"not an unreadable store", err, actionsqlite.ErrAuthorizationSnapshotCorrupt)
	}

	// 2 · The MIRROR direction, which nobody had thought about: a non-strict id
	//     against marker 1. The pair is symmetric and so is its refusal.
	mirror := newStrictDoorFixture(t)
	mexec := mirror.coordinator(tool.Registry{"launch": &strictDoorTool{name: "launch"}})
	mparked := mirror.submit(t, mexec, "launch")
	if mparked.ApprovalID == "" {
		t.Fatalf("mirror park: %v", mparked.ApprovalError)
	}
	writeThroughASecondConnection(t, mirror.dbPath,
		`UPDATE approvals SET approval_id=? WHERE approval_id=?`,
		"apr_"+strings.TrimPrefix(mparked.ApprovalID, "apr3_"), mparked.ApprovalID)
	// The assert is HARD. An earlier shape logged a wrong class instead of
	// failing on it — an either/or that the doctrine forbids and that hid what
	// the adversary then found: the mirror direction is refused by guards that
	// PREDATE this cure (`approvals_v15.go`'s own snapshot-kind arms), so this
	// branch would have passed with the pair check gone. It is kept because the
	// guarantee is real and must not regress, and the canto no longer credits
	// this cure with it.
	_, err = mirror.store.ApprovalDetail(ctx, "apr_"+strings.TrimPrefix(mparked.ApprovalID, "apr3_"))
	if err == nil {
		t.Fatal("a non-strict id against marker 1 was served")
	}
	if !errors.Is(err, actionsqlite.ErrAuthorizationSnapshotCorrupt) {
		t.Fatalf("the mirror direction refused as %v, want %v", err,
			actionsqlite.ErrAuthorizationSnapshotCorrupt)
	}

	// 3 · The THIRD door. The pair is corrupt evidence wherever it is consumed,
	//     and `approve`/`reject` consume it too — so the decision is refused by
	//     the same sentinel the reader answers.
	//
	//     This assertion REPLACES an earlier one, and the earlier one was true
	//     of an earlier tree: on 2026-09-22, with the pair named at the reader
	//     ALONE, this same fixture had its reject ACCEPTED — `rule="" err=<nil>`,
	//     the row closed REJECTED with a sealed receipt, zero dispatches, zero
	//     debits. That capture is what established the screen needed no new
	//     state. The adversary then walked the other doors and found that
	//     `approve` and the plain claim still consumed the pair: a request born
	//     strict could have its parameters purged and handed back with no
	//     authority verification at all. The director ruled the refusal belongs
	//     at all THREE doors, so the reject's acceptance is gone with them.
	decisionEnv := action.NewEnvelope(action.NewID(), "cli",
		action.Source{Kind: "operator", Protocol: "cli", Channel: "cli"},
		action.Operation{Namespace: "approval", Name: "reject", Version: 1},
		`{"approval_id":"`+parked.ApprovalID+`"}`, time.Now().UTC())
	decisionEnv.Principal = action.PrincipalRef{PrincipalID: action.OperatorPrincipal().PrincipalID}
	decisionEnv.IntentID = action.RootIntentID

	_, err = f.store.DecideApprovalUnderLaw(ctx, parked.ApprovalID,
		action.DecisionRejected, time.Now().UTC(), decisionEnv,
		actionsqlite.AttemptIdentity{
			PrincipalID: action.OperatorPrincipal().PrincipalID,
			IntentID:    action.RootIntentID,
		}, "probe", f.law)
	if err == nil {
		t.Fatal("the decide door consumed a row whose id and marker disagree")
	}
	if !errors.Is(err, actionsqlite.ErrAuthorizationSnapshotCorrupt) {
		t.Fatalf("the decide door refused as %v, want %v — one question, the same "+
			"answer at every door that consumes the marker", err,
			actionsqlite.ErrAuthorizationSnapshotCorrupt)
	}

	// 4 · And the PLAIN CLAIM, the door that could purge and hand back the
	//     parameters of a request born strict. It is the reason the refusal
	//     could not live at the reader alone.
	if _, _, err := f.store.ClaimApprovalParamsUnderDigest(ctx, parked.ApprovalID,
		nil, parked.Action.ParametersDigest, nil); err == nil {
		t.Fatal("the plain claim purged and handed back a strict-born request's parameters")
	} else if !errors.Is(err, actionsqlite.ErrAuthorizationSnapshotCorrupt) {
		t.Fatalf("the plain claim refused as %v, want %v", err,
			actionsqlite.ErrAuthorizationSnapshotCorrupt)
	}

	if launch.calls.Load() != 0 {
		t.Fatalf("something dispatched the tool %d times, want 0", launch.calls.Load())
	}
	if n := f.count(t, `SELECT COUNT(*) FROM budget_debits WHERE action_id=?`, parked.Action.ActionID); n != 0 {
		t.Fatalf("a refused row spent %d debits, want 0", n)
	}
}

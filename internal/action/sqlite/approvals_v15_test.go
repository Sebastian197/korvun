// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.15.0 — the store gates the approvals SCREEN needs, and the three
// tree defects the pre-test paper found. RED: none of the symbols below
// exists yet.
//
// Every test here is an ATTACK. The guarantee, the scenario that would
// make it false, and the EXACT named outcome are in each test's doc; the
// probing mutation that must redden it is named beside the assertion, in
// the form the canto will declare.
//
// Evidence level, honest and uniform for this file: a REAL SQLite store,
// one handle unless the test opens a second one and says so. Nothing here
// is in-process against a fake.
//
// Anchors: docs/superpowers/specs/2026-09-08-approvals-screen-ux.md
// §11 (FR-API-1, 19, 21, 22, 25), §12 (AS-96..AS-121), §13-bis, §17 F
// rows 16-21; and docs/superpowers/specs/2026-09-12-approvals-adapter-
// pre-test-review.md §0-sexies (the director's adjudications).

package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// ---------------------------------------------------------------------------
// Cure 19 · the LIST gate: it brings the preview WITHOUT the belt, and it
// serves every row it can serve.
// ---------------------------------------------------------------------------

// TestListPendingApprovals_bringsTheRowWithoutRunningTheBelt pins FR-API-1:
// the list needs operation, effect class and CHANNEL, and it must NOT run the
// verification belt — a row whose story no longer verifies still comes out,
// marked, and it is the DETAIL that refuses it.
//
// Attack: corrupt the story (the actions row's effect_class) so the belt
// WOULD refuse, then list. The row must still be served.
// Probing mutation: run verifyApprovalStory inside the list ⇒ the row
// disappears and this reddens.
func TestListPendingApprovals_bringsTheRowWithoutRunningTheBelt(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	a := boundPark(t, store, "act_list_1")
	corruptCell(t, store, "actions", "effect_class", "action_id", "act_list_1", string(action.EffectPure))

	got, err := store.ListPendingApprovals(context.Background(), 200)
	if err != nil {
		t.Fatalf("the list must not refuse a row whose belt would: %v", err)
	}
	if len(got.Rows) != 1 {
		t.Fatalf("rows = %d, want exactly 1 — the list does not run the belt", len(got.Rows))
	}
	row := got.Rows[0]
	if row.Approval.ApprovalID != a.ApprovalID {
		t.Fatalf("approval_id = %q, want %q", row.Approval.ApprovalID, a.ApprovalID)
	}
	if row.Preview.Operation != "tool/echo" {
		t.Fatalf("operation = %q, want %q — FR-API-1 needs it in the row", row.Preview.Operation, "tool/echo")
	}
	if !row.PreviewReadable {
		t.Fatalf("PreviewReadable = false over a preview that parses fine")
	}
	if row.Channel != "console" {
		t.Fatalf("channel = %q, want %q — origin comes from the SEALED preview", row.Channel, "console")
	}
}

// TestListPendingApprovals_anUnreadablePreviewKeepsItsRow pins the other half
// of FR-API-1: a row whose preview cannot be PARSED does not vanish — it comes
// out with its identifier and its digest, flagged, so the screen can paint
// «SIN CLASE LEGIBLE».
//
// Probing mutation: drop the row when the preview fails to parse ⇒ this reddens.
func TestListPendingApprovals_anUnreadablePreviewKeepsItsRow(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	a := boundPark(t, store, "act_list_2")
	corruptCell(t, store, "approvals", "canonical_preview", "approval_id", a.ApprovalID, "{not json")

	got, err := store.ListPendingApprovals(context.Background(), 200)
	if err != nil {
		t.Fatalf("an unreadable preview must not fail the list: %v", err)
	}
	if len(got.Rows) != 1 {
		t.Fatalf("rows = %d, want 1 — the row does not disappear", len(got.Rows))
	}
	if got.Rows[0].PreviewReadable {
		t.Fatalf("PreviewReadable = true over a preview that does not parse")
	}
	if got.Rows[0].Approval.ActionDigest != a.ActionDigest {
		t.Fatalf("the digest must survive an unreadable preview: got %q", got.Rows[0].Approval.ActionDigest)
	}
	if got.Rows[0].Channel != "" {
		t.Fatalf("channel = %q, want empty — nothing may be presumed from a preview that did not parse", got.Rows[0].Channel)
	}
}

// TestListPendingApprovals_anEmptyResourcesSetIsNotAChannel pins HUECO 3 B:
// the list reads the preview WITHOUT the belt, so Resources carries whatever
// an external hand wrote. A mutated preview with "resources": [] must not
// panic, must not take the list down, and must not be confused with a row
// whose channel is legitimately empty.
//
// Probing mutation: index Resources[0] without checking the length ⇒ panic,
// and this reddens.
func TestListPendingApprovals_anEmptyResourcesSetIsNotAChannel(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	a := boundPark(t, store, "act_list_3")
	stripResources(t, store, a.ApprovalID)

	got, err := store.ListPendingApprovals(context.Background(), 200)
	if err != nil {
		t.Fatalf("a preview with no resources must not fail the list: %v", err)
	}
	if len(got.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(got.Rows))
	}
	if got.Rows[0].Channel != "" {
		t.Fatalf("channel = %q, want empty", got.Rows[0].Channel)
	}
	if got.Rows[0].ChannelKnown {
		t.Fatalf("ChannelKnown = true over a preview whose resources set is empty — corrupt is not the same as empty")
	}
}

// TestListPendingApprovals_aLegitimatelyEmptyChannelIsKnown is the twin of the
// test above and the reason ChannelKnown exists: an envelope born with no
// channel yields an EMPTY channel that the store DID read. Empty is not absent.
//
// Probing mutation: collapse the two into one «empty channel» ⇒ this reddens.
func TestListPendingApprovals_aLegitimatelyEmptyChannelIsKnown(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	boundParkOnChannel(t, store, "act_list_4", "")

	got, err := store.ListPendingApprovals(context.Background(), 200)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(got.Rows))
	}
	if got.Rows[0].Channel != "" {
		t.Fatalf("channel = %q, want empty", got.Rows[0].Channel)
	}
	if !got.Rows[0].ChannelKnown {
		t.Fatalf("ChannelKnown = false over a channel the store read and that is empty")
	}
}

// TestListPendingApprovals_isBoundedAtTheLimit pins the page bound: exactly
// the limit, never more.
//
// Probing mutation: serve limit+1 ⇒ this reddens.
func TestListPendingApprovals_isBoundedAtTheLimit(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	for i := 0; i < 5; i++ {
		boundPark(t, store, "act_bound_"+string(rune('a'+i)))
	}
	got, err := store.ListPendingApprovals(context.Background(), 3)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got.Rows) != 3 {
		t.Fatalf("rows = %d, want exactly 3 — the bound is equality, not «at most»", len(got.Rows))
	}
}

// ---------------------------------------------------------------------------
// V12 · the director's ruling: a corrupt row does NOT take the list down.
// It is skipped, COUNTED and NAMED.
// ---------------------------------------------------------------------------

// TestListPendingApprovals_aCorruptTimeDoesNotTakeTheListDown is the mould for
// V12. `ListApprovals` today does `return nil, err` on the first row whose
// requested_at will not parse, so ONE mutated column erases the healthy rows
// too and the operator silently loses parked requests.
//
// The director's ruling, literal: «se salta, se cuenta y se nombra». A single
// outcome, no either/or: the healthy rows come out AND the corrupt one is
// reported by name.
//
// Probing mutation: `return nil, err` on the scan — what the tree does today
// ⇒ this reddens.
func TestListPendingApprovals_aCorruptTimeDoesNotTakeTheListDown(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	boundPark(t, store, "act_v12_a")
	bad := boundPark(t, store, "act_v12_b")
	boundPark(t, store, "act_v12_c")
	corruptCell(t, store, "approvals", "requested_at", "approval_id", bad.ApprovalID, "ayer")

	got, err := store.ListPendingApprovals(context.Background(), 200)
	if err != nil {
		t.Fatalf("one corrupt row must never fail the whole list: %v", err)
	}
	if len(got.Rows) != 2 {
		t.Fatalf("rows = %d, want 2 — the healthy rows survive a corrupt neighbour", len(got.Rows))
	}
	if len(got.Skipped) != 1 {
		t.Fatalf("skipped = %d, want exactly 1 — the corrupt row is COUNTED", len(got.Skipped))
	}
	if got.Skipped[0].ApprovalID != bad.ApprovalID {
		t.Fatalf("skipped id = %q, want %q — the corrupt row is NAMED", got.Skipped[0].ApprovalID, bad.ApprovalID)
	}
	if got.Skipped[0].Reason == "" {
		t.Fatalf("the skipped row carries no reason — «se nombra» means it says why")
	}
	for _, r := range got.Rows {
		if r.Approval.ApprovalID == bad.ApprovalID {
			t.Fatalf("the corrupt row was served as if it were whole")
		}
	}
}

// ---------------------------------------------------------------------------
// Cure 20 · the DETAIL gate: state, terna, params and belts in ONE transaction.
// ---------------------------------------------------------------------------

// TestApprovalDetail_bringsTheTernaAndTheParamsTogether pins FR-API-19's
// precondition: re-deriving the digest needs the TERNA (namespace, name,
// VERSION) and the params from the SAME row in the SAME query. The canonical
// preview only stores "ns/name", without the version, so with the preview the
// re-derivation is impossible.
//
// Probing mutation: take the terna from the preview ⇒ Version is zero and this
// reddens.
func TestApprovalDetail_bringsTheTernaAndTheParamsTogether(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	a := boundPark(t, store, "act_detail_1")

	d, err := store.ApprovalDetail(context.Background(), a.ApprovalID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if d.Operation.Namespace != "tool" || d.Operation.Name != "echo" {
		t.Fatalf("operation = %+v, want tool/echo from the ACTIONS row", d.Operation)
	}
	if d.Operation.Version != 1 {
		t.Fatalf("op_version = %d, want 1 — the preview does not carry it and the digest needs it", d.Operation.Version)
	}
	if string(d.Params) != `{"a":1}` {
		t.Fatalf("params = %q, want the stored canonical params", string(d.Params))
	}
	if got := action.Digest(d.Operation, string(d.Params)); got != a.ActionDigest {
		t.Fatalf("the terna and the params must re-derive the sealed digest: got %s want %s", got, a.ActionDigest)
	}
}

// TestApprovalDetail_refusesAMutatedOpVersion is HUECO 2: op_version enters
// action.Digest and NO belt compares it — verifyApprovalStory compares
// ns+"/"+name without the version. So its corruption has to surface as the
// re-derivation refusing.
//
// Probing mutation, the one the fifth pass finally certified: read ns and name
// from the actions row but PIN the version to the production constant ⇒ the
// attacked row re-derives again, the mismatch vanishes, and this reddens.
func TestApprovalDetail_refusesAMutatedOpVersion(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	a := boundPark(t, store, "act_detail_2")
	corruptCell(t, store, "actions", "op_version", "action_id", "act_detail_2", "2")

	_, err := store.ApprovalDetail(context.Background(), a.ApprovalID)
	if !errors.Is(err, ErrApprovalParamsDigestMismatch) {
		t.Fatalf("err = %v, want ErrApprovalParamsDigestMismatch — op_version is in the digest", err)
	}
}

// TestApprovalDetail_anAbsentActionRowIsCorruptionNotAbsence pins AS-96 and the
// lesson R15 already taught: a LEFT JOIN distinguishes the missing row, and an
// approvals row whose actions row is gone is PERMANENT corruption — never
// not_found, never «retry».
//
// Probing mutation: an INNER join ⇒ the orphan row becomes «does not exist»,
// the fail-open, and this reddens.
func TestApprovalDetail_anAbsentActionRowIsCorruptionNotAbsence(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	a := boundPark(t, store, "act_detail_3")
	deleteActionRow(t, store, "act_detail_3")

	_, err := store.ApprovalDetail(context.Background(), a.ApprovalID)
	if !errors.Is(err, ErrApprovalEvidenceCorrupt) {
		t.Fatalf("err = %v, want ErrApprovalEvidenceCorrupt", err)
	}
	if errors.Is(err, ErrApprovalNotFound) {
		t.Fatalf("an orphan row must never read as absent")
	}
}

// TestApprovalDetail_separatesTheMissingRowFromTheDriverFailure is cure 21's
// residual: three sites collapse sql.ErrNoRows with a driver error today, so a
// DESTROYED row and an unreadable disk leave by the same return. Without the
// separation, «un fallo de driver, y solo él» cannot be honoured.
//
// Probing mutation: wrap sql.ErrNoRows like any other error ⇒ this reddens.
func TestApprovalDetail_separatesTheMissingRowFromTheDriverFailure(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)

	_, err := store.ApprovalDetail(context.Background(), "apr_nope")
	if !errors.Is(err, ErrApprovalNotFound) {
		t.Fatalf("err = %v, want ErrApprovalNotFound for a row that is not there", err)
	}
	if errors.Is(err, ErrApprovalUnreadable) {
		t.Fatalf("an absent row must never read as a driver failure")
	}
}

// TestApprovalDetail_namesEachBeltThatRefuses is the other half of cure 21: the
// belts refuse with a PLAIN fmt.Errorf today, so invalidated and
// evidence_corrupt can only be told apart by strings.Contains — which
// FR-API-15 calls a finding, not an implementation.
//
// Probing mutation: return a plain error from any of these branches ⇒ the
// errors.Is fails and this reddens.
func TestApprovalDetail_namesEachBeltThatRefuses(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		corrupt func(t *testing.T, s *Store, approvalID, actionID string)
		want    error
	}{
		{"story: effect class", func(t *testing.T, s *Store, _, id string) {
			corruptCell(t, s, "actions", "effect_class", "action_id", id, string(action.EffectPure))
		}, ErrApprovalEvidenceCorrupt},
		{"story: operation", func(t *testing.T, s *Store, _, id string) {
			corruptCell(t, s, "actions", "op_name", "action_id", id, "calc")
		}, ErrApprovalEvidenceCorrupt},
		{"story: decision policy", func(t *testing.T, s *Store, _, id string) {
			corruptCell(t, s, "action_decisions", "policy_digest", "action_id", id, "sha256:another")
		}, ErrApprovalEvidenceCorrupt},
		{"preview unparseable", func(t *testing.T, s *Store, ap, _ string) {
			corruptCell(t, s, "approvals", "canonical_preview", "approval_id", ap, "{not json")
		}, ErrApprovalEvidenceCorrupt},
		{"preview binding", func(t *testing.T, s *Store, ap, _ string) {
			corruptCell(t, s, "approvals", "preview_digest", "approval_id", ap, "sha256:not-the-preview")
		}, ErrApprovalEvidenceCorrupt},
		{"requested_at unparseable", func(t *testing.T, s *Store, ap, _ string) {
			corruptCell(t, s, "approvals", "requested_at", "approval_id", ap, "ayer")
		}, ErrApprovalEvidenceCorrupt},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, _ := openTemp(t)
			a := boundPark(t, store, "act_belt")
			tc.corrupt(t, store, a.ApprovalID, "act_belt")
			_, err := store.ApprovalDetail(context.Background(), a.ApprovalID)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v — the belt must refuse BY TYPE, not by text", err, tc.want)
			}
		})
	}
}

// TestApprovalDetail_theLawThatMovedHasItsOwnSentinel keeps invalidated apart
// from evidence_corrupt: they are different literals on the screen and one of
// them is repairable from the profile.
//
// Probing mutation: fold the law refusal into ErrApprovalEvidenceCorrupt ⇒
// this reddens.
func TestApprovalDetail_theLawThatMovedHasItsOwnSentinel(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	a := boundPark(t, store, "act_law_1")

	_, err := store.ApprovalDetailUnderLaw(context.Background(), a.ApprovalID,
		PolicyPin{Version: 8, Digest: "sha256:another-law"})
	if !errors.Is(err, ErrApprovalInvalidated) {
		t.Fatalf("err = %v, want ErrApprovalInvalidated", err)
	}
	if errors.Is(err, ErrApprovalEvidenceCorrupt) {
		t.Fatalf("a moved law is not corrupt evidence — the two have different literals and different cures")
	}
}

// TestApprovalDetail_classifiesTheParamsState pins FR-UI-16's four values, and
// that the digest belt runs BEFORE the classification: a mismatch precedes
// «empty» and «too_large».
//
// Probing mutation: classify before re-deriving ⇒ a mutated-params row would
// come back as «present» and this reddens.
func TestApprovalDetail_classifiesTheParamsState(t *testing.T) {
	t.Parallel()
	t.Run("present", func(t *testing.T) {
		t.Parallel()
		store, _ := openTemp(t)
		a := boundPark(t, store, "act_ps_1")
		d, err := store.ApprovalDetail(context.Background(), a.ApprovalID)
		if err != nil {
			t.Fatalf("detail: %v", err)
		}
		if d.ParamsState != ParamsPresent {
			t.Fatalf("state = %q, want %q", d.ParamsState, ParamsPresent)
		}
	})
	t.Run("empty: born without arguments", func(t *testing.T) {
		t.Parallel()
		store, _ := openTemp(t)
		a := boundParkWithParams(t, store, "act_ps_2", "")
		d, err := store.ApprovalDetail(context.Background(), a.ApprovalID)
		if err != nil {
			t.Fatalf("detail: %v", err)
		}
		if d.ParamsState != ParamsEmpty {
			t.Fatalf("state = %q, want %q — a row born without arguments is empty, not purged", d.ParamsState, ParamsEmpty)
		}
	})
}

// ---------------------------------------------------------------------------
// Cures 16 and 17 · the claim tells its three refusals apart, and stops
// discarding the RowsAffected error.
// ---------------------------------------------------------------------------

// TestClaimApprovalParams_tellsItsThreeRefusalsApart is cure 16. Today the
// missing row, the empty column and RowsAffected()==0 all wrap ErrNotFound, so
// params_unaccounted, not_started_params_gone and «already claimed» are
// indistinguishable to the caller — and the adapter would name by a sentinel
// that lies.
//
// Probing mutation: return the same ErrNotFound for the three ⇒ this reddens.
func TestClaimApprovalParams_tellsItsThreeRefusalsApart(t *testing.T) {
	t.Parallel()

	t.Run("the row is not there", func(t *testing.T) {
		t.Parallel()
		store, _ := openTemp(t)
		_, _, err := store.ClaimApprovalParamsUnderDigest(context.Background(), "apr_nope", nil, "sha256:x", nil)
		if !errors.Is(err, ErrApprovalNotFound) {
			t.Fatalf("err = %v, want ErrApprovalNotFound", err)
		}
	})

	t.Run("the row is there and the column is empty", func(t *testing.T) {
		t.Parallel()
		store, _ := openTemp(t)
		a := boundPark(t, store, "act_claim_1")
		corruptCell(t, store, "approvals", "canonical_params", "approval_id", a.ApprovalID, "")
		_, _, err := store.ClaimApprovalParamsUnderDigest(context.Background(), a.ApprovalID, nil, a.ActionDigest, nil)
		if !errors.Is(err, ErrApprovalParamsEmpty) {
			t.Fatalf("err = %v, want ErrApprovalParamsEmpty — an empty column is not an absent row", err)
		}
		if errors.Is(err, ErrApprovalNotFound) {
			t.Fatalf("the empty column must not read as a missing row")
		}
	})
}

// TestApprovalParams_tellsTheMissingRowFromTheEmptyColumn is cure 16's other
// half: the plain read has the same collapse.
//
// Probing mutation: one ErrNotFound for both ⇒ this reddens.
func TestApprovalParams_tellsTheMissingRowFromTheEmptyColumn(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	a := boundPark(t, store, "act_claim_2")
	corruptCell(t, store, "approvals", "canonical_params", "approval_id", a.ApprovalID, "")

	_, err := store.ApprovalParams(context.Background(), a.ApprovalID)
	if !errors.Is(err, ErrApprovalParamsEmpty) {
		t.Fatalf("err = %v, want ErrApprovalParamsEmpty", err)
	}
	_, err = store.ApprovalParams(context.Background(), "apr_nope")
	if !errors.Is(err, ErrApprovalNotFound) {
		t.Fatalf("err = %v, want ErrApprovalNotFound", err)
	}
}

// TestClaimApprovalParams_propagatesTheRowsAffectedError is cure 17. The tree
// writes `if n, _ := res.RowsAffected(); n == 0`, so a driver that fails to
// count reads as «zero rows» and produces an outcome name over a count that
// was never obtained.
//
// The forcing is the cheap one the spec offers: a BEFORE UPDATE trigger with
// RAISE(IGNORE) makes the row skip and changes() be 0 with NO concurrency and
// NO driver error — and its outcome, by AS-104, is not_started_params_held,
// because the deferred rollback leaves the bytes where they are.
//
// Probing mutation: discard the error with `n, _ :=`, or name it
// params_unaccounted over a row that keeps its params ⇒ this reddens.
//
// ELEVATED 2026-09-15: the row is APPROVED before the claim. On a PENDING row
// the claim now refuses by authority, which is P1-1's cure; the zero-row name
// this mould pins only exists for a request that may be claimed at all.
// Probing mutation (executed, red, declared in the canto): name the zero-row
// purge ErrApprovalNoLongerApproved over an approved row ⇒ this reddens.
func TestClaimApprovalParams_propagatesTheRowsAffectedError(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	a := boundPark(t, store, "act_claim_3")
	envD, identD := operatorDecisionEnv("approve", a.ApprovalID)
	if _, err := store.decideApproval(context.Background(), a.ApprovalID, "approved",
		a.RequestedAt.Add(time.Minute), envD, identD, ""); err != nil {
		t.Fatalf("approve: %v", err)
	}
	ignoreUpdatesOn(t, store, "approvals")

	_, _, err := store.ClaimApprovalParamsUnderDigest(context.Background(), a.ApprovalID, nil, a.ActionDigest, nil)
	if !errors.Is(err, ErrApprovalClaimSkipped) {
		t.Fatalf("err = %v, want ErrApprovalClaimSkipped — zero rows is not «already claimed»", err)
	}
	// The oracle by impossibility: the params are still there, so no literal
	// may say they were taken.
	got, err := store.ApprovalParams(context.Background(), a.ApprovalID)
	if err != nil {
		t.Fatalf("re-read after a skipped claim: %v", err)
	}
	if string(got) != `{"a":1}` {
		t.Fatalf("params = %q — a skipped claim must leave the bytes untouched", string(got))
	}
}

// ---------------------------------------------------------------------------
// V10 · the director's ruling: the digest is compared against the row READ
// INSIDE the claim's transaction, never against an earlier read.
// ---------------------------------------------------------------------------

// TestClaimApprovalParamsUnderDigest_judgesTheTernaItReadItself is the mould
// for V10. Today ExecuteApprovedAction reads the terna with store.Get, then the
// claim COMMITS (the approval is consumed and the column emptied), and only
// THEN compares the digest using the terna from that earlier read. An external
// UPDATE of op_version in that window passes every belt and executes an
// irreversible effect under an operation the row no longer declares.
//
// The ruling: the comparison happens inside the claiming transaction, over the
// row that transaction read. A mismatch refuses BY NAME and nothing is consumed.
//
// Probing mutation: compare against a terna read before the transaction ⇒ the
// attacked row passes and this reddens.
func TestClaimApprovalParamsUnderDigest_judgesTheTernaItReadItself(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	a := boundPark(t, store, "act_toctou")
	corruptCell(t, store, "actions", "op_version", "action_id", "act_toctou", "2")

	_, _, err := store.ClaimApprovalParamsUnderDigest(context.Background(), a.ApprovalID, nil, a.ActionDigest, nil)
	if !errors.Is(err, ErrApprovalParamsDigestMismatch) {
		t.Fatalf("err = %v, want ErrApprovalParamsDigestMismatch", err)
	}
	// Oracle by impossibility: nothing was consumed.
	got, err := store.ApprovalParams(context.Background(), a.ApprovalID)
	if err != nil {
		t.Fatalf("re-read after a refused claim: %v", err)
	}
	if string(got) != `{"a":1}` {
		t.Fatalf("params = %q — a refused claim consumes nothing", string(got))
	}
}

// ---------------------------------------------------------------------------
// V13 · the director's ruling: the transitionTx refusal gets a NAME, and the
// name says «the action was no longer pending», never «we do not know whether
// the effect happened».
// ---------------------------------------------------------------------------

// TestDecideApprovalUnderLaw_namesTheActionThatWasNoLongerPending is the mould
// for V13. transitionTx refuses with a plain fmt.Errorf when its UPDATE affects
// zero rows, and verifyApprovalStory does not read `state`, so nothing catches
// it earlier. The whole transaction rolls back: nothing was decided and
// exec.Run never ran.
//
// The director, literal: «mentir por exceso de cautela sigue siendo mentir».
//
// Probing mutation: let it fall to the unnamed 500, which is what the tree does
// today ⇒ this reddens.
func TestDecideApprovalUnderLaw_namesTheActionThatWasNoLongerPending(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	a := boundPark(t, store, "act_v13")
	setActionState(t, store, "act_v13", action.StateSucceeded)

	_, err := store.DecideApprovalUnderLaw(context.Background(), a.ApprovalID,
		action.DecisionApproved, time.Now().UTC(), testEnvelope("act_v13_op"),
		AttemptIdentity{PrincipalID: "principal_operator"}, "",
		PolicyPin{Version: 7, Digest: "sha256:law"})
	if !errors.Is(err, ErrApprovalActionNotPending) {
		t.Fatalf("err = %v, want ErrApprovalActionNotPending", err)
	}
	// Oracle by impossibility: the transaction rolled back whole.
	after, _, err := store.GetApproval(context.Background(), a.ApprovalID)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if after.Status != action.ApprovalPending {
		t.Fatalf("status = %q, want PENDING — the refusal rolls the whole transaction back", after.Status)
	}
}

// ---------------------------------------------------------------------------
// The attackers' tools. They write through the store's own handle, which is
// what a second process would do to the same file; where a test needs a real
// SECOND connection it opens one and says so in its doc.
// ---------------------------------------------------------------------------

// boundParkOnChannel parks through the real door on a named channel, so the
// tests can tell an empty channel from an absent one.
func boundParkOnChannel(t *testing.T, store *Store, id, channel string) action.Approval {
	t.Helper()
	return parkWith(t, store, id, channel, `{"a":1}`)
}

// boundParkWithParams parks through the real door with the given raw params —
// "" is the row BORN without arguments, which is `empty` and not `purged`.
func boundParkWithParams(t *testing.T, store *Store, id, rawParams string) action.Approval {
	t.Helper()
	return parkWith(t, store, id, "console", rawParams)
}

func parkWith(t *testing.T, store *Store, id, channel, rawParams string) action.Approval {
	t.Helper()
	env := action.NewEnvelope(
		id, "env-1",
		action.Source{Kind: "agent_brain", Protocol: "text", Channel: channel},
		action.Operation{Namespace: "tool", Name: "echo", Version: 1},
		rawParams,
		time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC),
	)
	env.IntentID = action.RootIntentID
	env.Principal = action.PrincipalRef{PrincipalID: "principal_brain_a"}
	env.Effect = action.Effect{Class: string(action.EffectWriteIrreversible)}
	b, err := action.NewBoundApprovalRequest(env, rawParams, action.ApprovalContext{
		IntentPurpose: "semana de pruebas",
		GrantID:       "grant_1", GrantDepth: 1, CostLine: "1 of 5",
		ToolCage: "echo cage",
		Descriptor: action.EffectDescriptor{
			Class: action.EffectWriteIrreversible, DataEgress: true,
		},
		HasDescriptor: true,
		LawVersion:    7, LawDigest: "sha256:law",
		Rule: "require_approval",
		Now:  time.Now().UTC(), TTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if err := store.CreateApprovalRequest(context.Background(), b); err != nil {
		t.Fatalf("park: %v", err)
	}
	return b.Approval()
}

// stripResources rewrites the stored preview with an EMPTY resources set — the
// shape an external hand can write and that the list reads without a belt.
func stripResources(t *testing.T, store *Store, approvalID string) {
	t.Helper()
	var raw string
	if err := store.db.QueryRow(
		`SELECT canonical_preview FROM approvals WHERE approval_id = ?`, approvalID).Scan(&raw); err != nil {
		t.Fatalf("read preview: %v", err)
	}
	var w map[string]any
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	w["resources"] = []string{}
	out, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("encode preview: %v", err)
	}
	if _, err := store.db.Exec(
		`UPDATE approvals SET canonical_preview = ? WHERE approval_id = ?`, string(out), approvalID); err != nil {
		t.Fatalf("write preview: %v", err)
	}
}

// deleteActionRow removes the actions row while keeping its approval — the
// orphan a LEFT JOIN distinguishes and an INNER one would turn into «absent».
// Foreign keys go off on this connection so the cascade does not take the
// approval with it: that is the external hand FR-API-19 assumes.
func deleteActionRow(t *testing.T, store *Store, actionID string) {
	t.Helper()
	if _, err := store.db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatalf("foreign_keys off: %v", err)
	}
	if _, err := store.db.Exec(`DELETE FROM actions WHERE action_id = ?`, actionID); err != nil {
		t.Fatalf("delete action: %v", err)
	}
}

// ignoreUpdatesOn installs a BEFORE UPDATE trigger that RAISEs IGNORE, so the
// row is skipped and changes() is 0 with NO concurrency and NO driver error.
// It is the cheap, deterministic way to force the zero-rows branch (§11
// FR-API-21).
func ignoreUpdatesOn(t *testing.T, store *Store, table string) {
	t.Helper()
	stmt := `CREATE TRIGGER skip_` + table + ` BEFORE UPDATE ON ` + table +
		` BEGIN SELECT RAISE(IGNORE); END` // #nosec G202 -- test-owned literal
	if _, err := store.db.Exec(stmt); err != nil {
		t.Fatalf("install trigger: %v", err)
	}
}

// setActionState moves the actions row out from under the decide, which is the
// V13 attack: nothing else in the decide reads `state`.
func setActionState(t *testing.T, store *Store, actionID string, to action.State) {
	t.Helper()
	if _, err := store.db.Exec(
		`UPDATE actions SET state = ? WHERE action_id = ?`, string(to), actionID); err != nil {
		t.Fatalf("set state: %v", err)
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
)

// Train E, batch 4 (GE2, GE3) — TE46 (plan v3, §6 and §7, O5): the adoption
// and the recovery, through their public entries, meet ONE failed read of
// their judgement. A structural code is ErrLedgerUnreadable and nothing
// follows: no adoption act, no receipt, no identity row; no recovery of an
// orphan that was eligible. BUSY and IOERR are named and poison nothing: the
// same handle, asked again, adopts or recovers.
//
// What batch 1's grid (TE20–TE26) already holds for these two origins is the
// class, the guard and the retry; this mould adds the effects the grid does
// not count — the adoption's act and receipt, and a real orphan the recovery
// must leave alone.
//
// PROBING MUTATIONS (MU46): the consumers skip their judgement's
// classification → reddens; an operational failure treated as a verdict →
// the retry is refused → reddens.
//
// Evidence level: in process, native connections on a real file, a synthetic
// coded failure put on the result of the read that really ran.

// e4Counts is the rows of the adoption's and the recovery's effects.
func e4Counts(t *testing.T, path string) string {
	t.Helper()
	raw := e2Raw(t, path)
	return fmt.Sprintf("actions %d · receipts %d · identity %d", rawCount(t, raw, `SELECT COUNT(*) FROM actions`),
		rawCount(t, raw, `SELECT COUNT(*) FROM receipts`), rawCount(t, raw, `SELECT COUNT(*) FROM ledger_identity WHERE owner_digest = '`+profileB+`'`))
}

func TestE4_TE46_aFailedJudgementLeavesTheAdoptionAndTheRecoveryUndone(t *testing.T) {
	fx := newE1Fixture(t)
	kinds := []struct {
		name  string
		fault func() error
		want  error
	}{
		{"code11", func() error { return &codedFault{code: 11, tag: "TE46"} }, ErrLedgerUnreadable},
		{"code5", func() error { return &codedFault{code: 5, tag: "TE46"} }, ErrLedgerBusy},
		{"code10", func() error { return &codedFault{code: 10, tag: "TE46"} }, ErrLedgerEnvironment},
	}
	for _, k := range kinds {
		t.Run("adoption/"+k.name, func(t *testing.T) {
			ctx := context.Background()
			path := ledgerCopy(t, fx.founded)
			b, err := OpenOperatorFor(path, profileB)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = b.Close() }()
			wireSealedLike(t, b, fx.foundedInk)
			before := e4Counts(t, path)
			env, evidence := fx.evidenceFor("founded", "te46_adopt")
			fault := k.fault()
			f := armJudgeFault(t, path, originBeginAdoption, siteOwnerDB, stageQuery, fault)
			_, err = b.AdoptLedger(ctx, adoptionEnv(env), Decision{Outcome: "allow", Rule: "operator"}, evidence, profileB)
			f.disarm()
			if !errors.Is(err, k.want) || !errors.Is(err, fault) {
				t.Fatalf("the adoption = %v, want %v carrying the fault", err, k.want)
			}
			if after := e4Counts(t, path); after != before {
				t.Fatalf("the refused adoption left effects: %s → %s", before, after)
			}
			if k.want == ErrLedgerUnreadable {
				return
			}
			env2, evidence2 := fx.evidenceFor("founded", "te46_adopt_retry")
			if _, err := b.AdoptLedger(ctx, adoptionEnv(env2), Decision{Outcome: "allow", Rule: "operator"}, evidence2, profileB); err != nil {
				t.Fatalf("the same handle's retried adoption = %v, want it recorded", err)
			}
			if after := e4Counts(t, path); after == before {
				t.Fatal("the retried adoption left no act, receipt or row")
			}
		})
		t.Run("recovery/"+k.name, func(t *testing.T) {
			ctx := context.Background()
			path := ledgerCopy(t, fx.founded)
			life, err := OpenFor(path, profileA)
			if err != nil {
				t.Fatal(err)
			}
			wireSealedLike(t, life, fx.foundedInk)
			mustRecord(t, life, "te46_orphan", action.StateAuthorized)
			_ = life.Close() // the previous life ends with the act in flight
			h, err := OpenFor(path, profileA)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = h.Close() }()
			wireSealedLike(t, h, fx.foundedInk)
			before := e4Counts(t, path)
			fault := k.fault()
			f := armJudgeFault(t, path, originRefuseMaintenance, siteOwnerDB, stageQuery, fault)
			_, err = h.RecoverPreviousLife(ctx)
			f.disarm()
			if !errors.Is(err, k.want) || !errors.Is(err, fault) {
				t.Fatalf("the recovery = %v, want %v carrying the fault", err, k.want)
			}
			if rec, gerr := h.Get(ctx, "te46_orphan"); gerr != nil || rec.State != action.StateAuthorized {
				t.Fatalf("the orphan after the refused recovery = %v %v, want it still AUTHORIZED", rec.State, gerr)
			}
			if after := e4Counts(t, path); after != before {
				t.Fatalf("the refused recovery left effects: %s → %s", before, after)
			}
			if k.want == ErrLedgerUnreadable {
				return
			}
			if _, err := h.RecoverPreviousLife(ctx); err != nil {
				t.Fatalf("the same handle's retried recovery = %v, want it to proceed", err)
			}
			if rec, gerr := h.Get(ctx, "te46_orphan"); gerr != nil || rec.State != action.StateOutcomeUnknown {
				t.Fatalf("the orphan after the retried recovery = %v %v, want OUTCOME_UNKNOWN", rec.State, gerr)
			}
		})
	}
}

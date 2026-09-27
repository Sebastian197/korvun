// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Train E, batch 3 (GE3/GE6) — TE53 (plan v3, §6 and §7), beside D08: OpenFor
// judges the standing, then prunes; the prune judges again inside its own
// transaction and decides there. A ledger that changed hands, or whose shape
// broke, between the two judgements is not pruned by that open, and the open
// still hands out its handle. D08 keeps its own contract untouched; these moulds
// add what D08 does not observe: a ledger above its retention cap, so the
// prune WOULD reach its DELETE, a trap that counts every DELETE attempted on
// the acts, and the commit of the change acknowledged from outside before A
// resumes.

package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/identity"
)

// e3ArmDeleteTrap puts a BEFORE DELETE trap on table: it counts the DELETE,
// outside the transaction that attempts it, and aborts it.
func e3ArmDeleteTrap(t *testing.T, path, table string) e2Traps {
	t.Helper()
	e2RegisterTrap()
	token := fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())
	e2Lay(t, path, fmt.Sprintf(`CREATE TRIGGER e3_trap_%s_delete BEFORE DELETE ON %s BEGIN SELECT e2_trap('%s'); SELECT RAISE(ABORT, 'e3 trap: DELETE on %s'); END`,
		table, table, strings.ReplaceAll(token, "'", "''"), table))
	return e2Traps{token: token}
}

// e3AboveTheCap is a ledger founded by profileA with three more terminal
// acts, and the handles the moulds need; OpenFor's handles on it get a
// retention cap of one act (openCapSeam), so a prune that runs has acts to
// delete.
func e3AboveTheCap(t *testing.T) (*Store, func(id string) (action.Envelope, identity.Evidence)) {
	t.Helper()
	a, evidenceFor := foundedFor(t, profileA)
	for i := 1; i <= 3; i++ {
		mustRecord(t, a, fmt.Sprintf("te53_denied_%d", i), action.StateDenied)
	}
	o := &openCapOverride{path: e2Abs(t, a.path), capRows: 1}
	if !openCapSeam.CompareAndSwap(nil, o) {
		t.Fatal("another cap override is armed: the moulds that set one are sequential")
	}
	t.Cleanup(func() { openCapSeam.CompareAndSwap(o, nil) })
	return a, evidenceFor
}

// e3PruneSeam arms openPruneSeam with fn for the mould.
func e3PruneSeam(t *testing.T, fn func()) {
	t.Helper()
	if !openPruneSeam.CompareAndSwap(nil, &fn) {
		t.Fatal("another prune seam is armed: the moulds that set one are sequential")
	}
	t.Cleanup(func() { openPruneSeam.Store(nil) })
}

// TE53, the calibration · with nothing between A's judgement and its prune,
// the ledger above its cap is pruned: the trap counts the DELETE and aborts
// it, and the open fails on the aborted prune. It proves the trap sees the
// prune and the fixture reaches the DELETE; it is not the guarantee.
//
// Evidence level: in process, native DB, a real trigger.
func TestE3_TE53_calibration_theTrapSeesThePrune(t *testing.T) {
	a, _ := e3AboveTheCap(t)
	path := a.path
	_ = a.Close()
	traps := e3ArmDeleteTrap(t, path, "actions")
	h, err := OpenFor(path, profileA)
	if h != nil {
		_ = h.Close()
	}
	if traps.hits() == 0 || err == nil {
		t.Fatalf("calibration: the prune of a ledger above its cap attempted %d DELETE(s), OpenFor = %v; want the DELETE seen and aborted", traps.hits(), err)
	}
	e3Evidence(t, "TE53 calibration: the trap counted %d DELETE(s); OpenFor = %v", traps.hits(), err)
}

// TE53 · an adoption by a second real handle, committed between OpenFor's
// judgement of the standing and its prune — its commit read back by a third
// connection before A resumes — leaves A's prune nothing to delete: zero
// DELETE attempted on a ledger above its cap, every record and receipt as the
// adoption left them, and A's handle comes back, foreign, owned by B, its
// writes refused by name.
//
// PROBING MUTATIONS (MU53): the prune's DELETE outside its judged transaction
// → the trap counts it → reddens; OpenFor fatal on the prune's named refusal
// → reddens.
//
// Evidence level: two real handles on one file (A the opener, B an existing
// operator handle), a third raw connection, in process.
func TestE3_TE53_anAdoptionBetweenTheJudgementAndThePrunePrunesNothing(t *testing.T) {
	ctx := context.Background()
	a, evidenceFor := e3AboveTheCap(t)
	path := a.path
	b, err := OpenOperatorFor(path, profileB)
	if err != nil {
		t.Fatalf("open B: %v", err)
	}
	defer func() { _ = b.Close() }()
	wireSealedLike(t, b, a)
	env, evidence := evidenceFor("act_te53_adopt")
	_ = a.Close()
	traps := e3ArmDeleteTrap(t, path, "actions")
	var adopted e2State
	seamRan := false
	e3PruneSeam(t, func() {
		seamRan = true
		if _, err := b.AdoptLedger(ctx, adoptionEnv(env), Decision{Outcome: "allow", Rule: "operator"}, evidence, profileB); err != nil {
			t.Errorf("B's adoption between A's judgement and its prune: %v", err)
			return
		}
		row := e3IdentityRow(t, path)
		if !strings.HasPrefix(row, "owner="+profileB+" ") {
			t.Errorf("B's adoption returned, but a third connection reads the identity row %q", row)
		}
		adopted = e2Snapshot(t, path)
		e3Evidence(t, "TE53 adoption: B's AdoptLedger returned; a third connection reads %s; actions %d, receipts %d", row, adopted.counts["actions"], adopted.counts["receipts"])
	})
	h, err := OpenFor(path, profileA)
	if hits := traps.hits(); hits != 0 {
		t.Fatalf("A's prune attempted %d DELETE(s) on the acts of a ledger that changed hands, want none", hits)
	}
	if err != nil {
		t.Fatalf("OpenFor after an adoption between the judgement and the prune = %v, want a handle (no prune, not a failed boot)", err)
	}
	defer func() { _ = h.Close() }()
	if !seamRan || adopted.counts == nil {
		t.Fatal("the open never reached its seam, or the adoption never committed")
	}
	e2Same(t, "every record and receipt after A's open", adopted, e2Snapshot(t, path))
	if standing, owner, err := h.Standing(ctx); err != nil || standing != LedgerStandingForeignProfile || owner != profileB {
		t.Fatalf("A's handle sees %q %q %v, want foreign owned by B", standing, owner, err)
	}
	if err := guardedWrite(h, "te53_after"); !errors.Is(err, ErrLedgerForeignProfile) {
		t.Fatalf("a write through A's handle = %v, want ErrLedgerForeignProfile", err)
	}
}

// TE53, the shape · a UNIQUE index dropped by another connection between
// OpenFor's judgement and its prune — the drop committed and read back before
// A resumes — makes the prune's own judgement unreadable: zero DELETE on a
// ledger above its cap, the file as the drop left it, and A's handle comes
// back blocked, naming the index, its writes refused.
//
// PROBING MUTATIONS (MU53): the DELETE outside the judged transaction →
// reddens; OpenFor fatal on the prune's unreadable refusal → reddens.
//
// Evidence level: in process, native connections, a second raw connection.
func TestE3_TE53_aShapeBrokenBeforeThePrunePrunesNothing(t *testing.T) {
	ctx := context.Background()
	a, _ := e3AboveTheCap(t)
	path := a.path
	_ = a.Close()
	traps := e3ArmDeleteTrap(t, path, "actions")
	var broken e2State
	e3PruneSeam(t, func() {
		e2Lay(t, path, `DROP INDEX budget_debits_sequence`)
		if n := rawCount(t, e2Raw(t, path), `SELECT COUNT(*) FROM sqlite_master WHERE name = 'budget_debits_sequence'`); n != 0 {
			t.Errorf("the drop returned, but another connection still reads %d budget_debits_sequence", n)
		}
		broken = e2Snapshot(t, path)
		e3Evidence(t, "TE53 shape: the index drop committed and was read back; actions %d, receipts %d", broken.counts["actions"], broken.counts["receipts"])
	})
	h, err := OpenFor(path, profileA)
	if hits := traps.hits(); hits != 0 {
		t.Fatalf("A's prune attempted %d DELETE(s) on the acts of a ledger whose shape broke, want none", hits)
	}
	if err != nil {
		t.Fatalf("OpenFor after the shape broke between the judgement and the prune = %v, want a blocked handle", err)
	}
	defer func() { _ = h.Close() }()
	if broken.counts == nil {
		t.Fatal("the open never reached its seam")
	}
	e2Same(t, "the file after A's open", broken, e2Snapshot(t, path))
	standing, owner, err := h.Standing(ctx)
	if standing != LedgerStandingUnreadable || !errors.Is(err, ErrLedgerUnreadable) || !strings.Contains(owner+" "+fmt.Sprint(err), "budget_debits_sequence") {
		t.Fatalf("A's handle sees %q %q %v, want unreadable naming budget_debits_sequence", standing, owner, err)
	}
	if err := guardedWrite(h, "te53_after"); !errors.Is(err, ErrLedgerUnreadable) {
		t.Fatalf("a write through A's handle = %v, want ErrLedgerUnreadable", err)
	}
}

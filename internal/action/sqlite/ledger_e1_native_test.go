// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Train E, batch 1 — TE67 (plan v3, §6 and §7 «TE67 preserves N9-1 step 5
// as a native page-corruption mould»): the receipts' B-tree pages of a
// legacy ledger damaged on disk, byte by byte, and the ledger opened through
// the real store path. No coded error is injected anywhere: the failure the
// judge meets is SQLite's own.
//
// Evidence level: a native damaged database, in process; the app's half of
// the row (a non-strict boot that lives) is in the app package.

package sqlite

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	msqlite "modernc.org/sqlite"
)

// e1DamagedReceipts is the damage of TE67: every page the receipts table and
// its indexes are rooted at gets 0xff for its B-tree page-type byte. It
// returns the offsets it wrote; page 1 and every page of another table are
// never touched.
func e1DamagedReceipts(t *testing.T, path string) []int64 {
	t.Helper()
	raw := rawConnPath(t, path)
	var pageSize int64
	if err := raw.QueryRow(`PRAGMA page_size`).Scan(&pageSize); err != nil {
		t.Fatal(err)
	}
	rows, err := raw.Query(`SELECT rootpage FROM sqlite_master WHERE tbl_name = 'receipts' AND rootpage > 1`)
	if err != nil {
		t.Fatal(err)
	}
	var roots []int64
	for rows.Next() {
		var r int64
		if err := rows.Scan(&r); err != nil {
			t.Fatal(err)
		}
		roots = append(roots, r)
	}
	_ = rows.Close()
	others := map[int64]bool{}
	orows, err := raw.Query(`SELECT rootpage FROM sqlite_master WHERE tbl_name <> 'receipts' AND rootpage > 0`)
	if err != nil {
		t.Fatal(err)
	}
	for orows.Next() {
		var r int64
		if err := orows.Scan(&r); err != nil {
			t.Fatal(err)
		}
		others[r] = true
	}
	_ = orows.Close()
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	if len(roots) == 0 {
		t.Fatal("the receipts table has no root page recorded")
	}
	if wal, err := os.Stat(path + "-wal"); err == nil && wal.Size() > 0 {
		t.Fatalf("a WAL of %d bytes is pending: the damage would not be the file's", wal.Size())
	}
	data, err := os.ReadFile(path) //nolint:gosec // G304: the test's own temp file
	if err != nil {
		t.Fatal(err)
	}
	var offsets []int64
	for _, root := range roots {
		if others[root] {
			t.Fatalf("page %d roots receipts AND another table", root)
		}
		off := (root - 1) * pageSize
		if off+pageSize > int64(len(data)) {
			t.Fatalf("root page %d lies past the file", root)
		}
		data[off] = 0xff
		offsets = append(offsets, off)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return offsets
}

// TE67 · N9-1 step 5, NATIVE: a legacy ledger (no identity row, no mark)
// with a valid unmarked receipt, checkpointed and closed, whose receipts
// pages are damaged on disk. CALIBRATION FIRST: the damage must surface as
// SQLite's own CORRUPT (11) at the MARK read — hook or store — while the
// catalog, the version and the owner reads succeed; any other code, an
// earlier failure or a mark read never reached fails the fixture, and no
// synthetic fault stands in for it. Then the outcome: OpenFor hands out a
// BLOCKED handle, its Standing names the receipts' damage, the guard refuses
// by name, and the damaged bytes are still there after the open.
//
// PROBING MUTATION (MU67): keep the mark read's failure as a raw operational
// error (no structural decision at Qmark) → the hook refuses the connection
// and the open dies → reddens.
func TestE1_TE67_nativeReceiptCorruptionOpensBlocked(t *testing.T) {
	src, _ := e1Legacy(t, true)
	path := filepath.Join(t.TempDir(), "korvun.db")
	coldCopy(t, src, path)
	untouched := path + ".untouched"
	copyFile(t, path, untouched)
	offsets := e1DamagedReceipts(t, path)

	abs, _ := filepath.Abs(path)
	observer := &judgeReadObserver{path: abs}
	judgeReadObserverSeam.Store(observer)
	t.Cleanup(func() { judgeReadObserverSeam.CompareAndSwap(observer, nil) })
	h, err := OpenFor(path, profileA)
	judgeReadObserverSeam.CompareAndSwap(observer, nil)

	// Calibration: the fixture is only a fixture if the damage surfaced
	// natively, as CORRUPT, at a mark read and nowhere before it.
	observer.mu.Lock()
	seen := append([]judgeReadObservation(nil), observer.seen...)
	observer.mu.Unlock()
	if len(seen) == 0 {
		if h != nil {
			_ = h.Close()
		}
		t.Fatalf("calibration failed: no judge read met the damage (OpenFor = %v)", err)
	}
	first := seen[0]
	var native *msqlite.Error
	if (first.site != siteMarkDriver && first.site != siteMarkDB) || !errors.As(first.err, &native) || native.Code()&0xff != 11 {
		if h != nil {
			_ = h.Close()
		}
		t.Fatalf("calibration failed: the first failure was %s/%s at %s: %v — want native CORRUPT (11) at a mark read", first.origin, first.stage, first.site, first.err)
	}
	// A failure seen at another read is only admitted when it IS the mark
	// read's own native error travelling up (a connection born inside that
	// read); a failure of its own — another error object — means the damage
	// reached a read before the mark, and the fixture is not this row's.
	for _, o := range seen[1:] {
		if o.site != siteMarkDriver && o.site != siteMarkDB && !errors.Is(o.err, first.err) {
			if h != nil {
				_ = h.Close()
			}
			t.Fatalf("calibration failed: %s failed at %s (%v): the damage reached a read before the mark", o.origin, o.site, o.err)
		}
	}

	if err != nil || h == nil {
		t.Fatalf("OpenFor over natively damaged receipts = %v, want a blocked handle", err)
	}
	defer func() { _ = h.Close() }()
	st, _, serr := h.Standing(context.Background())
	if st != LedgerStandingUnreadable || !errors.Is(serr, ErrLedgerUnreadable) || !strings.Contains(serr.Error(), "profile mark") {
		t.Fatalf("Standing = %q %v, want unreadable naming the mark read's damage", st, serr)
	}
	if !errors.As(serr, &native) || native.Code()&0xff != 11 {
		t.Fatalf("the standing's error %v lost the native code", serr)
	}
	if row := guardRow(t, h); row != string(LedgerStandingUnreadable) {
		t.Fatalf("the guard row = %q, want ledger_unreadable", row)
	}
	if err := guardedWrite(h, "e1_native"); !errors.Is(err, ErrLedgerUnreadable) {
		t.Fatalf("a write = %v, want ErrLedgerUnreadable", err)
	}
	_ = h.Close()
	data, err := os.ReadFile(path) //nolint:gosec // G304: the test's own temp file
	if err != nil {
		t.Fatal(err)
	}
	for _, off := range offsets {
		if data[off] != 0xff {
			t.Fatalf("the damaged byte at %d was rewritten (%#x): something repaired the receipts", off, data[off])
		}
	}
	if kept, err := os.ReadFile(untouched); err != nil || bytes.Equal(kept, data) { //nolint:gosec // G304: the test's own temp file
		t.Fatalf("the untouched copy is missing or equal to the damaged file (%v)", err)
	}
}

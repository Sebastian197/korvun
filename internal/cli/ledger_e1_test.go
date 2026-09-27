// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/app"
	msqlite "modernc.org/sqlite"
)

// Train E, batch 1 — the compiled CLI's half of TE13, TE15 and TE31, and
// TE52 (plan v3, §6): the korvun binary, built from this tree and run as a
// separate OS process over a ledger damaged, replaced or locked on disk.
//
// Evidence level: binary in a separate OS process on a real file; TE52's
// lock is held by a SECOND process (this test binary re-executed), and its
// native code is confirmed in process against the same held lock.

// TE13 (the CLI's half) · the version column of a founded ledger renamed:
// `ledger check` names the standing unreadable, then walks the intact chain
// and exits 0 — the defect is the ledger's, not the chain's.
//
// PROBING MUTATION (MU13): the shape judge propagates the code-1 failure as
// operational → the reader does not open → exit 1 → reddens.
func TestE1_TE13_ledgerCheckNamesARenamedVersionAndWalksTheChain(t *testing.T) {
	bin := buildKorvunBinary(t)
	cfgPath, dbPath := seedChain(t, 1)
	foundLedgerWithoutTheCLIPrincipal(t, cfgPath, dbPath)
	if _, err := rawLedger(t, dbPath).Exec(`ALTER TABLE action_schema RENAME COLUMN version TO v`); err != nil {
		t.Fatal(err)
	}
	code, out := runBinary(t, bin, "ledger", "check", "--config", cfgPath)
	if code != 0 || !strings.Contains(out, "ledger standing: ledger_unreadable") || !strings.Contains(out, "version") || !strings.Contains(out, "chain intact") {
		t.Fatalf("`ledger check` over a renamed version column = exit %d\n%s\nwant the standing named, the chain intact, exit 0", code, out)
	}
}

// TE15 (the CLI's half) · N9-1 over the compiled CLI: result_digest renamed
// to rd on a legacy ledger. The standing line names it; the walk itself
// cannot read the receipts and fails — exit 1, never «chain intact».
//
// PROBING MUTATION (MU15): ignore ListReceipts' failure, or print «chain
// intact» over it → reddens.
func TestE1_TE15_ledgerCheckOverARenamedMarkColumnFails(t *testing.T) {
	bin := buildKorvunBinary(t)
	cfgPath, dbPath := seedChain(t, 1)
	if _, err := rawLedger(t, dbPath).Exec(`ALTER TABLE receipts RENAME COLUMN result_digest TO rd`); err != nil {
		t.Fatal(err)
	}
	code, out := runBinary(t, bin, "ledger", "check", "--config", cfgPath)
	if code != 1 || !strings.Contains(out, "ledger standing: ledger_unreadable") || !strings.Contains(out, "no such column: result_digest") || strings.Contains(out, "chain intact") {
		t.Fatalf("`ledger check` over a renamed result_digest = exit %d\n%s\nwant the standing named, the walk's failure, exit 1", code, out)
	}
}

// TE31 (the CLI's half) · a text file where the ledger belongs: the reader
// and the writer both exit 1 naming ledger_unreadable, with the native
// «not a database» cause.
//
// PROBING MUTATION (MU31): the reader's outer classification omitted → the
// token disappears → reddens.
func TestE1_TE31_theCLIOverATextFileNamesItUnreadable(t *testing.T) {
	bin := buildKorvunBinary(t)
	cfgPath, dbPath := intentTestConfig(t)
	if err := os.WriteFile(dbPath, []byte(strings.Repeat("this file is not a SQLite database; it is text.\n", 128)), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"ledger", "check", "--config", cfgPath}, {"receipt", "rotate-key", "--config", cfgPath}} {
		code, out := runBinary(t, bin, args...)
		if code != 1 || !strings.Contains(out, "ledger_unreadable") || !strings.Contains(out, "not a database") {
			t.Fatalf("`%s` over a text file = exit %d\n%s\nwant exit 1 naming ledger_unreadable with the native cause", strings.Join(args[:2], " "), code, out)
		}
	}
}

// e1LockHolderEnv names the file the lock-holding child locks.
const e1LockHolderEnv = "KORVUN_E1_LOCK_HOLDER_DB"

// TestE1_TE52_lockHolderChild is not a mould: it is TE52's second process.
// It puts the closed ledger in DELETE journal mode, takes an EXCLUSIVE lock
// with a write inside it, announces it on stdout, and holds it until its
// stdin closes.
func TestE1_TE52_lockHolderChild(t *testing.T) {
	path := os.Getenv(e1LockHolderEnv)
	if path == "" {
		t.Skip("TE52's lock-holding child; run only by that mould")
	}
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	var mode string
	if err := conn.QueryRowContext(ctx, `PRAGMA journal_mode = DELETE`).Scan(&mode); err != nil || mode != "delete" {
		t.Fatalf("journal_mode = %q %v", mode, err)
	}
	for _, q := range []string{`PRAGMA locking_mode = EXCLUSIVE`, `BEGIN EXCLUSIVE`, `UPDATE action_schema SET version = version`} {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	fmt.Println("E1-LOCKED")
	_, _ = io.Copy(io.Discard, os.Stdin)
	_, _ = conn.ExecContext(ctx, `ROLLBACK`)
}

// TE52 · a reader beside another PROCESS that holds the file's exclusive
// lock (DELETE journal mode, a write inside BEGIN EXCLUSIVE): the reader's
// connection cannot be born — its seal fails with the native BUSY (5) before
// any Standing — and `ledger check` exits 1 naming ledger_busy; once the
// lock is released, the same command opens and walks the chain.
//
// PROBING MUTATIONS (MU52): remove the busy class at the reader's outer
// boundary → the token disappears → reddens; swallow the failed seal → a
// reader comes back → reddens.
func TestE1_TE52_aReaderBesideAnExclusiveLockIsNamedBusy(t *testing.T) {
	bin := buildKorvunBinary(t)
	cfgPath, dbPath := seedChain(t, 1)
	child := exec.Command(os.Args[0], "-test.run", "^TestE1_TE52_lockHolderChild$", "-test.count=1") //nolint:gosec // G204: this test binary re-executed as TE52's second process
	child.Env = append(os.Environ(), e1LockHolderEnv+"="+dbPath)
	stdin, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	released := false
	release := func() {
		if !released {
			released = true
			_ = stdin.Close()
			_ = child.Wait()
		}
	}
	t.Cleanup(release)
	ready := make(chan bool, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if strings.Contains(sc.Text(), "E1-LOCKED") {
				ready <- true
				_, _ = io.Copy(io.Discard, stdout)
				return
			}
		}
		ready <- false
	}()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("the lock holder exited without announcing its lock")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the lock holder never announced its lock")
	}

	// The native code and the site, confirmed in process against the held lock.
	h, err := actionsqlite.OpenReadOnlyFor(dbPath, app.ProfileIdentity(cfgPath))
	if h != nil {
		_ = h.Close()
		t.Fatal("the reader opened beside an exclusive lock")
	}
	var native *msqlite.Error
	if !errors.As(err, &native) || native.Code()&0xff != 5 || !strings.Contains(err.Error(), "seal read-only connection") {
		t.Fatalf("the reader beside an exclusive lock = %v, want the native BUSY (5) at its seal", err)
	}
	if !errors.Is(err, actionsqlite.ErrLedgerBusy) {
		t.Fatalf("the reader beside an exclusive lock = %v, want ErrLedgerBusy", err)
	}

	code, out := runBinary(t, bin, "ledger", "check", "--config", cfgPath)
	if code != 1 || !strings.Contains(out, "ledger_busy") || strings.Contains(out, "ledger standing:") {
		t.Fatalf("`ledger check` beside an exclusive lock = exit %d\n%s\nwant exit 1 naming ledger_busy before any standing", code, out)
	}
	release()
	code, out = runBinary(t, bin, "ledger", "check", "--config", cfgPath)
	if code != 0 || !strings.Contains(out, "chain intact") {
		t.Fatalf("`ledger check` after the release = exit %d\n%s\nwant it to open and walk the chain", code, out)
	}
}

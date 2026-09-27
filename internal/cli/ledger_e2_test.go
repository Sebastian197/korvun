// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bufio"
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"
	"time"

	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	convsqlite "github.com/Sebastian197/korvun/internal/conversation/sqlite"
)

// Train E, batch 2 — the CLI's half of TE02, TE05, TE69 and TE70 (plan v3,
// §6 and §13.1): an existing file that carries only the conversations and an
// EMPTY prefix of the action store's v1 bootstrap is «no action store» for
// the reader and for the operator's door — named, exit 1, nothing written.
//
// Evidence level: TE02 and TE05 run the korvun binary built from this tree as
// a separate OS process; TE05's prefix is left by a SECOND process (this test
// binary re-executed) that stays alive with the file open until the reader
// has exited. TE69 and TE70 run the CLI in process through Run. The one-shot
// read fault of TE69/TE70 is armed only by the sqlite package's own moulds —
// its seam is internal to that package (plan §7: no cross-package test
// control) — so these halves meet the prefix itself, not the fault.

// e2V1Bootstrap is the v1 bootstrap of the action store, statement by
// statement, as internal/action/sqlite's createStmt writes it.
var e2V1Bootstrap = []string{
	`CREATE TABLE IF NOT EXISTS action_schema (
    version INTEGER NOT NULL
)`,
	`CREATE TABLE IF NOT EXISTS actions (
    action_id         TEXT    NOT NULL PRIMARY KEY,
    schema_version    INTEGER NOT NULL,
    correlation_id    TEXT    NOT NULL,
    source_kind       TEXT    NOT NULL,
    source_protocol   TEXT    NOT NULL,
    source_channel    TEXT    NOT NULL,
    op_namespace      TEXT    NOT NULL,
    op_name           TEXT    NOT NULL,
    op_version        INTEGER NOT NULL,
    parameters_digest TEXT    NOT NULL,
    effect_class      TEXT    NOT NULL,
    state             TEXT    NOT NULL,
    recovery_marker   TEXT    NOT NULL DEFAULT '',
    requested_at      TEXT    NOT NULL,
    finished_at       TEXT
) WITHOUT ROWID`,
	`CREATE INDEX IF NOT EXISTS actions_by_correlation ON actions(correlation_id)`,
	`CREATE INDEX IF NOT EXISTS actions_by_requested ON actions(requested_at)`,
	`CREATE TABLE IF NOT EXISTS action_decisions (
    action_id  TEXT NOT NULL PRIMARY KEY REFERENCES actions(action_id) ON DELETE CASCADE,
    outcome    TEXT NOT NULL,
    rule       TEXT NOT NULL,
    decided_at TEXT NOT NULL
) WITHOUT ROWID`,
}

// e2PrefixConfig is a config whose storage is a conversation file with one
// session, carrying the first k bootstrap statements.
func e2PrefixConfig(t *testing.T, k int) (cfgPath, dbPath string) {
	t.Helper()
	cfgPath, dbPath = intentTestConfig(t)
	conv, err := convsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open the conversation store: %v", err)
	}
	if err := conv.Close(); err != nil {
		t.Fatal(err)
	}
	raw := rawLedger(t, dbPath)
	if _, err := raw.Exec(`INSERT INTO sessions (key, id, created_ts) VALUES ('k', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	for _, q := range e2V1Bootstrap[:k] {
		if _, err := raw.Exec(q); err != nil {
			t.Fatalf("lay the prefix: %v", err)
		}
	}
	return cfgPath, dbPath
}

// e2Catalog is the file's catalog and each table's row count, read raw.
func e2Catalog(t *testing.T, dbPath string) string {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	rows, err := raw.Query(`SELECT type, name, tbl_name, ifnull(sql, '') FROM sqlite_master`)
	if err != nil {
		t.Fatal(err)
	}
	var lines, tables []string
	for rows.Next() {
		var kind, name, tbl, ddl string
		if err := rows.Scan(&kind, &name, &tbl, &ddl); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, kind+":"+name+":"+tbl+":"+ddl)
		if kind == "table" {
			tables = append(tables, name)
		}
	}
	_ = rows.Close()
	for _, tb := range tables {
		var n int
		if err := raw.QueryRow(`SELECT COUNT(*) FROM "` + tb + `"`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, fmt.Sprintf("rows:%s:%d", tb, n))
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// e2NoStore is the one outcome of the CLI over an existing prefix: exit 1,
// the opener's «no action store», no standing line, the file as it was.
func e2NoStore(t *testing.T, label string, code int, out, before, after string) {
	t.Helper()
	if code != 1 || !strings.Contains(out, "no action store in the file") || strings.Contains(out, "ledger standing:") {
		t.Fatalf("%s = exit %d\n%s\nwant exit 1 naming «no action store» and no standing line", label, code, out)
	}
	if before != after {
		t.Fatalf("%s changed the file\nbefore:\n%s\nafter:\n%s", label, before, after)
	}
}

// TE02 (the compiled CLI's half) · each empty prefix k = 1…5 of an existing
// file: `ledger check` (the reader) and `receipt rotate-key` (the operator's
// door) exit 1 naming «no action store in the file», print no standing, and
// leave the file as it was.
//
// PROBING MUTATION (MU02): classify a prefix bad → the reader opens and
// prints ledger_unreadable → reddens.
func TestE2_TE02_theCLIOverAnEmptyPrefixNamesNoStore(t *testing.T) {
	bin := buildKorvunBinary(t)
	for k := 1; k <= 5; k++ {
		cfgPath, dbPath := e2PrefixConfig(t, k)
		for _, args := range [][]string{{"ledger", "check", "--config", cfgPath}, {"receipt", "rotate-key", "--config", cfgPath}} {
			before := e2Catalog(t, dbPath)
			code, out := runBinary(t, bin, args...)
			e2NoStore(t, fmt.Sprintf("k%d `%s`", k, strings.Join(args[:2], " ")), code, out, before, e2Catalog(t, dbPath))
		}
	}
}

// e2PrefixChildEnv names the file TE05's child lays its prefix on, and k.
const e2PrefixChildEnv = "KORVUN_E2_PREFIX_CHILD"

// TestE2_TE05_prefixChild is not a mould: it is TE05's second process. It
// lays the first k statements of the historical seed in autocommit, says so
// on stdout, and keeps the file open until its stdin closes — never laying
// the version row.
func TestE2_TE05_prefixChild(t *testing.T) {
	spec := os.Getenv(e2PrefixChildEnv)
	if spec == "" {
		t.Skip("TE05's prefix-laying child; run only by that mould")
	}
	path, kText, _ := strings.Cut(spec, "|")
	var k int
	if _, err := fmt.Sscan(kText, &k); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, q := range e2V1Bootstrap[:k] {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("lay the prefix: %v", err)
		}
	}
	_, _ = fmt.Fprintf(os.Stdout, "E2PREFIX-READY %d\n", k)
	_, _ = io.Copy(io.Discard, os.Stdin)
}

// TE05 · a reader beside a LIVE historical prefix. A second process lays
// the first k statements of the historical seed in autocommit and stays
// alive with the file open, its version row never written; the parent
// checks the prefix is there with action_schema empty, then the reader —
// the compiled `ledger check`, and the library reader in process — meets
// it: «no action store», exit 1, no standing line. The prefix is the same
// after the reader has exited, and only then does the child go.
//
// PROBING MUTATION (MU05): classify the residue bad → the reader opens and
// names ledger_unreadable → reddens.
func TestE2_TE05_aReaderBesideALiveHistoricalPrefixNamesNoStore(t *testing.T) {
	bin := buildKorvunBinary(t)
	for k := 1; k <= 5; k++ {
		cfgPath, dbPath := e2PrefixConfig(t, 0)
		// #nosec G204 -- TE05 deliberately re-executes this test binary.
		cmd := exec.Command(os.Args[0], "-test.run=^TestE2_TE05_prefixChild$", "-test.count=1")
		cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%s|%d", e2PrefixChildEnv, dbPath, k))
		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		ready := make(chan string, 1)
		go func() {
			sc := bufio.NewScanner(stdout)
			for sc.Scan() {
				if strings.HasPrefix(sc.Text(), "E2PREFIX-READY") {
					ready <- sc.Text()
				}
			}
			close(ready)
		}()
		select {
		case l, ok := <-ready:
			if !ok || l != fmt.Sprintf("E2PREFIX-READY %d", k) {
				_ = cmd.Process.Kill()
				t.Fatalf("k%d: the child never laid its prefix (%q)\n%s", k, l, stderr.String())
			}
		case <-time.After(time.Minute):
			_ = cmd.Process.Kill()
			t.Fatalf("k%d: the child said nothing within a minute\n%s", k, stderr.String())
		}
		// The readiness: the prefix is there, its version table empty.
		raw := rawLedger(t, dbPath)
		var objects, versions int
		if err := raw.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('action_schema', 'actions', 'actions_by_correlation', 'actions_by_requested', 'action_decisions')`).Scan(&objects); err != nil || objects != k {
			_ = cmd.Process.Kill()
			t.Fatalf("k%d: the file holds %d objects of the prefix (%v), want %d", k, objects, err, k)
		}
		if err := raw.QueryRow(`SELECT COUNT(*) FROM action_schema`).Scan(&versions); err != nil || versions != 0 {
			_ = cmd.Process.Kill()
			t.Fatalf("k%d: action_schema holds %d rows (%v), want 0", k, versions, err)
		}
		before := e2Catalog(t, dbPath)
		code, out := runBinary(t, bin, "ledger", "check", "--config", cfgPath)
		e2NoStore(t, fmt.Sprintf("k%d `ledger check` beside the live prefix", k), code, out, before, e2Catalog(t, dbPath))
		h, err := actionsqlite.OpenReadOnlyFor(dbPath, "sha256:"+strings.Repeat("ab", 32))
		if h != nil {
			_ = h.Close()
			_ = cmd.Process.Kill()
			t.Fatalf("k%d: the library reader opened over the live prefix", k)
		}
		if err == nil || !strings.Contains(err.Error(), "no action store in the file") {
			_ = cmd.Process.Kill()
			t.Fatalf("k%d: the library reader = %v, want «no action store»", k, err)
		}
		if after := e2Catalog(t, dbPath); after != before {
			_ = cmd.Process.Kill()
			t.Fatalf("k%d: the prefix changed while the child lived", k)
		}
		_ = stdin.Close()
		if err := cmd.Wait(); err != nil {
			t.Fatalf("k%d: the child ended with %v\n%s", k, err, stderr.String())
		}
	}
}

// TE69 (the CLI's half) · the operator's door through the CLI, in process:
// `receipt rotate-key` over an existing file carrying an empty prefix exits 1
// naming «no action store in the file», and the file keeps no version row,
// no DDL of the door's.
//
// PROBING MUTATION (MU02, the recogniser's): classify a prefix bad → the
// door opens it blocked and the rotation fails on ledger_unreadable instead
// of «no action store» → reddens. MU69 needs the probe's one-shot fault, and
// only the sqlite package's half can arm it.
func TestE2_TE69_theCLIWriterNeverSeedsAnExistingPrefix(t *testing.T) {
	for k := 1; k <= 5; k++ {
		cfgPath, dbPath := e2PrefixConfig(t, k)
		before := e2Catalog(t, dbPath)
		var stdout, stderr bytes.Buffer
		code := Run([]string{"receipt", "rotate-key", "--config", cfgPath}, &stdout, &stderr)
		e2NoStore(t, fmt.Sprintf("k%d `receipt rotate-key`", k), code, stdout.String()+stderr.String(), before, e2Catalog(t, dbPath))
	}
}

// TE70 (the CLI's half) · `ledger check` through Run, in process, over an
// existing file carrying an empty prefix: exit 1 with the reader's «no action
// store», and no standing line.
//
// PROBING MUTATION (MU02, the recogniser's): classify a prefix bad → the
// reader opens and prints ledger_unreadable → reddens. MU70 needs the
// reader's one-shot fault, and only the sqlite package's half can arm it.
func TestE2_TE70_ledgerCheckOverAnEmptyPrefixNamesNoStore(t *testing.T) {
	for k := 1; k <= 5; k++ {
		cfgPath, dbPath := e2PrefixConfig(t, k)
		before := e2Catalog(t, dbPath)
		var stdout, stderr bytes.Buffer
		code := Run([]string{"ledger", "check", "--config", cfgPath}, &stdout, &stderr)
		e2NoStore(t, fmt.Sprintf("k%d `ledger check`", k), code, stdout.String()+stderr.String(), before, e2Catalog(t, dbPath))
	}
}

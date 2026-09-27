// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"database/sql"
	"errors"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// catalogOfLedger is every object of the ledger file but SQLite's own, with
// its DDL: the oracle «nothing was repaired».
func catalogOfLedger(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	rows, err := db.Query(`SELECT type || ':' || name, COALESCE(sql, '') FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			t.Fatal(err)
		}
		out[k] = v
	}
	return out
}

// exitStatus is a finished command's own exit status — 0 for a nil error, the
// status an *exec.ExitError carries — and ran=false when err is a failure to
// run the binary at all.
func exitStatus(err error) (status int, ran bool) {
	if err == nil {
		return 0, true
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), true
	}
	return -1, false
}

// D05 · a NEW PROCESS on a ledger of a bad shape: the compiled korvun binary,
// run as a separate OS process, names ledger_unreadable from its reader
// (`ledger check`) and refuses its writer (`receipt rotate-key`) by name, and
// neither repairs the file — the catalog is identical after both.
//
// Evidence level: binary in a separate OS process, real file.
//
// PROBING MUTATIONS: createStmt unconditional in the opener → the dropped
// table comes back, and `receipt rotate-key` then fails on it without the
// ledger's name → reddens there, before the catalog is compared (M285 in
// docs/superpowers/specs/evidence/v0.16.2/mutations.txt). `ledger check`
// exiting 0 over the broken chain → reddens. Either invocation unable to run
// (a binary path that does not exist) → reddens with «did not run». Until the
// first CI run of PR #69 the error was discarded, and on Windows, where the
// binary lacked its .exe, a binary that never ran failed as «did not name it»
// over an empty output.
func TestLedgerBinary_aBadShapeIsNamedAndNeverRepaired(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	bin := filepath.Join(t.TempDir(), "korvun")
	if runtime.GOOS == "windows" {
		bin += ".exe" // exec finds an absolute path on Windows only by its extension
	}
	build := exec.Command("go", "build", "-o", bin, "../../cmd/korvun") //nolint:gosec // G204: the toolchain building this repository's own binary into TempDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build the binary: %v\n%s", err, out)
	}
	cfgPath, dbPath := seedChain(t, 1)
	raw := rawLedger(t, dbPath)
	if _, err := raw.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`DROP TABLE actions`); err != nil {
		t.Fatalf("drop actions: %v", err)
	}
	before := catalogOfLedger(t, raw)
	check := exec.Command(bin, "ledger", "check", "--config", cfgPath) //nolint:gosec // G204: the binary this test just built, arguments literal
	out, err := check.CombinedOutput()
	status, ran := exitStatus(err)
	if !ran {
		t.Fatalf("`ledger check` did not run: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "ledger standing: ledger_unreadable") || !strings.Contains(string(out), "actions") {
		t.Fatalf("`ledger check` on a ledger without actions did not name it:\n%s", out)
	}
	if status != 1 {
		t.Fatalf("`ledger check` on a ledger without actions exited %d, want 1: its chain cannot verify\n%s", status, out)
	}
	rotate := exec.Command(bin, "receipt", "rotate-key", "--config", cfgPath) //nolint:gosec // G204: the binary this test just built, arguments literal
	out2, err := rotate.CombinedOutput()
	if err == nil {
		t.Fatalf("`receipt rotate-key` on a ledger without actions exited 0:\n%s", out2)
	}
	if _, ran := exitStatus(err); !ran {
		t.Fatalf("`receipt rotate-key` did not run: %v\n%s", err, out2)
	}
	if !strings.Contains(string(out2), "ledger_unreadable") {
		t.Fatalf("`receipt rotate-key` refused without the name:\n%s", out2)
	}
	if after := catalogOfLedger(t, raw); !reflect.DeepEqual(before, after) {
		t.Fatalf("the binary REPAIRED the ledger: the catalog changed (%d objects before, %d after)", len(before), len(after))
	}
}

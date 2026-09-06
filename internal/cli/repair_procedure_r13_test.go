// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R13-G6b: the manual repair procedure, EVERY step, with the operator's
// real tools and the BUILT BINARY in a separate OS process where the
// document names a korvun command. What this file runs, exactly:
//   - step 1's process check: `pgrep -f korvun` EXECUTED against
//     controlled children — the host's `sleep` through a SYMLINK under
//     <tmp>/korvun/ run by absolute path (the FALSE POSITIVE the document
//     warns about: the word in a path matches; a byte COPY of /bin/sleep
//     is killed at exec by macOS code signing — captured, so symlinks)
//     and a symlink under a wordless path (a TRUE negative that
//     demonstrates the matching mechanism — the false negative proper
//     would be a korvun binary at a wordless path, and the document says
//     so); asserted by MEMBERSHIP of the spawned pids
//     in pgrep's output, never by an empty list (other matches MAY exist
//     in the environment — which ones is a capture on each runner);
//     precondition "TMPDIR carries no 'korvun'" asserted by name; scope
//     gated by runtime.GOOS != "windows" — DECLARED (D8): pgrep is not a
//     Windows tool and the document's Windows commands are unverified;
//   - step 2's `.backup` and step 4's hex recipe by the real sqlite3
//     shell over a TEXT carrying a real embedded NUL (exact bytes
//     asserted; the `.dump` rendering is captured, never asserted);
//   - the "boot first" step: `korvun ledger check` by the BINARY refuses
//     the v11 profile by name — the refusal string of store.go's
//     OpenReadOnly pinned here for the first time (A21);
//   - the boot itself is `Open` IN PROCESS, labeled: bootstrap DDL +
//     migration + prune; NOT the server's boot (no recovery, no sweep);
//   - step 6's `korvun ledger check` and `korvun receipt verify` by the
//     BINARY in a separate OS process, exit codes and output asserted.
// Evidence level: BINARY (separate OS process) for the operator
// commands; real sqlite3 and pgrep; `Open` in-process for the boot.
// Under CI the sqlite3 skip is impossible (R12-H10). The A21 pin's red
// lives in mutation m-a21: openOperatorStore switched to the migrating
// Open — the refusal disappears (collateral: the R4-F1 class guard
// reddens too, declared).

package cli

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
)

// buildKorvunBinary builds cmd/korvun into a temp dir. Failure to build
// is loud, never a skip (R3 of the paper).
func buildKorvunBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "korvun-under-test")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, "../../cmd/korvun") //nolint:gosec // G204: the toolchain building this repository's own binary into TempDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/korvun: %v\n%s", err, out)
	}
	return bin
}

func runBinary(t *testing.T, bin string, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command(bin, args...) //nolint:gosec // G204: the binary this test just built, arguments literal
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !strings.Contains(err.Error(), "exit status") {
			t.Fatalf("run %s %v: %v", bin, args, err)
		}
		_ = exitErr
		code = cmd.ProcessState.ExitCode()
	}
	return code, string(out)
}

func TestManualRepairProcedure_byBinary(t *testing.T) {
	t.Parallel()
	sqlite3bin, lookErr := exec.LookPath("sqlite3")
	if lookErr != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("R12-H10: the sqlite3 binary is not on PATH on this CI runner (%v) — the documented repair commands MUST run under CI; fix the workflow's sqlite3 step, never skip", lookErr)
		}
		t.Skip("SKIP NAMED (R12-A9): no sqlite3 binary on this runner — the documented commands cannot be exercised")
	}
	bin := buildKorvunBinary(t)

	// The fixture: a real approved story with its receipt (the CLI's
	// own doors), then the auditor's hand — the schema forced back to
	// v11 and the tombstone's date replaced by a TEXT carrying a real
	// embedded NUL byte, the shape `.dump` cannot render faithfully.
	cfgPath, dbPath, approvalID := parkedRequest(t)
	receiptID := approvedReceiptID(t, cfgPath, dbPath, approvalID)
	nulDate := "2026-09-04T10:00:00Z\x00trailing"
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("raw: %v", err)
	}
	var exactDate string
	if err := db.QueryRow(`SELECT decision_at FROM approval_tombstones WHERE approval_id = ?`, approvalID).Scan(&exactDate); err != nil {
		t.Fatalf("read the exact preimage (the independent evidence): %v", err)
	}
	if _, err := db.Exec(`UPDATE action_schema SET version = 11`); err != nil {
		t.Fatalf("force v11: %v", err)
	}
	if _, err := db.Exec(`UPDATE approval_tombstones SET decision_at = ? WHERE approval_id = ?`, nulDate, approvalID); err != nil {
		t.Fatalf("auditor's UPDATE: %v", err)
	}
	_ = db.Close()

	// Step 1 — the process check, EXECUTED (non-Windows: declared scope).
	if runtime.GOOS == "windows" {
		t.Log("SCOPE (D8): the pgrep step is not exercised on Windows — pgrep is not a Windows tool and the document's Windows commands are unverified")
	} else {
		if strings.Contains(strings.ToLower(os.TempDir()), "korvun") {
			t.Fatalf("PRECONDITION: TMPDIR %q carries the word korvun — the pgrep false-positive probe cannot be told apart", os.TempDir())
		}
		sleepBin, err := exec.LookPath("sleep")
		if err != nil {
			t.Fatalf("sleep: %v", err)
		}
		// SYMLINKS, not copies — CAPTURED on macOS 13: a byte copy of
		// /bin/sleep is killed at exec by code signing ("Killed: 9"),
		// so a copied child never appears in any process list; a
		// symlink runs the signed binary under the PATH the caller
		// names, which is exactly what `pgrep -f` matches.
		wordDir := filepath.Join(t.TempDir(), "korvun")
		plainDir := filepath.Join(t.TempDir(), "plain")
		for _, d := range []string{wordDir, plainDir} {
			if err := os.MkdirAll(d, 0o750); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.Symlink(sleepBin, filepath.Join(d, "sleep")); err != nil {
				t.Fatalf("symlink sleep: %v", err)
			}
		}
		wordChild := exec.Command(filepath.Join(wordDir, "sleep"), "30")   //nolint:gosec // G204: the host's sleep through a symlink by absolute path
		plainChild := exec.Command(filepath.Join(plainDir, "sleep"), "30") //nolint:gosec // G204: the host's sleep through a symlink by absolute path
		if err := wordChild.Start(); err != nil {
			t.Fatalf("start: %v", err)
		}
		if err := plainChild.Start(); err != nil {
			t.Fatalf("start: %v", err)
		}
		defer func() {
			_ = wordChild.Process.Kill()
			_ = plainChild.Process.Kill()
			_, _ = wordChild.Process.Wait()
			_, _ = plainChild.Process.Wait()
		}()
		out, _ := exec.Command("pgrep", "-f", "korvun").CombinedOutput()
		pids := map[string]bool{}
		for _, line := range strings.Fields(string(out)) {
			pids[line] = true
		}
		t.Logf("pgrep -f korvun on this runner (a capture, not a claim): %v", strings.Fields(string(out)))
		if !pids[strconv.Itoa(wordChild.Process.Pid)] {
			t.Fatalf("AUDIT R13-G6b pgrep: the FALSE POSITIVE the document warns about must be observed — a sleep under a korvun-named path (pid %d) is absent from %q", wordChild.Process.Pid, out)
		}
		if pids[strconv.Itoa(plainChild.Process.Pid)] {
			t.Fatalf("AUDIT R13-G6b pgrep: the matching mechanism is the command line — a sleep under a wordless path (pid %d) must NOT match: %q", plainChild.Process.Pid, out)
		}
	}

	// Step 2 — the consistent backup by the real shell.
	backupPath := filepath.Join(t.TempDir(), "pre-repair.db")
	if out, err := exec.Command(sqlite3bin, dbPath, ".backup '"+backupPath+"'").CombinedOutput(); err != nil { //nolint:gosec // G204: the DOCUMENTED operator command
		t.Fatalf("documented .backup: %v %s", err, out)
	}
	// Step 4 — the hex recipe over the NUL-carrying TEXT: exact bytes.
	wantHex := strings.ToUpper(hexOf(nulDate))
	out, err := exec.Command(sqlite3bin, backupPath, //nolint:gosec // G204: the DOCUMENTED operator command
		`.param set @apr "`+approvalID+`"`,
		"SELECT hex(decision_at) FROM approval_tombstones WHERE approval_id = @apr;").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != wantHex {
		t.Fatalf("AUDIT R13-G6b hex recipe: the exact bytes, NUL included: %v %q (want %s)", err, out, wantHex)
	}
	dump, _ := exec.Command(sqlite3bin, backupPath, ".dump approval_tombstones").CombinedOutput() //nolint:gosec // G204: the DOCUMENTED operator command
	t.Logf(".dump rendering of the NUL-carrying TEXT on this runner's sqlite3 (captured, not asserted): %q", firstLineContaining(string(dump), "INSERT INTO approval_tombstones"))

	// "Boot first" — the read-only consult refuses the v11 profile by
	// name (A21): the BINARY, a separate OS process.
	code, out2 := runBinary(t, bin, "ledger", "check", "--config", cfgPath)
	if code != 1 || !strings.Contains(out2, "a read-only consult never migrates") || !strings.Contains(out2, "schema v11") {
		t.Fatalf("AUDIT R13-A21: `korvun ledger check` on a v11 profile must refuse by name, exit 1: %d %q", code, out2)
	}
	// The boot itself, IN PROCESS (bootstrap DDL + migration + prune;
	// not the server's boot): it fails closed naming the row, the
	// field and the procedure.
	if _, err := actionsqlite.Open(dbPath); err == nil || !strings.Contains(err.Error(), "decision_at") ||
		!strings.Contains(err.Error(), "docs/operations/tombstone-manual-repair.md") {
		t.Fatalf("the boot must fail closed naming decision_at and pointing at the procedure: %v", err)
	}
	// Step 5 — adjudicate with the EXACT independent preimage.
	db2, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("raw: %v", err)
	}
	if _, err := db2.Exec(`UPDATE approval_tombstones SET decision_at = ? WHERE approval_id = ?`, exactDate, approvalID); err != nil {
		t.Fatalf("adjudicated correction: %v", err)
	}
	_ = db2.Close()
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("after the honest repair the boot (Open in process) must converge: %v", err)
	}
	if _, _, err := store.ApprovalTombstoneByDigest(context.Background(), ""); err == nil {
		t.Fatal("sanity: an empty digest never selects a row")
	}
	_ = store.Close()
	// Step 6 — verify after, by the BINARY.
	code, out3 := runBinary(t, bin, "ledger", "check", "--config", cfgPath)
	if code != 0 || !strings.Contains(out3, "chain intact") {
		t.Fatalf("AUDIT R13-G6b: `korvun ledger check` by binary after the repair: %d %q", code, out3)
	}
	code, out4 := runBinary(t, bin, "receipt", "verify", "--config", cfgPath, receiptID)
	if code != 0 || strings.Contains(out4, "FAIL") {
		t.Fatalf("AUDIT R13-G6b: `korvun receipt verify` by binary after the repair: %d %q", code, out4)
	}
	// The SECURITY.md reading, captured from the binary: an EMPTY
	// partition reports "chain intact".
	code, out5 := runBinary(t, bin, "ledger", "check", "--config", cfgPath, "--partition", "never-written")
	if code != 0 || !strings.Contains(out5, "never-written: 0 receipts, chain intact") {
		t.Fatalf("SECURITY.md's tail reading must match the binary: %d %q", code, out5)
	}
}

func hexOf(s string) string {
	const digits = "0123456789ABCDEF"
	b := make([]byte, 0, len(s)*2)
	for i := 0; i < len(s); i++ {
		b = append(b, digits[s[i]>>4], digits[s[i]&0x0f])
	}
	return string(b)
}

func firstLineContaining(text, needle string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}

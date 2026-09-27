// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/app"
)

// Train E, batch 4 (GE7) — the CLI's surfaces: TE42, TE43, TE47 and TE51
// (plan v3, §6, §7 and §13.2). The standing line is a function of the error's
// store NAME alone — errors.Is, the first match in a fixed order — and it
// never decides an exit status: the chain walk or the receipt read does. An
// opener that fails is its handler's fixed wrapper, the store's class and the
// native cause, exit 1, and nothing after it. A restore done the way D2 says
// gives the operator the history of the copy, not a verdict remembered from
// the damaged file.

// e4Run runs the built binary and keeps stdout and stderr apart.
func e4Run(t *testing.T, bin string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(bin, args...) //nolint:gosec // G204: the binary this test just built
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run %v: %v", args, err)
		}
		code = exitErr.ExitCode()
	}
	return code, stdout.String(), stderr.String()
}

// e4Source stands in for the store printLedgerStanding reads.
type e4Source struct{ err error }

func (s e4Source) Standing(context.Context) (actionsqlite.LedgerStanding, string, error) {
	return "", "", s.err
}

func (e4Source) ProfileIdentity() string { return "sha256:" + strings.Repeat("0", 64) }

// e4Coded is a coded storage error, in the driver's shape.
type e4Coded struct{ code int }

func (e *e4Coded) Error() string { return fmt.Sprintf("injected coded failure %d", e.code) }
func (e *e4Coded) Code() int     { return e.code }

// e4ClosedPool is the error database/sql answers on a closed pool.
func e4ClosedPool(t *testing.T) error {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "closed.db"))
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	perr := db.Ping()
	if perr == nil {
		t.Fatal("a closed pool pinged")
	}
	return perr
}

// e4ExpectedLine is the standing line the CLI owes a file whose Standing is a
// verdict: the class and the error an in-process reader of the same file gets.
func e4ExpectedLine(t *testing.T, cfgPath, dbPath string) string {
	t.Helper()
	store, err := actionsqlite.OpenReadOnlyFor(dbPath, app.ProfileIdentity(cfgPath))
	if err != nil {
		t.Fatalf("open the fixture in process: %v", err)
	}
	defer func() { _ = store.Close() }()
	_, _, serr := store.Standing(context.Background())
	if !errors.Is(serr, actionsqlite.ErrLedgerUnreadable) {
		t.Fatalf("the fixture's Standing = %v, want a verdict", serr)
	}
	return fmt.Sprintf("ledger standing: ledger_unreadable (%v)", serr)
}

// e4ApprovalID has the shape action.NewApprovalID mints, so the approvals
// arms reach their opener instead of the id's usage refusal (exit 2).
const e4ApprovalID = "apr_e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4"

// e4ReadClosed opens path raw for one read and closes it before returning.
// TE51 restores files in place with their -wal and -shm removed; with a
// connection this process kept open across such a restore, the next open in
// the same process failed with disk I/O error (522) — observed in batch 4's
// RED — which is the test's own residue, not the restore's.
func e4ReadClosed(t *testing.T, path string, read func(*sql.DB)) {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = raw.Close() }()
	read(raw)
}

// e4Receipts is every receipt row of dbPath, raw, in chain order.
func e4Receipts(t *testing.T, dbPath string) string {
	t.Helper()
	var out []string
	e4ReadClosed(t, dbPath, func(raw *sql.DB) {
		rows, err := raw.Query(`SELECT * FROM receipts ORDER BY chain_seq`)
		if err != nil {
			t.Fatalf("read the receipts: %v", err)
		}
		defer func() { _ = rows.Close() }()
		cols, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			out = append(out, fmt.Sprintf("%#v", vals))
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("read the receipts: %v", err)
		}
	})
	return strings.Join(out, "\n")
}

// TE42 (the renderer) · the standing line names the store's class, by
// errors.Is, the first match in a fixed order: ledger_unreadable (the two
// verdicts), ledger_environment, ledger_busy, and unavailable for every other
// error — the literal old «database is locked», a closed pool, a context's
// end, a code with no store name and each §5 uncategorised code. One exact
// line per subcase, the cause kept.
//
// PROBING MUTATIONS (MU42, MU71): every error prints ledger_unreadable, or
// every error outside the store's names does → the non-verdict subcases
// redden.
//
// Evidence level: in process, the renderer over a double of its one input.
func TestE4_TE42_theStandingLineNamesItsClass(t *testing.T) {
	closed := e4ClosedPool(t)
	type line struct {
		name  string
		err   error
		class string
	}
	cases := []line{
		{"unreadable", fmt.Errorf("%w: table intents missing at schema v16", actionsqlite.ErrLedgerUnreadable), "ledger_unreadable"},
		{"mark malformed", fmt.Errorf("%w: the mark is not canonical", actionsqlite.ErrLedgerMarkMalformed), "ledger_unreadable"},
		{"environment", fmt.Errorf("%w: %w", actionsqlite.ErrLedgerEnvironment, &e4Coded{code: 10}), "ledger_environment"},
		{"environment 522", fmt.Errorf("%w: %w", actionsqlite.ErrLedgerEnvironment, &e4Coded{code: 522}), "ledger_environment"},
		{"busy", fmt.Errorf("%w: %w", actionsqlite.ErrLedgerBusy, &e4Coded{code: 5}), "ledger_busy"},
		{"locked", fmt.Errorf("%w: %w", actionsqlite.ErrLedgerBusy, &e4Coded{code: 6}), "ledger_busy"},
		{"database is locked, uncoded", errors.New("database is locked"), "unavailable"},
		{"closed pool", closed, "unavailable"},
		{"canceled", context.Canceled, "unavailable"},
		{"deadline", context.DeadlineExceeded, "unavailable"},
		{"a structural code without the store's name", &e4Coded{code: 11}, "unavailable"},
		{"a verdict before busy", errors.Join(fmt.Errorf("%w: x", actionsqlite.ErrLedgerBusy), fmt.Errorf("%w: y", actionsqlite.ErrLedgerUnreadable)), "ledger_unreadable"},
		{"environment before busy", errors.Join(fmt.Errorf("%w: x", actionsqlite.ErrLedgerBusy), fmt.Errorf("%w: y", actionsqlite.ErrLedgerEnvironment)), "ledger_environment"},
	}
	for _, code := range []int{2, 4, 7, 9, 12, 15, 16, 17, 18, 19, 21, 25, 27, 28, 0, 101} {
		cases = append(cases, line{fmt.Sprintf("code %d", code), &e4Coded{code: code}, "unavailable"})
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out bytes.Buffer
			printLedgerStanding(context.Background(), &out, e4Source{err: c.err})
			if want := fmt.Sprintf("ledger standing: %s (%v)\n", c.class, c.err); out.String() != want {
				t.Fatalf("the standing line = %q, want %q", out.String(), want)
			}
		})
	}
}

// TE42 (the compiled command) · over the native fixtures of TE13 and TE15,
// `ledger check` prints the exact line an in-process reader of the same file
// is owed, and its exit status is the chain's: exit 0 and «chain intact» when
// only the version column is renamed, exit 1 and no «chain intact» when the
// receipts cannot be read.
//
// PROBING MUTATION (MU42): the check returns success after a failed chain
// query → the TE15 case reddens.
//
// Evidence level: the compiled binary in a separate OS process, real files.
func TestE4_TE42_theCompiledCheckPrintsTheExactLine(t *testing.T) {
	bin := buildKorvunBinary(t)
	t.Run("TE13: a renamed version column", func(t *testing.T) {
		cfgPath, dbPath := seedChain(t, 1)
		foundLedgerWithoutTheCLIPrincipal(t, cfgPath, dbPath)
		if _, err := rawLedger(t, dbPath).Exec(`ALTER TABLE action_schema RENAME COLUMN version TO v`); err != nil {
			t.Fatal(err)
		}
		want := e4ExpectedLine(t, cfgPath, dbPath)
		code, stdout, stderr := e4Run(t, bin, "ledger", "check", "--config", cfgPath)
		if first := strings.SplitN(stdout, "\n", 2)[0]; code != 0 || first != want || !strings.Contains(stdout, "chain intact") {
			t.Fatalf("`ledger check` = exit %d, first line %q\nstdout:\n%s\nstderr:\n%s\nwant exit 0, the line %q and the chain intact", code, first, stdout, stderr, want)
		}
	})
	t.Run("TE15: a renamed result_digest", func(t *testing.T) {
		cfgPath, dbPath := seedChain(t, 1)
		if _, err := rawLedger(t, dbPath).Exec(`ALTER TABLE receipts RENAME COLUMN result_digest TO rd`); err != nil {
			t.Fatal(err)
		}
		want := e4ExpectedLine(t, cfgPath, dbPath)
		code, stdout, stderr := e4Run(t, bin, "ledger", "check", "--config", cfgPath)
		if first := strings.SplitN(stdout, "\n", 2)[0]; code != 1 || first != want || strings.Contains(stdout, "chain intact") || !strings.Contains(stderr, "result_digest") {
			t.Fatalf("`ledger check` = exit %d, first line %q\nstdout:\n%s\nstderr:\n%s\nwant exit 1, the line %q and the walk's failure", code, first, stdout, stderr, want)
		}
	})
}

// TE43 · `receipt verify` prints the same line, then really reads and verifies
// the receipt: over TE13 the receipt verifies (exit 0); over TE15 the read
// fails naming result_digest (exit 1, no OK). The receipt rows are not touched.
//
// PROBING MUTATIONS (MU43): the command skips the standing line → reddens; it
// ignores the failed GetReceipt → reddens.
//
// Evidence level: the compiled binary in a separate OS process, real files.
func TestE4_TE43_receiptVerifyNamesTheStandingAndReadsTheReceipt(t *testing.T) {
	bin := buildKorvunBinary(t)
	firstReceipt := func(t *testing.T, dbPath string) string {
		t.Helper()
		var id string
		if err := rawLedger(t, dbPath).QueryRow(`SELECT receipt_id FROM receipts ORDER BY chain_seq LIMIT 1`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	t.Run("TE13: the receipt verifies", func(t *testing.T) {
		cfgPath, dbPath := seedChain(t, 1)
		foundLedgerWithoutTheCLIPrincipal(t, cfgPath, dbPath)
		id := firstReceipt(t, dbPath)
		if code, stdout, stderr := e4Run(t, bin, "receipt", "verify", "--config", cfgPath, id); code != 0 {
			t.Fatalf("baseline `receipt verify` = exit %d\n%s%s", code, stdout, stderr)
		}
		if _, err := rawLedger(t, dbPath).Exec(`ALTER TABLE action_schema RENAME COLUMN version TO v`); err != nil {
			t.Fatal(err)
		}
		before := e4Receipts(t, dbPath)
		want := e4ExpectedLine(t, cfgPath, dbPath)
		code, stdout, stderr := e4Run(t, bin, "receipt", "verify", "--config", cfgPath, id)
		if first := strings.SplitN(stdout, "\n", 2)[0]; code != 0 || first != want || !strings.Contains(stdout, "receipt "+id+" (seq 0): OK") {
			t.Fatalf("`receipt verify` = exit %d, first line %q\nstdout:\n%s\nstderr:\n%s\nwant exit 0, the line %q and the receipt OK", code, first, stdout, stderr, want)
		}
		if after := e4Receipts(t, dbPath); after != before {
			t.Fatal("the receipt rows changed")
		}
	})
	t.Run("TE15: the read fails", func(t *testing.T) {
		cfgPath, dbPath := seedChain(t, 1)
		id := firstReceipt(t, dbPath)
		if _, err := rawLedger(t, dbPath).Exec(`ALTER TABLE receipts RENAME COLUMN result_digest TO rd`); err != nil {
			t.Fatal(err)
		}
		before := e4Receipts(t, dbPath)
		want := e4ExpectedLine(t, cfgPath, dbPath)
		code, stdout, stderr := e4Run(t, bin, "receipt", "verify", "--config", cfgPath, id)
		if first := strings.SplitN(stdout, "\n", 2)[0]; code != 1 || first != want || strings.Contains(stdout, ": OK") ||
			!strings.HasPrefix(stderr, "korvun receipt verify: "+id+": ") || !strings.Contains(stderr, "result_digest") {
			t.Fatalf("`receipt verify` = exit %d, first line %q\nstdout:\n%s\nstderr:\n%s\nwant exit 1, the line %q and the read's failure naming result_digest", code, first, stdout, stderr, want)
		}
		if after := e4Receipts(t, dbPath); after != before {
			t.Fatal("the receipt rows changed")
		}
	})
}

// e4Arm is one dispatch arm that reaches an opener: its words, the rest of its
// arguments (valid enough to pass parsing and reach the opener) and whether it
// writes (the sealed helper, or the key rotation's own door).
type e4Arm struct {
	words  []string
	rest   func(cfg string) []string
	writes bool
}

// e4Arms are every dispatch arm of the plan's CLI inventory (O6–O8).
func e4Arms(t *testing.T) []e4Arm {
	t.Helper()
	v2File := writeIntentV2Fixture(t, t.TempDir())
	raw, err := os.ReadFile(v2File) //nolint:gosec // G304: the test's own fixture
	if err != nil {
		t.Fatal(err)
	}
	intent, err := action.ParseIntentContractV2(raw)
	if err != nil {
		t.Fatalf("the v2 fixture: %v", err)
	}
	grantFile := writeAuthorityGrantFile(t, authorityCLIRoot(intent))
	c := func(extra ...string) func(string) []string {
		return func(cfg string) []string { return append([]string{"--config", cfg}, extra...) }
	}
	return []e4Arm{
		{[]string{"ledger", "check"}, c(), false},
		{[]string{"receipt", "verify"}, c("rcpt_e4"), false},
		{[]string{"approvals", "list"}, c(), false},
		{[]string{"approvals", "show"}, c(e4ApprovalID), false},
		{[]string{"intent", "list"}, c(), false},
		{[]string{"intent", "show"}, c("int_e4"), false},
		{[]string{"intent", "verify-v2"}, c("int_e4", "1"), false},
		{[]string{"approvals", "approve"}, c(e4ApprovalID), true},
		{[]string{"approvals", "reject"}, c(e4ApprovalID), true},
		{[]string{"approvals", "execute"}, c(e4ApprovalID), true},
		{[]string{"grant", "issue"}, c("--intent", "int_e4", "--subject", "principal_e4", "--operations", "calc"), true},
		{[]string{"grant", "delegate"}, c("--parent", "grant_e4", "--subject", "principal_e4", "--operations", "calc"), true},
		{[]string{"grant", "revoke"}, c("grant_e4"), true},
		{[]string{"intent", "create"}, c("--purpose", "e4", "--operations", "calc"), true},
		{[]string{"intent", "activate"}, c("int_e4"), true},
		{[]string{"intent", "revoke"}, c("int_e4"), true},
		{[]string{"intent", "create-v2"}, c("--file", v2File), true},
		{[]string{"intent", "activate-v2"}, c("int_e4", "1"), true},
		{[]string{"intent", "expire-v2"}, c("int_e4", "1"), true},
		{[]string{"intent", "revoke-v2"}, c("int_e4", "1"), true},
		{[]string{"intent", "bind"}, c("--actor", "principal_e4", "--channel", "console", "int_e4", "1"), true},
		{[]string{"intent", "adopt-root"}, c("--profile", "profile_e4"), true},
		{[]string{"intent", "import-legacy"}, c("--profile", "profile_e4", "int_e4"), true},
		{[]string{"authority", "activate"}, c("--profile", "profile_e4", "--reason", "e4"), true},
		{[]string{"authority", "issue"}, c("--file", grantFile), true},
		{[]string{"authority", "admin-issue"}, c("--file", grantFile, "--reason", "e4"), true},
		{[]string{"authority", "delegate"}, c("--file", grantFile), true},
		{[]string{"authority", "admin-delegate"}, c("--file", grantFile, "--reason", "e4"), true},
		{[]string{"authority", "revoke"}, c("--reason", "e4", "grant_e4"), true},
		{[]string{"authority", "admin-revoke"}, c("--reason", "e4", "grant_e4"), true},
		{[]string{"authority", "import-v1"}, c("--file", grantFile, "--legacy-grant", "grant_e4", "--reason", "e4"), true},
		{[]string{"receipt", "rotate-key"}, c(), true},
	}
}

// e4Files is every byte of the files of a fixture's directory, by name,
// except what an open leaves by design: the -wal and -shm sidecars (created
// and removed around every connection) and the profile lock the key rotation
// takes. It is the «nothing written» oracle of a refused command.
func e4Files(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		name := info.Name()
		if strings.HasSuffix(name, "-wal") || strings.HasSuffix(name, "-shm") || name == "korvun.lock" {
			return nil
		}
		b, rerr := os.ReadFile(path) //nolint:gosec // G304: the test's own fixture directory
		if rerr != nil {
			return rerr
		}
		rel, _ := filepath.Rel(dir, path)
		out[rel] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// e4SameFiles fails unless two directory snapshots are identical.
func e4SameFiles(t *testing.T, label string, before, after map[string]string) {
	t.Helper()
	var names []string
	for k := range before {
		names = append(names, k)
	}
	for k := range after {
		if _, ok := before[k]; !ok {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		if before[n] != after[n] {
			t.Fatalf("%s: %s changed (%d → %d bytes)", label, n, len(before[n]), len(after[n]))
		}
	}
}

// TE47 · every dispatch arm of the CLI that reaches an opener, over a file the
// opener refuses — an existing empty bootstrap prefix (no store) and a text
// file (NOTADB) — exits 1 with its own fixed wrapper, the store's class and
// the native cause, and writes nothing: the fixture directory, the profile and
// the key material are byte for byte what they were. Over a current ledger
// with a renamed version column, every WRITING arm is refused by the existing
// signing or write path, named ledger_unreadable, before any mutation.
//
// PROBING MUTATIONS (MU47): one opener's outer classification omitted →
// reddens; one handler's wrapper drops the cause → reddens; one handler goes
// on after a failed open → reddens.
//
// Evidence level: the compiled binary in a separate OS process, real files.
func TestE4_TE47_everyOpenerFailureIsItsWrapperItsClassAndItsCause(t *testing.T) {
	bin := buildKorvunBinary(t)
	arms := e4Arms(t)
	fixtures := []struct {
		name   string
		make   func(t *testing.T) (cfgPath, dbPath string)
		class  string
		cause  string
		writes bool // only the writing arms reach this fixture's refusal
	}{
		{"an existing empty bootstrap prefix", func(t *testing.T) (string, string) {
			cfgPath, dbPath := e2PrefixConfig(t, 2)
			return cfgPath, dbPath
		}, "action/sqlite: no action store in the file", "not a korvun store", false},
		{"a text file", func(t *testing.T) (string, string) {
			cfgPath, dbPath := intentTestConfig(t)
			if err := os.WriteFile(dbPath, []byte(strings.Repeat("this file is not a SQLite database; it is text.\n", 128)), 0o600); err != nil {
				t.Fatal(err)
			}
			return cfgPath, dbPath
		}, "ledger_unreadable", "not a database", false},
		{"a renamed version column", func(t *testing.T) (string, string) {
			cfgPath, dbPath := seedChain(t, 1)
			if _, err := rawLedger(t, dbPath).Exec(`ALTER TABLE action_schema RENAME COLUMN version TO v`); err != nil {
				t.Fatal(err)
			}
			return cfgPath, dbPath
		}, "ledger_unreadable", "version", true},
	}
	for _, fx := range fixtures {
		for _, arm := range arms {
			if fx.writes && !arm.writes {
				continue
			}
			name := strings.Join(arm.words, " ")
			t.Run(fx.name+"/"+name, func(t *testing.T) {
				cfgPath, _ := fx.make(t)
				dir := filepath.Dir(cfgPath)
				before := e4Files(t, dir)
				args := append(append([]string(nil), arm.words...), arm.rest(cfgPath)...)
				code, stdout, stderr := e4Run(t, bin, args...)
				prefix := "korvun " + name + ": "
				if code != 1 || !strings.HasPrefix(stderr, prefix) || !strings.Contains(stderr, fx.class) || !strings.Contains(stderr, fx.cause) {
					t.Fatalf("`%s` = exit %d\nstdout:\n%s\nstderr:\n%s\nwant exit 1, %q then %q and %q", name, code, stdout, stderr, prefix, fx.class, fx.cause)
				}
				e4SameFiles(t, "`"+name+"`", before, e4Files(t, dir))
			})
		}
	}
}

// e4CrashEnv carries the file and the statements TE51's crashing child runs.
const (
	e4CrashDBEnv  = "KORVUN_E4_CRASH_DB"
	e4CrashSQLEnv = "KORVUN_E4_CRASH_SQL"
)

// TestE4_TE51_crashChild is not a mould: it is TE51's crashing process. It
// commits its statements through a connection it never closes and dies with
// exit 9, so their frames stay in the -wal file, never folded into the main
// file.
func TestE4_TE51_crashChild(t *testing.T) {
	path := os.Getenv(e4CrashDBEnv)
	if path == "" {
		t.Skip("batch 4's crashing child; run only by TE51")
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA wal_autocheckpoint = 0`); err != nil {
		t.Fatal(err)
	}
	for _, q := range strings.Split(os.Getenv(e4CrashSQLEnv), "\n") {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	_, _ = fmt.Fprintln(os.Stdout, "E4CRASH committed")
	os.Exit(9)
}

// e4Crash runs TE51's crashing child over dbPath with statements and waits
// for its death; the -wal must hold frames afterwards.
func e4Crash(t *testing.T, dbPath string, statements ...string) {
	t.Helper()
	// #nosec G204 -- TE51 deliberately re-executes this test binary.
	cmd := exec.Command(os.Args[0], "-test.run=^TestE4_TE51_crashChild$", "-test.count=1")
	cmd.Env = append(os.Environ(), e4CrashDBEnv+"="+dbPath, e4CrashSQLEnv+"="+strings.Join(statements, "\n"))
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	said := false
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		if sc.Text() == "E4CRASH committed" {
			said = true
		}
	}
	werr := cmd.Wait()
	var exitErr *exec.ExitError
	if !said || !errors.As(werr, &exitErr) || exitErr.ExitCode() != 9 {
		t.Fatalf("the crashing child: committed %v, wait %v; want its statements committed and exit 9", said, werr)
	}
	if info, err := os.Stat(dbPath + "-wal"); err != nil || info.Size() == 0 {
		t.Fatalf("the crashing child left no -wal frames: %v", err)
	}
}

// e4Copy copies src to dst.
func e4Copy(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src) //nolint:gosec // G304: the test's own files
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst) //nolint:gosec // G304: the test's own files
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

// e4Checkpoint folds every committed frame of path into its main file and
// truncates the -wal, through a raw connection that is then closed.
func e4Checkpoint(t *testing.T, path string) {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	var busy, logFrames, done int
	if err := raw.QueryRow(`PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &logFrames, &done); err != nil || busy != 0 || logFrames != done {
		t.Fatalf("checkpoint %s: %v busy=%d log=%d done=%d", path, err, busy, logFrames, done)
	}
}

// e4CheckLine is the compiled `ledger check`: its exit, its first line and
// everything it said.
func e4CheckLine(t *testing.T, bin, cfgPath string) (int, string, string) {
	t.Helper()
	code, stdout, stderr := e4Run(t, bin, "ledger", "check", "--config", cfgPath)
	return code, strings.SplitN(stdout, "\n", 2)[0], stdout + stderr
}

// e4RemoveSidecars removes the -wal and -shm of path, where they exist.
func e4RemoveSidecars(t *testing.T, path string) {
	t.Helper()
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
}

// e4HasTable reports whether path's catalog names table.
func e4HasTable(t *testing.T, path, table string) bool {
	t.Helper()
	var n int
	e4ReadClosed(t, path, func(raw *sql.DB) {
		if err := raw.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&n); err != nil {
			t.Fatal(err)
		}
	})
	return n == 1
}

// TE51 · the D2 restore, with every process stopped (the procedure is
// docs/operations/ledger-restore.md). After a clean offline single-file copy,
// a crashed process leaves an act of its own in the -wal, a consistent
// main/-wal set is taken, and a second crash commits the damage.
//   - The copy restored BESIDE the stale -wal and -shm gives the wrong history:
//     the damage comes back from the stale frames.
//   - The copy restored with -wal and -shm removed gives the copy's history —
//     the owner, the same receipts, the chain intact — and `ledger check` and
//     `receipt verify` exit 0.
//   - The set restored together gives the set's history: the act its -wal
//     held, without the damage.
//   - With no copy, the damaged files are set aside unchanged and Korvun starts
//     a new ledger: legacy_unfounded, none of the old history, never «ok».
//
// The key material is never touched.
//
// PROBING MUTATION (MU51): the opener remembers an unreadable verdict beside
// the file across reopens → the restored ledger still reads unreadable →
// reddens.
//
// Evidence level: the compiled binary in separate OS processes, crashed child
// processes, cold restarts.
func TestE4_TE51_theRestoreGivesTheCopysHistory(t *testing.T) {
	bin := buildKorvunBinary(t)
	cfgPath, dbPath := seedChain(t, 2)
	foundLedgerWithoutTheCLIPrincipal(t, cfgPath, dbPath)
	owner := app.ProfileIdentity(cfgPath)
	dir := filepath.Dir(dbPath)
	keysBefore := e4Files(t, filepath.Join(dir, "keys"))
	var founding string
	e4ReadClosed(t, dbPath, func(raw *sql.DB) {
		if err := raw.QueryRow(`SELECT receipt_id FROM receipts ORDER BY chain_seq DESC LIMIT 1`).Scan(&founding); err != nil {
			t.Fatal(err)
		}
	})
	// The clean offline single-file copy: every process stopped, every frame
	// folded into the main file.
	e4Checkpoint(t, dbPath)
	single := filepath.Join(t.TempDir(), "korvun.db")
	e4Copy(t, dbPath, single)
	copyReceipts := e4Receipts(t, single)
	// A crashed process leaves its own act in the -wal; the consistent set is
	// the main file and that -wal, taken together with nothing running.
	e4Crash(t, dbPath, `CREATE TABLE e4_after_the_copy (x INTEGER)`, `INSERT INTO e4_after_the_copy VALUES (1)`)
	setDir := t.TempDir()
	for _, suffix := range []string{"", "-wal"} {
		e4Copy(t, dbPath+suffix, filepath.Join(setDir, "korvun.db"+suffix))
	}
	// A second crash commits the damage; nothing reads the file before the
	// restores, so its frames stay in the -wal.
	e4Crash(t, dbPath, `ALTER TABLE action_schema RENAME COLUMN version TO v`)
	t.Run("the copy restored beside the stale -wal gives the wrong history", func(t *testing.T) {
		e4Copy(t, single, dbPath)
		code, first, out := e4CheckLine(t, bin, cfgPath)
		if !strings.HasPrefix(first, "ledger standing: ledger_unreadable (") {
			t.Fatalf("the copy beside the stale -wal: exit %d, %q\n%s\nwant the damage back from the stale frames", code, first, out)
		}
	})
	t.Run("the copy restored with -wal and -shm removed gives the copy's history", func(t *testing.T) {
		e4Copy(t, single, dbPath)
		e4RemoveSidecars(t, dbPath)
		code, first, out := e4CheckLine(t, bin, cfgPath)
		if code != 0 || first != "ledger standing: ok" || !strings.Contains(out, "(owner "+owner+";") || !strings.Contains(out, "chain intact") {
			t.Fatalf("the clean restore: exit %d, %q\n%s\nwant ok owned by this profile, the chain intact, exit 0", code, first, out)
		}
		if e4Receipts(t, dbPath) != copyReceipts || e4HasTable(t, dbPath, "e4_after_the_copy") {
			t.Fatal("the restored ledger is not the copy: other receipts, or the act after the copy")
		}
		if code, stdout, stderr := e4Run(t, bin, "receipt", "verify", "--config", cfgPath, founding); code != 0 || !strings.Contains(stdout, "receipt "+founding+" (seq ") || !strings.Contains(stdout, "): OK") {
			t.Fatalf("`receipt verify` of the founding receipt after the restore = exit %d\n%s%s", code, stdout, stderr)
		}
	})
	t.Run("a consistent main/-wal set restored together gives the set's history", func(t *testing.T) {
		e4RemoveSidecars(t, dbPath)
		for _, suffix := range []string{"", "-wal"} {
			e4Copy(t, filepath.Join(setDir, "korvun.db"+suffix), dbPath+suffix)
		}
		code, first, out := e4CheckLine(t, bin, cfgPath)
		if code != 0 || first != "ledger standing: ok" || !strings.Contains(out, "chain intact") || !e4HasTable(t, dbPath, "e4_after_the_copy") {
			t.Fatalf("the set restored together: exit %d, %q\n%s\nwant ok, the chain intact and the act its -wal held", code, first, out)
		}
	})
	t.Run("with no copy, the files are set aside and a new legacy ledger starts", func(t *testing.T) {
		// The damaged files first, as the operator would find them.
		e4Copy(t, single, dbPath)
		e4RemoveSidecars(t, dbPath)
		e4Crash(t, dbPath, `ALTER TABLE action_schema RENAME COLUMN version TO v`)
		archive := t.TempDir()
		kept := map[string]string{}
		for _, suffix := range []string{"", "-wal", "-shm"} {
			if _, err := os.Stat(dbPath + suffix); err != nil {
				continue
			}
			b, err := os.ReadFile(dbPath + suffix) //nolint:gosec // G304: the test's own file
			if err != nil {
				t.Fatal(err)
			}
			kept["korvun.db"+suffix] = string(b)
			if err := os.Rename(dbPath+suffix, filepath.Join(archive, "korvun.db"+suffix)); err != nil {
				t.Fatal(err)
			}
		}
		store, err := actionsqlite.OpenFor(dbPath, owner)
		if err != nil {
			t.Fatalf("Korvun starting without a copy: %v", err)
		}
		_ = store.Close()
		code, first, out := e4CheckLine(t, bin, cfgPath)
		if code != 0 || first != "ledger standing: legacy_unfounded" || !strings.Contains(out, "0 receipts, chain intact") {
			t.Fatalf("the new ledger: exit %d, %q\n%s\nwant legacy_unfounded, no receipts, exit 0", code, first, out)
		}
		var archived = map[string]string{}
		for name := range kept {
			b, err := os.ReadFile(filepath.Join(archive, name)) //nolint:gosec // G304: the test's own file
			if err != nil {
				t.Fatal(err)
			}
			archived[name] = string(b)
		}
		e4SameFiles(t, "the set-aside files", kept, archived)
	})
	e4SameFiles(t, "the key material", keysBefore, e4Files(t, filepath.Join(dir, "keys")))
}

// TE51, the procedure written down · docs/operations/ledger-restore.md says
// what the mould above does and what D2 cannot fit in a screen: the shared
// file also holds the conversations; every process that uses the profile is
// stopped first; a single-file copy goes back with -wal and -shm removed, a
// main/-wal set goes back together, and files from different moments are
// never mixed; the damaged files and the key material are kept; with no copy,
// Korvun starts a new ledger that is legacy_unfounded, not founded.
//
// Evidence level: an editorial check of the repository's own document.
func TestE4_TE51_theRestoreProcedureIsWrittenDown(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "operations", "ledger-restore.md"))
	if err != nil {
		t.Fatalf("the restore procedure: %v", err)
	}
	doc := string(raw)
	for _, must := range []string{
		"also holds this profile's conversations",
		"Stop every process that uses the profile",
		"-wal",
		"-shm",
		"restore the main file and its -wal together",
		"Never mix files from different moments",
		"Keep the damaged files",
		"keys",
		"legacy_unfounded",
	} {
		if !strings.Contains(doc, must) {
			t.Fatalf("the restore procedure does not say %q", must)
		}
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package testgates

// The fixed-date gate (2026-10-03). A test that hands a fixed calendar date to
// code which reads the wall clock changes its verdict on the day the calendar
// passes that date. operator_act_test.go went red that way on 2026-09-15, and
// on 2026-09-30 the parent grant that internal/cli's issueGrantID issued
// expired and took two delegation tests down on every tree. Expiries in tests
// come from the clock; this gate hunts only the class that went off twice.
//
// It reads every *_test.go file under the repository root (testFiles names the
// directories it never enters) and sees exactly two shapes, the year being the
// current UTC year or later:
//   - a string literal holding a match of rfc3339Instant;
//   - a call written time.Date(<integer literal year>, …).
//
// Each one fails the gate unless an entry of fixedDateExceptions excuses that
// date in that file, and an entry whose reason is empty or blank fails it too.
// A comment excuses nothing. This file is not judged: it holds the exception
// list, whose keys are dates by construction. Out of scope, declared: a date
// computed in a variable, and any other spelling of a date.

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// repoRoot is the repository root seen from this package's directory, where
// go test runs its tests.
var repoRoot = filepath.Join("..", "..")

// selfFile is this file, as the walk lists it.
const selfFile = "internal/testgates/fixed_dates_test.go"

// pastFixture is the reason the exceptions written with this gate give unless
// they give their own: each of those dates was measured already past on
// 2026-10-03, when the gate was written.
const pastFixture = "historical fixture: a date already past when this gate was written (2026-10-03)"

// fixedDateExceptions are the dates of the current year or later that the gate
// excuses, each in its own file and for its written reason. A date is keyed as
// the gate prints it: the RFC3339 match, or the time.Date call with its
// arguments as written.
var fixedDateExceptions = []fixedDateException{
	{file: "internal/action/approval_test.go", reason: pastFixture, dates: []string{
		"time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)",
		"time.Date(2026, 8, 31, 10, 30, 0, 0, time.UTC)",
		"time.Date(2026, 8, 31, 10, 59, 59, 0, time.UTC)",
		"time.Date(2026, 8, 31, 11, 0, 0, 0, time.UTC)",
	}},
	{file: "internal/action/attenuation_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)"}},
	{file: "internal/action/authority_contract_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)"}},
	{file: "internal/action/authority_phase3_test_helpers_test.go", reason: pastFixture, dates: []string{
		"time.Date(2026, 9, 21, 8, 0, 0, 0, time.UTC)",
		"time.Date(2026, 9, 22, 8, 0, 0, 0, time.UTC)",
	}},
	{file: "internal/action/authorization_snapshot_digest_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)"}},
	{file: "internal/action/bound_r12_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)"}},
	{file: "internal/action/bound_test.go", reason: pastFixture, dates: []string{
		"time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)",
		"time.Date(2026, 9, 2, 10, 0, 1, 0, time.UTC)",
	}},
	{file: "internal/action/contract_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)"}},
	{file: "internal/action/envelope_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 8, 30, 12, 0, 0, 0, time.FixedZone(\"CEST\", 2*3600))"}},
	{file: "internal/action/executor/authority_strict_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 9, 21, 13, 30, 0, 0, time.UTC)"}},
	{file: "internal/action/executor/executor_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)"}},
	{file: "internal/action/executor/identity_phase1_test.go", reason: pastFixture, dates: []string{
		"time.Date(2026, 9, 21, 13, 0, 0, 0, time.UTC)",
		"time.Date(2026, 9, 21, 13, 30, 0, 0, time.UTC)",
	}},
	{file: "internal/action/identity_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)"}},
	{file: "internal/action/intent_v2_test.go", reason: pastFixture, dates: []string{
		"2026-09-19T12:00:00Z",
		"2026-09-20T12:00:00Z",
		"time.Date(2026, 9, 19, 12, 1, 0, 0, time.UTC)",
	}},
	{file: "internal/action/receipt_test.go", reason: pastFixture, dates: []string{
		"time.Date(2026, 8, 30, 14, 0, 0, 0, time.UTC)",
		"time.Date(2026, 8, 30, 14, 0, 1, 0, time.UTC)",
	}},
	{file: "internal/action/receipt_v2_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)"}},
	{file: "internal/action/sqlite/approvals_rowmoved_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)"}},
	{file: "internal/action/sqlite/approvals_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 8, 30, 9, 0, 0, 0, time.UTC)"}},
	{file: "internal/action/sqlite/approvals_v15_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)"}},
	{file: "internal/action/sqlite/authority_administrative_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)"}},
	{file: "internal/action/sqlite/authority_as11_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 9, 21, 16, 0, 0, 0, time.UTC)"}},
	{file: "internal/action/sqlite/contracts_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)"}},
	{file: "internal/action/sqlite/evidence_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)"}},
	{file: "internal/action/sqlite/identity_phase1_test.go", reason: pastFixture, dates: []string{
		"time.Date(2026, 9, 19, 1, 2, 3, 0, time.UTC)",
		"time.Date(2026, 9, 21, 16, 0, 0, 0, time.UTC)",
		"time.Date(2026, 9, 21, 20, 0, 0, 0, time.UTC)",
	}},
	{file: "internal/action/sqlite/intent_v2_test.go", reason: pastFixture, dates: []string{
		"2026-09-19T12:00:00Z",
		"2026-09-20T12:00:00Z",
		"time.Date(2026, 9, 19, 11, 0, 0, 0, time.UTC)",
		"time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)",
		"time.Date(2026, 9, 19, 12, 1, 0, 0, time.UTC)",
		"time.Date(2026, 9, 19, 12, 2, 0, 0, time.UTC)",
	}},
	{file: "internal/action/sqlite/ledger_e1_persistent_test.go", reason: pastFixture, dates: []string{"2026-09-26T00:00:00Z"}},
	{file: "internal/action/sqlite/ledger_e2_residue_test.go", reason: pastFixture, dates: []string{"2026-09-26T00:00:00Z"}},
	{file: "internal/action/sqlite/ledger_hook_test.go", reason: pastFixture, dates: []string{"2026-09-25T00:00:00Z"}},
	{file: "internal/action/sqlite/ledger_identity_test.go", reason: pastFixture, dates: []string{"2026-09-24T00:00:00Z"}},
	{file: "internal/action/sqlite/ledger_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 8, 30, 16, 0, 0, 0, time.UTC)"}},
	{file: "internal/action/sqlite/migration_r8z1_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 9, 3, 1, 2, 3, 0, time.UTC)"}},
	{file: "internal/action/sqlite/migration_r9w1_test.go", reason: pastFixture, dates: []string{
		"2026-09-03T01:02:03Z",
		"2026-09-03T04:05:06Z",
	}},
	{file: "internal/action/sqlite/migration_test.go", reason: pastFixture, dates: []string{
		"2026-08-29T10:00:00Z",
		"2026-08-29T10:00:01Z",
	}},
	{file: "internal/action/sqlite/migration_v3_test.go", reason: pastFixture, dates: []string{"2026-08-30T00:00:00Z"}},
	{file: "internal/action/sqlite/recovery_test.go", reason: pastFixture, dates: []string{
		"time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)",
		"time.Date(2026, 8, 30, 11, 0, 0, 0, time.UTC)",
	}},
	{file: "internal/action/sqlite/repair_r11_test.go", reason: pastFixture, dates: []string{"2026-09-01T10:20:30Z"}},
	{file: "internal/action/sqlite/signing_keys_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 8, 30, 15, 0, 0, 0, time.UTC)"}},
	{file: "internal/action/sqlite/store_c6_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)"}},
	{file: "internal/action/sqlite/store_test.go", reason: pastFixture, dates: []string{
		"time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)",
		"time.Date(2026, 8, 30, 10, 0, 5, 0, time.UTC)",
	}},
	{file: "internal/action/sqlite/tombstone_r12_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)"}},
	{file: "internal/action/sqlite/tombstone_r13_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)"}},
	{file: "internal/action/validity_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)"}},
	{file: "internal/action/wire_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)"}},
	{file: "internal/app/app_identity_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)"}},
	{file: "internal/app/approvals_adapter_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)"}},
	{file: "internal/app/ledger_e1_boot_test.go", reason: pastFixture, dates: []string{"2026-09-26T00:00:00Z"}},
	{file: "internal/app/ledger_e3_boot_test.go", reason: pastFixture, dates: []string{"2026-09-03T01:02:03Z"}},
	{file: "internal/app/v0151b_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 9, 19, 10, 0, 0, 0, time.UTC)"}},
	{file: "internal/brain/agent_identity_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)"}},
	{file: "internal/brain/agent_options_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)"}},
	{file: "internal/channel/discord/gateway_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)"}},
	{file: "internal/channel/telegram/identity_phase1_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)"}},
	{file: "internal/channel/webhook/identity_cures_phase1_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 9, 21, 14, 0, 0, 0, time.UTC)"}},
	{file: "internal/channel/webhook/identity_phase1_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 9, 21, 15, 0, 0, 0, time.UTC)"}},
	{file: "internal/cli/grant_test.go", reason: "historical fixture: the window of an intent built to be expired; the CLI compares it with the real clock, which must find it past", dates: []string{
		"2026-08-01T00:00:00Z",
		"2026-08-02T00:00:00Z",
	}},
	{file: "internal/cli/receipt_r11_test.go", reason: pastFixture, dates: []string{
		"2026-09-03T01:02:03Z",
		"time.Date(2026, 9, 3, 1, 2, 3, 0, time.UTC)",
		"time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)",
	}},
	{file: "internal/cli/repair_procedure_r13_test.go", reason: pastFixture, dates: []string{"2026-09-04T10:00:00Z"}},
	{file: "internal/controlapi/approvals_test.go", reason: pastFixture, dates: []string{"2026-09-08T13:00:00Z"}},
	{file: "internal/controlapi/identity_phase1_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)"}},
	{file: "internal/conversation/sqlite/sqlite_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 6, 21, 15, 4, 5, 123456789, time.FixedZone(\"x\", 2*3600))"}},
	{file: "internal/identity/identity_phase1_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)"}},
	{file: "internal/liveview/liveview_test.go", reason: pastFixture, dates: []string{"time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)"}},
	{file: "internal/router/identity_phase1_test.go", reason: pastFixture, dates: []string{
		"time.Date(2026, 9, 21, 15, 0, 0, 0, time.UTC)",
		"time.Date(2026, 9, 21, 15, 30, 0, 0, time.UTC)",
	}},
	{file: "internal/router/session_sp2_test.go", reason: pastFixture, dates: []string{
		"time.Date(2026, 8, 7, 23, 0, 0, 0, time.UTC)",
		"time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)",
		"time.Date(2026, 8, 8, 3, 59, 0, 0, time.UTC)",
		"time.Date(2026, 8, 8, 4, 1, 0, 0, time.UTC)",
		"time.Date(2026, 8, 8, 9, 0, 0, 0, time.UTC)",
	}},
	{file: "internal/tool/builtin_test.go", reason: pastFixture, dates: []string{
		"2026-06-27T12:00:00Z",
		"time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)",
	}},
}

// TestNoFixedDateOfThisYearOrLater is the gate over the repository's test
// files.
//
// PROBING MUTATIONS: the fixed date of the base put back in issueGrantID
// (internal/cli/grant_test.go) → reddens, naming that file and
// 2026-09-30T00:00:00Z, while the two delegation tests answer
// authority_expired; one exception's reason blanked → reddens on «has no
// reason»; the walk listing nothing → reddens on «did not read»; this file
// judged like the others → reddens on its own exception keys.
//
// Evidence level: in process, over the test files as they are on disk
// (go/parser; nothing is compiled or run).
func TestNoFixedDateOfThisYearOrLater(t *testing.T) {
	files, err := testFiles(repoRoot)
	if err != nil {
		t.Fatalf("walk %s: %v", repoRoot, err)
	}
	for _, want := range []string{"internal/cli/grant_test.go", selfFile} {
		if !slices.Contains(files, want) {
			t.Fatalf("the walk did not read %s (%d files): the gate would judge the wrong tree", want, len(files))
		}
	}
	year := time.Now().UTC().Year()
	var found []fixedDate
	for _, f := range files {
		if f == selfFile {
			continue
		}
		dates, err := scanFile(repoRoot, f, year)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		found = append(found, dates...)
	}
	for _, e := range reasonless(fixedDateExceptions) {
		t.Errorf("fixedDateExceptions: the entry for %s has no reason", e.file)
	}
	for _, d := range unexcused(found, fixedDateExceptions) {
		t.Errorf("%s:%d: %s — a fixed date of %d or later: take it from the clock, or excuse it in fixedDateExceptions with its reason", d.file, d.line, d.key, year)
	}
}

// TestFixedDateScan_seesExactlyTheTwoShapes holds the scan to each shape it
// claims, and to the shapes it must let through.
//
// PROBING MUTATIONS, each red on its own rows: the year threshold raised by
// one; the threshold lowered by one; string literals not read; time.Date not
// read; the year of time.Date read from its day.
//
// Evidence level: unit, in process; the scan over source built here, its
// years taken from the clock at run time.
func TestFixedDateScan_seesExactlyTheTwoShapes(t *testing.T) {
	year := time.Now().UTC().Year()
	fill := strings.NewReplacer("THIS", strconv.Itoa(year), "NEXT", strconv.Itoa(year+1), "LAST", strconv.Itoa(year-1))
	cases := []struct {
		name, body string
		want       []string
	}{
		{"an instant of this year", `_ = "THIS-01-02T03:04:05Z"`, []string{"THIS-01-02T03:04:05Z"}},
		{"an instant of a later year", `_ = "NEXT-01-02T03:04:05Z"`, []string{"NEXT-01-02T03:04:05Z"}},
		{"an instant with an offset", `_ = "THIS-01-02T03:04:05+02:00"`, []string{"THIS-01-02T03:04:05+02:00"}},
		{"an instant inside a longer string", `_ = "--expires=THIS-01-02T03:04:05Z"`, []string{"THIS-01-02T03:04:05Z"}},
		{"an instant inside a raw string", "_ = `{\"expires_at\":\"THIS-01-02T03:04:05Z\"}`", []string{"THIS-01-02T03:04:05Z"}},
		{"two instants in one string", `_ = "THIS-01-02T03:04:05Z/NEXT-01-02T03:04:05Z"`, []string{"THIS-01-02T03:04:05Z", "NEXT-01-02T03:04:05Z"}},
		{"an instant of last year is not seen", `_ = "LAST-12-31T23:59:59Z"`, nil},
		{"a date without a time is not seen", `_ = "THIS-01-02"`, nil},
		{"a time without seconds is not seen", `_ = "THIS-01-02T03:04Z"`, nil},
		{"a time without a zone is not seen", `_ = "THIS-01-02T03:04:05"`, nil},
		{"an instant in a comment is not seen", "// THIS-01-02T03:04:05Z\n\t_ = 0", nil},
		{"time.Date of this year", `_ = time.Date(THIS, 1, 2, 3, 4, 5, 0, time.UTC)`, []string{"time.Date(THIS, 1, 2, 3, 4, 5, 0, time.UTC)"}},
		{"time.Date of last year is not seen", `_ = time.Date(LAST, 12, 31, 0, 0, 0, 0, time.UTC)`, nil},
		{"time.Date with this year as its day is not seen", `_ = time.Date(LAST, 1, THIS, 0, 0, 0, 0, time.UTC)`, nil},
		{"time.Date with the year in a variable is out of scope", `y := THIS; _ = time.Date(y, 1, 2, 3, 4, 5, 0, time.UTC)`, nil},
		{"time.Date across lines is keyed on one line", "_ = time.Date(\n\t\tTHIS, 1, 2,\n\t\t3, 4, 5, 0, time.UTC)", []string{"time.Date(THIS, 1, 2, 3, 4, 5, 0, time.UTC)"}},
	}
	for _, c := range cases {
		src := "package p\n\nimport \"time\"\n\nvar _ = time.UTC\n\nfunc f() {\n\t" + fill.Replace(c.body) + "\n}\n"
		dates, err := scanSource("case_test.go", []byte(src), year)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		var got, want []string
		for _, d := range dates {
			got = append(got, d.key)
		}
		for _, w := range c.want {
			want = append(want, fill.Replace(w))
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s: got %q, want %q", c.name, got, want)
		}
	}
}

// TestFixedDateExceptions_excuseOnlyTheirOwnDates: an exception excuses the
// dates it lists in its own file only, and an entry whose reason is empty or
// blank is named.
//
// PROBING MUTATIONS: matching by file alone → reddens on the other date of the
// same file; matching by date alone → reddens on the same date in another
// file; the reason check neutralized → reddens on both entries without a
// reason; blank reasons taken for reasons → reddens on the blank one.
//
// Evidence level: unit, in process.
func TestFixedDateExceptions_excuseOnlyTheirOwnDates(t *testing.T) {
	at := func(file, key string) fixedDate { return fixedDate{file: file, line: 1, key: key} }
	exceptions := []fixedDateException{{file: "a_test.go", reason: "a reason", dates: []string{"K1"}}}
	found := []fixedDate{at("a_test.go", "K1"), at("a_test.go", "K2"), at("b_test.go", "K1")}
	if got, want := unexcused(found, exceptions), []fixedDate{at("a_test.go", "K2"), at("b_test.go", "K1")}; !slices.Equal(got, want) {
		t.Errorf("unexcused: got %v, want %v", got, want)
	}
	entries := []fixedDateException{{file: "empty", reason: ""}, {file: "blank", reason: " \t"}, {file: "kept", reason: "a reason"}}
	var named []string
	for _, e := range reasonless(entries) {
		named = append(named, e.file)
	}
	if want := []string{"empty", "blank"}; !slices.Equal(named, want) {
		t.Errorf("reasonless: got %q, want %q", named, want)
	}
}

// TestTestFiles_listsTestFilesAndPrunesThree: the walk lists the *_test.go
// files of a tree built here, as slash paths from its root, and never enters a
// directory named node_modules, .git or design-drafts.
//
// PROBING MUTATIONS: each pruned name dropped in turn → reddens on its file;
// the suffix check dropped → reddens on a plain .go file.
//
// Evidence level: unit, in process, over a temporary directory.
func TestTestFiles_listsTestFilesAndPrunesThree(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"a/x_test.go", "a/y.go", "b/c/v_test.go", "node_modules/p/z_test.go", ".git/q_test.go", "design-drafts/c/w_test.go", "d/node_modules/u_test.go"} {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("package p\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := testFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a/x_test.go", "b/c/v_test.go"}; !slices.Equal(got, want) {
		t.Errorf("testFiles: got %q, want %q", got, want)
	}
}

// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.15.1 block B — the CLI moulds. RED: written before any cure.
//
// P2-2: -h/--help on the approvals, ledger and receipt families is a QUERY
// (ADR-0032 "Exit codes": usage to stdout, exit 0, stderr empty), the same
// contract parseStyled gives serve and config check. Today every verb of the
// three families parses with a bare fs.Parse whose output is stderr and whose
// failure returns 2, so a help request is answered as a usage error.
//
// P2-1: an approval id is judged by its SHAPE at the door ("apr_" + 32
// lowercase hex, the form action.NewApprovalID mints) before any store is
// opened, and whatever the CLI prints back of it carries no raw invisible.
//
// Sister (docs/HANDOFF.md, «Hermana de la P1-2»): `approvals show` prints the
// raw parameters without the escape alphabet the screen uses.
//
// Evidence level: in-process CLI (Run over buffers) against a REAL store file.
// Not a compiled binary in a separate OS process.

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/app"
	"github.com/Sebastian197/korvun/internal/config"
)

// TestV0151B_P2_2_helpIsAQueryOnEveryOperatorVerb walks every verb of the
// operator families — approvals, ledger, receipt, and the intent and grant
// families ADR-0032's «on any subcommand» also covers — under the three help
// spellings. Each row must answer exit 0, print a usage that names --config on
// stdout, and leave stderr empty.
//
// Evidence: in-process CLI (Run over buffers).
// Probing mutation (planned): route one verb back to the bare fs.Parse ⇒ its
// rows redden.
func TestV0151B_P2_2_helpIsAQueryOnEveryOperatorVerb(t *testing.T) {
	t.Parallel()
	verbs := [][]string{
		{"approvals", "list"},
		{"approvals", "show"},
		{"approvals", "approve"},
		{"approvals", "reject"},
		{"approvals", "execute"},
		{"ledger", "check"},
		{"receipt", "verify"},
		{"receipt", "rotate-key"},
		{"intent", "create"},
		{"intent", "activate"},
		{"intent", "revoke"},
		{"intent", "list"},
		{"intent", "show"},
		{"grant", "issue"},
		{"grant", "delegate"},
		{"grant", "revoke"},
	}
	for _, verb := range verbs {
		// -help is the third spelling flag.ErrHelp answers; ADR-0032 names
		// -h/--help, and a verb that honours those two through the shared
		// helper honours -help by construction — so it is pinned, not argued.
		for _, form := range []string{"-h", "--help", "-help"} {
			args := append(append([]string{}, verb...), form)
			t.Run(strings.Join(args, " "), func(t *testing.T) {
				t.Parallel()
				code, stdout, stderr := runIntentCLI(t, args...)
				if code != 0 {
					t.Fatalf("exit = %d, want 0 — help is a query (ADR-0032), never the usage-error path", code)
				}
				if !strings.Contains(stdout, "config") {
					t.Fatalf("the verb's usage (naming its --config flag) must reach stdout; stdout = %q", stdout)
				}
				if stderr != "" {
					t.Fatalf("help is a query: stderr must stay empty, got %q", stderr)
				}
			})
		}
	}
}

// TestV0151B_P2_2_helpIsAQueryOnEveryFamilyNoun is the same contract one level
// up: `korvun approvals -h` today reads -h as an unknown subcommand and exits 2.
// The noun's help must list its verbs on stdout.
//
// Evidence: in-process CLI (Run over buffers).
// Probing mutation (planned): drop the noun-level help branch ⇒ its rows red.
func TestV0151B_P2_2_helpIsAQueryOnEveryFamilyNoun(t *testing.T) {
	t.Parallel()
	nouns := map[string][]string{
		"approvals": {"list", "show", "approve", "reject", "execute"},
		"ledger":    {"check"},
		"receipt":   {"verify", "rotate-key"},
		"intent":    {"create", "activate", "revoke", "list", "show"},
		"grant":     {"issue", "delegate", "revoke"},
	}
	for noun, sub := range nouns {
		for _, form := range []string{"-h", "--help"} {
			t.Run(noun+" "+form, func(t *testing.T) {
				t.Parallel()
				code, stdout, stderr := runIntentCLI(t, noun, form)
				if code != 0 {
					t.Fatalf("exit = %d, want 0", code)
				}
				for _, v := range sub {
					if !strings.Contains(stdout, v) {
						t.Fatalf("the %s usage must name its verb %q on stdout; stdout = %q", noun, v, stdout)
					}
				}
				if stderr != "" {
					t.Fatalf("stderr must stay empty, got %q", stderr)
				}
			})
		}
	}
}

// malformedApprovalIDs are ids no mint can produce. Each is refused by shape.
func malformedApprovalIDs() map[string]string {
	hex := strings.Repeat("ab", 16)
	return map[string]string{
		"quote injected":   "apr_" + hex + `","x":"y`,
		"word joiner":      "apr_" + hex + "⁠",
		"bidi override":    "apr_‮" + hex,
		"C1 control":       "apr_" + hex + "",
		"upper-case stem":  "APR_" + hex,
		"upper-case hex":   "apr_" + strings.ToUpper(hex),
		"short":            "apr_" + hex[:30],
		"path traversal":   "apr_" + hex + "/../x",
		"action namespace": "act_" + hex,
	}
}

// TestV0151B_P2_1_aMalformedApprovalIDIsRefusedByShapeBeforeTheStore feeds every
// malformed id to every verb that takes one. The outcome is the usage error
// (exit 2) naming the approval id, and stderr never echoes a raw invisible.
// The row count checked at the end only proves nothing was WRITTEN; «before
// the store is opened» is proved by TestV0151B_P2_1_theShapeIsJudgedBeforeAnyStoreOpen.
//
// Today each verb opens the store, fails to find the row and exits 1.
//
// Evidence: in-process CLI against a real store file.
// Probing mutation (planned): remove the shape check from one verb ⇒ its rows
// answer 1 and redden.
func TestV0151B_P2_1_aMalformedApprovalIDIsRefusedByShapeBeforeTheStore(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, _ := parkedRequest(t)
	before := countRows(t, dbPath, "actions")
	for _, verb := range []string{"show", "approve", "reject", "execute"} {
		for name, id := range malformedApprovalIDs() {
			t.Run(verb+"/"+name, func(t *testing.T) {
				code, stdout, stderr := runIntentCLI(t, "approvals", verb, "--config", cfgPath, id)
				if code != 2 {
					t.Fatalf("exit = %d, want 2 — a malformed id is a usage error judged at the door", code)
				}
				if stdout != "" {
					t.Fatalf("stdout must stay empty, got %q", stdout)
				}
				for _, raw := range []string{"⁠", "‮", ""} {
					if strings.Contains(stderr, raw) {
						t.Fatalf("stderr echoes a raw invisible (%U): %q", []rune(raw)[0], stderr)
					}
				}
				if !strings.Contains(stderr, "approval id") {
					t.Fatalf("stderr must name what was refused (the approval id); got %q", stderr)
				}
			})
		}
	}
	if after := countRows(t, dbPath, "actions"); after != before {
		t.Fatalf("actions rows %d -> %d: a refusal at the door writes nothing", before, after)
	}
}

// TestV0151B_sister_showEscapesTheRawParameters is the HANDOFF sister of P2-1:
// two requests whose parameters differ only by U+2060 seal different digests
// and must not print the same text. The escape alphabet is the screen's
// (`<U+XXXX>`, and '<' itself as <U+003C>), so the two surfaces read alike.
//
// Evidence: in-process CLI against a real store file.
// Probing mutation (planned): print string(params) raw again ⇒ this reddens.
func TestV0151B_sister_showEscapesTheRawParameters(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath := intentTestConfig(t)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	law, err := app.PolicyPinFor(cfg, "a")
	if err != nil {
		t.Fatalf("law: %v", err)
	}
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	raw := "pagar⁠100 EUR <U+2060>"
	env := action.NewEnvelope("act_escape1", "env-escape",
		action.Source{Kind: "agent_brain", Protocol: "text", Channel: "console"},
		action.Operation{Namespace: "tool", Name: "calc", Version: 1},
		raw, time.Now().UTC())
	env.IntentID = action.RootIntentID
	env.Principal = action.PrincipalRef{PrincipalID: "principal_brain_a"}
	env.Effect = action.Effect{Class: string(action.EffectPure)}
	now := time.Now().UTC()
	b, err := action.NewBoundApprovalRequest(env, raw, action.ApprovalContext{
		IntentPurpose: "escape", GrantID: "-", CostLine: "unbudgeted", ToolCage: "calc",
		Descriptor:    action.EffectDescriptor{Class: action.EffectPure, Reversible: true},
		HasDescriptor: true, LawVersion: law.Version, LawDigest: law.Digest,
		Rule: "require_approval", Now: now, TTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if err := store.CreateApprovalRequest(context.Background(), b); err != nil {
		t.Fatalf("park: %v", err)
	}
	_ = store.Close()

	code, stdout, stderr := runIntentCLI(t, "approvals", "show", "--config", cfgPath, b.Approval().ApprovalID)
	if code != 0 {
		t.Fatalf("show: %d %q", code, stderr)
	}
	if strings.Contains(stdout, "⁠") {
		t.Fatalf("the raw U+2060 reached the terminal: %q", stdout)
	}
	if !strings.Contains(stdout, "pagar<U+2060>100 EUR <U+003C>U+2060>") {
		t.Fatalf("the parameters must print in the screen's escape alphabet; stdout = %q", stdout)
	}
}

// unopenableStoreConfig writes a profile whose storage path is a DIRECTORY:
// any attempt to open the store fails, and every verb that tries answers 1.
func unopenableStoreConfig(t *testing.T) string {
	t.Helper()
	cfgPath, _ := intentTestConfig(t)
	raw, err := os.ReadFile(cfgPath) // #nosec G304 -- t.TempDir-controlled test path
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	cfg["storage"] = map[string]any{"path": t.TempDir()}
	out, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(cfgPath, out, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return cfgPath
}

// TestV0151B_P2_1_theShapeIsJudgedBeforeAnyStoreOpen is the oracle by
// impossibility for «before the store»: the store cannot be opened at all, so a
// cure that opens (or queries) it first answers 1. The malformed id must answer
// 2 with the named message; the well-formed control proves the store really is
// unopenable (it answers 1).
//
// Evidence: in-process CLI; the store path is a directory on the real disk.
// Probing mutation (planned): move the shape check after the open ⇒ red.
func TestV0151B_P2_1_theShapeIsJudgedBeforeAnyStoreOpen(t *testing.T) {
	t.Parallel()
	cfgPath := unopenableStoreConfig(t)
	good := "apr_" + strings.Repeat("ab", 16)
	for _, verb := range []string{"show", "approve", "reject", "execute"} {
		t.Run(verb, func(t *testing.T) {
			if code, _, _ := runIntentCLI(t, "approvals", verb, "--config", cfgPath, good); code != 1 {
				t.Fatalf("control: well-formed id exit = %d, want 1 (the store must be unopenable)", code)
			}
			code, _, stderr := runIntentCLI(t, "approvals", verb, "--config", cfgPath, "apr_x\u202e")
			if code != 2 || !strings.Contains(stderr, "approval id") {
				t.Fatalf("malformed id: exit %d stderr %q, want 2 naming the approval id", code, stderr)
			}
		})
	}
}

// TestV0151B_P2_1_sister_showAndListEscapeEveryUntrustedField: `approvals show`
// prints the purpose raw, and `approvals list` prints the id and the risk
// summary raw. Each is a stored string no human typed into this terminal.
//
// Evidence: in-process CLI against a real store file; the list-side bytes are
// written through a second real connection.
// Probing mutation (planned): print one of the three fields raw ⇒ its row red.
func TestV0151B_P2_1_sister_showAndListEscapeEveryUntrustedField(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, approvalID := parkedRequest(t)
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
	if err != nil {
		t.Fatalf("second connection: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`UPDATE approvals SET risk_summary = risk_summary || char(8238) WHERE approval_id = ?`, approvalID); err != nil {
		t.Fatalf("attack risk_summary: %v", err)
	}
	if _, err := db.Exec(`UPDATE approvals SET approval_id = approval_id || char(8238) WHERE approval_id = ?`, approvalID); err != nil {
		t.Fatalf("attack approval_id: %v", err)
	}
	code, stdout, stderr := runIntentCLI(t, "approvals", "list", "--config", cfgPath)
	if code != 0 {
		t.Fatalf("list: %d %q", code, stderr)
	}
	if strings.Contains(stdout, "\u202e") {
		t.Fatalf("list printed a raw U+202E: %q", stdout)
	}
	if strings.Count(stdout, "<U+202E>") != 2 {
		t.Fatalf("list must escape BOTH the id and the risk summary; stdout = %q", stdout)
	}
}

// TestV0151B_P2_1_sister_showEscapesThePurpose.
//
// Evidence: in-process CLI against a real store file.
// Probing mutation (planned): print p.IntentPurpose raw ⇒ red.
func TestV0151B_P2_1_sister_showEscapesThePurpose(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath := intentTestConfig(t)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	law, err := app.PolicyPinFor(cfg, "a")
	if err != nil {
		t.Fatalf("law: %v", err)
	}
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	env := action.NewEnvelope("act_purpose1", "env-purpose",
		action.Source{Kind: "agent_brain", Protocol: "text", Channel: "console"},
		action.Operation{Namespace: "tool", Name: "calc", Version: 1},
		`7*6`, time.Now().UTC())
	env.IntentID = action.RootIntentID
	env.Principal = action.PrincipalRef{PrincipalID: "principal_brain_a"}
	env.Effect = action.Effect{Class: string(action.EffectPure)}
	now := time.Now().UTC()
	b, err := action.NewBoundApprovalRequest(env, `7*6`, action.ApprovalContext{
		IntentPurpose: "pagar a\u202eproveedor", GrantID: "-", CostLine: "unbudgeted", ToolCage: "calc",
		Descriptor:    action.EffectDescriptor{Class: action.EffectPure, Reversible: true},
		HasDescriptor: true, LawVersion: law.Version, LawDigest: law.Digest,
		Rule: "require_approval", Now: now, TTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if err := store.CreateApprovalRequest(context.Background(), b); err != nil {
		t.Fatalf("park: %v", err)
	}
	_ = store.Close()
	code, stdout, stderr := runIntentCLI(t, "approvals", "show", "--config", cfgPath, b.Approval().ApprovalID)
	if code != 0 {
		t.Fatalf("show: %d %q", code, stderr)
	}
	if strings.Contains(stdout, "\u202e") || !strings.Contains(stdout, "pagar a<U+202E>proveedor") {
		t.Fatalf("the purpose must print escaped; stdout = %q", stdout)
	}
}

// markCase describes how one mark is produced through the RECOVERY door with
// the CLI's own sealed store: the approval row is made unusable and REPAIRED
// after the seal (column + bad value; or a rename for the driver case).
type markCase struct {
	name     string
	column   string // the approval column set to bad, then restored
	bad      any
	rename   bool // unreadable:driver: rename decision_at away and back
	wantMark string
	wantFail string
}

var markCases = []markCase{
	{name: "row_scan", column: "policy_version", bad: "x", wantMark: "corrupt:row_scan", wantFail: "approval_evidence_corrupt"},
	{name: "decision_at-unparseable", column: "decision_at", bad: "garbage", wantMark: "corrupt:decision_at", wantFail: "approval_evidence_corrupt"},
	{name: "decision_at-empty", column: "decision_at", bad: "", wantMark: "corrupt:decision_at", wantFail: "approval_evidence_corrupt"},
	{name: "decision_at-null", column: "decision_at", bad: nil, wantMark: "corrupt:decision_at", wantFail: "approval_evidence_corrupt"},
	{name: "decision_principal", column: "decision_principal_id", bad: "", wantMark: "corrupt:decision_principal", wantFail: "approval_evidence_corrupt"},
	{name: "decision_verb", column: "decision", bad: "bogus", wantMark: "corrupt:decision_verb", wantFail: "approval_evidence_corrupt"},
	{name: "approval_missing", column: "status", bad: "PENDING", wantMark: "corrupt:approval_missing", wantFail: "approval_evidence_corrupt"},
	{name: "status", column: "status", bad: "bogus", wantMark: "corrupt:status", wantFail: "approval_evidence_corrupt"},
	{name: "driver", rename: true, wantMark: "unreadable:driver", wantFail: "approval_evidence_unreadable"},
}

// sealMarkThroughRecovery approves, claims (params purged, action still
// APPROVED), makes the mark happen, runs RecoverPreviousLife, then repairs
// the row — so the verifiers meet a row that reads and coheres and only the
// receipt's mark can fail them. With prune it then deletes the action row with
// the schema's foreign keys ON, which is the statement shape Store.Prune runs
// (its cascade takes the approval and history rows). It is NOT Store.Prune:
// the retention cap is set only by the unexported openWithCap.
func sealMarkThroughRecovery(t *testing.T, mc markCase, prune bool) (cfgPath, receiptID, mark string) {
	t.Helper()
	cfgPath, dbPath, approvalID := parkedRequest(t)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	law, err := app.PolicyPinFor(cfg, "a")
	if err != nil {
		t.Fatalf("law: %v", err)
	}
	store, err := openOperatorStoreSealed(cfgPath)
	if err != nil {
		t.Fatalf("sealed store: %v", err)
	}
	ctx := context.Background()
	env, ident, err := operatorEnvelope("approval", "approve", `{"approval_id":"`+approvalID+`"}`)
	if err != nil {
		t.Fatalf("operator: %v", err)
	}
	if rule, err := store.DecideApprovalUnderLaw(ctx, approvalID, action.DecisionApproved,
		time.Now().UTC(), env, ident, "", law); err != nil || rule != "" {
		t.Fatalf("approve: %q %v", rule, err)
	}
	a, _, err := store.GetApproval(ctx, approvalID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if _, _, err := store.ClaimApprovalParamsUnderDigest(ctx, approvalID, &law, a.ActionDigest, nil); err != nil {
		t.Fatalf("claim: %v", err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatalf("second connection: %v", err)
	}
	defer func() { _ = db.Close() }()
	exec := func(stmt string, args ...any) {
		if _, err := db.Exec(stmt, args...); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	var original any
	switch {
	case mc.rename:
		exec(`ALTER TABLE approvals RENAME COLUMN decision_at TO decision_at_gone`)
	default:
		if err := db.QueryRow(`SELECT `+mc.column+` FROM approvals WHERE approval_id = ?`, approvalID).Scan(&original); err != nil {
			t.Fatalf("stash %s: %v", mc.column, err)
		}
		exec(`UPDATE approvals SET `+mc.column+` = ? WHERE approval_id = ?`, mc.bad, approvalID) // #nosec G202 -- test-owned column names
	}
	if _, err := store.RecoverPreviousLife(ctx); err != nil {
		t.Fatalf("recovery: %v", err)
	}
	rs, _ := store.ReceiptsByAction(ctx, "act_inbox1")
	_ = store.Close()
	if len(rs) != 1 {
		t.Fatalf("instrument: one recovered receipt expected, got %d", len(rs))
	}
	switch {
	case mc.rename:
		exec(`ALTER TABLE approvals RENAME COLUMN decision_at_gone TO decision_at`)
	default:
		exec(`UPDATE approvals SET `+mc.column+` = ? WHERE approval_id = ?`, original, approvalID) // #nosec G202 -- test-owned column names
	}
	// Instrument: the repaired row must read and cohere again, or a verifier
	// could fail through its unreadable-row arm instead of the mark.
	check, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	_, _, gerr := check.GetApproval(ctx, approvalID)
	_ = check.Close()
	if gerr != nil {
		t.Fatalf("instrument: the repaired approval row does not read: %v", gerr)
	}
	if prune {
		exec(`DELETE FROM actions WHERE action_id = 'act_inbox1'`)
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM approvals WHERE approval_id = ?`, approvalID).Scan(&n); err != nil || n != 0 {
			t.Fatalf("instrument: the cascade must take the approval row (n=%d err=%v)", n, err)
		}
	}
	return cfgPath, rs[0].ReceiptID, rs[0].ApprovalDigest
}

// TestV0151B_P2_6_sister_theVerifiersFailNamingEachMark: every code of the
// closed list, intact and pruned, meets both verifiers and fails as
// approval_evidence_corrupt / approval_evidence_unreadable. Never OK, never the
// NOTE approval_row_absent. (Values OUTSIDE the list: see
// TestV0151B_P2_6_sister_aValueOutsideTheListFailsClosed.)
//
// The oracle is the NAMED failure token in each verifier's own format.
//
// Evidence: in-process CLI against a real store file; row surgery and the
// cascade through a second real connection.
// Probing mutation (planned): let a verifier know only some codes, or skip a
// non-sha256 value ⇒ the rows of the forgotten codes red.
func TestV0151B_P2_6_sister_theVerifiersFailNamingEachMark(t *testing.T) {
	t.Parallel()
	for _, mc := range markCases {
		for _, prune := range []bool{false, true} {
			name := mc.name + "/intact-row"
			if prune {
				name = mc.name + "/pruned"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				cfgPath, receiptID, mark := sealMarkThroughRecovery(t, mc, prune)
				vcode, vout, verr := runIntentCLI(t, "receipt", "verify", "--config", cfgPath, receiptID)
				lcode, lout, lerr := runIntentCLI(t, "ledger", "check", "--config", cfgPath)
				if mark != mc.wantMark {
					t.Errorf("sealed approval_digest = %q, want exactly %q", mark, mc.wantMark)
				}
				verifyRe := regexp.MustCompile(`\): FAIL ` + mc.wantFail + `(:|\s|$)`)
				if vcode != 1 || !verifyRe.MatchString(vout) {
					t.Errorf("receipt verify: exit %d stdout %q stderr %q, want 1 with FAIL %s", vcode, vout, verr, mc.wantFail)
				}
				// Exclusivity (class i): the mark is the ONLY failure named for
				// this receipt — never the mark beside approval_mismatch. Asserted
				// on `receipt verify`, which prints every failure; `ledger check`
				// prints only the first failure of a receipt (ledger.go), so an
				// exclusivity assertion there could never fail and is not made.
				if n := strings.Count(vout, "): FAIL "); n > 1 {
					t.Errorf("receipt verify names %d failures for one receipt, want exactly 1: %q", n, vout)
				}
				ledgerRe := regexp.MustCompile(`FAIL at receipt [^\n]*: ` + mc.wantFail + `(:|\s|$)`)
				if lcode != 1 || !ledgerRe.MatchString(lout) {
					t.Errorf("ledger check: exit %d stdout %q stderr %q, want 1 with FAIL … %s", lcode, lout, lerr, mc.wantFail)
				}
			})
		}
	}
}

// TestV0151B_P2_6_sister_aValueOutsideTheListFailsClosed: the list is closed at
// the verifier too. A non-sha256 approval_digest that is not one of the
// closed-list marks fails closed as approval_evidence_unknown_mark.
//
// Such a value cannot be SEALED through the production doors — the store
// refuses a sealer that alters receipt fields (receipt_mutated_at_birth,
// observed while building this mould) — so this mould judges an in-memory
// receipt: a real recovery-sealed receipt with its approval_digest replaced,
// re-signed with the profile's real key and re-hashed, handed to
// verifyReceiptChecks, the ONE judgement function both `receipt verify` and
// `ledger check` call. An instrument asserts the re-seal is clean (no hash or
// signature failure), so only the value can be what fails.
//
// Evidence: in-process unit on the shared judgement function (not a stored
// receipt, not the verifier's CLI door).
// Probing mutation (planned): accept any "corrupt:"/"unreadable:" prefix, or
// skip a non-sha256 value ⇒ red.
func TestV0151B_P2_6_sister_aValueOutsideTheListFailsClosed(t *testing.T) {
	t.Parallel()
	// sha256:not-a-digest has the digest PREFIX and not its shape: a verifier
	// that accepts any sha256: prefix passes it.
	for _, stamp := range []string{"zzz:anything", "corrupt:not_in_the_list", "unreadable:not_in_the_list", "sha256:not-a-digest"} {
		for _, prune := range []bool{false, true} {
			name := stamp + "/intact-row"
			if prune {
				name = stamp + "/pruned"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				cfgPath, dbPath, approvalID := parkedRequest(t)
				// A real receipt sealed through the production CLI door (approve
				// executes calc and seals its outcome).
				if code, _, stderr := runIntentCLI(t, "approvals", "approve", "--config", cfgPath, approvalID); code != 0 {
					t.Fatalf("approve: %q", stderr)
				}
				cfg, err := config.Load(cfgPath)
				if err != nil {
					t.Fatalf("load: %v", err)
				}
				store, err := openOperatorStoreSealed(cfgPath)
				if err != nil {
					t.Fatalf("sealed store: %v", err)
				}
				defer func() { _ = store.Close() }()
				ctx := context.Background()
				priv, err := app.EnsureSigningKey(ctx, store, filepath.Dir(app.StoragePath(cfg)))
				if err != nil {
					t.Fatalf("signing key: %v", err)
				}
				rs, err := store.ReceiptsByAction(ctx, "act_inbox1")
				if err != nil || len(rs) != 1 {
					t.Fatalf("instrument: one sealed receipt expected: %v %d", err, len(rs))
				}
				if prune {
					// The statement shape Store.Prune runs, with the schema's
					// cascade (not Store.Prune: its cap is not settable here).
					db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=foreign_keys(on)")
					if err != nil {
						t.Fatalf("second connection: %v", err)
					}
					if _, err := db.Exec(`DELETE FROM actions WHERE action_id = 'act_inbox1'`); err != nil {
						t.Fatalf("prune: %v", err)
					}
					_ = db.Close()
				}
				r := rs[0]
				r.ApprovalDigest = stamp
				r = action.SignReceipt(priv, r)
				r.ReceiptHash = action.ComputeReceiptHash(r)
				failures, _ := verifyReceiptChecks(ctx, store, r)
				for _, f := range failures {
					if strings.HasPrefix(f, "hash_mismatch") || strings.Contains(f, "signature") {
						t.Fatalf("instrument: the re-seal is not clean: %v", failures)
					}
				}
				// Exactly ONE failure, and it is the named one (class i).
				if len(failures) != 1 || !strings.HasPrefix(failures[0], "approval_evidence_unknown_mark:") {
					t.Fatalf("failures = %v, want exactly [approval_evidence_unknown_mark: …]", failures)
				}
			})
		}
	}
}

// TestV0151B_P2_6_sister_aPlainOrphanVerifiesClean: the inverse half at the
// verifiers. An action that never went through approval, left AUTHORIZED by a
// crash — or in a pre-AUTHORIZED state an older life could leave (the
// recovery_legacy_test.go placement), or reusing the action id of an earlier
// approved and pruned life — is closed by the recovery with
// approval_digest "" and both verifiers answer OK. GREEN today, by design: it
// guards the cure against sealing corrupt:approval_missing over an action that
// never had an approval (which would fail the whole ledger check). Its probing
// mutations (M2b: decided on every recovery close; M2c: decided on every pass
// but the claimed-orphan one) are planned for the green step.
//
// Evidence: in-process CLI against a real store file, recovery through the
// CLI's own sealed store; the legacy state written through a second real
// connection.
func TestV0151B_P2_6_sister_aPlainOrphanVerifiesClean(t *testing.T) {
	t.Parallel()
	for _, shape := range []string{"authorized-orphan", "crash-orphan", "reused-id-recovery"} {
		t.Run(shape, func(t *testing.T) {
			t.Parallel()
			var cfgPath, dbPath string
			if shape == "reused-id-recovery" {
				// An earlier APPROVED life of act_plain_orphan, executed and
				// sealed through the production CLI door, then pruned by the
				// cascade (its tombstone survives). The plain life this row
				// then records reuses the id.
				var approvalID string
				cfgPath, dbPath, approvalID = parkedRequestWithID(t, "act_plain_orphan")
				if code, _, stderr := runIntentCLI(t, "approvals", "approve", "--config", cfgPath, approvalID); code != 0 {
					t.Fatalf("approve the earlier life: %q", stderr)
				}
				db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=foreign_keys(on)")
				if err != nil {
					t.Fatalf("second connection: %v", err)
				}
				if _, err := db.Exec(`DELETE FROM actions WHERE action_id = 'act_plain_orphan'`); err != nil {
					t.Fatalf("prune the earlier life: %v", err)
				}
				_ = db.Close()
			} else {
				cfgPath, dbPath = intentTestConfig(t)
			}
			store, err := openOperatorStoreSealed(cfgPath)
			if err != nil {
				t.Fatalf("sealed store: %v", err)
			}
			ctx := context.Background()
			env := action.NewEnvelope("act_plain_orphan", "env-plain",
				action.Source{Kind: "agent_brain", Protocol: "text", Channel: "console"},
				action.Operation{Namespace: "tool", Name: "calc", Version: 1},
				`7*6`, time.Now().UTC())
			if err := store.RecordAttempt(ctx, env, actionsqlite.Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized); err != nil {
				t.Fatalf("record: %v", err)
			}
			if shape == "crash-orphan" {
				db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
				if err != nil {
					t.Fatalf("second connection: %v", err)
				}
				if _, err := db.Exec(`UPDATE actions SET state = 'PREPARING' WHERE action_id = 'act_plain_orphan'`); err != nil {
					t.Fatalf("legacy state: %v", err)
				}
				_ = db.Close()
			}
			if _, err := store.RecoverPreviousLife(ctx); err != nil {
				t.Fatalf("recovery: %v", err)
			}
			rs, _ := store.ReceiptsByAction(ctx, "act_plain_orphan")
			_ = store.Close()
			if len(rs) == 0 {
				t.Fatal("instrument: no receipt for the plain orphan")
			}
			last := rs[len(rs)-1]
			if last.ApprovalDigest != "" {
				t.Fatalf("plain orphan approval_digest = %q, want \"\"", last.ApprovalDigest)
			}
			if code, out, errOut := runIntentCLI(t, "receipt", "verify", "--config", cfgPath, last.ReceiptID); code != 0 || !strings.Contains(out, ": OK") {
				t.Fatalf("receipt verify: exit %d %q %q, want 0 OK", code, out, errOut)
			}
			code, out, errOut := runIntentCLI(t, "ledger", "check", "--config", cfgPath)
			if shape == "reused-id-recovery" {
				// Scoped, declared: with a reused id `ledger check` stops at the
				// EARLIER life's receipt, which it fails (approval_mismatch:
				// «no approval row exists … while its action row remains») —
				// the pre-existing verifier defect this train filed. The walk
				// never reaches the receipt of the plain life, so ledger check
				// cannot judge it here: the plain life is judged by this row's
				// approval_digest == "" and `receipt verify` OK assertions.
				// While the filed defect stands, BOTH disjuncts of the
				// ledger-check assertion are unreachable (the walk stops
				// before the plain receipt and before any mark could be
				// printed); it is kept so the row reddens if the defect is
				// cured and the walk then fails the plain life or names a mark.
				if strings.Contains(out, "FAIL at receipt "+last.ReceiptID) || strings.Contains(out, "approval_evidence_") {
					t.Fatalf("ledger check failed the plain life or named a mark: %q %q", out, errOut)
				}
				return
			}
			if code != 0 || !strings.Contains(out, "chain intact") {
				t.Fatalf("ledger check: exit %d %q %q, want 0 chain intact", code, out, errOut)
			}
		})
	}
}

// parkedRequestWithID is parkedRequestExpiring with a caller-chosen action id,
// for the reused-id shape.
func parkedRequestWithID(t *testing.T, actionID string) (cfgPath, dbPath, approvalID string) {
	t.Helper()
	cfgPath, dbPath = intentTestConfig(t)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	law, err := app.PolicyPinFor(cfg, "a")
	if err != nil {
		t.Fatalf("law: %v", err)
	}
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	env := action.NewEnvelope(actionID, "env-reused",
		action.Source{Kind: "agent_brain", Protocol: "text", Channel: "console"},
		action.Operation{Namespace: "tool", Name: "calc", Version: 1},
		`7*6`, time.Now().UTC())
	env.IntentID = action.RootIntentID
	env.Principal = action.PrincipalRef{PrincipalID: "principal_brain_a"}
	env.Effect = action.Effect{Class: string(action.EffectPure)}
	now := time.Now().UTC()
	b, err := action.NewBoundApprovalRequest(env, `7*6`, action.ApprovalContext{
		IntentPurpose: "reused id", GrantID: "-", CostLine: "unbudgeted", ToolCage: "calc",
		Descriptor:    action.EffectDescriptor{Class: action.EffectPure, Reversible: true},
		HasDescriptor: true, LawVersion: law.Version, LawDigest: law.Digest,
		Rule: "require_approval", Now: now, TTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if err := store.CreateApprovalRequest(context.Background(), b); err != nil {
		t.Fatalf("park: %v", err)
	}
	return cfgPath, dbPath, b.Approval().ApprovalID
}

// TestV0151B_P2_1_sister_showEscapesTheStatus: the stored status (no CHECK
// constraint on the column), written with a U+202E through a second real
// connection, must print escaped in `show`, never raw. ADDED in the green step
// (diff pass #1, P3-b). `list` has no such row: it asks ListApprovals for the
// five known statuses one by one, so a rewritten status is never listed. The
// action digest has its own mould,
// TestV0151B_P2_1_sister_showEscapesACoherentlyRewrittenDigest.
//
// Evidence: in-process CLI against a real store file; the bytes are written
// through a second real connection.
// Probing mutation EXECUTED: print the status raw ⇒ red.
func TestV0151B_P2_1_sister_showEscapesTheStatus(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, stmt string
		verbs      []string
	}{
		{"status", `UPDATE approvals SET status = 'REJ' || char(8238) || 'ECTED' WHERE approval_id = ?`, []string{"show"}},
	} {
		for _, verb := range tc.verbs {
			t.Run(tc.name+"/"+verb, func(t *testing.T) {
				t.Parallel()
				cfgPath, dbPath, approvalID := parkedRequest(t)
				db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
				if err != nil {
					t.Fatalf("second connection: %v", err)
				}
				defer func() { _ = db.Close() }()
				if _, err := db.Exec(tc.stmt, approvalID); err != nil {
					t.Fatalf("attack %s: %v", tc.name, err)
				}
				code, stdout, stderr := runIntentCLI(t, "approvals", verb, "--config", cfgPath, approvalID)
				if code != 0 {
					t.Fatalf("%s: exit %d %q", verb, code, stderr)
				}
				if strings.Contains(stdout, "\u202e") {
					t.Fatalf("%s printed a raw U+202E: %q", verb, stdout)
				}
				if !strings.Contains(stdout, "<U+202E>") {
					t.Fatalf("%s must print the %s escaped; stdout = %q", verb, tc.name, stdout)
				}
			})
		}
	}
}

// TestV0151B_P2_1_sister_showEscapesACoherentlyRewrittenDigest: the preview
// belt (action.ValidatePreviewBinding) compares VALUES, not shape, and
// ParseCanonicalPreview does not validate ArgsDigest. A hand that rewrites the
// preview's args digest to "sha256:EVIL<U+202E>dcba", recomputes
// preview_digest, and sets action_digest to the same value passes every belt,
// and `show` prints that digest under «APPROVING EXACTLY THIS». It must print
// escaped, never raw. ADDED in the green step (delta re-pass of diff pass #1,
// P2 [INSTRUMENT]: the digest escape had no mould).
//
// Evidence: in-process CLI against a real store file; the rewrite goes
// through a second real connection.
// Probing mutation EXECUTED: print the action digest raw ⇒ red.
func TestV0151B_P2_1_sister_showEscapesACoherentlyRewrittenDigest(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath, approvalID := parkedRequest(t)
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
	if err != nil {
		t.Fatalf("second connection: %v", err)
	}
	defer func() { _ = db.Close() }()
	var raw string
	if err := db.QueryRow(`SELECT canonical_preview FROM approvals WHERE approval_id = ?`, approvalID).Scan(&raw); err != nil {
		t.Fatalf("read preview: %v", err)
	}
	p, err := action.ParseCanonicalPreview([]byte(raw))
	if err != nil {
		t.Fatalf("parse preview: %v", err)
	}
	const forged = "sha256:EVIL\u202edcba"
	p.ArgsDigest = forged
	if _, err := db.Exec(`UPDATE approvals SET canonical_preview = ?, preview_digest = ?, action_digest = ? WHERE approval_id = ?`,
		string(action.CanonicalPreview(p)), p.Digest(), forged, approvalID); err != nil {
		t.Fatalf("coherent rewrite: %v", err)
	}
	code, stdout, stderr := runIntentCLI(t, "approvals", "show", "--config", cfgPath, approvalID)
	if code != 0 {
		t.Fatalf("instrument: the coherent rewrite must pass every belt; show exit %d %q", code, stderr)
	}
	if strings.Contains(stdout, "\u202e") {
		t.Fatalf("show printed a raw U+202E: %q", stdout)
	}
	if !strings.Contains(stdout, "APPROVING EXACTLY THIS — digest: sha256:EVIL<U+202E>dcba") {
		t.Fatalf("show must print the digest escaped under its headline; stdout = %q", stdout)
	}
}

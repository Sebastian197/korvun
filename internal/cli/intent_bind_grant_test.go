// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
)

// The operator's door to delegation, attacked from the outside.
//
// THE FAULT. `korvun intent bind` wrote a binding whose three grant columns
// stayed NULL, and nothing else ever filled them. An operator could issue a
// signed grant, see it ACTIVE, and still have every start resolve through the
// config clause. `--grant` is that missing door; these moulds attack it at the
// only surface an operator touches.
//
// Evidence level, honest: in-process CLI (cli.Run over buffers) against a real
// SQLite file, read back through a second read-only connection. NOT the
// compiled binary in a separate OS process.
//
// The existing `TestIntentV2CLI_CreateActivateVerifyBind` is deliberately left
// untouched: it holds the contract for a bind WITHOUT a grant, which this piece
// must not change, and it counts receipts that a grant act would move.
func TestIntentBindGrantCLI_fillsTheBindingAndRevokesTheOldOne(t *testing.T) {
	cfg, dbPath, intent := authorityCLIFixture(t)
	file := writeAuthorityGrantFile(t, authorityCLIRoot(intent))
	if code, _, stderr := runIntentCLI(t, "authority", "issue", "--config", cfg, "--file", file); code != 0 {
		t.Fatalf("issue: code=%d stderr=%q", code, stderr)
	}

	// First the bind an operator has always been able to write: no grant.
	code, _, stderr := runIntentCLI(t, "intent", "bind", "--config", cfg,
		"--actor", "principal_operator", "--channel", "console", "int_cli_v2", "1")
	if code != 0 {
		t.Fatalf("plain bind: code=%d stderr=%q", code, stderr)
	}
	if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM execution_bindings
		WHERE status='ACTIVE' AND grant_id IS NULL`); n != 1 {
		t.Fatalf("bindings with no grant = %d, want the one a plain bind writes", n)
	}

	// And now the door. The grant is named by ID, which is all an operator has.
	code, stdout, stderr := runIntentCLI(t, "intent", "bind", "--config", cfg,
		"--actor", "principal_operator", "--channel", "console",
		"--grant", "grant_cli_root", "int_cli_v2", "1")
	if code != 0 {
		t.Fatalf("bind --grant: code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "ACTIVE under grant grant_cli_root") {
		t.Fatalf("stdout = %q, want the grant the binding now carries", stdout)
	}
	if !strings.Contains(stdout, "revoked binding ") {
		t.Fatalf("stdout = %q, want the binding this one replaced named", stdout)
	}

	// The row carries the WHOLE triple. A partial one is what
	// `bindingAuthorityTx` calls corrupt evidence, and the start would refuse it.
	if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM execution_bindings
		WHERE status='ACTIVE' AND grant_id='grant_cli_root'
		  AND grant_version=1 AND grant_digest IS NOT NULL AND grant_digest<>''`); n != 1 {
		t.Fatalf("ACTIVE bindings carrying the whole grant triple = %d, want 1", n)
	}
	// Exactly one ACTIVE row for the selector, and the old one kept as REVOKED
	// rather than overwritten: it is the evidence of what authorised yesterday.
	if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM execution_bindings WHERE status='ACTIVE'`); n != 1 {
		t.Fatalf("ACTIVE bindings = %d, want exactly 1", n)
	}
	if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM execution_bindings
		WHERE status='REVOKED' AND grant_id IS NULL`); n != 1 {
		t.Fatalf("the grant-less binding was not kept as REVOKED: %d rows", n)
	}
	if n := authorityCLICount(t, dbPath, `SELECT revision FROM execution_bindings
		WHERE status='ACTIVE'`); n != 2 {
		t.Fatalf("the new binding is at revision %d, want it counting on from 1", n)
	}

	// The act is an AUTHORITY act, closed with the truth of how it went. A
	// grant bind recorded as a plain operator act would leave the authority
	// history with a hole exactly where a delegation was attached.
	if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM actions
		WHERE op_namespace='authority' AND op_name='bind' AND state='SUCCEEDED'`); n != 1 {
		t.Fatalf("closed authority bind acts = %d, want 1", n)
	}
}

func TestIntentBindGrantCLI_refusesByName(t *testing.T) {
	// An EMPTY --grant must not be read as «no grant». Folding the two together
	// would hand the operator the config-clause path they were leaving, exit 0,
	// with no sign anything went wrong.
	t.Run("an empty --grant is a usage error, not a grant-less bind", func(t *testing.T) {
		cfg, dbPath, _ := authorityCLIFixture(t)
		code, stdout, _ := runIntentCLI(t, "intent", "bind", "--config", cfg,
			"--actor", "principal_operator", "--channel", "console",
			"--grant", "", "int_cli_v2", "1")
		if code != 2 {
			t.Fatalf("code = %d, want 2 (usage)", code)
		}
		if strings.Contains(stdout, "binding ") {
			t.Fatalf("an empty --grant wrote a binding: %q", stdout)
		}
		if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM execution_bindings`); n != 0 {
			t.Fatalf("bindings after the refusal = %d, want 0", n)
		}
	})

	t.Run("a grant that does not exist", func(t *testing.T) {
		cfg, dbPath, _ := authorityCLIFixture(t)
		code, _, stderr := runIntentCLI(t, "intent", "bind", "--config", cfg,
			"--actor", "principal_operator", "--channel", "console",
			"--grant", "grant_nowhere", "int_cli_v2", "1")
		if code != 1 {
			t.Fatalf("code = %d, want 1", code)
		}
		if !strings.Contains(stderr, "no grant \"grant_nowhere\"") {
			t.Fatalf("stderr = %q, want the grant named", stderr)
		}
		if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM execution_bindings`); n != 0 {
			t.Fatalf("bindings after the refusal = %d, want 0", n)
		}
		// The refusal is recorded, and recorded as a FAILURE. An authority act
		// left open, or closed SUCCEEDED, would read as a delegation that took.
		if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM actions
			WHERE op_namespace='authority' AND op_name='bind' AND state='FAILED'`); n != 1 {
			t.Fatalf("FAILED authority bind acts = %d, want 1", n)
		}
	})

	t.Run("a grant held by somebody else", func(t *testing.T) {
		cfg, dbPath, intent := authorityCLIFixture(t)
		file := writeAuthorityGrantFile(t, authorityCLIRoot(intent))
		if code, _, stderr := runIntentCLI(t, "authority", "issue", "--config", cfg, "--file", file); code != 0 {
			t.Fatalf("issue: code=%d stderr=%q", code, stderr)
		}
		code, _, stderr := runIntentCLI(t, "intent", "bind", "--config", cfg,
			"--actor", "principal_someone_else", "--channel", "console",
			"--grant", "grant_cli_root", "int_cli_v2", "1")
		if code != 1 {
			t.Fatalf("code = %d, want 1", code)
		}
		if !strings.Contains(stderr, "is held by \"principal_operator\", not by \"principal_someone_else\"") {
			t.Fatalf("stderr = %q, want both principals named", stderr)
		}
		if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM execution_bindings`); n != 0 {
			t.Fatalf("bindings after the refusal = %d, want 0", n)
		}
	})

	// THE CHANNEL, at the operator's own door. An adversarial pass drove exactly
	// this and got exit 0 with an ACTIVE row written, then watched every start
	// under it die. The fixture's grant carries only "console".
	t.Run("a channel the grant does not carry", func(t *testing.T) {
		cfg, dbPath, intent := authorityCLIFixture(t)
		file := writeAuthorityGrantFile(t, authorityCLIRoot(intent))
		if code, _, stderr := runIntentCLI(t, "authority", "issue", "--config", cfg, "--file", file); code != 0 {
			t.Fatalf("issue: code=%d stderr=%q", code, stderr)
		}
		code, stdout, stderr := runIntentCLI(t, "intent", "bind", "--config", cfg,
			"--actor", "principal_operator", "--channel", "webhook",
			"--grant", "grant_cli_root", "int_cli_v2", "1")
		if code != 1 {
			t.Fatalf("code = %d, want 1; stdout=%q", code, stdout)
		}
		if !strings.Contains(stderr, "webhook") {
			t.Fatalf("stderr = %q, want the channel the operator typed", stderr)
		}
		if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM execution_bindings`); n != 0 {
			t.Fatalf("bindings after the refusal = %d, want 0", n)
		}
	})

	// A flag the operator never wrote must not be blamed. `--channel --grant`
	// makes the parser read "--grant" as the CHANNEL's value, so `--grant` is
	// never set; a guard that scanned the raw arguments for the text said
	// otherwise and told the operator «--grant needs a grant id» about a flag
	// absent from their line.
	//
	// WHAT THIS MOULD DOES NOT CLAIM: that the line is refused. It is not — the
	// parser's reading is the ordinary one, so the command binds on a channel
	// literally named "--grant", by the path that takes no grant. That is plain
	// `flag` semantics and predates this door; refusing a channel that looks
	// like a flag is filed in docs/HANDOFF.md. What is cured, and what is
	// asserted here, is the DIAGNOSIS.
	t.Run("--grant named as another flag's value is not --grant", func(t *testing.T) {
		cfg, dbPath, _ := authorityCLIFixture(t)
		code, stdout, stderr := runIntentCLI(t, "intent", "bind", "--config", cfg,
			"--actor", "principal_operator", "--channel", "--grant", "int_cli_v2", "1")
		if strings.Contains(stderr, "--grant needs a grant id") {
			t.Fatalf("the operator never wrote --grant and was told they did: %q", stderr)
		}
		// And the line took the grant-LESS path, which is the parser's reading:
		// no grant on the row, and the channel is the text the parser assigned.
		if code != 0 {
			t.Fatalf("code = %d, want 0 (the parser's ordinary reading); stderr=%q", code, stderr)
		}
		if strings.Contains(stdout, "under grant") {
			t.Fatalf("a line with no --grant set bound one anyway: %q", stdout)
		}
		if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM execution_bindings
			WHERE channel='--grant' AND grant_id IS NULL`); n != 1 {
			t.Fatalf("rows bound on the parsed channel with no grant = %d, want 1", n)
		}
	})

	t.Run("a revoked grant", func(t *testing.T) {
		cfg, dbPath, intent := authorityCLIFixture(t)
		file := writeAuthorityGrantFile(t, authorityCLIRoot(intent))
		if code, _, stderr := runIntentCLI(t, "authority", "issue", "--config", cfg, "--file", file); code != 0 {
			t.Fatalf("issue: code=%d stderr=%q", code, stderr)
		}
		if code, _, stderr := runIntentCLI(t, "authority", "revoke", "--config", cfg,
			"--reason", "rotation", "grant_cli_root"); code != 0 {
			t.Fatalf("revoke: code=%d stderr=%q", code, stderr)
		}
		code, _, stderr := runIntentCLI(t, "intent", "bind", "--config", cfg,
			"--actor", "principal_operator", "--channel", "console",
			"--grant", "grant_cli_root", "int_cli_v2", "1")
		if code != 1 {
			t.Fatalf("code = %d, want 1", code)
		}
		if !strings.Contains(stderr, "authority revoked") {
			t.Fatalf("stderr = %q, want the revocation named", stderr)
		}
		if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM execution_bindings`); n != 0 {
			t.Fatalf("bindings after the refusal = %d, want 0", n)
		}
	})
}

// A COMMITTED WRITE IS NEVER REPORTED AS A REFUSAL.
//
// `recordAuthorityAct` closes the operator's act after the mutation has already
// committed. Its first shape returned the close's error as the command's, so a
// bind that had revoked the previous binding and written its replacement — both
// durable, one of them destructive — exited 1 printing a refusal, and neither
// the `revoked binding` line nor the `ACTIVE under grant` line was shown. The
// operator's belief and the ledger then disagreed permanently.
//
// It is the class `85013fd` cured for four store writers on 2026-09-22 («a
// committed write never reports the cadence's failure»). This door was the fifth
// caller and the first whose mutation destroys a previous row, which is why it
// is cured here rather than filed.
//
// The close is forced to fail by a trigger that aborts the UPDATE `Finish`
// performs — an oracle by impossibility, not a hope: the write below cannot
// close, and the command must still report what it did.
//
// Evidence level, honest: in-process CLI (`cli.Run`) against a real SQLite file,
// with a real trigger installed through a separate connection.
func TestIntentBindGrantCLI_aCommittedBindIsNotReportedAsARefusal(t *testing.T) {
	cfg, dbPath, intent := authorityCLIFixture(t)
	file := writeAuthorityGrantFile(t, authorityCLIRoot(intent))
	if code, _, stderr := runIntentCLI(t, "authority", "issue", "--config", cfg, "--file", file); code != 0 {
		t.Fatalf("issue: code=%d stderr=%q", code, stderr)
	}

	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	// Only the CLOSE is broken: the trigger fires on the state transition that
	// `Finish` writes, never on the insert that records the attempt.
	if _, err := db.Exec(`CREATE TRIGGER audit_block_finish BEFORE UPDATE ON actions
		WHEN NEW.state IN ('SUCCEEDED','FAILED')
		BEGIN SELECT RAISE(ABORT, 'the cadence cannot close this act'); END`); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runIntentCLI(t, "intent", "bind", "--config", cfg,
		"--actor", "principal_operator", "--channel", "console",
		"--grant", "grant_cli_root", "int_cli_v2", "1")

	// The binding IS there: the mutation committed before the close was tried.
	if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM execution_bindings
		WHERE status='ACTIVE' AND grant_id='grant_cli_root'`); n != 1 {
		t.Fatalf("ACTIVE bindings under the grant = %d, want 1: the premise of this mould is gone", n)
	}
	if code != 0 {
		t.Fatalf("a committed bind exited %d: the operator is told it failed; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "ACTIVE under grant grant_cli_root") {
		t.Fatalf("stdout = %q, want the binding the command actually wrote", stdout)
	}
	// And the failure is NOT swallowed: it is reported beside the success, so
	// the act left open is visible to whoever reads the output.
	if !strings.Contains(stderr, "close authority act") {
		t.Fatalf("stderr = %q, want the cadence failure reported beside the result", stderr)
	}
}

// THE SAME GUARANTEE ON THE OTHER AUTHORITY VERBS.
//
// `recordAuthorityAct` serves five commands and only `intent bind --grant` had a
// mould. An adversarial pass replaced its reporting with nil in the other four
// and the whole package stayed green — a cure whose wire exists in one of five
// places, with nothing watching the rest. The wire is structural now (it is a
// method on `*cli`, so there is no nil to pass), and these rows prove the
// behaviour on the verbs that actually revoke and issue authority.
//
// `revoke` is the one that matters most: it is durable and destructive, and an
// operator who reads "failed" over a revocation that stands will re-issue
// authority that was deliberately taken away.
func TestAuthorityCLI_aCommittedActIsNotReportedAsARefusal(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(t *testing.T, cfg, dbPath string, intent action.IntentContractV2) (int, string, string)
		then func(t *testing.T, dbPath string)
	}{
		{
			name: "authority issue",
			run: func(t *testing.T, cfg, dbPath string, intent action.IntentContractV2) (int, string, string) {
				file := writeAuthorityGrantFile(t, authorityCLIRoot(intent))
				return runIntentCLI(t, "authority", "issue", "--config", cfg, "--file", file)
			},
			then: func(t *testing.T, dbPath string) {
				if n := authorityCLICount(t, dbPath,
					`SELECT COUNT(*) FROM grant_heads WHERE grant_id='grant_cli_root' AND status='ACTIVE'`); n != 1 {
					t.Fatalf("the grant is not ACTIVE (%d): the premise of this row is gone", n)
				}
			},
		},
		{
			name: "authority revoke",
			run: func(t *testing.T, cfg, dbPath string, intent action.IntentContractV2) (int, string, string) {
				file := writeAuthorityGrantFile(t, authorityCLIRoot(intent))
				if code, _, stderr := runIntentCLI(t, "authority", "issue", "--config", cfg, "--file", file); code != 0 {
					t.Fatalf("issue: code=%d stderr=%q", code, stderr)
				}
				armCloseTrigger(t, dbPath)
				return runIntentCLI(t, "authority", "revoke", "--config", cfg,
					"--reason", "rotation", "grant_cli_root")
			},
			then: func(t *testing.T, dbPath string) {
				if n := authorityCLICount(t, dbPath,
					`SELECT COUNT(*) FROM grant_heads WHERE grant_id='grant_cli_root' AND status='REVOKED'`); n != 1 {
					t.Fatalf("the revocation did not commit (%d): the premise of this row is gone", n)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, dbPath, intent := authorityCLIFixture(t)
			if tc.name == "authority issue" {
				armCloseTrigger(t, dbPath)
			}
			code, _, stderr := tc.run(t, cfg, dbPath, intent)
			tc.then(t, dbPath)
			if code != 0 {
				t.Fatalf("a committed authority act exited %d: the operator is told it failed; stderr=%q", code, stderr)
			}
			if !strings.Contains(stderr, "close authority act") {
				t.Fatalf("stderr = %q, want the cadence failure reported beside the result", stderr)
			}
		})
	}
}

// armCloseTrigger breaks ONLY the close: the trigger fires on the state
// transition `Finish` writes and on nothing else, so the act's own mutation
// commits and its closing does not.
func armCloseTrigger(t *testing.T, dbPath string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TRIGGER audit_block_finish BEFORE UPDATE ON actions
		WHEN NEW.state IN ('SUCCEEDED','FAILED')
		BEGIN SELECT RAISE(ABORT, 'the cadence cannot close this act'); END`); err != nil {
		t.Fatal(err)
	}
}

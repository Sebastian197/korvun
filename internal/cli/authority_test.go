// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
)

// authorityCLIFixture creates and activates the v2 intent the authority verbs
// issue under, and returns the config path, the database path and the intent.
func authorityCLIFixture(t *testing.T) (string, string, action.IntentContractV2) {
	t.Helper()
	cfg, dbPath := intentTestConfig(t)
	file := writeIntentV2Fixture(t, t.TempDir())
	for _, args := range [][]string{
		{"intent", "create-v2", "--config", cfg, "--file", file},
		{"intent", "activate-v2", "--config", cfg, "int_cli_v2", "1"},
	} {
		if code, _, stderr := runIntentCLI(t, args...); code != 0 {
			t.Fatalf("%s: code=%d stderr=%q", strings.Join(args[:2], " "), code, stderr)
		}
	}
	store, err := actionsqlite.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	signed, err := store.GetIntentV2(context.Background(), "int_cli_v2", 1)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, dbPath, signed.Contract
}

func authorityCLIRoot(intent action.IntentContractV2) action.AuthorityGrantV2 {
	return action.AuthorityGrantV2{
		GrantID: "grant_cli_root", SchemaVersion: 2, Version: 1, ProfileID: intent.ProfileID,
		IntentID: intent.IntentID, IntentVersion: intent.Version, IntentDigest: intent.Digest(),
		IssuerPrincipalID: intent.OwnerPrincipalID, SubjectPrincipalID: intent.OwnerPrincipalID,
		Operations: append([]action.OperationRef(nil), intent.Operations...), Channels: []string{"console"},
		AllowedResources:   append([]action.ResourceRef(nil), intent.AllowedResources...),
		AllowedData:        append([]string(nil), intent.DataScope...),
		OutputDestinations: append([]string(nil), intent.OutputDestinations...),
		EffectClasses:      append([]action.EffectClass(nil), intent.EffectClasses...),
		EffectCeiling:      action.EffectReadExternal,
		ValidFrom:          intent.ValidFrom, ExpiresAt: intent.ExpiresAt,
		DelegationDepthRemaining: 0, Status: action.LifecycleActive,
	}
}

func writeAuthorityGrantFile(t *testing.T, grant action.AuthorityGrantV2) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), grant.GrantID+".json")
	if err := os.WriteFile(path, grant.CanonicalBytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func authorityCLICount(t *testing.T, dbPath, query string, args ...any) int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// TestAuthorityCLI_OperatorDoor drives the authority verbs through cli.Run — the
// door an operator actually uses — and reads the database afterwards through a
// separate read-only connection. Each verb must leave BOTH things: the authority
// mutation, and the operator's own identified act that authorized it, closed
// with the truth of how it went.
//
// Evidence level: in-process CLI (cli.Run over buffers) against a real SQLite
// file, read back through a second, read-only connection. NOT the compiled
// binary in a separate OS process.
// Probing mutation executed: close the operator's act SUCCEEDED whatever the
// mutation returned — red on both refusal rows: «FAILED revoke acts = 0, want
// 1» and «FAILED issue acts = 0, want 1».
func TestAuthorityCLI_OperatorDoor(t *testing.T) {
	t.Run("issue, then revoke: each leaves its mutation and its closed operator act", func(t *testing.T) {
		cfg, dbPath, intent := authorityCLIFixture(t)
		file := writeAuthorityGrantFile(t, authorityCLIRoot(intent))

		code, stdout, stderr := runIntentCLI(t, "authority", "issue", "--config", cfg, "--file", file)
		if code != 0 || !strings.Contains(stdout, "authority grant grant_cli_root issued") {
			t.Fatalf("issue: code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
		if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM grant_heads WHERE grant_id='grant_cli_root' AND status='ACTIVE'`); n != 1 {
			t.Errorf("active root grants = %d, want 1", n)
		}
		if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM actions a JOIN grant_events e ON e.actor_action_id=a.action_id
			WHERE a.op_namespace='authority' AND a.op_name='issue' AND a.state='SUCCEEDED' AND e.grant_id='grant_cli_root'`); n != 1 {
			t.Errorf("closed operator acts the issuance points to = %d, want 1", n)
		}

		code, stdout, stderr = runIntentCLI(t, "authority", "revoke", "--config", cfg, "--reason", "rotation", "grant_cli_root")
		if code != 0 || !strings.Contains(stdout, "authority grant grant_cli_root revoked") {
			t.Fatalf("revoke: code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
		if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM grant_heads WHERE grant_id='grant_cli_root' AND status='REVOKED'`); n != 1 {
			t.Errorf("revoked root grants = %d, want 1", n)
		}
		// A second revocation is refused, and the refusal is recorded as a FAILED act.
		code, _, stderr = runIntentCLI(t, "authority", "revoke", "--config", cfg, "--reason", "again", "grant_cli_root")
		if code != 1 || !strings.Contains(stderr, "authority revoked") {
			t.Errorf("second revoke: code=%d stderr=%q, want 1 naming the revocation", code, stderr)
		}
		if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM actions WHERE op_namespace='authority' AND op_name='revoke' AND state='FAILED'`); n != 1 {
			t.Errorf("FAILED revoke acts = %d, want 1: a refused act must not close as succeeded", n)
		}
	})

	t.Run("a grant wider than its intent is refused by name and leaves no grant", func(t *testing.T) {
		cfg, dbPath, intent := authorityCLIFixture(t)
		wide := authorityCLIRoot(intent)
		wide.Operations = append(wide.Operations, action.OperationRef{Namespace: "tool", Name: "write_file", Version: 1})
		file := writeAuthorityGrantFile(t, wide)

		code, _, stderr := runIntentCLI(t, "authority", "issue", "--config", cfg, "--file", file)
		if code != 1 || !strings.Contains(stderr, "delegation widens authority") || !strings.Contains(stderr, "operations") {
			t.Errorf("issue: code=%d stderr=%q, want 1 naming the widened dimension", code, stderr)
		}
		if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM grant_versions`); n != 0 {
			t.Errorf("grant rows = %d, want 0", n)
		}
		if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM actions WHERE op_namespace='authority' AND op_name='issue' AND state='FAILED'`); n != 1 {
			t.Errorf("FAILED issue acts = %d, want 1", n)
		}
	})

	t.Run("a delegation under a root that allows none is refused by the depth dimension", func(t *testing.T) {
		cfg, dbPath, intent := authorityCLIFixture(t)
		root := authorityCLIRoot(intent)
		root.DelegationDepthRemaining = 0
		if code, _, stderr := runIntentCLI(t, "authority", "issue", "--config", cfg, "--file", writeAuthorityGrantFile(t, root)); code != 0 {
			t.Fatalf("issue root: %d %q", code, stderr)
		}
		child := root
		child.GrantID, child.ParentGrantID, child.ParentGrantVersion = "grant_cli_child", root.GrantID, root.Version
		child.IssuerPrincipalID, child.SubjectPrincipalID = root.SubjectPrincipalID, "principal_brain"
		code, _, stderr := runIntentCLI(t, "authority", "delegate", "--config", cfg, "--file", writeAuthorityGrantFile(t, child))
		// The root allows no further delegation (depth 0), so the child is a
		// widening in the depth dimension and must be refused by that name.
		if code != 1 || !strings.Contains(stderr, "depth") {
			t.Errorf("delegate: code=%d stderr=%q, want 1 naming the depth dimension", code, stderr)
		}
		if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM grant_versions WHERE grant_id='grant_cli_child'`); n != 0 {
			t.Errorf("child rows = %d, want 0", n)
		}
	})

	t.Run("activate pins a digest once, and refuses to activate again", func(t *testing.T) {
		cfg, dbPath, _ := authorityCLIFixture(t)
		code, stdout, stderr := runIntentCLI(t, "authority", "activate", "--config", cfg, "--profile", "profile_cli", "--reason", "enable strict mode")
		if code != 0 || !strings.Contains(stdout, "activation_digest: sha256:") {
			t.Fatalf("activate: code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
		if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM approval_birth_heads WHERE profile_id='profile_cli'`); n != 1 {
			t.Errorf("activation roots = %d, want 1", n)
		}
		code, _, stderr = runIntentCLI(t, "authority", "activate", "--config", cfg, "--profile", "profile_cli", "--reason", "again")
		if code != 1 || !strings.Contains(stderr, "authorization snapshot corrupt") {
			t.Errorf("second activate: code=%d stderr=%q, want 1", code, stderr)
		}
		if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM approval_birth_heads`); n != 1 {
			t.Errorf("activation roots after the refused second act = %d, want 1", n)
		}
	})

	t.Run("an unreadable or malformed grant file is refused before any store is opened", func(t *testing.T) {
		cfg, dbPath, _ := authorityCLIFixture(t)
		malformed := filepath.Join(t.TempDir(), "malformed.json")
		if err := os.WriteFile(malformed, []byte(`{"grant_id":"x"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, file := range []string{filepath.Join(t.TempDir(), "absent.json"), malformed} {
			for _, verb := range []string{"issue", "delegate"} {
				if code, _, _ := runIntentCLI(t, "authority", verb, "--config", cfg, "--file", file); code != 1 {
					t.Errorf("%s %s: code=%d, want 1", verb, filepath.Base(file), code)
				}
			}
			if code, _, _ := runIntentCLI(t, "authority", "import-v1", "--config", cfg, "--file", file, "--legacy-grant", "grant_x", "--reason", "r"); code != 1 {
				t.Errorf("import-v1 %s: code=%d, want 1", filepath.Base(file), code)
			}
		}
		if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM actions WHERE op_namespace='authority'`); n != 0 {
			t.Errorf("operator acts recorded for unreadable files = %d, want 0", n)
		}
	})
}

// TestAuthorityCLI_UsageIsExitTwo: every verb refuses an incomplete command line
// with exit 2 BEFORE touching the profile, and the two informational forms
// answer without error.
//
// Evidence level: in-process CLI; no store is opened by any row.
func TestAuthorityCLI_UsageIsExitTwo(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{"no subcommand", []string{"authority"}, 2},
		{"unknown subcommand", []string{"authority", "frobnicate"}, 2},
		{"help", []string{"authority", "--help"}, 0},
		{"activate without a profile", []string{"authority", "activate", "--config", "x", "--reason", "r"}, 2},
		{"activate with a stray argument", []string{"authority", "activate", "--config", "x", "--profile", "p", "--reason", "r", "stray"}, 2},
		{"issue without a file", []string{"authority", "issue", "--config", "x"}, 2},
		{"admin-issue without a reason", []string{"authority", "admin-issue", "--config", "x", "--file", "f"}, 2},
		{"admin-delegate without a reason", []string{"authority", "admin-delegate", "--config", "x", "--file", "f"}, 2},
		{"revoke without a grant id", []string{"authority", "revoke", "--config", "x", "--reason", "r"}, 2},
		{"admin-revoke without a reason", []string{"authority", "admin-revoke", "--config", "x", "grant_x"}, 2},
		{"import-v1 without a legacy grant", []string{"authority", "import-v1", "--config", "x", "--file", "f", "--reason", "r"}, 2},
		{"an unknown flag", []string{"authority", "issue", "--nope"}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if code, _, _ := runIntentCLI(t, tt.args...); code != tt.want {
				t.Errorf("exit = %d, want %d", code, tt.want)
			}
		})
	}
}

// TestAuthorityCLI_AdministrativeDoorNamesTheOperator: the administrative verbs
// are separate doors. They keep the grant's issuer as the file states it and
// record the authenticated HUMAN operator, with a reason, as the actor of the
// signed lifecycle event — never the other way round.
//
// Evidence level: in-process CLI over a real SQLite file, read back through a
// second, read-only connection.
// Probing mutation executed: route `admin-issue` to the ordinary issue door —
// red, and by a belt worth naming: the one-shot act was recorded over the
// ADMINISTRATIVE canonical parameters, so the ordinary door refuses it as
// «ingress binding mismatch». An act authorized for one door does not open
// another.
func TestAuthorityCLI_AdministrativeDoorNamesTheOperator(t *testing.T) {
	cfg, dbPath, intent := authorityCLIFixture(t)
	file := writeAuthorityGrantFile(t, authorityCLIRoot(intent))

	code, stdout, stderr := runIntentCLI(t, "authority", "admin-issue", "--config", cfg, "--file", file, "--reason", "recover an externally agreed grant")
	if code != 0 || !strings.Contains(stdout, "authority grant grant_cli_root issued") {
		t.Fatalf("admin-issue: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM grant_events WHERE grant_id='grant_cli_root'
		AND administrative=1 AND reason='recover an externally agreed grant' AND actor_principal_id!=''`); n != 1 {
		t.Errorf("administrative issue events carrying the operator and the reason = %d, want 1", n)
	}
	if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM grant_events e JOIN principals p ON p.principal_id=e.actor_principal_id
		WHERE e.grant_id='grant_cli_root' AND p.kind='human'`); n != 1 {
		t.Errorf("issue events whose actor is a HUMAN principal = %d, want 1", n)
	}

	code, stdout, stderr = runIntentCLI(t, "authority", "admin-revoke", "--config", cfg, "--reason", "withdrawn by the operator", "grant_cli_root")
	if code != 0 || !strings.Contains(stdout, "revoked") {
		t.Fatalf("admin-revoke: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM grant_events WHERE grant_id='grant_cli_root'
		AND to_status='REVOKED' AND administrative=1 AND reason='withdrawn by the operator'`); n != 1 {
		t.Errorf("administrative revocation events = %d, want 1", n)
	}
}

// TestAuthorityCLI_OrdinaryDelegationRefusesAForgedIssuer: through the ORDINARY
// delegation door the issuer is whoever authenticated, not whoever the file
// names. A child whose file claims another issuer is refused BY NAME, leaves no
// row, and the operator's act is closed FAILED.
//
// Evidence level: in-process CLI over a real SQLite file, read back through a
// second, read-only connection.
// Probing mutation executed: route `delegate` to the administrative door — red:
// the refusal is no longer the issuer mismatch this door owes.
func TestAuthorityCLI_OrdinaryDelegationRefusesAForgedIssuer(t *testing.T) {
	cfg, dbPath, intent := authorityCLIFixture(t)
	root := authorityCLIRoot(intent)
	root.DelegationDepthRemaining = 0
	if code, _, stderr := runIntentCLI(t, "authority", "issue", "--config", cfg, "--file", writeAuthorityGrantFile(t, root)); code != 0 {
		t.Fatalf("issue root: %d %q", code, stderr)
	}
	child := root
	child.GrantID, child.ParentGrantID, child.ParentGrantVersion = "grant_cli_child", root.GrantID, root.Version
	child.IssuerPrincipalID, child.SubjectPrincipalID = "principal_someone_else", "principal_brain"
	code, _, stderr := runIntentCLI(t, "authority", "delegate", "--config", cfg, "--file", writeAuthorityGrantFile(t, child))
	if code != 1 || !strings.Contains(stderr, "issuer mismatch") {
		t.Errorf("delegate with a forged issuer: code=%d stderr=%q, want 1 naming the issuer mismatch", code, stderr)
	}
	if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM grant_versions WHERE grant_id='grant_cli_child'`); n != 0 {
		t.Errorf("child rows = %d, want 0", n)
	}
	if n := authorityCLICount(t, dbPath, `SELECT COUNT(*) FROM actions WHERE op_namespace='authority' AND op_name='delegate' AND state='FAILED'`); n != 1 {
		t.Errorf("FAILED delegate acts = %d, want 1", n)
	}
}
